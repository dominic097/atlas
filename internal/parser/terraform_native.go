package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter_hcl "github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func parseTerraformNative(content []byte) ([]symbolDraft, bool) {
	grammar := tree_sitter.NewLanguage(tree_sitter_hcl.Language())
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

	symbols := make([]symbolDraft, 0)
	walkNode(tree.RootNode(), func(n *tree_sitter.Node) bool {
		if n.Kind() != "block" {
			return true
		}
		if draft, ok := terraformBlockDraft(n, content); ok {
			symbols = append(symbols, draft)
		}
		return true
	})
	for _, recovered := range recoverTerraformBlocks(string(content)) {
		found := false
		for _, symbol := range symbols {
			if symbol.kind == recovered.kind && symbol.name == recovered.name && symbol.startLine == recovered.startLine {
				found = true
				break
			}
		}
		if !found {
			symbols = append(symbols, recovered)
		}
	}
	return sortDedupDrafts(symbols), true
}

func terraformBlockDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind, labels := terraformBlockParts(n, content)
	name := terraformSymbolName(kind, labels)
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		kind:      kind,
		name:      name,
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		signature: terraformFirstLine(n, content),
		metadata:  graph.JSONBMap{"source": "tree_sitter_hcl"},
	}, true
}

func terraformBlockParts(n *tree_sitter.Node, content []byte) (string, []string) {
	kind := ""
	labels := make([]string, 0, 2)
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		switch child.Kind() {
		case "identifier":
			text := strings.TrimSpace(nodeText(child, content))
			if kind == "" {
				kind = text
			} else {
				labels = append(labels, text)
			}
		case "string_lit":
			labels = append(labels, terraformStringLabel(child, content))
		}
	}
	return kind, labels
}

func terraformStringLabel(n *tree_sitter.Node, content []byte) string {
	parts := make([]string, 0)
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child.Kind() == "template_literal" {
			parts = append(parts, nodeText(child, content))
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "")
	}
	text := strings.TrimSpace(nodeText(n, content))
	text = strings.TrimPrefix(text, "\"")
	text = strings.TrimSuffix(text, "\"")
	return text
}

func terraformSymbolName(kind string, labels []string) string {
	switch kind {
	case "resource", "data":
		if len(labels) >= 2 && labels[0] != "" && labels[1] != "" {
			return labels[0] + "." + labels[1]
		}
	case "module", "variable", "output":
		if len(labels) >= 1 && labels[0] != "" {
			return labels[0]
		}
	}
	return ""
}

func recoverTerraformBlocks(text string) []symbolDraft {
	lines := strings.Split(text, "\n")
	out := make([]symbolDraft, 0)
	for idx, line := range lines {
		kind, labels, ok := terraformLineBlock(line)
		if !ok {
			continue
		}
		name := terraformSymbolName(kind, labels)
		if name == "" {
			continue
		}
		out = append(out, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: idx + 1,
			endLine:   idx + 1,
			signature: strings.TrimSpace(line),
			metadata:  graph.JSONBMap{"source": "tree_sitter_hcl_recovery"},
		})
	}
	return out
}

func terraformLineBlock(line string) (string, []string, bool) {
	tokens := terraformHeaderTokens(line)
	if len(tokens) < 2 {
		return "", nil, false
	}
	kind := tokens[0]
	switch kind {
	case "resource", "data":
		if len(tokens) < 3 {
			return "", nil, false
		}
		return kind, tokens[1:3], true
	case "module", "variable", "output":
		return kind, tokens[1:2], true
	default:
		return "", nil, false
	}
}

func terraformHeaderTokens(line string) []string {
	tokens := make([]string, 0, 3)
	for i := 0; i < len(line); {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t' || line[i] == '\r') {
			i++
		}
		if i >= len(line) || line[i] == '#' || line[i] == '/' && i+1 < len(line) && line[i+1] == '/' {
			break
		}
		if line[i] == '{' || line[i] == '=' {
			break
		}
		if line[i] == '"' {
			i++
			start := i
			for i < len(line) && line[i] != '"' {
				i++
			}
			tokens = append(tokens, line[start:i])
			if i < len(line) {
				i++
			}
			continue
		}
		start := i
		for i < len(line) && !strings.ContainsRune(" \t\r\n{=", rune(line[i])) {
			i++
		}
		tokens = append(tokens, line[start:i])
	}
	return tokens
}

func terraformFirstLine(n *tree_sitter.Node, content []byte) string {
	text := nodeText(n, content)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
}
