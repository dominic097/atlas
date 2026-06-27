package parser

import (
	"regexp"
	"strings"
	"unsafe"

	"github.com/dominic097/atlas/internal/graph"
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

// parseTreeSitter parses a non-Go file with tree-sitter ONCE and returns the
// symbol drafts, import paths, the AST root, and a cleanup func the caller MUST
// invoke when done with the root. Keeping the parser/tree alive lets the caller
// reuse the SAME root for AST-based call-edge extraction (see calls.go) instead
// of re-parsing. The returned root is nil (and cleanup is a no-op) when parsing
// is not possible. Ported from pulse walk*AST + extract*Import.
func parseTreeSitter(path, language string, content []byte) (syms []symbolDraft, imports []string, root *tree_sitter.Node, cleanup func()) {
	cleanup = func() {}
	ptr := languagePointer(language)
	if ptr == nil {
		return nil, nil, nil, cleanup
	}
	lang := tree_sitter.NewLanguage(ptr)
	if lang == nil {
		return nil, nil, nil, cleanup
	}

	p := tree_sitter.NewParser()
	if err := p.SetLanguage(lang); err != nil {
		p.Close()
		return nil, nil, nil, cleanup
	}

	tree := p.Parse(content, nil)
	if tree == nil {
		p.Close()
		return nil, nil, nil, cleanup
	}

	root = tree.RootNode()
	if root == nil {
		tree.Close()
		p.Close()
		return nil, nil, nil, cleanup
	}
	cleanup = func() {
		tree.Close()
		p.Close()
	}

	switch language {
	case "python":
		syms, imports = walkPythonAST(root, content)
	case "javascript", "typescript":
		syms, imports = walkJSAST(root, content)
	case "java":
		syms, imports = walkJavaAST(root, content)
	case "c", "cpp":
		syms, imports = walkCAST(root, content)
	default:
		cleanup()
		return nil, nil, nil, func() {}
	}
	return syms, imports, root, cleanup
}

// parseTreeSitterSymbols parses a non-Go file with tree-sitter, returning symbol
// drafts and import paths. Thin wrapper over parseTreeSitter that discards the
// root (kept for callers that only need symbols/imports).
func parseTreeSitterSymbols(path, language string, content []byte) ([]symbolDraft, []string) {
	syms, imports, _, cleanup := parseTreeSitter(path, language, content)
	cleanup()
	return syms, imports
}

// ── Python ──────────────────────────────────────────────────────────────────

func walkPythonAST(root *tree_sitter.Node, src []byte) ([]symbolDraft, []string) {
	var imports []string
	var symbols []symbolDraft
	walkPythonModuleScope(root, src, &symbols, &imports)
	return symbols, imports
}

func walkPythonModuleScope(node *tree_sitter.Node, src []byte, symbols *[]symbolDraft, imports *[]string) {
	if node == nil {
		return
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "import_statement", "import_from_statement":
			*imports = append(*imports, extractPythonImport(child, src)...)
		case "assignment", "expression_statement":
			*symbols = append(*symbols, pythonAssignmentSymbols(child, src, "", "module")...)
		case "function_definition":
			if sym, ok := simpleSymbol(child, src, "function"); ok {
				*symbols = append(*symbols, sym)
				*symbols = append(*symbols, pythonNestedFunctionSymbols(child, src, sym.name)...)
			}
		case "class_definition":
			*symbols = append(*symbols, pythonClassSymbols(child, src)...)
		case "decorated_definition":
			for j := uint(0); j < child.ChildCount(); j++ {
				inner := child.Child(j)
				if inner == nil {
					continue
				}
				switch inner.Kind() {
				case "function_definition":
					if sym, ok := simpleSymbol(inner, src, "function"); ok {
						*symbols = append(*symbols, sym)
						*symbols = append(*symbols, pythonNestedFunctionSymbols(inner, src, sym.name)...)
					}
				case "class_definition":
					*symbols = append(*symbols, pythonClassSymbols(inner, src)...)
				}
			}
		default:
			if pythonModuleScopeContainer(child.Kind()) {
				walkPythonModuleScope(child, src, symbols, imports)
			}
		}
	}
}

func pythonModuleScopeContainer(kind string) bool {
	switch kind {
	case "module", "block", "if_statement", "elif_clause", "else_clause",
		"try_statement", "except_clause", "finally_clause", "for_statement",
		"while_statement", "with_statement", "match_statement", "case_clause":
		return true
	default:
		return false
	}
}

func pythonClassSymbols(classNode *tree_sitter.Node, src []byte) []symbolDraft {
	classSym, ok := simpleSymbol(classNode, src, "class")
	if !ok {
		return nil
	}
	out := []symbolDraft{classSym}
	owner := classSym.name
	body := classNode.ChildByFieldName("body")
	if body == nil {
		for i := uint(0); i < classNode.ChildCount(); i++ {
			child := classNode.Child(i)
			if child != nil && child.Kind() == "block" {
				body = child
				break
			}
		}
	}
	if body == nil {
		return out
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		child := body.Child(i)
		if child == nil {
			continue
		}
		methodNode := child
		if child.Kind() == "decorated_definition" {
			methodNode = nil
			for j := uint(0); j < child.ChildCount(); j++ {
				inner := child.Child(j)
				if inner != nil && inner.Kind() == "function_definition" {
					methodNode = inner
					break
				}
			}
		}
		if methodNode == nil || methodNode.Kind() != "function_definition" {
			out = append(out, pythonAssignmentSymbols(child, src, owner, "class")...)
			continue
		}
		if method, ok := simpleSymbol(methodNode, src, "method"); ok {
			method.recvType = owner
			method.metadata = graph.JSONBMap{"owner_type": owner}
			out = append(out, method)
			out = append(out, pythonNestedFunctionSymbols(methodNode, src, method.name)...)
		}
	}
	return out
}

func pythonNestedFunctionSymbols(fnNode *tree_sitter.Node, src []byte, owner string) []symbolDraft {
	body := fnNode.ChildByFieldName("body")
	if body == nil {
		for i := uint(0); i < fnNode.ChildCount(); i++ {
			child := fnNode.Child(i)
			if child != nil && child.Kind() == "block" {
				body = child
				break
			}
		}
	}
	if body == nil {
		return nil
	}
	var out []symbolDraft
	walkPythonNestedFunctionScope(body, src, owner, &out)
	return out
}

func walkPythonNestedFunctionScope(node *tree_sitter.Node, src []byte, owner string, out *[]symbolDraft) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "function_definition":
			if sym, ok := simpleSymbol(child, src, "function"); ok {
				sym.metadata = graph.JSONBMap{"scope": "local_function", "owner_function": owner}
				*out = append(*out, sym)
				*out = append(*out, pythonNestedFunctionSymbols(child, src, sym.name)...)
			}
		case "decorated_definition":
			for j := uint(0); j < child.ChildCount(); j++ {
				inner := child.Child(j)
				if inner == nil || inner.Kind() != "function_definition" {
					continue
				}
				if sym, ok := simpleSymbol(inner, src, "function"); ok {
					sym.metadata = graph.JSONBMap{"scope": "local_function", "owner_function": owner}
					*out = append(*out, sym)
					*out = append(*out, pythonNestedFunctionSymbols(inner, src, sym.name)...)
				}
			}
		default:
			if pythonModuleScopeContainer(child.Kind()) {
				walkPythonNestedFunctionScope(child, src, owner, out)
			}
		}
	}
}

func pythonAssignmentSymbols(node *tree_sitter.Node, src []byte, owner, scope string) []symbolDraft {
	assignments := pythonAssignmentNodes(node)
	out := make([]symbolDraft, 0, len(assignments))
	for _, assignment := range assignments {
		left := assignment.ChildByFieldName("left")
		if left == nil {
			left = assignment.Child(0)
		}
		for _, name := range pythonAssignedNames(left, src) {
			if name == "" {
				continue
			}
			kind := "variable"
			meta := graph.JSONBMap{"scope": scope}
			if owner != "" {
				kind = "field"
				meta["owner_type"] = owner
			} else if isPythonConstantName(name) {
				kind = "constant"
			}
			out = append(out, symbolDraft{
				name:      name,
				kind:      kind,
				signature: pythonAssignmentSignature(assignment, src),
				startLine: int(assignment.StartPosition().Row) + 1,
				endLine:   int(assignment.EndPosition().Row) + 1,
				metadata:  meta,
			})
		}
	}
	return out
}

func pythonAssignmentNodes(node *tree_sitter.Node) []*tree_sitter.Node {
	if node == nil {
		return nil
	}
	if node.Kind() == "assignment" {
		return []*tree_sitter.Node{node}
	}
	var out []*tree_sitter.Node
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if child.Kind() == "assignment" {
			out = append(out, child)
		}
	}
	return out
}

func pythonAssignedNames(node *tree_sitter.Node, src []byte) []string {
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "identifier":
		return []string{nodeText(node, src)}
	case "pattern_list", "tuple", "tuple_pattern", "list", "list_pattern":
		var out []string
		for i := uint(0); i < node.ChildCount(); i++ {
			out = append(out, pythonAssignedNames(node.Child(i), src)...)
		}
		return out
	default:
		return nil
	}
}

func pythonAssignmentSignature(node *tree_sitter.Node, src []byte) string {
	sig := nodeText(node, src)
	if nl := strings.IndexByte(sig, '\n'); nl > 0 {
		sig = sig[:nl]
	}
	const maxLen = 160
	sig = strings.TrimSpace(sig)
	if len(sig) > maxLen {
		return sig[:maxLen] + "..."
	}
	return sig
}

func isPythonConstantName(name string) bool {
	hasLetter := false
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
	}
	return hasLetter
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
			symbols = append(symbols, jsClassSymbols(node, src)...)
			return false
		case "interface_declaration":
			symbols = append(symbols, jsInterfaceSymbols(node, src)...)
			return false
		case "type_alias_declaration":
			if sym, ok := jsNamedSymbol(node, src, "type"); ok {
				symbols = append(symbols, sym)
			}
		case "enum_declaration":
			if sym, ok := jsNamedSymbol(node, src, "enum"); ok {
				symbols = append(symbols, sym)
			}
		case "lexical_declaration", "variable_declaration":
			symbols = append(symbols, jsVariableSymbols(node, src)...)
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

func jsClassSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	classSym, ok := jsNamedSymbol(node, src, "class")
	if !ok {
		return nil
	}
	out := []symbolDraft{classSym}
	body := node.ChildByFieldName("body")
	if body == nil {
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child != nil && child.Kind() == "class_body" {
				body = child
				break
			}
		}
	}
	if body == nil {
		return out
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		child := body.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "method_definition", "abstract_method_signature", "method_signature":
			if method, ok := jsNamedSymbol(child, src, "method"); ok {
				method.recvType = classSym.name
				method.metadata = graph.JSONBMap{"owner_type": classSym.name}
				out = append(out, method)
			}
		case "public_field_definition", "field_definition", "property_signature":
			if field, ok := jsNamedSymbol(child, src, "field"); ok {
				field.metadata = graph.JSONBMap{"owner_type": classSym.name}
				out = append(out, field)
			}
		}
	}
	return out
}

func jsInterfaceSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	iface, ok := jsNamedSymbol(node, src, "interface")
	if !ok {
		return nil
	}
	out := []symbolDraft{iface}
	walkNode(node, func(child *tree_sitter.Node) bool {
		if child == nil || child == node {
			return true
		}
		switch child.Kind() {
		case "method_signature", "abstract_method_signature":
			if method, ok := jsNamedSymbol(child, src, "method"); ok {
				method.recvType = iface.name
				method.metadata = graph.JSONBMap{"owner_type": iface.name}
				out = append(out, method)
			}
			return false
		case "property_signature":
			if field, ok := jsNamedSymbol(child, src, "field"); ok {
				field.metadata = graph.JSONBMap{"owner_type": iface.name}
				out = append(out, field)
			}
			return false
		case "type_alias_declaration", "interface_declaration", "class_declaration":
			return false
		default:
			return true
		}
	})
	return out
}

// jsVariableSymbols captures top-level/local declarations as compact symbols:
// `const fn = (...) => {}` remains a function, while non-callable declarations
// become constant/variable entries for navigation and review context.
func jsVariableSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	var out []symbolDraft
	declKind := "variable"
	if strings.HasPrefix(strings.TrimSpace(nodeText(node, src)), "const ") {
		declKind = "constant"
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil || nameNode.Kind() != "identifier" {
			continue
		}
		name := strings.TrimSpace(nodeText(nameNode, src))
		if name == "" {
			continue
		}
		kind := declKind
		if value := child.ChildByFieldName("value"); value != nil && jsValueIsFunction(value) {
			kind = "function"
		}
		out = append(out, symbolDraft{
			name:      name,
			kind:      kind,
			signature: jsOneLineSignature(child, src),
			startLine: int(child.StartPosition().Row) + 1,
			endLine:   int(child.EndPosition().Row) + 1,
			metadata:  graph.JSONBMap{"scope": "declaration"},
		})
	}
	return out
}

func jsValueIsFunction(node *tree_sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "arrow_function", "function", "function_expression":
		return true
	default:
		return false
	}
}

func jsNamedSymbol(node *tree_sitter.Node, src []byte, kind string) (symbolDraft, bool) {
	name := jsNodeName(node, src)
	if name == "" {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: jsOneLineSignature(node, src),
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
	}, true
}

func jsNodeName(node *tree_sitter.Node, src []byte) string {
	if name := node.ChildByFieldName("name"); name != nil {
		if text := strings.TrimSpace(nodeText(name, src)); text != "" {
			return text
		}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "identifier", "type_identifier", "property_identifier", "private_property_identifier":
			if text := strings.TrimSpace(nodeText(child, src)); text != "" {
				return strings.TrimPrefix(text, "#")
			}
		}
	}
	return ""
}

func jsOneLineSignature(node *tree_sitter.Node, src []byte) string {
	sig := strings.TrimSpace(nodeText(node, src))
	if nl := strings.IndexByte(sig, '\n'); nl > 0 {
		sig = sig[:nl]
	}
	const maxLen = 160
	if len(sig) > maxLen {
		return sig[:maxLen] + "..."
	}
	return sig
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
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration":
			symbols = append(symbols, javaTypeSymbols(child, src)...)
		}
	}
	return symbols, imports
}

func javaTypeSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	var out []symbolDraft
	kind := javaTypeKind(node.Kind())

	typeName := javaDeclName(node, src)
	if typeName != "" {
		out = append(out, symbolDraft{
			name:      typeName,
			kind:      kind,
			signature: javaOneLineSignature(node, src),
			startLine: int(node.StartPosition().Row) + 1,
			endLine:   int(node.EndPosition().Row) + 1,
		})
	}
	if node.Kind() == "record_declaration" {
		out = append(out, javaRecordHeaderComponentSymbols(node, src, typeName)...)
	}
	out = append(out, javaTypeParameterSymbols(node, src, typeName)...)

	body := node.ChildByFieldName("body")
	if body == nil {
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child != nil && strings.Contains(child.Kind(), "body") {
				body = child
				break
			}
		}
	}
	if body == nil {
		return out
	}

	hasExplicitConstructor := false
	for i := uint(0); i < body.ChildCount(); i++ {
		member := body.Child(i)
		if member == nil {
			continue
		}
		if member.Kind() == "constructor_declaration" || member.Kind() == "compact_constructor_declaration" {
			hasExplicitConstructor = true
		}
		out = append(out, javaMemberSymbols(member, src, typeName)...)
	}
	if kind == "class" && typeName != "" && !hasExplicitConstructor {
		out = append(out, javaSyntheticConstructorSymbol(node, src, typeName))
	}
	return out
}

func javaRecordHeaderComponentSymbols(node *tree_sitter.Node, src []byte, owner string) []symbolDraft {
	var out []symbolDraft
	var walk func(n *tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "class_body", "interface_body", "enum_body", "record_body", "annotation_type_body",
			"class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration":
			if n != node {
				return
			}
		case "record_component", "formal_parameter", "spread_parameter":
			out = append(out, javaRecordComponentSymbol(n, src, owner)...)
			return
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		walk(node.Child(i))
	}
	return out
}

func javaTypeKind(kind string) string {
	switch kind {
	case "interface_declaration":
		return "interface"
	case "enum_declaration":
		return "enum"
	case "record_declaration":
		return "record"
	case "annotation_type_declaration":
		return "annotation"
	default:
		return "class"
	}
}

func javaMemberSymbols(member *tree_sitter.Node, src []byte, owner string) []symbolDraft {
	switch member.Kind() {
	case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration":
		return javaTypeSymbols(member, src)
	case "method_declaration":
		return javaCallableSymbol(member, src, owner, "method")
	case "constructor_declaration", "compact_constructor_declaration":
		return javaCallableSymbol(member, src, owner, "constructor")
	case "field_declaration", "constant_declaration":
		return javaFieldSymbols(member, src, owner, "field")
	case "enum_constant":
		return javaEnumConstantSymbol(member, src, owner)
	case "record_component":
		return javaRecordComponentSymbol(member, src, owner)
	case "annotation_type_element_declaration":
		return javaCallableSymbol(member, src, owner, "annotation_member")
	}

	var out []symbolDraft
	for i := uint(0); i < member.ChildCount(); i++ {
		child := member.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration",
			"method_declaration", "constructor_declaration", "compact_constructor_declaration",
			"field_declaration", "constant_declaration", "enum_constant", "record_component", "annotation_type_element_declaration":
			out = append(out, javaMemberSymbols(child, src, owner)...)
		}
	}
	return out
}

func javaCallableSymbol(node *tree_sitter.Node, src []byte, owner, kind string) []symbolDraft {
	name := javaDeclName(node, src)
	if name == "" && kind == "constructor" {
		name = owner
	}
	if name == "" {
		return nil
	}
	out := []symbolDraft{{
		name:      name,
		kind:      kind,
		signature: javaOneLineSignature(node, src),
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
		metadata:  javaOwnerMetadata(owner),
		recvType:  owner,
	}}
	out = append(out, javaTypeParameterSymbols(node, src, owner)...)
	return out
}

func javaSyntheticConstructorSymbol(node *tree_sitter.Node, src []byte, owner string) symbolDraft {
	meta := javaOwnerMetadata(owner)
	meta["synthetic"] = true
	return symbolDraft{
		name:      owner,
		kind:      "constructor",
		signature: owner + "()",
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.StartPosition().Row) + 1,
		metadata:  meta,
		recvType:  owner,
	}
}

func javaFieldSymbols(node *tree_sitter.Node, src []byte, owner, kind string) []symbolDraft {
	declType := javaDeclTypeName(node, src)
	var out []symbolDraft
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		name := javaDeclName(child, src)
		if name == "" {
			continue
		}
		meta := javaOwnerMetadata(owner)
		if declType != "" {
			meta["decl_type"] = declType
		}
		out = append(out, symbolDraft{
			name:      name,
			kind:      kind,
			signature: javaOneLineSignature(child, src),
			startLine: int(child.StartPosition().Row) + 1,
			endLine:   int(child.EndPosition().Row) + 1,
			metadata:  meta,
			recvType:  owner,
		})
	}
	return out
}

func javaEnumConstantSymbol(node *tree_sitter.Node, src []byte, owner string) []symbolDraft {
	name := javaDeclName(node, src)
	if name == "" {
		return nil
	}
	return []symbolDraft{{
		name:      name,
		kind:      "enum_constant",
		signature: javaOneLineSignature(node, src),
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
		metadata:  javaOwnerMetadata(owner),
		recvType:  owner,
	}}
}

func javaRecordComponentSymbol(node *tree_sitter.Node, src []byte, owner string) []symbolDraft {
	name := javaDeclName(node, src)
	if name == "" {
		return nil
	}
	meta := javaOwnerMetadata(owner)
	if typ := javaDeclTypeName(node, src); typ != "" {
		meta["decl_type"] = typ
	}
	return []symbolDraft{{
		name:      name,
		kind:      "field",
		signature: javaOneLineSignature(node, src),
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
		metadata:  meta,
		recvType:  owner,
	}}
}

func javaTypeParameterSymbols(node *tree_sitter.Node, src []byte, owner string) []symbolDraft {
	var out []symbolDraft
	var walk func(n *tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "class_body", "interface_body", "enum_body", "record_body", "annotation_type_body", "block":
			return
		case "type_parameter":
			name := javaDeclName(n, src)
			if name == "" {
				name = childText(n, "type_identifier", src)
			}
			if name != "" {
				out = append(out, symbolDraft{
					name:      name,
					kind:      "type_parameter",
					signature: javaOneLineSignature(n, src),
					startLine: int(n.StartPosition().Row) + 1,
					endLine:   int(n.EndPosition().Row) + 1,
					metadata:  javaOwnerMetadata(owner),
				})
			}
			return
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		walk(node.Child(i))
	}
	return out
}

func javaOwnerMetadata(owner string) graph.JSONBMap {
	if owner == "" {
		return graph.JSONBMap{}
	}
	return graph.JSONBMap{"owner_type": owner}
}

func javaDeclName(node *tree_sitter.Node, src []byte) string {
	if name := node.ChildByFieldName("name"); name != nil {
		if text := strings.TrimSpace(nodeText(name, src)); text != "" {
			return text
		}
	}
	if name := childText(node, "identifier", src); name != "" {
		return name
	}
	if name := childText(node, "type_identifier", src); name != "" {
		return name
	}
	return ""
}

func javaOneLineSignature(node *tree_sitter.Node, src []byte) string {
	sig := strings.TrimSpace(nodeText(node, src))
	if nl := strings.IndexByte(sig, '\n'); nl > 0 {
		sig = sig[:nl]
	}
	const maxLen = 160
	if len(sig) > maxLen {
		return sig[:maxLen] + "..."
	}
	return sig
}

// ── C / C++ ─────────────────────────────────────────────────────────────────

var cIncludeRe = regexp.MustCompile(`[<"]([^>"]+)[>"]`)

func walkCAST(root *tree_sitter.Node, src []byte) ([]symbolDraft, []string) {
	var imports []string
	var symbols []symbolDraft

	walkNode(root, func(child *tree_sitter.Node) bool {
		switch child.Kind() {
		case "preproc_include":
			if m := cIncludeRe.FindStringSubmatch(nodeText(child, src)); len(m) > 1 {
				imports = append(imports, m[1])
			}
		case "function_definition":
			if sym, ok := cFunctionSymbol(child, src); ok {
				symbols = append(symbols, sym)
			}
			return false
		case "class_specifier", "struct_specifier":
			symbols = append(symbols, cppTypeSymbols(child, src)...)
			return false
		}
		return true
	})
	return symbols, imports
}

// cppTypeSymbols extracts a C++ class/struct symbol plus its member functions,
// stamping each member's recv_type with the enclosing type name (SHARED METADATA
// CONTRACT) so the query layer can disambiguate same-named methods across types.
// Best-effort: leaves recv_type "" when the type name is not determinable.
func cppTypeSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	var out []symbolDraft
	kind := "class"
	if node.Kind() == "struct_specifier" {
		kind = "struct"
	}

	typeName := childText(node, "type_identifier", src)
	if typeName != "" {
		out = append(out, symbolDraft{
			name:      typeName,
			kind:      kind,
			startLine: int(node.StartPosition().Row) + 1,
			endLine:   int(node.EndPosition().Row) + 1,
		})
	}

	body := node.ChildByFieldName("body")
	if body == nil {
		return out
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		member := body.Child(i)
		if member == nil || member.Kind() != "function_definition" {
			continue
		}
		name := cppFunctionName(member, src)
		if name == "" {
			continue
		}
		out = append(out, symbolDraft{
			name:      name,
			kind:      "method",
			startLine: int(member.StartPosition().Row) + 1,
			endLine:   int(member.EndPosition().Row) + 1,
			recvType:  typeName,
		})
	}
	return out
}

// cppFunctionName pulls the declared name out of a C++ function_definition's
// function_declarator, handling plain identifiers and field identifiers.
func cppFunctionName(node *tree_sitter.Node, src []byte) string {
	name, _, _ := cppFunctionIdentity(node, src)
	return name
}

func cppFunctionIdentity(node *tree_sitter.Node, src []byte) (name, qualified, recvType string) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "function_declarator" {
			continue
		}
		name = cppDeclaratorName(child, src)
		qualified = cppDeclaratorQualifiedName(child, src)
		if qualified != "" {
			recvType = cppQualifiedReceiver(qualified)
		}
		return name, qualified, recvType
	}
	return "", "", ""
}

func cFunctionSymbol(node *tree_sitter.Node, src []byte) (symbolDraft, bool) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "function_declarator" {
			continue
		}
		name, qualified, recvType := cppFunctionIdentity(node, src)
		if name == "" {
			continue
		}
		kind := "function"
		meta := graph.JSONBMap{}
		if recvType != "" {
			kind = "method"
			meta["owner_type"] = recvType
			meta["qualified_name"] = qualified
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
			metadata:  meta,
			recvType:  recvType,
		}, true
	}
	return symbolDraft{}, false
}

func cppDeclaratorQualifiedName(node *tree_sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "qualified_identifier" {
		return strings.TrimSpace(nodeText(node, src))
	}
	if inner := node.ChildByFieldName("declarator"); inner != nil {
		if q := cppDeclaratorQualifiedName(inner, src); q != "" {
			return q
		}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		if q := cppDeclaratorQualifiedName(node.Child(i), src); q != "" {
			return q
		}
	}
	return ""
}

func cppQualifiedReceiver(qualified string) string {
	parts := strings.Split(strings.TrimSpace(qualified), "::")
	if len(parts) < 2 {
		return ""
	}
	recv := strings.TrimSpace(parts[len(parts)-2])
	if recv == "" || strings.ContainsAny(recv, " \t\n") {
		return ""
	}
	return recv
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
