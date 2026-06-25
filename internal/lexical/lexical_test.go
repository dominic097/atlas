package lexical

import (
	"path/filepath"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

func contains(toks []string, want string) bool {
	for _, t := range toks {
		if t == want {
			return true
		}
	}
	return false
}

func TestTokenizeIdentifier(t *testing.T) {
	cases := []struct {
		in   string
		want []string // tokens that MUST be present
	}{
		{"GetUserByID", []string{"getuserbyid", "get", "user", "by", "id"}},
		{"user_profile", []string{"user_profile", "user", "profile"}},
		{"render-list-item", []string{"render-list-item", "render", "list", "item"}},
		{"HTTPServer", []string{"http", "server"}},
	}
	for _, c := range cases {
		got := TokenizeIdentifier(c.in)
		for _, w := range c.want {
			if !contains(got, w) {
				t.Errorf("TokenizeIdentifier(%q) = %v; missing %q", c.in, got, w)
			}
		}
	}

	// Explicit contract assertion: "user" must be derivable from "GetUserByID".
	if !contains(TokenizeIdentifier("GetUserByID"), "user") {
		t.Fatalf("TokenizeIdentifier(GetUserByID) must contain \"user\"")
	}
	// The original whole token (lowercased) must be kept.
	if !contains(TokenizeIdentifier("GetUserByID"), "getuserbyid") {
		t.Fatalf("TokenizeIdentifier must keep the original whole token")
	}
}

func TestBuildAndSearchRoundtrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lexidx")
	ix, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ix.Close()

	snap := "snap-1"
	syms := []graph.CodeSymbol{
		{
			ID:         "sym-1",
			SnapshotID: snap,
			Name:       "GetUserByID",
			Kind:       "function",
			Signature:  "func GetUserByID(id string) (*User, error)",
			Doc:        "fetches a user by primary key",
			Path:       "internal/users/store.go",
			Language:   "go",
		},
		{
			ID:         "sym-2",
			SnapshotID: snap,
			Name:       "RenderListItem",
			Kind:       "function",
			Signature:  "func RenderListItem(item Item) string",
			Path:       "internal/ui/list.go",
			Language:   "go",
		},
		{
			// Different snapshot — must NOT leak into snap-1 search.
			ID:         "sym-3",
			SnapshotID: "snap-2",
			Name:       "GetUserByID",
			Kind:       "function",
			Path:       "other/store.go",
			Language:   "go",
		},
	}
	if err := ix.BuildForSnapshot(snap, syms[:2]); err != nil {
		t.Fatalf("BuildForSnapshot(snap-1): %v", err)
	}
	if err := ix.BuildForSnapshot("snap-2", syms[2:]); err != nil {
		t.Fatalf("BuildForSnapshot(snap-2): %v", err)
	}

	// "user" should match GetUserByID via the code-aware analyzer.
	hits, err := ix.Search(snap, "user", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit for \"user\"")
	}
	foundSym1 := false
	for _, h := range hits {
		if h.SymbolID == "sym-1" {
			foundSym1 = true
		}
		if h.SymbolID == "sym-3" {
			t.Errorf("snapshot scope leaked: got sym-3 (snap-2) in snap-1 search")
		}
	}
	if !foundSym1 {
		t.Errorf("expected sym-1 (GetUserByID) in hits for \"user\", got %+v", hits)
	}

	// Whole-token search must also work.
	whole, err := ix.Search(snap, "getuserbyid", 10)
	if err != nil {
		t.Fatalf("Search(getuserbyid): %v", err)
	}
	if len(whole) == 0 {
		t.Fatalf("expected a hit for the whole token \"getuserbyid\"")
	}

	// Reopen the persistent index and search again (durability).
	if err := ix.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ix2, err := New(dir)
	if err != nil {
		t.Fatalf("reopen New: %v", err)
	}
	defer ix2.Close()
	reopened, err := ix2.Search(snap, "user", 10)
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
	if len(reopened) == 0 {
		t.Fatalf("expected hits after reopening the persistent index")
	}
}
