package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter_pascal "github.com/dominic097/atlas/internal/parser/tree_sitter_pascal"
)

func parsePascalNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_pascal.Language())
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
	candidates := pascalLineDrafts(content)
	verified := make(map[string][]symbolDraft)
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		if draft, ok := pascalDefinitionDraft(n, content); ok {
			key := pascalVerificationKey(draft.kind, draft.name)
			verified[key] = append(verified[key], draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	for i := range candidates {
		key := pascalVerificationKey(candidates[i].kind, candidates[i].name)
		matches := verified[key]
		if len(matches) == 0 {
			candidates[i].metadata = graph.JSONBMap{"source": "tree_sitter_pascal_recovery"}
			continue
		}
		ast := matches[0]
		verified[key] = matches[1:]
		candidates[i].endLine = ast.endLine
		candidates[i].metadata = graph.JSONBMap{"source": "tree_sitter_pascal"}
	}
	return sortDedupDrafts(candidates), true
}

func pascalDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	switch n.Kind() {
	case "unit", "program", "library":
		if name := pascalModuleName(n, content); name != "" {
			return pascalDraft("unit", name, n, content), true
		}
	case "declType":
		typeNode := n.ChildByFieldName("type")
		if !pascalRecordLikeType(typeNode) {
			return symbolDraft{}, false
		}
		if name := pascalName(n.ChildByFieldName("name"), content); name != "" {
			return pascalDraft("type", name, n, content), true
		}
	case "declProc":
		if !pascalProcedureLike(n) {
			return symbolDraft{}, false
		}
		if name := pascalName(n.ChildByFieldName("name"), content); name != "" {
			return pascalDraft("function", name, n, content), true
		}
	}
	return symbolDraft{}, false
}

func pascalDraft(kind, name string, n *tree_sitter.Node, content []byte) symbolDraft {
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_pascal"},
	}
}

func pascalModuleName(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child != nil && child.Kind() == "moduleName" {
			return strings.TrimSpace(nodeText(child, content))
		}
	}
	return ""
}

func pascalRecordLikeType(n *tree_sitter.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind() {
	case "declClass", "declIntf", "declHelper":
		return true
	}
	return false
}

func pascalProcedureLike(n *tree_sitter.Node) bool {
	if n == nil {
		return false
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "kProcedure", "kFunction", "kConstructor", "kDestructor":
			return true
		}
	}
	return false
}

func pascalName(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "identifier", "operatorName":
		return strings.TrimSpace(nodeText(n, content))
	case "genericDot":
		lhs := pascalName(n.ChildByFieldName("lhs"), content)
		rhs := pascalName(n.ChildByFieldName("rhs"), content)
		if lhs == "" {
			return rhs
		}
		if rhs == "" {
			return lhs
		}
		return lhs + "." + rhs
	case "genericTpl":
		return pascalName(n.ChildByFieldName("entity"), content)
	default:
		return strings.TrimSpace(nodeText(n, content))
	}
}

func pascalVerificationKey(kind, name string) string {
	return kind + "\x00" + strings.ToLower(name)
}

func pascalLineDrafts(content []byte) []symbolDraft {
	var drafts []symbolDraft
	offset := 0
	lineNo := 1
	for offset <= len(content) {
		next := offset
		for next < len(content) && content[next] != '\n' && content[next] != '\r' {
			next++
		}
		line := string(content[offset:next])
		if draft, ok := pascalLineDraft(line, lineNo); ok {
			drafts = append(drafts, draft)
		}
		if next >= len(content) {
			break
		}
		if content[next] == '\r' && next+1 < len(content) && content[next+1] == '\n' {
			next++
		}
		offset = next + 1
		lineNo++
	}
	return drafts
}

func pascalLineDraft(line string, lineNo int) (symbolDraft, bool) {
	for _, keyword := range []string{"unit", "program", "library", "package"} {
		if name := pascalKeywordName(line, keyword, false); name != "" {
			return pascalLineSymbol("unit", name, line, lineNo), true
		}
	}
	if name := pascalTypeLineName(line); name != "" {
		return pascalLineSymbol("type", name, line, lineNo), true
	}
	if name := pascalFunctionLineName(line); name != "" {
		return pascalLineSymbol("function", name, line, lineNo), true
	}
	return symbolDraft{}, false
}

func pascalLineSymbol(kind, name, line string, lineNo int) symbolDraft {
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: strings.TrimSpace(line),
		startLine: lineNo,
		endLine:   lineNo,
		metadata:  graph.JSONBMap{"source": "tree_sitter_pascal"},
	}
}

func pascalKeywordName(line, keyword string, dotted bool) string {
	line = strings.TrimLeft(line, " \t")
	rest, ok := pascalCutKeyword(line, keyword)
	if !ok {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if dotted {
		name, _ := pascalReadDottedIdentifier(rest)
		return name
	}
	name, _ := pascalReadIdentifier(rest)
	return name
}

func pascalTypeLineName(line string) string {
	line = strings.TrimLeft(line, " \t")
	name, rest := pascalReadIdentifier(line)
	if name == "" {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "=") {
		return ""
	}
	rest = strings.TrimLeft(rest[1:], " \t")
	for _, keyword := range []string{"class", "record", "interface", "object"} {
		if _, ok := pascalCutKeyword(rest, keyword); ok {
			return name
		}
	}
	return ""
}

func pascalFunctionLineName(line string) string {
	line = strings.TrimLeft(line, " \t")
	if rest, ok := pascalCutKeyword(line, "class"); ok {
		line = strings.TrimLeft(rest, " \t")
	}
	for _, keyword := range []string{"procedure", "function", "constructor", "destructor"} {
		if name := pascalKeywordName(line, keyword, true); name != "" {
			return name
		}
	}
	return ""
}

func pascalCutKeyword(line, keyword string) (string, bool) {
	if len(line) < len(keyword) || !strings.EqualFold(line[:len(keyword)], keyword) {
		return "", false
	}
	if len(line) > len(keyword) && pascalIdentByte(line[len(keyword)]) {
		return "", false
	}
	return line[len(keyword):], true
}

func pascalReadDottedIdentifier(s string) (string, string) {
	name, rest := pascalReadIdentifier(s)
	if name == "" {
		return "", s
	}
	var b strings.Builder
	b.WriteString(name)
	for strings.HasPrefix(rest, ".") {
		next, remainder := pascalReadIdentifier(rest[1:])
		if next == "" {
			break
		}
		b.WriteByte('.')
		b.WriteString(next)
		rest = remainder
	}
	return b.String(), rest
}

func pascalReadIdentifier(s string) (string, string) {
	if len(s) == 0 || !pascalIdentStartByte(s[0]) {
		return "", s
	}
	i := 1
	for i < len(s) && pascalIdentByte(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func pascalIdentStartByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

func pascalIdentByte(c byte) bool {
	return pascalIdentStartByte(c) || (c >= '0' && c <= '9')
}
