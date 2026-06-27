// Package gotypes is the precise, type-checked Go analyzer that closes two
// quality residuals the lexical/AST parser cannot reach without full type
// information:
//
//  1. recv_type — the AST parser infers a method call's receiver base type with
//     an intra-function heuristic (var tables, struct-field index). It misses
//     deep field chains (s.client.transport.Do()) and func-result receivers
//     (newClient().Do()). go/types resolves every selection's receiver exactly.
//  2. real reference edges — the AST parser emits only CALL edges. A type used as
//     a parameter / field / variable type (func f(x Foo)) is a genuine reference
//     to Foo that callers should see in `refs`, but is not a call. go/types' Uses
//     map exposes every identifier-to-object binding, so we can emit a
//     type-USE reference edge per such occurrence.
//
// The analyzer is BEST-EFFORT and NON-REGRESSING by construction: it only runs
// when packages.Load succeeds and returns type-checked packages; on any failure
// (load error, zero packages, panic, timeout, oversized repo) it returns
// Result{OK:false} and the caller keeps the heuristic edges untouched. It never
// removes or downgrades a heuristic value — it only refines recv_type where it
// has a precise answer and ADDS reference edges.
package gotypes

import (
	"context"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// maxGoFiles is the repo-size ceiling above which we decline to type-check.
// Type checking the whole world (the standard library + every dependency, which
// NeedDeps forces) is quadratic-ish in practice; on a giant monorepo like
// golang/go it would dominate index time and risk OOM. Above this we honestly
// fall back to the AST heuristic — correctness of the smaller graphs over a
// brittle attempt at the largest.
const maxGoFiles = 4000

// loadTimeout caps the whole packages.Load + analysis window. A pathological
// module (cgo, build tags, generated code) can make the type checker spin; we
// bound it so indexing always terminates and degrades to the heuristic.
const loadTimeout = 90 * time.Second

// maxRefEdges caps the number of type-use reference edges emitted, so a huge
// module cannot balloon the snapshot. Reached only on very large in-bound repos
// (we already cap total files at maxGoFiles).
const maxRefEdges = 8000

// CallRecv is a precise receiver base-type binding for a single method-call
// site. File is relative to the repo root with forward slashes; Line/Callee
// match the AST parser's call-edge Line and ToRef so the index can join them.
type CallRecv struct {
	File   string // repo-relative, forward-slash
	Line   int    // line of the selector's method identifier
	Callee string // method name (sel.Sel.Name)
	Type   string // receiver base named type, deref'd + de-generified + unqualified
}

// RefEdge is a single type-use reference: the enclosing declaration FromSymbol
// (in FromFile) references the named type ToRef at Line. Qualified carries the
// fully package-qualified type name for display/audit.
type RefEdge struct {
	FromSymbol string
	FromFile   string // repo-relative, forward-slash
	ToRef      string // base type name (matches an indexed `type` symbol's Name)
	Line       int
	Qualified  string // pkgpath.TypeName
}

// Result is the analyzer output. OK is false whenever the caller must keep the
// heuristic (load failed / declined / panicked); CallRecvs/RefEdges are then nil.
type Result struct {
	CallRecvs []CallRecv
	RefEdges  []RefEdge
	OK        bool
}

// Analyze type-checks every package under repoRoot and returns precise receiver
// types and type-use reference edges. It is the honest, non-regressing path:
//
//   - goFileCount > maxGoFiles            -> Result{OK:false} (decline; too big)
//   - packages.Load error / zero packages -> Result{OK:false}
//   - panic anywhere                       -> recovered -> Result{OK:false}
//   - context cancel / 90s timeout         -> partial or Result{OK:false}
//
// On success it walks each package's syntax with its TypesInfo to record
// CallRecvs (method selections) and RefEdges (TypeName uses defined in this
// module). Output is deterministic (sorted, deduped).
func Analyze(ctx context.Context, repoRoot string, goFileCount int) (result Result) {
	// Guard 1: oversized repos degrade to the heuristic, honestly.
	if goFileCount > maxGoFiles {
		return Result{OK: false}
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return Result{OK: false}
	}

	// Guard 2: never let a type-checker panic sink the whole index.
	defer func() {
		if r := recover(); r != nil {
			result = Result{OK: false}
		}
	}()

	loadCtx, cancel := context.WithTimeout(ctx, loadTimeout)
	defer cancel()

	cfg := &packages.Config{
		Mode:    packages.LoadSyntax | packages.NeedModule,
		Dir:     repoRoot,
		Context: loadCtx,
		Tests:   false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil || len(pkgs) == 0 {
		return Result{OK: false}
	}
	if loadCtx.Err() != nil {
		return Result{OK: false}
	}

	var (
		callRecvs []CallRecv
		refEdges  []RefEdge
		refSeen   = map[string]struct{}{}
		analyzed  bool
	)

	for _, pkg := range pkgs {
		if pkg == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
			continue
		}
		analyzed = true
		fset := pkg.Fset
		info := pkg.TypesInfo

		for _, syntax := range pkg.Syntax {
			collectCallRecvs(fset, info, syntax, repoRoot, &callRecvs)
			collectRefEdges(fset, info, syntax, repoRoot, pkg.Module, &refEdges, refSeen)
			if len(refEdges) >= maxRefEdges {
				break
			}
		}
		if len(refEdges) >= maxRefEdges {
			break
		}
	}

	// If we loaded packages but none carried usable type info, treat as a miss so
	// the heuristic stands rather than reporting an empty (false-confident) result.
	if !analyzed {
		return Result{OK: false}
	}

	sortCallRecvs(callRecvs)
	sortRefEdges(refEdges)

	return Result{CallRecvs: callRecvs, RefEdges: refEdges, OK: true}
}

// collectCallRecvs walks one file's syntax for method-call sites and records the
// precise receiver base type from the type-checker's Selections / Types maps.
// This resolves the field-chain (s.client.Do) and func-result (mk().Do) cases
// the intra-function heuristic cannot reach.
func collectCallRecvs(fset *token.FileSet, info *types.Info, file *ast.File, repoRoot string, out *[]CallRecv) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		recvType := ""
		// (a) A method selection: TypesInfo.Selections[sel] is set when sel.Sel is
		// a method/field selected through a value or pointer. Recv() is the type
		// the selection is rooted at — exactly the receiver base we want.
		if selection := info.Selections[sel]; selection != nil {
			if selection.Kind() == types.MethodVal || selection.Kind() == types.MethodExpr {
				recvType = baseTypeName(selection.Recv())
			}
		}
		// (b) Fallback: the selector base sel.X is a value of some type (covers
		// cases not recorded as a Selection, e.g. interface method values). Use the
		// static type of the base expression.
		if recvType == "" {
			if tv, ok := info.Types[sel.X]; ok && tv.Type != nil {
				// Only treat it as a receiver when sel.X is a VALUE (not a package
				// or a type name); types.TypeAndValue.IsValue covers that.
				if tv.IsValue() {
					recvType = baseTypeName(tv.Type)
				}
			}
		}
		if recvType == "" {
			return true
		}

		pos := fset.Position(sel.Sel.Pos())
		rel := relFile(repoRoot, pos.Filename)
		if rel == "" {
			return true
		}
		*out = append(*out, CallRecv{
			File:   rel,
			Line:   pos.Line,
			Callee: sel.Sel.Name,
			Type:   recvType,
		})
		return true
	})
}

// collectRefEdges walks one file for TYPE-USE references: every identifier whose
// resolved object is a *types.TypeName defined in THIS module. Each such use is a
// real reference to that type (parameter type, field type, var type, embedding,
// composite-literal type, conversion, …) that is NOT a call. The reference is
// attributed to the enclosing func/type declaration.
func collectRefEdges(fset *token.FileSet, info *types.Info, file *ast.File, repoRoot string, mod *packages.Module, out *[]RefEdge, seen map[string]struct{}) {
	// Pre-extract the top-level decls with their position ranges so we can attribute
	// each use to the enclosing func/type name without a parent-stack walk.
	decls := topLevelDecls(file)

	for ident, obj := range info.Uses {
		if len(*out) >= maxRefEdges {
			return
		}
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		// Skip the universe/builtin and unexported-to-us cases with no package.
		pkg := tn.Pkg()
		if pkg == nil {
			continue
		}
		// Restrict to types defined IN THIS MODULE — references to stdlib/3p types
		// (sql.DB) are not part of our graph and would never resolve to an indexed
		// symbol. When module info is unavailable we keep the type (best-effort).
		if !inModule(pkg, mod) {
			continue
		}

		pos := fset.Position(ident.Pos())
		if pos.Filename == "" {
			continue
		}
		rel := relFile(repoRoot, pos.Filename)
		if rel == "" {
			continue
		}
		base := plainTypeName(tn.Name())
		if base == "" {
			continue
		}
		from := enclosingDecl(decls, ident.Pos())
		qualified := pkg.Path() + "." + tn.Name()

		// Dedup identical (from, file, type, line) tuples.
		key := from + "\x00" + rel + "\x00" + base + "\x00" + qualified + "\x00" + itoa(pos.Line)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		*out = append(*out, RefEdge{
			FromSymbol: from,
			FromFile:   rel,
			ToRef:      base,
			Line:       pos.Line,
			Qualified:  qualified,
		})
	}
}

// declRange is a top-level declaration's name and source span.
type declRange struct {
	name       string
	start, end token.Pos
}

// topLevelDecls returns the func/type declarations of a file with their spans,
// so a use position can be attributed to its enclosing declaration name.
func topLevelDecls(file *ast.File) []declRange {
	var out []declRange
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			out = append(out, declRange{name: d.Name.Name, start: d.Pos(), end: d.End()})
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					out = append(out, declRange{name: ts.Name.Name, start: ts.Pos(), end: ts.End()})
				}
			}
		}
	}
	return out
}

// enclosingDecl returns the name of the top-level declaration whose span
// contains pos, or "" (package-level use, e.g. a var-block type) when none does.
func enclosingDecl(decls []declRange, pos token.Pos) string {
	for _, d := range decls {
		if pos >= d.start && pos < d.end {
			return d.name
		}
	}
	return ""
}

// baseTypeName reduces a types.Type to its bare named-type name: deref pointers,
// unwrap named/alias, strip the package path and generic type arguments.
//
//	*pkg.Store[T]  -> "Store"
//	pkg.Client     -> "Client"
//	[]Foo / map…   -> "" (unnameable)
func baseTypeName(t types.Type) string {
	if t == nil {
		return ""
	}
	switch x := t.(type) {
	case *types.Pointer:
		return baseTypeName(x.Elem())
	case *types.Named:
		obj := x.Obj()
		if obj == nil {
			return ""
		}
		return plainTypeName(obj.Name())
	case *types.Alias:
		// Go 1.22+ alias type node; reduce to its target's name.
		return plainTypeName(x.Obj().Name())
	default:
		return ""
	}
}

// plainTypeName strips any leftover package qualifier and generic instantiation
// brackets from a type name string. Defensive: Named.Obj().Name() is already
// bare, but selection receivers can surface "pkg.T[int]" shapes.
func plainTypeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSpace(name)
}

// inModule reports whether pkg belongs to the module under analysis. When module
// metadata is missing on either side we fall back to true (best-effort keep),
// since a wrong-keep degrades only to an unresolved reference edge, never a
// regression of the heuristic call edges.
func inModule(pkg *types.Package, mod *packages.Module) bool {
	if pkg == nil {
		return false
	}
	if mod == nil || mod.Path == "" {
		return true
	}
	path := pkg.Path()
	return path == mod.Path || strings.HasPrefix(path, mod.Path+"/")
}

// relFile makes an absolute file path relative to repoRoot with forward slashes,
// returning "" when it escapes the root (so out-of-tree dependency files — which
// NeedDeps pulls in — are skipped, not mis-attributed).
func relFile(repoRoot, abs string) string {
	if abs == "" {
		return ""
	}
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return rel
}

func sortCallRecvs(cr []CallRecv) {
	sort.SliceStable(cr, func(i, j int) bool {
		if cr[i].File != cr[j].File {
			return cr[i].File < cr[j].File
		}
		if cr[i].Line != cr[j].Line {
			return cr[i].Line < cr[j].Line
		}
		if cr[i].Callee != cr[j].Callee {
			return cr[i].Callee < cr[j].Callee
		}
		return cr[i].Type < cr[j].Type
	})
}

func sortRefEdges(re []RefEdge) {
	sort.SliceStable(re, func(i, j int) bool {
		if re[i].FromFile != re[j].FromFile {
			return re[i].FromFile < re[j].FromFile
		}
		if re[i].Line != re[j].Line {
			return re[i].Line < re[j].Line
		}
		if re[i].FromSymbol != re[j].FromSymbol {
			return re[i].FromSymbol < re[j].FromSymbol
		}
		return re[i].ToRef < re[j].ToRef
	})
}

// itoa is a tiny base-10 int formatter (avoids importing strconv for one use in
// the dedup key).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
