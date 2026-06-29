// Package store defines the StorageDriver interface — the keystone of the
// two-tier spin-out. One contract, two implementations: SQLite (local,
// zero-infra) and Postgres (hosted, org-wide). Tier selection is a one-line
// swap at Open().
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

// ErrNotImplemented is the scaffold sentinel.
var ErrNotImplemented = errors.New("store: not implemented")

// Capabilities advertises what a driver/tier supports, used for feature gating.
type Capabilities struct {
	DurableQueue    bool
	CrossScope      bool
	ConcurrentWrite bool
	PushReindex     bool // push-driven delta reindex; Pulse/CI triggers it, Atlas owns the indexing
}

// StorageDriver is the single persistence contract. The full engine adds the
// rest of the catalog's read/write methods (cross-repo, coverage, index
// lifecycle); the scaffold pins the lifecycle + the core snapshot/graph reads so
// the shape is concrete and compiling.
type StorageDriver interface {
	// lifecycle
	Migrate(ctx context.Context) error
	Dialect() string // "sqlite" | "postgres"
	Capabilities() Capabilities
	Close() error

	// repos
	EnsureRepo(ctx context.Context, r *graph.Repo) (*graph.Repo, error)
	ListRepos(ctx context.Context, scope string) ([]graph.Repo, error)

	// snapshots (temporal moat)
	SaveSnapshot(ctx context.Context, s *graph.Snapshot, files []graph.File,
		symbols []graph.CodeSymbol, edges []graph.DependencyEdge, routes []graph.Route) error
	UpdateSnapshotMetadata(ctx context.Context, snapshotID string, metadata graph.JSONBMap) error
	LatestSnapshot(ctx context.Context, repoID string) (*graph.Snapshot, error)
	ListSnapshots(ctx context.Context, repoID string, limit int) ([]graph.Snapshot, error)

	// graph reads (feed search / impact / neighbors / path)
	ListSymbols(ctx context.Context, snapshotID string) ([]graph.CodeSymbol, error)
	ListEdges(ctx context.Context, snapshotID string) ([]graph.DependencyEdge, error)
	ListRoutes(ctx context.Context, snapshotID, role string) ([]graph.Route, error)
	// ListFiles returns the indexed file rows of a snapshot (path/language/imports),
	// feeding explain's defining-file import bundle.
	ListFiles(ctx context.Context, snapshotID string) ([]graph.File, error)
	// FilesByPaths is the indexed, batched form used by latency-sensitive context
	// assembly when only a small set of files is needed.
	FilesByPaths(ctx context.Context, snapshotID string, paths []string) ([]graph.File, error)

	// indexed graph reads (scale impact to the blast radius, not the whole repo)
	//
	// SymbolsByName returns symbols with an exact name match, using the
	// (snapshot_id, name) index. SymbolsByPath does the same on path. Both feed
	// the reverse-BFS seed/expansion so a query touches only the rows it needs.
	SymbolsByName(ctx context.Context, snapshotID, name string) ([]graph.CodeSymbol, error)
	// SymbolsByNames is the batched form of SymbolsByName: it returns every symbol
	// whose name is in the given set, in ONE chunked IN-list query (mirroring
	// CallEdgesByToRefs) instead of one round-trip per name. It backs the impact
	// reverse-BFS, which would otherwise issue thousands of one-name point queries
	// against a hub symbol's blast radius. Uses idx_symbols_snapshot_name; no dedupe.
	SymbolsByNames(ctx context.Context, snapshotID string, names []string) ([]graph.CodeSymbol, error)
	SymbolsByPath(ctx context.Context, snapshotID, path string) ([]graph.CodeSymbol, error)
	// SymbolsByIDs returns every symbol whose id is in the given set, served by the
	// symbols primary key (id) scoped to snapshot_id. The IN-list is chunked to
	// stay under SQLite's bound-parameter limit (mirroring SymbolsByNames). It is
	// the targeted counterpart to ListSymbols for the search/context paths, which
	// only need to resolve a handful of known hit ids rather than loading every
	// symbol in the snapshot. Rows carry node_id + decoded metadata (no dedupe).
	SymbolsByIDs(ctx context.Context, snapshotID string, ids []string) ([]graph.CodeSymbol, error)
	// CallEdgesByToRefs returns every "calls" edge whose to_ref is in the given
	// set, using the (snapshot_id, to_ref) index. The IN-list is chunked to stay
	// under SQLite's bound-parameter limit; all matching edges are returned with
	// Metadata populated (no dedupe).
	CallEdgesByToRefs(ctx context.Context, snapshotID string, toRefs []string) ([]graph.DependencyEdge, error)
	// CallEdgesByFromSymbols returns every "calls" edge whose from_symbol is in the
	// given set, using the (snapshot_id, from_symbol) index — the callees side, for
	// the `symbol` op's outgoing-call context. Same chunking/Metadata semantics.
	CallEdgesByFromSymbols(ctx context.Context, snapshotID string, fromSymbols []string) ([]graph.DependencyEdge, error)
	// RefEdgesByToRefs returns every "references" (type-use) edge whose to_ref is in
	// the given set, using the (snapshot_id, to_ref) index. It mirrors
	// CallEdgesByToRefs exactly but filters kind="references" instead of "calls", so
	// `refs` can return TRUE type-use references alongside call-site callers. Same
	// chunking/Metadata semantics (no dedupe).
	RefEdgesByToRefs(ctx context.Context, snapshotID string, toRefs []string) ([]graph.DependencyEdge, error)
	// EdgesByFromFiles returns every edge (any kind) whose from_file is in the
	// given set, served by idx_edges_snapshot_fromfile. The IN-list is chunked to
	// stay under SQLite's bound-parameter limit. It is the targeted counterpart to
	// ListEdges for the `context` op, which only needs edges originating in the
	// changed paths rather than the whole snapshot's edge set. Rows carry decoded
	// Metadata (no dedupe).
	EdgesByFromFiles(ctx context.Context, snapshotID string, fromFiles []string) ([]graph.DependencyEdge, error)

	// coverage (runtime test-coverage facts)
	//
	// SaveCoverage replaces the coverage rows for the affected snapshot(s) and
	// persists the given runtime coverage facts (symbol<->span coverage from an
	// ingested coverprofile / LCOV). ListCoverage returns the stored coverage rows
	// for a snapshot, optionally filtered to a single symbol name (empty = all).
	SaveCoverage(ctx context.Context, rows []graph.Coverage) error
	ListCoverage(ctx context.Context, snapshotID, symbolName string) ([]graph.Coverage, error)

	// embeddings (OPTIONAL semantic-search substrate; written only when vectors
	// are enabled — the deterministic core never touches these)
	//
	// SaveEmbeddings replaces the embedding rows for snapshotID (delete-then-insert)
	// and persists the given per-symbol vectors. NearestSymbols loads the snapshot's
	// vectors, scores each by cosine (== dot, since both sides are L2-normalized)
	// against vec, keeps Score>=minScore, and returns the top-`limit` descending.
	SaveEmbeddings(ctx context.Context, snapshotID string, embs []graph.SymbolEmbedding) error
	NearestSymbols(ctx context.Context, snapshotID string, vec []float32, limit int, minScore float64) ([]graph.ScoredSymbol, error)
}

// Options configures Open().
type Options struct {
	Kind        string // "sqlite" | "postgres"
	SQLitePath  string
	PostgresDSN string
}

// Open returns the StorageDriver for the requested tier. This is the keystone
// swap: the rest of the engine is written only against the interface.
func Open(ctx context.Context, opts Options) (StorageDriver, error) {
	switch opts.Kind {
	case "", "sqlite":
		return openSQLite(ctx, opts.SQLitePath)
	case "postgres":
		return openPostgres(ctx, opts.PostgresDSN)
	default:
		return nil, fmt.Errorf("store: unknown driver %q", opts.Kind)
	}
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
