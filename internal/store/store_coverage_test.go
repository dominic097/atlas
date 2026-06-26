package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// TestCoverageRoundTrip persists runtime coverage facts and reads them back, both
// unfiltered and filtered by symbol name, then asserts a re-save for the same
// snapshot replaces (does not append to) the prior facts.
func TestCoverageRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-cov"
	rows := []graph.Coverage{
		{SnapshotID: snapID, RepoFullName: "org/repo", SymbolRef: "Foo", CoverageType: "runtime_go", Strength: "3/3"},
		{SnapshotID: snapID, RepoFullName: "org/repo", SymbolRef: "Bar", CoverageType: "runtime_go", Strength: "0/4"},
	}
	if err := d.SaveCoverage(ctx, rows); err != nil {
		t.Fatalf("SaveCoverage: %v", err)
	}

	all, err := d.ListCoverage(ctx, snapID, "")
	if err != nil {
		t.Fatalf("ListCoverage(all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListCoverage(all) = %d rows, want 2", len(all))
	}

	foo, err := d.ListCoverage(ctx, snapID, "Foo")
	if err != nil {
		t.Fatalf("ListCoverage(Foo): %v", err)
	}
	if len(foo) != 1 || foo[0].SymbolRef != "Foo" || foo[0].Strength != "3/3" {
		t.Fatalf("ListCoverage(Foo) = %+v, want one Foo 3/3 row", foo)
	}
	if foo[0].CoverageType != "runtime_go" {
		t.Errorf("CoverageType = %q, want runtime_go", foo[0].CoverageType)
	}

	// Re-import for the same snapshot: a single new fact must REPLACE the prior two.
	if err := d.SaveCoverage(ctx, []graph.Coverage{
		{SnapshotID: snapID, RepoFullName: "org/repo", SymbolRef: "Baz", CoverageType: "runtime_lcov", Strength: "5/5"},
	}); err != nil {
		t.Fatalf("SaveCoverage (re-import): %v", err)
	}
	after, err := d.ListCoverage(ctx, snapID, "")
	if err != nil {
		t.Fatalf("ListCoverage(after): %v", err)
	}
	if len(after) != 1 || after[0].SymbolRef != "Baz" {
		t.Fatalf("after re-import = %+v, want only Baz (prior facts replaced)", after)
	}
}
