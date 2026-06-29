package index

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/store"
)

// snapshotSymbolNames loads a snapshot's symbols and returns whether `name` is
// among them — the cheapest end-to-end "did the delta index the edit" probe.
func snapshotHasSymbol(t *testing.T, drv store.StorageDriver, snapshotID, name string) bool {
	t.Helper()
	syms, err := drv.ListSymbols(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	for _, s := range syms {
		if s.Name == name {
			return true
		}
	}
	return false
}

func appendToFile(t *testing.T, path, extra string) {
	t.Helper()
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(cur, []byte(extra)...), 0o644); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}

// TestDeltaIndexesUncommittedEdit is the keystone regression: an AI agent edits a
// file in the working tree WITHOUT committing, then re-indexes. The run must take
// the DELTA path (not noop) and the new symbol must be in the snapshot. A third
// run with no further change must be a GENUINE noop.
func TestDeltaIndexesUncommittedEdit(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	repo := writeGoRepo(t)
	gitCmd(t, git, repo, "init", "-q")
	gitCmd(t, git, repo, "add", ".")
	gitCmd(t, git, repo, "-c", "user.name=Atlas Test", "-c", "user.email=atlas@example.invalid", "commit", "-q", "--no-gpg-sign", "-m", "init")

	drv := openTestStore(t)

	// 1) Full index of the committed tree.
	first, firstStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if firstStats.Mode != "full" {
		t.Fatalf("first Run mode = %q, want full", firstStats.Mode)
	}
	if snapshotHasSymbol(t, drv, first.ID, "ZzMarkerFix") {
		t.Fatal("ZzMarkerFix present before it was written")
	}

	// 2) Append a new function to svc.go WITHOUT committing.
	appendToFile(t, filepath.Join(repo, "svc.go"), "\nfunc ZzMarkerFix() int { return 1 }\n")

	second, secondStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if secondStats.Mode != "delta" {
		t.Fatalf("second Run mode = %q, want delta (uncommitted edit must be detected)", secondStats.Mode)
	}
	if secondStats.ChangedFiles != 1 {
		t.Fatalf("second Run ChangedFiles = %d, want 1", secondStats.ChangedFiles)
	}
	// The uncommitted edit leaves HEAD unchanged, so the snapshot for this
	// (repo, commit) is refreshed in place — SaveSnapshot's idempotency reuses the
	// base id and rebuilds its child rows. The proof the delta WORKED is that the
	// new symbol is now in that snapshot (it was absent at step 1).
	if !snapshotHasSymbol(t, drv, second.ID, "ZzMarkerFix") {
		t.Fatal("delta did NOT index the uncommitted edit: ZzMarkerFix missing from snapshot")
	}

	// 3) No further change -> genuine noop (tree matches the base snapshot).
	third, thirdStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("third Run: %v", err)
	}
	if thirdStats.Mode != "noop" {
		t.Fatalf("third Run mode = %q, want noop (no change since delta)", thirdStats.Mode)
	}
	if third.ID != second.ID {
		t.Fatalf("noop returned snapshot %q, want the delta snapshot %q", third.ID, second.ID)
	}
	// The noop snapshot still carries the indexed edit.
	if !snapshotHasSymbol(t, drv, third.ID, "ZzMarkerFix") {
		t.Fatal("noop snapshot lost ZzMarkerFix")
	}
}

// TestDeltaIndexesUncommittedEditNonGit proves the same edit-detection works in a
// directory with NO git history at all (the hash-compare is the backstop).
func TestDeltaIndexesUncommittedEditNonGit(t *testing.T) {
	ctx := context.Background()
	repo := writeGoRepo(t) // plain temp dir, never git-init'd
	drv := openTestStore(t)

	_, firstStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if firstStats.Mode != "full" {
		t.Fatalf("first Run mode = %q, want full", firstStats.Mode)
	}

	appendToFile(t, filepath.Join(repo, "svc.go"), "\nfunc ZzNonGitMarker() int { return 7 }\n")

	second, secondStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if secondStats.Mode != "delta" {
		t.Fatalf("non-git second Run mode = %q, want delta", secondStats.Mode)
	}
	if !snapshotHasSymbol(t, drv, second.ID, "ZzNonGitMarker") {
		t.Fatal("non-git delta did NOT index the edit: ZzNonGitMarker missing")
	}

	// And a follow-up no-change run noops.
	_, thirdStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("third Run: %v", err)
	}
	if thirdStats.Mode != "noop" {
		t.Fatalf("non-git third Run mode = %q, want noop", thirdStats.Mode)
	}
}

// TestDeltaReflectsNewFile asserts an untracked NEW file is indexed by the delta.
func TestDeltaReflectsNewFile(t *testing.T) {
	ctx := context.Background()
	repo := writeGoRepo(t)
	drv := openTestStore(t)

	first, _, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if snapshotHasSymbol(t, drv, first.ID, "ZzBrandNew") {
		t.Fatal("ZzBrandNew present before its file existed")
	}

	newSrc := "package svc\n\nfunc ZzBrandNew() string { return \"new\" }\n"
	if err := os.WriteFile(filepath.Join(repo, "extra.go"), []byte(newSrc), 0o644); err != nil {
		t.Fatalf("write extra.go: %v", err)
	}

	second, secondStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if secondStats.Mode != "delta" {
		t.Fatalf("new-file Run mode = %q, want delta", secondStats.Mode)
	}
	if secondStats.Files != 2 {
		t.Fatalf("new-file Run Files = %d, want 2", secondStats.Files)
	}
	if !snapshotHasSymbol(t, drv, second.ID, "ZzBrandNew") {
		t.Fatal("delta did NOT index the new file: ZzBrandNew missing")
	}
}

// TestDeltaReflectsDeletion asserts a deleted file's symbols are dropped from the
// snapshot and FileCount shrinks.
func TestDeltaReflectsDeletion(t *testing.T) {
	ctx := context.Background()
	repo := writeGoRepo(t)
	// Add a second file so we can delete one and still have a non-empty tree.
	extra := "package svc\n\nfunc ZzToDelete() int { return 0 }\n"
	if err := os.WriteFile(filepath.Join(repo, "gone.go"), []byte(extra), 0o644); err != nil {
		t.Fatalf("write gone.go: %v", err)
	}
	drv := openTestStore(t)

	first, firstStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if firstStats.Files != 2 {
		t.Fatalf("first Run Files = %d, want 2", firstStats.Files)
	}
	if !snapshotHasSymbol(t, drv, first.ID, "ZzToDelete") {
		t.Fatal("ZzToDelete missing from base snapshot")
	}

	if err := os.Remove(filepath.Join(repo, "gone.go")); err != nil {
		t.Fatalf("remove gone.go: %v", err)
	}

	second, secondStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if secondStats.Mode != "delta" {
		t.Fatalf("deletion Run mode = %q, want delta", secondStats.Mode)
	}
	if secondStats.Files != 1 {
		t.Fatalf("deletion Run Files = %d, want 1", secondStats.Files)
	}
	if snapshotHasSymbol(t, drv, second.ID, "ZzToDelete") {
		t.Fatal("deletion delta still carries ZzToDelete (stale symbol not dropped)")
	}
}

// TestCommittedDeltaStillWorks is the existing committed-commit delta path: index
// the committed tree, commit a new file, re-index. The delta must pick up the new
// committed symbol (the working tree reflects the committed change, so the
// hash-compare detects it just as the old commit-diff did).
func TestCommittedDeltaStillWorks(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	repo := writeGoRepo(t)
	gitCmd(t, git, repo, "init", "-q")
	gitCmd(t, git, repo, "add", ".")
	gitCmd(t, git, repo, "-c", "user.name=Atlas Test", "-c", "user.email=atlas@example.invalid", "commit", "-q", "--no-gpg-sign", "-m", "init")

	drv := openTestStore(t)
	if _, _, err := Run(ctx, drv, nil, "", "svc", repo, Options{}); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Add + COMMIT a new file.
	committedSrc := "package svc\n\nfunc ZzCommitted() int { return 42 }\n"
	if err := os.WriteFile(filepath.Join(repo, "committed.go"), []byte(committedSrc), 0o644); err != nil {
		t.Fatalf("write committed.go: %v", err)
	}
	gitCmd(t, git, repo, "add", ".")
	gitCmd(t, git, repo, "-c", "user.name=Atlas Test", "-c", "user.email=atlas@example.invalid", "commit", "-q", "--no-gpg-sign", "-m", "add committed.go")

	second, secondStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if secondStats.Mode != "delta" {
		t.Fatalf("committed-delta Run mode = %q, want delta", secondStats.Mode)
	}
	if !snapshotHasSymbol(t, drv, second.ID, "ZzCommitted") {
		t.Fatal("committed delta did NOT index ZzCommitted")
	}
}

// TestDeltaPreservesGoTypesEnrichment asserts that after a delta over a Go change,
// the go/types-grounded edges (precise recv_source + type-use reference edges) are
// still produced — precision is not lost on the incremental path.
func TestDeltaPreservesGoTypesEnrichment(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// A package where a method call gives go/types a real receiver type and a
	// type-use reference edge to assert on.
	src := `package svc

type Greeter struct{ name string }

func (g Greeter) Hello() string { return g.name }

func Run() string {
	g := Greeter{name: "x"}
	return g.Hello()
}
`
	if err := os.WriteFile(filepath.Join(dir, "svc.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write svc.go: %v", err)
	}
	// A go.mod so go/types can load the package as a real module.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module svc\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	drv := openTestStore(t)

	// Full index first to capture the baseline go/types edge shape.
	full, _, err := Run(ctx, drv, nil, "", "svc", dir, Options{})
	if err != nil {
		t.Fatalf("full Run: %v", err)
	}
	fullGoTypes := goTypesEdgeCount(t, drv, full.ID)
	if fullGoTypes == 0 {
		t.Skip("go/types produced no grounded edges in this environment; nothing to compare")
	}

	// Edit the Go file (uncommitted) -> delta path re-runs go/types over the merge.
	appendToFile(t, filepath.Join(dir, "svc.go"), "\nfunc ZzExtra() string { g := Greeter{}; return g.Hello() }\n")

	delta, deltaStats, err := Run(ctx, drv, nil, "", "svc", dir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta Run mode = %q, want delta", deltaStats.Mode)
	}
	deltaGoTypes := goTypesEdgeCount(t, drv, delta.ID)
	if deltaGoTypes < fullGoTypes {
		t.Fatalf("delta dropped go/types precision: grounded edges %d < baseline %d", deltaGoTypes, fullGoTypes)
	}
}

// goTypesEdgeCount counts edges that carry go/types grounding: either a precise
// recv_source==go_types on a call edge, or a type-use reference edge sourced from
// go_types. These exist only when enrichGoTypes ran over the snapshot.
func goTypesEdgeCount(t *testing.T, drv store.StorageDriver, snapshotID string) int {
	t.Helper()
	edges, err := drv.ListEdges(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	n := 0
	for _, e := range edges {
		if e.Metadata == nil {
			continue
		}
		if src, _ := e.Metadata["recv_source"].(string); src == "go_types" {
			n++
			continue
		}
		if e.Kind == graph.EdgeReferences {
			if src, _ := e.Metadata["source"].(string); src == "go_types" {
				n++
			}
		}
	}
	return n
}

// TestScanWorkTreeClassifies is a focused unit test of the change classifier: a
// base snapshot's stored hashes are compared against a freshly-written tree and
// the changed/added/deleted sets must be exact.
func TestScanWorkTreeClassifies(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("keep.go", "package svc\n\nfunc Keep() {}\n")
	write("mod.go", "package svc\n\nfunc Mod() int { return 1 }\n")
	write("gone.go", "package svc\n\nfunc Gone() {}\n")

	drv := openTestStore(t)
	base, _, err := Run(ctx, drv, nil, "", "svc", dir, Options{})
	if err != nil {
		t.Fatalf("base Run: %v", err)
	}

	// Mutate the tree: modify mod.go, delete gone.go, add new.go; keep.go untouched.
	write("mod.go", "package svc\n\nfunc Mod() int { return 2 }\n")
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatalf("remove gone.go: %v", err)
	}
	write("new.go", "package svc\n\nfunc New() {}\n")

	absRoot, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	scan, err := scanWorkTree(ctx, drv, base.ID, absRoot)
	if err != nil {
		t.Fatalf("scanWorkTree: %v", err)
	}

	assertSet := func(label string, got map[string]struct{}, want ...string) {
		if len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", label, keys(got), want)
		}
		for _, w := range want {
			if _, ok := got[w]; !ok {
				t.Fatalf("%s missing %q (got %v)", label, w, keys(got))
			}
		}
	}
	assertSet("changed", scan.changed, "mod.go")
	assertSet("added", scan.added, "new.go")
	assertSet("deleted", scan.deleted, "gone.go")
	if scan.noChanges() {
		t.Fatal("scan reported noChanges() with a modified/added/deleted tree")
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestScanWorkTreeNoChanges asserts a tree identical to the base snapshot yields
// an all-empty classification (the genuine-noop precondition).
func TestScanWorkTreeNoChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "svc.go"), []byte("package svc\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	drv := openTestStore(t)
	base, _, err := Run(ctx, drv, nil, "", "svc", dir, Options{})
	if err != nil {
		t.Fatalf("base Run: %v", err)
	}
	absRoot, _ := filepath.Abs(dir)
	scan, err := scanWorkTree(ctx, drv, base.ID, absRoot)
	if err != nil {
		t.Fatalf("scanWorkTree: %v", err)
	}
	if !scan.noChanges() {
		t.Fatalf("unchanged tree reported changes: changed=%v added=%v deleted=%v",
			keys(scan.changed), keys(scan.added), keys(scan.deleted))
	}
}

// sanity: ensure the package's File hash field is what the classifier compares on
// (guards against a future schema change silently dropping the hash).
func TestBaseFileHashesArePersisted(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "svc.go"), []byte("package svc\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	drv := openTestStore(t)
	base, _, err := Run(ctx, drv, nil, "", "svc", dir, Options{})
	if err != nil {
		t.Fatalf("base Run: %v", err)
	}
	files, err := drv.ListFiles(ctx, base.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files persisted")
	}
	for _, f := range files {
		if strings.TrimSpace(f.Hash) == "" {
			t.Fatalf("file %q persisted with empty hash; change detection would break", f.Path)
		}
	}
}
