package parser

import "testing"

// pyCallSource exercises the Python AST call extractor:
//   - a qualified attribute call obj.method() -> qualified_ref "obj.method", bare "method".
//   - a module/package call os.path.join() -> qualified_ref "os.path.join", bare "join",
//     and (Python being dynamic) empty recv_type — resolves sanely as an external call.
//   - a bare-name call helper() -> qualified_ref == bare == "helper", empty recv_type.
//   - a self.method() call inside a class -> recv_type == enclosing class name.
const pyCallSource = `import os

DEFAULT_TIMEOUT = 30
api_version = "v1"

if os.name:
    def platform_helper():
        return os.name

    class ConditionalGreeter:
        def ping(self):
            return "pong"

def helper():
    return 1


def driver(obj):
    def local_helper():
        return helper()

    helper()
    local_helper()
    obj.method(1, 2)
    os.path.join("a", "b")


class Greeter:
    greeting = "hello"
    DEFAULT_TITLE: str = "Hello"

    def greet(self):
        def nested_greeting():
            return "nested"

        self.say_hello()
        nested_greeting()

    @property
    def title(self):
        return "hello"

    def say_hello(self):
        return "hi"
`

func TestPythonCallEdges(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "sample.py", "python", []byte(pyCallSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	type edgeInfo struct {
		qualified string
		from      string
		recv      string
		found     bool
	}
	got := map[string]*edgeInfo{
		"method":    {},
		"join":      {},
		"helper":    {},
		"say_hello": {},
	}

	for _, e := range res.Edges {
		if e.Kind != "calls" {
			continue
		}
		info, ok := got[e.ToRef]
		if !ok {
			continue
		}
		info.found = true
		info.from = e.FromSymbol
		info.qualified, _ = e.Metadata["qualified_ref"].(string)
		info.recv, _ = e.Metadata["recv_type"].(string)
	}

	// (a) Qualified attribute call obj.method(): qualified_ref carries the full
	//     dotted form, bare ToRef is the method name, recv_type empty (dynamic).
	if m := got["method"]; !m.found {
		t.Errorf("obj.method() call edge not found; edges=%+v", res.Edges)
	} else {
		if m.qualified != "obj.method" {
			t.Errorf("obj.method() qualified_ref = %q, want %q", m.qualified, "obj.method")
		}
		if m.recv != "" {
			t.Errorf("obj.method() recv_type = %q, want empty (Python dynamic)", m.recv)
		}
	}

	// (b) Module call os.path.join(): bare-name external call resolves sanely —
	//     ToRef "join", full qualified_ref preserved, no recv_type.
	if j := got["join"]; !j.found {
		t.Errorf("os.path.join() call edge not found; edges=%+v", res.Edges)
	} else {
		if j.qualified != "os.path.join" {
			t.Errorf("os.path.join() qualified_ref = %q, want %q", j.qualified, "os.path.join")
		}
		if j.recv != "" {
			t.Errorf("os.path.join() recv_type = %q, want empty", j.recv)
		}
	}

	// (c) Bare-name call helper(): qualified_ref == bare name, no recv_type.
	if h := got["helper"]; !h.found {
		t.Errorf("helper() call edge not found; edges=%+v", res.Edges)
	} else if h.qualified != "helper" {
		t.Errorf("helper() qualified_ref = %q, want %q", h.qualified, "helper")
	}

	// (d) self.method() inside class Greeter: recv_type is the enclosing class.
	if s := got["say_hello"]; !s.found {
		t.Errorf("self.say_hello() call edge not found; edges=%+v", res.Edges)
	} else {
		if s.from != "greet" {
			t.Errorf("self.say_hello() FromSymbol = %q, want %q", s.from, "greet")
		}
		if s.qualified != "self.say_hello" {
			t.Errorf("self.say_hello() qualified_ref = %q, want %q", s.qualified, "self.say_hello")
		}
		if s.recv != "Greeter" {
			t.Errorf("self.say_hello() recv_type = %q, want %q (enclosing class)", s.recv, "Greeter")
		}
	}
}

func TestPythonModuleScopeConditionalSymbols(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "sample.py", "python", []byte(pyCallSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	seen := map[string]graphMeta{}
	for _, sym := range res.Symbols {
		seen[sym.Kind+":"+sym.Name] = graphMeta(sym.Metadata)
	}

	if _, ok := seen["function:platform_helper"]; !ok {
		t.Fatalf("conditional function platform_helper not indexed; symbols=%+v", res.Symbols)
	}
	if _, ok := seen["class:ConditionalGreeter"]; !ok {
		t.Fatalf("conditional class ConditionalGreeter not indexed; symbols=%+v", res.Symbols)
	}
	if meta, ok := seen["method:ping"]; !ok {
		t.Fatalf("conditional class method ping not indexed; symbols=%+v", res.Symbols)
	} else if meta["recv_type"] != "ConditionalGreeter" {
		t.Fatalf("ping recv_type = %v, want ConditionalGreeter", meta["recv_type"])
	}
}

func TestPythonNestedFunctionSymbols(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "sample.py", "python", []byte(pyCallSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	seen := map[string]graphMeta{}
	for _, sym := range res.Symbols {
		if sym.Kind == "function" && sym.Name == "local_helper" {
			meta := graphMeta(sym.Metadata)
			if meta["scope"] != "local_function" {
				t.Fatalf("local_helper scope = %v, want local_function", meta["scope"])
			}
			if meta["owner_function"] != "driver" {
				t.Fatalf("local_helper owner_function = %v, want driver", meta["owner_function"])
			}
		}
		if sym.Kind == "function" {
			seen[sym.Name] = graphMeta(sym.Metadata)
		}
	}
	if _, ok := seen["local_helper"]; !ok {
		t.Fatalf("nested function local_helper not indexed; symbols=%+v", res.Symbols)
	}
	if meta, ok := seen["nested_greeting"]; !ok {
		t.Fatalf("method-local function nested_greeting not indexed; symbols=%+v", res.Symbols)
	} else if meta["owner_function"] != "greet" {
		t.Fatalf("nested_greeting owner_function = %v, want greet", meta["owner_function"])
	}
}

func TestPythonMethodSymbols(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "sample.py", "python", []byte(pyCallSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	seen := map[string]graphMeta{}
	for _, sym := range res.Symbols {
		if sym.Kind == "method" {
			seen[sym.Name] = graphMeta(sym.Metadata)
		}
	}

	for _, name := range []string{"greet", "title", "say_hello"} {
		meta, ok := seen[name]
		if !ok {
			t.Fatalf("method %s not indexed; symbols=%+v", name, res.Symbols)
		}
		if meta["owner_type"] != "Greeter" {
			t.Fatalf("method %s owner_type = %v, want Greeter", name, meta["owner_type"])
		}
		if meta["recv_type"] != "Greeter" {
			t.Fatalf("method %s recv_type = %v, want Greeter", name, meta["recv_type"])
		}
	}
}

func TestPythonAssignmentSymbols(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "sample.py", "python", []byte(pyCallSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	seen := map[string]graphMeta{}
	for _, sym := range res.Symbols {
		seen[sym.Kind+":"+sym.Name] = graphMeta(sym.Metadata)
	}

	if _, ok := seen["constant:DEFAULT_TIMEOUT"]; !ok {
		t.Fatalf("module constant DEFAULT_TIMEOUT not indexed; symbols=%+v", res.Symbols)
	}
	if meta, ok := seen["variable:api_version"]; !ok {
		t.Fatalf("module variable api_version not indexed; symbols=%+v", res.Symbols)
	} else if meta["scope"] != "module" {
		t.Fatalf("api_version scope = %v, want module", meta["scope"])
	}
	for _, name := range []string{"greeting", "DEFAULT_TITLE"} {
		meta, ok := seen["field:"+name]
		if !ok {
			t.Fatalf("class field %s not indexed; symbols=%+v", name, res.Symbols)
		}
		if meta["owner_type"] != "Greeter" {
			t.Fatalf("class field %s owner_type = %v, want Greeter", name, meta["owner_type"])
		}
	}
}
