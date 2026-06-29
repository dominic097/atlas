package parser

import (
	"strings"

	tree_sitter_fortran "github.com/stadelmanma/tree-sitter-fortran/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
)

func parseFortranNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_fortran.Language())
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
		if draft, ok := fortranDefinitionDraft(n, content); ok {
			drafts = append(drafts, draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), true
}

func fortranDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	var kind string
	switch n.Kind() {
	case "module":
		kind = "module"
	case "program":
		kind = "program"
	case "derived_type_definition":
		kind = "type"
	case "function", "subroutine":
		kind = "function"
	default:
		return symbolDraft{}, false
	}
	stmt := fortranFirstStatement(n)
	name := fortranNameFrom(stmt, content)
	if name == "" {
		return symbolDraft{}, false
	}
	return fortranDraft(kind, name, stmt, n, content), true
}

func fortranFirstStatement(n *tree_sitter.Node) *tree_sitter.Node {
	if n == nil {
		return nil
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child != nil && strings.HasSuffix(child.Kind(), "_statement") {
			return child
		}
	}
	return n
}

func fortranNameFrom(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	if child := n.ChildByFieldName("name"); child != nil {
		return strings.TrimSpace(nodeText(child, content))
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "name", "identifier", "type_name":
			return strings.TrimSpace(nodeText(child, content))
		}
	}
	return ""
}

func fortranDraft(kind, name string, stmt, block *tree_sitter.Node, content []byte) symbolDraft {
	if stmt == nil {
		stmt = block
	}
	if block == nil {
		block = stmt
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(stmt, content),
		startLine: int(stmt.StartPosition().Row) + 1,
		endLine:   int(block.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_fortran"},
	}
}
