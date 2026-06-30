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
	var drafts []symbolDraft
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		if draft, ok := powerShellVariableDefinitionDraft(n, content); ok {
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

func powerShellVariableDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	if n == nil || n.Kind() != "assignment_expression" || powerShellInsideDefinitionBody(n) {
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

func powerShellInsideDefinitionBody(n *tree_sitter.Node) bool {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		switch cur.Kind() {
		case "function_statement", "class_statement", "class_method_definition", "script_block":
			return true
		}
	}
	return false
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
