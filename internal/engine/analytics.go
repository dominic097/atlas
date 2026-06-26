package engine

import (
	"context"
	"fmt"
	"strings"

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
