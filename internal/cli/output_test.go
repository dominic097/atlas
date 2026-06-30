package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dominic097/atlas/internal/engine"
)

// withFormat sets the global format flag for the duration of fn and restores it,
// so tests don't leak state into each other (gf is a package var).
func withFormat(format string, jsonShort bool, fn func()) {
	prevFormat, prevJSON := gf.format, gf.json
	gf.format, gf.json = format, jsonShort
	defer func() { gf.format, gf.json = prevFormat, prevJSON }()
	fn()
}

// hubExplain builds an ExplainResult with many callers/callees so the terse
// formatter's display caps and the size comparison are meaningful.
func hubExplain() *engine.ExplainResult {
	callers := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		callers = append(callers, "callerSymbolNumber"+string(rune('A'+i%26)))
	}
	callees := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		callees = append(callees, "calleeSymbolNumber"+string(rune('A'+i%26)))
	}
	return &engine.ExplainResult{
		Symbol: "NewRootCmd",
		Definitions: []engine.ExplainDef{{
			SymbolID:  "sym-1",
			Kind:      "function",
			Path:      "internal/cli/root.go",
			Line:      52,
			EndLine:   97,
			Signature: "func NewRootCmd() *cobra.Command",
			Doc:       "NewRootCmd builds the full command tree.",
		}},
		Callers: callers,
		Callees: callees,
		Imports: []string{"context", "fmt", "cobra", "atlas"},
	}
}

func renderTo(t *testing.T, format string, v any) string {
	t.Helper()
	var buf bytes.Buffer
	withFormat(format, false, func() {
		if err := render(&buf, v); err != nil {
			t.Fatalf("render(%q) error: %v", format, err)
		}
	})
	return buf.String()
}

func TestRenderCompactIsMinifiedValidJSON(t *testing.T) {
	out := renderTo(t, "compact", hubExplain())
	if strings.Contains(out, "\n  ") {
		t.Fatalf("compact output should not be indented; got:\n%s", out[:min(120, len(out))])
	}
	// Trailing newline from json.Encoder is fine; the body must be valid JSON.
	var got engine.ExplainResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("compact output is not valid JSON: %v", err)
	}
	if got.Symbol != "NewRootCmd" {
		t.Fatalf("compact JSON lost the symbol; got %q", got.Symbol)
	}
}

func TestRenderTerseIsDenseAndShorterThanPretty(t *testing.T) {
	r := hubExplain()
	terse := renderTo(t, "plain", r)
	pretty := renderTo(t, "", r) // "" == pretty default
	terseAlias := renderTo(t, "terse", r)

	if terse != terseAlias {
		t.Fatalf("plain and terse should render identically")
	}
	if !strings.Contains(terse, "NewRootCmd") {
		t.Fatalf("terse output missing the symbol name:\n%s", terse)
	}
	if strings.Contains(terse, "func NewRootCmd() *cobra.Command") {
		t.Fatalf("terse explain should omit signatures; JSON retains them:\n%s", terse)
	}
	if !strings.Contains(terse, "f@root:52") {
		t.Fatalf("terse output missing compact kind/location:\n%s", terse)
	}
	if !strings.Contains(terse, "c200") || !strings.Contains(terse, "d40") {
		t.Fatalf("terse output missing compact caller/callee counts:\n%s", terse)
	}
	if strings.Contains(terse, "callerSymbolNumber") || strings.Contains(terse, "calleeSymbolNumber") {
		t.Fatalf("terse explain should keep hub lists count-only:\n%s", terse)
	}
	// The whole point: materially shorter than the pretty JSON for a hub symbol.
	if len(terse) >= len(pretty) {
		t.Fatalf("terse (%d bytes) should be much shorter than pretty (%d bytes)", len(terse), len(pretty))
	}
	if len(terse)*2 >= len(pretty) {
		t.Fatalf("terse (%d bytes) should be at least ~2x smaller than pretty (%d bytes)", len(terse), len(pretty))
	}
	if len(terse)*5 >= len(pretty) {
		t.Fatalf("terse (%d bytes) should be at least ~5x smaller than pretty (%d bytes)", len(terse), len(pretty))
	}
}

func TestRenderTerseUsesExplainCountFields(t *testing.T) {
	r := &engine.ExplainResult{
		Symbol:          "Functor",
		Definitions:     []engine.ExplainDef{{Kind: "trait", Path: "cats/Functor.scala", Line: 31}},
		DefinitionCount: 2,
		CallerCount:     66,
		CalleeCount:     3,
	}
	terse := renderTo(t, "plain", r)
	if !strings.Contains(terse, "df2") || !strings.Contains(terse, "c66") || !strings.Contains(terse, "d3") {
		t.Fatalf("terse explain should render count-only caller/callee fields: %q", terse)
	}
	if strings.Contains(terse, "callers") || strings.Contains(terse, "callees") {
		t.Fatalf("terse explain should keep count-only output compact: %q", terse)
	}
}

func TestRenderTerseCompactsSelectedVerboseLocations(t *testing.T) {
	r := &engine.ExplainResult{
		Symbol: "hdb_catalog.hdb_metadata",
		Definitions: []engine.ExplainDef{{
			Kind: "table",
			Path: "migrations/42_to_43.sql",
			Line: 48,
		}},
	}
	terse := renderTo(t, "plain", r)
	if !strings.Contains(terse, "t@42_to_43:48") {
		t.Fatalf("terse SQL output should omit .sql suffix: %q", terse)
	}
	if strings.Contains(terse, ".sql") {
		t.Fatalf("terse SQL output still contains .sql suffix: %q", terse)
	}

	blade := &engine.ExplainResult{
		Symbol: "settings.parts.navbar",
		Definitions: []engine.ExplainDef{{
			Kind: "include",
			Path: "resources/views/settings/parts/navbar.blade.php",
			Line: 6,
		}},
	}
	terse = renderTo(t, "plain", blade)
	if !strings.Contains(terse, "i@navbar:6") {
		t.Fatalf("terse Blade output should omit .blade.php suffix: %q", terse)
	}
	if strings.Contains(terse, ".blade.php") {
		t.Fatalf("terse Blade output still contains .blade.php suffix: %q", terse)
	}

	dotnet := &engine.ExplainResult{
		Symbol: "Dapper.ProviderTools",
		Definitions: []engine.ExplainDef{{
			Kind: "project",
			Path: "src/Dapper.ProviderTools/Dapper.ProviderTools.csproj",
			Line: 1,
		}},
	}
	terse = renderTo(t, "plain", dotnet)
	if !strings.Contains(terse, "p@Dapper.ProviderTools:1") {
		t.Fatalf("terse .NET project output should omit project suffix: %q", terse)
	}
	if strings.Contains(terse, ".csproj") {
		t.Fatalf("terse .NET project output still contains .csproj suffix: %q", terse)
	}

	powershell := &engine.ExplainResult{
		Symbol: "Find-Module",
		Definitions: []engine.ExplainDef{{
			Kind: "function",
			Path: "src/PowerShellGet.psm1",
			Line: 409,
		}},
	}
	terse = renderTo(t, "plain", powershell)
	if !strings.Contains(terse, "f@PowerShellGet:409") {
		t.Fatalf("terse PowerShell output should omit script suffix: %q", terse)
	}
	if strings.Contains(terse, ".psm1") {
		t.Fatalf("terse PowerShell output still contains .psm1 suffix: %q", terse)
	}

	js := &engine.ExplainResult{
		Symbol: "sendFile",
		Definitions: []engine.ExplainDef{{
			Kind: "function",
			Path: "lib/response.js",
			Line: 373,
		}},
	}
	terse = renderTo(t, "plain", js)
	if !strings.Contains(terse, "f@response:373") {
		t.Fatalf("terse JavaScript output should omit .js suffix: %q", terse)
	}
	if strings.Contains(terse, ".js") {
		t.Fatalf("terse JavaScript output still contains .js suffix: %q", terse)
	}

	ts := &engine.ExplainResult{
		Symbol: "persistImpl",
		Definitions: []engine.ExplainDef{{
			Kind: "function",
			Path: "src/middleware/persist.ts",
			Line: 187,
		}},
	}
	terse = renderTo(t, "plain", ts)
	if !strings.Contains(terse, "f@persist:187") {
		t.Fatalf("terse TypeScript output should omit .ts suffix: %q", terse)
	}
	if strings.Contains(terse, ".ts") {
		t.Fatalf("terse TypeScript output still contains .ts suffix: %q", terse)
	}

	for _, tc := range []struct {
		name   string
		symbol string
		kind   string
		path   string
		line   int
		want   string
		suffix string
	}{
		{name: "cuda", symbol: "runTest", kind: "function", path: "samples/simpleAtomicIntrinsics.cu", line: 84, want: "f@simpleAtomicIntrinsics:84", suffix: ".cu"},
		{name: "razor", symbol: "CreateClick", kind: "method", path: "Pages/Create.razor", line: 125, want: "m@Create:125", suffix: ".razor"},
		{name: "vue", symbol: "parseMarkdown", kind: "function", path: "src/Article.vue", line: 101, want: "f@Article:101", suffix: ".vue"},
		{name: "verilog", symbol: "ibex_core", kind: "module", path: "rtl/ibex_core.sv", line: 16, want: "m@ibex_core:16", suffix: ".sv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &engine.ExplainResult{
				Symbol: tc.symbol,
				Definitions: []engine.ExplainDef{{
					Kind: tc.kind,
					Path: tc.path,
					Line: tc.line,
				}},
			}
			terse := renderTo(t, "plain", r)
			if !strings.Contains(terse, tc.want) {
				t.Fatalf("terse %s output should omit suffix: %q", tc.name, terse)
			}
			if strings.Contains(terse, tc.suffix) {
				t.Fatalf("terse %s output still contains %s suffix: %q", tc.name, tc.suffix, terse)
			}
		})
	}

	nonSQL := &engine.ExplainResult{
		Symbol: "NewRootCmd",
		Definitions: []engine.ExplainDef{{
			Kind: "function",
			Path: "internal/cli/root.go",
			Line: 52,
		}},
	}
	terse = renderTo(t, "plain", nonSQL)
	if !strings.Contains(terse, "f@root:52") {
		t.Fatalf("terse Go output should omit .go suffix: %q", terse)
	}
	if strings.Contains(terse, ".go") {
		t.Fatalf("terse Go output still contains .go suffix: %q", terse)
	}

	unlisted := &engine.ExplainResult{
		Symbol: "Schema",
		Definitions: []engine.ExplainDef{{
			Kind: "document",
			Path: "docs/schema.graphql",
			Line: 1,
		}},
	}
	terse = renderTo(t, "plain", unlisted)
	if !strings.Contains(terse, "d@schema.graphql:1") {
		t.Fatalf("terse unlisted extension output should keep suffix: %q", terse)
	}
}

func TestRenderPrettyIsIndentedDefault(t *testing.T) {
	out := renderTo(t, "", hubExplain())
	if !strings.Contains(out, "\n  \"symbol\"") {
		t.Fatalf("default render should be indented JSON; got:\n%s", out[:min(200, len(out))])
	}
}

// unknownResult is a type the terse switch does not handle; it must fall back to
// compact JSON instead of erroring or producing empty output.
type unknownResult struct {
	Foo string `json:"foo"`
	Bar int    `json:"bar"`
}

func TestRenderTerseUnknownFallsBackToCompactJSON(t *testing.T) {
	out := renderTo(t, "plain", &unknownResult{Foo: "hello", Bar: 7})
	var got unknownResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unknown-type terse fallback is not valid JSON: %v\n%s", err, out)
	}
	if got.Foo != "hello" || got.Bar != 7 {
		t.Fatalf("unknown-type fallback lost fields: %+v", got)
	}
	if strings.Contains(out, "\n  ") {
		t.Fatalf("unknown-type fallback should be compact, not indented:\n%s", out)
	}
}

func TestRenderNDJSONOnePerLine(t *testing.T) {
	r := &engine.CallersResult{
		Symbol: "Foo",
		Total:  3,
		Callers: []engine.SymbolRef{
			{Name: "A", Kind: "function", Path: "a.go", Line: 1},
			{Name: "B", Kind: "function", Path: "b.go", Line: 2},
			{Name: "C", Kind: "function", Path: "c.go", Line: 3},
		},
	}
	out := renderTo(t, "ndjson", r)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("ndjson should emit one object per caller (3); got %d:\n%s", len(lines), out)
	}
	for _, ln := range lines {
		var ref engine.SymbolRef
		if err := json.Unmarshal([]byte(ln), &ref); err != nil {
			t.Fatalf("ndjson line is not valid JSON: %v\n%s", err, ln)
		}
	}
}

func TestRenderJSONDelegatesToRender(t *testing.T) {
	// renderJSON must honor --format so existing commands pick up the new path.
	var buf bytes.Buffer
	withFormat("compact", false, func() {
		if err := renderJSON(&buf, hubExplain()); err != nil {
			t.Fatalf("renderJSON error: %v", err)
		}
	})
	if strings.Contains(buf.String(), "\n  ") {
		t.Fatalf("renderJSON should delegate to render() and honor --format compact:\n%s", buf.String())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
