package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter_p4 "github.com/dominic097/atlas/internal/parser/tree_sitter_p4"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func parseP4Native(_ string, content []byte) ([]symbolDraft, []string, bool) {
	imports := parseLightweightImports("p4", content)
	treeDecls, ok := p4TreeSitterDeclarations(content)
	if !ok {
		return nil, imports, false
	}
	treeDecls = append(treeDecls, p4RecoveredDeclarations(content)...)
	return sortDedupDrafts(treeDecls), imports, true
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
		kind, nameKind = p4HeaderKind(n, content), "type_identifier"
	case "struct_definition":
		kind, nameKind = "struct", "type_identifier"
	case "extern_definition":
		kind, nameKind = "extern", "type_identifier"
	case "typedef_definition", "type_decl":
		kind, nameKind = p4TypeDeclKind(n, content), "type_identifier"
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

func p4HeaderKind(n *tree_sitter.Node, content []byte) string {
	if strings.HasPrefix(strings.TrimSpace(nodeText(n, content)), "header_union") {
		return "header_union"
	}
	return "header"
}

func p4TypeDeclKind(n *tree_sitter.Node, content []byte) string {
	text := strings.TrimSpace(nodeText(n, content))
	switch {
	case strings.HasPrefix(text, "enum "):
		return "enum"
	case strings.HasPrefix(text, "header_union "):
		return "header_union"
	case strings.HasPrefix(text, "header "):
		return "header"
	default:
		return "type"
	}
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

func p4RecoveredDeclarations(content []byte) []symbolDraft {
	text := string(content)
	var drafts []symbolDraft
	lineStart := 0
	lineNo := 1
	for i := 0; i <= len(text); i++ {
		if i < len(text) && text[i] != '\n' {
			continue
		}
		line := text[lineStart:i]
		if draft, ok := p4RecoveredLineDeclaration(line, lineNo); ok {
			drafts = append(drafts, draft)
		}
		lineStart = i + 1
		lineNo++
	}
	return drafts
}

func p4RecoveredLineDeclaration(line string, lineNo int) (symbolDraft, bool) {
	trimmed := strings.TrimSpace(p4StripLineComment(line))
	if trimmed == "" {
		return symbolDraft{}, false
	}
	kind := ""
	name := ""
	switch {
	case strings.HasPrefix(trimmed, "parser "):
		kind, name = "parser", p4IdentifierAfterKeyword(trimmed, "parser")
	case strings.HasPrefix(trimmed, "control "):
		kind, name = "control", p4IdentifierAfterKeyword(trimmed, "control")
	case strings.HasPrefix(trimmed, "package "):
		kind, name = "package", p4IdentifierAfterKeyword(trimmed, "package")
	case strings.HasPrefix(trimmed, "action "):
		kind, name = "action", p4IdentifierAfterKeyword(trimmed, "action")
	case strings.HasPrefix(trimmed, "table "):
		kind, name = "table", p4IdentifierAfterKeyword(trimmed, "table")
	case strings.HasPrefix(trimmed, "state "):
		kind, name = "state", p4IdentifierAfterKeyword(trimmed, "state")
	case strings.HasPrefix(trimmed, "header_union "):
		kind, name = "header_union", p4IdentifierAfterKeyword(trimmed, "header_union")
	case strings.HasPrefix(trimmed, "header "):
		kind, name = "header", p4IdentifierAfterKeyword(trimmed, "header")
	case strings.HasPrefix(trimmed, "struct "):
		kind, name = "struct", p4IdentifierAfterKeyword(trimmed, "struct")
	case strings.HasPrefix(trimmed, "extern "):
		kind, name = "extern", p4IdentifierAfterKeyword(trimmed, "extern")
	case strings.HasPrefix(trimmed, "enum "):
		kind, name = "enum", p4EnumName(trimmed)
	case strings.HasPrefix(trimmed, "type ") || strings.HasPrefix(trimmed, "typedef "):
		kind, name = "type", p4LastIdentifierBefore(trimmed, ";")
	case strings.HasPrefix(trimmed, "const "):
		kind, name = "constant", p4LastIdentifierBefore(trimmed, "=")
	default:
		return symbolDraft{}, false
	}
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		kind:      kind,
		name:      name,
		startLine: lineNo,
		endLine:   lineNo,
		signature: trimmed,
		metadata:  graph.JSONBMap{"source": "tree_sitter_p4_recovery"},
	}, true
}

func p4StripLineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func p4IdentifierAfterKeyword(line, keyword string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, keyword))
	tokens := p4IdentifierTokens(rest)
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

func p4EnumName(line string) string {
	tokens := p4IdentifierTokens(strings.TrimSpace(strings.TrimPrefix(line, "enum")))
	if len(tokens) == 0 {
		return ""
	}
	if tokens[0] == "bit" && len(tokens) > 1 {
		return tokens[1]
	}
	return tokens[0]
}

func p4LastIdentifierBefore(line, delimiter string) string {
	if delimiter != "" {
		if idx := strings.Index(line, delimiter); idx >= 0 {
			line = line[:idx]
		}
	}
	tokens := p4IdentifierTokens(line)
	if len(tokens) == 0 {
		return ""
	}
	return tokens[len(tokens)-1]
}

func p4IdentifierTokens(text string) []string {
	var tokens []string
	for i := 0; i < len(text); {
		if !p4IdentStart(text[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < len(text) && p4IdentPart(text[i]) {
			i++
		}
		tokens = append(tokens, text[start:i])
	}
	return tokens
}

func p4IdentStart(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func p4IdentPart(b byte) bool {
	return p4IdentStart(b) || (b >= '0' && b <= '9')
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
