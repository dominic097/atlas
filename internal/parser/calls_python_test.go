package parser

import "testing"

// pyCallSource exercises the Python AST call extractor:
//   - a qualified attribute call obj.method() -> qualified_ref "obj.method", bare "method".
//   - a module/package call os.path.join() -> qualified_ref "os.path.join", bare "join",
//     and (Python being dynamic) empty recv_type — resolves sanely as an external call.
//   - a bare-name call helper() -> qualified_ref == bare == "helper", empty recv_type.
//   - a self.method() call inside a class -> recv_type == enclosing class name.
const pyCallSource = `import os


def helper():
    return 1


def driver(obj):
    helper()
    obj.method(1, 2)
    os.path.join("a", "b")


class Greeter:
    def greet(self):
        self.say_hello()

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
		if s.qualified != "self.say_hello" {
			t.Errorf("self.say_hello() qualified_ref = %q, want %q", s.qualified, "self.say_hello")
		}
		if s.recv != "Greeter" {
			t.Errorf("self.say_hello() recv_type = %q, want %q (enclosing class)", s.recv, "Greeter")
		}
	}
}
