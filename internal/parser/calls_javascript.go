package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// jsCallEdges extracts AST-based call edges from a parsed JavaScript OR
// TypeScript file (the two share enough grammar shape to use one walker —
// tree-sitter-javascript and tree-sitter-typescript both model a call as a
// `call_expression` whose `function` child is either a bare identifier or a
// `member_expression`).
//
// Node kinds handled:
//   - call_expression                       the call itself (one edge per call)
//   - function child = identifier           bare callee foo() -> toRef=foo,
//     qualified_ref=foo, recv_type=""
//   - function child = member_expression    obj.method() -> toRef=method,
//     qualified_ref="obj.method" (object source text + "." + property). The
//     receiver is dynamic in JS/TS, so recv_type is "" EXCEPT:
//   - this.method()        -> recv_type = enclosing class name (walk up the
//     AST to the nearest class_declaration / class)
//   - x.method() where x   -> recv_type = best-effort TS type from a typed
//     declaration `let x: Type` in scope (cheap, single-name annotations).
//
// `new Thing()` is a `new_expression`, not a `call_expression`, so constructor
// invocations are naturally excluded (matching goCallEdges, which keys off
// CallExpr). Edges are attributed to the enclosing symbol via
// enclosingSymbolName(syms, line) and deduped by (fromSymbol,toRef,line) — the
// same metadata contract goCallEdges produces, so the query layer's
// resolveTargets treats these uniformly.
func jsCallEdges(root *tree_sitter.Node, source []byte, repoID, repoFullName, filePath string, syms []graph.CodeSymbol) []graph.DependencyEdge {
	if root == nil {
		return nil
	}

	// Best-effort TS type table: variable name -> declared base type, gathered
	// once up front from `let/const/var x: Type` typed declarations across the
	// file. JS/TS scoping is dynamic, so this is a file-wide best-effort lookup
	// (a same-named variable in two scopes resolves to the last seen type); it
	// only ever ADDS a recv_type hint, and resolveTargets degrades gracefully on
	// a wrong/empty hint. Kept cheap: simple type_identifier / generic base only.
	typedVars := jsTypedVarTypes(root, source)

	var edges []graph.DependencyEdge
	walkNode(root, func(node *tree_sitter.Node) bool {
		if node == nil || node.Kind() != "call_expression" {
			return true
		}
		fn := node.ChildByFieldName("function")
		if fn == nil {
			return true
		}

		var toRef, qualifiedRef, recvType string
		switch fn.Kind() {
		case "identifier":
			// Bare call: foo(), helper(), gen<string>() (the type_arguments are a
			// sibling child, the function is still the identifier).
			toRef = nodeText(fn, source)
			qualifiedRef = toRef
		case "member_expression":
			toRef, qualifiedRef, recvType = jsMemberCall(fn, source, typedVars)
		default:
			// Other callee shapes (IIFE (function(){})(), arr[i](), ?.optional
			// chains parsed as other nodes, etc.) carry no stable bare name —
			// skip rather than emit noise.
			return true
		}

		toRef = strings.TrimSpace(toRef)
		if toRef == "" {
			return true
		}

		line := int(node.StartPosition().Row) + 1
		fromSymbol := enclosingSymbolName(syms, line)
		edges = append(edges, newCallEdge(repoID, filePath, fromSymbol, toRef, qualifiedRef, recvType, "javascript", line))
		return true
	})

	return dedupeEdges(edges)
}

// jsMemberCall decomposes a member_expression callee (obj.method) into the bare
// callee name, its qualified form, and a best-effort receiver type.
//
//	obj.method()        -> ("method", "obj.method", "")
//	this.method()       -> ("method", "this.method", <enclosing class name>)
//	a.b.c()             -> ("c", "a.b.c", "")
//	typed.run()  (TS)   -> ("run", "typed.run", <typed decl type, if known>)
//
// The bare name is the member_expression's `property` (a property_identifier);
// the qualifier is the `object` child's source text.
func jsMemberCall(member *tree_sitter.Node, source []byte, typedVars map[string]string) (toRef, qualifiedRef, recvType string) {
	prop := member.ChildByFieldName("property")
	obj := member.ChildByFieldName("object")
	if prop == nil {
		return "", "", ""
	}
	toRef = nodeText(prop, source)

	if obj == nil {
		qualifiedRef = toRef
		return toRef, qualifiedRef, ""
	}

	objText := strings.TrimSpace(nodeText(obj, source))
	if objText == "" {
		qualifiedRef = toRef
	} else {
		qualifiedRef = objText + "." + toRef
	}

	switch obj.Kind() {
	case "this":
		// this.method() — receiver is the enclosing class instance.
		recvType = jsEnclosingClassName(member, source)
	case "identifier":
		// x.method() — dynamic in general, but a TS `let x: Type` annotation
		// gives a static hint when present.
		if t := typedVars[objText]; t != "" {
			recvType = t
		}
	}
	return toRef, qualifiedRef, recvType
}

// jsEnclosingClassName walks up from a node to the nearest enclosing class and
// returns its name (the `name` field of a class_declaration / class node), or
// "" when the node is not inside a class. Used to stamp this.method() calls with
// the receiver type per the SHARED METADATA CONTRACT.
func jsEnclosingClassName(node *tree_sitter.Node, source []byte) string {
	for cur := node.Parent(); cur != nil; cur = cur.Parent() {
		switch cur.Kind() {
		case "class_declaration", "class":
			if name := cur.ChildByFieldName("name"); name != nil {
				return strings.TrimSpace(nodeText(name, source))
			}
			return ""
		}
	}
	return ""
}

// jsTypedVarTypes collects a best-effort variable -> declared-base-type table
// from TypeScript typed declarations (`let x: Type`, `const y: Foo<T>`). It
// walks every variable_declarator with a `type` annotation, reads the bare base
// type, and records it. File-wide (no per-scope tracking) and intentionally
// conservative: only simple type_identifier / nested_type_identifier last
// segment / generic_type base are resolved; unions, intersections, literals,
// arrays and the like yield "" and are skipped. Returns nil for plain JS (no
// annotations present), which the caller treats as an empty map.
func jsTypedVarTypes(root *tree_sitter.Node, source []byte) map[string]string {
	var out map[string]string
	walkNode(root, func(node *tree_sitter.Node) bool {
		if node == nil || node.Kind() != "variable_declarator" {
			return true
		}
		nameNode := node.ChildByFieldName("name")
		typeNode := node.ChildByFieldName("type")
		if nameNode == nil || typeNode == nil {
			return true
		}
		if nameNode.Kind() != "identifier" {
			return true // destructuring pattern -> no single name
		}
		name := strings.TrimSpace(nodeText(nameNode, source))
		if name == "" {
			return true
		}
		base := jsBaseTypeFromAnnotation(typeNode, source)
		if base == "" {
			return true
		}
		if out == nil {
			out = make(map[string]string)
		}
		out[name] = base
		return true
	})
	return out
}

// jsBaseTypeFromAnnotation reduces a TS `type_annotation` (": Type") to its bare
// base type name, handling the cheap, unambiguous cases and returning "" for
// everything else:
//
//	: MyType            -> MyType          (type_identifier)
//	: pkg.Nested        -> Nested          (nested_type_identifier, last segment)
//	: Array<string>     -> Array           (generic_type base)
//	: Repo | null       -> ""              (union_type — ambiguous, skipped)
func jsBaseTypeFromAnnotation(typeAnno *tree_sitter.Node, source []byte) string {
	// The type_annotation's first NAMED child is the type expression (the ":" is
	// an anonymous child).
	var inner *tree_sitter.Node
	for i := uint(0); i < typeAnno.ChildCount(); i++ {
		c := typeAnno.Child(i)
		if c != nil && c.IsNamed() {
			inner = c
			break
		}
	}
	if inner == nil {
		return ""
	}
	return jsBaseTypeName(inner, source)
}

// jsBaseTypeName extracts the bare type name from a TS type node, mirroring
// goBaseTypeName's "reduce to a nameable identifier, else empty" contract.
func jsBaseTypeName(node *tree_sitter.Node, source []byte) string {
	switch node.Kind() {
	case "type_identifier", "identifier":
		return strings.TrimSpace(nodeText(node, source))
	case "nested_type_identifier":
		// pkg.Nested -> last segment "Nested".
		text := strings.TrimSpace(nodeText(node, source))
		if idx := strings.LastIndexByte(text, '.'); idx >= 0 && idx+1 < len(text) {
			return strings.TrimSpace(text[idx+1:])
		}
		return text
	case "generic_type":
		// Array<string> -> base type before the type_arguments.
		if name := node.ChildByFieldName("name"); name != nil {
			return jsBaseTypeName(name, source)
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			c := node.Child(i)
			if c != nil && (c.Kind() == "type_identifier" || c.Kind() == "nested_type_identifier") {
				return jsBaseTypeName(c, source)
			}
		}
	}
	return ""
}
