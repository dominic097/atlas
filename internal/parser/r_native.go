package parser

import (
	"strings"

	tree_sitter_r "github.com/r-lib/tree-sitter-r/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
)

func parseRNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_r.Language())
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
		if draft, ok := rDefinitionDraft(n, content); ok {
			drafts = append(drafts, draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), true
}

func rDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	if !rStartsLogicalLine(n, content) {
		return symbolDraft{}, false
	}
	switch n.Kind() {
	case "binary_operator":
		return rAssignmentDraft(n, content)
	case "argument":
		return rArgumentDraft(n, content)
	case "parameter":
		return rParameterDraft(n, content)
	case "call":
		if rCallFunctionName(n, content) == "setClass" {
			if name := rFirstStringArgument(n, content); name != "" {
				return rDraft("type", name, n, content), true
			}
		}
	}
	return symbolDraft{}, false
}

func rArgumentDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	nameNode := n.ChildByFieldName("name")
	value := n.ChildByFieldName("value")
	if nameNode == nil || value == nil {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(nameNode, content))
	if !rCounterName(name) {
		return symbolDraft{}, false
	}
	switch {
	case value.Kind() == "function_definition":
		return rDraft("function", name, n, content), true
	case rVariableValue(value, content):
		return rDraft("variable", name, n, content), true
	}
	return symbolDraft{}, false
}

func rParameterDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	nameNode := n.ChildByFieldName("name")
	value := n.ChildByFieldName("default")
	if nameNode == nil || value == nil {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(nameNode, content))
	if !rCounterName(name) {
		return symbolDraft{}, false
	}
	switch {
	case value.Kind() == "function_definition":
		return rDraft("function", name, n, content), true
	case rVariableValue(value, content):
		return rDraft("variable", name, n, content), true
	}
	return symbolDraft{}, false
}

func rAssignmentDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	lhs := n.ChildByFieldName("lhs")
	rhs := n.ChildByFieldName("rhs")
	op := rOperator(n)
	if lhs == nil || rhs == nil || lhs.Kind() != "identifier" {
		return symbolDraft{}, false
	}
	if op != "<-" && op != "=" && op != "<<-" {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(lhs, content))
	if !rCounterName(name) {
		return symbolDraft{}, false
	}
	switch {
	case rhs.Kind() == "function_definition":
		return rDraft("function", name, n, content), true
	case op == "<<-":
		return symbolDraft{}, false
	case rhs.Kind() == "call":
		switch rCallFunctionName(rhs, content) {
		case "R6::R6Class", "ggproto":
			if rFirstStringArgument(rhs, content) != "" {
				return rDraft("type", name, n, content), true
			}
		}
	}
	if rVariableValue(rhs, content) {
		return rDraft("variable", name, n, content), true
	}
	return symbolDraft{}, false
}

func rVariableValue(n *tree_sitter.Node, content []byte) bool {
	if n == nil {
		return false
	}
	value := strings.TrimSpace(nodeText(n, content))
	if value != "" {
		first := value[0]
		if first == '\'' || first == '"' || first == '[' || (first >= '0' && first <= '9') {
			return true
		}
		if rCounterCallPrefix(value) {
			return true
		}
	}
	switch n.Kind() {
	case "string", "integer", "float", "complex":
		return true
	case "call":
		switch rCallFunctionName(n, content) {
		case "new.env", "c", "list", "data.frame", "tibble":
			return true
		}
	}
	return false
}

func rCounterCallPrefix(value string) bool {
	for _, name := range []string{"new.env", "c", "list", "data.frame", "tibble"} {
		rest, ok := strings.CutPrefix(value, name)
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(rest, " \t"), "(") {
			return true
		}
	}
	return false
}

func rCounterName(name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	if !((first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || first == '.') {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '.' {
			continue
		}
		return false
	}
	return true
}

func rDraft(kind, name string, n *tree_sitter.Node, content []byte) symbolDraft {
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_r"},
	}
}

func rOperator(n *tree_sitter.Node) string {
	if n == nil {
		return ""
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil || child.IsNamed() {
			continue
		}
		switch child.Kind() {
		case "<-", "=", "<<-":
			return child.Kind()
		}
	}
	return ""
}

func rCallFunctionName(n *tree_sitter.Node, content []byte) string {
	if n == nil || n.Kind() != "call" {
		return ""
	}
	fn := n.ChildByFieldName("function")
	if fn == nil && n.ChildCount() > 0 {
		fn = n.Child(0)
	}
	if fn == nil {
		return ""
	}
	return strings.TrimSpace(nodeText(fn, content))
}

func rFirstStringArgument(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	args := n.ChildByFieldName("arguments")
	if args == nil {
		for i := uint(0); i < n.ChildCount(); i++ {
			child := n.Child(i)
			if child != nil && child.Kind() == "arguments" {
				args = child
				break
			}
		}
	}
	if args == nil {
		return ""
	}
	for i := uint(0); i < args.ChildCount(); i++ {
		arg := args.Child(i)
		if arg == nil || arg.Kind() != "argument" {
			continue
		}
		for j := uint(0); j < arg.ChildCount(); j++ {
			child := arg.Child(j)
			if child != nil && child.Kind() == "string" {
				return strings.Trim(strings.TrimSpace(nodeText(child, content)), `"'`)
			}
		}
	}
	return ""
}

func rStartsLogicalLine(n *tree_sitter.Node, content []byte) bool {
	if n == nil {
		return false
	}
	offset := int(n.StartByte())
	if offset > len(content) {
		offset = len(content)
	}
	for i := offset - 1; i >= 0; i-- {
		switch content[i] {
		case '\n', '\r':
			return true
		case ' ', '\t':
			continue
		default:
			return false
		}
	}
	return true
}
