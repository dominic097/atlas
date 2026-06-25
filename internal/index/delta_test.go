package index

import (
	"reflect"
	"sort"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// names extracts the symbol names (sorted) for stable assertions.
func symNames(syms []graph.CodeSymbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}

func TestKeepBaseSymbols_DropsTouchedFiles(t *testing.T) {
	base := []graph.CodeSymbol{
		{Name: "A", Path: "a.go"}, // a.go is changed -> dropped
		{Name: "B", Path: "a.go"}, // a.go is changed -> dropped
		{Name: "K", Path: "keep.go"},
		{Name: "G", Path: "gone.go"}, // gone.go is deleted -> dropped
	}
	// touched = changed{a.go} ∪ deleted{gone.go}
	touched := makeSet([]string{"a.go"}, []string{"gone.go"})

	got := keepBaseSymbols(base, touched)
	if want := []string{"K"}; !reflect.DeepEqual(symNames(got), want) {
		t.Fatalf("kept symbols = %v, want %v", symNames(got), want)
	}
}

func TestKeepBaseSymbols_PathCanonicalization(t *testing.T) {
	// A base symbol stored with an OS-style backslash path must still match a
	// touched set built from slash paths (canonicalPath normalizes both).
	base := []graph.CodeSymbol{
		{Name: "Win", Path: `pkg\sub\file.go`},
		{Name: "Other", Path: "pkg/other.go"},
	}
	touched := makeSet([]string{"pkg/sub/file.go"})

	got := symNames(keepBaseSymbols(base, touched))
	if want := []string{"Other"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kept symbols = %v, want %v (backslash path should have been dropped)", got, want)
	}
}

func TestKeepBaseEdges_DropsByFromFile(t *testing.T) {
	base := []graph.DependencyEdge{
		{FromFile: "a.go", FromSymbol: "A", ToRef: "X", Kind: graph.EdgeCalls}, // dropped (a.go changed)
		{FromFile: "keep.go", FromSymbol: "K", ToRef: "A", Kind: graph.EdgeCalls},
		{FromFile: "gone.go", FromSymbol: "G", ToRef: "K", Kind: graph.EdgeCalls}, // dropped (deleted)
	}
	touched := makeSet([]string{"a.go"}, []string{"gone.go"})

	got := keepBaseEdges(base, touched)
	if len(got) != 1 || got[0].FromFile != "keep.go" {
		t.Fatalf("kept edges = %+v, want exactly the keep.go edge", got)
	}
}

func TestDropTypeUseRefs_RemovesGoTypesRefsKeepsRest(t *testing.T) {
	in := []graph.DependencyEdge{
		{FromFile: "a.go", Kind: graph.EdgeCalls, ToRef: "Do"},
		{FromFile: "a.go", Kind: graph.EdgeReferences, ToRef: "Foo", Metadata: graph.JSONBMap{"source": "go_types"}}, // dropped
		{FromFile: "b.go", Kind: graph.EdgeReferences, ToRef: "Bar", Metadata: graph.JSONBMap{"source": "go_types"}}, // dropped
		{FromFile: "c.go", Kind: graph.EdgeReferences, ToRef: "Baz", Metadata: graph.JSONBMap{"source": "other"}},    // kept (not go_types)
		{FromFile: "d.go", Kind: graph.EdgeImports, ToRef: "fmt"},
	}
	got := dropTypeUseRefs(in)
	if len(got) != 3 {
		t.Fatalf("dropTypeUseRefs kept %d edges, want 3 (both go_types references removed)", len(got))
	}
	for _, e := range got {
		if e.Kind == graph.EdgeReferences {
			if src, _ := e.Metadata["source"].(string); src == "go_types" {
				t.Fatalf("a go_types reference edge survived: %+v", e)
			}
		}
	}
}

func TestKeepBaseRoutes_DropsByHandlerFile(t *testing.T) {
	base := []graph.Route{
		{Method: "GET", PathPattern: "/a", HandlerFile: "a.go", Role: "producer"}, // dropped
		{Method: "GET", PathPattern: "/k", HandlerFile: "keep.go", Role: "producer"},
		{Method: "POST", PathPattern: "/g", HandlerFile: "gone.go", Role: "consumer"}, // dropped
	}
	touched := makeSet([]string{"a.go"}, []string{"gone.go"})

	got := keepBaseRoutes(base, touched)
	if len(got) != 1 || got[0].HandlerFile != "keep.go" {
		t.Fatalf("kept routes = %+v, want exactly the keep.go route", got)
	}
}

func TestKeepBase_EmptyTouchedKeepsAll(t *testing.T) {
	syms := []graph.CodeSymbol{{Name: "A", Path: "a.go"}, {Name: "B", Path: "b.go"}}
	edges := []graph.DependencyEdge{{FromFile: "a.go"}, {FromFile: "b.go"}}
	rts := []graph.Route{{HandlerFile: "a.go"}, {HandlerFile: "b.go"}}
	empty := map[string]struct{}{}

	if got := keepBaseSymbols(syms, empty); len(got) != 2 {
		t.Fatalf("empty touched dropped symbols: got %d, want 2", len(got))
	}
	if got := keepBaseEdges(edges, empty); len(got) != 2 {
		t.Fatalf("empty touched dropped edges: got %d, want 2", len(got))
	}
	if got := keepBaseRoutes(rts, empty); len(got) != 2 {
		t.Fatalf("empty touched dropped routes: got %d, want 2", len(got))
	}
}

func TestParseNameStatusZ(t *testing.T) {
	// A flat NUL stream: status, path, status, path, ...
	// A added, M modified, D deleted, R/C handled like changed.
	raw := "A\x00added.go\x00M\x00mod.go\x00D\x00del.go\x00R\x00renamed.go\x00C\x00copied.go\x00"
	changed, deleted := parseNameStatusZ([]byte(raw))

	sort.Strings(changed)
	wantChanged := []string{"added.go", "copied.go", "mod.go", "renamed.go"}
	if !reflect.DeepEqual(changed, wantChanged) {
		t.Fatalf("changed = %v, want %v", changed, wantChanged)
	}
	if !reflect.DeepEqual(deleted, []string{"del.go"}) {
		t.Fatalf("deleted = %v, want [del.go]", deleted)
	}
}

func TestParseNameStatusZ_Empty(t *testing.T) {
	changed, deleted := parseNameStatusZ([]byte(""))
	if len(changed) != 0 || len(deleted) != 0 {
		t.Fatalf("empty diff produced changed=%v deleted=%v", changed, deleted)
	}
}

func TestMakeSet_CanonicalizesAndUnions(t *testing.T) {
	set := makeSet([]string{" a.go ", `b\c.go`}, []string{"d.go", ""})
	want := map[string]struct{}{"a.go": {}, "b/c.go": {}, "d.go": {}}
	if !reflect.DeepEqual(set, want) {
		t.Fatalf("makeSet = %v, want %v", set, want)
	}
}
