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
	if !strings.Contains(terse, "f@root.go:52") {
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
