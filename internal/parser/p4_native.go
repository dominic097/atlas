package parser

import (
	"github.com/dominic097/atlas/internal/graph"
	tree_sitter_p4 "github.com/dominic097/atlas/internal/parser/tree_sitter_p4"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func parseP4Native(path string, content []byte) ([]symbolDraft, []string, bool) {
	legacy, imports := parseRegexFallback(path, "p4", content)
	treeDecls, ok := p4TreeSitterDeclarations(content)
	if !ok {
		return legacy, imports, true
	}
	for i := range legacy {
		match, found := p4MatchingTreeDeclaration(legacy[i], treeDecls)
		if !found {
			continue
		}
		if legacy[i].metadata == nil {
			legacy[i].metadata = graph.JSONBMap{}
		}
		legacy[i].metadata["source"] = "tree_sitter_p4"
		if match.signature != "" {
			legacy[i].signature = match.signature
		}
	}
	return legacy, imports, true
}

func p4TreeSitterDeclarations(content []byte) ([]symbolDraft, bool) {
	grammar := tree_sitter.NewLanguage(tree_sitter_p4.Language())
	if grammar == nil {
		return nil, false
	}
	p := tree_sitter.NewParser()
	if err := p.SetLanguage(grammar); err != nil {
		p.Close()
		return nil, false
	}
	tree := p.Parse(content, nil)
	if tree == nil {
		p.Close()
		return nil, false
	}
	defer tree.Close()
	defer p.Close()

	out := make([]symbolDraft, 0)
	walkNode(tree.RootNode(), func(n *tree_sitter.Node) bool {
		if draft, ok := p4DeclarationDraft(n, content); ok {
			out = append(out, draft)
		}
		return true
	})
	return out, true
}

func p4DeclarationDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind := ""
	nameKind := ""
	switch n.Kind() {
	case "parser_definition":
		kind, nameKind = "parser", "method_identifier"
	case "control_definition":
		kind, nameKind = "control", "method_identifier"
	case "package":
		kind, nameKind = "package", "method_identifier"
	case "action":
		kind, nameKind = "action", "method_identifier"
	case "table":
		kind, nameKind = "table", "type_identifier"
	case "header_definition":
		kind, nameKind = "header", "type_identifier"
	case "struct_definition":
		kind, nameKind = "struct", "type_identifier"
	case "extern_definition":
		kind, nameKind = "extern", "type_identifier"
	case "typedef_definition", "type_decl":
		kind, nameKind = "type", "type_identifier"
	case "const_definition":
		kind, nameKind = "constant", "identifier"
	case "state":
		kind, nameKind = "state", "method_identifier"
	default:
		return symbolDraft{}, false
	}
	name := p4FirstNamedChildText(n, content, nameKind)
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		kind:      kind,
		name:      name,
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		signature: p4FirstLine(n, content),
		metadata:  graph.JSONBMap{"source": "tree_sitter_p4"},
	}, true
}

func p4FirstNamedChildText(n *tree_sitter.Node, content []byte, kind string) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child.Kind() == kind {
			return p4IdentifierText(child, content)
		}
	}
	return ""
}

func p4IdentifierText(n *tree_sitter.Node, content []byte) string {
	if n.ChildCount() == 0 {
		return nodeText(n, content)
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child.Kind() == "identifier" {
			return nodeText(child, content)
		}
	}
	return nodeText(n, content)
}

func p4MatchingTreeDeclaration(symbol symbolDraft, treeDecls []symbolDraft) (symbolDraft, bool) {
	for i := range treeDecls {
		if treeDecls[i].kind == symbol.kind && treeDecls[i].name == symbol.name && treeDecls[i].startLine == symbol.startLine {
			return treeDecls[i], true
		}
	}
	for i := range treeDecls {
		if treeDecls[i].kind == symbol.kind && treeDecls[i].name == symbol.name {
			return treeDecls[i], true
		}
	}
	return symbolDraft{}, false
}

func p4FirstLine(n *tree_sitter.Node, content []byte) string {
	text := nodeText(n, content)
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' || text[i] == '{' || text[i] == ';' {
			return text[:i]
		}
	}
	return text
}
