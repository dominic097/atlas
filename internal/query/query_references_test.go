package query

import (
	"reflect"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// TestReferencesGraphTypeUses asserts ReferencesGraph returns the enclosing
// declarations that NAME a type via "references" (type-use) edges — not callers.
//
// Graph:
//
//	type Widget (widget.go)             — the referenced type
//	func New (widget.go) references Widget  (e.g. returns *Widget)
//	func Wrap (other.go) references Widget   (e.g. takes a Widget param)
//	func Unrelated (other.go) — no reference
func TestReferencesGraphTypeUses(t *testing.T) {
	const snapID = "snap-refs"
	syms := []graph.CodeSymbol{
		{ID: "w", Path: "widget.go", Language: "go", Kind: "type", Name: "Widget", StartLine: 1, EndLine: 3},
		{ID: "n", Path: "widget.go", Language: "go", Kind: "function", Name: "New", StartLine: 5, EndLine: 7},
		{ID: "p", Path: "other.go", Language: "go", Kind: "function", Name: "Wrap", StartLine: 1, EndLine: 3},
		{ID: "u", Path: "other.go", Language: "go", Kind: "function", Name: "Unrelated", StartLine: 5, EndLine: 7},
	}
	edges := []graph.DependencyEdge{
		{ID: "r1", FromFile: "widget.go", FromSymbol: "New", ToRef: "Widget", Kind: graph.EdgeReferences, Language: "go", Line: 6},
		{ID: "r2", FromFile: "other.go", FromSymbol: "Wrap", ToRef: "Widget", Kind: graph.EdgeReferences, Language: "go", Line: 2},
	}
	ctx, d := saveGraph(t, snapID, syms, edges)

	got, err := ReferencesGraph(ctx, d, snapID, "Widget")
	if err != nil {
		t.Fatalf("ReferencesGraph: %v", err)
	}
	// sortSymbols orders by path then name: other.go/Wrap, widget.go/New.
	if want := []string{"Wrap", "New"}; !reflect.DeepEqual(symNames(got), want) {
		t.Errorf("references = %v, want %v", symNames(got), want)
	}
}

// TestRefEdgesByToRefsKindFilter asserts the store reader returns ONLY
// "references" edges for a to_ref, never "calls" edges that happen to share it.
func TestRefEdgesByToRefsKindFilter(t *testing.T) {
	const snapID = "snap-refs-kind"
	syms := []graph.CodeSymbol{
		{ID: "w", Path: "widget.go", Language: "go", Kind: "type", Name: "Widget", StartLine: 1, EndLine: 3},
		{ID: "n", Path: "widget.go", Language: "go", Kind: "function", Name: "New", StartLine: 5, EndLine: 7},
	}
	edges := []graph.DependencyEdge{
		// A call edge to "Widget" (e.g. a constructor-style call) — must be excluded.
		{ID: "c1", FromFile: "widget.go", FromSymbol: "New", ToRef: "Widget", Kind: graph.EdgeCalls, Language: "go", Line: 6},
		// A reference edge to "Widget" — must be included.
		{ID: "r1", FromFile: "widget.go", FromSymbol: "New", ToRef: "Widget", Kind: graph.EdgeReferences, Language: "go", Line: 6},
	}
	ctx, d := saveGraph(t, snapID, syms, edges)

	refEdges, err := d.RefEdgesByToRefs(ctx, snapID, []string{"Widget"})
	if err != nil {
		t.Fatalf("RefEdgesByToRefs: %v", err)
	}
	if len(refEdges) != 1 {
		t.Fatalf("RefEdgesByToRefs returned %d edges, want 1 (references only)", len(refEdges))
	}
	if refEdges[0].Kind != graph.EdgeReferences {
		t.Errorf("edge kind = %q, want references", refEdges[0].Kind)
	}
}
