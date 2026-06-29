package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// TestSnapshotSurrogateRoundTripsPublicID proves the compact integer snapshot_id
// surrogate is invisible at the boundary: every child row read back (symbols,
// edges, files, routes) carries the EXACT public snapshot uuid it was saved with —
// including a non-UUID id — so engine/MCP output is byte-identical. It also checks
// the unknown-snapshot path still yields an empty result (NULL surrogate match).
func TestSnapshotSurrogateRoundTripsPublicID(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// A deliberately non-UUID public id — the surrogate must key off this exact
	// string, never parse it.
	const snapID = "head@feature/compact-schema"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "abc123"}
	files := []graph.File{{ID: "f1", SnapshotID: snapID, Path: "a.go", Language: "go"}}
	syms := []graph.CodeSymbol{
		{ID: "s1", SnapshotID: snapID, NodeID: "n1", Path: "a.go", Language: "go", Kind: "function", Name: "Alpha", StartLine: 1},
		{ID: "s2", SnapshotID: snapID, NodeID: "n2", Path: "a.go", Language: "go", Kind: "function", Name: "Beta", StartLine: 9},
	}
	edges := []graph.DependencyEdge{
		{SnapshotID: snapID, FromFile: "a.go", FromSymbol: "Alpha", ToRef: "Beta", Kind: graph.EdgeCalls, Language: "go", Line: 2},
	}
	routes := []graph.Route{
		{ID: "r1", SnapshotID: snapID, RepoFullName: "repo-1", Method: "GET", PathPattern: "/x", HandlerFile: "a.go", Role: "public"},
	}
	if err := d.SaveSnapshot(ctx, snap, files, syms, edges, routes); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	gotSyms, err := d.ListSymbols(ctx, snapID)
	if err != nil || len(gotSyms) != 2 {
		t.Fatalf("ListSymbols: %v (n=%d)", err, len(gotSyms))
	}
	for _, s := range gotSyms {
		if s.SnapshotID != snapID {
			t.Fatalf("symbol SnapshotID = %q, want %q", s.SnapshotID, snapID)
		}
	}
	gotEdges, err := d.ListEdges(ctx, snapID)
	if err != nil || len(gotEdges) != 1 {
		t.Fatalf("ListEdges: %v (n=%d)", err, len(gotEdges))
	}
	if gotEdges[0].SnapshotID != snapID {
		t.Fatalf("edge SnapshotID = %q, want %q", gotEdges[0].SnapshotID, snapID)
	}
	gotFiles, err := d.ListFiles(ctx, snapID)
	if err != nil || len(gotFiles) != 1 {
		t.Fatalf("ListFiles: %v (n=%d)", err, len(gotFiles))
	}
	if gotFiles[0].SnapshotID != snapID {
		t.Fatalf("file SnapshotID = %q, want %q", gotFiles[0].SnapshotID, snapID)
	}
	gotRoutes, err := d.ListRoutes(ctx, snapID, "")
	if err != nil || len(gotRoutes) != 1 {
		t.Fatalf("ListRoutes: %v (n=%d)", err, len(gotRoutes))
	}
	if gotRoutes[0].SnapshotID != snapID {
		t.Fatalf("route SnapshotID = %q, want %q", gotRoutes[0].SnapshotID, snapID)
	}

	// Targeted readers must also re-attach the public id.
	byName, err := d.SymbolsByName(ctx, snapID, "Alpha")
	if err != nil || len(byName) != 1 || byName[0].SnapshotID != snapID {
		t.Fatalf("SymbolsByName surrogate round-trip failed: %v %+v", err, byName)
	}

	// Unknown snapshot -> empty (the NULL-surrogate match), not an error.
	none, err := d.ListSymbols(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("ListSymbols(unknown): %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown snapshot returned %d symbols, want 0", len(none))
	}
}
