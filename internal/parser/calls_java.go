package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// javaCallEdges extracts AST-based call edges from a parsed Java file.
//
// It walks `root` for the two Java call-shaped nodes —
//   - method_invocation       (obj.m(args) / m(args) / this.m(args))
//   - object_creation_expression (new Type(args), Type treated as the callee)
//
// — and emits one EdgeCalls graph.DependencyEdge per call via newCallEdge,
// attributing each to its enclosing symbol (enclosingSymbolName) and recording:
//   - toRef        the BARE callee name (method name / constructed type's bare name)
//   - qualifiedRef the qualified form ("obj.m", "this.m", "Type" for new)
//   - recvType     best-effort STATIC base type of the receiver, defeating
//     method-name collisions in the query layer exactly as Go does.
//
// Receiver-type inference is intra-class + intra-method and grammar-only (no
// type checker): a per-scope name->type table is built from local var decls,
// method params, and class fields; `this.m()` / implicit `m()` resolve to the
// enclosing class. Unknown receivers yield "" (best-effort dispatch upstream).
//
// Calls textually inside string/comment nodes never reach here — tree-sitter
// does not parse call syntax inside literals — but we still avoid descending
// into those node kinds defensively. Edges are deduped by (fromSymbol,toRef,line).
func javaCallEdges(root *tree_sitter.Node, source []byte, repoID, repoFullName, filePath string, syms []graph.CodeSymbol) []graph.DependencyEdge {
	if root == nil {
		return nil
	}

	var edges []graph.DependencyEdge

	// emit records one call. recvType "" is passed through; newCallEdge omits the
	// metadata key when blank (so resolveTargets reads it as unknown). When the
	// receiver is a STATIC class qualifier (Type.method()), recvType is the class
	// name and static=true so we tag Metadata["recv_kind"]="static" — purely
	// informational (the query layer reads only recv_type, where a static method's
	// declaring class IS the correct receiver type, so resolution stays correct).
	emit := func(node *tree_sitter.Node, toRef, qualifiedRef, recvType string, static bool) {
		toRef = strings.TrimSpace(toRef)
		if toRef == "" {
			return
		}
		line := int(node.StartPosition().Row) + 1
		fromSymbol := enclosingSymbolName(syms, line)
		edge := newCallEdge(repoID, filePath, fromSymbol, toRef, qualifiedRef, recvType, "java", line)
		if static && strings.TrimSpace(recvType) != "" {
			if edge.Metadata == nil {
				edge.Metadata = graph.JSONBMap{}
			}
			edge.Metadata["recv_kind"] = "static"
		}
		edges = append(edges, edge)
	}

	// walk descends the tree. typeTable is the name->base-type map in scope for
	// the current node; className is the enclosing class/interface/enum name (for
	// `this`/implicit receivers). Both refine as we enter declarations.
	var walk func(node *tree_sitter.Node, typeTable map[string]string, className string)
	walk = func(node *tree_sitter.Node, typeTable map[string]string, className string) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "line_comment", "block_comment", "string_literal", "character_literal":
			// No call syntax lives inside these; don't descend.
			return
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
			// Entering a (possibly nested) type: derive its name and field types,
			// layering fields onto a fresh table scope.
			name := javaTypeName(node, source)
			fields := javaFieldTypes(node, source)
			merged := mergeTypeTables(typeTable, fields)
			for i := uint(0); i < node.ChildCount(); i++ {
				walk(node.Child(i), merged, name)
			}
			return
		case "method_declaration", "constructor_declaration", "compact_constructor_declaration":
			// Entering a method/constructor: layer its params + local vars onto the
			// current (field-bearing) table for receiver inference within the body.
			locals := javaMethodLocalTypes(node, source)
			merged := mergeTypeTables(typeTable, locals)
			for i := uint(0); i < node.ChildCount(); i++ {
				walk(node.Child(i), merged, className)
			}
			return
		case "method_invocation":
			toRef, qualifiedRef, recvType, static := javaMethodInvocation(node, source, typeTable, className)
			emit(node, toRef, qualifiedRef, recvType, static)
			// Still descend: arguments / receiver may contain nested calls.
		case "object_creation_expression":
			if typ := node.ChildByFieldName("type"); typ != nil {
				qualified := strings.TrimSpace(nodeText(typ, source))
				bare := javaBareTypeName(qualified)
				// A constructor has no value receiver; recv_type stays "".
				emit(node, bare, qualified, "", false)
			}
			// Descend for nested calls in the argument list.
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			walk(node.Child(i), typeTable, className)
		}
	}

	walk(root, map[string]string{}, "")
	return dedupeEdges(edges)
}

// javaMethodInvocation resolves a method_invocation node into (bareName,
// qualifiedRef, recvType). Grammar shape (verified):
//
//	with receiver:    object=<expr> name=identifier arguments=...
//	implicit/static:  name=identifier arguments=...   (no object field)
//
// recvType inference (the fourth return is `static` — true when the receiver is a
// CLASS qualifier rather than a value, i.e. a static call Type.method()):
//   - object is `this`                  -> enclosing class
//   - no object (implicit m())          -> enclosing class
//   - object is a simple identifier in   -> typeTable[ident]  (var/param/field value)
//     the type table
//   - object is a simple identifier NOT  -> the identifier itself, static=true,
//     in the table but shaped like a       PROVIDED it follows the Java class-name
//     type name (String.valueOf, ...)       convention (initial uppercase). This is
//                                            the static-call precision win: the
//                                            declaring class IS the receiver type.
//   - object is `this.field`            -> typeTable[field]
//   - anything else (chained/external)  -> "" (unknown — never guessed)
func javaMethodInvocation(node *tree_sitter.Node, source []byte, typeTable map[string]string, className string) (string, string, string, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return "", "", "", false
	}
	bare := strings.TrimSpace(nodeText(nameNode, source))
	if bare == "" {
		return "", "", "", false
	}

	obj := node.ChildByFieldName("object")
	if obj == nil {
		// Implicit receiver: m() -> this.m on the enclosing class.
		qualified := bare
		if className != "" {
			qualified = "this." + bare
		}
		return bare, qualified, className, false
	}

	objText := strings.TrimSpace(nodeText(obj, source))
	qualified := objText + "." + bare

	switch obj.Kind() {
	case "this":
		return bare, qualified, className, false
	case "identifier":
		// A known value (local var / param / field) -> its declared type.
		if t := typeTable[objText]; t != "" {
			return bare, qualified, t, false
		}
		// Not a value in scope. If it follows the Java class-naming convention
		// (initial uppercase, e.g. String, Math, LoggerFactory), this is a static
		// call on a class: the class is the receiver type. Conventional but
		// grounded — Java requires types to be capitalized and values to be in
		// scope, so an unbound capitalized identifier qualifier is a type ref.
		if javaLooksLikeTypeName(objText) {
			return bare, qualified, objText, true
		}
		// Unbound lowercase identifier (an outer-scope/inherited field we can't
		// see): genuinely unknown -> leave empty rather than guess wrong.
		return bare, qualified, "", false
	case "field_access":
		// this.field.m() -> type of `field`; foreign.field (System.out) -> unknown.
		if base := obj.ChildByFieldName("object"); base != nil && base.Kind() == "this" {
			if fld := obj.ChildByFieldName("field"); fld != nil {
				return bare, qualified, typeTable[strings.TrimSpace(nodeText(fld, source))], false
			}
		}
		return bare, qualified, "", false
	default:
		// Chained calls, casts, parenthesized, package-qualified statics, etc.
		return bare, qualified, "", false
	}
}

// javaLooksLikeTypeName reports whether a bare identifier follows the Java type
// (class/interface/enum) naming convention: a single identifier segment whose
// first letter is an uppercase ASCII letter. Used ONLY to recognize a static-call
// qualifier (Type.method()) when the qualifier is not a value in scope. Anything
// with a lowercase initial (a value/package fragment) is rejected, so we never
// mis-tag a package- or variable-qualified call as a static type receiver.
func javaLooksLikeTypeName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	c := s[0]
	return c >= 'A' && c <= 'Z'
}

// javaTypeName returns the declared name of a class/interface/enum/record node
// (its `name` field), or "" when absent.
func javaTypeName(node *tree_sitter.Node, source []byte) string {
	if n := node.ChildByFieldName("name"); n != nil {
		return strings.TrimSpace(nodeText(n, source))
	}
	// Fallback: first direct identifier child.
	return childText(node, "identifier", source)
}

// javaFieldTypes builds a fieldName->baseType table from a type declaration's
// direct field_declaration members. Only same-class fields are included; nested
// types are skipped (they get their own table when walked).
func javaFieldTypes(typeNode *tree_sitter.Node, source []byte) map[string]string {
	out := map[string]string{}
	body := typeNode.ChildByFieldName("body")
	if body == nil {
		return out
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		member := body.Child(i)
		if member == nil || member.Kind() != "field_declaration" {
			continue
		}
		for name, typ := range javaDeclaratorTypes(member, source) {
			out[name] = typ
		}
	}
	return out
}

// javaMethodLocalTypes builds a name->baseType table for a method/constructor
// from its formal parameters and the local_variable_declarations in its body.
func javaMethodLocalTypes(methodNode *tree_sitter.Node, source []byte) map[string]string {
	out := map[string]string{}

	// Formal parameters: formal_parameter{ type, name } (and spread_parameter).
	if params := methodNode.ChildByFieldName("parameters"); params != nil {
		for i := uint(0); i < params.ChildCount(); i++ {
			p := params.Child(i)
			if p == nil {
				continue
			}
			if p.Kind() != "formal_parameter" && p.Kind() != "spread_parameter" {
				continue
			}
			typ := javaDeclTypeName(p, source)
			if typ == "" {
				continue
			}
			if nameNode := p.ChildByFieldName("name"); nameNode != nil {
				out[strings.TrimSpace(nodeText(nameNode, source))] = typ
			} else {
				// spread_parameter exposes its name via a variable_declarator child.
				for _, name := range javaDeclaratorNames(p, source) {
					out[name] = typ
				}
			}
		}
	}

	// Local variable declarations anywhere in the body (block-scope flattened —
	// best-effort, mirroring the Go intra-function table).
	body := methodNode.ChildByFieldName("body")
	if body != nil {
		var collect func(n *tree_sitter.Node)
		collect = func(n *tree_sitter.Node) {
			if n == nil {
				return
			}
			// Do not descend into nested type/lambda bodies — they have their own
			// scope and would pollute this method's table.
			switch n.Kind() {
			case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
				return
			case "local_variable_declaration":
				for name, typ := range javaDeclaratorTypes(n, source) {
					out[name] = typ
				}
			}
			for i := uint(0); i < n.ChildCount(); i++ {
				collect(n.Child(i))
			}
		}
		collect(body)
	}
	return out
}

// javaDeclaratorTypes resolves a declaration node (field_declaration /
// local_variable_declaration) into a name->baseType map across all its
// variable_declarator children. The declared `type` field applies to every
// declarator, EXCEPT for `var` (Java 10+ local type inference), where the type is
// recovered PER declarator from its initializer:
//
//	var x = new Foo();            -> x:Foo   (object_creation_expression)
//	var y = new Bar<Baz>();       -> y:Bar
//	Foo a, b;                     -> a:Foo, b:Foo
//	var z = something.unknown();  -> z omitted (no declaration-grounded type)
//
// A declarator whose type cannot be grounded is skipped — never guessed.
func javaDeclaratorTypes(declNode *tree_sitter.Node, source []byte) map[string]string {
	out := map[string]string{}
	declared := javaDeclTypeName(declNode, source)
	isVar := declared == "var"
	for i := uint(0); i < declNode.ChildCount(); i++ {
		child := declNode.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := strings.TrimSpace(nodeText(nameNode, source))
		if name == "" {
			continue
		}
		typ := declared
		if isVar {
			// Local `var`: recover the concrete type from the initializer.
			typ = javaInitializerType(child.ChildByFieldName("value"), source)
		}
		if typ == "" {
			continue
		}
		out[name] = typ
	}
	return out
}

// javaInitializerType infers the base type produced by a variable_declarator's
// initializer expression, for `var` locals. Only declaration-grounded shapes are
// honored:
//
//	new Foo(...)            object_creation_expression -> Foo
//	new Foo[...] / Foo[]{}  array_creation_expression  -> Foo
//
// Anything else (method calls, ternaries, casts, literals) returns "" so the
// variable is left untyped rather than mis-typed.
func javaInitializerType(value *tree_sitter.Node, source []byte) string {
	if value == nil {
		return ""
	}
	switch value.Kind() {
	case "object_creation_expression", "array_creation_expression":
		if typ := value.ChildByFieldName("type"); typ != nil {
			return javaBareTypeName(nodeText(typ, source))
		}
	}
	return ""
}

// javaDeclTypeName returns the bare base type of a declaration node's `type`
// field (field_declaration / local_variable_declaration / formal_parameter),
// stripping generics and package/scope qualifiers. "" when unresolvable.
//
// NOTE: for a `var` local this returns the literal "var" — callers that must
// resolve inferred locals use javaDeclaratorTypes, which special-cases it.
func javaDeclTypeName(declNode *tree_sitter.Node, source []byte) string {
	typeNode := declNode.ChildByFieldName("type")
	if typeNode == nil {
		return ""
	}
	return javaBareTypeName(nodeText(typeNode, source))
}

// javaDeclaratorNames returns the declared names from a declaration node's
// variable_declarator children (a single field/var line can declare several).
func javaDeclaratorNames(declNode *tree_sitter.Node, source []byte) []string {
	var out []string
	for i := uint(0); i < declNode.ChildCount(); i++ {
		child := declNode.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		if nameNode := child.ChildByFieldName("name"); nameNode != nil {
			if name := strings.TrimSpace(nodeText(nameNode, source)); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// javaBareTypeName reduces a Java type's source text to its bare type name:
//
//	List<String>          -> List
//	Map<String, Integer>  -> Map
//	com.example.Other     -> Other
//	HashMap<>             -> HashMap
//	String[]              -> String
//	int                   -> int
//
// Returns "" for empty/anonymous input.
func javaBareTypeName(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	// Drop generic type arguments.
	if lt := strings.IndexByte(t, '<'); lt >= 0 {
		t = t[:lt]
	}
	// Drop array brackets.
	if br := strings.IndexByte(t, '['); br >= 0 {
		t = t[:br]
	}
	t = strings.TrimSpace(t)
	// Drop package/scope qualifier: keep the last dotted segment.
	if dot := strings.LastIndexByte(t, '.'); dot >= 0 {
		t = t[dot+1:]
	}
	return strings.TrimSpace(t)
}

// mergeTypeTables returns a new table = base overlaid with overlay (overlay wins
// on conflicts), so an inner scope's locals shadow outer fields without mutating
// the shared parent map.
func mergeTypeTables(base, overlay map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}
