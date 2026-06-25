// Package engine is the composition root. It defines the Engine interface — one
// method per canonical operation in the Atlas catalog — and a stub
// implementation that wires a StorageDriver in. Real logic (parser, lexical
// index, impact walk, cross-repo matcher) is filled in behind this seam.
package engine

import (
	"context"
	"errors"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

// ErrNotImplemented is the sentinel every scaffold stub returns.
var ErrNotImplemented = errors.New("atlas: not implemented")

// ErrTierUnsupported is returned when a hosted-only op runs on a local engine.
var ErrTierUnsupported = errors.New("atlas: operation requires hosted tier")

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
	RepoID       string `json:"repo_id"`
	SnapshotID   string `json:"snapshot_id"`
	CommitSHA    string `json:"commit_sha"`
	IndexedFiles int    `json:"indexed_files"`
	Symbols      int    `json:"symbols"`
	Edges        int    `json:"edges"`
	Routes       int    `json:"routes"`
	Mode         string `json:"mode"`
	DurationMS   int64  `json:"duration_ms"`
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
	Snippet   string  `json:"snippet,omitempty"`
}

type SearchResult struct {
	Results  []SearchHit `json:"results"`
	ModeUsed string      `json:"mode_used"`
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
	Path      string `json:"path"`
	Reason    string `json:"reason"`
	RiskLevel string `json:"risk_level"`
	Depth     int    `json:"depth"`
}

type ImpactResult struct {
	ImpactedSymbols  []string     `json:"impacted_symbols"`
	ImpactedFiles    []FileImpact `json:"impacted_files"`
	ImpactedTests    []string     `json:"impacted_tests"`
	DirectnessRanked []FileImpact `json:"directness_ranked"`
	DepthReached     int          `json:"depth_reached"`
}

type StatusInput struct {
	RepoID  string
	Verbose bool
}

type RepoStatus struct {
	RepoID       string `json:"repo_id"`
	LastSnapshot string `json:"last_snapshot"`
	CommitSHA    string `json:"commit_sha"`
	IndexedAt    string `json:"indexed_at"`
	Stale        bool   `json:"stale"`
}

type StatusResult struct {
	Tier          string       `json:"tier"`
	StorageDriver string       `json:"storage_driver"`
	VectorBackend string       `json:"vector_backend"`
	ReposIndexed  int          `json:"repos_indexed"`
	QueuePending  int          `json:"queue_pending"`
	QueueRunning  int          `json:"queue_running"`
	QueueFailed   int          `json:"queue_failed"`
	Repos         []RepoStatus `json:"repos"`
}

// ── The Engine interface ────────────────────────────────────────────────────

// Engine is the single contract all surfaces depend on. Only the ops exercised
// by the scaffold's surfaces are spelled out; the full catalog (callers, refs,
// neighbors, path, explain, graph_export, cross_repo_impact, consumers,
// route_contracts, history, snapshot_diff, tests_for_change, coverage, rca, fix,
// review, repos, link) extends this interface following the same pattern.
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

// New builds the stub Engine, selecting a StorageDriver per Config (the
// one-line tier swap keystone).
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
	return &stubEngine{cfg: cfg, store: drv}, nil
}

// stubEngine is a compiling placeholder: every op returns ErrNotImplemented,
// except Status which reports real driver/tier metadata so the surfaces have
// something honest to render.
type stubEngine struct {
	cfg   Config
	store store.StorageDriver
}

func (e *stubEngine) Index(ctx context.Context, in IndexInput) (*IndexResult, error) {
	return nil, ErrNotImplemented
}

func (e *stubEngine) Search(ctx context.Context, in SearchInput) (*SearchResult, error) {
	if in.Mode == "semantic" || in.Mode == "hybrid" {
		if !e.cfg.EnableVector {
			return nil, errors.New("atlas: semantic mode requires vectors enabled")
		}
	}
	return nil, ErrNotImplemented
}

func (e *stubEngine) Impact(ctx context.Context, in ImpactInput) (*ImpactResult, error) {
	return nil, ErrNotImplemented
}

func (e *stubEngine) Status(ctx context.Context, in StatusInput) (*StatusResult, error) {
	return &StatusResult{
		Tier:          e.cfg.Tier,
		StorageDriver: e.store.Dialect(),
		VectorBackend: "disabled",
		ReposIndexed:  0,
		Repos:         []RepoStatus{},
	}, nil
}

func (e *stubEngine) Close() error { return e.store.Close() }
