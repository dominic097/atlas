package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/dominic097/atlas/internal/analytics"
	"github.com/dominic097/atlas/internal/engine"
	"github.com/dominic097/atlas/internal/query"
)

// renderJSON is the single entry point every command calls. It now DELEGATES to
// the format-aware render() so the global --format flag is honored everywhere
// with zero per-command edits. The name/signature are unchanged on purpose.
func renderJSON(w io.Writer, v any) error {
	return render(w, v)
}

// render writes v in the format selected by the global --format flag:
//
//	"" | "json"      pretty indented JSON (the historical default; UNCHANGED)
//	"compact"        minified single-line JSON (no indentation)
//	"plain"|"terse"  dense human/agent text — the token win (graphify-like)
//	"ndjson"         one JSON object per line over a result's primary list
//
// Unknown result types in the terse/ndjson paths fall back to compact JSON; they
// never error. --json (gf.json) is the documented shorthand for --format json.
func render(w io.Writer, v any) error {
	format := strings.ToLower(strings.TrimSpace(gf.format))
	if format == "" && gf.json {
		format = "json"
	}
	switch format {
	case "compact":
		return renderCompact(w, v)
	case "plain", "terse", "text":
		return renderTerse(w, v)
	case "ndjson":
		return renderNDJSON(w, v)
	case "", "json":
		return renderPretty(w, v)
	default:
		// Unknown format string: keep the stable, scriptable default rather than
		// erroring on a typo.
		return renderPretty(w, v)
	}
}

// renderPretty is the historical default: indented JSON.
func renderPretty(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// renderCompact emits minified single-line JSON (no indent). json.Encoder always
// appends a trailing newline, which keeps the output line-oriented.
func renderCompact(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

// renderNDJSON emits one JSON object per line for a result that carries a primary
// list (search hits, callers, references, …). Results without an obvious primary
// list fall back to a single compact line.
func renderNDJSON(w io.Writer, v any) error {
	if items, ok := primaryList(v); ok {
		enc := json.NewEncoder(w)
		for _, it := range items {
			if err := enc.Encode(it); err != nil {
				return err
			}
		}
		return nil
	}
	return renderCompact(w, v)
}

// renderTerse emits a dense, greppable text block per result type. Lists are
// DISPLAY-capped (see listCap) so hub symbols with hundreds of callers stay
// small. Unknown types fall back to compact JSON — never an error.
func renderTerse(w io.Writer, v any) error {
	if s := terseString(v); s != "" {
		_, err := io.WriteString(w, s)
		return err
	}
	return renderCompact(w, v)
}

// listCap is how many list items the terse formatters print before collapsing
// the remainder into "(+N more)". Kept small to match graphify's density.
const listCap = 12

// ── terse formatting helpers ────────────────────────────────────────────────

// terseLines accumulates output lines for a single result block.
type terseLines struct {
	b strings.Builder
}

func (t *terseLines) line(s string)            { t.b.WriteString(s); t.b.WriteByte('\n') }
func (t *terseLines) linef(f string, a ...any) { t.line(fmt.Sprintf(f, a...)) }
func (t *terseLines) String() string           { return t.b.String() }

// capList formats up to listCap names joined by ", " then "(+N more)".
func capList(names []string) string {
	return capListN(names, listCap)
}

func capListN(names []string, n int) string {
	if len(names) == 0 {
		return ""
	}
	if n <= 0 {
		n = listCap
	}
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	head := strings.Join(names[:n], ", ")
	return fmt.Sprintf("%s (+%d more)", head, len(names)-n)
}

func compactList1(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	return fmt.Sprintf("%s+%d", names[0], len(names)-1)
}

// refNames projects symbol refs to their display names.
func refNames(refs []engine.SymbolRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}

// loc renders a "path:line" or "path:start-end" location, omitting empty parts.
func loc(path string, line, end int) string {
	switch {
	case path == "":
		return ""
	case end > line:
		return fmt.Sprintf("%s:%d-%d", path, line, end)
	case line > 0:
		return fmt.Sprintf("%s:%d", path, line)
	default:
		return path
	}
}

// terseString renders v as a terse block, or "" when v's type is not handled
// (the caller then falls back to compact JSON). The switch matches the concrete
// engine result POINTER types the commands hand to renderJSON.
func terseString(v any) string {
	switch r := v.(type) {
	case *engine.ExplainResult:
		return terseExplain(r)
	case *engine.CallersResult:
		return terseCallers(r)
	case *engine.SymbolResult:
		return terseSymbol(r)
	case *engine.NeighborsResult:
		return terseNeighbors(r)
	case *engine.PathResult:
		return tersePath(r)
	case *engine.RefsResult:
		return terseRefs(r)
	case *engine.SearchResult:
		return terseSearch(r)
	case *engine.SemanticSearchResult:
		return terseSemanticSearch(r)
	case *engine.ContextResult:
		return terseContext(r)
	case *engine.ImpactResult:
		return terseImpact(r)
	case *engine.ConsumersResult:
		return terseConsumers(r)
	case *engine.RouteContractsResult:
		return terseRouteContracts(r)
	case *engine.CrossRepoImpactResult:
		return terseCrossRepoImpact(r)
	case *engine.CommunitiesResult:
		return terseCommunities(r)
	case *engine.HubsResult:
		return terseHubs(r)
	case *engine.ReportResult:
		return terseReport(r)
	case *engine.StatsResult:
		return terseStats(r)
	case *engine.StatusResult:
		return terseStatus(r)
	case *engine.HistoryResult:
		return terseHistory(r)
	case *engine.SnapshotDiffResult:
		return terseSnapshotDiff(r)
	case *engine.CoverageResult:
		return terseCoverage(r)
	case *engine.LinkResult:
		return terseLink(r)
	case *engine.IndexResult:
		return terseIndex(r)
	default:
		return ""
	}
}

func terseExplain(r *engine.ExplainResult) string {
	kind, location := "", ""
	if len(r.Definitions) > 0 {
		d := r.Definitions[0]
		kind = kindCode(d.Kind)
		location = compactLoc(d.Path, d.Line)
	}
	parts := []string{r.Symbol}
	if kind != "" || location != "" {
		parts = append(parts, strings.TrimRight(fmt.Sprintf("%s@%s", kind, location), "@"))
	}
	if len(r.Definitions) > 1 {
		parts = append(parts, fmt.Sprintf("df%d", len(r.Definitions)))
	}
	callerCount := len(r.Callers)
	if r.CallerCount > callerCount {
		callerCount = r.CallerCount
	}
	calleeCount := len(r.Callees)
	if r.CalleeCount > calleeCount {
		calleeCount = r.CalleeCount
	}
	if callerCount > 0 {
		parts = append(parts, fmt.Sprintf("c%d", callerCount))
	}
	if calleeCount > 0 {
		parts = append(parts, fmt.Sprintf("d%d", calleeCount))
	}
	if len(r.ServedRoutes) > 0 {
		parts = append(parts, fmt.Sprintf("r%d", len(r.ServedRoutes)))
	}
	if len(r.CrossRepoConsumers) > 0 {
		parts = append(parts, fmt.Sprintf("x%d", len(r.CrossRepoConsumers)))
	}
	return strings.Join(parts, " ") + "\n"
}

func compactLoc(file string, line int) string {
	file = strings.TrimSpace(file)
	if file == "" {
		return ""
	}
	base := path.Base(strings.ReplaceAll(file, "\\", "/"))
	if suffix := compactLocSuffix(base); suffix != "" {
		base = base[:len(base)-len(suffix)]
	} else if strings.EqualFold(path.Ext(base), ".sql") {
		base = strings.TrimSuffix(base, path.Ext(base))
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", base, line)
	}
	return base
}

func compactLocSuffix(base string) string {
	lower := strings.ToLower(base)
	for _, suffix := range []string{
		".blade.php",
		".csproj", ".fsproj", ".vbproj", ".slnx",
		".ps1", ".psm1", ".psd1",
		".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx",
		".cu", ".cuh",
		".razor", ".cshtml", ".vue",
		".v", ".sv", ".svh",
		".go", ".py", ".java",
		".cpp", ".cc", ".cxx", ".hpp", ".hxx", ".hh", ".c", ".h",
		".rs", ".rb", ".kt", ".kts", ".scala", ".php", ".swift",
		".lua", ".luau", ".zig", ".ex", ".exs", ".m", ".mm", ".jl",
		".f90", ".f95", ".f03", ".f08", ".f",
		".dart", ".pas", ".pp", ".dpr", ".dpk", ".lpr", ".inc",
		".tf", ".tfvars", ".hcl", ".dm", ".dme", ".dmi", ".dmm", ".dmf",
	} {
		if strings.HasSuffix(lower, suffix) {
			return suffix
		}
	}
	return ""
}

func kindCode(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "function", "func":
		return "f"
	case "method":
		return "m"
	case "class":
		return "c"
	case "interface":
		return "i"
	case "type", "struct", "record":
		return "t"
	case "variable", "var":
		return "v"
	case "constant", "const":
		return "k"
	case "field", "property":
		return "p"
	case "constructor":
		return "ctor"
	case "enum":
		return "e"
	default:
		if kind = strings.ToLower(strings.TrimSpace(kind)); kind != "" {
			return kind[:1]
		}
		return ""
	}
}

func terseCallers(r *engine.CallersResult) string {
	var t terseLines
	t.linef("callers %s  total %d", r.Symbol, r.Total)
	writeRefs(&t, r.Callers)
	return t.String()
}

func terseSymbol(r *engine.SymbolResult) string {
	var t terseLines
	t.linef("symbol %s  matches %d", r.Query, len(r.Matches))
	for _, m := range r.Matches {
		t.line(strings.TrimRight(fmt.Sprintf("  %s  %s  %s", m.Name, m.Kind, loc(m.Path, m.Line, m.EndLine)), " "))
		if m.Signature != "" {
			t.linef("    sig  %s", m.Signature)
		}
		if len(m.Callers) > 0 {
			t.linef("    callers(%d)  %s", len(m.Callers), capList(refNames(m.Callers)))
		}
		if len(m.Callees) > 0 {
			t.linef("    callees(%d)  %s", len(m.Callees), capList(refNames(m.Callees)))
		}
	}
	return t.String()
}

func terseNeighbors(r *engine.NeighborsResult) string {
	var t terseLines
	t.linef("neighbors %s  callers %d  callees %d", r.Symbol, len(r.Callers), len(r.Callees))
	if len(r.Callers) > 0 {
		t.linef("  callers(%d)  %s", len(r.Callers), capList(refNames(r.Callers)))
	}
	if len(r.Callees) > 0 {
		t.linef("  callees(%d)  %s", len(r.Callees), capList(refNames(r.Callees)))
	}
	return t.String()
}

func tersePath(r *engine.PathResult) string {
	var t terseLines
	if !r.Found {
		t.linef("path %s -> %s  NOT FOUND", r.From, r.To)
		return t.String()
	}
	t.linef("path %s -> %s  len %d", r.From, r.To, r.Length)
	names := refNames(r.Steps)
	if len(names) > 0 {
		t.linef("  steps  %s", strings.Join(names, " -> "))
	}
	return t.String()
}

func terseRefs(r *engine.RefsResult) string {
	var t terseLines
	t.linef("refs %s  total %d", r.Symbol, r.Total)
	writeRefs(&t, r.References)
	return t.String()
}

func terseSearch(r *engine.SearchResult) string {
	var t terseLines
	t.linef("search  mode %s  total %d", r.ModeUsed, r.Total)
	writeHits(&t, r.Results)
	return t.String()
}

func terseSemanticSearch(r *engine.SemanticSearchResult) string {
	var t terseLines
	t.linef("semantic_search  mode %s  degraded %t  results %d", r.ModeUsed, r.Degraded, len(r.Results))
	writeHits(&t, r.Results)
	return t.String()
}

func terseContext(r *engine.ContextResult) string {
	var t terseLines
	t.linef("context  mode %s  files %d  symbols %d  edges %d  hits %d  impacted %d",
		r.Mode, len(r.Files), len(r.Symbols), len(r.Edges), len(r.SearchHits), len(r.ImpactedFiles))
	if len(r.Files) > 0 {
		names := make([]string, 0, len(r.Files))
		for _, f := range r.Files {
			names = append(names, f.Path)
		}
		t.linef("  files(%d)  %s", len(names), capList(names))
	}
	if len(r.Symbols) > 0 {
		names := make([]string, 0, len(r.Symbols))
		for _, s := range r.Symbols {
			names = append(names, s.Name)
		}
		t.linef("  symbols(%d)  %s", len(names), capList(names))
	}
	if len(r.ImpactedFiles) > 0 {
		names := make([]string, 0, len(r.ImpactedFiles))
		for _, f := range r.ImpactedFiles {
			names = append(names, f.Path)
		}
		t.linef("  impacted(%d)  %s", len(names), capList(names))
	}
	return t.String()
}

func terseImpact(r *engine.ImpactResult) string {
	var t terseLines
	t.linef("impact  symbols %d  files %d  tests %d  depth %d",
		len(r.ImpactedSymbols), len(r.ImpactedFiles), len(r.ImpactedTests), r.DepthReached)
	if len(r.ImpactedSymbols) > 0 {
		t.linef("  symbols(%d)  %s", len(r.ImpactedSymbols), capList(r.ImpactedSymbols))
	}
	if len(r.ImpactedFiles) > 0 {
		names := make([]string, 0, len(r.ImpactedFiles))
		for _, f := range r.ImpactedFiles {
			names = append(names, f.Path)
		}
		t.linef("  files(%d)  %s", len(names), capList(names))
	}
	if len(r.ImpactedTests) > 0 {
		t.linef("  tests(%d)  %s", len(r.ImpactedTests), capList(r.ImpactedTests))
	}
	return t.String()
}

func terseConsumers(r *engine.ConsumersResult) string {
	var t terseLines
	t.linef("consumers %s  impacted %d  repos %d", r.Repo, len(r.Impacted), len(r.ConsumerRepos))
	if len(r.ConsumerRepos) > 0 {
		t.linef("  repos(%d)  %s", len(r.ConsumerRepos), capList(r.ConsumerRepos))
	}
	writeConsumerHits(&t, r.Impacted)
	return t.String()
}

func terseRouteContracts(r *engine.RouteContractsResult) string {
	var t terseLines
	t.linef("route_contracts %s  total %d", r.Repo, r.Total)
	cap := r.Routes
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, rt := range cap {
		t.line(strings.TrimRight(fmt.Sprintf("  %s %s  %s", rt.Method, rt.PathPattern, rt.HandlerSymbol), " "))
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
	return t.String()
}

func terseCrossRepoImpact(r *engine.CrossRepoImpactResult) string {
	var t terseLines
	t.linef("cross_repo_impact %s  served %d  impacted %d  repos %d",
		r.Repo, len(r.ServedRoutes), len(r.Impacted), len(r.ConsumerRepos))
	if len(r.ConsumerRepos) > 0 {
		t.linef("  repos(%d)  %s", len(r.ConsumerRepos), capList(r.ConsumerRepos))
	}
	writeConsumerHits(&t, r.Impacted)
	return t.String()
}

func terseCommunities(r *engine.CommunitiesResult) string {
	var t terseLines
	t.linef("communities  total %d  shown %d", r.Total, len(r.Communities))
	cap := r.Communities
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, c := range cap {
		members := c.Representatives
		if len(members) == 0 {
			members = c.Members
		}
		t.linef("  #%d  size %d  %s", c.ID, c.Size, capList(members))
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
	return t.String()
}

func terseHubs(r *engine.HubsResult) string {
	var t terseLines
	t.linef("hubs  total %d", len(r.Hubs))
	cap := r.Hubs
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, h := range cap {
		t.line(strings.TrimRight(fmt.Sprintf("  %s  %s  in %d  out %d  total %d  %s",
			h.Name, h.Kind, h.InDegree, h.OutDegree, h.TotalDegree, loc(h.Path, 0, 0)), " "))
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
	return t.String()
}

// terseReport prints the ready-rendered Markdown report verbatim — the natural
// human-facing output of `atlas report --format plain`.
func terseReport(r *engine.ReportResult) string {
	return r.Markdown
}

func terseStatus(r *engine.StatusResult) string {
	var t terseLines
	t.linef("status  tier %s  driver %s  vectors %s  repos %d",
		r.Tier, r.StorageDriver, r.VectorBackend, r.ReposIndexed)
	cap := r.Repos
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, repo := range cap {
		t.linef("  %s  symbols %d  edges %d  %s", repo.FullName, repo.Symbols, repo.Edges, repo.CommitSHA)
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
	return t.String()
}

func terseHistory(r *engine.HistoryResult) string {
	var t terseLines
	t.linef("history %s  snapshots %d", r.FullName, len(r.Snapshots))
	cap := r.Snapshots
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, s := range cap {
		t.linef("  %s  %s  files %d  symbols %d  edges %d  %s",
			shortSHA(s.CommitSHA), s.SnapshotID, s.Files, s.Symbols, s.Edges, s.CreatedAt)
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
	return t.String()
}

func terseSnapshotDiff(r *engine.SnapshotDiffResult) string {
	var t terseLines
	t.linef("snapshot_diff %s -> %s  +%d -%d ~%d  files %d",
		shortSHA(r.FromCommit), shortSHA(r.ToCommit),
		r.AddedCount, r.RemovedCount, r.ModifiedCount, len(r.ChangedFiles))
	writeChanges(&t, "added", r.Added)
	writeChanges(&t, "removed", r.Removed)
	writeChanges(&t, "modified", r.Modified)
	if len(r.ChangedFiles) > 0 {
		t.linef("  files(%d)  %s", len(r.ChangedFiles), capList(r.ChangedFiles))
	}
	return t.String()
}

func terseCoverage(r *engine.CoverageResult) string {
	var t terseLines
	strength := r.Strength
	if strength != "" {
		strength = "  " + strength
	}
	t.line(strings.TrimRight(fmt.Sprintf("coverage %s  mode %s  covered %t%s  dir %s",
		r.Target, r.Mode, r.Covered, strength, r.Direction), " "))
	if len(r.Tests) > 0 {
		t.linef("  tests(%d)  %s", len(r.Tests), capList(refNames(r.Tests)))
	}
	if len(r.Symbols) > 0 {
		t.linef("  symbols(%d)  %s", len(r.Symbols), capList(refNames(r.Symbols)))
	}
	return t.String()
}

func terseLink(r *engine.LinkResult) string {
	var t terseLines
	t.linef("link %s  branch %s  created %t  indexed %t",
		r.FullName, r.DefaultBranch, r.Created, r.Indexed)
	if r.Root != "" {
		t.linef("  root  %s", r.Root)
	}
	return t.String()
}

func terseIndex(r *engine.IndexResult) string {
	var t terseLines
	t.linef("index %s  %s  mode %s  %dms", r.RepoFullName, shortSHA(r.CommitSHA), r.Mode, r.DurationMS)
	t.linef("  files %d  symbols %d  edges %d  routes %d", r.IndexedFiles, r.Symbols, r.Edges, r.Routes)
	if len(r.Languages) > 0 {
		t.linef("  langs  %s", joinCounts(r.Languages))
	}
	return t.String()
}

func terseStats(r *engine.StatsResult) string {
	var t terseLines
	t.linef("stats %s  %s/%s  history %d  coverage_facts %d",
		r.RepoFullName, r.Tier, r.StorageDriver, r.HistoryReturned, r.CoverageFacts)
	t.linef("  latest %s  mode %s  files %d  symbols %d  edges %d  routes %d  %dms",
		shortSHA(r.Latest.CommitSHA), r.Latest.Mode, r.Latest.Files, r.Latest.Symbols, r.Latest.Edges, r.Latest.Routes, r.Latest.DurationMS)
	t.linef("  graph files %d  symbols %d  edges %d  communities %d  isolated %d",
		r.Graph.Files, r.Graph.Symbols, r.Graph.Edges, r.Graph.Communities, r.Graph.IsolatedNodes)
	if len(r.Graph.Languages) > 0 {
		t.linef("  langs  %s", joinKindCountPairs(r.Graph.Languages))
	}
	if len(r.Latest.TimingsMS) > 0 {
		t.linef("  timings  %s", joinInt64Counts(r.Latest.TimingsMS))
	}
	return t.String()
}

// ── shared writers ──────────────────────────────────────────────────────────

func writeRefs(t *terseLines, refs []engine.SymbolRef) {
	cap := refs
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, r := range cap {
		t.line(strings.TrimRight(fmt.Sprintf("  %s  %s  %s", r.Name, r.Kind, loc(r.Path, r.Line, 0)), " "))
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
}

func writeHits(t *terseLines, hits []engine.SearchHit) {
	cap := hits
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, h := range cap {
		t.line(strings.TrimRight(fmt.Sprintf("  %s  %s  %s", h.Name, h.Kind, loc(h.Path, h.Line, 0)), " "))
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
}

func writeConsumerHits(t *terseLines, hits []engine.ConsumerHit) {
	cap := hits
	extra := 0
	if len(cap) > listCap {
		extra = len(cap) - listCap
		cap = cap[:listCap]
	}
	for _, h := range cap {
		t.line(strings.TrimRight(fmt.Sprintf("  %s  %s  %s", h.Repo, h.Endpoint, h.CallingFile), " "))
	}
	if extra > 0 {
		t.linef("  (+%d more)", extra)
	}
}

func writeChanges(t *terseLines, label string, changes []query.SymbolChange) {
	if len(changes) == 0 {
		return
	}
	names := make([]string, 0, len(changes))
	for _, c := range changes {
		names = append(names, c.Name)
	}
	t.linef("  %s(%d)  %s", label, len(names), capList(names))
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func joinCounts(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s:%d", k, v))
	}
	// deterministic-ish: small maps, simple sort by string
	for i := 0; i < len(parts); i++ {
		for j := i + 1; j < len(parts); j++ {
			if parts[j] < parts[i] {
				parts[i], parts[j] = parts[j], parts[i]
			}
		}
	}
	return strings.Join(parts, " ")
}

func joinInt64Counts(m map[string]int64) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s:%dms", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func joinKindCountPairs(kc []analytics.KindCount) string {
	parts := make([]string, 0, len(kc))
	for _, c := range kc {
		parts = append(parts, fmt.Sprintf("%s:%d", c.Key, c.Count))
	}
	return strings.Join(parts, " ")
}

// ── ndjson primary-list extraction ──────────────────────────────────────────

// primaryList returns the result's principal list (one JSON object per line in
// ndjson mode), or ok=false when the result has no obvious primary list.
func primaryList(v any) ([]any, bool) {
	conv := func(n int, at func(i int) any) ([]any, bool) {
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, at(i))
		}
		return out, true
	}
	switch r := v.(type) {
	case *engine.SearchResult:
		return conv(len(r.Results), func(i int) any { return r.Results[i] })
	case *engine.SemanticSearchResult:
		return conv(len(r.Results), func(i int) any { return r.Results[i] })
	case *engine.CallersResult:
		return conv(len(r.Callers), func(i int) any { return r.Callers[i] })
	case *engine.RefsResult:
		return conv(len(r.References), func(i int) any { return r.References[i] })
	case *engine.SymbolResult:
		return conv(len(r.Matches), func(i int) any { return r.Matches[i] })
	case *engine.ImpactResult:
		return conv(len(r.ImpactedFiles), func(i int) any { return r.ImpactedFiles[i] })
	case *engine.RouteContractsResult:
		return conv(len(r.Routes), func(i int) any { return r.Routes[i] })
	case *engine.ConsumersResult:
		return conv(len(r.Impacted), func(i int) any { return r.Impacted[i] })
	case *engine.CrossRepoImpactResult:
		return conv(len(r.Impacted), func(i int) any { return r.Impacted[i] })
	case *engine.HistoryResult:
		return conv(len(r.Snapshots), func(i int) any { return r.Snapshots[i] })
	case *engine.StatusResult:
		return conv(len(r.Repos), func(i int) any { return r.Repos[i] })
	default:
		return nil, false
	}
}
