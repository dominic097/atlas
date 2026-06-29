package parser

import (
	"path/filepath"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
)

func parseAstroNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	text := string(content)
	symbols := []symbolDraft{{
		kind:      "component",
		name:      strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		startLine: 1,
		endLine:   1,
		metadata:  graph.JSONBMap{"source": "astro_file"},
	}}
	frontmatter, frontmatterLine := astroFrontmatter(text)
	imports := []string(nil)
	if frontmatter != "" {
		frontmatterSymbols, frontmatterImports, ok := astroFrontmatterSymbols([]byte(frontmatter), frontmatterLine)
		if !ok {
			return nil, nil, false
		}
		symbols = append(symbols, frontmatterSymbols...)
		imports = append(imports, frontmatterImports...)
	}
	symbols = append(symbols, astroComponentTagSymbols(text)...)
	return sortDedupDrafts(symbols), uniqueStrings(imports), true
}

func astroFrontmatterSymbols(content []byte, startLine int) ([]symbolDraft, []string, bool) {
	ptr := languagePointer("typescript")
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

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil, nil, false
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil, nil, false
	}
	imports := extractJSImportsFromText(string(content))
	var symbols []symbolDraft
	walkNode(root, func(node *tree_sitter.Node) bool {
		if node == nil {
			return true
		}
		switch node.Kind() {
		case "function_declaration":
			if astroFunctionLine(content, node) {
				if sym, ok := astroFunctionSymbol(content, startLine, node); ok {
					symbols = append(symbols, sym)
				}
			}
			return true
		case "lexical_declaration", "variable_declaration":
			if astroVariableLine(content, node) {
				symbols = append(symbols, astroVariableSymbols(content, startLine, node)...)
			}
			return true
		default:
			return true
		}
	})
	return symbols, imports, true
}

func astroFunctionSymbol(content []byte, startLine int, node *tree_sitter.Node) (symbolDraft, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(nameNode, content))
	if name == "" {
		return symbolDraft{}, false
	}
	return astroFrontmatterSymbol(content, startLine, node, "function", name), true
}

func astroVariableSymbols(content []byte, startLine int, node *tree_sitter.Node) []symbolDraft {
	var out []symbolDraft
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		if nameNode.Kind() == "identifier" {
			name := strings.TrimSpace(nodeText(nameNode, content))
			if name != "" {
				out = append(out, astroFrontmatterSymbol(content, startLine, child, "variable", name))
			}
			continue
		}
		if nameNode.Kind() == "object_pattern" {
			names := astroDestructureNames(strings.Trim(strings.TrimSpace(nodeText(nameNode, content)), "{}"))
			for _, name := range names {
				out = append(out, astroFrontmatterSymbol(content, startLine, child, "variable", name))
			}
		}
	}
	return out
}

func astroFrontmatterSymbol(content []byte, startLine int, node *tree_sitter.Node, kind, name string) symbolDraft {
	return symbolDraft{
		kind:      kind,
		name:      name,
		signature: vueOneLineSignature(node, content),
		startLine: startLine + int(node.StartPosition().Row),
		endLine:   startLine + int(node.EndPosition().Row),
		metadata:  graph.JSONBMap{"source": "tree_sitter_astro_frontmatter"},
	}
}

func astroComponentTagSymbols(text string) []symbolDraft {
	var symbols []symbolDraft
	for i := 0; i < len(text); i++ {
		if text[i] != '<' || i+1 >= len(text) || text[i+1] < 'A' || text[i+1] > 'Z' {
			continue
		}
		start := i + 1
		end := start
		for end < len(text) && astroComponentNameByte(text[end]) {
			end++
		}
		if end == start {
			continue
		}
		symbols = append(symbols, symbolDraft{
			kind:      "component",
			name:      text[start:end],
			startLine: lineForOffset(text, i),
			endLine:   lineForOffset(text, i),
			metadata:  graph.JSONBMap{"source": "astro_component_tag"},
		})
	}
	return symbols
}

func astroFunctionLine(content []byte, node *tree_sitter.Node) bool {
	line := strings.TrimLeft(vueLineAt(content, int(node.StartPosition().Row)+1), " \t")
	for _, prefix := range []string{"function", "async function", "export function", "export async function"} {
		if astroLineHasKeywordPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func astroVariableLine(content []byte, node *tree_sitter.Node) bool {
	line := strings.TrimLeft(vueLineAt(content, int(node.StartPosition().Row)+1), " \t")
	for _, prefix := range []string{"const", "let", "var", "export const", "export let", "export var"} {
		if astroLineHasKeywordPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func astroLineHasKeywordPrefix(line, prefix string) bool {
	if len(line) < len(prefix) || line[:len(prefix)] != prefix {
		return false
	}
	if len(line) == len(prefix) {
		return true
	}
	return !vueIdentifierByte(line[len(prefix)])
}

func astroComponentNameByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '.' || c == ':' || c == '-'
}
