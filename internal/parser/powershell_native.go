package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter_powershell "github.com/dominic097/atlas/internal/parser/tree_sitter_powershell"
)

func parsePowerShellNative(content []byte) ([]symbolDraft, []string, bool) {
	imports := powerShellImports(content)
	if len(content) == 0 {
		return nil, imports, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_powershell.Language())
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
	recoveredFunctions, recoveredFunctionRanges := powerShellRecoveredFunctions(content, root)
	drafts := append([]symbolDraft{}, recoveredFunctions...)
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		if draft, ok := powerShellVariableDefinitionDraft(n, content, recoveredFunctionRanges); ok {
			drafts = append(drafts, draft)
		}
		if draft, ok := powerShellDefinitionDraft(n, content); ok {
			drafts = append(drafts, draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), imports, true
}

func powerShellDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind := ""
	name := ""
	switch n.Kind() {
	case "function_statement":
		kind = "function"
		name = powerShellFirstChildText(n, content, "function_name")
	case "class_statement":
		kind = "class"
		name = powerShellFirstChildText(n, content, "simple_name")
	case "class_method_definition":
		kind = "method"
		name = powerShellFirstChildText(n, content, "simple_name")
	default:
		return symbolDraft{}, false
	}
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
		metadata:  graph.JSONBMap{"source": "tree_sitter_powershell"},
	}, true
}

type powerShellLineRange struct {
	start int
	end   int
}

func powerShellVariableDefinitionDraft(n *tree_sitter.Node, content []byte, recoveredFunctionRanges []powerShellLineRange) (symbolDraft, bool) {
	if n == nil || n.Kind() != "assignment_expression" || powerShellInsideDefinitionBody(n) ||
		powerShellLineInRanges(int(n.StartPosition().Row)+1, recoveredFunctionRanges) {
		return symbolDraft{}, false
	}
	name := powerShellVariableAssignmentName(n, content)
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      "variable",
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_powershell"},
	}, true
}

func powerShellLineInRanges(line int, ranges []powerShellLineRange) bool {
	for _, r := range ranges {
		if line >= r.start && line <= r.end {
			return true
		}
	}
	return false
}

func powerShellInsideDefinitionBody(n *tree_sitter.Node) bool {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		switch cur.Kind() {
		case "function_statement", "class_statement", "class_method_definition", "script_block":
			return true
		}
	}
	return false
}

func powerShellRecoveredFunctions(content []byte, root *tree_sitter.Node) ([]symbolDraft, []powerShellLineRange) {
	if root == nil {
		return nil, nil
	}
	lines := strings.Split(string(content), "\n")
	var drafts []symbolDraft
	var ranges []powerShellLineRange
	hereEnd := ""
	blockComment := false
	for i, line := range lines {
		scan := powerShellScannerLine(line, &hereEnd, &blockComment)
		name, ok := powerShellFunctionHeaderName(scan)
		if !ok {
			continue
		}
		startLine := i + 1
		endLine := powerShellFunctionEndLine(lines, i)
		drafts = append(drafts, symbolDraft{
			name:      name,
			kind:      "function",
			signature: strings.TrimSpace(line),
			startLine: startLine,
			endLine:   startLine,
			metadata:  graph.JSONBMap{"source": "tree_sitter_powershell"},
		})
		ranges = append(ranges, powerShellLineRange{start: startLine, end: endLine})
	}
	return drafts, ranges
}

func powerShellFunctionHeaderName(line string) (string, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if len(line) < len("function") || !strings.EqualFold(line[:len("function")], "function") {
		return "", false
	}
	rest := line[len("function"):]
	if rest == "" || powerShellIdentByte(rest[0]) {
		return "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" || rest[0] == '$' {
		return "", false
	}
	end := 0
	for end < len(rest) {
		switch rest[end] {
		case ' ', '\t', '\r', '\n', '(', '{':
			name := strings.TrimSpace(rest[:end])
			return name, powerShellValidFunctionName(name)
		}
		end++
	}
	name := strings.TrimSpace(rest)
	return name, powerShellValidFunctionName(name)
}

func powerShellValidFunctionName(name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || first == '_'
}

func powerShellFunctionEndLine(lines []string, start int) int {
	hereEnd := ""
	blockComment := false
	balance := 0
	seenOpen := false
	for i := start; i < len(lines); i++ {
		line := powerShellScannerLine(lines[i], &hereEnd, &blockComment)
		for _, r := range line {
			switch r {
			case '{':
				seenOpen = true
				balance++
			case '}':
				if seenOpen {
					balance--
					if balance <= 0 {
						return i + 1
					}
				}
			}
		}
	}
	return len(lines)
}

func powerShellScannerLine(line string, hereEnd *string, blockComment *bool) string {
	if hereEnd != nil && *hereEnd != "" {
		if strings.HasPrefix(strings.TrimSpace(line), *hereEnd) {
			*hereEnd = ""
		}
		return ""
	}
	if blockComment != nil && *blockComment {
		if end := strings.Index(line, "#>"); end >= 0 {
			*blockComment = false
			return powerShellScannerLine(line[end+2:], hereEnd, blockComment)
		}
		return ""
	}
	var b strings.Builder
	quote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '`' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			b.WriteByte(' ')
			continue
		}
		if c == '#' {
			break
		}
		if c == '<' && i+1 < len(line) && line[i+1] == '#' {
			if end := strings.Index(line[i+2:], "#>"); end >= 0 {
				i += end + 3
				continue
			}
			if blockComment != nil {
				*blockComment = true
			}
			break
		}
		if c == '\'' || c == '"' {
			quote = c
			b.WriteByte(' ')
			continue
		}
		if c == '@' && i+1 < len(line) && (line[i+1] == '\'' || line[i+1] == '"') {
			if hereEnd != nil {
				*hereEnd = string([]byte{line[i+1], '@'})
			}
			break
		}
		b.WriteByte(c)
	}
	return b.String()
}

func powerShellVariableAssignmentName(n *tree_sitter.Node, content []byte) string {
	if n == nil || n.ChildCount() == 0 {
		return ""
	}
	left := n.Child(0)
	if left == nil || left.Kind() != "left_assignment_expression" {
		return ""
	}
	lhs := strings.TrimSpace(nodeText(left, content))
	if strings.ContainsAny(lhs, ".,[") {
		return ""
	}
	return powerShellNormalizeVariableName(lhs)
}

func powerShellNormalizeVariableName(name string) string {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "$") {
		return ""
	}
	name = strings.TrimPrefix(name, "$")
	if strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
		name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(name, "{"), "}"))
	}
	if before, after, ok := strings.Cut(name, ":"); ok {
		switch strings.ToLower(before) {
		case "global", "local", "private", "script":
			name = after
		default:
			return ""
		}
	}
	name = strings.TrimSpace(name)
	switch strings.ToLower(name) {
	case "", "_", "args", "false", "input", "null", "psitem", "this", "true":
		return ""
	default:
		return name
	}
}

func powerShellFirstChildText(n *tree_sitter.Node, content []byte, kinds ...string) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		for _, kind := range kinds {
			if child.Kind() == kind {
				return nodeText(child, content)
			}
		}
	}
	return ""
}

func powerShellImports(content []byte) []string {
	var imports []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimLeft(line, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := powerShellCutKeywordFold(line, "using"); ok {
			rest = strings.TrimLeft(rest, " \t")
			if moduleRest, ok := powerShellCutKeywordFold(rest, "module"); ok {
				if name := powerShellReadImportArgument(moduleRest); name != "" {
					imports = append(imports, name)
				}
			}
			continue
		}
		if rest, ok := powerShellCutCommandFold(line, "Import-Module"); ok {
			if name := powerShellReadImportArgument(rest); name != "" {
				imports = append(imports, name)
			}
		}
	}
	return uniqueStrings(imports)
}

func powerShellReadImportArgument(s string) string {
	s = strings.TrimLeft(s, " \t")
	for s != "" {
		token, rest := powerShellReadToken(s)
		if token == "" {
			return ""
		}
		if strings.HasPrefix(token, "-") {
			s = strings.TrimLeft(rest, " \t")
			if strings.EqualFold(token, "-Name") || strings.EqualFold(token, "-FullyQualifiedName") {
				continue
			}
			if !powerShellFlagTakesValue(token) {
				continue
			}
			_, s = powerShellReadToken(s)
			s = strings.TrimLeft(s, " \t")
			continue
		}
		return strings.Trim(token, `'"`)
	}
	return ""
}

func powerShellCutCommandFold(line, command string) (string, bool) {
	if len(line) < len(command) || !strings.EqualFold(line[:len(command)], command) {
		return "", false
	}
	if len(line) > len(command) && powerShellIdentByte(line[len(command)]) {
		return "", false
	}
	return line[len(command):], true
}

func powerShellCutKeywordFold(line, keyword string) (string, bool) {
	if len(line) < len(keyword) || !strings.EqualFold(line[:len(keyword)], keyword) {
		return "", false
	}
	if len(line) > len(keyword) && powerShellIdentByte(line[len(keyword)]) {
		return "", false
	}
	return line[len(keyword):], true
}

func powerShellReadToken(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", ""
	}
	if s[0] == '\'' || s[0] == '"' {
		quote := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == quote {
				return s[:i+1], s[i+1:]
			}
		}
		return s, ""
	}
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', ';', '\r', '\n', '#':
			return s[:i], s[i:]
		}
		i++
	}
	return s, ""
}

func powerShellFlagTakesValue(flag string) bool {
	switch strings.ToLower(flag) {
	case "-minimumversion", "-maximumversion", "-requiredversion", "-prefix", "-scope":
		return true
	default:
		return false
	}
}

func powerShellIdentByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-'
}
