package parser

import (
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// jsParseEdges parses a snippet through the public Parse entrypoint and returns
// only the EdgeCalls edges (drops EdgeImports), for assertion convenience.
func jsParseEdges(t *testing.T, path, language, src string) []graph.DependencyEdge {
	t.Helper()
	res, err := Parse("repo1", "owner/repo", path, language, []byte(src))
	if err != nil {
		t.Fatalf("Parse(%s) error: %v", language, err)
	}
	var calls []graph.DependencyEdge
	for _, e := range res.Edges {
		if e.Kind == graph.EdgeCalls {
			calls = append(calls, e)
		}
	}
	return calls
}

// findEdge returns the first call edge whose bare callee (ToRef) matches.
func findEdge(edges []graph.DependencyEdge, toRef string) (graph.DependencyEdge, bool) {
	for _, e := range edges {
		if e.ToRef == toRef {
			return e, true
		}
	}
	return graph.DependencyEdge{}, false
}

func edgeMetaString(e graph.DependencyEdge, key string) string {
	if e.Metadata == nil {
		return ""
	}
	s, _ := e.Metadata[key].(string)
	return s
}

func TestJSCallEdges_QualifiedAndBare(t *testing.T) {
	// A top-level function is an indexed symbol, so call attribution is precise.
	src := `
function render() {
  obj.method(1, 2);
  helper();
  a.b.c();
}
`
	edges := jsParseEdges(t, "widget.js", "javascript", src)

	// Qualified member call obj.method() -> bare ToRef "method", qualified_ref
	// "obj.method", recv_type omitted (dynamic), attributed to enclosing render().
	e, ok := findEdge(edges, "method")
	if !ok {
		t.Fatalf("expected an edge for obj.method(); got edges %v", edges)
	}
	if got := edgeMetaString(e, "qualified_ref"); got != "obj.method" {
		t.Errorf("obj.method qualified_ref = %q, want %q", got, "obj.method")
	}
	if _, present := e.Metadata["recv_type"]; present {
		t.Errorf("obj.method recv_type should be omitted (dynamic), got %v", e.Metadata["recv_type"])
	}
	if e.FromSymbol != "render" {
		t.Errorf("obj.method FromSymbol = %q, want %q (enclosing function)", e.FromSymbol, "render")
	}

	// Bare external call helper() -> ToRef and qualified_ref both bare, recv_type
	// omitted. Resolves sanely (no qualifier, no spurious receiver type).
	hb, ok := findEdge(edges, "helper")
	if !ok {
		t.Fatalf("expected an edge for helper(); got edges %v", edges)
	}
	if got := edgeMetaString(hb, "qualified_ref"); got != "helper" {
		t.Errorf("helper qualified_ref = %q, want %q", got, "helper")
	}
	if _, present := hb.Metadata["recv_type"]; present {
		t.Errorf("bare helper() should carry no recv_type, got %v", hb.Metadata["recv_type"])
	}

	// Nested member a.b.c() -> bare "c", qualified "a.b.c".
	if nb, ok := findEdge(edges, "c"); !ok {
		t.Errorf("expected an edge for a.b.c()")
	} else if got := edgeMetaString(nb, "qualified_ref"); got != "a.b.c" {
		t.Errorf("a.b.c qualified_ref = %q, want %q", got, "a.b.c")
	}
}

func TestJSCallEdges_ThisReceiverType(t *testing.T) {
	src := `
class Widget {
  render() {
    this.update();
  }
}
`
	edges := jsParseEdges(t, "widget.js", "javascript", src)
	e, ok := findEdge(edges, "update")
	if !ok {
		t.Fatalf("expected an edge for this.update(); got %v", edges)
	}
	if got := edgeMetaString(e, "qualified_ref"); got != "this.update" {
		t.Errorf("this.update qualified_ref = %q, want %q", got, "this.update")
	}
	// this.method() stamps the enclosing class name as the receiver type so the
	// query layer can disambiguate same-named methods across classes.
	if got := edgeMetaString(e, "recv_type"); got != "Widget" {
		t.Errorf("this.update recv_type = %q, want enclosing class %q", got, "Widget")
	}
}

func TestJSCallEdges_NewExpressionExcluded(t *testing.T) {
	src := `
function build() {
  let t = new Thing();
  run();
}
`
	edges := jsParseEdges(t, "b.js", "javascript", src)
	if _, ok := findEdge(edges, "Thing"); ok {
		t.Errorf("new Thing() is a constructor (new_expression), should not yield a call edge")
	}
	if _, ok := findEdge(edges, "run"); !ok {
		t.Errorf("expected a call edge for run()")
	}
}

func TestTSCallEdges_TypedReceiver(t *testing.T) {
	// TypeScript: a typed local declaration gives a best-effort receiver type
	// hint on the subsequent method call.
	src := `
function handle() {
  let x: Repo = getRepo();
  x.save();
}
`
	edges := jsParseEdges(t, "svc.ts", "typescript", src)

	e, ok := findEdge(edges, "save")
	if !ok {
		t.Fatalf("expected an edge for x.save(); got %v", edges)
	}
	if got := edgeMetaString(e, "qualified_ref"); got != "x.save" {
		t.Errorf("x.save qualified_ref = %q, want %q", got, "x.save")
	}
	if got := edgeMetaString(e, "recv_type"); got != "Repo" {
		t.Errorf("x.save recv_type = %q, want %q (from `let x: Repo`)", got, "Repo")
	}

	// The bare constructor-style call getRepo() resolves sanely: bare ToRef, no
	// receiver type.
	if gb, ok := findEdge(edges, "getRepo"); !ok {
		t.Errorf("expected an edge for getRepo()")
	} else if _, present := gb.Metadata["recv_type"]; present {
		t.Errorf("bare getRepo() should carry no recv_type, got %v", gb.Metadata["recv_type"])
	}
}
