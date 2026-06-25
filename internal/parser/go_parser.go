package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// parseGoSymbols extracts functions, methods, and types from a Go source file
// using the native go/parser (compiler-grade fidelity), plus the import paths.
// Ported from pulse parseGoFile.
func parseGoSymbols(path string, content []byte) ([]symbolDraft, []string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, nil
	}

	imports := make([]string, 0, len(file.Imports))
	for _, imported := range file.Imports {
		imports = append(imports, strings.Trim(imported.Path.Value, `"`))
	}

	var symbols []symbolDraft
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		kind := "function"
		if fn.Recv != nil {
			kind = "method"
		}
		symbols = append(symbols, symbolDraft{
			name:      fn.Name.Name,
			kind:      kind,
			signature: goSignature(fset, fn, content),
			startLine: fset.Position(fn.Pos()).Line,
			endLine:   fset.Position(fn.End()).Line,
		})
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
		return false
	})

	return symbols, imports
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
func goCallEdges(filePath string, content []byte) []graph.DependencyEdge {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, 0)
	if err != nil {
		return nil
	}

	var edges []graph.DependencyEdge
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		caller := fn.Name.Name
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, qualified := goCallName(call.Fun)
			if callee == "" || callee == caller {
				return true
			}
			line := fset.Position(call.Lparen).Line
			edges = append(edges, graph.DependencyEdge{
				ID:         newUUID(),
				FromFile:   filePath,
				FromSymbol: caller,
				ToRef:      callee,
				Kind:       graph.EdgeCalls,
				Language:   "go",
				Line:       line,
				Metadata: graph.JSONBMap{
					"source":         "go_ast",
					"qualified_ref":  qualified,
					"analysis_level": "ast_call_expression",
				},
			})
			return true
		})
	}
	return dedupeEdges(edges)
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
