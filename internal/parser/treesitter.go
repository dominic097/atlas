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
		// `.h` headers default to the C grammar, but many ship C++ API (leveldb's
		// include/leveldb/*.h wrap everything in `namespace`). The C parser then
		// silently mis-models the file — often WITHOUT setting an error flag — and
		// drops every def. When the content is unmistakably C++ (namespace/class/
		// template/`::`), re-parse with the cpp grammar and keep its results when
		// they capture at least as many symbols, so we never trade C recall away.
		if language == "c" && looksLikeCPP(content) {
			if cppSyms, cppImports, cppRoot, cppCleanup, ok := reparseAsCPP(content); ok {
				if len(cppSyms) >= len(syms) {
					cleanup()
					return cppSyms, cppImports, cppRoot, cppCleanup
				}
				cppCleanup()
			}
		}
	default:
		cleanup()
		return nil, nil, nil, func() {}
	}
	return syms, imports, root, cleanup
}

// cppOnlyTokenRe matches tokens that are valid C++ but not C, used to decide
// whether a `.h` that failed to parse as C should be retried as C++. Word
// boundaries avoid matching identifiers that merely contain these substrings.
var cppOnlyTokenRe = regexp.MustCompile(`\b(namespace|template|class|public:|private:|protected:)\b|::|\boperator\b`)

// looksLikeCPP reports whether content contains C++-only constructs. Conservative
// by design: only triggers a cpp re-parse for headers the C grammar cannot model.
func looksLikeCPP(content []byte) bool {
	return cppOnlyTokenRe.Match(content)
}

// reparseAsCPP parses content with the cpp grammar and walks it, returning the
// live root + cleanup so callers can still extract call edges. ok is false if the
// cpp parse fails entirely.
func reparseAsCPP(content []byte) (syms []symbolDraft, imports []string, root *tree_sitter.Node, cleanup func(), ok bool) {
	ptr := languagePointer("cpp")
	if ptr == nil {
		return nil, nil, nil, func() {}, false
	}
	lang := tree_sitter.NewLanguage(ptr)
	if lang == nil {
		return nil, nil, nil, func() {}, false
	}
	p := tree_sitter.NewParser()
	if err := p.SetLanguage(lang); err != nil {
		p.Close()
		return nil, nil, nil, func() {}, false
	}
	tree := p.Parse(content, nil)
	if tree == nil {
		p.Close()
		return nil, nil, nil, func() {}, false
	}
	root = tree.RootNode()
	if root == nil {
		tree.Close()
		p.Close()
		return nil, nil, nil, func() {}, false
	}
	syms, imports = walkCAST(root, content)
	return syms, imports, root, func() { tree.Close(); p.Close() }, true
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
	pyFromRe   = regexp.MustCompile(`from\s+([\w.]+)\s+import\s+([^\n#]+)`)
	pyImportRe = regexp.MustCompile(`(?m)^\s*import\s+([^\n#]+)`)
)

func extractPythonImport(node *tree_sitter.Node, src []byte) []string {
	text := nodeText(node, src)
	var out []string
	for _, m := range pyFromRe.FindAllStringSubmatch(text, -1) {
		module := strings.TrimSpace(m[1])
		if module == "" {
			continue
		}
		out = append(out, module)
		for _, part := range strings.Split(m[2], ",") {
			name := pythonImportName(part)
			if name == "" || name == "*" {
				continue
			}
			out = append(out, module+"."+name)
		}
	}
	for _, m := range pyImportRe.FindAllStringSubmatch(text, -1) {
		for _, part := range strings.Split(m[1], ",") {
			if p := pythonImportName(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func pythonImportName(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return ""
	}
	if fields := strings.Fields(part); len(fields) > 0 {
		return strings.TrimSpace(fields[0])
	}
	return ""
}

var (
	jsImportFromRe   = regexp.MustCompile(`\bfrom\s+['"]([^'"]+)['"]`)
	jsImportBareRe   = regexp.MustCompile(`(?m)^\s*import\s+['"]([^'"]+)['"]`)
	jsRequireRe      = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	jsExportAssignRe = regexp.MustCompile(`\bmodule\.exports\b|\bexports\.[A-Za-z_$][A-Za-z0-9_$]*`)
)

func extractJSImportsFromText(text string) []string {
	out := make([]string, 0)
	for _, re := range []*regexp.Regexp{jsImportFromRe, jsImportBareRe, jsRequireRe} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
				out = append(out, strings.TrimSpace(m[1]))
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
			imports = append(imports, extractJSImportsFromText(nodeText(node, src))...)
		case "export_statement":
			imports = append(imports, extractJSImportsFromText(nodeText(node, src))...)
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
			imports = append(imports, extractJSImportsFromText(nodeText(node, src))...)
			symbols = append(symbols, jsVariableSymbols(node, src)...)
		case "expression_statement":
			text := nodeText(node, src)
			if jsExportAssignRe.MatchString(text) {
				imports = append(imports, extractJSImportsFromText(text)...)
			}
		case "assignment_expression":
			// X.foo = function(){} / exports.bar = () => {} /
			// module.exports.baz = function(){}. Anonymous function/arrow
			// expressions bound to a member target are definitions (e.g. the
			// Express public API) that the named-fn-expr path never sees.
			if sym, ok := jsAssignedFunctionSymbol(node, src); ok {
				symbols = append(symbols, sym)
			}
		}
		return true
	})
	return symbols, imports
}

// jsAssignedFunctionSymbol promotes `LHS = <anonymous fn/arrow>` assignments to
// definitions when the LHS is a member_expression (object.property, including
// prototype/exports/module.exports chains). The emitted name is the bare LHS
// property (matching the class-method naming convention), with the dotted
// qualifier captured as recvType + an owner_type hint for disambiguation.
//
// Only ANONYMOUS right-hand sides are emitted: named function expressions
// (`X.foo = function foo(){}`) are already captured by the function_expression
// path, so emitting here would double-count them. const/let arrow declarations
// are likewise handled by jsVariableSymbols, so this stays scoped to
// member-expression targets.
func jsAssignedFunctionSymbol(node *tree_sitter.Node, src []byte) (symbolDraft, bool) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return symbolDraft{}, false
	}
	if !jsValueIsFunction(right) {
		return symbolDraft{}, false
	}
	// Skip named function/arrow expressions — already emitted by simpleSymbol.
	if jsFunctionExprHasName(right, src) {
		return symbolDraft{}, false
	}
	if left.Kind() != "member_expression" {
		return symbolDraft{}, false
	}
	prop := left.ChildByFieldName("property")
	if prop == nil {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(prop, src))
	if name == "" {
		return symbolDraft{}, false
	}
	owner := ""
	if obj := left.ChildByFieldName("object"); obj != nil {
		// Reject literal-rooted targets (`"x".foo = fn`): these are never valid
		// assignment LHS in real code and only arise when the non-TSX grammar
		// mis-parses a `.tsx` JSX attribute (e.g. data-x="y" onClick={()=>...}).
		switch obj.Kind() {
		case "string", "template_string", "number", "regex", "true", "false", "null":
			return symbolDraft{}, false
		}
		owner = strings.TrimSpace(nodeText(obj, src))
	}
	sig := strings.TrimSpace(nodeText(left, src)) + " = " + jsValueFunctionHead(right, src)
	if len(sig) > 160 {
		sig = sig[:160] + "..."
	}
	sym := symbolDraft{
		name:      name,
		kind:      "method",
		signature: sig,
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
		metadata:  graph.JSONBMap{"scope": "assignment"},
	}
	if owner != "" {
		sym.recvType = owner
		sym.metadata["owner_type"] = owner
	}
	return sym, true
}

// jsFunctionExprHasName reports whether a function/arrow expression node has its
// own identifier (named function expression). Arrow functions are always
// anonymous; only function_expression/function nodes can carry a name.
func jsFunctionExprHasName(node *tree_sitter.Node, src []byte) bool {
	if node == nil {
		return false
	}
	if name := node.ChildByFieldName("name"); name != nil {
		return strings.TrimSpace(nodeText(name, src)) != ""
	}
	return false
}

// jsValueFunctionHead returns a compact one-line head of a function/arrow value
// for use in an assignment signature (e.g. "function(req, res)").
func jsValueFunctionHead(node *tree_sitter.Node, src []byte) string {
	head := strings.TrimSpace(nodeText(node, src))
	if i := strings.IndexByte(head, '{'); i > 0 {
		head = strings.TrimSpace(head[:i])
	}
	if nl := strings.IndexByte(head, '\n'); nl > 0 {
		head = strings.TrimSpace(head[:nl])
	}
	return head
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

	for i := uint(0); i < body.ChildCount(); i++ {
		member := body.Child(i)
		if member == nil {
			continue
		}
		out = append(out, javaMemberSymbols(member, src, typeName)...)
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
		out := javaCallableSymbol(member, src, owner, "method")
		// Descend into the method body: anonymous classes, local types.
		out = append(out, javaNestedTypeSymbols(member, src, owner)...)
		return out
	case "constructor_declaration", "compact_constructor_declaration":
		out := javaCallableSymbol(member, src, owner, "constructor")
		out = append(out, javaNestedTypeSymbols(member, src, owner)...)
		return out
	case "field_declaration", "constant_declaration":
		out := javaFieldSymbols(member, src, owner, "field")
		// Field initializers can host anonymous classes (e.g. a factory field
		// initialized to `new TypeAdapterFactory(){...}`).
		out = append(out, javaNestedTypeSymbols(member, src, owner)...)
		return out
	case "enum_constant":
		return javaEnumConstantSymbol(member, src, owner)
	case "record_component":
		return javaRecordComponentSymbol(member, src, owner)
	case "annotation_type_element_declaration":
		return javaCallableSymbol(member, src, owner, "annotation_member")
	case "static_initializer", "block":
		// Static / instance initializer blocks can host anonymous classes and
		// local types whose members are real definitions.
		return javaNestedTypeSymbols(member, src, owner)
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

// javaNestedTypeSymbols walks the subtree of a member declaration (method,
// constructor, field initializer) looking for type definitions that live inside
// executable bodies — anonymous classes (`object_creation_expression` with a
// `class_body`) and local classes/records/interfaces/enums declared inside a
// block. Members of those types are emitted as real definitions so they are not
// lost from the symbol index.
//
// It walks DOWN through expressions/statements but STOPS at the boundary of a
// named type body or anonymous class body, handing that subtree to the
// appropriate extractor (which recurses on its own). This keeps each type body
// processed exactly once and avoids double-counting.
func javaNestedTypeSymbols(node *tree_sitter.Node, src []byte, owner string) []symbolDraft {
	var out []symbolDraft
	var walk func(n *tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "object_creation_expression":
			// Anonymous class: `new Type(...) { members }`. The class_body holds
			// the overriding/added members. The instantiated type names the
			// receiver so qualified names stay deterministic.
			if cb := childNode(n, "class_body"); cb != nil {
				anonOwner := javaAnonymousOwner(n, src, owner)
				for i := uint(0); i < cb.ChildCount(); i++ {
					out = append(out, javaMemberSymbols(cb.Child(i), src, anonOwner)...)
				}
				// Continue scanning the constructor argument list (which can host
				// further anonymous classes) but NOT the class_body again.
				for i := uint(0); i < n.ChildCount(); i++ {
					if c := n.Child(i); c != nil && c.Kind() != "class_body" {
						walk(c)
					}
				}
				return
			}
		case "class_declaration", "interface_declaration", "enum_declaration",
			"record_declaration", "annotation_type_declaration":
			// Local type declared inside a block. javaTypeSymbols recurses into
			// its body itself, so do not descend further here.
			out = append(out, javaTypeSymbols(n, src)...)
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

// javaAnonymousOwner derives a deterministic owner-type name for the members of
// an anonymous class from the type being instantiated, qualified by the
// enclosing owner (e.g. "Excluder$TypeAdapter"). Falls back to the enclosing
// owner alone when the instantiated type cannot be read.
func javaAnonymousOwner(objCreation *tree_sitter.Node, src []byte, owner string) string {
	typeNode := objCreation.ChildByFieldName("type")
	base := ""
	if typeNode != nil {
		base = javaBareTypeName(nodeText(typeNode, src))
	}
	switch {
	case base != "" && owner != "":
		return owner + "$" + base
	case base != "":
		return base
	default:
		return owner
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
	out := []symbolDraft{{
		name:      name,
		kind:      "enum_constant",
		signature: javaOneLineSignature(node, src),
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
		metadata:  javaOwnerMetadata(owner),
		recvType:  owner,
	}}
	// An enum constant may carry a body that overrides/adds members
	// (`CONST() { @Override ... }`). Those are real method/field defs owned by
	// the constant's specialized type.
	if cb := childNode(node, "class_body"); cb != nil {
		constOwner := owner
		if owner != "" {
			constOwner = owner + "$" + name
		} else {
			constOwner = name
		}
		for i := uint(0); i < cb.ChildCount(); i++ {
			out = append(out, javaMemberSymbols(cb.Child(i), src, constOwner)...)
		}
	}
	return out
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
		case "enum_specifier":
			// A named enum (`enum Direction {...}`) carries its name as a direct
			// type_identifier. typedef'd anonymous enums are handled via the
			// enclosing type_definition case below, so skip nameless ones here.
			if sym, ok := cEnumSymbol(child, src, ""); ok {
				symbols = append(symbols, sym)
			}
			return false
		case "type_definition":
			// `typedef enum {...} ValueType;` — the enum_specifier is anonymous
			// and the real type name is the type_definition's declarator. Emit it
			// under the typedef name.
			symbols = append(symbols, cTypedefEnumSymbols(child, src)...)
			return false
		}
		return true
	})
	return symbols, imports
}

// cEnumSymbol builds an enum definition from an enum_specifier. When the enum is
// anonymous (no type_identifier, e.g. inside a typedef) the caller supplies the
// typedef name via fallbackName. Returns false when no name is determinable
// (truly anonymous enums have no def name worth indexing).
func cEnumSymbol(node *tree_sitter.Node, src []byte, fallbackName string) (symbolDraft, bool) {
	name := childText(node, "type_identifier", src)
	if name == "" {
		name = strings.TrimSpace(fallbackName)
	}
	if name == "" {
		return symbolDraft{}, false
	}
	sig := nodeText(node, src)
	if nl := strings.IndexByte(sig, '\n'); nl > 0 {
		sig = sig[:nl]
	}
	return symbolDraft{
		name:      name,
		kind:      "enum",
		signature: strings.TrimSpace(sig),
		startLine: int(node.StartPosition().Row) + 1,
		endLine:   int(node.EndPosition().Row) + 1,
	}, true
}

// cTypedefEnumSymbols handles `typedef enum {...} Name;`. It locates the wrapped
// enum_specifier and names it after the typedef's declarator identifier. Returns
// nothing when the type_definition does not wrap an enum.
func cTypedefEnumSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	var enumNode *tree_sitter.Node
	for i := uint(0); i < node.ChildCount(); i++ {
		if c := node.Child(i); c != nil && c.Kind() == "enum_specifier" {
			enumNode = c
			break
		}
	}
	if enumNode == nil {
		return nil
	}
	// The typedef name is the type_definition's trailing type_identifier (or a
	// declarator wrapping one). If the enum_specifier itself is named, prefer that.
	name := childText(enumNode, "type_identifier", src)
	if name == "" {
		name = childText(node, "type_identifier", src)
	}
	if sym, ok := cEnumSymbol(enumNode, src, name); ok {
		// Anchor the def to the typedef span so line ranges cover the full decl.
		sym.startLine = int(node.StartPosition().Row) + 1
		sym.endLine = int(node.EndPosition().Row) + 1
		return []symbolDraft{sym}
	}
	return nil
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
		if member == nil {
			continue
		}
		switch member.Kind() {
		case "function_definition":
			// Inline-defined member (`int bar() { ... }`).
			name := cppFunctionName(member, src)
			if name == "" || isMacroAnnotationName(name) {
				// Skip Clang annotation / test macros mis-parsed as members
				// (e.g. a method whose body trails LOCKS_EXCLUDED(mu_)).
				continue
			}
			out = append(out, symbolDraft{
				name:      name,
				kind:      cppMemberKind(name, typeName),
				startLine: int(member.StartPosition().Row) + 1,
				endLine:   int(member.EndPosition().Row) + 1,
				recvType:  typeName,
			})
		case "field_declaration", "declaration":
			// A nested enum is a field_declaration wrapping an enum_specifier
			// (`enum Inner { A, B };`); emit it as an enum def.
			if enumNode := childNode(member, "enum_specifier"); enumNode != nil {
				if sym, ok := cEnumSymbol(enumNode, src, ""); ok {
					out = append(out, sym)
				}
				continue
			}
			// Declared-only member: a method prototype (`int bar(int x);`), a
			// constructor (`Foo();`) or destructor (`~Foo();`) — common in headers
			// where the definition lives in a .cc. Only emit when a
			// function_declarator is present so plain data fields are skipped.
			if sym, ok := cppMemberDeclSymbol(member, src, typeName); ok {
				out = append(out, sym)
			}
		case "enum_specifier":
			// A nested enum declared directly in the class body.
			if sym, ok := cEnumSymbol(member, src, ""); ok {
				out = append(out, sym)
			}
		}
	}
	return out
}

// cppMemberKind classifies a member function by name relative to its enclosing
// type: a member whose name equals the type name is a constructor; everything
// else (including destructors `~Type`) is a method, matching clangd's reporting.
func cppMemberKind(name, typeName string) string {
	if typeName != "" && name == typeName {
		return "constructor"
	}
	return "method"
}

// cppMemberDeclSymbol extracts a declared-only class member (no body) from a
// field_declaration / declaration node, capturing methods, constructors and
// destructors. Returns false for non-callable declarations (data fields, typedefs).
func cppMemberDeclSymbol(member *tree_sitter.Node, src []byte, typeName string) (symbolDraft, bool) {
	decl := cFindFunctionDeclarator(member)
	if decl == nil {
		return symbolDraft{}, false
	}
	name := cppDeclaratorName(decl, src)
	if name == "" {
		return symbolDraft{}, false
	}
	// Clang thread-safety annotations trail a data-field declaration
	// (e.g. `int x_ GUARDED_BY(mu_);`, `EXCLUSIVE_LOCKS_REQUIRED(mu_)`,
	// `LOCKS_EXCLUDED(mu_)`) and tree-sitter-cpp mis-parses the ALL-CAPS macro as a
	// function declarator on the field. They are never real members — skip them so
	// they don't inflate method definitions (precision).
	if isMacroAnnotationName(name) {
		return symbolDraft{}, false
	}
	return symbolDraft{
		name:      name,
		kind:      cppMemberKind(name, typeName),
		startLine: int(member.StartPosition().Row) + 1,
		endLine:   int(member.EndPosition().Row) + 1,
		recvType:  typeName,
	}, true
}

// isMacroAnnotationName reports whether name is an ALL-CAPS underscored macro
// identifier (e.g. a Clang annotation like GUARDED_BY). C/C++ never name a real
// member that way, so a "method" parsed with such a name is a mis-parsed macro.
func isMacroAnnotationName(name string) bool {
	if !strings.Contains(name, "_") {
		return false
	}
	// All-caps (equals its own uppercase) and contains at least one letter
	// (differs from its own lowercase).
	return name == strings.ToUpper(name) && name != strings.ToLower(name)
}

// cppFunctionName pulls the declared name out of a C++ function_definition's
// function_declarator, handling plain identifiers and field identifiers.
func cppFunctionName(node *tree_sitter.Node, src []byte) string {
	name, _, _ := cppFunctionIdentity(node, src)
	return name
}

func cppFunctionIdentity(node *tree_sitter.Node, src []byte) (name, qualified, recvType string) {
	decl := cFindFunctionDeclarator(node)
	if decl == nil {
		return "", "", ""
	}
	name = cppDeclaratorName(decl, src)
	qualified = cppDeclaratorQualifiedName(decl, src)
	if qualified != "" {
		recvType = cppQualifiedReceiver(qualified)
	}
	return name, qualified, recvType
}

// cFindFunctionDeclarator locates the function_declarator inside a
// function_definition's declarator slot. Pointer/reference-returning functions
// (`cJSON *foo(...)`, `unsigned char* ensure(...)`, `Foo& bar(...)`) wrap the
// function_declarator in one or more pointer_declarator / reference_declarator
// nodes, so a flat scan of direct children misses them. We unwrap those layers
// down to the function_declarator. Multi-line signatures parse to the same shape,
// so this also recovers functions whose return type / params span lines.
func cFindFunctionDeclarator(node *tree_sitter.Node) *tree_sitter.Node {
	if node == nil {
		return nil
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "function_declarator":
			return child
		case "pointer_declarator", "reference_declarator":
			if d := cFindFunctionDeclarator(child); d != nil {
				return d
			}
		}
	}
	return nil
}

func cFunctionSymbol(node *tree_sitter.Node, src []byte) (symbolDraft, bool) {
	if cFindFunctionDeclarator(node) == nil {
		return symbolDraft{}, false
	}
	name, qualified, recvType := cppFunctionIdentity(node, src)
	if name == "" {
		return symbolDraft{}, false
	}
	// ALL-CAPS underscored "functions" are macro invocations mis-parsed as
	// function definitions — Clang annotations (EXCLUSIVE_LOCKS_REQUIRED,
	// LOCKS_EXCLUDED) and test-framework macros (TEST_F, TEST_P). clangd, the
	// semantic truth, never emits them as symbols, so neither should Atlas.
	if isMacroAnnotationName(name) {
		return symbolDraft{}, false
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

// childNode returns the first direct child of the given Kind, or nil.
func childNode(node *tree_sitter.Node, kind string) *tree_sitter.Node {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil && child.Kind() == kind {
			return child
		}
	}
	return nil
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
