package engine

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dominic097/atlas/internal/export"
	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/store"
)

// referenceFullGraph is the pre-streaming slice-based fold that streamFullGraph
// replaced. The test pins streamFullGraph to produce a byte-identical export.Graph
// to this reference over the SAME persisted rows — the Phase-4 "stream the whole-
// graph read without changing output" guarantee.
func referenceFullGraph(syms []graph.CodeSymbol, edges []graph.DependencyEdge) export.Graph {
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

// TestStreamFullGraphMatchesSliceFold persists a snapshot that exercises every
// branch of the fold — duplicate symbol names (first-seen rep wins), a non-calls
// edge (skipped), an external callee (to_ref not an indexed symbol, dropped), a
// self call (f==t, dropped), and a duplicate call edge (deduped) — then asserts
// the streamed export.Graph is deeply equal to the reference slice fold AND that
// both render to byte-identical JSON.
func TestStreamFullGraphMatchesSliceFold(t *testing.T) {
	ctx := context.Background()
	drv, err := store.Open(ctx, store.Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = drv.Close() })
	if err := drv.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-export"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-x", CommitSHA: "c0ffee"}
	syms := []graph.CodeSymbol{
		{ID: "s1", SnapshotID: snapID, NodeID: "n1", Path: "a.go", Language: "go", Kind: "function", Name: "Alpha", StartLine: 1},
		{ID: "s2", SnapshotID: snapID, NodeID: "n2", Path: "b.go", Language: "go", Kind: "function", Name: "Beta", StartLine: 2},
		// Duplicate name "Alpha" — first-seen rep (s1) must win for both fold paths.
		{ID: "s3", SnapshotID: snapID, NodeID: "n3", Path: "c.go", Language: "go", Kind: "function", Name: "Alpha", StartLine: 3},
		{ID: "s4", SnapshotID: snapID, NodeID: "n4", Path: "d.go", Language: "go", Kind: "function", Name: "Gamma", StartLine: 4},
	}
	edges := []graph.DependencyEdge{
		{SnapshotID: snapID, FromFile: "a.go", FromSymbol: "Alpha", ToRef: "Beta", Kind: graph.EdgeCalls, Language: "go", Line: 1},
		// Duplicate of the first edge (after rep-mapping) — must be deduped.
		{SnapshotID: snapID, FromFile: "c.go", FromSymbol: "Alpha", ToRef: "Beta", Kind: graph.EdgeCalls, Language: "go", Line: 9},
		// Non-calls edge — skipped by the fold.
		{SnapshotID: snapID, FromFile: "b.go", FromSymbol: "Beta", ToRef: "Alpha", Kind: graph.EdgeReferences, Language: "go", Line: 2},
		// External callee (not an indexed symbol) — dropped.
		{SnapshotID: snapID, FromFile: "b.go", FromSymbol: "Beta", ToRef: "ExternalThing", Kind: graph.EdgeCalls, Language: "go", Line: 3},
		// Self call (f==t) — dropped.
		{SnapshotID: snapID, FromFile: "d.go", FromSymbol: "Gamma", ToRef: "Gamma", Kind: graph.EdgeCalls, Language: "go", Line: 4},
		// A real second distinct edge.
		{SnapshotID: snapID, FromFile: "b.go", FromSymbol: "Beta", ToRef: "Gamma", Kind: graph.EdgeCalls, Language: "go", Line: 5},
	}
	if err := drv.SaveSnapshot(ctx, snap, nil, syms, edges, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Reference fold over the persisted rows (read back via the List* path).
	wantSyms, err := drv.ListSymbols(ctx, snapID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	wantEdges, err := drv.ListEdges(ctx, snapID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	want := referenceFullGraph(wantSyms, wantEdges)

	eng := &localEngine{cfg: Config{Tier: "local", StorageKind: "sqlite"}, store: drv}
	got, err := eng.streamFullGraph(ctx, snapID)
	if err != nil {
		t.Fatalf("streamFullGraph: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("streamFullGraph != reference fold\n got %+v\nwant %+v", got, want)
	}
	// Byte-identical rendered output is the actual contract for graph_export.
	gotJSON, err := got.Render("json")
	if err != nil {
		t.Fatalf("render got: %v", err)
	}
	wantJSON, err := want.Render("json")
	if err != nil {
		t.Fatalf("render want: %v", err)
	}
	if gotJSON != wantJSON {
		t.Fatalf("streamed JSON != reference JSON\n got %s\nwant %s", gotJSON, wantJSON)
	}
}
