// Package engine is the composition root. It defines the Engine interface — one
// method per canonical operation in the Atlas catalog — and the local-tier
// implementation that wires the real packages together: parser + index (write
// path), store (persistence), lexical (search), and query (graph traversal).
//
// Atlas is the deterministic code-intelligence layer: every method here is a
// pure function of the indexed graph. No LLM, no embeddings on the core path.
package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/index"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/lexical"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/query"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

// ErrNotImplemented is the sentinel for ops not yet wired.
var ErrNotImplemented = errors.New("atlas: not implemented")

// ErrTierUnsupported is returned when a hosted-only op runs on a local engine.
var ErrTierUnsupported = errors.New("atlas: operation requires hosted tier")

// ErrNoIndex is returned when a read op runs before anything has been indexed.
var ErrNoIndex = errors.New("atlas: no indexed repo found (run `atlas index` first)")

// ── Canonical I/O types (one Input/Result pair per op) ──────────────────────

type IndexInput struct {
	ProjectPath   string
	Repo          string
	Ref           string
	Base          string
	Langs         []string
	Reindex       bool
	EnableVectors bool
}

type IndexResult struct {
	RepoID       string         `json:"repo_id"`
	RepoFullName string         `json:"repo_full_name"`
	SnapshotID   string         `json:"snapshot_id"`
	CommitSHA    string         `json:"commit_sha"`
	IndexedFiles int            `json:"indexed_files"`
	Symbols      int            `json:"symbols"`
	Edges        int            `json:"edges"`
	Routes       int            `json:"routes"`
	Languages    map[string]int `json:"languages"`
	Mode         string         `json:"mode"`
	DurationMS   int64          `json:"duration_ms"`
}

type SearchInput struct {
	Query    string
	RepoID   string
	Kind     string
	PathGlob string
	Limit    int
	Mode     string // lexical | semantic | hybrid (semantic/hybrid require vectors)
}

type SearchHit struct {
	SymbolID  string  `json:"symbol_id"`
	Name      string  `json:"symbol"`
	Kind      string  `json:"kind"`
	RepoID    string  `json:"repo_id"`
	Path      string  `json:"path"`
	Line      int     `json:"line"`
	Signature string  `json:"signature"`
	Doc       string  `json:"doc,omitempty"`
	Score     float64 `json:"score"`
}

type SearchResult struct {
	Results  []SearchHit `json:"results"`
	ModeUsed string      `json:"mode_used"`
	Total    int         `json:"total"`
}

type ImpactInput struct {
	Symbols      []string
	ChangedPaths []string
	Diff         string
	RepoID       string
	MaxDepth     int
	IncludeTests bool
}

type FileImpact struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ImpactResult struct {
	ImpactedSymbols []string     `json:"impacted_symbols"`
	ImpactedFiles   []FileImpact `json:"impacted_files"`
	ImpactedTests   []string     `json:"impacted_tests"`
	DepthReached    int          `json:"depth_reached"`
}

type StatusInput struct {
	RepoID  string
	Verbose bool
}

type RepoStatus struct {
	RepoID    string `json:"repo_id"`
	FullName  string `json:"repo_full_name"`
	Snapshot  string `json:"snapshot_id"`
	CommitSHA string `json:"commit_sha"`
	Symbols   int    `json:"symbols"`
	Edges     int    `json:"edges"`
	IndexedAt string `json:"indexed_at"`
}

type StatusResult struct {
	Tier          string       `json:"tier"`
	StorageDriver string       `json:"storage_driver"`
	VectorBackend string       `json:"vector_backend"`
	ReposIndexed  int          `json:"repos_indexed"`
	Repos         []RepoStatus `json:"repos"`
}

// ── The Engine interface ────────────────────────────────────────────────────

// Engine is the single contract all surfaces depend on. The full catalog
// (callers, refs, neighbors, path, explain, graph_export, cross_repo_impact,
// consumers, route_contracts, history, snapshot_diff, coverage, repos, link)
// extends this interface following the same pattern.
type Engine interface {
	Index(ctx context.Context, in IndexInput) (*IndexResult, error)
	Search(ctx context.Context, in SearchInput) (*SearchResult, error)
	Impact(ctx context.Context, in ImpactInput) (*ImpactResult, error)
	Status(ctx context.Context, in StatusInput) (*StatusResult, error)
	Close() error
}

// ── Construction ────────────────────────────────────────────────────────────

// Config holds resolved construction options.
type Config struct {
	Tier         string // "local" | "hosted"
	StorageKind  string // "sqlite" | "postgres"
	SQLitePath   string
	PostgresDSN  string
	LexicalDir   string
	EnableVector bool
}

// Option mutates a Config during New().
type Option func(*Config)

func WithSQLite(path string) Option {
	return func(c *Config) { c.Tier, c.StorageKind, c.SQLitePath = "local", "sqlite", path }
}

func WithPostgres(dsn string) Option {
	return func(c *Config) { c.Tier, c.StorageKind, c.PostgresDSN = "hosted", "postgres", dsn }
}

func defaultConfig() Config {
	return Config{Tier: "local", StorageKind: "sqlite", SQLitePath: "./.atlas/atlas.db"}
}

// New builds the local engine: opens the StorageDriver (the one-line tier swap),
// migrates the schema, and opens the on-disk lexical index alongside the DB.
func New(ctx context.Context, opts ...Option) (Engine, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	drv, err := store.Open(ctx, store.Options{
		Kind:        cfg.StorageKind,
		SQLitePath:  cfg.SQLitePath,
		PostgresDSN: cfg.PostgresDSN,
	})
	if err != nil {
		return nil, err
	}
	if err := drv.Migrate(ctx); err != nil {
		_ = drv.Close()
		return nil, fmt.Errorf("engine: migrate: %w", err)
	}
	lexDir := cfg.LexicalDir
	if lexDir == "" {
		base := filepath.Dir(cfg.SQLitePath)
		if base == "" || base == "." {
			base = ".atlas"
		}
		lexDir = filepath.Join(base, "lexical")
	}
	lx, err := lexical.New(lexDir)
	if err != nil {
		_ = drv.Close()
		return nil, fmt.Errorf("engine: open lexical index: %w", err)
	}
	return &localEngine{cfg: cfg, store: drv, lexical: lx}, nil
}

// localEngine is the deterministic, single-DB code-intelligence engine.
type localEngine struct {
	cfg     Config
	store   store.StorageDriver
	lexical *lexical.Index
}

func (e *localEngine) Index(ctx context.Context, in IndexInput) (*IndexResult, error) {
	root := in.ProjectPath
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("engine: resolve path: %w", err)
	}
	fullName := in.Repo
	if fullName == "" {
		fullName = filepath.Base(abs)
	}
	// repoID left empty: the store resolves/mints the canonical id by full_name,
	// so re-indexing the same repo reuses its row.
	snap, stats, err := index.Run(ctx, e.store, e.lexical, "", fullName, abs, index.Options{Reindex: in.Reindex})
	if err != nil {
		return nil, err
	}
	return &IndexResult{
		RepoID:       snap.RepoID,
		RepoFullName: fullName,
		SnapshotID:   snap.ID,
		CommitSHA:    snap.CommitSHA,
		IndexedFiles: stats.Files,
		Symbols:      stats.Symbols,
		Edges:        stats.Edges,
		Routes:       stats.Routes,
		Languages:    stats.Languages,
		Mode:         stats.Mode,
		DurationMS:   stats.DurationMS,
	}, nil
}

func (e *localEngine) Search(ctx context.Context, in SearchInput) (*SearchResult, error) {
	if in.Mode == "semantic" || in.Mode == "hybrid" {
		if !e.cfg.EnableVector {
			return nil, errors.New("atlas: semantic mode requires vectors enabled (off by default)")
		}
	}
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	hits, err := e.lexical.Search(snap.ID, in.Query, limit*2) // over-fetch for post-filtering
	if err != nil {
		return nil, fmt.Errorf("engine: search: %w", err)
	}
	syms, err := e.store.ListSymbols(ctx, snap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: load symbols: %w", err)
	}
	byID := make(map[string]graph.CodeSymbol, len(syms))
	for _, s := range syms {
		byID[s.ID] = s
	}
	out := make([]SearchHit, 0, limit)
	for _, h := range hits {
		s, ok := byID[h.SymbolID]
		if !ok {
			continue
		}
		if in.Kind != "" && !strings.EqualFold(s.Kind, in.Kind) {
			continue
		}
		out = append(out, SearchHit{
			SymbolID:  s.ID,
			Name:      s.Name,
			Kind:      s.Kind,
			RepoID:    s.RepoID,
			Path:      s.Path,
			Line:      s.StartLine,
			Signature: s.Signature,
			Doc:       s.Doc,
			Score:     h.Score,
		})
		if len(out) >= limit {
			break
		}
	}
	return &SearchResult{Results: out, ModeUsed: "lexical", Total: len(out)}, nil
}

func (e *localEngine) Impact(ctx context.Context, in ImpactInput) (*ImpactResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	syms, err := e.store.ListSymbols(ctx, snap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: load symbols: %w", err)
	}
	edges, err := e.store.ListEdges(ctx, snap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: load edges: %w", err)
	}
	depth := in.MaxDepth
	if depth <= 0 {
		depth = 3
	}
	r := query.Impact(syms, edges, in.ChangedPaths, in.Symbols, depth)
	files := make([]FileImpact, 0, len(r.ImpactedFiles))
	for _, p := range r.ImpactedFiles {
		files = append(files, FileImpact{Path: p, Reason: "caller"})
	}
	return &ImpactResult{
		ImpactedSymbols: r.ImpactedSymbols,
		ImpactedFiles:   files,
		ImpactedTests:   nil, // deterministic coverage join lands with the coverage op
		DepthReached:    r.DepthReached,
	}, nil
}

func (e *localEngine) Status(ctx context.Context, in StatusInput) (*StatusResult, error) {
	repos, err := e.store.ListRepos(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("engine: list repos: %w", err)
	}
	out := make([]RepoStatus, 0, len(repos))
	for _, r := range repos {
		rs := RepoStatus{RepoID: r.ID, FullName: r.FullName, CommitSHA: r.LastCommit}
		if r.LastIndexedAt != nil {
			rs.IndexedAt = r.LastIndexedAt.Format("2006-01-02T15:04:05Z")
		}
		if snap, _ := e.store.LatestSnapshot(ctx, r.ID); snap != nil {
			rs.Snapshot = snap.ID
			rs.CommitSHA = snap.CommitSHA
			rs.Symbols = snap.SymbolCount
			rs.Edges = snap.EdgeCount
		}
		out = append(out, rs)
	}
	return &StatusResult{
		Tier:          e.cfg.Tier,
		StorageDriver: e.store.Dialect(),
		VectorBackend: "disabled",
		ReposIndexed:  len(repos),
		Repos:         out,
	}, nil
}

func (e *localEngine) Close() error {
	var first error
	if e.lexical != nil {
		if err := e.lexical.Close(); err != nil {
			first = err
		}
	}
	if e.store != nil {
		if err := e.store.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// resolveSnapshot picks the snapshot a read op should run against: the latest
// for the named repo, or — when no repo is named — the most recently indexed
// repo in the DB (the common single-repo local case).
func (e *localEngine) resolveSnapshot(ctx context.Context, repoID string) (*graph.Snapshot, error) {
	if repoID != "" {
		snap, err := e.store.LatestSnapshot(ctx, repoID)
		if err != nil {
			return nil, err
		}
		if snap == nil {
			return nil, ErrNoIndex
		}
		return snap, nil
	}
	repos, err := e.store.ListRepos(ctx, "")
	if err != nil {
		return nil, err
	}
	var best *graph.Snapshot
	for _, r := range repos {
		snap, err := e.store.LatestSnapshot(ctx, r.ID)
		if err != nil || snap == nil {
			continue
		}
		if best == nil || snap.CreatedAt.After(best.CreatedAt) {
			best = snap
		}
	}
	if best == nil {
		return nil, ErrNoIndex
	}
	return best, nil
}

// sortHitsByScore is a stable helper kept for callers that post-merge hit lists.
func sortHitsByScore(h []SearchHit) {
	sort.SliceStable(h, func(i, j int) bool { return h[i].Score > h[j].Score })
}
