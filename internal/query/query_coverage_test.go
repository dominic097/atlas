package query

import (
	"reflect"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// coveredGraph builds a tiny test-coverage graph:
//
//	TestFoo (foo_test.go) calls Foo
//	Foo     (foo.go)      calls helper
//	helper  (foo.go)      is the leaf
//
// So tests_for_symbol(Foo) = [TestFoo]; symbols_for_test(TestFoo) = [Foo helper].
func coveredGraph() ([]graph.CodeSymbol, []graph.DependencyEdge) {
	syms := []graph.CodeSymbol{
		{ID: "t", Path: "foo_test.go", Language: "go", Kind: "function", Name: "TestFoo", StartLine: 1, EndLine: 3},
		{ID: "f", Path: "foo.go", Language: "go", Kind: "function", Name: "Foo", StartLine: 1, EndLine: 3},
		{ID: "h", Path: "foo.go", Language: "go", Kind: "function", Name: "helper", StartLine: 5, EndLine: 7},
	}
	edges := []graph.DependencyEdge{
		{ID: "e1", FromFile: "foo_test.go", FromSymbol: "TestFoo", ToRef: "Foo", Kind: graph.EdgeCalls, Language: "go", Line: 2},
		{ID: "e2", FromFile: "foo.go", FromSymbol: "Foo", ToRef: "helper", Kind: graph.EdgeCalls, Language: "go", Line: 2},
	}
	return syms, edges
}

// TestCoverageTestsForSymbol asserts the transitive test callers of Foo are
// [TestFoo] and that auto-direction resolves to tests_for_symbol for a non-test.
func TestCoverageTestsForSymbol(t *testing.T) {
	const snapID = "snap-cov-tests"
	syms, edges := coveredGraph()
	ctx, d := saveGraph(t, snapID, syms, edges)

	got, err := Coverage(ctx, d, snapID, "Foo", "", 8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if got.Direction != "tests_for_symbol" {
		t.Errorf("auto direction = %q, want tests_for_symbol", got.Direction)
	}
	if !got.Covered {
		t.Errorf("Covered = false, want true (TestFoo reaches Foo)")
	}
	if want := []string{"TestFoo"}; !reflect.DeepEqual(refNames(got.Tests), want) {
		t.Errorf("tests = %v, want %v", refNames(got.Tests), want)
	}
}

// TestCoverageSymbolsForTest asserts the non-test symbols TestFoo exercises are
// [Foo helper], and that auto-direction resolves to symbols_for_test for a test.
func TestCoverageSymbolsForTest(t *testing.T) {
	const snapID = "snap-cov-syms"
	syms, edges := coveredGraph()
	ctx, d := saveGraph(t, snapID, syms, edges)

	got, err := Coverage(ctx, d, snapID, "TestFoo", "", 8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if got.Direction != "symbols_for_test" {
		t.Errorf("auto direction = %q, want symbols_for_test", got.Direction)
	}
	// sortSymbols orders by path then name: both in foo.go -> Foo, helper.
	if want := []string{"Foo", "helper"}; !reflect.DeepEqual(refNames(got.Symbols), want) {
		t.Errorf("symbols = %v, want %v", refNames(got.Symbols), want)
	}
}

// TestCoverageUncovered asserts a symbol no test reaches is reported uncovered.
func TestCoverageUncovered(t *testing.T) {
	const snapID = "snap-cov-uncov"
	syms := []graph.CodeSymbol{
		{ID: "a", Path: "a.go", Language: "go", Kind: "function", Name: "Alone", StartLine: 1, EndLine: 3},
	}
	ctx, d := saveGraph(t, snapID, syms, nil)

	got, err := Coverage(ctx, d, snapID, "Alone", "tests_for_symbol", 8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if got.Covered || len(got.Tests) != 0 {
		t.Errorf("Coverage(Alone) covered=%v tests=%v, want uncovered/empty", got.Covered, refNames(got.Tests))
	}
}

// TestIsTestSymbol exercises the file-pattern and name-prefix detection.
func TestIsTestSymbol(t *testing.T) {
	cases := []struct {
		s    graph.CodeSymbol
		want bool
	}{
		{graph.CodeSymbol{Name: "TestThing", Path: "x.go"}, true},
		{graph.CodeSymbol{Name: "test_thing", Path: "x.py"}, true},
		{graph.CodeSymbol{Name: "handler", Path: "svc_test.go"}, true},
		{graph.CodeSymbol{Name: "handler", Path: "pkg/test_svc.py"}, true},
		{graph.CodeSymbol{Name: "render", Path: "ui/App.test.tsx"}, true},
		{graph.CodeSymbol{Name: "render", Path: "ui/App.spec.ts"}, true},
		{graph.CodeSymbol{Name: "doIt", Path: "com/FooTest.java"}, true},
		{graph.CodeSymbol{Name: "doIt", Path: "com/FooTests.java"}, true},
		{graph.CodeSymbol{Name: "doIt", Path: "com/FooIT.java"}, true},
		{graph.CodeSymbol{Name: "Foo", Path: "foo.go"}, false},
		{graph.CodeSymbol{Name: "render", Path: "ui/App.tsx"}, false},
	}
	for _, c := range cases {
		if got := isTestSymbol(c.s); got != c.want {
			t.Errorf("isTestSymbol(%q @ %q) = %v, want %v", c.s.Name, c.s.Path, got, c.want)
		}
	}
}

func refNames(refs []CoverageRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}
