package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_julia "github.com/tree-sitter/tree-sitter-julia/bindings/go"

	"github.com/dominic097/atlas/internal/graph"
)

func parseJuliaNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_julia.Language())
	if grammar == nil {
		return nil, false
	}
	p := tree_sitter.NewParser()
	if err := p.SetLanguage(grammar); err != nil {
		p.Close()
		return nil, false
	}
	defer p.Close()

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil, false
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil, false
	}
	var drafts []symbolDraft
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		if draft, ok := juliaDefinitionDraft(n, content); ok {
			drafts = append(drafts, draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), true
}

func juliaDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind := ""
	name := ""
	switch n.Kind() {
	case "module_definition":
		kind = "module"
		name = juliaFirstIdentifier(firstNonNilNode(n.ChildByFieldName("name"), n), content)
	case "struct_definition", "abstract_definition", "primitive_definition":
		kind = "type"
		name = juliaFirstIdentifier(firstNonNilNode(n.ChildByFieldName("name"), n), content)
	case "function_definition":
		kind = "function"
		sig := n.ChildByFieldName("signature")
		if sig == nil {
			for i := uint(0); i < n.ChildCount(); i++ {
				child := n.Child(i)
				if child == nil {
					continue
				}
				switch child.Kind() {
				case "signature", "call_expression", "identifier", "where_expression":
					sig = child
				}
				if sig != nil {
					break
				}
			}
		}
		name = juliaCallableName(sig, content)
	case "macro_definition":
		kind = "macro"
		name = juliaFirstIdentifier(n, content)
	case "const_statement":
		kind = "constant"
		name = juliaFirstIdentifier(n, content)
	case "assignment":
		left := n.ChildByFieldName("left")
		if left == nil && n.ChildCount() > 0 {
			left = n.Child(0)
		}
		if juliaIsCallableAssignmentLeft(left) {
			kind = "function"
			name = juliaCallableName(left, content)
			break
		}
		if !juliaVariableDefinitionScope(n) {
			return symbolDraft{}, false
		}
		kind = "variable"
		name = juliaVariableName(left, content)
	default:
		return symbolDraft{}, false
	}
	if !juliaStableCallableName(name) && kind == "function" {
		return symbolDraft{}, false
	}
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_julia"},
	}, true
}

func firstNonNilNode(nodes ...*tree_sitter.Node) *tree_sitter.Node {
	for _, n := range nodes {
		if n != nil {
			return n
		}
	}
	return nil
}

func juliaFirstIdentifier(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "identifier", "operator_identifier", "type_identifier":
		return strings.TrimLeft(strings.TrimSpace(nodeText(n, content)), "@")
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		if name := juliaFirstIdentifier(n.Child(i), content); name != "" {
			return name
		}
	}
	return ""
}

func juliaVariableDefinitionScope(n *tree_sitter.Node) bool {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		switch cur.Kind() {
		case "const_statement", "function_definition", "macro_definition", "for_statement",
			"while_statement", "if_statement", "elseif_clause", "else_clause", "let_statement",
			"quote_statement", "do_clause", "try_statement":
			return false
		case "source_file", "module_definition":
			return true
		}
	}
	return false
}

func juliaVariableName(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "identifier", "type_identifier":
		return strings.TrimSpace(nodeText(n, content))
	case "tuple_expression", "open_tuple":
		return ""
	default:
		return ""
	}
}

func juliaCallableName(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "identifier", "operator_identifier", "type_identifier":
		return strings.TrimLeft(strings.TrimSpace(nodeText(n, content)), "@")
	case "field_expression", "qualified_identifier", "scoped_identifier":
		return strings.TrimSpace(nodeText(n, content))
	case "parenthesized_expression":
		if typed := juliaCallableTypeFromParenthesized(n, content); typed != "" {
			return typed
		}
		if n.ChildCount() > 0 {
			return juliaCallableName(n.Child(0), content)
		}
	case "signature", "where_expression", "typed_expression", "parametrized_type_expression", "type_head":
		if n.ChildCount() > 0 {
			return juliaCallableName(n.Child(0), content)
		}
	case "call_expression":
		fn := n.ChildByFieldName("function")
		if fn == nil && n.ChildCount() > 0 {
			fn = n.Child(0)
		}
		return juliaCallableName(fn, content)
	}
	return ""
}

func juliaCallableTypeFromParenthesized(n *tree_sitter.Node, content []byte) string {
	raw := strings.TrimSpace(nodeText(n, content))
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	idx := strings.Index(raw, "::")
	if idx < 0 {
		return ""
	}
	name := strings.TrimSpace(raw[idx+2:])
	for _, marker := range []string{" where ", "{", ")"} {
		if cut := strings.Index(name, marker); cut >= 0 {
			name = strings.TrimSpace(name[:cut])
		}
	}
	return name
}

func juliaIsCallableAssignmentLeft(n *tree_sitter.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind() {
	case "call_expression":
		return true
	case "where_expression", "parametrized_type_expression":
		for i := uint(0); i < n.ChildCount(); i++ {
			if juliaIsCallableAssignmentLeft(n.Child(i)) {
				return true
			}
		}
	}
	return false
}

func juliaStableCallableName(name string) bool {
	return name != "" && !strings.Contains(name, "$")
}
