package parser

import (
	"regexp"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_elixir "github.com/tree-sitter/tree-sitter-elixir/bindings/go"

	"github.com/dominic097/atlas/internal/graph"
)

var (
	elixirModuleNameRE   = regexp.MustCompile(`^([A-Z][A-Za-z0-9_]*(?:\.[A-Z][A-Za-z0-9_]*)*)\b`)
	elixirCallableNameRE = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_!?]*|[+\-*/%<>=!&|^~]+)\s*(?:\(|,|\b)`)
)

func parseElixirNative(content []byte) ([]symbolDraft, bool) {
	if len(content) == 0 {
		return nil, true
	}
	grammar := tree_sitter.NewLanguage(tree_sitter_elixir.Language())
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
		if n.Kind() == "call" {
			if draft, ok := elixirDefinitionDraft(n, content); ok {
				drafts = append(drafts, draft)
			}
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return sortDedupDrafts(drafts), true
}

func elixirDefinitionDraft(call *tree_sitter.Node, content []byte) (symbolDraft, bool) {
	head := elixirCallHead(call, content)
	args := strings.TrimSpace(elixirArgumentsText(call, content))
	if head == "" || args == "" {
		return symbolDraft{}, false
	}
	kind := ""
	name := ""
	switch head {
	case "defmodule":
		kind = "module"
		name = elixirModuleName(args)
	case "defprotocol":
		kind = "protocol"
		name = elixirModuleName(args)
	case "defimpl":
		kind = "implementation"
		name = elixirModuleName(args)
	case "def", "defp":
		kind = "function"
		name = elixirCallableName(args)
	case "defmacro", "defmacrop":
		kind = "macro"
		name = elixirCallableName(args)
	case "defdelegate":
		kind = "delegate"
		name = elixirCallableName(args)
	case "defguard", "defguardp":
		kind = "guard"
		name = elixirCallableName(args)
	default:
		return symbolDraft{}, false
	}
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: tagsFirstLine(call, content),
		startLine: int(call.StartPosition().Row) + 1,
		endLine:   int(call.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"source": "tree_sitter_elixir"},
	}, true
}

func elixirCallHead(call *tree_sitter.Node, content []byte) string {
	if call == nil {
		return ""
	}
	if target := call.ChildByFieldName("target"); target != nil && target.Kind() == "identifier" {
		return strings.TrimSpace(nodeText(target, content))
	}
	for i := uint(0); i < call.ChildCount(); i++ {
		child := call.Child(i)
		if child != nil && child.IsNamed() {
			return strings.TrimSpace(nodeText(child, content))
		}
	}
	return ""
}

func elixirArgumentsText(call *tree_sitter.Node, content []byte) string {
	if call == nil {
		return ""
	}
	if args := call.ChildByFieldName("arguments"); args != nil {
		return nodeText(args, content)
	}
	for i := uint(0); i < call.ChildCount(); i++ {
		child := call.Child(i)
		if child != nil && child.Kind() == "arguments" {
			return nodeText(child, content)
		}
	}
	return ""
}

func elixirModuleName(args string) string {
	if match := elixirModuleNameRE.FindStringSubmatch(args); len(match) == 2 {
		return match[1]
	}
	return ""
}

func elixirCallableName(args string) string {
	if match := elixirCallableNameRE.FindStringSubmatch(args); len(match) == 2 {
		return match[1]
	}
	return ""
}
