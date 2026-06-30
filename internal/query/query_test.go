package query

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/store"
)

// tinyGraph builds the canonical A -> B -> C call chain:
//
//	A (a.go) calls B
//	B (b.go) calls C
//	C (c.go) is the leaf
//
// Reverse-BFS from C must reach B at depth 1 and A at depth 2.
func tinyGraph() ([]graph.CodeSymbol, []graph.DependencyEdge) {
	syms := []graph.CodeSymbol{
		{ID: "sa", Path: "a.go", Language: "go", Kind: "function", Name: "A", StartLine: 1, EndLine: 3},
		{ID: "sb", Path: "b.go", Language: "go", Kind: "function", Name: "B", StartLine: 1, EndLine: 3},
		{ID: "sc", Path: "c.go", Language: "go", Kind: "function", Name: "C", StartLine: 1, EndLine: 3},
	}
	edges := []graph.DependencyEdge{
		{ID: "e1", FromFile: "a.go", FromSymbol: "A", ToRef: "B", Kind: graph.EdgeCalls, Language: "go", Line: 2},
		{ID: "e2", FromFile: "b.go", FromSymbol: "B", ToRef: "C", Kind: graph.EdgeCalls, Language: "go", Line: 2},
	}
	return syms, edges
}

func TestImpactReachesRootAtDepth2(t *testing.T) {
	syms, edges := tinyGraph()

	got := Impact(syms, edges, nil, []string{"C"}, 2)

	wantSyms := []string{"A", "B"}
	if !reflect.DeepEqual(got.ImpactedSymbols, wantSyms) {
		t.Errorf("ImpactedSymbols = %v, want %v", got.ImpactedSymbols, wantSyms)
	}

	wantFiles := []string{"a.go", "b.go"}
	if !reflect.DeepEqual(got.ImpactedFiles, wantFiles) {
		t.Errorf("ImpactedFiles = %v, want %v", got.ImpactedFiles, wantFiles)
	}

	if got.DepthReached != 2 {
		t.Errorf("DepthReached = %d, want 2 (A is two hops up from C)", got.DepthReached)
	}
}

func TestImpactDepth1StopsAtDirectCaller(t *testing.T) {
	syms, edges := tinyGraph()

	got := Impact(syms, edges, nil, []string{"C"}, 1)

	if !reflect.DeepEqual(got.ImpactedSymbols, []string{"B"}) {
		t.Errorf("depth 1 ImpactedSymbols = %v, want [B] only", got.ImpactedSymbols)
	}
	if got.DepthReached != 1 {
		t.Errorf("depth 1 DepthReached = %d, want 1", got.DepthReached)
	}
}

func TestImpactSeedByPath(t *testing.T) {
	syms, edges := tinyGraph()

	// Seeding by the changed FILE c.go should be equivalent to seeding by symbol C.
	got := Impact(syms, edges, []string{"c.go"}, nil, 2)

	if !reflect.DeepEqual(got.ImpactedSymbols, []string{"A", "B"}) {
		t.Errorf("path-seeded ImpactedSymbols = %v, want [A B]", got.ImpactedSymbols)
	}
	if got.DepthReached != 2 {
		t.Errorf("path-seeded DepthReached = %d, want 2", got.DepthReached)
	}
}

func TestImpactNoSeedMatch(t *testing.T) {
	syms, edges := tinyGraph()

	got := Impact(syms, edges, nil, []string{"Nonexistent"}, 3)

	if len(got.ImpactedSymbols) != 0 || len(got.ImpactedFiles) != 0 {
		t.Errorf("unmatched seed should yield empty impact, got %+v", got)
	}
	if got.DepthReached != 0 {
		t.Errorf("unmatched seed DepthReached = %d, want 0", got.DepthReached)
	}
	// Non-nil empty slices, not nil, so JSON encodes as [].
	if got.ImpactedSymbols == nil || got.ImpactedFiles == nil {
		t.Error("empty impact slices should be non-nil")
	}
}

func TestCallers(t *testing.T) {
	syms, edges := tinyGraph()

	callersOfC := Callers(edges, syms, "C")
	if len(callersOfC) != 1 || callersOfC[0].Name != "B" {
		t.Errorf("Callers(C) = %v, want [B]", names(callersOfC))
	}

	callersOfB := Callers(edges, syms, "B")
	if len(callersOfB) != 1 || callersOfB[0].Name != "A" {
		t.Errorf("Callers(B) = %v, want [A]", names(callersOfB))
	}

	if got := Callers(edges, syms, "A"); len(got) != 0 {
		t.Errorf("Callers(A) = %v, want empty (A is the root)", names(got))
	}
}

func TestReferencesIncludesReferenceEdges(t *testing.T) {
	syms, edges := tinyGraph()
	// D references C without calling it.
	syms = append(syms, graph.CodeSymbol{ID: "sd", Path: "d.go", Kind: "function", Name: "D", StartLine: 1, EndLine: 2})
	edges = append(edges, graph.DependencyEdge{
		ID: "e3", FromFile: "d.go", FromSymbol: "D", ToRef: "C", Kind: graph.EdgeReferences, Language: "go", Line: 1,
	})

	// Callers (calls only) sees just B.
	if got := names(Callers(edges, syms, "C")); !reflect.DeepEqual(got, []string{"B"}) {
		t.Errorf("Callers(C) = %v, want [B]", got)
	}

	// References (calls + references) sees both B and D.
	got := names(References(edges, syms, "C"))
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"B", "D"}) {
		t.Errorf("References(C) = %v, want [B D]", got)
	}
}

func TestGraphCountHelpersMatchFullGraph(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	drv, err := store.Open(ctx, store.Options{Kind: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = drv.Close() })
	if err := drv.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-1"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "deadbeef"}
	syms := []graph.CodeSymbol{
		{ID: "sa", SnapshotID: snapID, Path: "a.go", Language: "go", Kind: "function", Name: "A", StartLine: 1},
		{ID: "sb", SnapshotID: snapID, Path: "b.go", Language: "go", Kind: "function", Name: "B", StartLine: 1},
		{ID: "sc", SnapshotID: snapID, Path: "c.go", Language: "go", Kind: "function", Name: "C", StartLine: 1},
	}
	edges := []graph.DependencyEdge{
		{SnapshotID: snapID, FromFile: "a.go", FromSymbol: "A", ToRef: "B", Kind: graph.EdgeCalls, Language: "go", Line: 2},
		{SnapshotID: snapID, FromFile: "a.go", FromSymbol: "A", ToRef: "B", Kind: graph.EdgeCalls, Language: "go", Line: 3},
		{SnapshotID: snapID, FromFile: "b.go", FromSymbol: "B", ToRef: "C", Kind: graph.EdgeCalls, Language: "go", Line: 2},
	}
	if err := drv.SaveSnapshot(ctx, snap, nil, syms, edges, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	fullCallers, err := CallersGraph(ctx, drv, snapID, "B")
	if err != nil {
		t.Fatalf("CallersGraph: %v", err)
	}
	countCallers, err := CallersGraphCount(ctx, drv, snapID, "B")
	if err != nil {
		t.Fatalf("CallersGraphCount: %v", err)
	}
	if countCallers != len(fullCallers) {
		t.Fatalf("CallersGraphCount = %d, want %d", countCallers, len(fullCallers))
	}

	fullCallees, err := CalleesGraph(ctx, drv, snapID, "A")
	if err != nil {
		t.Fatalf("CalleesGraph: %v", err)
	}
	countCallees, err := CalleesGraphCount(ctx, drv, snapID, "A")
	if err != nil {
		t.Fatalf("CalleesGraphCount: %v", err)
	}
	if countCallees != len(fullCallees) {
		t.Fatalf("CalleesGraphCount = %d, want %d", countCallees, len(fullCallees))
	}
}

func TestImpactAmbiguousNameDedupes(t *testing.T) {
	// Two distinct callers share the name "Helper" but live in different files,
	// both calling C. They must both surface (deduped by identity, not name).
	syms := []graph.CodeSymbol{
		{ID: "sc", Path: "c.go", Name: "C", StartLine: 1, EndLine: 2},
		{ID: "h1", Path: "h1.go", Name: "Helper", StartLine: 1, EndLine: 2},
		{ID: "h2", Path: "h2.go", Name: "Helper", StartLine: 1, EndLine: 2},
	}
	edges := []graph.DependencyEdge{
		{ID: "e1", FromFile: "h1.go", FromSymbol: "Helper", ToRef: "C", Kind: graph.EdgeCalls},
		{ID: "e2", FromFile: "h2.go", FromSymbol: "Helper", ToRef: "C", Kind: graph.EdgeCalls},
	}

	got := Impact(syms, edges, nil, []string{"C"}, 1)
	if !reflect.DeepEqual(got.ImpactedSymbols, []string{"Helper"}) {
		t.Errorf("ImpactedSymbols = %v, want [Helper] (name-deduped)", got.ImpactedSymbols)
	}
	if !reflect.DeepEqual(got.ImpactedFiles, []string{"h1.go", "h2.go"}) {
		t.Errorf("ImpactedFiles = %v, want [h1.go h2.go] (both distinct callers)", got.ImpactedFiles)
	}
}

func names(syms []graph.CodeSymbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.Name
	}
	return out
}
