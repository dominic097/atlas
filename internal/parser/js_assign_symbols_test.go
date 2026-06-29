package parser

import (
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// jsParseSymbols parses a snippet through the public Parse entrypoint and
// returns its CodeSymbols, for definition-extraction assertions.
func jsParseSymbols(t *testing.T, path, language, src string) []graph.CodeSymbol {
	t.Helper()
	res, err := Parse("repo1", "owner/repo", path, language, []byte(src))
	if err != nil {
		t.Fatalf("Parse(%s) error: %v", language, err)
	}
	return res.Symbols
}

// findJSSymbolKind returns the first symbol matching name (and kind, if non-empty).
func findJSSymbolKind(syms []graph.CodeSymbol, name, kind string) (graph.CodeSymbol, bool) {
	for _, s := range syms {
		if s.Name != name {
			continue
		}
		if kind != "" && s.Kind != kind {
			continue
		}
		return s, true
	}
	return graph.CodeSymbol{}, false
}

func countSymbol(syms []graph.CodeSymbol, name string) int {
	n := 0
	for _, s := range syms {
		if s.Name == name {
			n++
		}
	}
	return n
}

// TestJSAssignedFunctionSymbols covers the anonymous function-expression
// definitions that were previously dropped: members, prototype, exports,
// module.exports, and const/let arrows. This is the Express public-API gap.
func TestJSAssignedFunctionSymbols(t *testing.T) {
	src := `
var req = {};
req.accepts = function(){ return 1; };
req.proto = {};
req.proto.parse = () => 2;
exports.normalizeType = function(type){ return type; };
module.exports.compileETag = function(){ return 3; };
const baz = () => 4;
`
	syms := jsParseSymbols(t, "lib/request.js", "javascript", src)

	// X.foo = function(){}  -> bare property name "accepts", kind method.
	if s, ok := findJSSymbolKind(syms, "accepts", "method"); !ok {
		t.Fatalf("expected method symbol 'accepts'; got %v", symbolNames(syms))
	} else if s.Metadata["owner_type"] != "req" {
		t.Errorf("accepts owner_type = %v; want req", s.Metadata["owner_type"])
	}

	// X.proto.parse = () => {} (prototype-style chain) -> "parse".
	if _, ok := findJSSymbolKind(syms, "parse", "method"); !ok {
		t.Errorf("expected method symbol 'parse' from prototype-chain arrow; got %v", symbolNames(syms))
	}

	// exports.bar = function(){} -> "normalizeType".
	if _, ok := findJSSymbolKind(syms, "normalizeType", "method"); !ok {
		t.Errorf("expected method symbol 'normalizeType' from exports assignment; got %v", symbolNames(syms))
	}

	// module.exports.baz = function(){} -> "compileETag".
	if _, ok := findJSSymbolKind(syms, "compileETag", "method"); !ok {
		t.Errorf("expected method symbol 'compileETag' from module.exports assignment; got %v", symbolNames(syms))
	}

	// const baz = () => {} -> "baz" (function kind, via jsVariableSymbols).
	if _, ok := findJSSymbolKind(syms, "baz", "function"); !ok {
		t.Errorf("expected function symbol 'baz' from const arrow; got %v", symbolNames(syms))
	}
}

// TestJSNamedFnExprNotDoubleEmitted guards the dedup rule: a named function
// expression bound to a member is already captured under its own name, so the
// LHS property must NOT also be emitted.
func TestJSNamedFnExprNotDoubleEmitted(t *testing.T) {
	src := `
module.exports = function renderFile(){ return 1; };
`
	syms := jsParseSymbols(t, "tmpl.js", "javascript", src)
	if _, ok := findJSSymbolKind(syms, "renderFile", ""); !ok {
		t.Errorf("expected named fn-expr 'renderFile'; got %v", symbolNames(syms))
	}
	// 'exports' must not be additionally emitted from the member LHS.
	if n := countSymbol(syms, "exports"); n != 0 {
		t.Errorf("named fn-expr double-emitted member LHS 'exports' %d time(s)", n)
	}
}

// TestJSNonFunctionAssignmentNotEmitted guards precision: a member assignment
// whose RHS is NOT a function must never be promoted to a definition.
func TestJSNonFunctionAssignmentNotEmitted(t *testing.T) {
	src := `
var res = {};
res.statusCode = 200;
res.headers = { 'x': 'y' };
`
	syms := jsParseSymbols(t, "lib/response.js", "javascript", src)
	if n := countSymbol(syms, "statusCode"); n != 0 {
		t.Errorf("non-function assignment 'statusCode' wrongly emitted %d time(s)", n)
	}
	if n := countSymbol(syms, "headers"); n != 0 {
		t.Errorf("non-function assignment 'headers' wrongly emitted %d time(s)", n)
	}
}

func symbolNames(syms []graph.CodeSymbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Kind+":"+s.Name)
	}
	return out
}
