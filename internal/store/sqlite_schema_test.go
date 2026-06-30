package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dominic097/atlas/internal/graph"
)

func TestSQLiteMigrateCreatesExplainHotPathIndexes(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	sd, ok := d.(*sqliteDriver)
	if !ok {
		t.Fatalf("expected *sqliteDriver, got %T", d)
	}

	for _, name := range []string{
		"idx_snapshots_created",
		"idx_symbols_snapshot_name_path_line",
		"idx_edges_snapshot_kind_toref",
		"idx_edges_snapshot_kind_fromsymbol",
	} {
		var n int
		if err := sd.db.QueryRowContext(ctx,
			"SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?", name).Scan(&n); err != nil {
			t.Fatalf("count index %s: %v", name, err)
		}
		if n != 1 {
			t.Fatalf("index %s exists %d times, want 1", name, n)
		}
	}
	for _, name := range []string{
		"idx_symbols_snapshot_name",
		"idx_edges_snapshot_toref",
		"idx_edges_snapshot_fromsymbol",
	} {
		var n int
		if err := sd.db.QueryRowContext(ctx,
			"SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?", name).Scan(&n); err != nil {
			t.Fatalf("count index %s: %v", name, err)
		}
		if n != 0 {
			t.Fatalf("redundant index %s exists %d times, want 0", name, n)
		}
	}
}

func TestSQLiteLatestSnapshotAnyUsesNewestVisibleSnapshot(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, repo := range []graph.Repo{
		{ID: "repo-a", FullName: "org/a", Scope: "team-a"},
		{ID: "repo-b", FullName: "org/b", Scope: "team-b"},
		{ID: "repo-c", FullName: "org/c", Scope: "team-a"},
	} {
		if _, err := d.EnsureRepo(ctx, &repo); err != nil {
			t.Fatalf("EnsureRepo(%s): %v", repo.ID, err)
		}
	}

	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	for _, snap := range []graph.Snapshot{
		{ID: "snap-a", RepoID: "repo-a", CommitSHA: "a", CreatedAt: base},
		{ID: "snap-b", RepoID: "repo-b", CommitSHA: "b", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "snap-c", RepoID: "repo-c", CommitSHA: "c", CreatedAt: base.Add(time.Minute)},
	} {
		s := snap
		if err := d.SaveSnapshot(ctx, &s, nil, nil, nil, nil); err != nil {
			t.Fatalf("SaveSnapshot(%s): %v", snap.ID, err)
		}
	}

	any, err := d.LatestSnapshotAny(ctx, "")
	if err != nil {
		t.Fatalf("LatestSnapshotAny(all): %v", err)
	}
	if any == nil || any.ID != "snap-b" {
		t.Fatalf("LatestSnapshotAny(all) = %+v, want snap-b", any)
	}

	scoped, err := d.LatestSnapshotAny(ctx, "team-a")
	if err != nil {
		t.Fatalf("LatestSnapshotAny(team-a): %v", err)
	}
	if scoped == nil || scoped.ID != "snap-c" {
		t.Fatalf("LatestSnapshotAny(team-a) = %+v, want snap-c", scoped)
	}

	byFullName, err := d.(*sqliteDriver).LatestSnapshotByRepoRef(ctx, "team-a", "org/c")
	if err != nil {
		t.Fatalf("LatestSnapshotByRepoRef(full name): %v", err)
	}
	if byFullName == nil || byFullName.ID != "snap-c" {
		t.Fatalf("LatestSnapshotByRepoRef(full name) = %+v, want snap-c", byFullName)
	}

	byBase, err := d.(*sqliteDriver).LatestSnapshotByRepoRef(ctx, "team-a", "c")
	if err != nil {
		t.Fatalf("LatestSnapshotByRepoRef(base): %v", err)
	}
	if byBase == nil || byBase.ID != "snap-c" {
		t.Fatalf("LatestSnapshotByRepoRef(base) = %+v, want snap-c", byBase)
	}

	crossScope, err := d.(*sqliteDriver).LatestSnapshotByRepoRef(ctx, "team-b", "org/c")
	if err != nil {
		t.Fatalf("LatestSnapshotByRepoRef(cross scope): %v", err)
	}
	if crossScope != nil {
		t.Fatalf("LatestSnapshotByRepoRef(cross scope) = %+v, want nil", crossScope)
	}
}
