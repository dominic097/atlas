package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func matcherFrom(patterns ...string) *ignoreMatcher {
	var rules []ignoreRule
	for _, p := range patterns {
		if r, ok := compileIgnorePattern(p); ok {
			rules = append(rules, r)
		}
	}
	return &ignoreMatcher{rules: rules}
}

func TestIgnoreMatcherSemantics(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		path     string
		isDir    bool
		want     bool
	}{
		{"suffix glob at depth", []string{"*.log"}, "a/b/c.log", false, true},
		{"suffix glob miss", []string{"*.log"}, "a/b/c.txt", false, false},
		{"basename at any depth", []string{"secret.txt"}, "deep/dir/secret.txt", false, true},
		{"dir-only matches dir", []string{"build/"}, "pkg/build", true, true},
		{"dir-only ignores file of same name", []string{"build/"}, "pkg/build", false, false},
		{"anchored root only", []string{"/top.md"}, "top.md", false, true},
		{"anchored not at depth", []string{"/top.md"}, "sub/top.md", false, false},
		{"embedded slash anchors", []string{"cmd/python-server"}, "cmd/python-server", true, true},
		{"embedded slash not at depth", []string{"cmd/python-server"}, "x/cmd/python-server", true, false},
		{"comment is no rule", []string{"# *.log"}, "a.log", false, false},
		{"blank is no rule", []string{"   "}, "a.log", false, false},
		{"negation re-includes", []string{"*.log", "!keep.log"}, "keep.log", false, false},
		{"negation leaves others", []string{"*.log", "!keep.log"}, "drop.log", false, true},
		{"doublestar crosses dirs", []string{"**/temp"}, "a/b/temp", true, true},
		{"trailing doublestar covers subtree", []string{"docs/**"}, "docs/a/b.pptx", false, true},
		// A dir-only rule matches the directory (the walk then SkipDirs it, so its
		// descendants are never visited); it does not match a descendant in isolation.
		{"dir-only matches the dir itself", []string{"node_modules/"}, "node_modules", true, true},
		// A non-dir-only name covers descendants directly (no SkipDir needed).
		{"plain name covers descendants", []string{"node_modules"}, "node_modules/pkg/index.js", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matcherFrom(tc.patterns...).ignored(tc.path, tc.isDir); got != tc.want {
				t.Errorf("ignored(%q, dir=%v) with %v = %v, want %v", tc.path, tc.isDir, tc.patterns, got, tc.want)
			}
		})
	}
}

func TestNilIgnoreMatcherIsNoop(t *testing.T) {
	var m *ignoreMatcher
	if m.ignored("anything.go", false) {
		t.Error("nil matcher must ignore nothing")
	}
}

// TestRunAtlasignoreNonGitFolder proves .atlasignore works in a folder that is NOT
// a git repo (the documents-directory case) and without RespectGitignore.
func TestRunAtlasignoreNonGitFolder(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir() // deliberately NOT a git repo
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.go", "package docs\n\nfunc Keep() {}\n")
	write("drafts/d.go", "package drafts\n\nfunc D() {}\n")
	write("x.skip.go", "package docs\n\nfunc X() {}\n")
	write(".atlasignore", "drafts/\n*.skip.go\n")

	_, withIg, err := Run(ctx, openTestStore(t), nil, "", "docs", dir, Options{})
	if err != nil {
		t.Fatalf("Run(with .atlasignore): %v", err)
	}
	if err := os.Remove(filepath.Join(dir, ".atlasignore")); err != nil {
		t.Fatal(err)
	}
	_, without, err := Run(ctx, openTestStore(t), nil, "", "docs", dir, Options{})
	if err != nil {
		t.Fatalf("Run(without .atlasignore): %v", err)
	}
	// Removing .atlasignore must surface exactly the two ignored sources
	// (drafts/d.go + x.skip.go); with it present they are pruned even though this
	// is not a git repo and RespectGitignore is off.
	if without.Files-withIg.Files != 2 {
		t.Errorf(".atlasignore should prune exactly 2 files in a non-git folder: with=%d without=%d", withIg.Files, without.Files)
	}
}
