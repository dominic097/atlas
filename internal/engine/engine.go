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
	"strings"
	"time"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/export"
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

// ── The Engine interface ────────────────────────────────────────────────────

// Engine is the single contract all surfaces depend on. The full catalog
// (callers, refs, neighbors, path, explain, graph_export, cross_repo_impact,
// consumers, route_contracts, history, snapshot_diff, coverage, repos, link)
// extends this interface following the same pattern.
type Engine interface {
	Index(ctx context.Context, in IndexInput) (*IndexResult, error)
	Search(ctx context.Context, in SearchInput) (*SearchResult, error)
	Impact(ctx context.Context, in ImpactInput) (*ImpactResult, error)
	Callers(ctx context.Context, in CallersInput) (*CallersResult, error)
	Symbol(ctx context.Context, in SymbolInput) (*SymbolResult, error)
	GraphExport(ctx context.Context, in GraphExportInput) (*GraphExportResult, error)
	History(ctx context.Context, in HistoryInput) (*HistoryResult, error)
	SnapshotDiff(ctx context.Context, in SnapshotDiffInput) (*SnapshotDiffResult, error)
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

func (e *localEngine) GraphExport(ctx context.Context, in GraphExportInput) (*GraphExportResult, error) {
	snap, err := e.resolveSnapshot(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	var g export.Graph
	if in.All {
		syms, err := e.store.ListSymbols(ctx, snap.ID)
		if err != nil {
			return nil, fmt.Errorf("engine: export load symbols: %w", err)
		}
		edges, err := e.store.ListEdges(ctx, snap.ID)
		if err != nil {
			return nil, fmt.Errorf("engine: export load edges: %w", err)
		}
		g = fullGraph(syms, edges)
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
	content, err := g.Render(in.Format)
	if err != nil {
		return nil, err
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = "json"
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

// fullGraph maps a whole snapshot to an export.Graph: every symbol is a node;
// call edges connect by name to a representative node (external callees, whose
// to_ref is not an indexed symbol, are dropped).
func fullGraph(syms []graph.CodeSymbol, edges []graph.DependencyEdge) export.Graph {
	g := export.Graph{}
	rep := make(map[string]string, len(syms))
	for _, s := range syms {
		g.Nodes = append(g.Nodes, export.Node{ID: s.ID, Name: s.Name, Kind: s.Kind, Path: s.Path, Line: s.StartLine, Language: s.Language})
		if k := strings.ToLower(s.Name); k != "" {
			if _, ok := rep[k]; !ok {
				rep[k] = s.ID
			}
		}
	}
	seen := map[string]bool{}
	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			continue
		}
		f, ok1 := rep[strings.ToLower(strings.TrimSpace(e.FromSymbol))]
		t, ok2 := rep[strings.ToLower(strings.TrimSpace(e.ToRef))]
		if !ok1 || !ok2 || f == t {
			continue
		}
		if key := f + "\x00" + t; !seen[key] {
			seen[key] = true
			g.Edges = append(g.Edges, export.Edge{From: f, To: t, Kind: "calls"})
		}
	}
	return g
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

// resolveRepo selects the repo a temporal op runs against: the named one, the
// single indexed repo, or the most recently indexed.
func (e *localEngine) resolveRepo(ctx context.Context, repoID string) (*graph.Repo, error) {
	repos, err := e.store.ListRepos(ctx, "")
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
