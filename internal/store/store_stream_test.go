package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// TestStreamMatchesList verifies StreamSymbols/StreamEdges yield exactly the same
// rows (same order) as ListSymbols/ListEdges, across a small batch size so the
// batching boundary is exercised — the whole-graph consumers (analytics, export,
// snapshot-diff) rely on this equivalence to keep their output byte-identical.
func TestStreamMatchesList(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-1"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "deadbeef"}
	var symbols []graph.CodeSymbol
	var edges []graph.DependencyEdge
	var files []graph.File
	// 10 symbols across 3 files + 7 edges — enough to cross a batch size of 3.
	paths := []string{"a.go", "b.go", "c.go"}
	for i := 0; i < 10; i++ {
		p := paths[i%len(paths)]
		symbols = append(symbols, graph.CodeSymbol{
			ID: "sym-" + string(rune('a'+i)), SnapshotID: snapID, NodeID: graph.NodeID("node-" + string(rune('a'+i))),
			Path: p, Language: "go", Kind: "function", Name: "Fn" + string(rune('A'+i)),
			StartLine: i + 1, Metadata: graph.JSONBMap{"k": i},
		})
	}
	for i := 0; i < 7; i++ {
		edges = append(edges, graph.DependencyEdge{
			ID: "edge-" + string(rune('a'+i)), SnapshotID: snapID,
			FromFile: paths[i%len(paths)], FromSymbol: "FnA", ToRef: "FnB",
			Kind: graph.EdgeCalls, Language: "go", Line: i + 1,
		})
	}
	for _, p := range paths {
		files = append(files, graph.File{ID: "file-" + p, SnapshotID: snapID, Path: p, Language: "go"})
	}
	if err := d.SaveSnapshot(ctx, snap, files, symbols, edges, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	wantSyms, err := d.ListSymbols(ctx, snapID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	var gotSyms []graph.CodeSymbol
	batches := 0
	if err := d.StreamSymbols(ctx, snapID, 3, func(b []graph.CodeSymbol) error {
		batches++
		gotSyms = append(gotSyms, b...)
		return nil
	}); err != nil {
		t.Fatalf("StreamSymbols: %v", err)
	}
	if !reflect.DeepEqual(gotSyms, wantSyms) {
		t.Fatalf("StreamSymbols != ListSymbols\n got %+v\nwant %+v", gotSyms, wantSyms)
	}
	if batches < 3 { // 10 rows / batch 3 -> 4 batches; proves batching actually happened
		t.Fatalf("expected multiple stream batches, got %d", batches)
	}

	wantEdges, err := d.ListEdges(ctx, snapID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	var gotEdges []graph.DependencyEdge
	if err := d.StreamEdges(ctx, snapID, 2, func(b []graph.DependencyEdge) error {
		gotEdges = append(gotEdges, b...)
		return nil
	}); err != nil {
		t.Fatalf("StreamEdges: %v", err)
	}
	if !reflect.DeepEqual(gotEdges, wantEdges) {
		t.Fatalf("StreamEdges != ListEdges\n got %+v\nwant %+v", gotEdges, wantEdges)
	}
}
