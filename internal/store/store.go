// Package store defines the StorageDriver interface — the keystone of the
// two-tier spin-out. One contract, two implementations: SQLite (local,
// zero-infra) and Postgres (hosted, org-wide). Tier selection is a one-line
// swap at Open().
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
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
	LatestSnapshot(ctx context.Context, repoID string) (*graph.Snapshot, error)
	ListSnapshots(ctx context.Context, repoID string, limit int) ([]graph.Snapshot, error)

	// graph reads (feed search / impact / neighbors / path)
	ListSymbols(ctx context.Context, snapshotID string) ([]graph.CodeSymbol, error)
	ListEdges(ctx context.Context, snapshotID string) ([]graph.DependencyEdge, error)
	ListRoutes(ctx context.Context, snapshotID, role string) ([]graph.Route, error)
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
