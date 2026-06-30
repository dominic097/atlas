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
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dominic097/atlas/internal/coverage"
	"github.com/dominic097/atlas/internal/crossrepo"
	"github.com/dominic097/atlas/internal/embed"
	"github.com/dominic097/atlas/internal/export"
	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/lexical"
	"github.com/dominic097/atlas/internal/query"
	"github.com/dominic097/atlas/internal/store"
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
	// RespectGitignore prunes git-ignored paths from the walk (the CLI sets this
	// true by default). Zero value preserves the legacy "index everything" behavior
	// for SDK callers.
	RespectGitignore bool
}

type IndexResult struct {
	RepoID       string           `json:"repo_id"`
	RepoFullName string           `json:"repo_full_name"`
	SnapshotID   string           `json:"snapshot_id"`
	CommitSHA    string           `json:"commit_sha"`
	IndexedFiles int              `json:"indexed_files"`
	Symbols      int              `json:"symbols"`
	Edges        int              `json:"edges"`
	EdgeKinds    map[string]int   `json:"edge_kinds,omitempty"`
	Routes       int              `json:"routes"`
	Languages    map[string]int   `json:"languages"`
	Mode         string           `json:"mode"`
	DurationMS   int64            `json:"duration_ms"`
	TimingsMS    map[string]int64 `json:"timings_ms,omitempty"`
	// Repos is the per-repo breakdown of a SEGMENTED run (Mode=="segmented"),
	// produced when `index` is pointed at a workspace containing many repos. Empty
	// for a normal single-repo index. The top-level Symbols/Edges/IndexedFiles/
	// Routes fields hold the aggregate across all repos.
	Repos []RepoIndexSummary `json:"repos,omitempty"`
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

type ContextInput struct {
	RepoID   string
	Paths    []string
	Query    string
	Limit    int
	MaxFiles int
	MaxEdges int
	MaxDepth int
}

type ContextFile struct {
	Path      string   `json:"path"`
	Language  string   `json:"language"`
	SizeBytes int64    `json:"size_bytes"`
	Hash      string   `json:"hash,omitempty"`
	Imports   []string `json:"imports,omitempty"`
}

type ContextSymbol struct {
	SymbolID  string         `json:"symbol_id"`
	Name      string         `json:"symbol"`
	Kind      string         `json:"kind"`
	RepoID    string         `json:"repo_id"`
	Path      string         `json:"path"`
	Line      int            `json:"line"`
	EndLine   int            `json:"end_line"`
	Signature string         `json:"signature,omitempty"`
	Doc       string         `json:"doc,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ContextEdge struct {
	FromFile   string         `json:"from_file"`
	FromSymbol string         `json:"from_symbol,omitempty"`
	ToRef      string         `json:"to_ref"`
	Kind       string         `json:"kind"`
	Language   string         `json:"language,omitempty"`
	Line       int            `json:"line,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ContextResult struct {
	RepoID        string          `json:"repo_id"`
	SnapshotID    string          `json:"snapshot_id"`
	CommitSHA     string          `json:"commit_sha"`
	Files         []ContextFile   `json:"files"`
	Symbols       []ContextSymbol `json:"symbols"`
	Edges         []ContextEdge   `json:"edges"`
	SearchHits    []SearchHit     `json:"search_hits,omitempty"`
	ImpactedFiles []FileImpact    `json:"impacted_files,omitempty"`
	Mode          string          `json:"mode"`
}

// SemanticSearchInput drives the OPTIONAL, gated semantic_search op. When vectors
// are disabled or the snapshot has no embeddings, the op degrades to lexical and
// reports it (Degraded:true) rather than erroring.
type SemanticSearchInput struct {
	Query    string
	RepoID   string
	Limit    int
	MinScore float64
}

// SemanticSearchResult carries the hits plus an honest mode report: ModeUsed is
// "semantic" when vector nearest-neighbor ran, "lexical" when it degraded;
// Degraded mirrors that for a quick boolean check.
type SemanticSearchResult struct {
	Results  []SearchHit `json:"results"`
	Degraded bool        `json:"degraded"`
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

// LinkInput registers a repo into the graph WITHOUT indexing it. Repo may be a
// filesystem path, a git remote URL (git@host:Org/Repo.git or
// https://host/Org/Repo(.git)), or a bare org/name. Branch defaults to "main".
type LinkInput struct {
	Repo   string
	Branch string
}

// LinkResult reports the registered (or already-present) repo. Created is true
// when this call inserted a new repo row; Indexed is true when the repo already
// has at least one snapshot. Link never indexes — Indexed reflects prior state.
type LinkResult struct {
	RepoID        string `json:"repo_id"`
	FullName      string `json:"repo_full_name"`
	Root          string `json:"root"`
	DefaultBranch string `json:"default_branch"`
	Scope         string `json:"scope,omitempty"`
	Created       bool   `json:"created"`
	Indexed       bool   `json:"indexed"`
}

// SymbolRef is a lightweight pointer to a symbol used in callers/callees lists.
type SymbolRef struct {
	SymbolID  string `json:"symbol_id"`
	Name      string `json:"symbol"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Signature string `json:"signature,omitempty"`
}

type CallersInput struct {
	Name   string
	RepoID string
	Limit  int
}

type CallersResult struct {
	Symbol  string      `json:"symbol"`
	Callers []SymbolRef `json:"callers"`
	Total   int         `json:"total"`
}

type SymbolInput struct {
	Name   string
	RepoID string
}

type SymbolDef struct {
	SymbolID  string      `json:"symbol_id"`
	Name      string      `json:"symbol"`
	Kind      string      `json:"kind"`
	RepoID    string      `json:"repo_id"`
	Path      string      `json:"path"`
	Line      int         `json:"line"`
	EndLine   int         `json:"end_line"`
	Signature string      `json:"signature,omitempty"`
	Doc       string      `json:"doc,omitempty"`
	Callers   []SymbolRef `json:"callers"`
	Callees   []SymbolRef `json:"callees"`
}

type SymbolResult struct {
	Query   string      `json:"query"`
	Matches []SymbolDef `json:"matches"`
}

type GraphExportInput struct {
	RepoID   string
	Symbol   string // export the neighborhood around this symbol
	Depth    int    // hops of callers+callees (default 2)
	MaxNodes int    // subgraph node cap (default 200)
	Format   string // json | mermaid | dot
	All      bool   // export the whole snapshot instead of a subgraph
}

type GraphExportResult struct {
	Format  string `json:"format"`
	Nodes   int    `json:"nodes"`
	Edges   int    `json:"edges"`
	Content string `json:"content"`
}

// ── Temporal (the moat graphify lacks) ──────────────────────────────────────

type HistoryInput struct {
	RepoID string
	Limit  int
}

type SnapshotInfo struct {
	SnapshotID string `json:"snapshot_id"`
	CommitSHA  string `json:"commit_sha"`
	Branch     string `json:"branch,omitempty"`
	Files      int    `json:"files"`
	Symbols    int    `json:"symbols"`
	Edges      int    `json:"edges"`
	CreatedAt  string `json:"created_at"`
}

type HistoryResult struct {
	RepoID    string         `json:"repo_id"`
	FullName  string         `json:"repo_full_name"`
	Snapshots []SnapshotInfo `json:"snapshots"`
}

type SnapshotDiffInput struct {
	RepoID string
	From   string // commit sha (prefix) or snapshot id; default = the snapshot before To
	To     string // commit sha (prefix) or snapshot id; default = latest snapshot
}

type SnapshotDiffResult struct {
	FromCommit    string               `json:"from_commit"`
	FromSnapshot  string               `json:"from_snapshot"`
	ToCommit      string               `json:"to_commit"`
	ToSnapshot    string               `json:"to_snapshot"`
	AddedCount    int                  `json:"added_count"`
	RemovedCount  int                  `json:"removed_count"`
	ModifiedCount int                  `json:"modified_count"`
	Added         []query.SymbolChange `json:"added_symbols"`
	Removed       []query.SymbolChange `json:"removed_symbols"`
	Modified      []query.SymbolChange `json:"modified_symbols"`
	ChangedFiles  []string             `json:"changed_files"`
	AddedEdges    []query.EdgeChange   `json:"added_edges"`
	RemovedEdges  []query.EdgeChange   `json:"removed_edges"`
}

// ── Cross-repo (the USP) ─────────────────────────────────────────────────────

type CrossRepoImpactInput struct {
	Repo         string   // producer repo full_name; empty = single/most-recent repo
	ChangedPaths []string // changed handler files; empty = the whole repo's contract
}

// ConsumerHit is one calling site in another repo impacted by the change.
type ConsumerHit struct {
	Repo          string `json:"repo"`
	CallingFile   string `json:"calling_file"`
	CallingSymbol string `json:"calling_symbol,omitempty"`
	MatchedRoute  string `json:"matched_route"`
	Endpoint      string `json:"endpoint"`
}

// RouteContract is a producer route a repo serves (JSON-friendly projection of graph.Route).
type RouteContract struct {
	Method        string `json:"method"`
	PathPattern   string `json:"path_pattern"`
	HandlerFile   string `json:"handler_file,omitempty"`
	HandlerSymbol string `json:"handler_symbol,omitempty"`
	Source        string `json:"source,omitempty"`
	Confidence    string `json:"confidence,omitempty"`
}

type CrossRepoImpactResult struct {
	Repo          string          `json:"repo"`
	ServedRoutes  []RouteContract `json:"served_routes"`
	Impacted      []ConsumerHit   `json:"impacted"`
	ConsumerRepos []string        `json:"consumer_repos"`
}

type ConsumersInput struct {
	Repo string // producer repo full_name; empty = single/most-recent repo
}

type ConsumersResult struct {
	Repo          string        `json:"repo"`
	Impacted      []ConsumerHit `json:"impacted"`
	ConsumerRepos []string      `json:"consumer_repos"`
}

type RouteContractsInput struct {
	Repo string // repo full_name; empty = single/most-recent repo
}

type RouteContractsResult struct {
	Repo   string          `json:"repo"`
	Routes []RouteContract `json:"routes"`
	Total  int             `json:"total"`
}

// ── Local navigation ops (deterministic, single-repo) ───────────────────────

type NeighborsInput struct {
	Name   string
	RepoID string
}

// NeighborsResult is the depth-1 call neighborhood of a symbol.
type NeighborsResult struct {
	Symbol  string      `json:"symbol"`
	Callers []SymbolRef `json:"callers"`
	Callees []SymbolRef `json:"callees"`
}

type PathInput struct {
	From     string
	To       string
	RepoID   string
	MaxDepth int
}

// PathResult is the shortest forward call path from From to To.
type PathResult struct {
	From   string      `json:"from"`
	To     string      `json:"to"`
	Found  bool        `json:"found"`
	Length int         `json:"length"`
	Steps  []SymbolRef `json:"steps"`
}

type RefsInput struct {
	Name   string
	RepoID string
}

// RefsResult is the call-site AND type-use references to a symbol: the union of
// resolved callers (over call edges) and the symbols that name it as a type (over
// the go/types reference edges), deduped by identity.
type RefsResult struct {
	Symbol     string      `json:"symbol"`
	References []SymbolRef `json:"references"`
	Total      int         `json:"total"`
}

type ExplainInput struct {
	Name       string
	RepoID     string
	CountsOnly bool `json:"-"`
}

// ExplainDef is one definition of the explained symbol with its location/doc.
type ExplainDef struct {
	SymbolID  string `json:"symbol_id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	EndLine   int    `json:"end_line"`
	Signature string `json:"signature,omitempty"`
	Doc       string `json:"doc,omitempty"`
}

// ExplainRoute is a producer route served by the explained symbol.
type ExplainRoute struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	HandlerFile string `json:"handler_file,omitempty"`
}

// ExplainResult is a DETERMINISTIC context bundle for a symbol (no LLM narrative):
// definitions, caller/callee names, the defining files' imports, any producer
// routes it serves, and — when it serves routes — the cross-repo consumers of
// those routes.
type ExplainResult struct {
	Symbol             string         `json:"symbol"`
	Definitions        []ExplainDef   `json:"definitions"`
	Callers            []string       `json:"callers"`
	Callees            []string       `json:"callees"`
	CallerCount        int            `json:"caller_count,omitempty"`
	CalleeCount        int            `json:"callee_count,omitempty"`
	Imports            []string       `json:"imports,omitempty"`
	ServedRoutes       []ExplainRoute `json:"served_routes,omitempty"`
	CrossRepoConsumers []string       `json:"cross_repo_consumers,omitempty"`
}

type CoverageInput struct {
	Target    string
	RepoID    string
	Direction string // tests_for_symbol | symbols_for_test | "" (auto)
	MaxDepth  int
}

// CoverageResult reports coverage for a target. When runtime coverage has been
// imported for the target (mode="runtime") it reports the real covered/total line
// ratio from the ingested profile. Otherwise it falls back to STATIC call-graph
// reachability (mode="static"): the transitive test callers of a symbol, or the
// non-test symbols a test transitively exercises.
type CoverageResult struct {
	Target    string      `json:"target"`
	Mode      string      `json:"mode"` // runtime | static
	Direction string      `json:"direction,omitempty"`
	Covered   bool        `json:"covered"`
	Strength  string      `json:"strength,omitempty"` // runtime mode: "coveredLines/totalLines"
	Tests     []SymbolRef `json:"tests,omitempty"`
	Symbols   []SymbolRef `json:"symbols,omitempty"`
}

// CoverageImportInput points at a coverage profile to ingest (a Go coverprofile
// or an LCOV tracefile) and the repo it belongs to (empty = single/most-recent).
type CoverageImportInput struct {
	Path   string
	RepoID string
}

// CoverageImportResult summarizes a coverage import: the resolved snapshot, the
// auto-detected profile format, how many profile files were parsed, and how many
// indexed symbols received a runtime coverage fact.
type CoverageImportResult struct {
	SnapshotID     string `json:"snapshot_id"`
	RepoFullName   string `json:"repo_full_name"`
	Format         string `json:"format"`
	ProfileFiles   int    `json:"profile_files"`
	SymbolsTotal   int    `json:"symbols_total"`
	SymbolsCovered int    `json:"symbols_covered"`
	FactsWritten   int    `json:"facts_written"`
}

// ── The Engine interface ────────────────────────────────────────────────────

// Engine is the single contract all surfaces depend on. The full catalog
// (callers, refs, neighbors, path, explain, graph_export, cross_repo_impact,
// consumers, route_contracts, history, snapshot_diff, coverage, repos, link)
// extends this interface following the same pattern.
type Engine interface {
	Index(ctx context.Context, in IndexInput) (*IndexResult, error)
	Search(ctx context.Context, in SearchInput) (*SearchResult, error)
	Context(ctx context.Context, in ContextInput) (*ContextResult, error)
	SemanticSearch(ctx context.Context, in SemanticSearchInput) (*SemanticSearchResult, error)
	Impact(ctx context.Context, in ImpactInput) (*ImpactResult, error)
	Callers(ctx context.Context, in CallersInput) (*CallersResult, error)
	Symbol(ctx context.Context, in SymbolInput) (*SymbolResult, error)
	Neighbors(ctx context.Context, in NeighborsInput) (*NeighborsResult, error)
	Path(ctx context.Context, in PathInput) (*PathResult, error)
	Refs(ctx context.Context, in RefsInput) (*RefsResult, error)
	Explain(ctx context.Context, in ExplainInput) (*ExplainResult, error)
	Coverage(ctx context.Context, in CoverageInput) (*CoverageResult, error)
	CoverageImport(ctx context.Context, in CoverageImportInput) (*CoverageImportResult, error)
	GraphExport(ctx context.Context, in GraphExportInput) (*GraphExportResult, error)
	History(ctx context.Context, in HistoryInput) (*HistoryResult, error)
	SnapshotDiff(ctx context.Context, in SnapshotDiffInput) (*SnapshotDiffResult, error)
	CrossRepoImpact(ctx context.Context, in CrossRepoImpactInput) (*CrossRepoImpactResult, error)
	Consumers(ctx context.Context, in ConsumersInput) (*ConsumersResult, error)
	RouteContracts(ctx context.Context, in RouteContractsInput) (*RouteContractsResult, error)
	Communities(ctx context.Context, in CommunitiesInput) (*CommunitiesResult, error)
	Hubs(ctx context.Context, in HubsInput) (*HubsResult, error)
	Report(ctx context.Context, in ReportInput) (*ReportResult, error)
	Stats(ctx context.Context, in StatsInput) (*StatsResult, error)
	Status(ctx context.Context, in StatusInput) (*StatusResult, error)
	Link(ctx context.Context, in LinkInput) (*LinkResult, error)
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
	// Scope is the tenant/org id every repo read/write is scoped to. Empty means
	// single-tenant / all repos — the local default, so existing behaviour and the
	// SQLite flow are unchanged.
	Scope string
	// ContextBudget holds the configurable default budgets for the `context` op.
	ContextBudget ContextBudget
}

// ContextBudget bounds the `context` op's output so a caller can cap the token
// cost. Zero fields fall back to the engine defaults (Limit 80, MaxFiles 60,
// MaxEdges 500, MaxDepth 3). Configure the defaults with WithContextBudget or the
// ATLAS_CONTEXT_{LIMIT,MAX_FILES,MAX_EDGES,MAX_DEPTH} env vars; a per-request
// ContextInput value still overrides whatever is configured here.
type ContextBudget struct {
	Limit    int // max symbols packed
	MaxFiles int // max files referenced
	MaxEdges int // max scoped edges
	MaxDepth int // impact traversal depth
}

// Option mutates a Config during New().
type Option func(*Config)

func WithSQLite(path string) Option {
	return func(c *Config) { c.Tier, c.StorageKind, c.SQLitePath = "local", "sqlite", path }
}

func WithPostgres(dsn string) Option {
	return func(c *Config) { c.Tier, c.StorageKind, c.PostgresDSN = "hosted", "postgres", dsn }
}

// WithScope sets the tenant/org scope every repo read/write is isolated to. It
// does NOT change the tier: a scoped local SQLite engine and a scoped hosted
// Postgres engine are both valid. Empty scope ("") keeps single-tenant / all-repo
// behaviour.
func WithScope(scope string) Option {
	return func(c *Config) { c.Scope = scope }
}

// WithLexicalDir sets the on-disk BM25 index directory. Hosted callers that
// create multiple scoped engines in one process should provide a unique
// directory per scope so Bleve/bbolt locks and documents are not shared.
func WithLexicalDir(dir string) Option {
	return func(c *Config) { c.LexicalDir = strings.TrimSpace(dir) }
}

// WithVectors enables the OPTIONAL semantic layer for this engine: the index pass
// builds per-symbol embeddings (when --enable-vectors / IndexInput.EnableVectors
// also asks for them) and query-time semantic_search runs vector nearest-neighbor
// instead of degrading to lexical. Off by default — the deterministic core is
// unchanged. ATLAS_ENABLE_VECTORS=1 sets the same flag from the environment.
func WithVectors(enabled bool) Option {
	return func(c *Config) { c.EnableVector = enabled }
}

// WithContextBudget overrides the default `context` op budgets. Only non-zero
// fields are applied, so you can raise just one (e.g. MaxEdges) and leave the
// rest at their defaults. The ATLAS_CONTEXT_* env vars override this again, and a
// per-request ContextInput field still wins over everything.
func WithContextBudget(b ContextBudget) Option {
	return func(c *Config) {
		if b.Limit > 0 {
			c.ContextBudget.Limit = b.Limit
		}
		if b.MaxFiles > 0 {
			c.ContextBudget.MaxFiles = b.MaxFiles
		}
		if b.MaxEdges > 0 {
			c.ContextBudget.MaxEdges = b.MaxEdges
		}
		if b.MaxDepth > 0 {
			c.ContextBudget.MaxDepth = b.MaxDepth
		}
	}
}

func defaultConfig() Config {
	return Config{
		Tier: "local", StorageKind: "sqlite", SQLitePath: "./.atlas/atlas.db",
		ContextBudget: ContextBudget{Limit: 80, MaxFiles: 60, MaxEdges: 500, MaxDepth: 3},
	}
}

// envTrue reports whether an env value is a truthy flag ("1"/"true"/"yes"/"on",
// case-insensitive). Used to read ATLAS_ENABLE_VECTORS.
func envTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// envInt returns a positive integer parsed from an env value, or fallback when
// the value is unset, non-numeric, or non-positive. Used for the ATLAS_CONTEXT_*
// budget overrides.
func envInt(v string, fallback int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		return n
	}
	return fallback
}

// New builds the local engine: opens the StorageDriver (the one-line tier swap),
// migrates the schema, and opens the on-disk lexical index alongside the DB.
func New(ctx context.Context, opts ...Option) (Engine, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	// ATLAS_ENABLE_VECTORS=1 enables the optional semantic layer at query time even
	// when no explicit WithVectors option was passed (e.g. from the CLI/SDK env). An
	// explicit option still wins — this only flips the default on, never off.
	if envTrue(os.Getenv("ATLAS_ENABLE_VECTORS")) {
		cfg.EnableVector = true
	}
	// ATLAS_CONTEXT_* override the `context` op budget defaults from the
	// environment (deployment config wins over a WithContextBudget option); a
	// per-request ContextInput value still overrides these at call time.
	cfg.ContextBudget.Limit = envInt(os.Getenv("ATLAS_CONTEXT_LIMIT"), cfg.ContextBudget.Limit)
	cfg.ContextBudget.MaxFiles = envInt(os.Getenv("ATLAS_CONTEXT_MAX_FILES"), cfg.ContextBudget.MaxFiles)
	cfg.ContextBudget.MaxEdges = envInt(os.Getenv("ATLAS_CONTEXT_MAX_EDGES"), cfg.ContextBudget.MaxEdges)
	cfg.ContextBudget.MaxDepth = envInt(os.Getenv("ATLAS_CONTEXT_MAX_DEPTH"), cfg.ContextBudget.MaxDepth)
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
	return &localEngine{cfg: cfg, store: drv}, nil
}

// localEngine is the deterministic, single-DB code-intelligence engine.
type localEngine struct {
	cfg       Config
	store     store.StorageDriver
	lexical   *lexical.Index
	lexicalMu sync.Mutex

	// ctxCache memoizes Context() bundles keyed by (snapshotID, sorted changed
	// paths, query, budget). A new snapshot produces a new key, so it invalidates
	// naturally; the watch/serve warm process — which re-asks for context against
	// the same snapshot — benefits most. nil until the first Context call.
	ctxCache *contextCache
}

func (e *localEngine) ensureLexical() (*lexical.Index, error) {
	if e.lexical != nil {
		return e.lexical, nil
	}
	e.lexicalMu.Lock()
	defer e.lexicalMu.Unlock()
	if e.lexical != nil {
		return e.lexical, nil
	}
	lexDir := e.cfg.LexicalDir
	if lexDir == "" {
		base := filepath.Dir(e.cfg.SQLitePath)
		if base == "" || base == "." {
			base = ".atlas"
		}
		lexDir = filepath.Join(base, "lexical")
	}
	lx, err := lexical.New(lexDir)
	if err != nil {
		return nil, fmt.Errorf("engine: open lexical index: %w", err)
	}
	e.lexical = lx
	return lx, nil
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
	// Auto-segment a workspace into its constituent repos. When the caller did not
	// name a single repo, look for git repos nested under the path and, if it is a
	// workspace (a parent of repos), index each nested repo on its own. This bounds
	// memory — each repo's parse + go/types pass runs and flushes before the next,
	// so a 200k-file many-repo tree no longer accumulates one giant in-memory graph
	// and OOMs — and preserves cross-repo links, which form at query time from the
	// shared store.
	//
	// A workspace is either (a) a plain parent dir holding repos, or (b) a dir that
	// is ITSELF under git yet still contains several independent repos (e.g. a dev
	// workspace kept under one outer git, like ~/workspace with many cloned repos).
	// A normal repo with one vendored submodule (root is a repo, ≤1 nested) is NOT a
	// workspace and is indexed as a single repo.
	if strings.TrimSpace(in.Repo) == "" {
		nested := discoverRepos(abs)
		rootIsRepo := isGitRepo(abs)
		if (len(nested) >= 1 && !rootIsRepo) || (len(nested) >= 2 && rootIsRepo) {
			return e.indexSegmented(ctx, in, abs, nested, rootIsRepo)
		}
	}
	// repoID left empty: the store resolves/mints the canonical id by full_name,
	// so re-indexing the same repo reuses its row.
	return e.indexOne(ctx, in, abs, "", nil)
}

// Search is the catalog entry point. The default is deterministic lexical (BM25)
// search. "semantic" delegates to SemanticSearch (degrading to lexical when
// vectors are off or the snapshot has no embeddings). "hybrid" blends lexical and
// semantic when vectors are on, else runs lexical and notes the degrade in
// ModeUsed. Vectors are never required for Search to succeed.
func (e *localEngine) Search(ctx context.Context, in SearchInput) (*SearchResult, error) {
	switch in.Mode {
	case "semantic":
		sr, err := e.SemanticSearch(ctx, SemanticSearchInput{Query: in.Query, RepoID: in.RepoID, Limit: in.Limit})
		if err != nil {
			return nil, err
		}
		hits := filterByKind(sr.Results, in.Kind)
		return &SearchResult{Results: hits, ModeUsed: sr.ModeUsed, Total: len(hits)}, nil
	case "hybrid":
		return e.hybridSearch(ctx, in)
	}
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	limit := searchLimit(in.Limit)
	out, err := e.lexicalSearch(ctx, snap.ID, in.Query, in.Kind, limit)
	if err != nil {
		return nil, err
	}
	return &SearchResult{Results: out, ModeUsed: "lexical", Total: len(out)}, nil
}

// Context returns a bounded, deterministic fact bundle for code-review/RCA
// consumers. It starts from changed paths, then adds lexical retrieval hits and
// reverse-impact files without packing whole files into the caller's prompt.
func (e *localEngine) Context(ctx context.Context, in ContextInput) (*ContextResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	// Budget precedence: a per-request value wins over the configured default
	// (WithContextBudget / ATLAS_CONTEXT_*), which wins over the built-in floor.
	limit := firstPositive(in.Limit, e.cfg.ContextBudget.Limit, 80)
	maxFiles := firstPositive(in.MaxFiles, e.cfg.ContextBudget.MaxFiles, 60)
	maxEdges := firstPositive(in.MaxEdges, e.cfg.ContextBudget.MaxEdges, 500)
	depth := firstPositive(in.MaxDepth, e.cfg.ContextBudget.MaxDepth, 3)

	seedPaths := normalizeContextPaths(in.Paths)

	// Context-pack cache: a hit returns a fresh deep copy of the previously
	// assembled bundle without re-querying the store. The key folds the snapshot
	// id (so a new snapshot is a new key — natural invalidation), the sorted seed
	// paths, the query, and the resolved budget.
	cacheKey := contextCacheKey(snap.ID, seedPaths, in.Query, limit, maxFiles, maxEdges, depth)
	if cached, ok := e.contextCacheGet(cacheKey); ok {
		return cached, nil
	}

	selectedPaths := make([]string, 0, maxFiles)
	pathSet := map[string]bool{}
	addPath := func(path string) {
		path = strings.TrimSpace(filepath.ToSlash(path))
		if path == "" || pathSet[path] || len(selectedPaths) >= maxFiles {
			return
		}
		pathSet[path] = true
		selectedPaths = append(selectedPaths, path)
	}
	for _, path := range seedPaths {
		addPath(path)
	}

	selectedSymbols := make([]graph.CodeSymbol, 0, limit)
	symbolSet := map[string]bool{}
	addSymbol := func(sym graph.CodeSymbol) {
		if len(selectedSymbols) >= limit {
			return
		}
		key := symbolIdentity(sym)
		if key == "" || symbolSet[key] {
			return
		}
		symbolSet[key] = true
		selectedSymbols = append(selectedSymbols, sym)
		addPath(sym.Path)
	}
	for _, path := range seedPaths {
		syms, err := e.store.SymbolsByPath(ctx, snap.ID, path)
		if err != nil {
			return nil, fmt.Errorf("engine: context symbols by path: %w", err)
		}
		for _, sym := range syms {
			addSymbol(sym)
		}
	}

	queryText := strings.TrimSpace(in.Query)
	if queryText == "" {
		queryText = contextQueryFor(seedPaths, selectedSymbols)
	}

	searchHits := []SearchHit{}
	searchDegraded := false
	if queryText != "" {
		search, err := e.Search(ctx, SearchInput{RepoID: in.RepoID, Query: queryText, Limit: limit, Mode: "hybrid"})
		if err != nil {
			searchDegraded = true
		} else {
			searchHits = search.Results
			// Resolve only the hit symbols (a bounded set), not the whole snapshot:
			// fetch them by id via the PK-indexed SymbolsByIDs instead of loading
			// every symbol with ListSymbols.
			hitIDs := make([]string, 0, len(searchHits))
			for _, hit := range searchHits {
				if hit.SymbolID != "" {
					hitIDs = append(hitIDs, hit.SymbolID)
				}
			}
			hitByID, err := e.symbolsByID(ctx, snap.ID, hitIDs)
			if err != nil {
				return nil, err
			}
			for _, hit := range searchHits {
				if sym, ok := hitByID[hit.SymbolID]; ok {
					addSymbol(sym)
				} else {
					addPath(hit.Path)
				}
			}
		}
	}

	impact, err := query.ImpactGraph(ctx, e.store, snap.ID, seedPaths, namesOf(selectedSymbols, limit), depth)
	if err != nil {
		return nil, fmt.Errorf("engine: context impact: %w", err)
	}
	impactedFiles := make([]FileImpact, 0, len(impact.ImpactedFiles))
	for _, path := range impact.ImpactedFiles {
		addPath(path)
		impactedFiles = append(impactedFiles, FileImpact{Path: path, Reason: "caller"})
	}
	if len(impact.ImpactedSymbols) > 0 {
		impactSymbols, err := e.store.SymbolsByNames(ctx, snap.ID, impact.ImpactedSymbols)
		if err != nil {
			return nil, fmt.Errorf("engine: context impact symbols: %w", err)
		}
		for _, sym := range impactSymbols {
			addSymbol(sym)
		}
	}

	files, err := e.contextFiles(ctx, snap.ID, pathSet)
	if err != nil {
		return nil, err
	}
	edges, err := e.contextEdges(ctx, snap.ID, pathSet, maxEdges)
	if err != nil {
		return nil, err
	}
	mode := "symbol_context"
	if searchDegraded {
		mode = "symbol_context(search_degraded)"
	}
	result := &ContextResult{
		RepoID:        snap.RepoID,
		SnapshotID:    snap.ID,
		CommitSHA:     snap.CommitSHA,
		Files:         files,
		Symbols:       contextSymbols(selectedSymbols),
		Edges:         edges,
		SearchHits:    searchHits,
		ImpactedFiles: impactedFiles,
		Mode:          mode,
	}
	e.contextCachePut(cacheKey, result)
	return result, nil
}

func normalizeContextPaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(filepath.ToSlash(path))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

// firstPositive returns the first argument greater than zero, or 0 if none are.
// It encodes the context-budget precedence in one place: per-request value, then
// the configured default, then the built-in floor.
func firstPositive(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

func contextQueryFor(paths []string, symbols []graph.CodeSymbol) string {
	const maxTokens = 48
	seen := map[string]bool{}
	tokens := make([]string, 0, maxTokens)
	add := func(text string) bool {
		for _, token := range strings.FieldsFunc(text, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_')
		}) {
			if len(token) < 3 {
				continue
			}
			key := strings.ToLower(token)
			if seen[key] {
				continue
			}
			seen[key] = true
			tokens = append(tokens, token)
			if len(tokens) >= maxTokens {
				return false
			}
		}
		return true
	}
	for _, sym := range symbols {
		if !add(sym.Name) || !add(sym.Signature) || !add(sym.Doc) {
			break
		}
	}
	for _, path := range paths {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		dir := filepath.Base(filepath.Dir(path))
		if !add(base) || !add(strings.ReplaceAll(base, "-", " ")) || !add(strings.ReplaceAll(base, "_", " ")) || !add(dir) {
			break
		}
	}
	return strings.Join(tokens, " ")
}

// symbolsByID resolves a known set of symbol ids into an id-keyed map, fetching
// only those rows via the PK-indexed SymbolsByIDs rather than scanning the whole
// snapshot. Empty ids yields an empty map without touching the store.
func (e *localEngine) symbolsByID(ctx context.Context, snapshotID string, ids []string) (map[string]graph.CodeSymbol, error) {
	if len(ids) == 0 {
		return map[string]graph.CodeSymbol{}, nil
	}
	syms, err := e.store.SymbolsByIDs(ctx, snapshotID, ids)
	if err != nil {
		return nil, fmt.Errorf("engine: context load symbols: %w", err)
	}
	out := make(map[string]graph.CodeSymbol, len(syms))
	for _, sym := range syms {
		out[sym.ID] = sym
	}
	return out, nil
}

func (e *localEngine) contextFiles(ctx context.Context, snapshotID string, paths map[string]bool) ([]ContextFile, error) {
	files, err := e.store.FilesByPaths(ctx, snapshotID, trueKeys(paths))
	if err != nil {
		return nil, fmt.Errorf("engine: context files: %w", err)
	}
	out := make([]ContextFile, 0, len(paths))
	for _, file := range files {
		out = append(out, ContextFile{
			Path:      file.Path,
			Language:  file.Language,
			SizeBytes: file.SizeBytes,
			Hash:      file.Hash,
			Imports:   capStrings(file.Imports, 25),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (e *localEngine) contextEdges(ctx context.Context, snapshotID string, paths map[string]bool, maxEdges int) ([]ContextEdge, error) {
	// Fetch ONLY the edges leaving the selected paths (index-backed), not the
	// whole snapshot's edge set. EdgesByFromFiles returns them in the same
	// (from_file, to_ref, kind) order ListEdges did, so the slice + cap below
	// produces byte-identical output to the prior load-all-then-filter form.
	edges, err := e.store.EdgesByFromFiles(ctx, snapshotID, trueKeys(paths))
	if err != nil {
		return nil, fmt.Errorf("engine: context edges: %w", err)
	}
	out := make([]ContextEdge, 0, minInt(maxEdges, len(edges)))
	for _, edge := range edges {
		if !paths[edge.FromFile] {
			continue
		}
		out = append(out, ContextEdge{
			FromFile:   edge.FromFile,
			FromSymbol: edge.FromSymbol,
			ToRef:      edge.ToRef,
			Kind:       string(edge.Kind),
			Language:   edge.Language,
			Line:       edge.Line,
			Metadata:   copyAnyMap(edge.Metadata),
		})
		if len(out) >= maxEdges {
			break
		}
	}
	return out, nil
}

func contextSymbols(symbols []graph.CodeSymbol) []ContextSymbol {
	out := make([]ContextSymbol, 0, len(symbols))
	for _, sym := range symbols {
		out = append(out, ContextSymbol{
			SymbolID:  sym.ID,
			Name:      sym.Name,
			Kind:      sym.Kind,
			RepoID:    sym.RepoID,
			Path:      sym.Path,
			Line:      sym.StartLine,
			EndLine:   sym.EndLine,
			Signature: sym.Signature,
			Doc:       sym.Doc,
			Metadata:  copyAnyMap(sym.Metadata),
		})
	}
	return out
}

func copyAnyMap(in graph.JSONBMap) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SemanticSearch is the OPTIONAL, gated semantic op. When vectors are disabled OR
// the snapshot has no embeddings, it runs the lexical path and returns
// Degraded:true, ModeUsed:"lexical" — an honest fallback, never an error. With
// vectors on and embeddings present, it embeds the query, finds the nearest
// symbols by cosine, loads those symbols, and returns Degraded:false,
// ModeUsed:"semantic".
func (e *localEngine) SemanticSearch(ctx context.Context, in SemanticSearchInput) (*SemanticSearchResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	limit := searchLimit(in.Limit)

	// Degrade to lexical when this snapshot has no embeddings — which is the case
	// unless it was indexed with --enable-vectors. Presence of embeddings is the
	// gate (not a global flag), so a user who runs `index --enable-vectors` then
	// `semantic-search` gets real ranking without any extra env/flag. The
	// nearest-neighbor probe with limit=1 is the cheap "are there embeddings?" check.
	degrade := false
	probe, perr := e.store.NearestSymbols(ctx, snap.ID, nil, 1, -1)
	if perr != nil || len(probe) == 0 {
		degrade = true
	}
	if degrade {
		out, lerr := e.lexicalSearch(ctx, snap.ID, in.Query, "", limit)
		if lerr != nil {
			return nil, lerr
		}
		return &SemanticSearchResult{Results: out, Degraded: true, ModeUsed: "lexical"}, nil
	}

	qvecs, err := embed.NewProvider().Embed(ctx, []string{in.Query})
	if err != nil || len(qvecs) == 0 {
		// Embedding the query failed (e.g. HTTP provider down): degrade rather than
		// fail — lexical still answers.
		out, lerr := e.lexicalSearch(ctx, snap.ID, in.Query, "", limit)
		if lerr != nil {
			return nil, lerr
		}
		return &SemanticSearchResult{Results: out, Degraded: true, ModeUsed: "lexical"}, nil
	}

	// Default (MinScore<=0): exclude orthogonal, zero-similarity symbols so they
	// don't pad the results out to --limit; any positive cosine is still kept.
	minScore := in.MinScore
	if minScore <= 0 {
		minScore = 1e-9
	}
	scored, err := e.store.NearestSymbols(ctx, snap.ID, qvecs[0], limit, minScore)
	if err != nil {
		return nil, fmt.Errorf("engine: semantic search: %w", err)
	}
	// Fetch ONLY the scored symbols (targeted, index-backed) instead of loading
	// the whole snapshot's symbols just to resolve a handful of nearest-neighbor
	// ids — peak memory becomes proportional to the result set, not the repo.
	ids := make([]string, 0, len(scored))
	for _, sc := range scored {
		ids = append(ids, sc.SymbolID)
	}
	syms, err := e.store.SymbolsByIDs(ctx, snap.ID, ids)
	if err != nil {
		return nil, fmt.Errorf("engine: load symbols: %w", err)
	}
	byID := make(map[string]graph.CodeSymbol, len(syms))
	for _, s := range syms {
		byID[s.ID] = s
	}
	out := make([]SearchHit, 0, len(scored))
	for _, sc := range scored {
		s, ok := byID[sc.SymbolID]
		if !ok {
			continue
		}
		out = append(out, symbolToHit(s, sc.Score))
	}
	return &SemanticSearchResult{Results: out, Degraded: false, ModeUsed: "semantic"}, nil
}

// hybridSearch blends lexical and semantic results when vectors are on, else runs
// lexical and reports ModeUsed:"lexical (degraded)". The blend unions the two hit
// lists by symbol id, keeping the higher score, then sorts descending and caps to
// the limit.
func (e *localEngine) hybridSearch(ctx context.Context, in SearchInput) (*SearchResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	limit := searchLimit(in.Limit)

	lex, err := e.lexicalSearch(ctx, snap.ID, in.Query, in.Kind, limit)
	if err != nil {
		return nil, err
	}
	// SemanticSearch self-degrades (sem.Degraded) when this snapshot has no
	// embeddings, so no global-flag gate is needed here — the check below blends
	// only when a real semantic ranking came back.
	sem, err := e.SemanticSearch(ctx, SemanticSearchInput{Query: in.Query, RepoID: in.RepoID, Limit: limit})
	if err != nil {
		return nil, err
	}
	if sem.Degraded {
		// No embeddings for this snapshot: hybrid is just lexical, said honestly.
		return &SearchResult{Results: lex, ModeUsed: "lexical (degraded)", Total: len(lex)}, nil
	}
	blended := blendHits(lex, filterByKind(sem.Results, in.Kind), limit)
	return &SearchResult{Results: blended, ModeUsed: "hybrid", Total: len(blended)}, nil
}

// lexicalSearch runs the BM25 lexical query for a snapshot and maps hits to
// SearchHits, applying the optional kind filter. It is the shared core behind
// Search (lexical mode), the semantic degrade path, and hybrid's lexical leg.
func (e *localEngine) lexicalSearch(ctx context.Context, snapshotID, query, kind string, limit int) ([]SearchHit, error) {
	lx, err := e.ensureLexical()
	if err != nil {
		return e.fallbackLexicalSearch(ctx, snapshotID, query, kind, limit)
	}
	hits, err := lx.Search(snapshotID, query, limit*2) // over-fetch for post-filtering
	if err != nil {
		return e.fallbackLexicalSearch(ctx, snapshotID, query, kind, limit)
	}
	// Resolve only the hit symbols (a handful), not the whole snapshot. Collect
	// the distinct hit ids and fetch them via the PK-indexed SymbolsByIDs instead
	// of scanning every symbol in the snapshot with ListSymbols.
	hitIDs := make([]string, 0, len(hits))
	seenID := make(map[string]bool, len(hits))
	for _, h := range hits {
		if h.SymbolID == "" || seenID[h.SymbolID] {
			continue
		}
		seenID[h.SymbolID] = true
		hitIDs = append(hitIDs, h.SymbolID)
	}
	syms, err := e.store.SymbolsByIDs(ctx, snapshotID, hitIDs)
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
		if kind != "" && !strings.EqualFold(s.Kind, kind) {
			continue
		}
		out = append(out, symbolToHit(s, h.Score))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *localEngine) fallbackLexicalSearch(ctx context.Context, snapshotID, rawQuery, kind string, limit int) ([]SearchHit, error) {
	syms, err := e.store.ListSymbols(ctx, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("engine: fallback search load symbols: %w", err)
	}
	tokens := lexical.TokenizeIdentifier(rawQuery)
	if len(tokens) == 0 {
		return nil, nil
	}
	type scored struct {
		symbol graph.CodeSymbol
		score  float64
	}
	scoredHits := make([]scored, 0, limit)
	for _, sym := range syms {
		if kind != "" && !strings.EqualFold(sym.Kind, kind) {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{sym.Name, sym.Signature, sym.Doc, sym.Path, sym.Kind}, " "))
		score := 0.0
		for _, token := range tokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if score == 0 {
			continue
		}
		scoredHits = append(scoredHits, scored{symbol: sym, score: score})
	}
	sort.Slice(scoredHits, func(i, j int) bool {
		if scoredHits[i].score != scoredHits[j].score {
			return scoredHits[i].score > scoredHits[j].score
		}
		if scoredHits[i].symbol.Path != scoredHits[j].symbol.Path {
			return scoredHits[i].symbol.Path < scoredHits[j].symbol.Path
		}
		return scoredHits[i].symbol.Name < scoredHits[j].symbol.Name
	})
	if len(scoredHits) > limit {
		scoredHits = scoredHits[:limit]
	}
	out := make([]SearchHit, 0, len(scoredHits))
	for _, hit := range scoredHits {
		out = append(out, symbolToHit(hit.symbol, hit.score))
	}
	return out, nil
}

// searchLimit applies the default result cap (20) for a non-positive request.
func searchLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	return limit
}

// symbolToHit projects a symbol + score into a SearchHit.
func symbolToHit(s graph.CodeSymbol, score float64) SearchHit {
	return SearchHit{
		SymbolID:  s.ID,
		Name:      s.Name,
		Kind:      s.Kind,
		RepoID:    s.RepoID,
		Path:      s.Path,
		Line:      s.StartLine,
		Signature: s.Signature,
		Doc:       s.Doc,
		Score:     finiteScore(score),
	}
}

func finiteScore(score float64) float64 {
	switch {
	case math.IsNaN(score):
		return 0
	case math.IsInf(score, 1):
		return math.MaxFloat64
	case math.IsInf(score, -1):
		return -math.MaxFloat64
	default:
		return score
	}
}

// filterByKind keeps only hits whose Kind matches (case-insensitive); an empty
// kind passes everything through unchanged.
func filterByKind(hits []SearchHit, kind string) []SearchHit {
	if kind == "" {
		return hits
	}
	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		if strings.EqualFold(h.Kind, kind) {
			out = append(out, h)
		}
	}
	return out
}

// blendHits unions lexical and semantic hits by symbol id (keeping the higher
// score), sorts descending (ties broken by symbol id for determinism), and caps
// to limit. Lexical and semantic scores live on different scales, so this is a
// best-effort recall union, not a calibrated rank fusion.
func blendHits(lex, sem []SearchHit, limit int) []SearchHit {
	best := make(map[string]SearchHit, len(lex)+len(sem))
	merge := func(hits []SearchHit) {
		for _, h := range hits {
			if cur, ok := best[h.SymbolID]; !ok || h.Score > cur.Score {
				best[h.SymbolID] = h
			}
		}
	}
	merge(lex)
	merge(sem)
	out := make([]SearchHit, 0, len(best))
	for _, h := range best {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].SymbolID < out[j].SymbolID
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (e *localEngine) Impact(ctx context.Context, in ImpactInput) (*ImpactResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	depth := in.MaxDepth
	if depth <= 0 {
		depth = 3
	}
	// Scalable reverse-BFS: ImpactGraph drives the traversal through INDEXED store
	// reads (SymbolsByName/SymbolsByPath/CallEdgesByToRefs), touching only the blast
	// radius instead of loading the whole snapshot into memory.
	r, err := query.ImpactGraph(ctx, e.store, snap.ID, in.ChangedPaths, in.Symbols, depth)
	if err != nil {
		return nil, fmt.Errorf("engine: impact: %w", err)
	}
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
	repos, err := e.store.ListRepos(ctx, e.cfg.Scope)
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

// Link registers a repo into the graph WITHOUT indexing it, so it participates
// in cross-repo and shows in status/repos. (Webhooks and index jobs are Pulse's
// job — Atlas link is declarative repo registration only.) It derives the
// full_name/root from the ref (path vs remote URL vs bare org/name), upserts the
// repo via EnsureRepo with a pending status, and reports whether the row was
// newly created and whether it already had an index.
func (e *localEngine) Link(ctx context.Context, in LinkInput) (*LinkResult, error) {
	ref := strings.TrimSpace(in.Repo)
	if ref == "" {
		return nil, errors.New("atlas: link needs a repo (path, remote URL, or org/name)")
	}
	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = "main"
	}
	fullName, root := deriveRepoRef(ref)

	// Created = the repo did not already exist under this scope (case-insensitive
	// full_name match), checked BEFORE the upsert.
	created := true
	if repos, err := e.store.ListRepos(ctx, e.cfg.Scope); err == nil {
		for i := range repos {
			if strings.EqualFold(repos[i].FullName, fullName) {
				created = false
				break
			}
		}
	}

	repo, err := e.store.EnsureRepo(ctx, &graph.Repo{
		FullName:      fullName,
		Root:          root,
		DefaultBranch: branch,
		Scope:         e.cfg.Scope,
		Status:        graph.StatusPending,
	})
	if err != nil {
		return nil, fmt.Errorf("engine: link: %w", err)
	}

	indexed := false
	if snap, _ := e.store.LatestSnapshot(ctx, repo.ID); snap != nil {
		indexed = true
	}

	return &LinkResult{
		RepoID:        repo.ID,
		FullName:      repo.FullName,
		Root:          repo.Root,
		DefaultBranch: repo.DefaultBranch,
		Scope:         e.cfg.Scope,
		Created:       created,
		Indexed:       indexed,
	}, nil
}

// deriveRepoRef resolves a link ref into a (full_name, root) pair. A git remote
// URL (git@host:Org/Repo.git or https://host/Org/Repo(.git)) yields full_name
// "Org/Repo" and the remote URL string as root. An existing path or a
// path-looking ref yields filepath.Base(filepath.Abs(ref)) as full_name and the
// absolute path as root. Anything else (a bare org/name) is taken verbatim as
// both full_name and root.
func deriveRepoRef(ref string) (fullName, root string) {
	ref = strings.TrimSpace(ref)
	if name, ok := remoteURLFullName(ref); ok {
		return name, ref
	}
	if looksLikePath(ref) {
		abs, err := filepath.Abs(ref)
		if err != nil {
			abs = ref
		}
		return filepath.Base(abs), abs
	}
	return ref, ref
}

// remoteURLFullName extracts "Org/Repo" from a git remote URL. It recognizes the
// scp-like SSH form (git@host:Org/Repo.git) and the http(s)/ssh/git URL forms
// (scheme://host/Org/Repo(.git)). It returns ok=false for non-URL refs.
func remoteURLFullName(ref string) (string, bool) {
	// scp-like SSH: git@host:Org/Repo(.git) — has a colon before any slash and no scheme.
	if !strings.Contains(ref, "://") && strings.Contains(ref, "@") && strings.Contains(ref, ":") {
		if i := strings.Index(ref, ":"); i >= 0 {
			if name, ok := orgRepoFromPath(ref[i+1:]); ok {
				return name, true
			}
		}
		return "", false
	}
	// scheme://host/Org/Repo(.git)
	for _, scheme := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(ref, scheme) {
			rest := strings.TrimPrefix(ref, scheme)
			if i := strings.Index(rest, "/"); i >= 0 {
				return orgRepoFromPath(rest[i+1:])
			}
			return "", false
		}
	}
	return "", false
}

// orgRepoFromPath turns a "Org/Repo(.git)" remote path (possibly with extra
// leading/trailing slashes) into "Org/Repo". It returns ok=false when it cannot
// recover both an org and a repo segment.
func orgRepoFromPath(p string) (string, bool) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return "", false
	}
	org := parts[len(parts)-2]
	repo := parts[len(parts)-1]
	if org == "" || repo == "" {
		return "", false
	}
	return org + "/" + repo, true
}

// looksLikePath reports whether a ref should be treated as a filesystem path: it
// already exists on disk, or it carries an unambiguous path shape (absolute,
// relative "./"/"../", or "~"). A bare "org/name" is NOT a path.
func looksLikePath(ref string) bool {
	if ref == "" {
		return false
	}
	if _, err := os.Stat(ref); err == nil {
		return true
	}
	if filepath.IsAbs(ref) {
		return true
	}
	if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") ||
		strings.HasPrefix(ref, ".\\") || strings.HasPrefix(ref, "..\\") ||
		strings.HasPrefix(ref, "~") {
		return true
	}
	return false
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
		if fast, ok := e.store.(interface {
			LatestSnapshotByRepoRef(context.Context, string, string) (*graph.Snapshot, error)
		}); ok {
			snap, err := fast.LatestSnapshotByRepoRef(ctx, e.cfg.Scope, repoID)
			if err != nil {
				return nil, err
			}
			if snap != nil {
				return snap, nil
			}
		}
		// Resolve the ref (repo_id, org/name, or path) to a canonical repo
		// first — LatestSnapshot keys on the repo UUID, so a bare full_name or
		// path would otherwise miss. Mirrors resolveRepo used by the other ops.
		repo, err := e.resolveRepo(ctx, repoID)
		if err != nil {
			return nil, err
		}
		snap, err := e.store.LatestSnapshot(ctx, repo.ID)
		if err != nil {
			return nil, err
		}
		if snap == nil {
			return nil, ErrNoIndex
		}
		return snap, nil
	}
	snap, err := e.store.LatestSnapshotAny(ctx, e.cfg.Scope)
	if err != nil {
		return nil, err
	}
	if snap != nil {
		return snap, nil
	}
	return nil, ErrNoIndex
}

func (e *localEngine) Callers(ctx context.Context, in CallersInput) (*CallersResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	syms, err := query.CallersGraph(ctx, e.store, snap.ID, in.Name)
	if err != nil {
		return nil, fmt.Errorf("engine: callers: %w", err)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	return &CallersResult{Symbol: in.Name, Callers: refsOf(syms, limit), Total: len(syms)}, nil
}

func (e *localEngine) Symbol(ctx context.Context, in SymbolInput) (*SymbolResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	defs, err := e.store.SymbolsByName(ctx, snap.ID, in.Name)
	if err != nil {
		return nil, fmt.Errorf("engine: symbol: %w", err)
	}
	callers, err := query.CallersGraph(ctx, e.store, snap.ID, in.Name)
	if err != nil {
		return nil, err
	}
	callees, err := query.CalleesGraph(ctx, e.store, snap.ID, in.Name)
	if err != nil {
		return nil, err
	}
	callerRefs, calleeRefs := refsOf(callers, 50), refsOf(callees, 50)
	matches := make([]SymbolDef, 0, len(defs))
	for _, s := range defs {
		matches = append(matches, SymbolDef{
			SymbolID: s.ID, Name: s.Name, Kind: s.Kind, RepoID: s.RepoID,
			Path: s.Path, Line: s.StartLine, EndLine: s.EndLine,
			Signature: s.Signature, Doc: s.Doc,
			Callers: callerRefs, Callees: calleeRefs,
		})
	}
	return &SymbolResult{Query: in.Name, Matches: matches}, nil
}

func symRef(s graph.CodeSymbol) SymbolRef {
	return SymbolRef{SymbolID: s.ID, Name: s.Name, Kind: s.Kind, Path: s.Path, Line: s.StartLine, Signature: s.Signature}
}

func refsOf(syms []graph.CodeSymbol, limit int) []SymbolRef {
	out := make([]SymbolRef, 0, len(syms))
	for _, s := range syms {
		out = append(out, symRef(s))
		if len(out) >= limit {
			break
		}
	}
	return out
}

// navCap bounds the lists the local navigation ops return.
const navCap = 200

func (e *localEngine) Neighbors(ctx context.Context, in NeighborsInput) (*NeighborsResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	callers, err := query.CallersGraph(ctx, e.store, snap.ID, in.Name)
	if err != nil {
		return nil, fmt.Errorf("engine: neighbors callers: %w", err)
	}
	callees, err := query.CalleesGraph(ctx, e.store, snap.ID, in.Name)
	if err != nil {
		return nil, fmt.Errorf("engine: neighbors callees: %w", err)
	}
	return &NeighborsResult{
		Symbol:  in.Name,
		Callers: refsOf(callers, navCap),
		Callees: refsOf(callees, navCap),
	}, nil
}

func (e *localEngine) Path(ctx context.Context, in PathInput) (*PathResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	depth := in.MaxDepth
	if depth <= 0 {
		depth = 6
	}
	chain, err := query.Path(ctx, e.store, snap.ID, in.From, in.To, depth)
	if err != nil {
		return nil, fmt.Errorf("engine: path: %w", err)
	}
	res := &PathResult{From: in.From, To: in.To, Steps: []SymbolRef{}}
	if len(chain) > 0 {
		res.Found = true
		res.Length = len(chain) - 1 // edges, not nodes
		res.Steps = refsOf(chain, navCap)
	}
	return res, nil
}

func (e *localEngine) Refs(ctx context.Context, in RefsInput) (*RefsResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	// True references = the UNION of call-site references (resolved callers over
	// EdgeCalls) and type-use references (symbols that name this symbol as a type,
	// over the EdgeReferences edges emitted by the go/types analyzer). Deduped by
	// symbol identity so a symbol that both calls AND type-references appears once.
	callers, err := query.CallersGraph(ctx, e.store, snap.ID, in.Name)
	if err != nil {
		return nil, fmt.Errorf("engine: refs call-sites: %w", err)
	}
	typeUses, err := query.ReferencesGraph(ctx, e.store, snap.ID, in.Name)
	if err != nil {
		return nil, fmt.Errorf("engine: refs type-uses: %w", err)
	}
	syms := unionSymbols(callers, typeUses)
	return &RefsResult{Symbol: in.Name, References: refsOf(syms, navCap), Total: len(syms)}, nil
}

// unionSymbols merges two symbol slices, deduping by symbol identity and keeping
// a deterministic (path, then name) order. The identity key mirrors query's
// symbolKey rule (id, then node_id, else path+name+start-line).
func unionSymbols(a, b []graph.CodeSymbol) []graph.CodeSymbol {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]graph.CodeSymbol, 0, len(a)+len(b))
	add := func(syms []graph.CodeSymbol) {
		for _, s := range syms {
			k := symbolIdentity(s)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, s)
		}
	}
	add(a)
	add(b)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// symbolIdentity is a stable dedup key for a symbol: prefer ID, then NodeID, else
// path + name + start-line (mirroring query.symbolKey).
func symbolIdentity(s graph.CodeSymbol) string {
	if s.ID != "" {
		return "id:" + s.ID
	}
	if s.NodeID != "" {
		return "node:" + string(s.NodeID)
	}
	return "fnl:" + s.Path + "\x00" + s.Name + "\x00" + strconv.Itoa(s.StartLine)
}

func (e *localEngine) Explain(ctx context.Context, in ExplainInput) (*ExplainResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	defs, err := e.store.SymbolsByName(ctx, snap.ID, in.Name)
	if err != nil {
		return nil, fmt.Errorf("engine: explain defs: %w", err)
	}
	res := &ExplainResult{
		Symbol:      in.Name,
		Definitions: make([]ExplainDef, 0, len(defs)),
	}
	if in.CountsOnly {
		callerCount, err := query.CallersGraphCount(ctx, e.store, snap.ID, in.Name)
		if err != nil {
			return nil, fmt.Errorf("engine: explain callers: %w", err)
		}
		calleeCount, err := query.CalleesGraphCount(ctx, e.store, snap.ID, in.Name)
		if err != nil {
			return nil, fmt.Errorf("engine: explain callees: %w", err)
		}
		res.CallerCount = callerCount
		res.CalleeCount = calleeCount
	} else {
		callers, err := query.CallersGraph(ctx, e.store, snap.ID, in.Name)
		if err != nil {
			return nil, fmt.Errorf("engine: explain callers: %w", err)
		}
		callees, err := query.CalleesGraph(ctx, e.store, snap.ID, in.Name)
		if err != nil {
			return nil, fmt.Errorf("engine: explain callees: %w", err)
		}
		res.Callers = namesOf(callers, navCap)
		res.Callees = namesOf(callees, navCap)
	}
	defPaths := map[string]bool{}
	for _, s := range defs {
		res.Definitions = append(res.Definitions, ExplainDef{
			SymbolID: s.ID, Kind: s.Kind, Path: s.Path, Line: s.StartLine,
			EndLine: s.EndLine, Signature: s.Signature, Doc: s.Doc,
		})
		if p := strings.TrimSpace(s.Path); p != "" {
			defPaths[p] = true
		}
		if len(res.Definitions) >= navCap {
			break
		}
	}

	// Imports of the defining file(s), via the indexed file rows.
	res.Imports = capStrings(e.importsForPaths(ctx, snap.ID, defPaths), navCap)
	if len(res.Imports) == 0 {
		res.Imports = nil
	}

	servedLabels := map[string]bool{}
	if snap.RouteCount > 0 {
		// Producer routes served by this symbol: handler_symbol == SYMBOL OR the
		// route's handler file is one of the definition paths.
		routes, err := e.store.ListRoutes(ctx, snap.ID, "producer")
		if err != nil {
			return nil, fmt.Errorf("engine: explain routes: %w", err)
		}
		for _, r := range routes {
			if metaStr(r.Metadata, "handler_symbol") == in.Name ||
				(r.HandlerFile != "" && defPaths[r.HandlerFile]) {
				res.ServedRoutes = append(res.ServedRoutes, ExplainRoute{
					Method: r.Method, Path: r.PathPattern, HandlerFile: r.HandlerFile,
				})
				servedLabels[routeLabelEng(r.Method, r.PathPattern)] = true
				if len(res.ServedRoutes) >= navCap {
					break
				}
			}
		}
	}

	// Cross-repo consumers — only when this symbol actually serves routes; keep
	// hits whose matched route is one of the served routes.
	if len(servedLabels) > 0 {
		if repo, rerr := e.resolveRepo(ctx, in.RepoID); rerr == nil {
			cr, cerr := crossrepo.Consumers(ctx, e.store, e.cfg.Scope, repo.FullName)
			if cerr == nil {
				seen := map[string]bool{}
				for _, h := range cr.Impacted {
					if !servedLabels[h.MatchedRoute] {
						continue
					}
					rk := strings.ToLower(h.Repo)
					if seen[rk] {
						continue
					}
					seen[rk] = true
					res.CrossRepoConsumers = append(res.CrossRepoConsumers, h.Repo)
					if len(res.CrossRepoConsumers) >= navCap {
						break
					}
				}
			}
		}
	}
	return res, nil
}

func (e *localEngine) Coverage(ctx context.Context, in CoverageInput) (*CoverageResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}

	// Runtime-first: if a coverage profile was imported for this target, report the
	// real covered/total line ratio (mode="runtime"). Multiple symbols can share a
	// name (overloads / per-package); a target is "covered" when ANY of its facts
	// reports a covered line, and the reported strength sums covered/total lines
	// across the matching facts.
	if facts, ferr := e.store.ListCoverage(ctx, snap.ID, in.Target); ferr == nil && len(facts) > 0 {
		coveredLines, totalLines := 0, 0
		coverageType := ""
		for _, f := range facts {
			c, t := parseStrengthRatio(f.Strength)
			coveredLines += c
			totalLines += t
			if coverageType == "" {
				coverageType = f.CoverageType
			}
		}
		dir := coverageType
		if dir == "" {
			dir = "runtime"
		}
		return &CoverageResult{
			Target:    in.Target,
			Mode:      "runtime",
			Direction: dir,
			Covered:   coveredLines > 0,
			Strength:  fmt.Sprintf("%d/%d", coveredLines, totalLines),
		}, nil
	}

	depth := in.MaxDepth
	if depth <= 0 {
		depth = 8
	}
	r, err := query.Coverage(ctx, e.store, snap.ID, in.Target, in.Direction, depth)
	if err != nil {
		return nil, fmt.Errorf("engine: coverage: %w", err)
	}
	return &CoverageResult{
		Target:    r.Target,
		Mode:      "static",
		Direction: r.Direction,
		Covered:   r.Covered,
		Tests:     coverageRefsToEngine(r.Tests, navCap),
		Symbols:   coverageRefsToEngine(r.Symbols, navCap),
	}, nil
}

// parseStrengthRatio parses a "covered/total" strength string into its two
// integer parts, returning (0,0) on any malformed input.
func parseStrengthRatio(s string) (covered, total int) {
	parts := strings.SplitN(strings.TrimSpace(s), "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	c, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	t, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return c, t
}

// CoverageImport ingests a runtime coverage profile (Go coverprofile or LCOV),
// maps the covered line sets onto the indexed symbols of the resolved snapshot,
// and persists one graph.Coverage fact per symbol. For each symbol the covered
// fraction is (#covered lines in [StartLine,EndLine]) / (line span); the fact's
// Strength is "coveredLines/spanLines" and CoverageType is "runtime_<format>".
// Re-importing replaces the snapshot's prior runtime facts (idempotent).
func (e *localEngine) CoverageImport(ctx context.Context, in CoverageImportInput) (*CoverageImportResult, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, errors.New("atlas: coverage import needs a profile path")
	}
	// Resolve the repo first (accepts an ID OR a full_name), then the snapshot from
	// its canonical id — so `--repo org/name` works as well as a raw repo_id.
	repo, err := e.resolveRepo(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	snap, err := e.resolveSnapshot(ctx, repo.ID)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(in.Path)
	if err != nil {
		return nil, fmt.Errorf("engine: read coverage profile: %w", err)
	}
	format, files, err := coverage.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("engine: parse coverage profile: %w", err)
	}

	// Index covered-line sets by canonical file path, joining absolute /
	// repo-relative profile paths against the (repo-relative) symbol paths via a
	// suffix match so a "github.com/org/repo/internal/x.go" coverprofile path lines
	// up with the stored "internal/x.go".
	coveredByFile := make(map[string]map[int]bool, len(files))
	var profilePaths []string
	for _, fc := range files {
		p := normalizeCoveragePath(fc.File)
		if p == "" {
			continue
		}
		if coveredByFile[p] == nil {
			coveredByFile[p] = map[int]bool{}
			profilePaths = append(profilePaths, p)
		}
		for ln, cov := range fc.Covered {
			if cov {
				coveredByFile[p][ln] = true
			}
		}
	}

	syms, err := e.store.ListSymbols(ctx, snap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: coverage import load symbols: %w", err)
	}

	coverageType := "runtime_" + format
	rows := make([]graph.Coverage, 0, len(syms))
	covered := 0
	for _, s := range syms {
		if s.StartLine <= 0 || s.EndLine < s.StartLine {
			continue
		}
		lineSet := coveredLinesForSymbol(coveredByFile, profilePaths, s.Path)
		if lineSet == nil {
			continue // no profile data for this file
		}
		span := s.EndLine - s.StartLine + 1
		hit := 0
		for ln := s.StartLine; ln <= s.EndLine; ln++ {
			if lineSet[ln] {
				hit++
			}
		}
		if hit > 0 {
			covered++
		}
		rows = append(rows, graph.Coverage{
			SnapshotID:   snap.ID,
			RepoFullName: repo.FullName,
			SymbolRef:    s.Name,
			CoverageType: coverageType,
			Strength:     fmt.Sprintf("%d/%d", hit, span),
		})
	}

	if err := e.store.SaveCoverage(ctx, rows); err != nil {
		return nil, fmt.Errorf("engine: save coverage: %w", err)
	}

	return &CoverageImportResult{
		SnapshotID:     snap.ID,
		RepoFullName:   repo.FullName,
		Format:         format,
		ProfileFiles:   len(files),
		SymbolsTotal:   len(rows),
		SymbolsCovered: covered,
		FactsWritten:   len(rows),
	}, nil
}

// normalizeCoveragePath canonicalizes a coverage-profile file path: trim, convert
// backslashes, and strip a leading "./" — matching query.canonicalPath so the
// suffix join lines up with stored symbol paths.
func normalizeCoveragePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.TrimPrefix(p, "./")
}

// coveredLinesForSymbol returns the covered-line set for a symbol's file. It
// first tries an exact canonical match, then a suffix match in either direction
// (profile path ends with the symbol path, or vice-versa) so absolute or
// module-qualified coverprofile paths join the repo-relative symbol paths.
func coveredLinesForSymbol(coveredByFile map[string]map[int]bool, profilePaths []string, symbolPath string) map[int]bool {
	sp := normalizeCoveragePath(symbolPath)
	if sp == "" {
		return nil
	}
	if m, ok := coveredByFile[sp]; ok {
		return m
	}
	for _, pp := range profilePaths {
		if strings.HasSuffix(pp, "/"+sp) || strings.HasSuffix(sp, "/"+pp) {
			return coveredByFile[pp]
		}
	}
	return nil
}

// namesOf returns the distinct symbol names (capped), order-preserving.
func namesOf(syms []graph.CodeSymbol, limit int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		if s.Name == "" || seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		out = append(out, s.Name)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// importsForPaths aggregates the dedup'd import list of the given files from the
// indexed file rows of the snapshot. Best-effort: an empty result is fine.
func (e *localEngine) importsForPaths(ctx context.Context, snapshotID string, paths map[string]bool) []string {
	if len(paths) == 0 {
		return nil
	}
	files, err := e.store.FilesByPaths(ctx, snapshotID, trueKeys(paths))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		for _, imp := range f.Imports {
			imp = strings.TrimSpace(imp)
			if imp == "" || seen[imp] {
				continue
			}
			seen[imp] = true
			out = append(out, imp)
		}
	}
	sort.Strings(out)
	return out
}

func trueKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k, ok := range set {
		if ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// routeLabelEng renders "METHOD path" (METHOD omitted when unknown), matching
// the cross-repo MatchedRoute label so the served/consumer join lines up.
func routeLabelEng(method, path string) string {
	m := strings.TrimSpace(strings.ToUpper(method))
	if m == "" {
		return path
	}
	return m + " " + path
}

func coverageRefsToEngine(refs []query.CoverageRef, limit int) []SymbolRef {
	out := make([]SymbolRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, SymbolRef{SymbolID: r.SymbolID, Name: r.Name, Kind: r.Kind, Path: r.Path, Line: r.Line})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (e *localEngine) GraphExport(ctx context.Context, in GraphExportInput) (*GraphExportResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	var g export.Graph
	if in.All {
		g, err = e.streamFullGraph(ctx, snap.ID)
		if err != nil {
			return nil, fmt.Errorf("engine: export load graph: %w", err)
		}
	} else {
		if strings.TrimSpace(in.Symbol) == "" {
			return nil, errors.New("atlas: graph export needs --symbol (a focus symbol) or --all")
		}
		depth := in.Depth
		if depth <= 0 {
			depth = 2
		}
		maxN := in.MaxNodes
		if maxN <= 0 {
			maxN = 200
		}
		sg, err := query.Subgraph(ctx, e.store, snap.ID, in.Symbol, depth, maxN)
		if err != nil {
			return nil, fmt.Errorf("engine: subgraph: %w", err)
		}
		g = subgraphToExport(sg)
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = "json"
	}
	var content string
	if format == "html" {
		// The interactive HTML page gets a context-aware title; everything else
		// flows through the generic Render path so json/mermaid/dot are unchanged.
		title := "Atlas graph"
		if in.All {
			if in.RepoID != "" {
				title = "Atlas — " + in.RepoID
			}
		} else if s := strings.TrimSpace(in.Symbol); s != "" {
			title = "Atlas — " + s
		}
		content, err = g.HTML(export.HTMLOptions{Title: title})
	} else {
		content, err = g.Render(format)
	}
	if err != nil {
		return nil, err
	}
	return &GraphExportResult{Format: format, Nodes: len(g.Nodes), Edges: len(g.Edges), Content: content}, nil
}

// subgraphToExport maps a name-level neighborhood subgraph to an export.Graph.
func subgraphToExport(sg query.SubgraphResult) export.Graph {
	byName := make(map[string]string, len(sg.Nodes))
	g := export.Graph{}
	for _, s := range sg.Nodes {
		g.Nodes = append(g.Nodes, export.Node{ID: s.ID, Name: s.Name, Kind: s.Kind, Path: s.Path, Line: s.StartLine, Language: s.Language})
		byName[strings.ToLower(s.Name)] = s.ID
	}
	for _, e := range sg.Edges {
		f, ok1 := byName[strings.ToLower(e.From)]
		t, ok2 := byName[strings.ToLower(e.To)]
		if ok1 && ok2 {
			g.Edges = append(g.Edges, export.Edge{From: f, To: t, Kind: "calls"})
		}
	}
	return g
}

// streamFullGraph maps a whole snapshot to an export.Graph: every symbol is a
// node; call edges connect by name to a representative node (external callees,
// whose to_ref is not an indexed symbol, are dropped).
//
// It STREAMS symbols then edges rather than loading both whole-graph slices, so
// peak RAM is bounded by the projected export.Graph (only ID/Name/Kind/Path/Line/
// Language survive per node) plus the name→rep map — never the heavy per-symbol
// signature/doc/metadata payload (the body_excerpt that dominates each row). The
// output is byte-identical to the old slice-based fold: StreamSymbols/StreamEdges
// yield the same rows in the same order as ListSymbols/ListEdges, and the fold is
// order-deterministic (first-seen rep, first-seen edge key).
func (e *localEngine) streamFullGraph(ctx context.Context, snapID string) (export.Graph, error) {
	g := export.Graph{}
	rep := map[string]string{}
	if err := e.store.StreamSymbols(ctx, snapID, 0, func(batch []graph.CodeSymbol) error {
		for i := range batch {
			s := &batch[i]
			g.Nodes = append(g.Nodes, export.Node{ID: s.ID, Name: s.Name, Kind: s.Kind, Path: s.Path, Line: s.StartLine, Language: s.Language})
			if k := strings.ToLower(s.Name); k != "" {
				if _, ok := rep[k]; !ok {
					rep[k] = s.ID
				}
			}
		}
		return nil
	}); err != nil {
		return export.Graph{}, err
	}
	seen := map[string]bool{}
	if err := e.store.StreamEdges(ctx, snapID, 0, func(batch []graph.DependencyEdge) error {
		for i := range batch {
			ed := &batch[i]
			if ed.Kind != graph.EdgeCalls {
				continue
			}
			f, ok1 := rep[strings.ToLower(strings.TrimSpace(ed.FromSymbol))]
			t, ok2 := rep[strings.ToLower(strings.TrimSpace(ed.ToRef))]
			if !ok1 || !ok2 || f == t {
				continue
			}
			if key := f + "\x00" + t; !seen[key] {
				seen[key] = true
				g.Edges = append(g.Edges, export.Edge{From: f, To: t, Kind: "calls"})
			}
		}
		return nil
	}); err != nil {
		return export.Graph{}, err
	}
	return g, nil
}

func (e *localEngine) History(ctx context.Context, in HistoryInput) (*HistoryResult, error) {
	repo, err := e.resolveRepo(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	snaps, err := e.store.ListSnapshots(ctx, repo.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("engine: history: %w", err)
	}
	out := make([]SnapshotInfo, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, SnapshotInfo{
			SnapshotID: s.ID, CommitSHA: s.CommitSHA, Branch: s.Branch,
			Files: s.FileCount, Symbols: s.SymbolCount, Edges: s.EdgeCount,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
		})
	}
	return &HistoryResult{RepoID: repo.ID, FullName: repo.FullName, Snapshots: out}, nil
}

func (e *localEngine) SnapshotDiff(ctx context.Context, in SnapshotDiffInput) (*SnapshotDiffResult, error) {
	repo, err := e.resolveRepo(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	snaps, err := e.store.ListSnapshots(ctx, repo.ID, 500) // newest first
	if err != nil {
		return nil, fmt.Errorf("engine: diff: %w", err)
	}
	if len(snaps) == 0 {
		return nil, ErrNoIndex
	}
	// Resolve the head ("to") snapshot, default = latest.
	toSnap := &snaps[0]
	if in.To != "" {
		if m := matchSnapshot(snaps, in.To); m != nil {
			toSnap = m
		} else {
			return nil, fmt.Errorf("atlas: snapshot %q not found", in.To)
		}
	}
	// Resolve the base ("from") snapshot, default = the one indexed just before To.
	var fromSnap *graph.Snapshot
	if in.From != "" {
		if fromSnap = matchSnapshot(snaps, in.From); fromSnap == nil {
			return nil, fmt.Errorf("atlas: snapshot %q not found", in.From)
		}
	} else {
		fromSnap = snapshotBefore(snaps, toSnap.ID)
	}
	if fromSnap == nil {
		return nil, errors.New("atlas: need two snapshots to diff — index the repo at two commits, or pass --from")
	}

	symsA, err := e.store.ListSymbols(ctx, fromSnap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: diff load base symbols: %w", err)
	}
	symsB, err := e.store.ListSymbols(ctx, toSnap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: diff load head symbols: %w", err)
	}
	edgesA, err := e.store.ListEdges(ctx, fromSnap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: diff load base edges: %w", err)
	}
	edgesB, err := e.store.ListEdges(ctx, toSnap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: diff load head edges: %w", err)
	}
	d := query.Diff(symsA, symsB, edgesA, edgesB)
	if d.ChangedFiles == nil {
		d.ChangedFiles = []string{}
	}
	const cap = 100
	return &SnapshotDiffResult{
		FromCommit: fromSnap.CommitSHA, FromSnapshot: fromSnap.ID,
		ToCommit: toSnap.CommitSHA, ToSnapshot: toSnap.ID,
		AddedCount: len(d.Added), RemovedCount: len(d.Removed), ModifiedCount: len(d.Modified),
		Added: capChanges(d.Added, cap), Removed: capChanges(d.Removed, cap), Modified: capChanges(d.Modified, cap),
		ChangedFiles: d.ChangedFiles,
		AddedEdges:   capEdges(d.AddedEdges, cap), RemovedEdges: capEdges(d.RemovedEdges, cap),
	}, nil
}

// crossRepoCap bounds the lists returned by the cross-repo ops so a hub repo's
// fan-out can't return an unbounded payload.
const crossRepoCap = 200

// resolveRepoFullName resolves the repo full_name a cross-repo op runs against:
// the named one, else the single/most-recent indexed repo (reusing resolveRepo).
func (e *localEngine) resolveRepoFullName(ctx context.Context, repo string) (string, error) {
	if strings.TrimSpace(repo) != "" {
		return repo, nil
	}
	r, err := e.resolveRepo(ctx, "")
	if err != nil {
		return "", err
	}
	return r.FullName, nil
}

func (e *localEngine) CrossRepoImpact(ctx context.Context, in CrossRepoImpactInput) (*CrossRepoImpactResult, error) {
	repo, err := e.resolveRepoFullName(ctx, in.Repo)
	if err != nil {
		return nil, err
	}
	r, err := crossrepo.Impact(ctx, e.store, e.cfg.Scope, repo, in.ChangedPaths)
	if err != nil {
		if errors.Is(err, crossrepo.ErrRepoNotFound) {
			return nil, ErrNoIndex
		}
		return nil, fmt.Errorf("engine: cross-repo impact: %w", err)
	}
	return &CrossRepoImpactResult{
		Repo:          r.Repo,
		ServedRoutes:  routeContracts(r.ServedRoutes, crossRepoCap),
		Impacted:      consumerHits(r.Impacted, crossRepoCap),
		ConsumerRepos: capStrings(r.ConsumerRepos, crossRepoCap),
	}, nil
}

func (e *localEngine) Consumers(ctx context.Context, in ConsumersInput) (*ConsumersResult, error) {
	repo, err := e.resolveRepoFullName(ctx, in.Repo)
	if err != nil {
		return nil, err
	}
	r, err := crossrepo.Consumers(ctx, e.store, e.cfg.Scope, repo)
	if err != nil {
		if errors.Is(err, crossrepo.ErrRepoNotFound) {
			return nil, ErrNoIndex
		}
		return nil, fmt.Errorf("engine: consumers: %w", err)
	}
	return &ConsumersResult{
		Repo:          r.Repo,
		Impacted:      consumerHits(r.Impacted, crossRepoCap),
		ConsumerRepos: capStrings(r.ConsumerRepos, crossRepoCap),
	}, nil
}

func (e *localEngine) RouteContracts(ctx context.Context, in RouteContractsInput) (*RouteContractsResult, error) {
	repo, err := e.resolveRepoFullName(ctx, in.Repo)
	if err != nil {
		return nil, err
	}
	routes, err := crossrepo.RouteContracts(ctx, e.store, e.cfg.Scope, repo)
	if err != nil {
		return nil, fmt.Errorf("engine: route contracts: %w", err)
	}
	return &RouteContractsResult{
		Repo:   repo,
		Routes: routeContracts(routes, crossRepoCap),
		Total:  len(routes),
	}, nil
}

// routeContracts maps producer graph.Routes to JSON-friendly RouteContracts, capped at n.
func routeContracts(routes []graph.Route, n int) []RouteContract {
	out := make([]RouteContract, 0, len(routes))
	for _, r := range routes {
		out = append(out, RouteContract{
			Method:        r.Method,
			PathPattern:   r.PathPattern,
			HandlerFile:   r.HandlerFile,
			HandlerSymbol: metaStr(r.Metadata, "handler_symbol"),
			Source:        r.Source,
			Confidence:    r.Confidence,
		})
		if len(out) >= n {
			break
		}
	}
	return out
}

// consumerHits maps crossrepo hits to engine ConsumerHits, capped at n.
func consumerHits(hits []crossrepo.ConsumerHit, n int) []ConsumerHit {
	out := make([]ConsumerHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, ConsumerHit{
			Repo:          h.Repo,
			CallingFile:   h.CallingFile,
			CallingSymbol: h.CallingSymbol,
			MatchedRoute:  h.MatchedRoute,
			Endpoint:      h.Endpoint,
		})
		if len(out) >= n {
			break
		}
	}
	return out
}

func capStrings(s []string, n int) []string {
	if s == nil {
		return []string{}
	}
	if len(s) > n {
		return s[:n]
	}
	return s
}

func metaStr(m graph.JSONBMap, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// resolveRepo selects the repo a temporal op runs against: the named one, the
// single indexed repo, or the most recently indexed.
func (e *localEngine) resolveRepo(ctx context.Context, repoID string) (*graph.Repo, error) {
	repos, err := e.store.ListRepos(ctx, e.cfg.Scope)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, ErrNoIndex
	}
	if repoID != "" {
		for i := range repos {
			if repos[i].ID == repoID || strings.EqualFold(repos[i].FullName, repoID) {
				return &repos[i], nil
			}
		}
		// Path or bare-name ref: match on basename, mirroring the index-time
		// derivation (full_name defaults to filepath.Base of the indexed path).
		base := filepath.Base(repoID)
		for i := range repos {
			if strings.EqualFold(repos[i].FullName, base) || strings.EqualFold(filepath.Base(repos[i].FullName), base) {
				return &repos[i], nil
			}
		}
		return nil, fmt.Errorf("atlas: repo %q not found", repoID)
	}
	if len(repos) == 1 {
		return &repos[0], nil
	}
	best := &repos[0]
	for i := range repos {
		if repos[i].LastIndexedAt != nil && (best.LastIndexedAt == nil || repos[i].LastIndexedAt.After(*best.LastIndexedAt)) {
			best = &repos[i]
		}
	}
	return best, nil
}

// matchSnapshot finds a snapshot by exact ID, exact commit SHA, then commit-SHA prefix.
func matchSnapshot(snaps []graph.Snapshot, ref string) *graph.Snapshot {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	for i := range snaps {
		if snaps[i].ID == ref || snaps[i].CommitSHA == ref {
			return &snaps[i]
		}
	}
	for i := range snaps {
		if strings.HasPrefix(snaps[i].CommitSHA, ref) {
			return &snaps[i]
		}
	}
	return nil
}

// snapshotBefore returns the snapshot indexed immediately before the one with id
// (snaps is newest-first, so that's the next index).
func snapshotBefore(snaps []graph.Snapshot, id string) *graph.Snapshot {
	for i := range snaps {
		if snaps[i].ID == id && i+1 < len(snaps) {
			return &snaps[i+1]
		}
	}
	return nil
}

func capChanges(c []query.SymbolChange, n int) []query.SymbolChange {
	if c == nil {
		return []query.SymbolChange{}
	}
	if len(c) > n {
		return c[:n]
	}
	return c
}

func capEdges(c []query.EdgeChange, n int) []query.EdgeChange {
	if c == nil {
		return []query.EdgeChange{}
	}
	if len(c) > n {
		return c[:n]
	}
	return c
}
