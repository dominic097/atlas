package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mkRepo creates a fake git repo (a .git dir is enough for isGitRepo/discoverRepos;
// index.Run falls back to a "working-tree" SHA when git rev-parse finds no commit)
// containing one Go source file, and returns its path.
func mkRepo(t *testing.T, parent, name, goSrc string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(goSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDiscoverReposFindsNestedSkipsScratch(t *testing.T) {
	ws := t.TempDir()
	a := mkRepo(t, ws, "repo-a", "package a\nfunc A() {}\n")
	b := mkRepo(t, ws, "repo-b", "package b\nfunc B() {}\n")
	// Agent scratch worktree under .claude — a full duplicate checkout that must
	// NOT be discovered as a repo to index.
	mkRepo(t, filepath.Join(ws, ".claude", "worktrees"), "dupe", "package d\nfunc D() {}\n")
	// node_modules with a stray .git — must be skipped.
	mkRepo(t, filepath.Join(ws, "node_modules", "pkg"), "nested", "package n\nfunc N() {}\n")
	// A plain non-git source dir — not a repo.
	if err := os.MkdirAll(filepath.Join(ws, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := discoverRepos(ws)
	want := []string{a, b} // sorted
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("discoverRepos = %v, want %v", got, want)
	}
	if isGitRepo(ws) {
		t.Errorf("workspace root should not be a git repo")
	}
	if !isGitRepo(a) {
		t.Errorf("repo-a should be a git repo")
	}
}

func TestIndexSegmentsWorkspace(t *testing.T) {
	ws := t.TempDir()
	mkRepo(t, ws, "svc-one", "package one\n\nfunc One() int { return 1 }\n")
	mkRepo(t, ws, "svc-two", "package two\n\nfunc Two() int { return 2 }\n")

	eng := newTestEngine(t, false)
	res, err := eng.Index(context.Background(), IndexInput{ProjectPath: ws})
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if res.Mode != "segmented" {
		t.Fatalf("Mode = %q, want segmented", res.Mode)
	}
	if len(res.Repos) != 2 {
		t.Fatalf("Repos = %d, want 2", len(res.Repos))
	}
	if res.Symbols < 2 {
		t.Errorf("aggregate Symbols = %d, want >=2 (One+Two)", res.Symbols)
	}
	for _, r := range res.Repos {
		if r.Error != "" {
			t.Errorf("repo %s failed: %s", r.Repo, r.Error)
		}
		if r.Symbols < 1 {
			t.Errorf("repo %s symbols = %d, want >=1", r.Repo, r.Symbols)
		}
		if r.SnapshotID == "" {
			t.Errorf("repo %s has empty snapshot id", r.Repo)
		}
	}
}

func TestIndexSegmentsOuterRepoWithNested(t *testing.T) {
	// The Aziron case: the workspace root is ITSELF a git repo (tracks loose
	// top-level files) AND contains several nested repos. Segmentation must still
	// trigger, index each nested repo, AND index the root's own loose files with the
	// nested repos pruned (so no file is indexed twice).
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "root.go"), []byte("package root\n\nfunc Root() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, ws, "svc-a", "package a\n\nfunc A() {}\n")
	mkRepo(t, ws, "svc-b", "package b\n\nfunc B() {}\n")

	eng := newTestEngine(t, false)
	res, err := eng.Index(context.Background(), IndexInput{ProjectPath: ws})
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if res.Mode != "segmented" {
		t.Fatalf("Mode = %q, want segmented (outer repo + nested)", res.Mode)
	}
	if len(res.Repos) != 3 {
		t.Fatalf("Repos = %d, want 3 (svc-a, svc-b, root)", len(res.Repos))
	}
	// The root job must see ONLY its loose file — the two nested repos are pruned, so
	// it indexes 1 file, not 3. This proves SkipPaths prevents double-indexing.
	rootName := filepath.Base(ws)
	var rootSummary *RepoIndexSummary
	for i := range res.Repos {
		if res.Repos[i].Repo == rootName {
			rootSummary = &res.Repos[i]
		}
	}
	if rootSummary == nil {
		t.Fatalf("no summary for the root repo %q in %+v", rootName, res.Repos)
	}
	if rootSummary.Files != 1 {
		t.Errorf("root repo Files = %d, want 1 (nested repos must be pruned, not re-walked)", rootSummary.Files)
	}
	if rootSummary.Symbols < 1 {
		t.Errorf("root repo Symbols = %d, want >=1 (Root func)", rootSummary.Symbols)
	}
}

func TestIndexSingleRepoNotSegmented(t *testing.T) {
	// Pointing directly AT a repo (its root has .git) indexes it as one repo and
	// never goes hunting for nested repos.
	parent := t.TempDir()
	repo := mkRepo(t, parent, "solo", "package solo\n\nfunc Solo() {}\n")

	eng := newTestEngine(t, false)
	res, err := eng.Index(context.Background(), IndexInput{ProjectPath: repo})
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if res.Mode == "segmented" {
		t.Fatalf("a single repo should not segment (Mode=%q)", res.Mode)
	}
	if len(res.Repos) != 0 {
		t.Fatalf("single repo should have no per-repo breakdown, got %d", len(res.Repos))
	}
	if res.RepoFullName != "solo" {
		t.Errorf("RepoFullName = %q, want solo", res.RepoFullName)
	}
}
