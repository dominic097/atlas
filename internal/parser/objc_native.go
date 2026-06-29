package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter_objc "github.com/dominic097/atlas/internal/parser/tree_sitter_objc"
)

var objcTypeKinds = map[string]bool{
	"class_interface":         true,
	"class_implementation":    true,
	"protocol_declaration":    true,
	"category_interface":      true,
	"category_implementation": true,
}

func parseObjCNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_objc.Language())
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
		switch {
		case objcTypeKinds[n.Kind()]:
			if name := objcFirstIdentifier(n, content); name != "" {
				drafts = append(drafts, objcDraft("type", name, n, content))
			}
		case n.Kind() == "method_declaration" || n.Kind() == "method_definition":
			if name := objcSelectorName(n, content); name != "" {
				drafts = append(drafts, objcDraft("method", name, n, content))
			}
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), true
}

func objcDraft(kind, name string, n *tree_sitter.Node, content []byte) symbolDraft {
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_objc"},
	}
}

func objcFirstIdentifier(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child != nil && child.Kind() == "identifier" {
			if value := strings.TrimSpace(nodeText(child, content)); value != "" {
				return value
			}
		}
	}
	return ""
}

func objcSelectorName(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	var names []string
	hasParameter := false
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "identifier":
			if value := strings.TrimSpace(nodeText(child, content)); value != "" {
				names = append(names, value)
			}
		case "method_parameter":
			hasParameter = true
		}
	}
	if len(names) == 0 {
		return ""
	}
	if hasParameter {
		return strings.Join(names, ":") + ":"
	}
	return names[0]
}
