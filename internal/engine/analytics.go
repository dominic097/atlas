package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dominic097/atlas/internal/analytics"
	"github.com/dominic097/atlas/internal/graph"
)

// ── Graph-analytics I/O types (deterministic, LLM-free) ─────────────────────
//
// communities, hubs, and report are pure functions of the indexed call graph,
// computed by internal/analytics. Each loads the resolved snapshot's symbols +
// edges (mirroring impact/explain) and runs the analytics package — no model
// calls, no randomness, ties broken by name everywhere.

// CommunitiesInput selects the repo and caps how many communities to return.
type CommunitiesInput struct {
	RepoID string
	Limit  int
}

// CommunityInfo is a JSON-friendly projection of analytics.Community.
type CommunityInfo struct {
	ID              int      `json:"id"`
	Size            int      `json:"size"`
	Members         []string `json:"members"`
	Representatives []string `json:"representatives"`
}

// CommunitiesResult is the detected clusters (size-ranked) plus the total found
// before the Limit cap was applied.
type CommunitiesResult struct {
	Communities []CommunityInfo `json:"communities"`
	Total       int             `json:"total"`
}

// HubsInput selects the repo and caps how many hubs ("god nodes") to return.
type HubsInput struct {
	RepoID string
	Limit  int
}

// HubInfo is a JSON-friendly projection of analytics.Hub. Name is the QUALIFIED
// display name ("localEngine.Close") so distinct same-named methods are
// distinguishable; BareName keeps the unqualified symbol name.
type HubInfo struct {
	Name        string `json:"name"`
	BareName    string `json:"bare_name,omitempty"`
	Path        string `json:"path,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Language    string `json:"language,omitempty"`
	InDegree    int    `json:"in_degree"`
	OutDegree   int    `json:"out_degree"`
	TotalDegree int    `json:"total_degree"`
}

// HubsResult is the top hubs by total degree, descending (ties by name).
type HubsResult struct {
	Hubs []HubInfo `json:"hubs"`
}

// GraphStats is a JSON-friendly projection of analytics.Stats: totals plus
// deterministic breakdowns by edge kind and language.
type GraphStats struct {
	Files         int                   `json:"files"`
	Symbols       int                   `json:"symbols"`
	Edges         int                   `json:"edges"`
	RawSymbols    int                   `json:"raw_symbols"`
	RawEdges      int                   `json:"raw_edges"`
	Communities   int                   `json:"communities"`
	IsolatedNodes int                   `json:"isolated_nodes"`
	EdgeKinds     []analytics.KindCount `json:"edge_kinds"`
	Languages     []analytics.KindCount `json:"languages"`
}

// StatsInput selects the repo and the number of recent index telemetry rows to
// return. It is intentionally observation-oriented: graph totals plus the
// runtime/index metadata persisted with snapshots.
type StatsInput struct {
	RepoID string
	Limit  int
}

// IndexDelta compares a snapshot's counts with the prior snapshot in the
// returned history (newest row relative to the next older row).
type IndexDelta struct {
	Files   int `json:"files"`
	Symbols int `json:"symbols"`
	Edges   int `json:"edges"`
	Routes  int `json:"routes"`
}

// SnapshotTelemetry is the persisted observability record for an index run.
// DurationMS/TimingsMS are present for snapshots created by newer Atlas builds;
// older snapshots still expose counts/mode/created_at.
type SnapshotTelemetry struct {
	SnapshotID   string           `json:"snapshot_id"`
	CommitSHA    string           `json:"commit_sha"`
	CreatedAt    string           `json:"created_at"`
	Files        int              `json:"files"`
	Symbols      int              `json:"symbols"`
	Edges        int              `json:"edges"`
	Routes       int              `json:"routes"`
	Mode         string           `json:"mode,omitempty"`
	DurationMS   int64            `json:"duration_ms,omitempty"`
	ChangedFiles int              `json:"changed_files,omitempty"`
	TimingsMS    map[string]int64 `json:"timings_ms,omitempty"`
	Delta        *IndexDelta      `json:"delta,omitempty"`
}

// StatsResult combines telemetry and graph statistics for CLI/API/MCP. It is
// direct enough for dashboards but still compact enough for LLM tools.
type StatsResult struct {
	RepoID          string              `json:"repo_id"`
	RepoFullName    string              `json:"repo_full_name"`
	Tier            string              `json:"tier"`
	StorageDriver   string              `json:"storage_driver"`
	Latest          SnapshotTelemetry   `json:"latest"`
	Graph           GraphStats          `json:"graph"`
	CoverageFacts   int                 `json:"coverage_facts"`
	History         []SnapshotTelemetry `json:"history"`
	HistoryReturned int                 `json:"history_returned"`
}

// ReportInput selects the repo a graph report is rendered for.
type ReportInput struct {
	RepoID string
}

// ReportResult composes the snapshot's graph Stats with the top hubs and top
// communities and a deterministic GRAPH_REPORT.md-style Markdown rendering of
// the same data (so the report reads well in a terminal or a PR comment).
type ReportResult struct {
	Stats       GraphStats      `json:"stats"`
	Hubs        []HubInfo       `json:"hubs"`
	Communities []CommunityInfo `json:"communities"`
	Markdown    string          `json:"markdown"`
}

// These defaults bound the analytics ops to a sensible top-N when the caller
// doesn't pass a Limit. The report uses tighter caps than the standalone ops so
// its Markdown stays compact.
const (
	defaultCommunitiesLimit = 20
	defaultHubsLimit        = 20
	reportHubsLimit         = 10
	reportCommunitiesLimit  = 10
)

// loadAnalyticsGraph resolves the snapshot for repoID and builds the name-level
// analytics call graph from its symbols + edges. It mirrors how impact/explain
// load graph data: resolveSnapshot, then store.ListSymbols / store.ListEdges.
func (e *localEngine) loadAnalyticsGraph(ctx context.Context, repoID string) (*analytics.Graph, error) {
	snap, err := e.resolveSnapshot(ctx, repoID)
	if err != nil {
		return nil, err
	}
	syms, err := e.store.ListSymbols(ctx, snap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: analytics load symbols: %w", err)
	}
	edges, err := e.store.ListEdges(ctx, snap.ID)
	if err != nil {
		return nil, fmt.Errorf("engine: analytics load edges: %w", err)
	}
	// Hubs / communities / report describe the real ARCHITECTURE, so test files are
	// excluded by default — test scaffolding (e.g. every package's stub Close()) would
	// otherwise dominate the degree ranking with names that collapse across types.
	keptSyms := make([]graph.CodeSymbol, 0, len(syms))
	for _, s := range syms {
		if !isTestPath(s.Path) && isCodeLang(s.Language) {
			keptSyms = append(keptSyms, s)
		}
	}
	keptEdges := make([]graph.DependencyEdge, 0, len(edges))
	for _, ed := range edges {
		if !isTestPath(ed.FromFile) {
			keptEdges = append(keptEdges, ed)
		}
	}
	return analytics.Build(keptSyms, keptEdges), nil
}

// isTestPath reports whether a repo-relative path looks like a test file, across
// the supported languages, so the architecture-oriented analytics ignore test
// scaffolding. Heuristic but deterministic.
func isTestPath(p string) bool {
	p = strings.ToLower(p)
	base := p
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		base = p[i+1:]
	}
	switch {
	case strings.HasSuffix(base, "_test.go"): // Go
		return true
	case strings.HasSuffix(base, "_test.py"), strings.HasPrefix(base, "test_"): // Python
		return true
	case strings.HasSuffix(base, ".test.js"), strings.HasSuffix(base, ".spec.js"),
		strings.HasSuffix(base, ".test.jsx"), strings.HasSuffix(base, ".spec.jsx"),
		strings.HasSuffix(base, ".test.ts"), strings.HasSuffix(base, ".spec.ts"),
		strings.HasSuffix(base, ".test.tsx"), strings.HasSuffix(base, ".spec.tsx"): // JS/TS
		return true
	case strings.HasSuffix(base, "test.java"), strings.HasSuffix(base, "tests.java"): // Java
		return true
	}
	return strings.Contains(p, "/test/") || strings.Contains(p, "/tests/") || strings.Contains(p, "/__tests__/")
}

// isCodeLang reports whether a symbol's language participates in the call graph.
// Doc/config/data "symbols" (markdown headings, JSON/YAML keys, etc.) have no
// call edges and would otherwise flood the partition with isolated singletons, so
// the call-graph analytics (hubs/communities/report) consider code symbols only.
func isCodeLang(lang string) bool {
	switch strings.ToLower(lang) {
	case "markdown", "mdx", "yaml", "yml", "json", "toml", "xml", "plist",
		"config", "text", "csv", "gomod", "gosum", "dockerfile", "makefile",
		"html", "css", "ini", "properties", "sql", "":
		return false
	default:
		return true
	}
}

// Communities returns the snapshot's symbol-name communities (size-ranked),
// capped at Limit (default defaultCommunitiesLimit). Total reports how many
// communities were detected before the cap.
func (e *localEngine) Communities(ctx context.Context, in CommunitiesInput) (*CommunitiesResult, error) {
	g, err := e.loadAnalyticsGraph(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	all := g.Communities()
	limit := in.Limit
	if limit <= 0 {
		limit = defaultCommunitiesLimit
	}
	return &CommunitiesResult{
		Communities: communityInfos(all, limit),
		Total:       len(all),
	}, nil
}

// Hubs returns the top hubs ("god nodes") by total degree, descending (ties by
// name), capped at Limit (default defaultHubsLimit).
func (e *localEngine) Hubs(ctx context.Context, in HubsInput) (*HubsResult, error) {
	g, err := e.loadAnalyticsGraph(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultHubsLimit
	}
	return &HubsResult{Hubs: hubInfos(g.Hubs(limit))}, nil
}

// Report composes the snapshot's graph Stats, top hubs, and top communities,
// and renders a deterministic GRAPH_REPORT.md-style Markdown summary of them.
func (e *localEngine) Report(ctx context.Context, in ReportInput) (*ReportResult, error) {
	g, err := e.loadAnalyticsGraph(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	stats := graphStats(g.Stats())
	hubs := hubInfos(g.Hubs(reportHubsLimit))
	communities := communityInfos(g.Communities(), reportCommunitiesLimit)
	return &ReportResult{
		Stats:       stats,
		Hubs:        hubs,
		Communities: communities,
		Markdown:    renderGraphReport(stats, hubs, communities),
	}, nil
}

const defaultStatsHistoryLimit = 20

func (e *localEngine) Stats(ctx context.Context, in StatsInput) (*StatsResult, error) {
	repo, err := e.resolveRepo(ctx, in.RepoID)
	if err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultStatsHistoryLimit
	}
	if limit > 500 {
		limit = 500
	}
	snaps, err := e.store.ListSnapshots(ctx, repo.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("engine: stats history: %w", err)
	}
	if len(snaps) == 0 {
		return nil, ErrNoIndex
	}
	g, err := e.loadAnalyticsGraph(ctx, repo.ID)
	if err != nil {
		return nil, err
	}
	coverageFacts, err := e.store.ListCoverage(ctx, snaps[0].ID, "")
	if err != nil {
		return nil, fmt.Errorf("engine: stats coverage: %w", err)
	}
	history := make([]SnapshotTelemetry, 0, len(snaps))
	for i := range snaps {
		var prev *graph.Snapshot
		if i+1 < len(snaps) {
			prev = &snaps[i+1]
		}
		history = append(history, snapshotTelemetry(snaps[i], prev))
	}
	return &StatsResult{
		RepoID:          repo.ID,
		RepoFullName:    repo.FullName,
		Tier:            e.cfg.Tier,
		StorageDriver:   e.store.Dialect(),
		Latest:          history[0],
		Graph:           graphStats(g.Stats()),
		CoverageFacts:   len(coverageFacts),
		History:         history,
		HistoryReturned: len(history),
	}, nil
}

// ── projections ─────────────────────────────────────────────────────────────

func communityInfos(cs []analytics.Community, limit int) []CommunityInfo {
	if limit > 0 && limit < len(cs) {
		cs = cs[:limit]
	}
	out := make([]CommunityInfo, 0, len(cs))
	for _, c := range cs {
		out = append(out, CommunityInfo{
			ID:              c.ID,
			Size:            c.Size,
			Members:         append([]string(nil), c.Members...),
			Representatives: append([]string(nil), c.Representatives...),
		})
	}
	return out
}

func hubInfos(hs []analytics.Hub) []HubInfo {
	out := make([]HubInfo, 0, len(hs))
	for _, h := range hs {
		out = append(out, HubInfo{
			Name:        h.Name,
			BareName:    h.BareName,
			Path:        h.Path,
			Kind:        h.Kind,
			Language:    h.Language,
			InDegree:    h.InDegree,
			OutDegree:   h.OutDegree,
			TotalDegree: h.TotalDegree,
		})
	}
	return out
}

func graphStats(s analytics.Stats) GraphStats {
	return GraphStats{
		Files:         s.Files,
		Symbols:       s.Symbols,
		Edges:         s.Edges,
		RawSymbols:    s.RawSymbols,
		RawEdges:      s.RawEdges,
		Communities:   s.Communities,
		IsolatedNodes: s.IsolatedNodes,
		EdgeKinds:     s.EdgeKindsSorted(),
		Languages:     s.LanguagesSorted(),
	}
}

func snapshotTelemetry(s graph.Snapshot, prev *graph.Snapshot) SnapshotTelemetry {
	mode := metaStr(s.Metadata, "last_index_mode")
	if mode == "" {
		mode = metaStr(s.Metadata, "mode")
	}
	durationMS := metaInt64(s.Metadata, "last_index_duration_ms")
	if durationMS == 0 {
		durationMS = metaInt64(s.Metadata, "duration_ms")
	}
	changedFiles := metaInt64(s.Metadata, "last_index_changed_files")
	if changedFiles == 0 {
		changedFiles = metaInt64(s.Metadata, "changed_files")
	}
	timings := metaTimings(s.Metadata, "last_index_timings_ms")
	if len(timings) == 0 {
		timings = metaTimings(s.Metadata, "timings_ms")
	}
	out := SnapshotTelemetry{
		SnapshotID:   s.ID,
		CommitSHA:    s.CommitSHA,
		CreatedAt:    s.CreatedAt.Format(time.RFC3339),
		Files:        s.FileCount,
		Symbols:      s.SymbolCount,
		Edges:        s.EdgeCount,
		Routes:       s.RouteCount,
		Mode:         mode,
		DurationMS:   durationMS,
		ChangedFiles: int(changedFiles),
		TimingsMS:    timings,
	}
	if prev != nil {
		out.Delta = &IndexDelta{
			Files:   s.FileCount - prev.FileCount,
			Symbols: s.SymbolCount - prev.SymbolCount,
			Edges:   s.EdgeCount - prev.EdgeCount,
			Routes:  s.RouteCount - prev.RouteCount,
		}
	}
	return out
}

func metaInt64(m graph.JSONBMap, key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

func metaTimings(m graph.JSONBMap, key string) map[string]int64 {
	if m == nil {
		return nil
	}
	raw, ok := m[key]
	if !ok {
		return nil
	}
	out := map[string]int64{}
	switch v := raw.(type) {
	case map[string]int64:
		for k, n := range v {
			out[k] = n
		}
	case map[string]any:
		for k, n := range v {
			out[k] = valueInt64(n)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func valueInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

// ── Markdown rendering ──────────────────────────────────────────────────────

// renderGraphReport produces a deterministic GRAPH_REPORT.md-style document:
// title, a summary-stats section, a "Top hubs (god nodes)" table, and a
// "Communities" list with sizes and representative members. The input is
// already deterministically ordered, so the output is stable across runs.
func renderGraphReport(stats GraphStats, hubs []HubInfo, communities []CommunityInfo) string {
	var b strings.Builder
	b.WriteString("# Graph report\n\n")

	// Summary stats.
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "- Files: %d\n", stats.Files)
	fmt.Fprintf(&b, "- Symbols (nodes): %d\n", stats.Symbols)
	fmt.Fprintf(&b, "- Call edges: %d\n", stats.Edges)
	// Communities counts every partition; subtract the isolated singletons so the
	// headline reflects the real (size > 1) clusters, with isolates reported below.
	nonTrivial := stats.Communities - stats.IsolatedNodes
	if nonTrivial < 0 {
		nonTrivial = 0
	}
	fmt.Fprintf(&b, "- Communities (size > 1): %d\n", nonTrivial)
	fmt.Fprintf(&b, "- Isolated nodes: %d\n", stats.IsolatedNodes)
	if len(stats.Languages) > 0 {
		fmt.Fprintf(&b, "- Languages: %s\n", joinKindCounts(stats.Languages))
	}
	if len(stats.EdgeKinds) > 0 {
		fmt.Fprintf(&b, "- Edge kinds: %s\n", joinKindCounts(stats.EdgeKinds))
	}
	b.WriteString("\n")

	// Top hubs (god nodes).
	b.WriteString("## Top hubs (god nodes)\n\n")
	if len(hubs) == 0 {
		b.WriteString("_No hubs detected._\n\n")
	} else {
		b.WriteString("| Symbol | Kind | In | Out | Total | Location |\n")
		b.WriteString("| --- | --- | --: | --: | --: | --- |\n")
		for _, h := range hubs {
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %s |\n",
				mdCell(h.Name), mdCell(h.Kind), h.InDegree, h.OutDegree, h.TotalDegree, mdCell(h.Path))
		}
		b.WriteString("\n")
	}

	// Communities.
	b.WriteString("## Communities\n\n")
	if len(communities) == 0 {
		b.WriteString("_No communities detected._\n")
	} else {
		for _, c := range communities {
			reps := c.Representatives
			if len(reps) == 0 {
				reps = c.Members
			}
			fmt.Fprintf(&b, "- Community %d (%d symbols): %s\n", c.ID, c.Size, strings.Join(reps, ", "))
		}
	}
	return b.String()
}

// joinKindCounts renders deterministically-ordered KindCounts as "key:count"
// pairs joined by spaces (the slice is already sorted by the analytics package).
func joinKindCounts(kc []analytics.KindCount) string {
	parts := make([]string, 0, len(kc))
	for _, c := range kc {
		parts = append(parts, fmt.Sprintf("%s:%d", c.Key, c.Count))
	}
	return strings.Join(parts, " ")
}

// mdCell escapes the pipe character so a value can't break a Markdown table row;
// empty values render as "-" so a column never collapses.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	if s == "" {
		return "-"
	}
	return s
}
