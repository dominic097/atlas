package parser

import "testing"

// javaCallSource exercises the Java AST call extractor:
//   - a qualified method call on a typed local (obj.format(...)) must yield an edge
//     whose qualified_ref is "obj.method" and whose recv_type is the local's static
//     type ("Helper");
//   - an implicit/this call (helper-less log(...)) must resolve recv_type to the
//     enclosing class;
//   - a this.field.method() call must infer the field's declared type;
//   - a constructor (new Helper()) must produce a bare-name "Helper" edge;
//   - a bare-name external-shaped call (String.valueOf) must still produce a sane
//     edge (qualified_ref set, recv_type empty since the receiver is not a typed
//     in-scope value).
const javaCallSource = `package com.example;

public class Greeter {
    private Helper helper;

    public String greet(String name) {
        Helper local = new Helper();
        String trimmed = name.trim();
        String result = local.format(trimmed);
        this.helper.log(result);
        log(result);
        return String.valueOf(result);
    }

    void log(String s) {}
}
`

func TestJavaCallEdges(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "Greeter.java", "java", []byte(javaCallSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Index call edges by bare callee for assertions.
	type edgeInfo struct {
		qualifiedRef string
		recvType     string
		fromSymbol   string
		found        bool
	}
	byRef := map[string]edgeInfo{}
	for _, e := range res.Edges {
		if e.Kind != "calls" {
			continue
		}
		qr, _ := e.Metadata["qualified_ref"].(string)
		rt, _ := e.Metadata["recv_type"].(string)
		byRef[e.ToRef] = edgeInfo{qualifiedRef: qr, recvType: rt, fromSymbol: e.FromSymbol, found: true}
	}

	// (1) Qualified call on a typed local: local.format(trimmed).
	if got := byRef["format"]; !got.found {
		t.Errorf("format() call edge not found; edges=%+v", res.Edges)
	} else {
		if got.qualifiedRef != "local.format" {
			t.Errorf("format() qualified_ref = %q, want %q", got.qualifiedRef, "local.format")
		}
		if got.recvType != "Helper" {
			t.Errorf("format() recv_type = %q, want %q", got.recvType, "Helper")
		}
		if got.fromSymbol != "greet" {
			t.Errorf("format() from_symbol = %q, want %q", got.fromSymbol, "greet")
		}
	}

	// (2) this.helper.log(result): receiver type inferred from the field's type.
	if got := byRef["log"]; !got.found {
		t.Errorf("log() call edge not found; edges=%+v", res.Edges)
	} else if got.recvType != "Helper" {
		// log() appears both as this.helper.log (recv Helper) and implicit log
		// (recv Greeter); the map keeps the last write. Assert at least one of the
		// expected receiver types was produced by scanning all log edges.
		var sawHelper, sawGreeter bool
		for _, e := range res.Edges {
			if e.ToRef != "log" || e.Kind != "calls" {
				continue
			}
			rt, _ := e.Metadata["recv_type"].(string)
			if rt == "Helper" {
				sawHelper = true
			}
			if rt == "Greeter" {
				sawGreeter = true
			}
		}
		if !sawHelper {
			t.Errorf("this.helper.log() recv_type Helper not found among log edges")
		}
		if !sawGreeter {
			t.Errorf("implicit log() recv_type Greeter not found among log edges")
		}
	}

	// (3) Constructor: new Helper() -> bare callee "Helper".
	if got := byRef["Helper"]; !got.found {
		t.Errorf("new Helper() constructor edge not found; edges=%+v", res.Edges)
	} else if got.qualifiedRef != "Helper" {
		t.Errorf("new Helper() qualified_ref = %q, want %q", got.qualifiedRef, "Helper")
	}

	// (4) Bare-name external-shaped call: String.valueOf(result). qualified_ref is
	// set; recv_type stays empty (String is not a typed in-scope value here).
	if got := byRef["valueOf"]; !got.found {
		t.Errorf("String.valueOf() call edge not found; edges=%+v", res.Edges)
	} else {
		if got.qualifiedRef != "String.valueOf" {
			t.Errorf("valueOf() qualified_ref = %q, want %q", got.qualifiedRef, "String.valueOf")
		}
		if got.recvType != "" {
			t.Errorf("valueOf() recv_type = %q, want empty (untyped receiver)", got.recvType)
		}
	}

	// name.trim(): receiver is a String param -> recv_type "String".
	if got := byRef["trim"]; !got.found {
		t.Errorf("name.trim() call edge not found")
	} else {
		if got.qualifiedRef != "name.trim" {
			t.Errorf("trim() qualified_ref = %q, want %q", got.qualifiedRef, "name.trim")
		}
		if got.recvType != "String" {
			t.Errorf("trim() recv_type = %q, want %q (param type)", got.recvType, "String")
		}
	}
}
