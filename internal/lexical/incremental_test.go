package lexical

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// hitIDs returns the symbol ids of a hit slice, sorted, for set comparison
// independent of BM25 score ordering.
func hitIDs(hits []Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.SymbolID)
	}
	sort.Strings(out)
	return out
}

func searchIDs(t *testing.T, ix *Index, snap, query string) []string {
	t.Helper()
	hits, err := ix.Search(snap, query, 50)
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	return hitIDs(hits)
}

// TestUpdateForSnapshotEqualsFullRebuild is the incremental-lexical keystone:
// applying UpdateForSnapshot for a touched file (delete its base docs, index the
// file's new symbols) must yield search results IDENTICAL to a full
// BuildForSnapshot of the same FINAL symbol set. Concretely, after the edit:
//   - the changed file's NEW symbol is findable,
//   - the changed file's REMOVED symbol is gone,
//   - an UNCHANGED file's symbol is still findable.
//
// Two indexes are built side by side under the same snapshot id: one via the
// incremental path, one via a from-scratch full rebuild of the merged set. Their
// hit sets for a battery of queries must match exactly.
func TestUpdateForSnapshotEqualsFullRebuild(t *testing.T) {
	snap := "snap-incr"

	// Base symbol set: two files. users/store.go has GetUserByID + DeleteUser;
	// ui/list.go has RenderListItem (the untouched file).
	base := []graph.CodeSymbol{
		{ID: "u-get", SnapshotID: snap, Name: "GetUserByID", Kind: "function",
			Signature: "func GetUserByID(id string) (*User, error)", Doc: "fetch a user by id",
			Path: "internal/users/store.go", Language: "go"},
		{ID: "u-del", SnapshotID: snap, Name: "DeleteUser", Kind: "function",
			Signature: "func DeleteUser(id string) error", Path: "internal/users/store.go", Language: "go"},
		{ID: "ui-render", SnapshotID: snap, Name: "RenderListItem", Kind: "function",
			Signature: "func RenderListItem(item Item) string", Path: "internal/ui/list.go", Language: "go"},
	}

	// The edit: users/store.go is rewritten. GetUserByID survives (same id), DeleteUser
	// is removed, and a brand-new symbol UpdateUserEmail is added. ui/list.go is
	// untouched. The FINAL merged set is therefore:
	//   GetUserByID (kept), UpdateUserEmail (new), RenderListItem (untouched).
	newForChangedFile := []graph.CodeSymbol{
		{ID: "u-get", SnapshotID: snap, Name: "GetUserByID", Kind: "function",
			Signature: "func GetUserByID(id string) (*User, error)", Doc: "fetch a user by id",
			Path: "internal/users/store.go", Language: "go"},
		{ID: "u-upd", SnapshotID: snap, Name: "UpdateUserEmail", Kind: "function",
			Signature: "func UpdateUserEmail(id, email string) error", Path: "internal/users/store.go", Language: "go"},
	}
	// Base docs to delete for the touched file (users/store.go): both its base symbols.
	removeIDs := []string{"u-get", "u-del"}

	finalMerged := []graph.CodeSymbol{
		newForChangedFile[0], // GetUserByID
		newForChangedFile[1], // UpdateUserEmail
		base[2],              // RenderListItem (untouched)
	}

	// --- Index A: incremental path (BuildForSnapshot base, then UpdateForSnapshot) ---
	dirA := filepath.Join(t.TempDir(), "incr")
	ixA, err := New(dirA)
	if err != nil {
		t.Fatalf("New(A): %v", err)
	}
	defer ixA.Close()
	if err := ixA.BuildForSnapshot(snap, base); err != nil {
		t.Fatalf("BuildForSnapshot(base): %v", err)
	}
	if err := ixA.UpdateForSnapshot(snap, removeIDs, newForChangedFile); err != nil {
		t.Fatalf("UpdateForSnapshot: %v", err)
	}

	// --- Index B: full rebuild of the final merged set (ground truth) ---
	dirB := filepath.Join(t.TempDir(), "full")
	ixB, err := New(dirB)
	if err != nil {
		t.Fatalf("New(B): %v", err)
	}
	defer ixB.Close()
	if err := ixB.BuildForSnapshot(snap, finalMerged); err != nil {
		t.Fatalf("BuildForSnapshot(final): %v", err)
	}

	// The two indexes must return identical hit sets for every probe.
	queries := []string{
		"user",        // matches GetUserByID + UpdateUserEmail (both have "user")
		"getuserbyid", // the kept symbol's whole token
		"update",      // the new symbol
		"email",       // the new symbol's name/signature
		"delete",      // the REMOVED symbol — must be gone in BOTH
		"render",      // the untouched file's symbol
		"list",        // untouched file
	}
	for _, q := range queries {
		gotA := searchIDs(t, ixA, snap, q)
		gotB := searchIDs(t, ixB, snap, q)
		if len(gotA) != len(gotB) {
			t.Fatalf("query %q: incremental hits %v != full-rebuild hits %v", q, gotA, gotB)
		}
		for i := range gotA {
			if gotA[i] != gotB[i] {
				t.Fatalf("query %q: incremental hits %v != full-rebuild hits %v", q, gotA, gotB)
			}
		}
	}

	// Explicit per-property assertions (independent of the equivalence above).
	if got := searchIDs(t, ixA, snap, "delete"); len(got) != 0 {
		t.Fatalf("removed symbol DeleteUser still findable after incremental update: %v", got)
	}
	if got := searchIDs(t, ixA, snap, "update"); !containsID(got, "u-upd") {
		t.Fatalf("new symbol UpdateUserEmail not findable after incremental update: %v", got)
	}
	if got := searchIDs(t, ixA, snap, "render"); !containsID(got, "ui-render") {
		t.Fatalf("untouched symbol RenderListItem no longer findable after incremental update: %v", got)
	}
	if got := searchIDs(t, ixA, snap, "getuserbyid"); !containsID(got, "u-get") {
		t.Fatalf("kept symbol GetUserByID no longer findable after incremental update: %v", got)
	}
}

// TestUpdateForSnapshotAddedAndDeletedFiles covers the pure-add and pure-delete
// shapes: an ADDED file (no base docs to remove, just index its symbols) and a
// DELETED file (base docs removed, no new symbols indexed). Deleting an absent
// doc id is a harmless no-op; indexing under a fresh id is an insert.
func TestUpdateForSnapshotAddedAndDeletedFiles(t *testing.T) {
	snap := "snap-adddel"
	base := []graph.CodeSymbol{
		{ID: "a-1", SnapshotID: snap, Name: "AlphaHandler", Kind: "function",
			Path: "alpha.go", Language: "go"},
		{ID: "b-1", SnapshotID: snap, Name: "BetaHandler", Kind: "function",
			Path: "beta.go", Language: "go"},
	}
	dir := filepath.Join(t.TempDir(), "adddel")
	ix, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ix.Close()
	if err := ix.BuildForSnapshot(snap, base); err != nil {
		t.Fatalf("BuildForSnapshot: %v", err)
	}

	// DELETE beta.go (remove b-1, no new symbols) and ADD gamma.go (no removals,
	// index its symbol) in one incremental update.
	added := []graph.CodeSymbol{
		{ID: "g-1", SnapshotID: snap, Name: "GammaHandler", Kind: "function",
			Path: "gamma.go", Language: "go"},
	}
	if err := ix.UpdateForSnapshot(snap, []string{"b-1"}, added); err != nil {
		t.Fatalf("UpdateForSnapshot: %v", err)
	}

	if got := searchIDs(t, ix, snap, "gamma"); !containsID(got, "g-1") {
		t.Fatalf("added symbol GammaHandler not findable: %v", got)
	}
	if got := searchIDs(t, ix, snap, "beta"); len(got) != 0 {
		t.Fatalf("deleted symbol BetaHandler still findable: %v", got)
	}
	if got := searchIDs(t, ix, snap, "alpha"); !containsID(got, "a-1") {
		t.Fatalf("untouched symbol AlphaHandler no longer findable: %v", got)
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
