package query

import (
	"context"
	"reflect"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/store"
)

// saveGraph persists syms/edges under snapID into a throwaway store and returns
// the context + driver, stamping the snapshot id onto every row.
func saveGraph(t *testing.T, snapID string, syms []graph.CodeSymbol, edges []graph.DependencyEdge) (context.Context, store.StorageDriver) {
	t.Helper()
	ctx := context.Background()
	d := openTempStore(t)
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
	return ctx, d
}

func symNames(syms []graph.CodeSymbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}

// TestPathForwardChain walks the canonical A -> B -> C chain forward and asserts
// the shortest path A..C is [A B C].
func TestPathForwardChain(t *testing.T) {
	const snapID = "snap-path"
	syms, edges := tinyGraph()
	ctx, d := saveGraph(t, snapID, syms, edges)

	got, err := Path(ctx, d, snapID, "A", "C", 6)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if want := []string{"A", "B", "C"}; !reflect.DeepEqual(symNames(got), want) {
		t.Errorf("Path(A,C) = %v, want %v", symNames(got), want)
	}
}

// TestPathDirectHop asserts a one-edge path B -> C returns [B C] (length 1).
func TestPathDirectHop(t *testing.T) {
	const snapID = "snap-path-direct"
	syms, edges := tinyGraph()
	ctx, d := saveGraph(t, snapID, syms, edges)

	got, err := Path(ctx, d, snapID, "B", "C", 6)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if want := []string{"B", "C"}; !reflect.DeepEqual(symNames(got), want) {
		t.Errorf("Path(B,C) = %v, want %v", symNames(got), want)
	}
}

// TestPathUnreachableBackward asserts there is no FORWARD path from C to A
// (edges go A->B->C), so Path returns nil.
func TestPathUnreachableBackward(t *testing.T) {
	const snapID = "snap-path-unreach"
	syms, edges := tinyGraph()
	ctx, d := saveGraph(t, snapID, syms, edges)

	got, err := Path(ctx, d, snapID, "C", "A", 6)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != nil {
		t.Errorf("Path(C,A) = %v, want nil (C cannot reach A forward)", symNames(got))
	}
}

// TestPathDepthBound asserts the depth bound cuts off a path longer than allowed:
// A..C needs 2 hops, so max-depth 1 must fail to find it.
func TestPathDepthBound(t *testing.T) {
	const snapID = "snap-path-depth"
	syms, edges := tinyGraph()
	ctx, d := saveGraph(t, snapID, syms, edges)

	got, err := Path(ctx, d, snapID, "A", "C", 1)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != nil {
		t.Errorf("Path(A,C,depth=1) = %v, want nil (C is 2 hops from A)", symNames(got))
	}
}
