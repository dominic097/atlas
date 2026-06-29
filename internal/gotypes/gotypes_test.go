package gotypes

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// writeModule lays down a tiny on-disk Go module under dir and returns dir.
// Each entry in files maps a repo-relative path to its source content.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// TestAnalyzeFieldChainAndTypeUse is the keystone: it proves the two residuals
// the AST heuristic cannot close.
//
//	(1) a FIELD-CHAIN method call  t.c.Do()  resolves the receiver to "C" — the
//	    heuristic var-table only knows t:T and cannot follow t.c into type C's
//	    method without full type info.
//	(2) a TYPE-USE  func f(x Foo)  yields a RefEdge to "Foo" — the AST parser
//	    emits only call edges, never type-use references.
func TestAnalyzeFieldChainAndTypeUse(t *testing.T) {
	src := `package app

// C is a type with a method Do.
type C struct{}

func (c C) Do() {}

// T embeds C as a named field c, so t.c.Do() is a field-chain method call.
type T struct {
	c C
}

// Foo is a type used purely as a parameter type below (a type-use reference).
type Foo struct{}

// run exercises the field-chain receiver: t.c.Do(). The heuristic cannot
// resolve t.c -> C; go/types can.
func run() {
	var t T
	t.c.Do()
}

// f references Foo as a parameter type — a real reference, not a call.
func f(x Foo) {
	_ = x
}
`
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.21\n",
		"app.go": src,
	})

	res := Analyze(context.Background(), dir, 1)
	if !res.OK {
		t.Fatalf("Analyze returned OK=false; expected a successful type-check of the fixture module")
	}

	// (1) Field-chain receiver: t.c.Do() must resolve to receiver base type "C".
	var foundDo bool
	for _, cr := range res.CallRecvs {
		if cr.Callee == "Do" {
			foundDo = true
			if cr.Type != "C" {
				t.Errorf("Do() CallRecv.Type = %q, want %q (field-chain t.c.Do)", cr.Type, "C")
			}
			if cr.File != "app.go" {
				t.Errorf("Do() CallRecv.File = %q, want %q (repo-relative, forward-slash)", cr.File, "app.go")
			}
		}
	}
	if !foundDo {
		t.Errorf("no CallRecv recorded for the field-chain call Do(); got %+v", res.CallRecvs)
	}

	// (2) Type-use reference: func f(x Foo) must yield a RefEdge to Foo.
	var foundFooRef bool
	for _, r := range res.RefEdges {
		if r.ToRef == "Foo" {
			foundFooRef = true
			if r.FromSymbol != "f" {
				t.Errorf("Foo RefEdge.FromSymbol = %q, want %q (enclosing func)", r.FromSymbol, "f")
			}
			if r.Qualified != "example.com/app.Foo" {
				t.Errorf("Foo RefEdge.Qualified = %q, want %q", r.Qualified, "example.com/app.Foo")
			}
			if r.FromFile != "app.go" {
				t.Errorf("Foo RefEdge.FromFile = %q, want %q", r.FromFile, "app.go")
			}
		}
	}
	if !foundFooRef {
		t.Errorf("no RefEdge to type-use Foo; got %+v", res.RefEdges)
	}
}

// TestAnalyzeOversizedRepoDeclines proves the honest fallback: a repo above the
// file ceiling returns OK=false WITHOUT attempting a load, so the caller keeps
// the heuristic (no regression).
func TestAnalyzeOversizedRepoDeclines(t *testing.T) {
	res := Analyze(context.Background(), t.TempDir(), maxGoFiles+1)
	if res.OK {
		t.Fatalf("Analyze should decline (OK=false) for goFileCount=%d > %d", maxGoFiles+1, maxGoFiles)
	}
	if res.CallRecvs != nil || res.RefEdges != nil {
		t.Errorf("declined Analyze should return nil slices, got CallRecvs=%v RefEdges=%v", res.CallRecvs, res.RefEdges)
	}
}

// TestAnalyzeNonModuleDirIsHonest proves a directory that is not a buildable Go
// module returns OK=false rather than panicking or fabricating edges.
func TestAnalyzeNonModuleDirIsHonest(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"notes.txt": "not go source",
	})
	res := Analyze(context.Background(), dir, 1)
	if res.OK {
		t.Fatalf("Analyze should return OK=false for a non-module directory")
	}
}

// TestLoadModePreservesPrecisionBits guards the explicit minimal loadMode against
// a future regression that would either DROP a precision-bearing NeedX bit (which
// would silently kill recv_type / RefEdge output) or ADD NeedDeps (which would
// re-type-check the entire transitive world and blow up index time). Both are
// caught here without a full packages.Load, so the test is hermetic and fast.
func TestLoadModePreservesPrecisionBits(t *testing.T) {
	required := []struct {
		name string
		bit  packages.LoadMode
	}{
		{"NeedName", packages.NeedName},
		{"NeedFiles", packages.NeedFiles},
		{"NeedCompiledGoFiles", packages.NeedCompiledGoFiles},
		{"NeedImports", packages.NeedImports},
		{"NeedTypes", packages.NeedTypes},
		{"NeedTypesInfo", packages.NeedTypesInfo},
		{"NeedSyntax", packages.NeedSyntax},
		{"NeedModule", packages.NeedModule},
	}
	for _, r := range required {
		if loadMode&r.bit == 0 {
			t.Errorf("loadMode is missing %s; recv_type/RefEdge precision depends on it", r.name)
		}
	}
	// NeedDeps must stay OFF: it re-type-checks dependencies from source instead of
	// reading their export data, which is the single biggest index-time blowup.
	if loadMode&packages.NeedDeps != 0 {
		t.Errorf("loadMode must NOT set NeedDeps (it re-type-checks the whole transitive world)")
	}
}

// TestAnalyzeReceiverChainAndRefEdgeUnderMinimalMode is a second, distinct fixture
// (pointer receiver via a func-result chain + a field-type reference) that the
// minimal loadMode must still resolve precisely. It complements the keystone test:
// if a future Mode change quietly drops type info, both recv_type and the RefEdge
// here go missing and this fails. It directly exercises the configured loadMode
// through Analyze (no special wiring), so it is the regression net the task asks
// for.
func TestAnalyzeReceiverChainAndRefEdgeUnderMinimalMode(t *testing.T) {
	src := `package app

// Client has a method Send. mk() returns a *Client so mk().Send() is a
// func-result, pointer-receiver method call the AST heuristic cannot resolve.
type Client struct{}

func (c *Client) Send() {}

func mk() *Client { return &Client{} }

// Conn is referenced as a STRUCT FIELD type below — a type-use reference, not a
// call, attributed to the enclosing type decl Holder.
type Conn struct{}

type Holder struct {
	conn Conn
}

func use() {
	mk().Send()
}
`
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.21\n",
		"app.go": src,
	})

	res := Analyze(context.Background(), dir, 1)
	if !res.OK {
		t.Fatalf("Analyze returned OK=false; the minimal loadMode must still type-check the fixture")
	}

	// Func-result pointer-receiver chain: mk().Send() must resolve to base type "Client".
	var foundSend bool
	for _, cr := range res.CallRecvs {
		if cr.Callee == "Send" {
			foundSend = true
			if cr.Type != "Client" {
				t.Errorf("Send() CallRecv.Type = %q, want %q (mk() returns *Client)", cr.Type, "Client")
			}
		}
	}
	if !foundSend {
		t.Errorf("no CallRecv for mk().Send(); the loadMode dropped receiver precision; got %+v", res.CallRecvs)
	}

	// Field-type reference: Holder.conn Conn must yield a RefEdge to Conn.
	var foundConnRef bool
	for _, r := range res.RefEdges {
		if r.ToRef == "Conn" {
			foundConnRef = true
			if r.Qualified != "example.com/app.Conn" {
				t.Errorf("Conn RefEdge.Qualified = %q, want %q", r.Qualified, "example.com/app.Conn")
			}
		}
	}
	if !foundConnRef {
		t.Errorf("no RefEdge to field-type Conn; the loadMode dropped Uses info; got %+v", res.RefEdges)
	}
}

// multiPkgModule is a 3-package fixture: core defines a type+method; app and lib
// both import core (so both are reverse-deps of core); solo imports nothing in the
// module (so it is NOT a reverse-dep of core). It exercises the scoped analyzer's
// reverse-dep expansion and the carry-forward boundary.
func multiPkgModule(t *testing.T) string {
	t.Helper()
	return writeModule(t, map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.21\n",
		"core/core.go": `package core

type Widget struct{ n int }

func (w Widget) Do() int { return w.n }

func New() Widget { return Widget{} }
`,
		"app/app.go": `package app

import "example.com/m/core"

// Run uses core.Widget as a type-use reference AND calls its method.
func Run() int {
	w := core.New()
	var ref core.Widget = w
	return ref.Do()
}
`,
		"lib/lib.go": `package lib

import "example.com/m/core"

type Holder struct {
	w core.Widget
}

func (h Holder) Value() int { return h.w.Do() }
`,
		"solo/solo.go": `package solo

type Local struct{}

func (l Local) Ping() {}

func Use() { var x Local; x.Ping() }
`,
	})
}

// callRecvKey / refEdgeKey give stable, comparable string keys for an analyzer
// output so two runs (whole-module vs scoped) can be compared exactly.
func callRecvKey(c CallRecv) string {
	return c.File + "|" + itoa(c.Line) + "|" + c.Callee + "|" + c.Type
}

func refEdgeKey(r RefEdge) string {
	return r.FromFile + "|" + itoa(r.Line) + "|" + r.FromSymbol + "|" + r.ToRef + "|" + r.Qualified
}

// filterCallRecvs / filterRefEdges keep only the rows whose file is in the given
// set — the slice of a whole-module result that a scoped result is responsible for.
func filterCallRecvs(in []CallRecv, files map[string]struct{}) []string {
	var out []string
	for _, c := range in {
		if _, ok := files[c.File]; ok {
			out = append(out, callRecvKey(c))
		}
	}
	return out
}

func filterRefEdges(in []RefEdge, files map[string]struct{}) []string {
	var out []string
	for _, r := range in {
		if _, ok := files[r.FromFile]; ok {
			out = append(out, refEdgeKey(r))
		}
	}
	return out
}

func sortedKeys(c []CallRecv) []string {
	out := make([]string, 0, len(c))
	for _, x := range c {
		out = append(out, callRecvKey(x))
	}
	sort.Strings(out)
	return out
}

func sortedRefKeys(r []RefEdge) []string {
	out := make([]string, 0, len(r))
	for _, x := range r {
		out = append(out, refEdgeKey(x))
	}
	sort.Strings(out)
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAnalyzeScopedMatchesWholeModuleForAffectedFiles is the PRECISION keystone for
// the incremental path. Editing core/core.go must, on the scoped path, recompute the
// go/types output for core AND its reverse-deps (app, lib) and produce output that
// is BYTE-IDENTICAL (per affected file) to the whole-module Analyze. It also proves
// the carry-forward boundary: solo (not a reverse-dep) is NOT analyzed, so its edges
// would be carried forward, never recomputed.
func TestAnalyzeScopedMatchesWholeModuleForAffectedFiles(t *testing.T) {
	dir := multiPkgModule(t)

	full := Analyze(context.Background(), dir, 4)
	if !full.OK {
		t.Skip("whole-module Analyze produced no type info in this environment")
	}

	// Edit scope = core/core.go (the changed file).
	changed := map[string]struct{}{"core/core.go": {}}
	scoped := AnalyzeScoped(context.Background(), dir, 4, changed)
	if !scoped.OK {
		t.Fatalf("AnalyzeScoped returned OK=false; expected a scoped type-check")
	}

	// The analyzed set must be exactly core + its reverse-deps (app, lib), and must
	// NOT include solo (which does not import core).
	wantFiles := []string{"core/core.go", "app/app.go", "lib/lib.go"}
	for _, f := range wantFiles {
		if _, ok := scoped.AnalyzedFiles[f]; !ok {
			t.Fatalf("scoped AnalyzedFiles missing %q; reverse-dep expansion failed (got %v)", f, keysOf(scoped.AnalyzedFiles))
		}
	}
	if _, ok := scoped.AnalyzedFiles["solo/solo.go"]; ok {
		t.Fatalf("scoped AnalyzedFiles wrongly includes solo/solo.go (not a reverse-dep of core); got %v", keysOf(scoped.AnalyzedFiles))
	}

	// For every file the scoped run is authoritative for, its CallRecvs and RefEdges
	// must equal the whole-module run's rows for those same files — byte-identical.
	fullCallForFiles := filterCallRecvs(full.CallRecvs, scoped.AnalyzedFiles)
	scopedCallForFiles := filterCallRecvs(scoped.CallRecvs, scoped.AnalyzedFiles)
	if !equalStringSlices(fullCallForFiles, scopedCallForFiles) {
		t.Fatalf("scoped CallRecvs differ from whole-module for affected files:\n full=%v\n scoped=%v", fullCallForFiles, scopedCallForFiles)
	}

	fullRefForFiles := filterRefEdges(full.RefEdges, scoped.AnalyzedFiles)
	scopedRefForFiles := filterRefEdges(scoped.RefEdges, scoped.AnalyzedFiles)
	if !equalStringSlices(fullRefForFiles, scopedRefForFiles) {
		t.Fatalf("scoped RefEdges differ from whole-module for affected files:\n full=%v\n scoped=%v", fullRefForFiles, scopedRefForFiles)
	}

	// And the scoped run must NOT emit any row outside its analyzed set (it would
	// otherwise risk overwriting a carried-forward untouched file's edges).
	for _, c := range scoped.CallRecvs {
		if _, ok := scoped.AnalyzedFiles[c.File]; !ok {
			t.Fatalf("scoped CallRecv outside analyzed set: %+v", c)
		}
	}
	for _, r := range scoped.RefEdges {
		if _, ok := scoped.AnalyzedFiles[r.FromFile]; !ok {
			t.Fatalf("scoped RefEdge outside analyzed set: %+v", r)
		}
	}
}

// TestAnalyzeScopedDeterministic asserts repeated scoped runs over the same tree
// yield identical (sorted) output — no nondeterminism from the package walk / maps.
func TestAnalyzeScopedDeterministic(t *testing.T) {
	dir := multiPkgModule(t)
	changed := map[string]struct{}{"core/core.go": {}}

	first := AnalyzeScoped(context.Background(), dir, 4, changed)
	if !first.OK {
		t.Skip("scoped Analyze produced no type info in this environment")
	}
	for i := 0; i < 3; i++ {
		next := AnalyzeScoped(context.Background(), dir, 4, changed)
		if !next.OK {
			t.Fatalf("run %d returned OK=false", i)
		}
		if !equalStringSlices(sortedKeys(first.CallRecvs), sortedKeys(next.CallRecvs)) {
			t.Fatalf("run %d CallRecvs nondeterministic", i)
		}
		if !equalStringSlices(sortedRefKeys(first.RefEdges), sortedRefKeys(next.RefEdges)) {
			t.Fatalf("run %d RefEdges nondeterministic", i)
		}
	}
}

// TestAnalyzeScopedDeclinesForUnknownFile proves the honest fallback: a changed
// file that no loaded package claims yields OK=false (caller takes the safe path).
func TestAnalyzeScopedDeclinesForUnknownFile(t *testing.T) {
	dir := multiPkgModule(t)
	res := AnalyzeScoped(context.Background(), dir, 4, map[string]struct{}{"nope/ghost.go": {}})
	if res.OK {
		t.Fatalf("AnalyzeScoped should decline for a file no package owns; got OK=true")
	}
}

func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestBaseTypeNameReductions covers the name-reduction rules directly (pointer
// deref, generic stripping, package-qualifier stripping) without a full load.
func TestPlainTypeName(t *testing.T) {
	cases := map[string]string{
		"Store":          "Store",
		"Store[T]":       "Store",
		"pkg.Client":     "Client",
		"pkg.Store[int]": "Store",
		"":               "",
	}
	for in, want := range cases {
		if got := plainTypeName(in); got != want {
			t.Errorf("plainTypeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAnalyzeDeterministicAcrossRuns is the determinism keystone the global-cap bug
// violated: indexing the SAME multi-file module TWICE must yield byte-identical
// reference-edge and recv_source call sets, with NO dependence on the order packages
// or files happen to be walked (info.Uses is a Go map, so its iteration order is
// randomized per run). The previous GLOBAL maxRefEdges cap truncated by global
// emission order, so two runs that hit the ceiling on different files disagreed. With
// a per-FROM-FILE cap each file's set depends only on that file, so the analyzer is
// stable run-to-run. Multiple packages + multiple files per package make any
// global-order coupling observable.
func TestAnalyzeDeterministicAcrossRuns(t *testing.T) {
	dir := multiPkgModule(t)

	first := Analyze(context.Background(), dir, 4)
	if !first.OK {
		t.Skip("whole-module Analyze produced no type info in this environment")
	}
	wantRefs := sortedRefKeys(first.RefEdges)
	wantCalls := sortedKeys(first.CallRecvs)

	// Re-run several times: a single re-run can coincidentally match by luck, but a
	// map-order-dependent output would diverge across a handful of runs.
	for i := 0; i < 5; i++ {
		next := Analyze(context.Background(), dir, 4)
		if !next.OK {
			t.Fatalf("run %d returned OK=false", i)
		}
		if !equalStringSlices(wantRefs, sortedRefKeys(next.RefEdges)) {
			t.Fatalf("run %d RefEdges differ from run 0 — analyzer output is not deterministic\n first=%v\n run%d=%v",
				i, wantRefs, i, sortedRefKeys(next.RefEdges))
		}
		if !equalStringSlices(wantCalls, sortedKeys(next.CallRecvs)) {
			t.Fatalf("run %d CallRecvs differ from run 0 — analyzer output is not deterministic", i)
		}
	}
}

// bigRefFileModule lays down a module with ONE file whose package-level var block
// references a single in-module type far MORE than maxRefEdgesPerFile times, plus a
// second small file under the cap. It exercises the per-file cap: the big file must be
// truncated to EXACTLY maxRefEdgesPerFile in a stable prefix, while the small file
// keeps its full (uncapped) ref set — and neither file's selection perturbs the other.
func bigRefFileModule(t *testing.T, refsInBigFile int) string {
	t.Helper()

	var b strings.Builder
	b.WriteString("package big\n\n")
	b.WriteString("type T struct{ n int }\n\n")
	b.WriteString("var (\n")
	// Each line `vN T` is a distinct type-use reference to T on its own line. With
	// refsInBigFile > maxRefEdgesPerFile this overflows the per-file cap.
	for i := 0; i < refsInBigFile; i++ {
		b.WriteString("\tv")
		b.WriteString(itoa(i))
		b.WriteString(" T\n")
	}
	b.WriteString(")\n")

	return writeModule(t, map[string]string{
		"go.mod":         "module example.com/big\n\ngo 1.21\n",
		"big/big.go":     b.String(),
		"small/small.go": "package small\n\ntype S struct{}\n\nfunc use(x S) { _ = x }\n",
	})
}

// TestAnalyzePerFileRefCapIsBoundedAndDeterministic proves the per-file cap: a single
// file with FAR more than maxRefEdgesPerFile type-use references is truncated to
// exactly maxRefEdgesPerFile (never the global 8000-style cap), the truncation is a
// STABLE prefix across re-runs (deterministic, not map-order), and a SMALL file in the
// same module keeps its full ref set — proving the cap is genuinely per-file and one
// file's bound does not steal from another's.
func TestAnalyzePerFileRefCapIsBoundedAndDeterministic(t *testing.T) {
	dir := bigRefFileModule(t, maxRefEdgesPerFile+200)

	first := Analyze(context.Background(), dir, 2)
	if !first.OK {
		t.Skip("Analyze produced no type info in this environment")
	}

	countFor := func(res Result, file string) int {
		n := 0
		for _, r := range res.RefEdges {
			if r.FromFile == file && r.ToRef == "T" {
				n++
			}
		}
		return n
	}

	bigN := countFor(first, "big/big.go")
	if bigN != maxRefEdgesPerFile {
		t.Fatalf("big/big.go ref count = %d, want exactly maxRefEdgesPerFile=%d (per-file cap not applied or wrong bound)", bigN, maxRefEdgesPerFile)
	}

	// The small file (well under the cap) must keep its full set — its single S use is
	// not stolen by the big file overflowing.
	var smallHasS bool
	for _, r := range first.RefEdges {
		if r.FromFile == "small/small.go" && r.ToRef == "S" {
			smallHasS = true
		}
	}
	if !smallHasS {
		t.Fatalf("small/small.go lost its (under-cap) ref to S — the big file's overflow leaked across files")
	}

	// Stable prefix: re-running yields the IDENTICAL truncated set for the big file, so
	// the cap depends only on the file's own deterministic order, not map iteration.
	wantBig := sortedRefKeys(filterRefEdgesToSlice(first.RefEdges, "big/big.go"))
	for i := 0; i < 4; i++ {
		next := Analyze(context.Background(), dir, 2)
		if !next.OK {
			t.Fatalf("run %d OK=false", i)
		}
		gotBig := sortedRefKeys(filterRefEdgesToSlice(next.RefEdges, "big/big.go"))
		if !equalStringSlices(wantBig, gotBig) {
			t.Fatalf("run %d: capped big-file ref set is not a stable prefix (per-file cap is map-order dependent)\n want=%v\n got=%v", i, wantBig, gotBig)
		}
	}
}

// filterRefEdgesToSlice returns the RefEdges whose FromFile equals file.
func filterRefEdgesToSlice(in []RefEdge, file string) []RefEdge {
	var out []RefEdge
	for _, r := range in {
		if r.FromFile == file {
			out = append(out, r)
		}
	}
	return out
}
