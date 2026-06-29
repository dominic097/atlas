package store

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

func openSQLiteTest(t *testing.T) StorageDriver {
	t.Helper()
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return d
}

func symFP(t *testing.T, d StorageDriver, snap string) []string {
	t.Helper()
	syms, err := d.ListSymbols(context.Background(), snap)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	out := make([]string, 0, len(syms))
	for i := range syms {
		s := &syms[i]
		out = append(out, s.Path+"|"+s.Name+"|"+string(s.NodeID)+"|"+s.Kind)
	}
	sort.Strings(out)
	return out
}

func edgeFP(t *testing.T, d StorageDriver, snap string) []string {
	t.Helper()
	edges, err := d.ListEdges(context.Background(), snap)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	out := make([]string, 0, len(edges))
	for i := range edges {
		e := &edges[i]
		recv, _ := e.Metadata["recv_type"].(string)
		out = append(out, e.FromFile+"|"+e.ToRef+"|"+string(e.Kind)+"|"+strconv.Itoa(e.Line)+"|"+recv)
	}
	sort.Strings(out)
	return out
}

func routeFP(t *testing.T, d StorageDriver, snap string) []string {
	t.Helper()
	rts, err := d.ListRoutes(context.Background(), snap, "")
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	out := make([]string, 0, len(rts))
	for i := range rts {
		r := &rts[i]
		out = append(out, r.Method+"|"+r.PathPattern+"|"+r.HandlerFile+"|"+r.Role)
	}
	sort.Strings(out)
	return out
}

func eqFP(t *testing.T, label string, want, got []string) {
	t.Helper()
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Fatalf("%s mismatch:\n want=%v\n  got=%v", label, want, got)
	}
}

// TestReplaceFileRowsEquivalentToSaveSnapshot is the SQL-primitive parity test for
// the Phase 1B incremental delta. It builds a base snapshot, then applies a delta two
// ways and asserts they produce identical persisted rows + counts:
//
//	A) ReplaceFileRows: replace the edited file's rows in place (fileScope = {a.go});
//	   and a reverse-dep file's EDGES only (edgeScope = {a.go, b.go}).
//	B) SaveSnapshot of the equivalent fully-merged row set (the reference).
//
// A reverse-dep file (b.go) keeps its SYMBOL rows but gets fresh EDGE rows — the
// exact file-vs-edge scope split the Go scoped delta relies on.
func TestReplaceFileRowsEquivalentToSaveSnapshot(t *testing.T) {
	ctx := context.Background()

	// Base rows: three files. a.go (edited), b.go (reverse-dep: symbols kept, edges
	// refreshed), c.go (untouched).
	baseSymbols := []graph.CodeSymbol{
		{ID: "s-a1", NodeID: "n-a1", Path: "a.go", Language: "go", Kind: "function", Name: "Old"},
		{ID: "s-b1", NodeID: "n-b1", Path: "b.go", Language: "go", Kind: "function", Name: "BUser"},
		{ID: "s-c1", NodeID: "n-c1", Path: "c.go", Language: "go", Kind: "function", Name: "CUser"},
	}
	baseEdges := []graph.DependencyEdge{
		{ID: "e-a1", FromFile: "a.go", ToRef: "BUser", Kind: graph.EdgeCalls, Language: "go", Line: 2},
		{ID: "e-b1", FromFile: "b.go", ToRef: "Widget", Kind: graph.EdgeReferences, Language: "go", Line: 3,
			Metadata: graph.JSONBMap{"recv_type": "old"}},
		{ID: "e-c1", FromFile: "c.go", ToRef: "BUser", Kind: graph.EdgeCalls, Language: "go", Line: 4},
	}
	baseFiles := []graph.File{
		{ID: "f-a", Path: "a.go", Language: "go", Hash: "ha", SizeBytes: 10},
		{ID: "f-b", Path: "b.go", Language: "go", Hash: "hb", SizeBytes: 20},
		{ID: "f-c", Path: "c.go", Language: "go", Hash: "hc", SizeBytes: 30},
	}
	baseRoutes := []graph.Route{
		{ID: "r-a", Method: "GET", PathPattern: "/a", HandlerFile: "a.go", Role: "producer", Source: "route_table", Confidence: "high"},
		{ID: "r-c", Method: "GET", PathPattern: "/c", HandlerFile: "c.go", Role: "producer", Source: "route_table", Confidence: "high"},
	}

	// The delta: a.go's symbol Old -> New, its call edge target shifts; b.go's edge
	// recv_type refreshes (reverse-dep); c.go untouched.
	newFiles := []graph.File{{ID: "f-a2", Path: "a.go", Language: "go", Hash: "ha2", SizeBytes: 11}}
	newSymbolsA := []graph.CodeSymbol{
		{ID: "s-a2", NodeID: "n-a2", Path: "a.go", Language: "go", Kind: "function", Name: "New"},
	}
	newEdges := []graph.DependencyEdge{
		{ID: "e-a2", FromFile: "a.go", ToRef: "CUser", Kind: graph.EdgeCalls, Language: "go", Line: 2},
		{ID: "e-b2", FromFile: "b.go", ToRef: "Widget", Kind: graph.EdgeReferences, Language: "go", Line: 3,
			Metadata: graph.JSONBMap{"recv_type": "new"}},
	}
	newRoutesA := []graph.Route{
		{ID: "r-a2", Method: "POST", PathPattern: "/a", HandlerFile: "a.go", Role: "producer", Source: "route_table", Confidence: "high"},
	}

	// --- A) ReplaceFileRows path ---
	dA := openSQLiteTest(t)
	snapA := &graph.Snapshot{ID: "snapA", RepoID: "repo", CommitSHA: "c0"}
	if err := dA.SaveSnapshot(ctx, snapA, baseFiles, baseSymbols, baseEdges, baseRoutes); err != nil {
		t.Fatalf("base SaveSnapshot A: %v", err)
	}
	// fileScope = {a.go} (symbols/files/routes), edgeScope = {a.go, b.go} (edges).
	// New totals: files 3 (a replaced), symbols 3 (a's 1 replaced), edges: base 3 −
	// (a:1 + b:1) + new 2 = 3, routes: base 2 − (a:1) + new 1 = 2.
	if err := dA.ReplaceFileRows(ctx, "snapA",
		[]string{"a.go"}, []string{"a.go", "b.go"},
		newFiles, newSymbolsA, newEdges, newRoutesA,
		3, 3, 3, 2); err != nil {
		t.Fatalf("ReplaceFileRows: %v", err)
	}

	// --- B) Reference: SaveSnapshot of the merged set ---
	dB := openSQLiteTest(t)
	snapB := &graph.Snapshot{ID: "snapB", RepoID: "repo", CommitSHA: "c0"}
	mergedFiles := []graph.File{
		{ID: "f-a2", Path: "a.go", Language: "go", Hash: "ha2", SizeBytes: 11},
		{ID: "f-b", Path: "b.go", Language: "go", Hash: "hb", SizeBytes: 20},
		{ID: "f-c", Path: "c.go", Language: "go", Hash: "hc", SizeBytes: 30},
	}
	mergedSymbols := []graph.CodeSymbol{
		{ID: "s-a2", NodeID: "n-a2", Path: "a.go", Language: "go", Kind: "function", Name: "New"},
		{ID: "s-b1", NodeID: "n-b1", Path: "b.go", Language: "go", Kind: "function", Name: "BUser"},
		{ID: "s-c1", NodeID: "n-c1", Path: "c.go", Language: "go", Kind: "function", Name: "CUser"},
	}
	mergedEdges := []graph.DependencyEdge{
		{ID: "e-a2", FromFile: "a.go", ToRef: "CUser", Kind: graph.EdgeCalls, Language: "go", Line: 2},
		{ID: "e-b2", FromFile: "b.go", ToRef: "Widget", Kind: graph.EdgeReferences, Language: "go", Line: 3,
			Metadata: graph.JSONBMap{"recv_type": "new"}},
		{ID: "e-c1", FromFile: "c.go", ToRef: "BUser", Kind: graph.EdgeCalls, Language: "go", Line: 4},
	}
	mergedRoutes := []graph.Route{
		{ID: "r-a2", Method: "POST", PathPattern: "/a", HandlerFile: "a.go", Role: "producer", Source: "route_table", Confidence: "high"},
		{ID: "r-c", Method: "GET", PathPattern: "/c", HandlerFile: "c.go", Role: "producer", Source: "route_table", Confidence: "high"},
	}
	if err := dB.SaveSnapshot(ctx, snapB, mergedFiles, mergedSymbols, mergedEdges, mergedRoutes); err != nil {
		t.Fatalf("merged SaveSnapshot B: %v", err)
	}

	// Row-set parity (id-independent fingerprints).
	eqFP(t, "symbols", symFP(t, dB, "snapB"), symFP(t, dA, "snapA"))
	eqFP(t, "edges", edgeFP(t, dB, "snapB"), edgeFP(t, dA, "snapA"))
	eqFP(t, "routes", routeFP(t, dB, "snapB"), routeFP(t, dA, "snapA"))

	// Counts parity (ReplaceFileRows updated snapA's counts).
	la, err := dA.LatestSnapshot(ctx, "repo")
	if err != nil {
		t.Fatalf("LatestSnapshot A: %v", err)
	}
	if la.FileCount != 3 || la.SymbolCount != 3 || la.EdgeCount != 3 || la.RouteCount != 2 {
		t.Fatalf("snapA counts after replace = files=%d syms=%d edges=%d routes=%d, want 3/3/3/2",
			la.FileCount, la.SymbolCount, la.EdgeCount, la.RouteCount)
	}

	// Reverse-dep file b.go: SYMBOL kept (BUser), EDGE refreshed (recv_type new).
	bSyms, err := dA.SymbolsByPath(ctx, "snapA", "b.go")
	if err != nil {
		t.Fatalf("SymbolsByPath b.go: %v", err)
	}
	if len(bSyms) != 1 || bSyms[0].Name != "BUser" {
		t.Fatalf("reverse-dep b.go symbol not preserved: %+v", bSyms)
	}
	bEdges, err := dA.EdgesByFromFiles(ctx, "snapA", []string{"b.go"})
	if err != nil {
		t.Fatalf("EdgesByFromFiles b.go: %v", err)
	}
	if len(bEdges) != 1 {
		t.Fatalf("reverse-dep b.go edges = %d, want 1", len(bEdges))
	}
	if recv, _ := bEdges[0].Metadata["recv_type"].(string); recv != "new" {
		t.Fatalf("reverse-dep b.go edge recv_type = %q, want refreshed 'new'", recv)
	}

	// Untouched file c.go: rows entirely preserved.
	cSyms, _ := dA.SymbolsByPath(ctx, "snapA", "c.go")
	if len(cSyms) != 1 || cSyms[0].Name != "CUser" {
		t.Fatalf("untouched c.go symbol changed: %+v", cSyms)
	}
}
