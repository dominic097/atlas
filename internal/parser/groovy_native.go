package parser

import (
	"strings"

	tree_sitter_groovy "github.com/amaanq/tree-sitter-groovy/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
)

func parseGroovyNative(content []byte) ([]symbolDraft, []string, bool) {
	imports := groovyImports(content)
	if len(content) == 0 {
		return nil, imports, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_groovy.Language())
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
	candidates := groovyLineDrafts(content)
	verified := make(map[string][]symbolDraft)
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		if draft, ok := groovyDefinitionDraft(n, content); ok {
			key := groovyVerificationKey(draft.kind, draft.name)
			verified[key] = append(verified[key], draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	for i := range candidates {
		key := groovyVerificationKey(candidates[i].kind, candidates[i].name)
		matches := verified[key]
		if len(matches) == 0 {
			candidates[i].metadata = graph.JSONBMap{"source": "tree_sitter_groovy_recovery"}
			continue
		}
		ast := matches[0]
		verified[key] = matches[1:]
		candidates[i].endLine = ast.endLine
		candidates[i].metadata = graph.JSONBMap{"source": "tree_sitter_groovy"}
	}
	return sortDedupDrafts(candidates), imports, true
}

func groovyDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind := ""
	switch n.Kind() {
	case "class_declaration":
		kind = "class"
	case "interface_declaration":
		kind = "interface"
	case "enum_declaration":
		kind = "enum"
	case "method_declaration", "constructor_declaration", "compact_constructor_declaration":
		kind = "method"
	case "function_definition":
		kind = "function"
	case "juxt_function_call":
		head, arg := groovyJuxtHeadAndArg(n, content)
		switch head {
		case "trait":
			return groovyNativeDraft("trait", arg, n, content)
		case "task":
			return groovyNativeDraft("task", strings.Trim(arg, `"'`), n, content)
		default:
			return symbolDraft{}, false
		}
	default:
		return symbolDraft{}, false
	}
	name := groovyFirstIdentifier(n, content)
	return groovyNativeDraft(kind, name, n, content)
}

func groovyNativeDraft(kind, name string, n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_groovy"},
	}, true
}

func groovyFirstIdentifier(n *tree_sitter.Node, content []byte) string {
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
		case "identifier", "type_identifier":
			if name := strings.TrimSpace(nodeText(child, content)); name != "" {
				return name
			}
		}
	}
	return ""
}

func groovyJuxtHeadAndArg(n *tree_sitter.Node, content []byte) (string, string) {
	if n == nil {
		return "", ""
	}
	parts := make([]string, 0, 2)
	for i := uint(0); i < n.ChildCount() && len(parts) < 2; i++ {
		child := n.Child(i)
		if child == nil || !child.IsNamed() {
			continue
		}
		value := strings.TrimSpace(nodeText(child, content))
		if value == "" {
			continue
		}
		if fields := strings.Fields(value); len(fields) > 0 {
			value = fields[0]
		}
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func groovyVerificationKey(kind, name string) string {
	return kind + "\x00" + strings.ToLower(name)
}

func groovyLineDrafts(content []byte) []symbolDraft {
	text := string(content)
	scanText := maskGroovyNonCode(text)
	var drafts []symbolDraft
	offset := 0
	lineNo := 1
	for offset <= len(scanText) {
		next := offset
		for next < len(scanText) && scanText[next] != '\n' && scanText[next] != '\r' {
			next++
		}
		line := scanText[offset:next]
		if draft, ok := groovyLineDraft(line, lineNo, offset, scanText); ok {
			drafts = append(drafts, draft)
		}
		if next >= len(scanText) {
			break
		}
		if scanText[next] == '\r' && next+1 < len(scanText) && scanText[next+1] == '\n' {
			next++
		}
		offset = next + 1
		lineNo++
	}
	return drafts
}

func groovyLineDraft(line string, lineNo, offset int, text string) (symbolDraft, bool) {
	scan := groovyStripAnnotationsAndModifiers(line)
	if kind, name := groovyTypeLine(scan); name != "" {
		return groovyLineSymbol(kind, name, line, lineNo, offset, text), true
	}
	if name := groovyMethodLine(scan); name != "" {
		return groovyLineSymbol("method", name, line, lineNo, offset, text), true
	}
	if name := groovyConstructorLine(scan); name != "" {
		return groovyLineSymbol("method", name, line, lineNo, offset, text), true
	}
	if name := groovyTaskLine(line); name != "" {
		return groovyLineSymbol("task", name, line, lineNo, offset, text), true
	}
	if name := groovyTasksCallName(line); name != "" {
		return groovyLineSymbol("task", name, line, lineNo, offset, text), true
	}
	return symbolDraft{}, false
}

func groovyLineSymbol(kind, name, line string, lineNo, offset int, text string) symbolDraft {
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: strings.TrimSpace(line),
		startLine: lineNo,
		endLine:   blockEndLine(text, offset, lineNo),
		metadata:  graph.JSONBMap{"source": "tree_sitter_groovy"},
	}
}

func groovyStripAnnotationsAndModifiers(line string) string {
	s := strings.TrimLeft(line, " \t")
	for {
		start := s
		if strings.HasPrefix(s, "@") {
			s = strings.TrimLeft(groovyConsumeAnnotation(s), " \t")
			if s != start {
				continue
			}
		}
		keyword, rest := groovyReadIdentifier(s)
		if keyword != "" && groovyModifier(keyword) {
			s = strings.TrimLeft(rest, " \t")
			continue
		}
		return s
	}
}

func groovyConsumeAnnotation(s string) string {
	i := 1
	for i < len(s) {
		c := s[i]
		if groovyIdentByte(c) || c == '.' {
			i++
			continue
		}
		break
	}
	if i < len(s) && s[i] == '(' {
		depth := 1
		i++
		for i < len(s) && depth > 0 {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			i++
		}
	}
	return s[i:]
}

func groovyModifier(s string) bool {
	switch s {
	case "public", "private", "protected", "static", "final", "abstract",
		"synchronized", "transient", "volatile", "native", "strictfp":
		return true
	default:
		return false
	}
}

func groovyTypeLine(line string) (string, string) {
	keyword, rest := groovyReadIdentifier(line)
	switch keyword {
	case "class", "interface", "trait", "enum":
	default:
		return "", ""
	}
	rest = strings.TrimLeft(rest, " \t")
	name, _ := groovyReadIdentifier(rest)
	return keyword, name
}

func groovyMethodLine(line string) string {
	ret, rest := groovyReadTypeToken(line)
	if !groovyReturnType(ret) {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	name, rest := groovyReadIdentifier(rest)
	if name == "" {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "(") {
		return ""
	}
	after, ok := groovyAfterBalancedParens(rest)
	if !ok {
		return ""
	}
	after = strings.TrimLeft(after, " \t")
	if after == "" || strings.HasPrefix(after, "{") || strings.HasPrefix(after, ";") {
		return name
	}
	return ""
}

func groovyConstructorLine(line string) string {
	line = strings.TrimLeft(line, " \t")
	if strings.HasPrefix(line, "~") {
		line = strings.TrimLeft(line[1:], " \t")
	}
	name, rest := groovyReadIdentifier(line)
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "(") {
		return ""
	}
	after, ok := groovyAfterBalancedParens(rest)
	if !ok || !strings.HasPrefix(strings.TrimLeft(after, " \t"), "{") {
		return ""
	}
	return name
}

func groovyTaskLine(line string) string {
	line = strings.TrimLeft(line, " \t")
	rest, ok := groovyCutKeyword(line, "task")
	if !ok {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return ""
	}
	if rest[0] == '\'' || rest[0] == '"' {
		return groovyQuoted(rest)
	}
	name, _ := groovyReadTaskIdentifier(rest)
	return name
}

func groovyTasksCallName(line string) string {
	for _, marker := range []string{"tasks.register(", "tasks.named("} {
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimLeft(line[idx+len(marker):], " \t")
		if rest == "" || (rest[0] != '\'' && rest[0] != '"') {
			continue
		}
		if name := groovyQuoted(rest); name != "" {
			return name
		}
	}
	return ""
}

func groovyImports(content []byte) []string {
	var imports []string
	for _, line := range strings.Split(maskGroovyNonCode(string(content)), "\n") {
		line = strings.TrimLeft(line, " \t")
		rest, ok := groovyCutKeyword(line, "import")
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if staticRest, ok := groovyCutKeyword(rest, "static"); ok {
			rest = strings.TrimLeft(staticRest, " \t")
		}
		name := groovyImportName(rest)
		if name != "" {
			imports = append(imports, name)
		}
	}
	return uniqueStrings(imports)
}

func groovyImportName(s string) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if groovyIdentByte(c) || c == '.' || c == '*' {
			i++
			continue
		}
		break
	}
	return strings.TrimSpace(s[:i])
}

func groovyReadTypeToken(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", s
	}
	i := 0
	depth := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ' ', '\t':
			if depth == 0 {
				return s[:i], s[i:]
			}
		}
		i++
	}
	return s, ""
}

func groovyReturnType(s string) bool {
	if s == "" {
		return false
	}
	switch s {
	case "def", "void", "boolean", "byte", "short", "int", "long", "float",
		"double", "char", "String", "File", "Path", "URI", "URL", "Map",
		"List", "Set", "Collection", "Object", "Closure", "GString",
		"BigInteger", "BigDecimal":
		return true
	}
	first := s[0]
	return first >= 'A' && first <= 'Z'
}

func groovyAfterBalancedParens(s string) (string, bool) {
	if !strings.HasPrefix(s, "(") {
		return "", false
	}
	depth := 1
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[i+1:], true
			}
		}
	}
	return "", false
}

func groovyCutKeyword(line, keyword string) (string, bool) {
	if len(line) < len(keyword) || line[:len(keyword)] != keyword {
		return "", false
	}
	if len(line) > len(keyword) && groovyIdentByte(line[len(keyword)]) {
		return "", false
	}
	return line[len(keyword):], true
}

func groovyReadIdentifier(s string) (string, string) {
	if len(s) == 0 || !groovyIdentStartByte(s[0]) {
		return "", s
	}
	i := 1
	for i < len(s) && groovyIdentByte(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func groovyReadTaskIdentifier(s string) (string, string) {
	if len(s) == 0 || !groovyIdentStartByte(s[0]) {
		return "", s
	}
	i := 1
	for i < len(s) {
		c := s[i]
		if groovyIdentByte(c) || c == '-' {
			i++
			continue
		}
		break
	}
	return s[:i], s[i:]
}

func groovyQuoted(s string) string {
	if s == "" || (s[0] != '\'' && s[0] != '"') {
		return ""
	}
	quote := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] == quote {
			return s[1:i]
		}
	}
	return ""
}

func groovyIdentStartByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

func groovyIdentByte(c byte) bool {
	return groovyIdentStartByte(c) || (c >= '0' && c <= '9') || c == '$'
}
