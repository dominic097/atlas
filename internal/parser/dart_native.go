package parser

import (
	"strings"

	tree_sitter_dart "github.com/UserNobody14/tree-sitter-dart/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
)

var dartTypeKinds = map[string]bool{
	"class_definition":           true,
	"mixin_declaration":          true,
	"mixin_definition":           true,
	"extension_declaration":      true,
	"extension_type_declaration": true,
	"enum_declaration":           true,
}

func parseDartNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_dart.Language())
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
		if draft, ok := dartDefinitionDraft(n, content); ok {
			drafts = append(drafts, draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), true
}

func dartDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind := ""
	name := ""
	switch {
	case dartTypeKinds[n.Kind()]:
		kind = "type"
		name = dartFirstDirectIdentifier(n, content)
	case n.Kind() == "type_alias":
		kind = "typedef"
		name = dartFirstDirectIdentifierKind(n, content, "type_identifier")
	case n.Kind() == "constructor_signature" || n.Kind() == "constant_constructor_signature" || n.Kind() == "factory_constructor_signature":
		kind = "constructor"
		name = dartConstructorName(n, content)
	case n.Kind() == "function_signature":
		kind = "function"
		name = dartFirstDirectIdentifierKind(n, content, "identifier")
	case n.Kind() == "getter_signature":
		kind = "getter"
		name = dartFirstDirectIdentifierKind(n, content, "identifier")
	case n.Kind() == "setter_signature":
		kind = "setter"
		name = dartFirstDirectIdentifierKind(n, content, "identifier")
	default:
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
		metadata:  graph.JSONBMap{"source": "tree_sitter_dart"},
	}, true
}

func dartFirstDirectIdentifier(n *tree_sitter.Node, content []byte) string {
	if name := dartFirstDirectIdentifierKind(n, content, "identifier"); name != "" {
		return name
	}
	return dartFirstDirectIdentifierKind(n, content, "type_identifier")
}

func dartFirstDirectIdentifierKind(n *tree_sitter.Node, content []byte, kind string) string {
	if n == nil {
		return ""
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.Kind() == kind {
			if value := strings.TrimSpace(nodeText(child, content)); value != "" {
				return value
			}
		}
	}
	return ""
}

func dartConstructorName(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	names := make([]string, 0, 2)
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil || child.Kind() != "identifier" {
			continue
		}
		if value := strings.TrimSpace(nodeText(child, content)); value != "" {
			names = append(names, value)
		}
	}
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	return names[0] + "." + names[1]
}
