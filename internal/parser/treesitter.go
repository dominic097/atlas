package parser

import (
	"regexp"
	"strings"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_js "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// languagePointer returns the tree-sitter grammar pointer for a language name.
func languagePointer(lang string) unsafe.Pointer {
	switch lang {
	case "python":
		return tree_sitter_python.Language()
	case "javascript":
		return tree_sitter_js.Language()
	case "typescript":
		return tree_sitter_ts.LanguageTypescript()
	case "java":
		return tree_sitter_java.Language()
	case "c":
		return tree_sitter_c.Language()
	case "cpp":
		return tree_sitter_cpp.Language()
	default:
		return nil
	}
}

// parseTreeSitterSymbols parses a non-Go file with tree-sitter, returning symbol
// drafts and import paths. Ported from pulse walk*AST + extract*Import.
func parseTreeSitterSymbols(path, language string, content []byte) ([]symbolDraft, []string) {
	ptr := languagePointer(language)
	if ptr == nil {
		return nil, nil
	}
	lang := tree_sitter.NewLanguage(ptr)
	if lang == nil {
		return nil, nil
	}

	p := tree_sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(lang); err != nil {
		return nil, nil
	}

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil, nil
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil, nil
	}

	switch language {
	case "python":
		return walkPythonAST(root, content)
	case "javascript":
		return walkJSAST(root, content)
	case "typescript":
		return walkJSAST(root, content)
	case "java":
		return walkJavaAST(root, content)
	case "c":
		return walkCAST(root, content)
	case "cpp":
		return walkCAST(root, content)
	default:
		return nil, nil
	}
}

// ── Python ──────────────────────────────────────────────────────────────────

func walkPythonAST(root *tree_sitter.Node, src []byte) ([]symbolDraft, []string) {
	var imports []string
	var symbols []symbolDraft

	for i := uint(0); i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "import_statement", "import_from_statement":
			imports = append(imports, extractPythonImport(child, src)...)
		case "function_definition":
			if sym, ok := simpleSymbol(child, src, "function"); ok {
				symbols = append(symbols, sym)
			}
		case "class_definition":
			if sym, ok := simpleSymbol(child, src, "class"); ok {
				symbols = append(symbols, sym)
			}
		case "decorated_definition":
			for j := uint(0); j < child.ChildCount(); j++ {
				inner := child.Child(j)
				if inner == nil {
					continue
				}
				switch inner.Kind() {
				case "function_definition":
					if sym, ok := simpleSymbol(inner, src, "function"); ok {
						symbols = append(symbols, sym)
					}
				case "class_definition":
					if sym, ok := simpleSymbol(inner, src, "class"); ok {
						symbols = append(symbols, sym)
					}
				}
			}
		}
	}
	return symbols, imports
}

var (
	pyFromRe   = regexp.MustCompile(`from\s+([\w.]+)\s+import`)
	pyImportRe = regexp.MustCompile(`(?m)^\s*import\s+([\w.,\s]+)`)
)

func extractPythonImport(node *tree_sitter.Node, src []byte) []string {
	text := nodeText(node, src)
	var out []string
	for _, re := range []*regexp.Regexp{pyFromRe, pyImportRe} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			for _, part := range strings.Split(m[1], ",") {
				if p := strings.TrimSpace(part); p != "" {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// ── JavaScript / TypeScript ─────────────────────────────────────────────────

func walkJSAST(root *tree_sitter.Node, src []byte) ([]symbolDraft, []string) {
	var imports []string
	var symbols []symbolDraft

	walkNode(root, func(node *tree_sitter.Node) bool {
		switch node.Kind() {
		case "import_statement":
			imports = append(imports, extractJSImport(node, src)...)
		case "function_declaration", "function_expression":
			if sym, ok := simpleSymbol(node, src, "function"); ok {
				symbols = append(symbols, sym)
			}
		case "class_declaration":
			if sym, ok := simpleSymbol(node, src, "class"); ok {
				symbols = append(symbols, sym)
			}
		case "lexical_declaration", "variable_declaration":
			symbols = append(symbols, jsArrowFunctions(node, src)...)
		}
		return true
	})
	return symbols, imports
}

var jsImportRe = regexp.MustCompile(`from\s+['"]([^'"]+)['"]`)

func extractJSImport(node *tree_sitter.Node, src []byte) []string {
	text := nodeText(node, src)
	var out []string
	for _, m := range jsImportRe.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

// jsArrowFunctions captures `const fn = (...) => {...}` arrow declarations.
func jsArrowFunctions(node *tree_sitter.Node, src []byte) []symbolDraft {
	var out []symbolDraft
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		hasArrow := false
		varName := ""
		for j := uint(0); j < child.ChildCount(); j++ {
			gc := child.Child(j)
			if gc == nil {
				continue
			}
			if gc.Kind() == "identifier" && varName == "" {
				varName = nodeText(gc, src)
			}
			if gc.Kind() == "arrow_function" {
				hasArrow = true
			}
		}
		if hasArrow && varName != "" {
			out = append(out, symbolDraft{
				name:      varName,
				kind:      "function",
				startLine: int(child.StartPosition().Row) + 1,
				endLine:   int(child.EndPosition().Row) + 1,
			})
		}
	}
	return out
}

// ── Java ────────────────────────────────────────────────────────────────────

var javaImportRe = regexp.MustCompile(`import\s+([\w.]+);`)

func walkJavaAST(root *tree_sitter.Node, src []byte) ([]symbolDraft, []string) {
	var imports []string
	var symbols []symbolDraft

	for i := uint(0); i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "import_declaration":
			if m := javaImportRe.FindStringSubmatch(nodeText(child, src)); len(m) > 1 {
				imports = append(imports, m[1])
			}
		case "class_declaration", "interface_declaration", "enum_declaration":
			symbols = append(symbols, javaTypeSymbols(child, src)...)
		}
	}
	return symbols, imports
}

func javaTypeSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	var out []symbolDraft
	kind := "class"
	if strings.Contains(node.Kind(), "interface") {
		kind = "interface"
	} else if strings.Contains(node.Kind(), "enum") {
		kind = "enum"
	}

	if name := childText(node, "identifier", src); name != "" {
		out = append(out, symbolDraft{
			name:      name,
			kind:      kind,
			startLine: int(node.StartPosition().Row) + 1,
			endLine:   int(node.EndPosition().Row) + 1,
		})
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "class_body" {
			continue
		}
		for j := uint(0); j < child.ChildCount(); j++ {
			member := child.Child(j)
			if member == nil || member.Kind() != "method_declaration" {
				continue
			}
			if mname := childText(member, "identifier", src); mname != "" {
				out = append(out, symbolDraft{
					name:      mname,
					kind:      "method",
					startLine: int(member.StartPosition().Row) + 1,
					endLine:   int(member.EndPosition().Row) + 1,
				})
			}
		}
	}
	return out
}

// ── C / C++ ─────────────────────────────────────────────────────────────────

var cIncludeRe = regexp.MustCompile(`[<"]([^>"]+)[>"]`)

func walkCAST(root *tree_sitter.Node, src []byte) ([]symbolDraft, []string) {
	var imports []string
	var symbols []symbolDraft

	for i := uint(0); i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "preproc_include":
			if m := cIncludeRe.FindStringSubmatch(nodeText(child, src)); len(m) > 1 {
				imports = append(imports, m[1])
			}
		case "function_definition":
			if sym, ok := cFunctionSymbol(child, src); ok {
				symbols = append(symbols, sym)
			}
		}
	}
	return symbols, imports
}

func cFunctionSymbol(node *tree_sitter.Node, src []byte) (symbolDraft, bool) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "function_declarator" {
			continue
		}
		name := childText(child, "identifier", src)
		if name == "" {
			// C++ qualified / field identifiers
			name = childText(child, "field_identifier", src)
		}
		if name == "" {
			continue
		}
		sig := nodeText(node, src)
		if nl := strings.IndexByte(sig, '\n'); nl > 0 {
			sig = sig[:nl]
		}
		return symbolDraft{
			name:      name,
			kind:      "function",
			signature: strings.TrimSpace(sig),
			startLine: int(node.StartPosition().Row) + 1,
			endLine:   int(node.EndPosition().Row) + 1,
		}, true
	}
	return symbolDraft{}, false
}

// ── tree-sitter helpers ─────────────────────────────────────────────────────

// simpleSymbol builds a draft from a node whose name is its direct "identifier"
// child, with a one-line signature from the node's source.
func simpleSymbol(node *tree_sitter.Node, src []byte, kind string) (symbolDraft, bool) {
	name := childText(node, "identifier", src)
	if name == "" {
		return symbolDraft{}, false
	}
	sig := nodeText(node, src)
	if nl := strings.IndexByte(sig, '\n'); nl > 0 {
		sig = sig[:nl]
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: strings.TrimSpace(sig),
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
	}, true
}

// nodeText returns the source slice spanned by a node.
func nodeText(node *tree_sitter.Node, src []byte) string {
	start := node.StartByte()
	end := node.EndByte()
	if int(end) > len(src) || start > end {
		return ""
	}
	return string(src[start:end])
}

// childText returns the text of the first direct child of the given Kind.
func childText(node *tree_sitter.Node, kind string, src []byte) string {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil && child.Kind() == kind {
			return child.Utf8Text(src)
		}
	}
	return ""
}

// walkNode depth-first walks a tree, calling fn per node; returning false from
// fn skips the subtree.
func walkNode(node *tree_sitter.Node, fn func(*tree_sitter.Node) bool) {
	if node == nil {
		return
	}
	if !fn(node) {
		return
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		walkNode(node.Child(i), fn)
	}
}
