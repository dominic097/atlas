package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// cppCallEdges extracts AST-based call edges from a parsed C/C++ file.
//
// It walks every `call_expression` node and resolves its `function` child into a
// bare callee name + qualified reference, mirroring the goCallEdges metadata
// contract so the query layer's resolveTargets can defeat method-name collisions:
//
//   - identifier            free_func()        -> bare "free_func",  qualified ""
//   - field_expression      obj.method()       -> bare "method",     qualified "obj.method"
//     obj->method()      -> bare "method",     qualified "obj.method"
//     a.b.method()        -> bare "method",     qualified "a.b.method"
//   - qualified_identifier  ns::func()         -> bare "func",       qualified "ns::func"
//     Class::method()     -> bare "method",     qualified "Class::method"
//
// recv_type is best-effort: for a field_expression whose receiver is a plain
// local var / parameter with a known declared type (`Type x; x.m()`), recv_type
// is that type; otherwise "". Edges are deduped by (fromSymbol,toRef,line).
func cppCallEdges(root *tree_sitter.Node, source []byte, repoID, repoFullName, filePath string, syms []graph.CodeSymbol) []graph.DependencyEdge {
	if root == nil {
		return nil
	}

	// Best-effort var/param name -> declared type, used to infer recv_type for
	// `var.method()` / `var->method()`. File-scoped is good enough: locals shadow
	// rarely in practice and a wrong-type miss simply degrades to no recv_type.
	varTypes := collectCppVarTypes(root, source)

	var edges []graph.DependencyEdge
	walkNode(root, func(node *tree_sitter.Node) bool {
		if node.Kind() != "call_expression" {
			return true
		}
		fn := node.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		bare, qualified, recvType := cppCallee(fn, source, varTypes)
		if bare == "" {
			return true
		}
		line := int(node.StartPosition().Row) + 1
		fromSymbol := enclosingSymbolName(syms, line)
		edges = append(edges, newCallEdge(repoID, filePath, fromSymbol, bare, qualified, recvType, "cpp", line))
		return true
	})
	return dedupeEdges(edges)
}

// cppCallee resolves a call_expression's `function` child into the bare callee
// name, its qualified form, and a best-effort receiver type.
func cppCallee(fn *tree_sitter.Node, src []byte, varTypes map[string]string) (bare, qualified, recvType string) {
	switch fn.Kind() {
	case "identifier":
		// Free function: bare name, no qualifier.
		return nodeText(fn, src), "", ""

	case "field_expression":
		// obj.method / obj->method / a.b.method — bare = field, qualified = recv "." field.
		field := fn.ChildByFieldName("field")
		arg := fn.ChildByFieldName("argument")
		if field == nil {
			return "", "", ""
		}
		bare = nodeText(field, src)
		recv := ""
		if arg != nil {
			recv = nodeText(arg, src)
		}
		qualified = bare
		if recv != "" {
			qualified = recv + "." + bare
		}
		// recv_type only when the receiver is a plain var/param of a known type.
		if arg != nil && arg.Kind() == "identifier" {
			if t, ok := varTypes[nodeText(arg, src)]; ok {
				recvType = t
			}
		}
		return bare, qualified, recvType

	case "qualified_identifier":
		// ns::func / Class::method / a::b::method — bare = last segment, qualified = full.
		qualified = nodeText(fn, src)
		bare = qualified
		if idx := strings.LastIndex(qualified, "::"); idx >= 0 {
			bare = qualified[idx+2:]
		}
		bare = strings.TrimSpace(bare)
		return bare, qualified, ""

	default:
		// Other callee shapes (e.g. parenthesized_expression for fn-pointer calls,
		// template_function) — fall back to the bare text so the edge is still
		// emitted; resolveTargets handles an unqualified ref gracefully.
		txt := strings.TrimSpace(nodeText(fn, src))
		if txt == "" || strings.ContainsAny(txt, "()[]{} \t\n") {
			return "", "", ""
		}
		return txt, "", ""
	}
}

// collectCppVarTypes walks the tree and records a best-effort map of variable /
// parameter name -> declared type name, drawn from `declaration` and
// `parameter_declaration` nodes (`type` field + the innermost identifier in the
// `declarator` field). Pointer/reference/init declarators are unwrapped to the
// underlying name. Used purely to enrich field_expression recv_type.
func collectCppVarTypes(root *tree_sitter.Node, src []byte) map[string]string {
	out := map[string]string{}
	walkNode(root, func(node *tree_sitter.Node) bool {
		switch node.Kind() {
		case "declaration", "parameter_declaration":
			typeNode := node.ChildByFieldName("type")
			declNode := node.ChildByFieldName("declarator")
			if typeNode == nil || declNode == nil {
				return true
			}
			typeName := strings.TrimSpace(nodeText(typeNode, src))
			if typeName == "" {
				return true
			}
			if name := cppDeclaratorName(declNode, src); name != "" {
				if _, exists := out[name]; !exists {
					out[name] = typeName
				}
			}
		}
		return true
	})
	return out
}

// cppDeclaratorName unwraps a declarator (identifier / pointer_declarator /
// reference_declarator / init_declarator / function_declarator / array_declarator)
// down to the bound variable's plain identifier name.
func cppDeclaratorName(node *tree_sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier", "field_identifier":
		return nodeText(node, src)
	case "destructor_name", "operator_name":
		// `~Foo` / `operator==` — keep the full spelling so it matches the way
		// language servers (and call sites) name the member.
		return strings.TrimSpace(nodeText(node, src))
	case "qualified_identifier":
		qualified := strings.TrimSpace(nodeText(node, src))
		if idx := strings.LastIndex(qualified, "::"); idx >= 0 {
			return strings.TrimSpace(qualified[idx+2:])
		}
		return qualified
	}
	// Most declarators nest the real name under a "declarator" field; init_declarator
	// uses "declarator" for the value-bound name. Prefer the field, then fall back
	// to scanning children for the first identifier.
	if inner := node.ChildByFieldName("declarator"); inner != nil {
		if n := cppDeclaratorName(inner, src); n != "" {
			return n
		}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if child.Kind() == "identifier" || child.Kind() == "field_identifier" {
			return nodeText(child, src)
		}
		if n := cppDeclaratorName(child, src); n != "" {
			return n
		}
	}
	return ""
}
