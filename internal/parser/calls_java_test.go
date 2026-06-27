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
//   - a static class call (String.valueOf) must resolve recv_type to the declaring
//     class ("String") and be tagged recv_kind=static — the static-call precision
//     win, since a static method's declaring class IS its receiver type.
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
		recvKind     string
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
		rk, _ := e.Metadata["recv_kind"].(string)
		byRef[e.ToRef] = edgeInfo{qualifiedRef: qr, recvType: rt, recvKind: rk, fromSymbol: e.FromSymbol, found: true}
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

	// (4) Static class call: String.valueOf(result). The receiver is the class
	// String (not an in-scope value), so recv_type resolves to "String" and the
	// edge is tagged recv_kind=static — a static method's declaring class is its
	// receiver type, so this is a precise, declaration-grounded attribution.
	if got := byRef["valueOf"]; !got.found {
		t.Errorf("String.valueOf() call edge not found; edges=%+v", res.Edges)
	} else {
		if got.qualifiedRef != "String.valueOf" {
			t.Errorf("valueOf() qualified_ref = %q, want %q", got.qualifiedRef, "String.valueOf")
		}
		if got.recvType != "String" {
			t.Errorf("valueOf() recv_type = %q, want %q (static class receiver)", got.recvType, "String")
		}
		if got.recvKind != "static" {
			t.Errorf("valueOf() recv_kind = %q, want %q", got.recvKind, "static")
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

// javaReceiverMatrixSource exercises every declaration-grounded receiver source
// the resolver supports, with deliberate method-name reuse so an inference miss
// would surface as a wrong recv_type:
//   - var-with-initializer:  var v = new Engine();  v.run()  -> Engine (NOT "var")
//   - explicit local:        Engine e = ...;        e.run()  -> Engine
//   - field:                 this.svc.run()                  -> Service
//   - param:                 cfg.run()                       -> Config
//   - this/implicit:         run()                           -> Worker (enclosing)
//   - static class call:     Engine.boot()                   -> Engine, kind=static
//   - static stdlib call:    Integer.parseInt(x)             -> Integer, kind=static
const javaReceiverMatrixSource = `package com.example;

public class Worker {
    private Service svc;

    public void process(Config cfg, String x) {
        var v = new Engine();
        Engine e = new Engine();
        v.run();
        e.run();
        this.svc.run();
        cfg.run();
        run();
        Engine.boot();
        int n = Integer.parseInt(x);
    }

    void run() {}
}
`

func TestJavaReceiverMatrix(t *testing.T) {
	res, err := Parse("repo-2", "owner/repo", "Worker.java", "java", []byte(javaReceiverMatrixSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Collect every (recvType, recvKind, qualifiedRef) per bare callee. Several
	// receivers share the callee "run", so index those by qualified_ref.
	type info struct{ recvType, recvKind string }
	byQualified := map[string]info{}
	for _, e := range res.Edges {
		if e.Kind != "calls" {
			continue
		}
		qr, _ := e.Metadata["qualified_ref"].(string)
		rt, _ := e.Metadata["recv_type"].(string)
		rk, _ := e.Metadata["recv_kind"].(string)
		byQualified[qr] = info{recvType: rt, recvKind: rk}
	}

	// The keystone: `var v = new Engine()` must yield recv_type Engine, NOT the
	// literal "var" (the inference bug this strand fixes).
	check := func(qualified, wantType, wantKind string) {
		got, ok := byQualified[qualified]
		if !ok {
			t.Errorf("%s: call edge not found; edges=%+v", qualified, res.Edges)
			return
		}
		if got.recvType != wantType {
			t.Errorf("%s: recv_type = %q, want %q", qualified, got.recvType, wantType)
		}
		if got.recvKind != wantKind {
			t.Errorf("%s: recv_kind = %q, want %q", qualified, got.recvKind, wantKind)
		}
	}

	check("v.run", "Engine", "")                   // var-with-initializer (keystone)
	check("e.run", "Engine", "")                   // explicit local
	check("this.svc.run", "Service", "")           // field via this.f
	check("cfg.run", "Config", "")                 // param
	check("this.run", "Worker", "")                // implicit this -> enclosing class
	check("Engine.boot", "Engine", "static")       // static class call
	check("Integer.parseInt", "Integer", "static") // static stdlib call

	// Guard: no edge ever carries the literal "var" as a receiver type.
	for _, e := range res.Edges {
		if rt, _ := e.Metadata["recv_type"].(string); rt == "var" {
			t.Errorf("recv_type leaked the literal \"var\" on edge %+v", e)
		}
	}
}

const javaSymbolCoverageSource = `package com.example;

public class Model {
    private final String name;
    static int count;

    public Model(String name) {
        this.name = name;
    }

    public String name() {
        return name;
    }

    static class Nested {
        void ping() {}
    }
}

class Box<T extends Number> {
    <U> U id(U value) {
        return value;
    }
}

interface Named {
    String label();
    int VERSION = 1;
}

enum Mode {
    READ,
    WRITE;
}

record Pair(String left, int right) {}

@interface JsonName {
    String value();
}
`

func TestJavaSymbols_MembersAndModernTypes(t *testing.T) {
	res, err := Parse("repo-3", "owner/repo", "Model.java", "java", []byte(javaSymbolCoverageSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	type key struct {
		name string
		kind string
	}
	symbols := map[key]map[string]any{}
	for _, sym := range res.Symbols {
		symbols[key{sym.Name, sym.Kind}] = sym.Metadata
	}

	want := []key{
		{"Model", "class"},
		{"name", "field"},
		{"count", "field"},
		{"Model", "constructor"},
		{"name", "method"},
		{"Nested", "class"},
		{"ping", "method"},
		{"Named", "interface"},
		{"label", "method"},
		{"VERSION", "field"},
		{"Mode", "enum"},
		{"READ", "enum_constant"},
		{"WRITE", "enum_constant"},
		{"Pair", "record"},
		{"left", "field"},
		{"right", "field"},
		{"JsonName", "annotation"},
		{"value", "annotation_member"},
		{"Box", "class"},
		{"T", "type_parameter"},
		{"U", "type_parameter"},
		{"Box", "constructor"},
		{"id", "method"},
	}
	for _, k := range want {
		if _, ok := symbols[k]; !ok {
			t.Errorf("missing Java symbol %s/%s; got=%+v", k.name, k.kind, res.Symbols)
		}
	}

	if owner, _ := symbols[key{"name", "field"}]["owner_type"].(string); owner != "Model" {
		t.Errorf("field name owner_type = %q, want Model", owner)
	}
	if owner, _ := symbols[key{"Model", "constructor"}]["owner_type"].(string); owner != "Model" {
		t.Errorf("constructor owner_type = %q, want Model", owner)
	}
	if owner, _ := symbols[key{"left", "field"}]["owner_type"].(string); owner != "Pair" {
		t.Errorf("record component left owner_type = %q, want Pair", owner)
	}
	if synthetic, _ := symbols[key{"Box", "constructor"}]["synthetic"].(bool); !synthetic {
		t.Errorf("default Box constructor synthetic flag = %v, want true", synthetic)
	}
}
