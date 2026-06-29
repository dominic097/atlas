package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

// parseGoFile extracts symbols, imports, and call edges from one native Go AST.
// The index path uses this instead of parsing once for symbols and again for
// calls. Public helpers below keep the older split surface for narrow tests.
func parseGoFile(path string, content []byte) ([]symbolDraft, []string, []graph.DependencyEdge) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, nil, nil
	}
	symbols, imports := parseGoSymbolsFromFile(fset, file, content)
	edges := goCallEdgesFromFile(fset, file, path)
	return symbols, imports, edges
}

// parseGoSymbols extracts functions, methods, and types from a Go source file
// using the native go/parser (compiler-grade fidelity), plus the import paths.
// Ported from pulse parseGoFile.
func parseGoSymbols(path string, content []byte) ([]symbolDraft, []string) {
	symbols, imports, _ := parseGoFile(path, content)
	return symbols, imports
}

func parseGoSymbolsFromFile(fset *token.FileSet, file *ast.File, content []byte) ([]symbolDraft, []string) {
	imports := make([]string, 0, len(file.Imports))
	for _, imported := range file.Imports {
		imports = append(imports, strings.Trim(imported.Path.Value, `"`))
	}

	var symbols []symbolDraft
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "function"
			recvType := ""
			if d.Recv != nil {
				kind = "method"
				recvType = goReceiverType(d.Recv)
			}
			symbols = append(symbols, symbolDraft{
				name:      d.Name.Name,
				kind:      kind,
				signature: goSignature(fset, d, content),
				startLine: fset.Position(d.Pos()).Line,
				endLine:   fset.Position(d.End()).Line,
				recvType:  recvType,
			})
		case *ast.GenDecl:
			if d.Tok == token.CONST {
				symbols = append(symbols, goConstSymbols(fset, d)...)
			}
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		symbols = append(symbols, symbolDraft{
			name:      typeSpec.Name.Name,
			kind:      "type",
			startLine: fset.Position(typeSpec.Pos()).Line,
			endLine:   fset.Position(typeSpec.End()).Line,
		})
		symbols = append(symbols, goTypeMemberSymbols(fset, typeSpec)...)
		return false
	})

	// Record the file's package on every symbol so the query layer can resolve a
	// qualified call to an in-repo package (e.g. `logrus.New()` from an external
	// test / example / cross-package file). pkgBase(path) cannot see a ROOT
	// package's name, so without this such calls were dropped as external.
	if file.Name != nil && file.Name.Name != "" {
		pkg := file.Name.Name
		for i := range symbols {
			if symbols[i].metadata == nil {
				symbols[i].metadata = graph.JSONBMap{}
			}
			symbols[i].metadata["package"] = pkg
		}
	}

	return symbols, imports
}

func goConstSymbols(fset *token.FileSet, decl *ast.GenDecl) []symbolDraft {
	var out []symbolDraft
	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range value.Names {
			out = append(out, symbolDraft{
				name:      name.Name,
				kind:      "constant",
				startLine: fset.Position(name.Pos()).Line,
				endLine:   fset.Position(name.End()).Line,
			})
		}
	}
	return out
}

func goTypeMemberSymbols(fset *token.FileSet, typeSpec *ast.TypeSpec) []symbolDraft {
	var out []symbolDraft
	owner := typeSpec.Name.Name
	switch typ := typeSpec.Type.(type) {
	case *ast.StructType:
		if typ.Fields == nil {
			return nil
		}
		for _, field := range typ.Fields.List {
			for _, name := range field.Names {
				out = append(out, symbolDraft{
					name:      name.Name,
					kind:      "field",
					startLine: fset.Position(name.Pos()).Line,
					endLine:   fset.Position(name.End()).Line,
					metadata:  graph.JSONBMap{"owner_type": owner},
				})
			}
		}
	case *ast.InterfaceType:
		if typ.Methods == nil {
			return nil
		}
		for _, method := range typ.Methods.List {
			if _, ok := method.Type.(*ast.FuncType); !ok {
				continue
			}
			for _, name := range method.Names {
				out = append(out, symbolDraft{
					name:      name.Name,
					kind:      "method_spec",
					startLine: fset.Position(name.Pos()).Line,
					endLine:   fset.Position(name.End()).Line,
					metadata:  graph.JSONBMap{"owner_type": owner},
				})
			}
		}
	}
	return out
}

// goSignature returns the source first line of a func declaration (the header),
// prefixed for methods. Falls back to the bare name if slicing fails.
func goSignature(fset *token.FileSet, fn *ast.FuncDecl, content []byte) string {
	start := fset.Position(fn.Pos()).Offset
	end := fset.Position(fn.End()).Offset
	if start >= 0 && end <= len(content) && start < end {
		sig := string(content[start:end])
		if nl := strings.IndexByte(sig, '\n'); nl > 0 {
			sig = sig[:nl]
		}
		sig = strings.TrimSpace(strings.TrimSuffix(sig, "{"))
		if sig != "" {
			return sig
		}
	}
	if fn.Recv != nil {
		return "(method) " + fn.Name.Name
	}
	return fn.Name.Name
}

// goCallEdges emits one EdgeCalls per call expression, naming the enclosing
// function/method (FromSymbol) and the callee (ToRef). Ported from pulse
// parseGoCallEdges, adapted to graph.DependencyEdge.
//
// Atlas addition (SHARED METADATA CONTRACT): for a method call x.M(...) it sets
// Metadata["recv_type"] to the statically inferred base type of receiver x, so
// the query layer can match the call to a method declared on that exact type
// (e.g. distinguish bleve.Index() from localEngine.Index()). Inference is
// lightweight and intra-function (no go/types); see goVarTypeTable.
func goCallEdges(filePath string, content []byte) []graph.DependencyEdge {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, 0)
	if err != nil {
		return nil
	}
	return goCallEdgesFromFile(fset, file, filePath)
}

func goCallEdgesFromFile(fset *token.FileSet, file *ast.File, filePath string) []graph.DependencyEdge {
	// imports: package alias/name -> true, so receivers that are really package
	// qualifiers (time.Now()) are NOT treated as typed variables. We index by
	// the last path segment AND any explicit alias.
	pkgNames := goImportNames(file)

	// structFields: type name -> (field name -> base field type). Populated from
	// top-level `type T struct{ f F }` declarations; used for best-effort
	// `x.f.M()`-style field-access receiver inference.
	structFields := goStructFieldTypes(file)

	var edges []graph.DependencyEdge
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		caller := fn.Name.Name

		// Build the per-function var -> base-type-name table once, then resolve
		// each call's receiver against it.
		varTypes := goVarTypeTable(fn, pkgNames)

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, qualified := goCallName(call.Fun)
			if callee == "" || callee == caller {
				return true
			}
			recvType := goCallReceiverType(call.Fun, varTypes, pkgNames, structFields)
			line := fset.Position(call.Lparen).Line
			meta := graph.JSONBMap{
				"source":         "go_ast",
				"qualified_ref":  qualified,
				"analysis_level": "ast_call_expression",
			}
			// Always emit the key so the contract is explicit; "" means
			// unknown or a package call (resolveTargets ignores empty).
			meta["recv_type"] = recvType
			edges = append(edges, graph.DependencyEdge{
				ID:         newUUID(),
				FromFile:   filePath,
				FromSymbol: caller,
				ToRef:      callee,
				Kind:       graph.EdgeCalls,
				Language:   "go",
				Line:       line,
				Metadata:   meta,
			})
			return true
		})
	}
	return dedupeEdges(edges)
}

// goReceiverType returns the base receiver type name for a method's receiver
// field list — stripping a leading pointer and any generic type-parameter
// brackets. func (app *TodoApp) -> "TodoApp"; func (s Store[T]) -> "Store".
func goReceiverType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return goBaseTypeName(recv.List[0].Type)
}

// goCallReceiverType infers the base type of the receiver in a method call
// expression x.M(...). It returns "" when the callee is unqualified (foo()),
// when the qualifier is an imported package (time.Now()), or when the receiver
// type cannot be inferred.
func goCallReceiverType(fun ast.Expr, varTypes map[string]string, pkgNames map[string]bool, structFields map[string]string) string {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	switch recv := sel.X.(type) {
	case *ast.Ident:
		// Package qualifier (time.Now()) -> not a typed receiver.
		if pkgNames[recv.Name] {
			return ""
		}
		return varTypes[recv.Name]
	case *ast.SelectorExpr:
		// Best-effort field access: x.f.M() where x is a known struct var and f
		// is a declared field with a known type. x.f.M().
		if base, ok := recv.X.(*ast.Ident); ok {
			if pkgNames[base.Name] {
				return ""
			}
			if owner := varTypes[base.Name]; owner != "" {
				return fieldTypeLookup(structFields, owner, recv.Sel.Name)
			}
		}
		return ""
	default:
		return ""
	}
}

// fieldTypeLookup resolves struct.field -> field base type via the precomputed
// "Type\x00field" index built by goStructFieldTypes.
func fieldTypeLookup(structFields map[string]string, typeName, field string) string {
	return structFields[typeName+"\x00"+field]
}

// goImportNames returns the set of in-scope package identifiers for a file:
// each import's alias when present, otherwise the last path segment. These are
// the names that, when used as a selector base (name.Foo()), denote a PACKAGE
// rather than a typed variable.
func goImportNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)
	for _, imp := range file.Imports {
		if imp.Name != nil {
			// Aliased import; skip blank/dot which don't introduce a qualifier.
			if imp.Name.Name != "_" && imp.Name.Name != "." {
				names[imp.Name.Name] = true
			}
			continue
		}
		p := strings.Trim(imp.Path.Value, `"`)
		if seg := p[strings.LastIndexByte(p, '/')+1:]; seg != "" {
			names[seg] = true
		}
	}
	return names
}

// goStructFieldTypes indexes top-level struct field types as "Type\x00field" ->
// base field type, for best-effort field-access receiver inference. Only named
// (non-embedded, single-name) fields with a resolvable base type are recorded.
func goStructFieldTypes(file *ast.File) map[string]string {
	out := make(map[string]string)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				continue
			}
			for _, field := range st.Fields.List {
				ft := goBaseTypeName(field.Type)
				if ft == "" {
					continue
				}
				for _, fname := range field.Names {
					out[ts.Name.Name+"\x00"+fname.Name] = ft
				}
			}
		}
	}
	return out
}

// goVarTypeTable builds a best-effort var-name -> base-type-name table for a
// single function, with NO go/types. Priority order (later entries refine
// earlier ones as they come into scope):
//  1. method receiver (func (app *TodoApp) -> app:TodoApp)
//  2. function params and results (x T / x *T)
//  3. local `var x T` declarations
//  4. short decls `x := EXPR` where EXPR is a constructor call NewT()/pkg.NewT()
//     or a composite literal T{} / &T{} / pkg.T{}
//
// Package-qualifier names are skipped so package calls never look like typed
// receivers.
func goVarTypeTable(fn *ast.FuncDecl, pkgNames map[string]bool) map[string]string {
	table := make(map[string]string)
	record := func(name, typ string) {
		if name == "" || name == "_" || typ == "" {
			return
		}
		if pkgNames[name] {
			return
		}
		table[name] = typ
	}

	// (1) receiver
	if fn.Recv != nil {
		for _, f := range fn.Recv.List {
			typ := goBaseTypeName(f.Type)
			for _, n := range f.Names {
				record(n.Name, typ)
			}
		}
	}
	// (2) params + results
	recordFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, f := range fields.List {
			typ := goBaseTypeName(f.Type)
			for _, n := range f.Names {
				record(n.Name, typ)
			}
		}
	}
	if fn.Type != nil {
		recordFields(fn.Type.Params)
		recordFields(fn.Type.Results)
	}

	// (3)+(4) walk the body for var decls and short decls.
	if fn.Body == nil {
		return table
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.DeclStmt:
			gen, ok := stmt.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				return true
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				// var x T  (explicit type wins)
				if vs.Type != nil {
					typ := goBaseTypeName(vs.Type)
					for _, n := range vs.Names {
						record(n.Name, typ)
					}
					continue
				}
				// var x = EXPR  (infer from initializer)
				for i, n := range vs.Names {
					if i < len(vs.Values) {
						record(n.Name, goExprType(vs.Values[i], pkgNames))
					}
				}
			}
		case *ast.AssignStmt:
			// x := EXPR  (and x, y := ...). Only := introduces new vars; we
			// also accept = since it doesn't hurt the best-effort table.
			if stmt.Tok != token.DEFINE && stmt.Tok != token.ASSIGN {
				return true
			}
			for i, lhs := range stmt.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || i >= len(stmt.Rhs) {
					continue
				}
				record(ident.Name, goExprType(stmt.Rhs[i], pkgNames))
			}
		}
		return true
	})
	return table
}

// goExprType infers the base type name produced by an initializer expression,
// covering the cases the var table cares about:
//   - composite literal      T{...} / pkg.T{...}        -> T
//   - address-of composite  &T{...} / &pkg.T{...}       -> T
//   - constructor call       NewT() / pkg.NewT()        -> T (strip "New")
//   - address-of constructor &NewT()                    -> T
//
// Returns "" for anything else (function results we can't name, builtins, etc.).
func goExprType(expr ast.Expr, pkgNames map[string]bool) string {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return goExprType(e.X, pkgNames)
		}
	case *ast.CompositeLit:
		return goBaseTypeName(e.Type)
	case *ast.CallExpr:
		name, _ := goCallName(e.Fun)
		if name == "" {
			return ""
		}
		// Constructor convention: NewFoo() / pkg.NewFoo() -> Foo.
		if strings.HasPrefix(name, "New") && len(name) > 3 {
			return name[3:]
		}
	}
	return ""
}

// goBaseTypeName reduces a type expression to its bare type name, stripping
// pointers, package qualifiers, and generic instantiation brackets.
//
//	*TodoApp        -> TodoApp
//	pkg.Thing       -> Thing
//	Store[T]        -> Store
//	*pkg.Store[T]   -> Store
//
// Returns "" for unnameable types (maps, slices, channels, funcs, ...).
func goBaseTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return goBaseTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		// pkg.Type -> Type
		return t.Sel.Name
	case *ast.IndexExpr:
		// Generic instantiation Type[T] -> Type
		return goBaseTypeName(t.X)
	case *ast.IndexListExpr:
		// Generic instantiation Type[T, U] -> Type
		return goBaseTypeName(t.X)
	case *ast.ParenExpr:
		return goBaseTypeName(t.X)
	default:
		return ""
	}
}

// goCallName resolves a callee identifier and its qualified form (pkg.Sel).
func goCallName(expr ast.Expr) (string, string) {
	switch call := expr.(type) {
	case *ast.Ident:
		return call.Name, call.Name
	case *ast.SelectorExpr:
		qualified := call.Sel.Name
		if left, ok := call.X.(*ast.Ident); ok && left.Name != "" {
			qualified = left.Name + "." + call.Sel.Name
		}
		return call.Sel.Name, qualified
	default:
		return "", ""
	}
}
