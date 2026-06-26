package atlas_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dominic097/atlas/pkg/atlas"
)

// TestCoverageImportRuntime indexes a tiny Go repo, imports a Go coverprofile
// over it, and asserts that `coverage` then reports mode=runtime with the real
// covered/total line ratio (covered for an exercised symbol, uncovered for one
// whose lines were never hit).
func TestCoverageImportRuntime(t *testing.T) {
	ctx := context.Background()

	repo := t.TempDir()
	// foo.go: Foo spans lines 3-5, Bar spans lines 7-9.
	src := "package sample\n" + // 1
		"\n" + // 2
		"func Foo() int {\n" + // 3
		"\treturn 1\n" + // 4
		"}\n" + // 5
		"\n" + // 6
		"func Bar() int {\n" + // 7
		"\treturn 2\n" + // 8
		"}\n" // 9
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	eng, err := atlas.New(ctx, atlas.WithSQLite(dbPath))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer eng.Close()

	if _, err := eng.Index(ctx, atlas.IndexInput{ProjectPath: repo, Repo: "org/sample"}); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// Coverprofile: Foo's lines 3-4 covered (count 1), Bar's lines 7-8 not (count 0).
	profile := "mode: set\n" +
		"foo.go:3.16,5.2 1 1\n" +
		"foo.go:7.16,9.2 1 0\n"
	profPath := filepath.Join(t.TempDir(), "cover.out")
	if err := os.WriteFile(profPath, []byte(profile), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	imp, err := eng.CoverageImport(ctx, atlas.CoverageImportInput{Path: profPath, RepoID: "org/sample"})
	if err != nil {
		t.Fatalf("CoverageImport: %v", err)
	}
	if imp.Format != "go" {
		t.Errorf("format = %q, want go", imp.Format)
	}
	if imp.FactsWritten == 0 {
		t.Fatalf("FactsWritten = 0, want > 0")
	}
	if imp.SymbolsCovered < 1 {
		t.Errorf("SymbolsCovered = %d, want >= 1 (Foo)", imp.SymbolsCovered)
	}

	// Foo: covered (lines 3-4 hit). Empty RepoID resolves the single indexed repo.
	fooCov, err := eng.Coverage(ctx, atlas.CoverageInput{Target: "Foo"})
	if err != nil {
		t.Fatalf("Coverage(Foo): %v", err)
	}
	if fooCov.Mode != "runtime" {
		t.Fatalf("Coverage(Foo).Mode = %q, want runtime", fooCov.Mode)
	}
	if !fooCov.Covered {
		t.Errorf("Coverage(Foo).Covered = false, want true (lines hit); strength=%q", fooCov.Strength)
	}
	if fooCov.Strength == "" {
		t.Errorf("Coverage(Foo).Strength is empty, want covered/total ratio")
	}

	// Bar: present in the profile but zero count -> uncovered, still runtime mode.
	barCov, err := eng.Coverage(ctx, atlas.CoverageInput{Target: "Bar"})
	if err != nil {
		t.Fatalf("Coverage(Bar): %v", err)
	}
	if barCov.Mode != "runtime" {
		t.Fatalf("Coverage(Bar).Mode = %q, want runtime", barCov.Mode)
	}
	if barCov.Covered {
		t.Errorf("Coverage(Bar).Covered = true, want false (count 0); strength=%q", barCov.Strength)
	}
}
