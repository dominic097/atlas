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

// ScopedMinGoFiles is the module-size floor below which the incremental delta path
// (AnalyzeScoped) is NOT worth taking: the scoped path pays a fixed metadata
// `go list ./...` cost to discover the package graph for reverse-dep expansion, and
// that discovery only amortizes when the whole-module type-check it replaces is much
// larger. MEASURED (warm, macOS, x/tools@v0.46.0 vs sirupsen/logrus):
//
//	module      go files  full ./... load   meta ./... list   scoped 1-pkg   scoped total
//	logrus           47       ~120ms            ~48ms            ~62ms          ~110ms   (break-even)
//	x/tools        1268       ~400ms           ~190ms            ~80ms          ~270ms   (~30% faster)
//
// Below this floor the index keeps the whole-module pass (no regression on the small
// repos that dominate real per-edit usage); at/above it the scoped path wins. The
// value is deliberately conservative — comfortably above small libraries, well below
// the large modules where the type-check cost dominates discovery.
//
// It is a var (not a const) ONLY so tests can lower it to exercise the scoped wiring
// on small fixtures; production never mutates it.
var ScopedMinGoFiles = 300

// loadMode is the explicit, minimal set of go/packages NeedX bits this analyzer
// actually consumes, replacing the deprecated LoadSyntax alias. Each bit is load-
// bearing for the two precision outputs (recv_type overrides + type-use RefEdges):
//
//	NeedName             pkg.Module.Path comparison (inModule) needs package paths
//	NeedFiles            establishes pkg.Fset / file positions
//	NeedCompiledGoFiles  the files actually type-checked (positions join AST edges)
//	NeedImports          import graph required before any package can type-check
//	NeedTypes            *types.Package — Selections/Types receiver resolution
//	NeedTypesInfo        pkg.TypesInfo (Selections, Types, Uses) — the whole engine
//	NeedSyntax           pkg.Syntax (the typed AST we walk)
//	NeedModule           pkg.Module — the in-module RefEdge filter (inModule)
//
// Deliberately ABSENT:
//   - NeedDeps: dependency type info is read from compiled export data, NOT
//     re-type-checked from source. Setting it would re-typecheck the whole
//     transitive world (the LoadAllSyntax behavior) and is the single biggest
//     way to blow up index time — we never want it.
//   - NeedTypesSizes: the loader derives type sizes from the `go list` response
//     whenever NeedTypes/NeedTypesInfo is set (packages.go: ld.sizes is populated
//     unconditionally from response.Compiler/Arch), so the explicit bit is
//     redundant for correct type-checking.
//
// NOTE (measured, do not re-litigate as a perf lever): swapping LoadSyntax for
// this minimal set was verified to produce byte-identical precision output
// (recv_source=go_types call edges + references edges) on logrus, and is NOT a
// cold-index speedup. The cold go/types cost is dominated by the Go toolchain
// compiling dependency export data — `go list -export ./...` alone is ~3.0s cold
// vs ~0.11s warm on logrus, i.e. essentially the entire cold go_types phase —
// which `packages.Load` must trigger for ANY mode that requests types. Mode
// choice moved cold load by less than its run-to-run variance. This is a
// correctness/clarity change (explicit non-deprecated intent), not a perf win.
const loadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedTypes |
	packages.NeedTypesInfo |
	packages.NeedSyntax |
	packages.NeedModule

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
	// AnalyzedFiles is the set of repo-relative, forward-slash source files the
	// analyzer actually type-checked and walked. On the whole-module path (Analyze)
	// this is every in-module file; on the scoped path (AnalyzeScoped) it is exactly
	// the files in the changed packages + their in-module reverse-deps. The caller
	// uses it to know which files' go/types edges this result is authoritative for —
	// edges for files NOT in this set must be carried forward untouched. Nil when OK
	// is false.
	AnalyzedFiles map[string]struct{}
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
		Mode:    loadMode,
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

	return analyzePackages(pkgs, repoRoot)
}

// AnalyzeScoped is the INCREMENTAL counterpart of Analyze: it type-checks only the
// packages that own changedFiles plus the in-module packages that (transitively)
// depend on them (reverse deps), reusing compiled export data for everything else.
// It returns precisely the same CallRecv / RefEdge shape Analyze produces, but only
// for the files in those analyzed packages — Result.AnalyzedFiles names exactly that
// set so the caller can refresh those files' go/types edges and carry forward every
// other file's edges untouched.
//
// PRECISION INVARIANT: for the affected files (the changed files and every file in
// a package that imports their package, directly or transitively), the CallRecvs and
// RefEdges are byte-identical to what the whole-module Analyze emits for the same
// files. This holds because:
//
//   - A call site's receiver base type and a file's type-use references are resolved
//     ENTIRELY from that file's own package (its syntax + the export data of its
//     imports). Loading only that package yields identical Selections/Uses for it.
//   - The ONLY way an edit to package P can change another package Q's go/types edges
//     is through Q importing P (a renamed/removed type in P alters Q's references or
//     a moved method alters Q's receiver). Every such Q is a reverse-dep of P and is
//     therefore included in the analyzed set, so its edges are recomputed too.
//   - Packages that do NOT depend on the changed packages cannot have their edges
//     changed by the edit, so carrying their base edges forward is exact.
//
// changedFiles are repo-relative, forward-slash paths (the index's canonical form).
// On any failure (oversized, load error, no usable type info, empty target set,
// panic, timeout) it returns Result{OK:false} and the caller keeps the whole-module
// path / heuristic — never a regression.
func AnalyzeScoped(ctx context.Context, repoRoot string, goFileCount int, changedFiles map[string]struct{}) (result Result) {
	if goFileCount > maxGoFiles {
		return Result{OK: false}
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" || len(changedFiles) == 0 {
		return Result{OK: false}
	}

	defer func() {
		if r := recover(); r != nil {
			result = Result{OK: false}
		}
	}()

	loadCtx, cancel := context.WithTimeout(ctx, loadTimeout)
	defer cancel()

	// Stage 1: cheap metadata-only load (no type-checking) to map files -> owning
	// package and build the import graph. This is the `go list ./...` cost only; it
	// does NOT compile any export data because no NeedTypes* bit is set.
	metaCfg := &packages.Config{
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedModule,
		Dir:     repoRoot,
		Context: loadCtx,
		Tests:   false,
	}
	metaPkgs, err := packages.Load(metaCfg, "./...")
	if err != nil || len(metaPkgs) == 0 || loadCtx.Err() != nil {
		return Result{OK: false}
	}

	// Identify the packages that own a changed file.
	changedPkgIDs := changedPackageIDs(metaPkgs, repoRoot, changedFiles)
	if len(changedPkgIDs) == 0 {
		// No loaded package claims a changed file (e.g. a brand-new file in a package
		// the metadata load hasn't picked up, or a file outside the module). Fall back
		// honestly so the whole-module path runs and precision is preserved.
		return Result{OK: false}
	}

	// Expand to the changed packages + every in-module package that transitively
	// imports one of them (reverse deps).
	targetIDs := expandReverseDeps(metaPkgs, changedPkgIDs)
	targetPatterns := patternsForPackages(metaPkgs, targetIDs)
	if len(targetPatterns) == 0 {
		return Result{OK: false}
	}

	// Stage 2: full type-check load of ONLY the target packages.
	typeCfg := &packages.Config{
		Mode:    loadMode,
		Dir:     repoRoot,
		Context: loadCtx,
		Tests:   false,
	}
	pkgs, err := packages.Load(typeCfg, targetPatterns...)
	if err != nil || len(pkgs) == 0 || loadCtx.Err() != nil {
		return Result{OK: false}
	}

	return analyzePackages(pkgs, repoRoot)
}

// analyzePackages walks the type-checked packages, collecting precise receiver
// types and type-use reference edges, and records the set of files it covered. It
// is shared by the whole-module (Analyze) and scoped (AnalyzeScoped) paths so both
// produce identical edge shapes — only the package SET they receive differs.
func analyzePackages(pkgs []*packages.Package, repoRoot string) Result {
	var (
		callRecvs []CallRecv
		refEdges  []RefEdge
		refSeen   = map[string]struct{}{}
		analyzed  bool
		files     = map[string]struct{}{}
	)

	for _, pkg := range pkgs {
		if pkg == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
			continue
		}
		analyzed = true
		fset := pkg.Fset
		info := pkg.TypesInfo

		for _, syntax := range pkg.Syntax {
			if rel := relFile(repoRoot, fset.Position(syntax.Pos()).Filename); rel != "" {
				files[rel] = struct{}{}
			}
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

	return Result{CallRecvs: callRecvs, RefEdges: refEdges, OK: true, AnalyzedFiles: files}
}

// changedPackageIDs returns the IDs of loaded packages that own at least one of
// changedFiles. A package owns a file when the file appears in its GoFiles (compared
// repo-relative, forward-slash, to match the index's canonical changedFiles paths).
func changedPackageIDs(pkgs []*packages.Package, repoRoot string, changedFiles map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}
		for _, abs := range pkg.GoFiles {
			rel := relFile(repoRoot, abs)
			if rel == "" {
				continue
			}
			if _, hit := changedFiles[rel]; hit {
				out[pkg.ID] = struct{}{}
				break
			}
		}
	}
	return out
}

// expandReverseDeps returns seed ∪ every package that transitively imports a seed
// package. It walks the forward import edges of the loaded set to build the reverse
// closure, so any package whose go/types edges could be affected by an edit to a
// seed package is included. Only packages present in the loaded (in-module) set are
// considered — stdlib/third-party importers are irrelevant to our graph.
func expandReverseDeps(pkgs []*packages.Package, seed map[string]struct{}) map[string]struct{} {
	byID := make(map[string]*packages.Package, len(pkgs))
	for _, p := range pkgs {
		if p != nil {
			byID[p.ID] = p
		}
	}
	// Build reverse-import adjacency: importer depends on imported.
	importers := map[string][]string{} // imported pkg ID -> []importer pkg ID
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		for impID := range p.Imports {
			if _, inSet := byID[impID]; inSet {
				importers[impID] = append(importers[impID], p.ID)
			}
		}
	}

	result := map[string]struct{}{}
	stack := make([]string, 0, len(seed))
	for id := range seed {
		result[id] = struct{}{}
		stack = append(stack, id)
	}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, importer := range importers[id] {
			if _, seen := result[importer]; seen {
				continue
			}
			result[importer] = struct{}{}
			stack = append(stack, importer)
		}
	}
	return result
}

// patternsForPackages maps a set of package IDs to the load patterns (their package
// paths) packages.Load expects. PkgPath is the importable path; loading by path is
// stable and avoids the ID/pattern ambiguity of file-list patterns.
func patternsForPackages(pkgs []*packages.Package, ids map[string]struct{}) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		if _, want := ids[p.ID]; !want {
			continue
		}
		path := p.PkgPath
		if path == "" {
			continue
		}
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
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
