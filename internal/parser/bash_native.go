package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"

	"github.com/dominic097/atlas/internal/graph"
)

func parseBashNative(content []byte) ([]symbolDraft, []string, bool) {
	imports := bashImports(content)
	if len(content) == 0 {
		return nil, imports, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_bash.Language())
	if grammar == nil {
		return nil, imports, false
	}
	p := tree_sitter.NewParser()
	if err := p.SetLanguage(grammar); err != nil {
		p.Close()
		return nil, imports, false
	}
	defer p.Close()

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil, imports, false
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil, imports, false
	}
	candidates := bashLineDrafts(content)
	verified := make(map[string][]symbolDraft)
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		if draft, ok := bashDefinitionDraft(n, content); ok {
			key := strings.ToLower(draft.name)
			verified[key] = append(verified[key], draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	for i := range candidates {
		key := strings.ToLower(candidates[i].name)
		matches := verified[key]
		if len(matches) == 0 {
			candidates[i].metadata = graph.JSONBMap{"source": "tree_sitter_bash_recovery"}
			continue
		}
		ast := matches[0]
		verified[key] = matches[1:]
		candidates[i].endLine = ast.endLine
		candidates[i].metadata = graph.JSONBMap{"source": "tree_sitter_bash"}
	}
	return sortDedupDrafts(candidates), imports, true
}

func bashDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	if n == nil || n.Kind() != "function_definition" {
		return symbolDraft{}, false
	}
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(nameNode, content))
	if !bashIdentifier(name) {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      "function",
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_bash"},
	}, true
}

func bashLineDrafts(content []byte) []symbolDraft {
	text := string(content)
	var drafts []symbolDraft
	offset := 0
	lineNo := 1
	for offset <= len(text) {
		next := offset
		for next < len(text) && text[next] != '\n' && text[next] != '\r' {
			next++
		}
		line := text[offset:next]
		if name := bashLineFunctionName(line); name != "" {
			drafts = append(drafts, symbolDraft{
				name:      name,
				kind:      "function",
				signature: strings.TrimSpace(line),
				startLine: lineNo,
				endLine:   lineNo,
				metadata:  graph.JSONBMap{"source": "tree_sitter_bash"},
			})
		}
		if next >= len(text) {
			break
		}
		if text[next] == '\r' && next+1 < len(text) && text[next+1] == '\n' {
			next++
		}
		offset = next + 1
		lineNo++
	}
	return drafts
}

func bashLineFunctionName(line string) string {
	line = strings.TrimLeft(line, " \t")
	if rest, ok := bashCutKeyword(line, "function"); ok {
		rest = strings.TrimLeft(rest, " \t")
		name, rest := bashReadIdentifier(rest)
		if name == "" {
			return ""
		}
		rest = strings.TrimLeft(rest, " \t")
		if strings.HasPrefix(rest, "()") {
			rest = strings.TrimLeft(rest[2:], " \t")
		}
		if strings.HasPrefix(rest, "{") {
			return name
		}
		return ""
	}
	name, rest := bashReadIdentifier(line)
	if name == "" {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "()") {
		return ""
	}
	rest = strings.TrimLeft(rest[2:], " \t")
	if strings.HasPrefix(rest, "{") {
		return name
	}
	return ""
}

func bashImports(content []byte) []string {
	var imports []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimLeft(line, " \t")
		var rest string
		var ok bool
		if strings.HasPrefix(line, ".") && (len(line) == 1 || line[1] == ' ' || line[1] == '\t') {
			rest, ok = line[1:], true
		} else {
			rest, ok = bashCutKeyword(line, "source")
		}
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if name := bashReadPathToken(rest); name != "" {
			imports = append(imports, name)
		}
	}
	return uniqueStrings(imports)
}

func bashCutKeyword(line, keyword string) (string, bool) {
	if len(line) < len(keyword) || line[:len(keyword)] != keyword {
		return "", false
	}
	if len(line) > len(keyword) && bashIdentByte(line[len(keyword)]) {
		return "", false
	}
	return line[len(keyword):], true
}

func bashReadIdentifier(s string) (string, string) {
	if len(s) == 0 || !bashIdentStartByte(s[0]) {
		return "", s
	}
	i := 1
	for i < len(s) && bashIdentByte(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func bashReadPathToken(s string) string {
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', ';', '\r', '\n':
			return s[:i]
		}
		i++
	}
	return s
}

func bashIdentifier(s string) bool {
	if s == "" || !bashIdentStartByte(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !bashIdentByte(s[i]) {
			return false
		}
	}
	return true
}

func bashIdentStartByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

func bashIdentByte(c byte) bool {
	return bashIdentStartByte(c) || (c >= '0' && c <= '9')
}
