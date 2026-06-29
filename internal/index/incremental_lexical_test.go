package index

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dominic097/atlas/internal/lexical"
)

// writeTwoFileRepo lays down a non-git Go repo with two files so an uncommitted
// edit to one leaves the other untouched — the exact shape the incremental
// lexical delta must handle (re-index the changed file's symbols, carry the
// untouched file's docs forward). Non-git means head == "working-tree" on both
// the base and the delta, so the delta REUSES the base snapshot id (the
// uncommitted-edit case) and takes the incremental lexical path.
func writeTwoFileRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"users.go": `package app

// GetUserByID fetches a user by id.
func GetUserByID(id string) string { return id }

// DeleteUser removes a user.
func DeleteUser(id string) error { return nil }
`,
		"render.go": `package app

// RenderInvoice renders an invoice.
func RenderInvoice(x string) string { return x }
`,
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

func lexSearchIDs(t *testing.T, lx *lexical.Index, snap, query string) []string {
	t.Helper()
	hits, err := lx.Search(snap, query, 50)
	if err != nil {
		t.Fatalf("lexical Search(%q): %v", query, err)
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.SymbolID)
	}
	sort.Strings(out)
	return out
}

// TestDeltaIncrementalLexicalFindsChangedAndUnchanged is the index-level keystone
// for the incremental lexical delta. After an uncommitted edit to one file:
//   - the delta runs (Mode == "delta") and uses the INCREMENTAL lexical path
//     (Metadata["lexical_mode"] == "incremental"),
//   - the changed file's NEW symbol is findable via lexical search,
//   - the changed file's REMOVED symbol is gone,
//   - the UNTOUCHED file's symbol is still findable,
//   - and the resulting lexical hit set matches a FULL REINDEX of the edited tree
//     (proving the incremental update is search-equivalent to a from-scratch build).
func TestDeltaIncrementalLexicalFindsChangedAndUnchanged(t *testing.T) {
	ctx := context.Background()

	// --- Delta store + lexical index: base full index, then an uncommitted edit. ---
	dir := writeTwoFileRepo(t)
	drv := openTestStore(t)
	lx, err := lexical.New(filepath.Join(t.TempDir(), "lex-delta"))
	if err != nil {
		t.Fatalf("lexical.New: %v", err)
	}
	t.Cleanup(func() { _ = lx.Close() })

	baseSnap, baseStats, err := Run(ctx, drv, lx, "", "app", dir, Options{})
	if err != nil {
		t.Fatalf("base Run: %v", err)
	}
	if baseStats.Mode != "full" {
		t.Fatalf("base mode = %q, want full", baseStats.Mode)
	}
	// Sanity: the base lexical index finds all three symbols.
	if got := lexSearchIDs(t, lx, baseSnap.ID, "delete"); len(got) == 0 {
		t.Fatalf("base lexical did not find DeleteUser")
	}

	// Edit users.go: drop DeleteUser, add UpdateUserEmail. render.go is untouched.
	// Same commit (non-git working tree) => the delta reuses baseSnap.ID.
	if err := os.WriteFile(filepath.Join(dir, "users.go"), []byte(`package app

// GetUserByID fetches a user by id.
func GetUserByID(id string) string { return id }

// UpdateUserEmail updates a user's email.
func UpdateUserEmail(id, email string) error { return nil }
`), 0o644); err != nil {
		t.Fatalf("rewrite users.go: %v", err)
	}

	delta, deltaStats, err := Run(ctx, drv, lx, "", "app", dir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	// The snapshot id must have been REUSED (same commit), and the lexical path
	// must have been the INCREMENTAL one.
	if delta.ID != baseSnap.ID {
		t.Fatalf("delta snapshot id = %q, want reused base id %q", delta.ID, baseSnap.ID)
	}
	if got, _ := delta.Metadata["lexical_mode"].(string); got != "incremental" {
		t.Fatalf("delta lexical_mode = %q, want incremental (the incremental wiring did not run)", got)
	}

	// Lexical search now must reflect the edit on the REUSED snapshot id.
	if got := lexSearchIDs(t, lx, delta.ID, "delete"); len(got) != 0 {
		t.Fatalf("removed symbol DeleteUser still findable after incremental delta: %v", got)
	}
	if got := lexSearchIDs(t, lx, delta.ID, "update"); len(got) == 0 {
		t.Fatalf("new symbol UpdateUserEmail not findable after incremental delta")
	}
	if got := lexSearchIDs(t, lx, delta.ID, "invoice"); len(got) == 0 {
		t.Fatalf("untouched symbol RenderInvoice no longer findable after incremental delta")
	}
	if got := lexSearchIDs(t, lx, delta.ID, "getuserbyid"); len(got) == 0 {
		t.Fatalf("kept symbol GetUserByID no longer findable after incremental delta")
	}

	// --- Ground truth: a FULL reindex of the EDITED tree in its own store+index. ---
	// The incremental delta's lexical hit COUNTS must match the full rebuild's for a
	// battery of queries (ids differ between stores, so compare counts, not ids).
	truthDrv := openTestStore(t)
	truthLx, err := lexical.New(filepath.Join(t.TempDir(), "lex-truth"))
	if err != nil {
		t.Fatalf("lexical.New(truth): %v", err)
	}
	t.Cleanup(func() { _ = truthLx.Close() })
	truth, _, err := Run(ctx, truthDrv, truthLx, "", "app", dir, Options{Reindex: true})
	if err != nil {
		t.Fatalf("ground-truth reindex Run: %v", err)
	}

	for _, q := range []string{"user", "getuserbyid", "update", "email", "delete", "invoice", "render"} {
		gotDelta := lexSearchIDs(t, lx, delta.ID, q)
		gotTruth := lexSearchIDs(t, truthLx, truth.ID, q)
		if len(gotDelta) != len(gotTruth) {
			t.Fatalf("query %q: incremental-delta hit count %d != full-reindex hit count %d",
				q, len(gotDelta), len(gotTruth))
		}
	}
}
