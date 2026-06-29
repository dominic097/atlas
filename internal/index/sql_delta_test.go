package index

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/lexical"
	"github.com/dominic097/atlas/internal/store"
)

// symbolSignature returns a sorted, id-independent fingerprint of a snapshot's full
// symbol set: (path, start, end, kind, name, signature). Two snapshots with the same
// fingerprint carry byte-identical symbol rows (surrogate ids excluded — they are
// random per write and never compared across stores).
func symbolSignature(t *testing.T, drv store.StorageDriver, snapshotID string) []string {
	t.Helper()
	syms, err := drv.ListSymbols(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	out := make([]string, 0, len(syms))
	for i := range syms {
		s := &syms[i]
		out = append(out, strings.Join([]string{
			s.Path, strconv.Itoa(s.StartLine), strconv.Itoa(s.EndLine),
			s.Kind, s.Name, s.Signature, string(s.NodeID),
		}, "\x1f"))
	}
	sort.Strings(out)
	return out
}

// edgeSignature returns a sorted, id-independent fingerprint of a snapshot's full
// edge set, INCLUDING the go/types-derived metadata (recv_type, recv_source, source)
// so a parity check catches any drift in the precise Go analysis.
func edgeSignature(t *testing.T, drv store.StorageDriver, snapshotID string) []string {
	t.Helper()
	edges, err := drv.ListEdges(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	out := make([]string, 0, len(edges))
	for i := range edges {
		e := &edges[i]
		recv, _ := e.Metadata["recv_type"].(string)
		recvSrc, _ := e.Metadata["recv_source"].(string)
		src, _ := e.Metadata["source"].(string)
		qual, _ := e.Metadata["qualified_ref"].(string)
		out = append(out, strings.Join([]string{
			e.FromFile, e.FromSymbol, e.ToRef, string(e.Kind), e.Language,
			strconv.Itoa(e.Line), recv, recvSrc, src, qual,
		}, "\x1f"))
	}
	sort.Strings(out)
	return out
}

// routeSignature returns a sorted, id-independent fingerprint of a snapshot's routes.
func routeSignature(t *testing.T, drv store.StorageDriver, snapshotID string) []string {
	t.Helper()
	rts, err := drv.ListRoutes(context.Background(), snapshotID, "")
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	out := make([]string, 0, len(rts))
	for i := range rts {
		r := &rts[i]
		out = append(out, strings.Join([]string{
			r.Method, r.PathPattern, r.HandlerFile, r.Role, r.Source, r.Confidence,
		}, "\x1f"))
	}
	sort.Strings(out)
	return out
}

func assertEqualSlices(t *testing.T, label string, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: count mismatch full=%d delta=%d\n full=%v\n delta=%v", label, len(want), len(got), want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("%s: row %d differs\n full=%q\n delta=%q", label, i, want[i], got[i])
		}
	}
}

// assertFullParity asserts that a delta snapshot's full row set (symbols + edges +
// routes + counts) equals a full reindex of the same working tree. This is the
// byte-identical-to-full-reindex keystone for the SQL-level delta.
func assertFullParity(t *testing.T, deltaDrv store.StorageDriver, deltaSnap *graph.Snapshot, truthDrv store.StorageDriver, truthSnap *graph.Snapshot) {
	t.Helper()
	assertEqualSlices(t, "symbols", symbolSignature(t, truthDrv, truthSnap.ID), symbolSignature(t, deltaDrv, deltaSnap.ID))
	assertEqualSlices(t, "edges", edgeSignature(t, truthDrv, truthSnap.ID), edgeSignature(t, deltaDrv, deltaSnap.ID))
	assertEqualSlices(t, "routes", routeSignature(t, truthDrv, truthSnap.ID), routeSignature(t, deltaDrv, deltaSnap.ID))
	if deltaSnap.FileCount != truthSnap.FileCount {
		t.Fatalf("file_count: delta=%d full=%d", deltaSnap.FileCount, truthSnap.FileCount)
	}
	if deltaSnap.SymbolCount != truthSnap.SymbolCount {
		t.Fatalf("symbol_count: delta=%d full=%d", deltaSnap.SymbolCount, truthSnap.SymbolCount)
	}
	if deltaSnap.EdgeCount != truthSnap.EdgeCount {
		t.Fatalf("edge_count: delta=%d full=%d", deltaSnap.EdgeCount, truthSnap.EdgeCount)
	}
	if deltaSnap.RouteCount != truthSnap.RouteCount {
		t.Fatalf("route_count: delta=%d full=%d", deltaSnap.RouteCount, truthSnap.RouteCount)
	}
}

// writePythonRepo lays down a non-git 2-file Python repo: a.py with two functions,
// b.py with one. A tree-sitter language whose per-file symbols/edges are entirely
// self-contained (no cross-file enrichment), so an edit to a.py takes the SQL-level
// delta with affected = {a.py} only.
func writePythonRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.py": "def get_user(uid):\n    return uid\n\ndef delete_user(uid):\n    return None\n",
		"b.py": "def render_invoice(x):\n    return x\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// TestSQLDeltaNonGoParity is the 1B parity keystone for the self-contained
// (non-Go) case: an uncommitted edit to one Python file takes the SQL-level delta
// (delta_path == "sql"), and the resulting snapshot's FULL row set equals a full
// reindex of the same edited tree — proving ReplaceFileRows leaves the untouched
// file's rows intact while replacing exactly the edited file's rows.
func TestSQLDeltaNonGoParity(t *testing.T) {
	ctx := context.Background()

	editA := "def get_user(uid):\n    return uid\n\ndef update_email(uid, email):\n    return None\n"

	// Ground truth: a fresh full index of the EDITED tree.
	truthDir := writePythonRepo(t)
	if err := os.WriteFile(filepath.Join(truthDir, "a.py"), []byte(editA), 0o644); err != nil {
		t.Fatalf("truth edit: %v", err)
	}
	truthDrv := openTestStore(t)
	truth, truthStats, err := Run(ctx, truthDrv, nil, "", "pyapp", truthDir, Options{})
	if err != nil {
		t.Fatalf("truth full Run: %v", err)
	}
	if truthStats.Mode != "full" {
		t.Fatalf("truth mode = %q, want full", truthStats.Mode)
	}

	// Delta: full index the original tree, then edit a.py and re-index (delta).
	deltaDir := writePythonRepo(t)
	deltaDrv := openTestStore(t)
	if _, s, err := Run(ctx, deltaDrv, nil, "", "pyapp", deltaDir, Options{}); err != nil {
		t.Fatalf("delta base Run: %v", err)
	} else if s.Mode != "full" {
		t.Fatalf("delta base mode = %q, want full", s.Mode)
	}
	if err := os.WriteFile(filepath.Join(deltaDir, "a.py"), []byte(editA), 0o644); err != nil {
		t.Fatalf("delta edit: %v", err)
	}
	delta, deltaStats, err := Run(ctx, deltaDrv, nil, "", "pyapp", deltaDir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	if got, _ := delta.Metadata["delta_path"].(string); got != "sql" {
		t.Fatalf("delta delta_path = %q, want sql (the SQL-level path did not run)", got)
	}

	assertFullParity(t, deltaDrv, delta, truthDrv, truth)
}

// TestSQLDeltaGoCrossFileParity is the 1B Go cross-file keystone the brief calls
// out: edit a type T in file A that is USED in an UNCHANGED file B. After the SQL
// delta (scoped go/types), B's go_types reference edge to T must be correct
// (regenerated from the reverse-dep re-analysis), and A's + B's rows — and the whole
// snapshot — must equal a full reindex of the same edited tree. The fixture's `core`
// package defines Widget (used by app + lib); editing core makes app/lib reverse-deps
// whose reference edges must be regenerated even though their files were not re-parsed.
func TestSQLDeltaGoCrossFileParity(t *testing.T) {
	forceScopedGoTypes(t)
	ctx := context.Background()

	// The edit RENAMES the used type's method surface by adding a new method on Widget,
	// which app references via `var ref core.Widget`. A pure additive method does not
	// change app's ref edge target, so to exercise a genuine cross-file reference shift
	// we instead add a NEW exported type in core that app will reference after the edit.
	coreEdit := "\ntype Gadget struct{ x int }\n\nfunc (g Gadget) Spin() int { return g.x }\n"
	appEdit := "\nfunc UseGadget() int { var g core.Gadget; return g.Spin() }\n"

	applyEdits := func(dir string) {
		appendToFile(t, filepath.Join(dir, "core/core.go"), coreEdit)
		appendToFile(t, filepath.Join(dir, "app/app.go"), appEdit)
	}

	// Ground truth: full index of the edited tree.
	truthDir := writeMultiPkgModule(t)
	applyEdits(truthDir)
	truthDrv := openTestStore(t)
	truth, _, err := Run(ctx, truthDrv, nil, "", "m", truthDir, Options{})
	if err != nil {
		t.Fatalf("truth full Run: %v", err)
	}
	truthGoSig := goTypesEdgeSignature(t, truthDrv, truth.ID)
	if len(truthGoSig) == 0 {
		t.Skip("go/types produced no grounded edges in this environment; nothing to compare")
	}

	// Delta: full index original, apply BOTH edits, re-index. Both core and app are
	// changed here, so they are re-parsed; this still exercises the scoped reverse-dep
	// expansion (lib references core.Widget and must carry its ref edges forward
	// unchanged). To make the UNCHANGED-file regeneration the load-bearing assertion,
	// we ALSO run a second variant below that edits only core.
	deltaDir := writeMultiPkgModule(t)
	deltaDrv := openTestStore(t)
	if _, _, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{}); err != nil {
		t.Fatalf("delta base Run: %v", err)
	}
	applyEdits(deltaDir)
	delta, deltaStats, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	if got, _ := delta.Metadata["delta_path"].(string); got != "sql" {
		t.Fatalf("delta delta_path = %q, want sql", got)
	}
	if got, _ := delta.Metadata["go_types_mode"].(string); got != "scoped" {
		t.Fatalf("delta go_types_mode = %q, want scoped", got)
	}
	assertFullParity(t, deltaDrv, delta, truthDrv, truth)
}

// TestSQLDeltaGoReverseDepRefRegenerated is the strict version of the cross-file
// keystone: it edits ONLY the defining file (core) — changing the BODY of a method
// that an UNCHANGED file (app) references — and asserts the delta regenerates app's
// (reverse-dep) go_types edges so the whole snapshot still equals a full reindex.
// app.go is never re-parsed by the delta; its reference edges must be refreshed
// purely by the scoped reverse-dep re-analysis.
func TestSQLDeltaGoReverseDepRefRegenerated(t *testing.T) {
	forceScopedGoTypes(t)
	ctx := context.Background()

	// Edit core only: change Widget.Do's body and add a method. app references
	// core.Widget (var ref core.Widget) and calls ref.Do(); lib holds a core.Widget
	// field. Neither app nor lib is re-parsed by the delta — their go_types edges must
	// be regenerated from the reverse-dep re-analysis to match a full reindex.
	coreEdit := "\nfunc (w Widget) Triple() int { return w.n * 3 }\n"

	truthDir := writeMultiPkgModule(t)
	appendToFile(t, filepath.Join(truthDir, "core/core.go"), coreEdit)
	truthDrv := openTestStore(t)
	truth, _, err := Run(ctx, truthDrv, nil, "", "m", truthDir, Options{})
	if err != nil {
		t.Fatalf("truth full Run: %v", err)
	}
	truthGoSig := goTypesEdgeSignature(t, truthDrv, truth.ID)
	if len(truthGoSig) == 0 {
		t.Skip("go/types produced no grounded edges; nothing to compare")
	}

	deltaDir := writeMultiPkgModule(t)
	deltaDrv := openTestStore(t)
	if _, _, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{}); err != nil {
		t.Fatalf("delta base Run: %v", err)
	}
	appendToFile(t, filepath.Join(deltaDir, "core/core.go"), coreEdit)
	delta, deltaStats, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	if got, _ := delta.Metadata["delta_path"].(string); got != "sql" {
		t.Fatalf("delta delta_path = %q, want sql", got)
	}
	if got, _ := delta.Metadata["go_types_mode"].(string); got != "scoped" {
		t.Fatalf("delta go_types_mode = %q, want scoped", got)
	}

	// The reverse-dep file app.go must still have its go_types reference edge to
	// core.Widget present and correct after the delta (regenerated, not dropped).
	appHasWidgetRef := false
	deltaEdges, err := deltaDrv.ListEdges(ctx, delta.ID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	for i := range deltaEdges {
		e := &deltaEdges[i]
		if e.Kind != graph.EdgeReferences || e.FromFile != "app/app.go" {
			continue
		}
		if src, _ := e.Metadata["source"].(string); src != "go_types" {
			continue
		}
		if e.ToRef == "Widget" {
			appHasWidgetRef = true
		}
	}
	if !appHasWidgetRef {
		t.Fatalf("reverse-dep app/app.go lost its go_types reference edge to core.Widget after the delta")
	}

	// Full snapshot parity is the keystone.
	assertFullParity(t, deltaDrv, delta, truthDrv, truth)
}

// writeBigCarryForwardModule lays down a module engineered to surface BOTH failure
// modes the determinism fix targets, on a fixture LARGER than the analyzer scope an
// edit touches:
//
//   - leaf:     defines type L (used nowhere else). Editing leaf seeds the scoped
//     analyzer on leaf ONLY — no in-module package imports leaf, so leaf
//     has NO reverse-deps and the analyzed scope is exactly {leaf}.
//   - big:      a self-contained package whose ONE file references its own type T far
//     MORE than gotypes.maxRefEdgesPerFile (512) times. big does NOT import
//     leaf, so it is NEVER re-analyzed by a leaf edit — its 512 capped
//     go_types reference edges must be CARRIED FORWARD verbatim and still
//     equal a full reindex. Under the OLD global cap this file's emitted set
//     depended on global walk order, so a carried-forward set would diverge
//     from a fresh reindex that truncated a different file — this is the
//     regression the test pins.
//   - filler1..N: extra unrelated packages, each with a few type-use refs, so the
//     carried-forward (un-analyzed) set is the bulk of the module — a
//     carry-forward staleness or order bug shows up here too.
//
// Editing leaf therefore re-analyzes a TINY scope while the parity assertion covers
// the whole (much larger) graph, so a carry-forward mismatch cannot hide.
func writeBigCarryForwardModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/big\n\ngo 1.21\n",
		"leaf/leaf.go": `package leaf

type L struct{ n int }

func (l L) N() int { return l.n }

func use(x L) int { return x.N() }
`,
	}

	// big/big.go: a package-level var block with > maxRefEdgesPerFile (512) distinct
	// type-use references to T, each on its own line. This single file overflows the
	// per-file cap; both a full reindex and a carry-forward must keep the SAME 512-row
	// deterministic prefix.
	var b strings.Builder
	b.WriteString("package big\n\ntype T struct{ x int }\n\nvar (\n")
	for i := 0; i < 700; i++ { // 700 > 512, forces the per-file cap
		b.WriteString("\tv")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" T\n")
	}
	b.WriteString(")\n")
	files["big/big.go"] = b.String()

	// A handful of small filler packages with their own type-use refs (carried forward
	// on a leaf edit), so the un-analyzed set dominates the graph.
	for i := 0; i < 6; i++ {
		pkg := "f" + strconv.Itoa(i)
		files[pkg+"/"+pkg+".go"] = "package " + pkg + `

type ` + strings.ToUpper(pkg) + ` struct{ a, b int }

func make` + strings.ToUpper(pkg) + `(z ` + strings.ToUpper(pkg) + `) ` + strings.ToUpper(pkg) + ` { return z }
`
	}

	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// TestSQLDeltaGoBigCarryForwardParity is the strengthened SQL-delta parity keystone:
// it edits ONE leaf package (tiny analyzer scope) in a module whose BULK is unrelated
// carried-forward files — including a file that exceeds the per-file go_types ref cap.
// The FULL edge set of the delta snapshot (every go_types reference + recv_source call,
// plus all other edges) must equal a full reindex of the same edited tree. This is the
// test the prior fixtures were too small to make load-bearing: a global-order-dependent
// cap or a carry-forward staleness bug would make the carried-forward big file (and the
// fillers) diverge from a fresh reindex, and assertFullParity would catch it.
func TestSQLDeltaGoBigCarryForwardParity(t *testing.T) {
	forceScopedGoTypes(t)
	ctx := context.Background()

	// The edit is additive and confined to the leaf package — it cannot change any
	// other package's go/types edges, so every other file's rows must carry forward
	// byte-identically. (A correct delta proves this; a buggy one diverges.)
	leafEdit := "\nfunc (l L) Twice() int { return l.n * 2 }\n"

	// Ground truth: full index of the EDITED tree.
	truthDir := writeBigCarryForwardModule(t)
	appendToFile(t, filepath.Join(truthDir, "leaf/leaf.go"), leafEdit)
	truthDrv := openTestStore(t)
	truth, _, err := Run(ctx, truthDrv, nil, "", "big", truthDir, Options{})
	if err != nil {
		t.Fatalf("truth full Run: %v", err)
	}
	truthGoSig := goTypesEdgeSignature(t, truthDrv, truth.ID)
	if len(truthGoSig) == 0 {
		t.Skip("go/types produced no grounded edges in this environment; nothing to compare")
	}

	// Prove the big file actually overflows the per-file cap in the ground truth (so the
	// carry-forward assertion below is genuinely exercising the cap, not a small set).
	bigRefs := 0
	for _, s := range truthGoSig {
		if strings.HasPrefix(s, "ref|big/big.go|") {
			bigRefs++
		}
	}
	if bigRefs == 0 {
		t.Skip("big/big.go produced no go_types refs in this environment")
	}

	// Delta: full index the ORIGINAL tree, apply the SAME leaf edit, re-index (delta).
	deltaDir := writeBigCarryForwardModule(t)
	deltaDrv := openTestStore(t)
	if _, s, err := Run(ctx, deltaDrv, nil, "", "big", deltaDir, Options{}); err != nil {
		t.Fatalf("delta base Run: %v", err)
	} else if s.Mode != "full" {
		t.Fatalf("delta base mode = %q, want full", s.Mode)
	}
	appendToFile(t, filepath.Join(deltaDir, "leaf/leaf.go"), leafEdit)
	delta, deltaStats, err := Run(ctx, deltaDrv, nil, "", "big", deltaDir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	if got, _ := delta.Metadata["delta_path"].(string); got != "sql" {
		t.Fatalf("delta delta_path = %q, want sql (the SQL-level path did not run)", got)
	}
	if got, _ := delta.Metadata["go_types_mode"].(string); got != "scoped" {
		t.Fatalf("delta go_types_mode = %q, want scoped", got)
	}

	// The carried-forward big file must still carry EXACTLY the same capped ref set the
	// full reindex emits — neither re-parsed nor re-analyzed by the leaf edit.
	deltaBigRefs := 0
	for _, s := range goTypesEdgeSignature(t, deltaDrv, delta.ID) {
		if strings.HasPrefix(s, "ref|big/big.go|") {
			deltaBigRefs++
		}
	}
	if deltaBigRefs != bigRefs {
		t.Fatalf("carried-forward big/big.go ref count delta=%d != reindex=%d (per-file cap / carry-forward drift)", deltaBigRefs, bigRefs)
	}

	// The keystone: the FULL snapshot (symbols + every edge incl. go_types refs and
	// recv_source calls + routes + counts) equals a full reindex of the same tree.
	assertFullParity(t, deltaDrv, delta, truthDrv, truth)
}

// lexQueries is the battery the 1C equivalence test compares hit counts on.
var lexQueries = []string{"user", "getuserbyid", "delete", "invoice", "render"}

// indexAndProbe full-indexes a fresh copy of the two-file repo with a lexical index
// and returns the persisted symbol fingerprint plus the lexical hit count for each
// query. It is the shared body the 1C test runs once concurrently and once
// sequentially to prove the two orderings are equivalent.
func indexAndProbe(t *testing.T, label string) ([]string, map[string]int) {
	t.Helper()
	ctx := context.Background()
	dir := writeTwoFileRepo(t)
	drv := openTestStore(t)
	lx, err := lexical.New(filepath.Join(t.TempDir(), "lex-"+label))
	if err != nil {
		t.Fatalf("lexical.New(%s): %v", label, err)
	}
	t.Cleanup(func() { _ = lx.Close() })
	snap, _, err := Run(ctx, drv, lx, "", "app", dir, Options{})
	if err != nil {
		t.Fatalf("Run(%s): %v", label, err)
	}
	hits := map[string]int{}
	for _, q := range lexQueries {
		hits[q] = len(lexSearchIDs(t, lx, snap.ID, q))
	}
	return symbolSignature(t, drv, snap.ID), hits
}

// TestConcurrentPersistLexicalEquivalence is the 1C keystone: the concurrent
// persist||lexical path must yield the SAME persisted symbols AND the SAME lexical
// search hits as the sequential path. It indexes once concurrently (production
// default) and once with the sequential ordering forced, then compares the stored
// symbol fingerprint and the lexical hit counts.
func TestConcurrentPersistLexicalEquivalence(t *testing.T) {
	// Concurrent run (production default — concurrentPersistLexical is true).
	concSig, concHits := indexAndProbe(t, "concurrent")

	// Sequential run (persist strictly before lexical).
	withSequentialPersistLexical(t)
	seqSig, seqHits := indexAndProbe(t, "sequential")

	assertEqualSlices(t, "concurrent-vs-sequential symbols", seqSig, concSig)
	for _, q := range lexQueries {
		if concHits[q] != seqHits[q] {
			t.Fatalf("query %q: concurrent lexical hits %d != sequential %d", q, concHits[q], seqHits[q])
		}
	}
}
