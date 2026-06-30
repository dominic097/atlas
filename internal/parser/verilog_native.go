package parser

import (
	"strings"

	tree_sitter_systemverilog "github.com/gmlarumbe/tree-sitter-systemverilog/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/dominic097/atlas/internal/graph"
)

func parseVerilogNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_systemverilog.Language())
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
	var drafts []symbolDraft
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		if draft, ok := verilogDefinitionDraft(n, content); ok {
			drafts = append(drafts, draft)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), true
}

func verilogDefinitionDraft(n *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	kind := ""
	switch n.Kind() {
	case "module_declaration":
		kind = "module"
	case "interface_declaration":
		kind = "interface"
	case "package_declaration":
		kind = "package"
	case "class_declaration":
		kind = "class"
	case "function_declaration":
		kind = "function"
	case "task_declaration":
		kind = "task"
	case "program_declaration":
		kind = "program"
	case "checker_declaration":
		kind = "checker"
	case "type_declaration":
		kind = "type"
	default:
		return symbolDraft{}, false
	}
	name := verilogFirstIdentifier(n, content)
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(n, content),
		startLine: int(n.StartPosition().Row) + 1,
		endLine:   int(n.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_systemverilog"},
	}, true
}

func verilogFirstIdentifier(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	if child := n.ChildByFieldName("name"); child != nil {
		return verilogIdentifierName(child, content)
	}
	if child := n.ChildByFieldName("type_name"); child != nil {
		return verilogIdentifierName(child, content)
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "simple_identifier", "escaped_identifier", "system_tf_identifier":
			if name := verilogIdentifierName(child, content); name != "" {
				return name
			}
		default:
			if child.IsNamed() {
				if name := verilogFirstIdentifier(child, content); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

func verilogIdentifierName(n *tree_sitter.Node, content []byte) string {
	name := strings.TrimSpace(nodeText(n, content))
	name = strings.TrimPrefix(name, "\\")
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}
	return name
}
