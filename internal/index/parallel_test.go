package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"

	"github.com/dominic097/atlas/internal/store"
)

// writeMultiLangRepo lays down a multi-file, multi-language repo so an index has
// real cross-file work to parallelize. Files are deliberately interdependent enough
// to exercise symbols, call edges, import edges, and go/types enrichment.
func writeMultiLangRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/p\n\ngo 1.21\n",
		"a.go": `package p

type Server struct{ name string }

func (s Server) Name() string { return s.name }

func NewServer() Server { return Server{name: "x"} }
`,
		"b.go": `package p

func Use() string {
	s := NewServer()
	return s.Name()
}

func Helper(x Server) string { return x.Name() }
`,
		"util/util.go": `package util

import "strings"

func Upper(s string) string { return strings.ToUpper(s) }
`,
		"script.py": `import os

class Greeter:
    def hello(self):
        return "hi"

def run():
    g = Greeter()
    return g.hello()
`,
		"app.js": `import { foo } from "./mod";

export function bar() {
	return foo();
}

class Widget {
	render() { return 1; }
}
`,
		"Main.java": `package com.example;

import java.util.List;

public class Main {
	public int compute() { return 42; }
	public void run() { compute(); }
}
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

// snapshotFingerprint is a fully-ordered, canonical fingerprint of a snapshot's
// graph: every file, symbol, and edge rendered to a sorted string slice. Two
// fingerprints are equal iff the snapshots are byte-identical in the graph content
// that matters (independent of the per-row surrogate IDs / snapshot id).
func snapshotFingerprint(t *testing.T, drv store.StorageDriver, snapshotID string) []string {
	t.Helper()
	ctx := context.Background()

	files, err := drv.ListFiles(ctx, snapshotID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	syms, err := drv.ListSymbols(ctx, snapshotID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	edges, err := drv.ListEdges(ctx, snapshotID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}

	var out []string
	for _, f := range files {
		out = append(out, "file|"+f.Path+"|"+f.Language+"|"+f.Hash+"|"+strconv.FormatInt(f.SizeBytes, 10))
	}
	for _, s := range syms {
		out = append(out, "sym|"+s.Path+"|"+s.Kind+"|"+s.Name+"|"+strconv.Itoa(s.StartLine)+"|"+s.Signature)
	}
	for _, e := range edges {
		recv, _ := e.Metadata["recv_type"].(string)
		recvSrc, _ := e.Metadata["recv_source"].(string)
		out = append(out, fmt.Sprintf("edge|%s|%s|%s|%s|%d|%s|%s",
			e.FromFile, e.FromSymbol, e.ToRef, e.Kind, e.Line, recv, recvSrc))
	}
	sort.Strings(out)
	return out
}

func equalFingerprint(a, b []string) (string, bool) {
	if len(a) != len(b) {
		return fmt.Sprintf("len %d vs %d", len(a), len(b)), false
	}
	for i := range a {
		if a[i] != b[i] {
			return fmt.Sprintf("row %d: %q vs %q", i, a[i], b[i]), false
		}
	}
	return "", true
}

// indexFresh full-indexes dir into a brand-new store and returns the snapshot
// fingerprint. Each call is isolated so there is no delta/noop interference.
func indexFresh(t *testing.T, dir string) []string {
	t.Helper()
	drv := openTestStore(t)
	snap, stats, err := Run(context.Background(), drv, nil, "", "p", dir, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Mode != "full" {
		t.Fatalf("mode = %q, want full", stats.Mode)
	}
	return snapshotFingerprint(t, drv, snap.ID)
}

// TestParallelIndexEqualsSerialIndex proves the parallel parse path produces a
// snapshot byte-identical (symbols/edges/files, sorted) to a serial (GOMAXPROCS=1)
// index of the same fixture. Forcing GOMAXPROCS=1 routes parseCandidates /
// hashWorkTreeCandidates through a single worker (the serial baseline); restoring it
// runs them across all CPUs (the parallel path). The deterministic sort the index
// applies must reconcile the two to the same graph.
func TestParallelIndexEqualsSerialIndex(t *testing.T) {
	dir := writeMultiLangRepo(t)

	prev := runtime.GOMAXPROCS(1)
	serial := indexFresh(t, dir)
	runtime.GOMAXPROCS(prev)
	if runtime.NumCPU() < 2 {
		t.Skip("single-CPU environment: serial and parallel paths are identical by construction")
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	parallel := indexFresh(t, dir)

	if len(serial) == 0 {
		t.Fatal("serial index produced an empty fingerprint")
	}
	if diff, ok := equalFingerprint(serial, parallel); !ok {
		t.Fatalf("parallel index != serial index: %s", diff)
	}
}

// TestParallelIndexDeterministicAcrossRuns indexes the same fixture repeatedly under
// full parallelism and asserts every run yields an identical snapshot fingerprint —
// no nondeterminism leaks from worker-completion order into the persisted graph.
func TestParallelIndexDeterministicAcrossRuns(t *testing.T) {
	dir := writeMultiLangRepo(t)

	prev := runtime.GOMAXPROCS(runtime.NumCPU())
	defer runtime.GOMAXPROCS(prev)

	first := indexFresh(t, dir)
	if len(first) == 0 {
		t.Fatal("first index produced an empty fingerprint")
	}
	for run := 0; run < 5; run++ {
		next := indexFresh(t, dir)
		if diff, ok := equalFingerprint(first, next); !ok {
			t.Fatalf("run %d differs from first (nondeterministic): %s", run, diff)
		}
	}
}
