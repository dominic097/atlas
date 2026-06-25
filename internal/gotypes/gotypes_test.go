package gotypes

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
