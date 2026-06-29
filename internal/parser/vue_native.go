package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
)

type vueScriptBlock struct {
	content  []byte
	language string
	line     int
}

func parseVueNative(content []byte) ([]symbolDraft, []string, bool) {
	blocks := vueScriptBlocks(content)
	if len(blocks) == 0 {
		return nil, nil, true
	}
	var symbols []symbolDraft
	var imports []string
	for _, block := range blocks {
		blockSymbols, blockImports, ok := vueScriptSymbols(block)
		if !ok {
			return nil, nil, false
		}
		symbols = append(symbols, blockSymbols...)
		imports = append(imports, blockImports...)
	}
	return sortDedupDrafts(symbols), uniqueStrings(imports), true
}

func vueScriptBlocks(content []byte) []vueScriptBlock {
	text := string(content)
	lower := strings.ToLower(text)
	var blocks []vueScriptBlock
	offset := 0
	for offset < len(text) {
		idx := strings.Index(lower[offset:], "<script")
		if idx < 0 {
			break
		}
		start := offset + idx
		afterName := start + len("<script")
		if afterName < len(text) && vueTagNameByte(text[afterName]) {
			offset = afterName
			continue
		}
		tagEnd := strings.IndexByte(text[afterName:], '>')
		if tagEnd < 0 {
			break
		}
		tagEnd += afterName
		tag := text[afterName:tagEnd]
		closeIdx := strings.Index(lower[tagEnd+1:], "</script>")
		if closeIdx < 0 {
			break
		}
		contentStart := tagEnd + 1
		contentEnd := tagEnd + 1 + closeIdx
		if !vueTagHasAttr(tag, "src") {
			blocks = append(blocks, vueScriptBlock{
				content:  []byte(text[contentStart:contentEnd]),
				language: vueScriptLanguage(tag),
				line:     1 + strings.Count(text[:contentStart], "\n"),
			})
		}
		offset = contentEnd + len("</script>")
	}
	return blocks
}

func vueScriptSymbols(block vueScriptBlock) ([]symbolDraft, []string, bool) {
	ptr := languagePointer(block.language)
	if ptr == nil {
		return nil, nil, false
	}
	lang := tree_sitter.NewLanguage(ptr)
	if lang == nil {
		return nil, nil, false
	}
	p := tree_sitter.NewParser()
	if err := p.SetLanguage(lang); err != nil {
		p.Close()
		return nil, nil, false
	}
	defer p.Close()

	tree := p.Parse(block.content, nil)
	if tree == nil {
		return nil, nil, false
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil, nil, false
	}
	imports := extractJSImportsFromText(string(block.content))
	var symbols []symbolDraft
	walkNode(root, func(node *tree_sitter.Node) bool {
		if node == nil {
			return true
		}
		switch node.Kind() {
		case "function_declaration":
			if vueNodeLineStarts(block.content, node, "function") {
				if sym, ok := vueFunctionDeclarationSymbol(block, node); ok {
					symbols = append(symbols, sym)
				}
			}
			return true
		case "lexical_declaration":
			if vueNodeLineStarts(block.content, node, "const") || vueNodeLineStarts(block.content, node, "let") {
				if sym, ok := vueLexicalDeclarationSymbol(block, node); ok {
					symbols = append(symbols, sym)
				}
			}
			return true
		default:
			return true
		}
	})
	return symbols, imports, true
}

func vueFunctionDeclarationSymbol(block vueScriptBlock, node *tree_sitter.Node) (symbolDraft, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(nameNode, block.content))
	if name == "" {
		return symbolDraft{}, false
	}
	return vueScriptSymbol(block, node, name), true
}

func vueLexicalDeclarationSymbol(block vueScriptBlock, node *tree_sitter.Node) (symbolDraft, bool) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil || nameNode.Kind() != "identifier" {
			return symbolDraft{}, false
		}
		name := strings.TrimSpace(nodeText(nameNode, block.content))
		if name == "" {
			return symbolDraft{}, false
		}
		return vueScriptSymbol(block, child, name), true
	}
	return symbolDraft{}, false
}

func vueScriptSymbol(block vueScriptBlock, node *tree_sitter.Node, name string) symbolDraft {
	return symbolDraft{
		name:      name,
		kind:      "function",
		signature: vueOneLineSignature(node, block.content),
		startLine: block.line + int(node.StartPosition().Row),
		endLine:   block.line + int(node.EndPosition().Row),
		metadata: graph.JSONBMap{
			"source":          "tree_sitter_vue_script",
			"script_language": block.language,
		},
	}
}

func vueOneLineSignature(node *tree_sitter.Node, content []byte) string {
	sig := strings.TrimSpace(nodeText(node, content))
	if nl := strings.IndexByte(sig, '\n'); nl >= 0 {
		sig = strings.TrimSpace(sig[:nl])
	}
	if len(sig) > 160 {
		return sig[:160] + "..."
	}
	return sig
}

func vueNodeLineStarts(content []byte, node *tree_sitter.Node, keyword string) bool {
	line := vueLineAt(content, int(node.StartPosition().Row)+1)
	line = strings.TrimLeft(line, " \t")
	if len(line) < len(keyword) || line[:len(keyword)] != keyword {
		return false
	}
	if len(line) == len(keyword) {
		return true
	}
	return !vueIdentifierByte(line[len(keyword)])
}

func vueLineAt(content []byte, lineNo int) string {
	if lineNo <= 0 {
		return ""
	}
	line := 1
	start := 0
	for i, b := range content {
		if b == '\n' {
			if line == lineNo {
				return string(content[start:i])
			}
			line++
			start = i + 1
		}
	}
	if line == lineNo {
		return string(content[start:])
	}
	return ""
}

func vueScriptLanguage(tag string) string {
	lang := strings.ToLower(vueAttrValue(tag, "lang"))
	switch lang {
	case "ts", "tsx", "typescript":
		return "typescript"
	default:
		return "javascript"
	}
}

func vueTagHasAttr(tag, attr string) bool {
	_, ok := vueAttrValueOK(tag, attr)
	return ok
}

func vueAttrValue(tag, attr string) string {
	value, _ := vueAttrValueOK(tag, attr)
	return value
}

func vueAttrValueOK(tag, attr string) (string, bool) {
	i := 0
	for i < len(tag) {
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t' || tag[i] == '\r' || tag[i] == '\n' || tag[i] == '/') {
			i++
		}
		start := i
		for i < len(tag) && (vueIdentifierByte(tag[i]) || tag[i] == ':') {
			i++
		}
		if start == i {
			i++
			continue
		}
		name := tag[start:i]
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t' || tag[i] == '\r' || tag[i] == '\n') {
			i++
		}
		value := ""
		if i < len(tag) && tag[i] == '=' {
			i++
			for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t' || tag[i] == '\r' || tag[i] == '\n') {
				i++
			}
			if i < len(tag) && (tag[i] == '\'' || tag[i] == '"') {
				quote := tag[i]
				i++
				valueStart := i
				for i < len(tag) && tag[i] != quote {
					i++
				}
				value = tag[valueStart:i]
				if i < len(tag) {
					i++
				}
			} else {
				valueStart := i
				for i < len(tag) && tag[i] != ' ' && tag[i] != '\t' && tag[i] != '\r' && tag[i] != '\n' && tag[i] != '/' {
					i++
				}
				value = tag[valueStart:i]
			}
		}
		if strings.EqualFold(name, attr) {
			return value, true
		}
	}
	return "", false
}

func vueTagNameByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-'
}

func vueIdentifierByte(c byte) bool {
	return vueTagNameByte(c) || c == '$'
}
