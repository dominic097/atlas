package parser

import (
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// pyCallEdges extracts AST-based call edges from a parsed Python file.
//
// It walks every `call` node in the tree-sitter AST and emits one EdgeCalls
// edge per call, attributing it to the enclosing symbol and recording the
// callee's qualified form so the query layer can defeat method-name collisions.
//
// Python call shape (verified empirically against tree-sitter-python):
//   - call.function == identifier        -> bare name, no qualifier.
//   - call.function == attribute         -> attribute.object "." attribute.attribute;
//     bare = the `attribute` field (identifier), qualified = full dotted text.
//
// recv_type: Python is dynamically typed, so the static receiver type is unknown
// for an arbitrary `obj.method()` and is left "". The one exception is the
// idiomatic `self.method()` inside a class body, where the receiver type is the
// enclosing class — we set recv_type to that class name so in-repo method
// dispatch can resolve it.
func pyCallEdges(root *tree_sitter.Node, source []byte, repoID, repoFullName, filePath string, syms []graph.CodeSymbol) []graph.DependencyEdge {
	if root == nil {
		return nil
	}

	var edges []graph.DependencyEdge
	walkNode(root, func(n *tree_sitter.Node) bool {
		if n.Kind() != "call" {
			return true
		}
		fn := n.ChildByFieldName("function")
		if fn == nil {
			return true
		}

		var bare, qualified, recvType string
		switch fn.Kind() {
		case "identifier":
			// Bare name call, e.g. helper().
			bare = nodeText(fn, source)
			qualified = bare
		case "attribute":
			// Qualified call, e.g. obj.method() / module.func() / self.method().
			attr := fn.ChildByFieldName("attribute")
			if attr == nil {
				return true
			}
			bare = nodeText(attr, source)
			// The attribute node text is the full dotted callee (e.g. "obj.method",
			// "os.path.join") — exactly the qualified reference we want.
			qualified = nodeText(fn, source)
			// self.method(): the static receiver type is the enclosing class.
			if obj := fn.ChildByFieldName("object"); obj != nil &&
				obj.Kind() == "identifier" && nodeText(obj, source) == "self" {
				recvType = enclosingClassName(syms, int(n.StartPosition().Row)+1)
			}
		default:
			// Other callees (e.g. a subscript or a call result like f()()).
			// Best-effort: use the raw text as the bare/qualified ref.
			bare = nodeText(fn, source)
			qualified = bare
		}

		if bare == "" {
			return true
		}

		line := int(n.StartPosition().Row) + 1
		fromSymbol := enclosingSymbolName(syms, line)
		edges = append(edges, newCallEdge(
			repoID, filePath, fromSymbol, bare, qualified, recvType, "python", line,
		))
		return true
	})

	return dedupeEdges(edges)
}

// enclosingClassName returns the name of the innermost class symbol whose span
// contains the given (1-based) line, or "" when the line is not inside a class.
// Used to resolve the receiver type of a `self.method()` call.
func enclosingClassName(syms []graph.CodeSymbol, line int) string {
	if line <= 0 {
		return ""
	}
	bestName := ""
	bestSpan := 0
	for _, s := range syms {
		if s.Kind != "class" {
			continue
		}
		if s.StartLine <= 0 || s.EndLine <= 0 {
			continue
		}
		if line < s.StartLine || line > s.EndLine {
			continue
		}
		span := s.EndLine - s.StartLine
		if bestName == "" || span < bestSpan {
			bestName = s.Name
			bestSpan = span
		}
	}
	return bestName
}
