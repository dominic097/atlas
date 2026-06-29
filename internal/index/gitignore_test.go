package index

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitIgnoredPathsPrunesIgnored(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "ignored/\n*.gen.go\n")
	write("keep.go", "package p\n")
	write("ignored/a.go", "package q\n") // whole dir ignored
	write("x.gen.go", "package p\n")      // single file ignored
	gitCmd(t, git, dir, "init", "-q")

	set := gitIgnoredPaths(context.Background(), dir)
	clean := func(rel string) string { return filepath.Clean(filepath.Join(dir, rel)) }
	if _, ok := set[clean("ignored")]; !ok {
		t.Errorf("ignored/ (fully-ignored dir) should be pruned, set=%v", set)
	}
	if _, ok := set[clean("x.gen.go")]; !ok {
		t.Errorf("x.gen.go (ignored file) should be pruned, set=%v", set)
	}
	if _, ok := set[clean("keep.go")]; ok {
		t.Errorf("keep.go (not ignored) must NOT be pruned, set=%v", set)
	}
}

func TestRunRespectsGitignore(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	repo := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.go", "package svc\n\nfunc Keep() {}\n")
	write(".gitignore", "build/\n*.gen.go\n")
	gitCmd(t, git, repo, "init", "-q")
	gitCmd(t, git, repo, "add", "keep.go", ".gitignore")
	gitCmd(t, git, repo, "-c", "user.name=Atlas Test", "-c", "user.email=atlas@example.invalid", "commit", "-q", "--no-gpg-sign", "-m", "init")
	// Untracked + git-ignored: a whole dir and a glob-matched file. git does not
	// ignore tracked paths, so these must be untracked to count as ignored.
	write("build/gen.go", "package gen\n\nfunc Gen() {}\n")
	write("x.gen.go", "package svc\n\nfunc X() {}\n")

	_, statsOn, err := Run(ctx, openTestStore(t), nil, "", "svc", repo, Options{RespectGitignore: true})
	if err != nil {
		t.Fatalf("Run(gitignore on): %v", err)
	}
	_, statsOff, err := Run(ctx, openTestStore(t), nil, "", "svc", repo, Options{RespectGitignore: false})
	if err != nil {
		t.Fatalf("Run(gitignore off): %v", err)
	}
	// Turning gitignore ON must prune EXACTLY the two ignored sources (build/gen.go
	// and x.gen.go) — no more (don't drop keep.go), no fewer (don't index the junk).
	if statsOff.Files-statsOn.Files != 2 {
		t.Errorf("gitignore should prune exactly 2 files (build/gen.go + x.gen.go): on=%d off=%d", statsOn.Files, statsOff.Files)
	}
	if statsOn.Files < 1 {
		t.Errorf("keep.go must still be indexed with gitignore on, got Files=%d", statsOn.Files)
	}
}
