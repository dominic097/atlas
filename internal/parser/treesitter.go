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
			// An unbalanced function-like macro in THIS function's body (Unity's
			// EXPECT_ABORT_BEGIN opens a brace VERIFY_FAILS_END closes) can make
			// tree-sitter nest the FOLLOWING function inside this one's body-ERROR.
			// Recover those buried siblings; guarded on an ERROR child so well-formed
			// bodies are untouched.
			if body := child.ChildByFieldName("body"); body != nil && cBodyHasError(body) {
				symbols = append(symbols, recoverFromError(body, src)...)
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
			// `typedef enum {...} ValueType;` and `typedef struct NAME {...} NAME;`.
			// The wrapped specifier may be anonymous, so the real type name is the
			// type_definition's trailing declarator. Emit under the typedef name.
			symbols = append(symbols, cTypedefSymbols(child, src)...)
			return false
		case "ERROR":
			// tree-sitter has no C preprocessor, so unbalanced function-like macros
			// (Unity's EXPECT_ABORT_BEGIN/VERIFY_FAILS_END) and #if-interleaved code
			// (leveldb's Limiter member-init list) produce ERROR nodes that swallow
			// real definitions. Descend into the ERROR subtree and recover the
			// orphaned class/struct/function/enum nodes buried inside. Each recovered
			// node is a genuine syntactic definition, so precision is preserved; the
			// caller de-dups so a def reachable by both paths is emitted once.
			symbols = append(symbols, recoverFromError(child, src)...)
			return false
		}
		return true
	})
	return dedupDrafts(symbols), imports
}

// dedupDrafts collapses drafts that name the SAME definition (same name + start
// line) so a def reachable by multiple recovery paths is emitted once. ERROR-node
// recovery can rediscover a definition the normal walk already produced — e.g. a
// member recovered as a `method` (with receiver context) by cppTypeSymbols AND as
// a bare `function` by the flat shred scanner. When two drafts collide we keep the
// RICHER one (a method/constructor carrying a recv_type beats a bare function),
// which preserves both correct kind and precision (honest def counts). A name at a
// given source line is a single definition, so this never merges distinct symbols.
func dedupDrafts(in []symbolDraft) []symbolDraft {
	if len(in) < 2 {
		return in
	}
	idx := make(map[string]int, len(in))
	out := make([]symbolDraft, 0, len(in))
	for _, d := range in {
		// Collapse callables (function/method/constructor) that share a name+line
		// regardless of kind — that is the SAME definition reached two ways (e.g.
		// recovered as a method by cppTypeSymbols and as a bare function by the
		// shred scanner). Types keep their kind in the key so a one-line
		// `class Foo { Foo(); }` never merges the class with its constructor.
		family := d.kind
		if cCallableKind(d.kind) {
			family = "callable"
		}
		k := family + "\x00" + d.name + "\x00" + itoa(d.startLine)
		if at, ok := idx[k]; ok {
			// Keep the richer draft: a method/constructor with a receiver type beats
			// a bare function (correct kind + honest count = precision).
			if out[at].recvType == "" && d.recvType != "" {
				out[at] = d
			}
			continue
		}
		idx[k] = len(out)
		out = append(out, d)
	}
	return out
}

// cCallableKind reports whether a draft kind is a callable definition (function,
// method or constructor) for dedup-family grouping.
func cCallableKind(kind string) bool {
	switch kind {
	case "function", "method", "constructor":
		return true
	}
	return false
}

// recoverableContainerKinds are nodes that, when buried under an ERROR, may still
// hold well-formed definitions we should descend into during error recovery.
var recoverableContainerKinds = map[string]bool{
	"ERROR":                 true,
	"preproc_if":            true,
	"preproc_ifdef":         true,
	"preproc_else":          true,
	"preproc_elif":          true,
	"linkage_specification": true, // extern "C" { ... }
	"declaration_list":      true,
	"namespace_definition":  true,
}

// recoverFromError walks an ERROR subtree (and any nested preproc/container nodes)
// and emits the genuine definitions buried inside it:
//   - class_specifier / struct_specifier → cppTypeSymbols (recurses members)
//   - function_definition / enum_specifier → their normal handlers
//   - an orphaned function_declarator immediately followed by a `{` sibling: the
//     ERROR split a real `RetType name(args) { body }` into loose siblings (Unity
//     testXxx defs), so reassemble it into a function/method draft.
//
// It does NOT emit bare declarators without a following body brace (those are
// prototypes/macro calls), so no forward-declaration or macro noise is added.
func recoverFromError(node *tree_sitter.Node, src []byte) []symbolDraft {
	if node == nil {
		return nil
	}
	var out []symbolDraft
	n := node.ChildCount()
	for i := uint(0); i < n; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "class_specifier", "struct_specifier":
			out = append(out, cppTypeSymbols(child, src)...)
		case "function_definition":
			if sym, ok := cFunctionSymbol(child, src); ok {
				out = append(out, sym)
			}
			// A buried function's OWN body may in turn ERROR-swallow the next
			// function (Unity chains testA→testB→testC through nested body-ERRORs),
			// so recurse into it when corrupted.
			if body := child.ChildByFieldName("body"); body != nil && cBodyHasError(body) {
				out = append(out, recoverFromError(body, src)...)
			}
		case "enum_specifier":
			if sym, ok := cEnumSymbol(child, src, ""); ok {
				out = append(out, sym)
			}
		case "type_definition":
			out = append(out, cTypedefSymbols(child, src)...)
		case "field_declaration", "declaration":
			// A real class/struct/enum DEFINITION buried under an ERROR is wrapped in
			// a field_declaration/declaration (e.g. env_posix.cc's PosixSequentialFile
			// nested in the #if-mangled Limiter body). Unwrap it. We only recurse into
			// aggregate/enum specifiers (real defs); a bare field_declaration with a
			// function_declarator here is a prototype with no body, so it is skipped
			// (precision — no forward-decl noise).
			if nested := childNode(child, "class_specifier"); nested != nil {
				out = append(out, cppTypeSymbols(nested, src)...)
			} else if nested := childNode(child, "struct_specifier"); nested != nil {
				out = append(out, cppTypeSymbols(nested, src)...)
			} else if e := childNode(child, "enum_specifier"); e != nil {
				if sym, ok := cEnumSymbol(e, src, ""); ok {
					out = append(out, sym)
				}
			}
		case "function_declarator":
			// Orphaned declarator: a real definition only if a `{` body brace
			// follows it among the ERROR's children (guards against prototypes).
			if errorDeclaratorHasBody(node, i) {
				if sym, ok := cOrphanFunctionSymbol(child, src); ok {
					out = append(out, sym)
				}
			}
		default:
			if recoverableContainerKinds[child.Kind()] {
				out = append(out, recoverFromError(child, src)...)
			}
		}
	}
	// Deeply-shredded function headers (Unity testunity.c: an unbalanced
	// function-like macro fragments `void testXxx(void) { ... }` so the name,
	// parens and body brace land in different ERROR fragments under this node and
	// never form a function_declarator). Scan the flat token stream of THIS ERROR
	// for `<type> <name> ( ... ) {` definitions the structural cases above missed.
	out = append(out, recoverShreddedFunctions(node, src)...)
	return out
}

// cBodyHasError reports whether a function/compound body contains a direct ERROR
// child — the cheap guard that an unbalanced macro corrupted it and may be hiding
// a following definition that recoverFromError should pull out.
func cBodyHasError(body *tree_sitter.Node) bool {
	for i := uint(0); i < body.ChildCount(); i++ {
		if c := body.Child(i); c != nil && c.Kind() == "ERROR" {
			return true
		}
	}
	return false
}

// shredTok is a flattened, document-ordered token used to recognise a shredded
// function header `<type> <name> ( params ) {` whose pieces tree-sitter scattered
// across ERROR fragments. Only the few token kinds that disambiguate a definition
// from a macro call / prototype are collected.
type shredTok struct {
	kind string // "type" | "id" | "(" | ")" | "{" | ";"
	text string
	line int
}

// recoverShreddedFunctions reconstructs function DEFINITIONS from the flat token
// stream of an ERROR subtree. It emits a symbol only for the precise sequence
//
//	<type> <identifier> ( … balanced … ) {
//
// i.e. a name preceded by a return type and FOLLOWED by a body-opening `{` (not a
// `;`). A trailing `;` (prototype) or no brace (macro call/statement) is rejected,
// and ALL-CAPS macro names are dropped — so assertion-macro invocations like
// `TEST_ASSERT_INT_WITHIN(...)` are never emitted (precision preserved). This only
// fires inside ERROR nodes, so well-formed code is untouched and deterministic.
func recoverShreddedFunctions(errNode *tree_sitter.Node, src []byte) []symbolDraft {
	var toks []shredTok
	collectShredTokens(errNode, src, &toks)
	if len(toks) < 4 {
		return nil
	}
	var out []symbolDraft
	seen := map[string]struct{}{}
	for i := 0; i+2 < len(toks); i++ {
		// pattern start: type, id, "("
		if toks[i].kind != "type" || toks[i+1].kind != "id" || toks[i+2].kind != "(" {
			continue
		}
		name := toks[i+1].text
		if name == "" || isMacroAnnotationName(name) {
			continue
		}
		// find the matching ")" with paren depth, then require the next
		// brace/semicolon token to be "{" (a body) — that is what marks a def.
		depth := 0
		j := i + 2
		closed := -1
		for ; j < len(toks); j++ {
			switch toks[j].kind {
			case "(":
				depth++
			case ")":
				depth--
				if depth == 0 {
					closed = j
				}
			}
			if closed >= 0 {
				break
			}
		}
		if closed < 0 || closed+1 >= len(toks) {
			continue
		}
		next := toks[closed+1]
		if next.kind != "{" {
			// ";" (prototype) or anything else (macro call / expression) → not a def.
			continue
		}
		key := name + "\x00" + itoa(toks[i+1].line)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, symbolDraft{
			name:      name,
			kind:      "function",
			signature: name + "()",
			startLine: toks[i+1].line,
			endLine:   next.line,
		})
		i = closed // skip past the consumed header
	}
	return out
}

// collectShredTokens DFS-flattens an ERROR subtree into the minimal token stream
// recoverShreddedFunctions needs. A `parenthesized_declarator`/`parameter_list`
// boundary still contributes its `(`/`)` leaves, so the params of a scattered
// header are seen in order.
func collectShredTokens(node *tree_sitter.Node, src []byte, out *[]shredTok) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case "primitive_type", "sized_type_specifier":
		*out = append(*out, shredTok{kind: "type", text: nodeText(node, src), line: int(node.StartPosition().Row) + 1})
		return
	case "type_identifier":
		*out = append(*out, shredTok{kind: "type", text: nodeText(node, src), line: int(node.StartPosition().Row) + 1})
		return
	case "identifier", "field_identifier":
		*out = append(*out, shredTok{kind: "id", text: nodeText(node, src), line: int(node.StartPosition().Row) + 1})
		return
	case "(":
		*out = append(*out, shredTok{kind: "(", line: int(node.StartPosition().Row) + 1})
		return
	case ")":
		*out = append(*out, shredTok{kind: ")", line: int(node.StartPosition().Row) + 1})
		return
	case "{":
		*out = append(*out, shredTok{kind: "{", line: int(node.StartPosition().Row) + 1})
		return
	case "}":
		// closing brace is not needed for header recognition; skip.
		return
	case ";":
		*out = append(*out, shredTok{kind: ";", line: int(node.StartPosition().Row) + 1})
		return
	case "compound_statement":
		// A well-formed body — its leading `{` marks a definition; record it then stop.
		*out = append(*out, shredTok{kind: "{", line: int(node.StartPosition().Row) + 1})
		return
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		collectShredTokens(node.Child(i), src, out)
	}
}

// errorDeclaratorHasBody reports whether the child at index decl in an ERROR
// node's child list is immediately followed (skipping comments/preproc trivia)
// by a `{` token — i.e. the orphaned declarator opens a function body, marking it
// a definition rather than a prototype.
func errorDeclaratorHasBody(errNode *tree_sitter.Node, decl uint) bool {
	n := errNode.ChildCount()
	for j := decl + 1; j < n; j++ {
		c := errNode.Child(j)
		if c == nil {
			continue
		}
		switch c.Kind() {
		case "comment":
			continue
		case "{", "compound_statement":
			return true
		default:
			return false
		}
	}
	return false
}

// cOrphanFunctionSymbol builds a function/method draft from a function_declarator
// recovered out of an ERROR node. Mirrors cFunctionSymbol's identity + macro guard
// but anchors the span on the declarator (the function_definition wrapper is gone).
func cOrphanFunctionSymbol(decl *tree_sitter.Node, src []byte) (symbolDraft, bool) {
	name := cppDeclaratorName(decl, src)
	if name == "" || isMacroAnnotationName(name) {
		return symbolDraft{}, false
	}
	qualified := cppDeclaratorQualifiedName(decl, src)
	recvType := ""
	if qualified != "" {
		recvType = cppQualifiedReceiver(qualified)
	}
	kind := "function"
	meta := graph.JSONBMap{}
	if recvType != "" {
		kind = "method"
		meta["owner_type"] = recvType
		meta["qualified_name"] = qualified
	}
	sig := nodeText(decl, src)
	if nl := strings.IndexByte(sig, '\n'); nl > 0 {
		sig = sig[:nl]
	}
	return symbolDraft{
		name:      name,
		kind:      kind,
		signature: strings.TrimSpace(sig),
		startLine: int(decl.StartPosition().Row) + 1,
		endLine:   int(decl.EndPosition().Row) + 1,
		metadata:  meta,
		recvType:  recvType,
	}, true
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

// cTypedefSymbols handles `typedef <specifier> {...} Name;` where the specifier is
// an enum (`typedef enum {...} ValueType;`), a struct (`typedef struct cJSON {...}
// cJSON;`), a union, or a class. It names the type after the typedef's trailing
// declarator identifier (preferring the specifier's own name when present), and
// for struct/union/class also emits the member functions via cppTypeSymbols.
//
// The typedef NAME is what callers reference, so it is always the emitted def name
// (matching clangd, which reports the typedef'd type under that name). Returns
// nothing when the type_definition wraps no aggregate/enum specifier (a scalar
// alias like `typedef int cJSON_bool;` is not a definition — precision).
func cTypedefSymbols(node *tree_sitter.Node, src []byte) []symbolDraft {
	// The typedef name is the type_definition's trailing type_identifier.
	typedefName := childText(node, "type_identifier", src)

	var spec *tree_sitter.Node
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		switch c.Kind() {
		case "enum_specifier", "struct_specifier", "union_specifier", "class_specifier":
			spec = c
		}
		if spec != nil {
			break
		}
	}
	if spec == nil {
		return nil
	}

	if spec.Kind() == "enum_specifier" {
		aggName := childText(spec, "type_identifier", src)
		name := aggName
		if name == "" {
			name = typedefName
		}
		var syms []symbolDraft
		if sym, ok := cEnumSymbol(spec, src, name); ok {
			// Anchor the def to the typedef span so line ranges cover the full decl.
			sym.startLine = int(node.StartPosition().Row) + 1
			sym.endLine = int(node.EndPosition().Row) + 1
			syms = append(syms, sym)
		}
		// `typedef enum TAG {...} ALIAS;` — clangd reports BOTH the tag and the
		// typedef alias. When they differ, add the alias too (only meaningful when
		// the enum has a body, i.e. a real definition).
		if typedefName != "" && typedefName != aggName && spec.ChildByFieldName("body") != nil {
			syms = append(syms, symbolDraft{
				name:      typedefName,
				kind:      "enum",
				startLine: int(node.StartPosition().Row) + 1,
				endLine:   int(node.EndPosition().Row) + 1,
			})
		}
		return syms
	}

	// struct / union / class: emit the type + its members via the shared walker,
	// but force the leading type's name to the typedef name when the aggregate is
	// anonymous (`typedef struct {...} Foo;`). When the aggregate is named
	// (`typedef struct cJSON {...} cJSON;`) cppTypeSymbols already names it.
	syms := cppTypeSymbols(spec, src)
	if len(syms) == 0 {
		// Anonymous aggregate with no captured leader — synthesize from typedef name.
		if typedefName == "" {
			return nil
		}
		kind := "struct"
		if spec.Kind() == "class_specifier" {
			kind = "class"
		}
		return []symbolDraft{{
			name:      typedefName,
			kind:      kind,
			startLine: int(node.StartPosition().Row) + 1,
			endLine:   int(node.EndPosition().Row) + 1,
		}}
	}
	// Emit the typedef NAME as its own type def when it is not already covered by
	// the aggregate's own name. Two cases:
	//   - anonymous aggregate (`typedef struct {...} Foo;`): cppTypeSymbols emitted
	//     no leading type, so Foo is the only type name.
	//   - named aggregate (`typedef struct GuardBytes {...} Guard;`): clangd reports
	//     BOTH the tag (GuardBytes) and the typedef alias (Guard) as types, so we
	//     add Guard alongside the tag cppTypeSymbols already emitted.
	aggName := childText(spec, "type_identifier", src)
	// Only when the aggregate actually has a body is this a DEFINITION; an
	// opaque `typedef struct Impl Handle;` (no body) is a forward declaration and
	// must not be emitted (precision — clangd-truth treats it as noise).
	hasBody := spec.ChildByFieldName("body") != nil
	if hasBody && typedefName != "" && typedefName != aggName {
		kind := "struct"
		if spec.Kind() == "class_specifier" {
			kind = "class"
		}
		lead := symbolDraft{
			name:      typedefName,
			kind:      kind,
			startLine: int(node.StartPosition().Row) + 1,
			endLine:   int(node.EndPosition().Row) + 1,
		}
		syms = append([]symbolDraft{lead}, syms...)
	}
	return syms
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

	// typeName is the receiver name stamped onto members (the immediate enclosing
	// type). qualified is the full dotted path for an out-of-line nested definition
	// (`class Outer::Inner { ... }`), whose name child is a qualified_identifier
	// rather than a plain type_identifier; "" for the common unqualified case.
	typeName, qualified := cppTypeName(node, src)
	if typeName != "" {
		draft := symbolDraft{
			name:      typeName,
			kind:      kind,
			startLine: int(node.StartPosition().Row) + 1,
			endLine:   int(node.EndPosition().Row) + 1,
		}
		// Out-of-line nested type: expose the qualified name so the query layer can
		// disambiguate (and a qualified-name-aware diff matches `Outer::Inner`). The
		// emitted symbol still NAMES the last segment, matching how clangd reports
		// the leaf while preserving the full path in metadata.
		if qualified != "" {
			draft.metadata = graph.JSONBMap{"qualified_name": qualified}
		}
		out = append(out, draft)
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
			// A nested class/struct defined inside this type's body
			// (`class ModelIter : public Iterator { ... };`) parses as a
			// field_declaration wrapping a class_specifier/struct_specifier. Recurse
			// so the nested type AND its inline members are captured.
			if nested := childNode(member, "class_specifier"); nested != nil {
				out = append(out, cppTypeSymbols(nested, src)...)
				continue
			}
			if nested := childNode(member, "struct_specifier"); nested != nil {
				out = append(out, cppTypeSymbols(nested, src)...)
				continue
			}
			// Declared-only member: a method prototype (`int bar(int x);`), a
			// constructor (`Foo();`) or destructor (`~Foo();`) — common in headers
			// where the definition lives in a .cc. Only emit when a
			// function_declarator is present so plain data fields are skipped.
			if sym, ok := cppMemberDeclSymbol(member, src, typeName); ok {
				out = append(out, sym)
			}
		case "class_specifier", "struct_specifier":
			// A nested type declared directly in the body (no field_declaration
			// wrapper, e.g. inside an anonymous-namespace or already-mangled parent).
			out = append(out, cppTypeSymbols(member, src)...)
		case "enum_specifier":
			// A nested enum declared directly in the class body.
			if sym, ok := cEnumSymbol(member, src, ""); ok {
				out = append(out, sym)
			}
		default:
			// #if-interleaved code (leveldb's Limiter member-init list) makes
			// tree-sitter bury the rest of an anonymous-namespace block inside a
			// preproc_if / ERROR child of THIS type's body, hiding sibling classes
			// (PosixSequentialFile/PosixEnv …) and their methods. Recover them by
			// descending into those containers, treating found defs as top-level
			// siblings (NOT members of this type — they are not, so no recv_type).
			if recoverableContainerKinds[member.Kind()] {
				out = append(out, recoverFromError(member, src)...)
			}
		}
	}
	return out
}

// cppTypeName resolves the name of a class_specifier / struct_specifier. Most
// types name themselves with a direct type_identifier (`class Foo`). An out-of-
// line nested definition (`class Outer::Inner { ... }`, `struct A::B::C { ... }`)
// instead carries a qualified_identifier name child, which the plain
// type_identifier lookup misses entirely — leaving the type unemitted and its
// members un-stamped. We unwrap it here:
//
//   - display is the leaf segment (`Inner`), matching how clangd names the symbol
//     and how the member-name dedup family keys.
//   - qualified is the full path (`Outer::Inner`) ONLY for the qualified case, so
//     the unqualified path is byte-for-byte unchanged (no new metadata, rounds
//     1-2 behaviour preserved).
//
// A template type's name child sits at the same position, so this also handles
// `template <...> class Outer::Inner`.
func cppTypeName(node *tree_sitter.Node, src []byte) (display, qualified string) {
	if name := childText(node, "type_identifier", src); name != "" {
		return name, ""
	}
	q := childNode(node, "qualified_identifier")
	if q == nil {
		return "", ""
	}
	full := strings.TrimSpace(nodeText(q, src))
	if full == "" {
		return "", ""
	}
	// A multi-line or whitespace-mangled qualifier is not a clean name — bail to
	// keep precision (no spurious type from a corrupted node).
	if strings.ContainsAny(full, " \t\n") {
		return "", ""
	}
	leaf := full
	if i := strings.LastIndex(full, "::"); i >= 0 {
		leaf = full[i+2:]
	}
	if leaf == "" {
		return "", ""
	}
	return leaf, full
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
//
// It also descends into a leading ERROR child: a method with a trailing
// thread-safety annotation (`int Read() LOCKS_EXCLUDED(mu_) { ... }`) parses as
// `function_definition( ERROR[ function_declarator "Read()" ],
// function_declarator "LOCKS_EXCLUDED(mu_)", body )`. Walking children in document
// order returns the REAL declarator (`Read`) buried in the ERROR before the
// trailing annotation declarator, so the def is captured under the right name and
// the macro declarator is left for isMacroAnnotationName to drop.
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
		case "pointer_declarator", "reference_declarator", "ERROR":
			if d := cFindFunctionDeclarator(child); d != nil {
				return d
			}
		}
	}
	return nil
}

func cFunctionSymbol(node *tree_sitter.Node, src []byte) (symbolDraft, bool) {
	if cFindFunctionDeclarator(node) == nil {
		// A function-like macro modifier between the return type and the name
		// (`int CJSON_CDECL main(void) { ... }`) makes tree-sitter peel `int
		// CJSON_CDECL` into a stray declaration and parse the rest as a
		// function_definition whose name is a bare type_identifier followed by a
		// parenthesized_declarator (the params) — there is NO function_declarator.
		// Recover it: this is a genuine definition (it has a compound_statement
		// body), so precision holds.
		if sym, ok := cMacroModifierFunctionSymbol(node, src); ok {
			return sym, true
		}
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

// cMacroModifierFunctionSymbol recovers `RET MACRO name(params) { body }` defs
// where a function-like calling-convention macro (CJSON_CDECL, WINAPI, STDCALL …)
// sits between the return type and the function name. tree-sitter then models the
// function_definition with a bare identifier/type_identifier name child directly
// followed by a parenthesized_declarator holding the params, and a body. Requires
// the body (compound_statement) so prototypes are not emitted (precision). The
// macro itself is never emitted as a symbol.
func cMacroModifierFunctionSymbol(node *tree_sitter.Node, src []byte) (symbolDraft, bool) {
	// Must be a real definition: a body must be present.
	if node.ChildByFieldName("body") == nil && childNode(node, "compound_statement") == nil {
		return symbolDraft{}, false
	}
	var nameNode, parenNode *tree_sitter.Node
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		switch c.Kind() {
		case "identifier", "type_identifier":
			// The LAST name token before the parenthesized_declarator is the
			// function name (an earlier one would be a return type, but those parse
			// as primitive_type/type_identifier on the peeled declaration).
			if parenNode == nil {
				nameNode = c
			}
		case "parenthesized_declarator":
			parenNode = c
		}
	}
	if nameNode == nil || parenNode == nil {
		return symbolDraft{}, false
	}
	name := strings.TrimSpace(nodeText(nameNode, src))
	if name == "" || isMacroAnnotationName(name) {
		return symbolDraft{}, false
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
