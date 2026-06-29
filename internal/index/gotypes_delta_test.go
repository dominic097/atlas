package index

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/dominic097/atlas/internal/gotypes"
	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/store"
)

// forceScopedGoTypes lowers the scoped-path size gate to 0 for the duration of a
// test so the incremental go/types wiring is exercised on small fixtures (production
// keeps the conservative gotypes.ScopedMinGoFiles floor). Restored on cleanup.
func forceScopedGoTypes(t *testing.T) {
	t.Helper()
	prev := gotypes.ScopedMinGoFiles
	gotypes.ScopedMinGoFiles = 0
	t.Cleanup(func() { gotypes.ScopedMinGoFiles = prev })
}

// writeMultiPkgModule lays down a 3-package module where app and lib import core,
// and solo imports nothing in the module. Editing core therefore makes app+lib
// reverse-deps (their go/types edges may shift) while solo's edges cannot change —
// exactly the boundary the scoped delta enrichment must respect.
func writeMultiPkgModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.21\n",
		"core/core.go": `package core

type Widget struct{ n int }

func (w Widget) Do() int { return w.n }

func New() Widget { return Widget{} }
`,
		"app/app.go": `package app

import "example.com/m/core"

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

// goTypesEdgeSignature returns a sorted, canonical fingerprint of every
// go/types-grounded edge in a snapshot: precise recv_type call edges (recv_source
// == go_types) and type-use reference edges (source == go_types). Two snapshots
// with the same fingerprint carry byte-identical go/types precision.
func goTypesEdgeSignature(t *testing.T, drv store.StorageDriver, snapshotID string) []string {
	t.Helper()
	edges, err := drv.ListEdges(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	var sig []string
	for _, e := range edges {
		if e.Metadata == nil {
			continue
		}
		if src, _ := e.Metadata["recv_source"].(string); src == "go_types" && e.Kind == graph.EdgeCalls {
			recv, _ := e.Metadata["recv_type"].(string)
			sig = append(sig, "call|"+e.FromFile+"|"+strconv.Itoa(e.Line)+"|"+e.ToRef+"|"+recv)
			continue
		}
		if e.Kind == graph.EdgeReferences {
			if src, _ := e.Metadata["source"].(string); src == "go_types" {
				qual, _ := e.Metadata["qualified_ref"].(string)
				sig = append(sig, "ref|"+e.FromFile+"|"+strconv.Itoa(e.Line)+"|"+e.FromSymbol+"|"+e.ToRef+"|"+qual)
			}
		}
	}
	sort.Strings(sig)
	return sig
}

func equalSig(a, b []string) (string, bool) {
	if len(a) != len(b) {
		return "len " + strconv.Itoa(len(a)) + " vs " + strconv.Itoa(len(b)), false
	}
	for i := range a {
		if a[i] != b[i] {
			return "row " + strconv.Itoa(i) + ": " + a[i] + " vs " + b[i], false
		}
	}
	return "", true
}

// TestDeltaScopedGoTypesMatchesFullReindex is the integration precision keystone:
// after an uncommitted edit to a file in package core, the DELTA path (which runs
// the SCOPED go/types enrichment over core + its reverse-deps) must produce a
// go/types edge set byte-identical to a FULL REINDEX of the same edited tree. If
// scoping ever drops a recv_type override or a reference edge — for the changed
// file OR any reverse-dep — this fails.
func TestDeltaScopedGoTypesMatchesFullReindex(t *testing.T) {
	forceScopedGoTypes(t)
	ctx := context.Background()

	// Ground truth: a fresh full index of the EDITED tree, in its own store.
	truthDir := writeMultiPkgModule(t)
	appendToFile(t, filepath.Join(truthDir, "core/core.go"),
		"\nfunc (w Widget) Twice() int { return w.n * 2 }\n")
	truthDrv := openTestStore(t)
	truth, truthStats, err := Run(ctx, truthDrv, nil, "", "m", truthDir, Options{})
	if err != nil {
		t.Fatalf("ground-truth full Run: %v", err)
	}
	if truthStats.Mode != "full" {
		t.Fatalf("ground-truth mode = %q, want full", truthStats.Mode)
	}
	truthSig := goTypesEdgeSignature(t, truthDrv, truth.ID)
	if len(truthSig) == 0 {
		t.Skip("go/types produced no grounded edges in this environment; nothing to compare")
	}

	// Delta path: full index the ORIGINAL tree, then apply the SAME edit and re-index
	// (delta). The scoped enrichment must reach core + app + lib.
	deltaDir := writeMultiPkgModule(t)
	deltaDrv := openTestStore(t)
	if _, s, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{}); err != nil {
		t.Fatalf("delta base full Run: %v", err)
	} else if s.Mode != "full" {
		t.Fatalf("delta base mode = %q, want full", s.Mode)
	}
	appendToFile(t, filepath.Join(deltaDir, "core/core.go"),
		"\nfunc (w Widget) Twice() int { return w.n * 2 }\n")
	delta, deltaStats, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	// Prove the SCOPED path actually ran (not a silent whole-module fallback).
	if got, _ := delta.Metadata["go_types_mode"].(string); got != "scoped" {
		t.Fatalf("delta go_types_mode = %q, want scoped (scoped wiring did not run)", got)
	}
	deltaSig := goTypesEdgeSignature(t, deltaDrv, delta.ID)

	if diff, ok := equalSig(truthSig, deltaSig); !ok {
		t.Fatalf("scoped delta go/types edges differ from full reindex: %s\n full(%d)=%v\n delta(%d)=%v",
			diff, len(truthSig), truthSig, len(deltaSig), deltaSig)
	}
}

// TestDeltaScopedGoTypesMatchesFullReindex_ReverseDepEdit edits a reverse-dep file
// (app) instead of core. The scoped path seeds on app's package; app has no in-module
// reverse-deps, so only app is re-analyzed, and core/lib/solo carry forward. The
// result must still match a full reindex of the edited tree.
func TestDeltaScopedGoTypesMatchesFullReindex_ReverseDepEdit(t *testing.T) {
	forceScopedGoTypes(t)
	ctx := context.Background()

	truthDir := writeMultiPkgModule(t)
	appendToFile(t, filepath.Join(truthDir, "app/app.go"),
		"\nfunc Extra() int { w := core.New(); return w.Do() }\n")
	truthDrv := openTestStore(t)
	truth, _, err := Run(ctx, truthDrv, nil, "", "m", truthDir, Options{})
	if err != nil {
		t.Fatalf("ground-truth full Run: %v", err)
	}
	truthSig := goTypesEdgeSignature(t, truthDrv, truth.ID)
	if len(truthSig) == 0 {
		t.Skip("no grounded edges; nothing to compare")
	}

	deltaDir := writeMultiPkgModule(t)
	deltaDrv := openTestStore(t)
	if _, _, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{}); err != nil {
		t.Fatalf("delta base Run: %v", err)
	}
	appendToFile(t, filepath.Join(deltaDir, "app/app.go"),
		"\nfunc Extra() int { w := core.New(); return w.Do() }\n")
	delta, deltaStats, err := Run(ctx, deltaDrv, nil, "", "m", deltaDir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	if got, _ := delta.Metadata["go_types_mode"].(string); got != "scoped" {
		t.Fatalf("delta go_types_mode = %q, want scoped", got)
	}
	deltaSig := goTypesEdgeSignature(t, deltaDrv, delta.ID)

	if diff, ok := equalSig(truthSig, deltaSig); !ok {
		t.Fatalf("reverse-dep-edit scoped delta differs from full reindex: %s\n full=%v\n delta=%v",
			diff, truthSig, deltaSig)
	}
}

// TestDeltaSmallModuleTakesWholeModulePath proves the size GATE: a small module
// (below gotypes.ScopedMinGoFiles, the default) takes the whole-module go/types path,
// not the scoped path — so there is no scoped overhead (the metadata `go list`) on
// the small repos where scoping does not pay off. Correctness is identical either
// way; this guards the performance gate, not precision.
func TestDeltaSmallModuleTakesWholeModulePath(t *testing.T) {
	ctx := context.Background()
	// Do NOT lower the gate — exercise the production default (300).
	dir := writeMultiPkgModule(t) // 4 Go files, well below the gate
	drv := openTestStore(t)
	if _, _, err := Run(ctx, drv, nil, "", "m", dir, Options{}); err != nil {
		t.Fatalf("base Run: %v", err)
	}
	appendToFile(t, filepath.Join(dir, "core/core.go"),
		"\nfunc (w Widget) Gate() int { return 1 }\n")
	delta, deltaStats, err := Run(ctx, drv, nil, "", "m", dir, Options{})
	if err != nil {
		t.Fatalf("delta Run: %v", err)
	}
	if deltaStats.Mode != "delta" {
		t.Fatalf("delta mode = %q, want delta", deltaStats.Mode)
	}
	mode, _ := delta.Metadata["go_types_mode"].(string)
	if mode != "whole_module" {
		t.Fatalf("small-module delta go_types_mode = %q, want whole_module (gate should keep small repos on the safe path)", mode)
	}
}
