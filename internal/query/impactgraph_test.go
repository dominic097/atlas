package query

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

// openTempStore opens a migrated, throwaway sqlite StorageDriver.
func openTempStore(t *testing.T) store.StorageDriver {
	t.Helper()
	ctx := context.Background()
	d, err := store.Open(ctx, store.Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return d
}

// TestImpactGraphParityWithInMemory asserts the indexed ImpactGraph returns the
// SAME blast radius as the in-memory Impact over the canonical A->B->C chain,
// both seeded by symbol and by path.
func TestImpactGraphParityWithInMemory(t *testing.T) {
	ctx := context.Background()
	d := openTempStore(t)
	const snapID = "snap-chain"

	syms, edges := tinyGraph()
	for i := range syms {
		syms[i].SnapshotID = snapID
	}
	for i := range edges {
		edges[i].SnapshotID = snapID
	}
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "deadbeef"}
	if err := d.SaveSnapshot(ctx, snap, nil, syms, edges, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Seeded by symbol C, depth 2 -> A and B, depth 2.
	want := Impact(syms, edges, nil, []string{"C"}, 2)
	got, err := ImpactGraph(ctx, d, snapID, nil, []string{"C"}, 2)
	if err != nil {
		t.Fatalf("ImpactGraph: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("by-symbol: ImpactGraph = %+v, in-memory Impact = %+v", got, want)
	}

	// Seeded by path c.go must be equivalent.
	wantPath := Impact(syms, edges, []string{"c.go"}, nil, 2)
	gotPath, err := ImpactGraph(ctx, d, snapID, []string{"c.go"}, nil, 2)
	if err != nil {
		t.Fatalf("ImpactGraph(by path): %v", err)
	}
	if !reflect.DeepEqual(gotPath, wantPath) {
		t.Errorf("by-path: ImpactGraph = %+v, in-memory Impact = %+v", gotPath, wantPath)
	}

	// Depth 1 stops at the direct caller B.
	got1, err := ImpactGraph(ctx, d, snapID, nil, []string{"C"}, 1)
	if err != nil {
		t.Fatalf("ImpactGraph(depth1): %v", err)
	}
	if !reflect.DeepEqual(got1.ImpactedSymbols, []string{"B"}) || got1.DepthReached != 1 {
		t.Errorf("depth1: ImpactGraph = %+v, want symbols [B] depth 1", got1)
	}
}

// TestImpactGraphReceiverTypePrecision asserts the receiver-type filter defeats
// method-name collisions in BOTH ImpactGraph and the in-memory Impact: a call
// app.Index() (recv_type=LocalEngine) must reach ONLY the LocalEngine.Index
// method, never an unrelated Index method on a different type — and a call whose
// receiver type matches NO indexed method (the bleve.Index() case) drops.
func TestImpactGraphReceiverTypePrecision(t *testing.T) {
	ctx := context.Background()
	d := openTempStore(t)
	const snapID = "snap-coll"

	// Two methods named Index on different types; one free func callerOurs that
	// calls app.Index() (our LocalEngine), and one callerBleve that calls
	// blv.Index() whose receiver type is the external "BleveIndex" (no indexed
	// method on that type -> must drop).
	syms := []graph.CodeSymbol{
		{ID: "m-local", SnapshotID: snapID, Path: "engine/engine.go", Language: "go", Kind: "method", Name: "Index",
			Metadata: graph.JSONBMap{"recv_type": "LocalEngine"}},
		{ID: "m-store", SnapshotID: snapID, Path: "store/store.go", Language: "go", Kind: "method", Name: "Index",
			Metadata: graph.JSONBMap{"recv_type": "Store"}},
		{ID: "f-ours", SnapshotID: snapID, Path: "engine/run.go", Language: "go", Kind: "function", Name: "callerOurs"},
		{ID: "f-bleve", SnapshotID: snapID, Path: "lexical/build.go", Language: "go", Kind: "function", Name: "callerBleve"},
	}
	edges := []graph.DependencyEdge{
		// app.Index() with recv_type LocalEngine -> only m-local.
		{ID: "e-ours", SnapshotID: snapID, FromFile: "engine/run.go", FromSymbol: "callerOurs", ToRef: "Index",
			Kind: graph.EdgeCalls, Language: "go",
			Metadata: graph.JSONBMap{"qualified_ref": "app.Index", "recv_type": "LocalEngine"}},
		// blv.Index() with recv_type BleveIndex -> no indexed method on that type -> drop.
		{ID: "e-bleve", SnapshotID: snapID, FromFile: "lexical/build.go", FromSymbol: "callerBleve", ToRef: "Index",
			Kind: graph.EdgeCalls, Language: "go",
			Metadata: graph.JSONBMap{"qualified_ref": "blv.Index", "recv_type": "BleveIndex"}},
	}
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "cafe"}
	if err := d.SaveSnapshot(ctx, snap, nil, syms, edges, nil); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Change the LocalEngine.Index method (seed by path engine/engine.go).
	// Only callerOurs must surface; callerBleve must NOT (its blv.Index() is external).
	want := []string{"callerOurs"}

	mem := Impact(syms, edges, []string{"engine/engine.go"}, nil, 1)
	if !reflect.DeepEqual(mem.ImpactedSymbols, want) {
		t.Errorf("in-memory Impact: ImpactedSymbols = %v, want %v (bleve.Index FP must drop)", mem.ImpactedSymbols, want)
	}

	idx, err := ImpactGraph(ctx, d, snapID, []string{"engine/engine.go"}, nil, 1)
	if err != nil {
		t.Fatalf("ImpactGraph: %v", err)
	}
	if !reflect.DeepEqual(idx.ImpactedSymbols, want) {
		t.Errorf("ImpactGraph: ImpactedSymbols = %v, want %v (bleve.Index FP must drop)", idx.ImpactedSymbols, want)
	}

	// And changing the OTHER same-named method (Store.Index) must NOT pull in
	// callerOurs (whose call resolves to LocalEngine.Index, a different identity).
	memStore := Impact(syms, edges, []string{"store/store.go"}, nil, 1)
	if len(memStore.ImpactedSymbols) != 0 {
		t.Errorf("in-memory Impact(Store.Index): got %v, want none (no caller targets Store.Index)", memStore.ImpactedSymbols)
	}
	idxStore, err := ImpactGraph(ctx, d, snapID, []string{"store/store.go"}, nil, 1)
	if err != nil {
		t.Fatalf("ImpactGraph(Store.Index): %v", err)
	}
	if len(idxStore.ImpactedSymbols) != 0 {
		t.Errorf("ImpactGraph(Store.Index): got %v, want none", idxStore.ImpactedSymbols)
	}
}
