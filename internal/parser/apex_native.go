package parser

import (
	"github.com/dominic097/atlas/internal/graph"
	tree_sitter_sfapex "github.com/dominic097/atlas/internal/parser/tree_sitter_sfapex"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func parseApexNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	legacy, imports := parseApexRegex(path, content)
	treeDecls, ok := apexTreeSitterDeclarations(content)
	if !ok {
		return legacy, imports, true
	}
	for i := range legacy {
		match, found := apexMatchingTreeDeclaration(legacy[i], treeDecls)
		if !found {
			continue
		}
		if legacy[i].metadata == nil {
			legacy[i].metadata = graph.JSONBMap{}
		}
		legacy[i].metadata["source"] = "tree_sitter_sfapex"
		if match.signature != "" {
			legacy[i].signature = match.signature
		}
	}
	return legacy, imports, true
}

func apexTreeSitterDeclarations(content []byte) ([]symbolDraft, bool) {
	grammar := tree_sitter.NewLanguage(tree_sitter_sfapex.Language())
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
		if draft, ok := apexDeclarationDraft(n, content); ok {
			out = append(out, draft)
		}
		return true
	})
	return out, true
}

func apexDeclarationDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind := ""
	switch n.Kind() {
	case "class_declaration", "interface_declaration", "enum_declaration":
		kind = "type"
	case "method_declaration":
		kind = "method"
	case "constructor_declaration":
		kind = "constructor"
	case "trigger_declaration":
		kind = "trigger"
	default:
		return symbolDraft{}, false
	}
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return symbolDraft{}, false
	}
	name := nodeText(nameNode, content)
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		kind:      kind,
		name:      name,
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		signature: apexTreeSignature(n, content),
		metadata:  graph.JSONBMap{"source": "tree_sitter_sfapex"},
	}, true
}

func apexMatchingTreeDeclaration(symbol symbolDraft, treeDecls []symbolDraft) (symbolDraft, bool) {
	switch symbol.kind {
	case "type", "method", "constructor", "trigger":
	default:
		return symbolDraft{}, false
	}
	var fallback *symbolDraft
	for i := range treeDecls {
		if treeDecls[i].kind != symbol.kind || treeDecls[i].name != symbol.name {
			continue
		}
		if treeDecls[i].startLine == symbol.startLine {
			return treeDecls[i], true
		}
		if fallback == nil {
			fallback = &treeDecls[i]
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	return symbolDraft{}, false
}

func apexTreeSignature(n *tree_sitter.Node, content []byte) string {
	text := nodeText(n, content)
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n', '{':
			return text[:i]
		}
	}
	return text
}
