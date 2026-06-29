package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// TestSymbolsByIDsAndEdgesByFromFiles covers the two targeted, index-backed
// readers added for the engine hot paths: SymbolsByIDs (PK lookup) and
// EdgesByFromFiles (idx_edges_snapshot_fromfile). It checks correct rows, chunk
// boundaries beyond the IN-list chunk size, and empty input.
func TestSymbolsByIDsAndEdgesByFromFiles(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")

	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-1"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "deadbeef"}

	symbols := []graph.CodeSymbol{
		{
			ID: "sym-auth", SnapshotID: snapID, NodeID: "node-auth",
			Path: "auth.go", Language: "go", Kind: "function", Name: "AuthenticateUser",
			StartLine: 4,
			Metadata:  graph.JSONBMap{"recv_type": "Service"},
		},
		{
			ID: "sym-token", SnapshotID: snapID, NodeID: "node-token",
			Path: "token.go", Language: "go", Kind: "function", Name: "ValidateToken",
			StartLine: 2,
		},
		{
			ID: "sym-other", SnapshotID: snapID, NodeID: "node-other",
			Path: "other.go", Language: "go", Kind: "function", Name: "Unrelated",
			StartLine: 1,
		},
	}
	edges := []graph.DependencyEdge{
		{
			ID: "edge-1", SnapshotID: snapID,
			FromFile: "auth.go", FromSymbol: "AuthenticateUser", ToRef: "ValidateToken",
			Kind: graph.EdgeCalls, Language: "go", Line: 5,
			Metadata: graph.JSONBMap{"qualified_ref": "svc.ValidateToken"},
		},
		{
			ID: "edge-2", SnapshotID: snapID,
			FromFile: "auth.go", FromSymbol: "AuthenticateUser", ToRef: "Logger",
			Kind: graph.EdgeReferences, Language: "go", Line: 6,
		},
		{
			ID: "edge-3", SnapshotID: snapID,
			FromFile: "other.go", FromSymbol: "Unrelated", ToRef: "ValidateToken",
			Kind: graph.EdgeCalls, Language: "go", Line: 3,
		},
	}

	if err := d.SaveSnapshot(ctx, snap, nil, symbols, edges, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// SymbolsByIDs: PK lookup returns exactly the requested rows, metadata + node_id
	// intact; an unknown id is silently skipped.
	got, err := d.SymbolsByIDs(ctx, snapID, []string{"sym-auth", "sym-token", "sym-missing"})
	if err != nil {
		t.Fatalf("SymbolsByIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("SymbolsByIDs: got %d symbols, want 2 (got %v)", len(got), symbolIDs(got))
	}
	byID := map[string]graph.CodeSymbol{}
	for _, s := range got {
		byID[s.ID] = s
		if s.NodeID == "" {
			t.Errorf("SymbolsByIDs: node_id not populated for %s", s.ID)
		}
	}
	if _, ok := byID["sym-auth"]; !ok {
		t.Error("SymbolsByIDs: missing sym-auth")
	}
	if rt := byID["sym-auth"].Metadata["recv_type"]; rt != "Service" {
		t.Errorf("SymbolsByIDs: recv_type = %v, want Service", rt)
	}
	if _, ok := byID["sym-other"]; ok {
		t.Error("SymbolsByIDs: sym-other should not be returned (not requested)")
	}

	// Empty input is a no-op.
	none, err := d.SymbolsByIDs(ctx, snapID, nil)
	if err != nil {
		t.Fatalf("SymbolsByIDs(nil): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("SymbolsByIDs(nil): got %d, want 0", len(none))
	}

	// EdgesByFromFiles: only edges leaving auth.go (both kinds), in
	// (from_file, to_ref, kind) order — matching ListEdges. other.go's edge is excluded.
	authEdges, err := d.EdgesByFromFiles(ctx, snapID, []string{"auth.go"})
	if err != nil {
		t.Fatalf("EdgesByFromFiles: %v", err)
	}
	if len(authEdges) != 2 {
		t.Fatalf("EdgesByFromFiles(auth.go): got %d edges, want 2", len(authEdges))
	}
	for _, e := range authEdges {
		if e.FromFile != "auth.go" {
			t.Errorf("EdgesByFromFiles: leaked edge from %q", e.FromFile)
		}
	}
	// Order: to_ref Logger < ValidateToken, so the references edge sorts first.
	if authEdges[0].ToRef != "Logger" || authEdges[1].ToRef != "ValidateToken" {
		t.Errorf("EdgesByFromFiles: order = [%s %s], want [Logger ValidateToken]", authEdges[0].ToRef, authEdges[1].ToRef)
	}
	if md := authEdges[1].Metadata["qualified_ref"]; md != "svc.ValidateToken" {
		t.Errorf("EdgesByFromFiles: metadata qualified_ref = %v, want svc.ValidateToken", md)
	}

	// Dedupe + empty input.
	deduped, err := d.EdgesByFromFiles(ctx, snapID, []string{"auth.go", "auth.go"})
	if err != nil {
		t.Fatalf("EdgesByFromFiles(dup): %v", err)
	}
	if len(deduped) != 2 {
		t.Errorf("EdgesByFromFiles(dup): got %d, want 2 (dedupe failed)", len(deduped))
	}
	emptyE, err := d.EdgesByFromFiles(ctx, snapID, nil)
	if err != nil {
		t.Fatalf("EdgesByFromFiles(nil): %v", err)
	}
	if len(emptyE) != 0 {
		t.Errorf("EdgesByFromFiles(nil): got %d, want 0", len(emptyE))
	}
}

// TestSymbolsByIDsAndEdgesByFromFilesChunkBoundary writes more rows than the
// IN-list chunk size (symbolsChunk / callEdgesChunk = 400) so the chunk-loop is
// exercised across a boundary. SymbolsByIDs must return every requested id and
// EdgesByFromFiles every from_file even when the input list spans >1 chunk.
func TestSymbolsByIDsAndEdgesByFromFilesChunkBoundary(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")

	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-chunk"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "cafe"}

	// n > symbolsChunk (400) and > callEdgesChunk (400) forces a multi-chunk loop.
	const n = symbolsChunk + callEdgesChunk + 37 // 837, spans 3 symbol chunks / 3 edge chunks
	symbols := make([]graph.CodeSymbol, 0, n)
	edges := make([]graph.DependencyEdge, 0, n)
	ids := make([]string, 0, n)
	files := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("sym-%04d", i)
		path := fmt.Sprintf("file_%04d.go", i)
		symbols = append(symbols, graph.CodeSymbol{
			ID: id, SnapshotID: snapID, NodeID: graph.NodeID("node-" + id),
			Path: path, Language: "go", Kind: "function", Name: fmt.Sprintf("Func%04d", i),
			StartLine: 1,
		})
		edges = append(edges, graph.DependencyEdge{
			ID: fmt.Sprintf("edge-%04d", i), SnapshotID: snapID,
			FromFile: path, FromSymbol: fmt.Sprintf("Func%04d", i), ToRef: "shared",
			Kind: graph.EdgeCalls, Language: "go", Line: i + 1,
		})
		ids = append(ids, id)
		files = append(files, path)
	}

	if err := d.SaveSnapshot(ctx, snap, nil, symbols, edges, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	gotSyms, err := d.SymbolsByIDs(ctx, snapID, ids)
	if err != nil {
		t.Fatalf("SymbolsByIDs(chunk): %v", err)
	}
	if len(gotSyms) != n {
		t.Fatalf("SymbolsByIDs(chunk): got %d, want %d — chunk boundary dropped rows", len(gotSyms), n)
	}
	seen := map[string]bool{}
	for _, s := range gotSyms {
		seen[s.ID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			t.Fatalf("SymbolsByIDs(chunk): missing %s", id)
		}
	}

	gotEdges, err := d.EdgesByFromFiles(ctx, snapID, files)
	if err != nil {
		t.Fatalf("EdgesByFromFiles(chunk): %v", err)
	}
	if len(gotEdges) != n {
		t.Fatalf("EdgesByFromFiles(chunk): got %d, want %d — chunk boundary dropped rows", len(gotEdges), n)
	}
}

// TestSymbolsByIDsMatchesListSymbols proves the targeted reader returns the SAME
// symbol shapes the load-all ListSymbols path would, for the requested ids.
func TestSymbolsByIDsMatchesListSymbols(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-eq"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1"}
	symbols := []graph.CodeSymbol{
		{ID: "a", SnapshotID: snapID, NodeID: "na", Path: "a.go", Kind: "function", Name: "A", Metadata: graph.JSONBMap{"k": "v"}},
		{ID: "b", SnapshotID: snapID, NodeID: "nb", Path: "b.go", Kind: "function", Name: "B"},
	}
	if err := d.SaveSnapshot(ctx, snap, nil, symbols, nil, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	all, err := d.ListSymbols(ctx, snapID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	allByID := map[string]graph.CodeSymbol{}
	for _, s := range all {
		allByID[s.ID] = s
	}

	targeted, err := d.SymbolsByIDs(ctx, snapID, []string{"a", "b"})
	if err != nil {
		t.Fatalf("SymbolsByIDs: %v", err)
	}
	for _, s := range targeted {
		want := allByID[s.ID]
		if s.Name != want.Name || s.Path != want.Path || string(s.NodeID) != string(want.NodeID) || s.Kind != want.Kind {
			t.Errorf("SymbolsByIDs %s diverges from ListSymbols: got %+v want %+v", s.ID, s, want)
		}
		if fmt.Sprint(s.Metadata) != fmt.Sprint(want.Metadata) {
			t.Errorf("SymbolsByIDs %s metadata diverges: got %v want %v", s.ID, s.Metadata, want.Metadata)
		}
	}
}

func symbolIDs(syms []graph.CodeSymbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.ID)
	}
	sort.Strings(out)
	return out
}
