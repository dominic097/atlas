package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeContextFixture writes a 2-file Go package where AuthenticateUser (auth.go)
// calls ValidateToken (token.go), so the context op exercises symbols, a
// cross-file call edge, and impact.
func writeContextFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"auth.go":  "package svc\n\n// AuthenticateUser verifies a token and returns the user id.\nfunc AuthenticateUser(token string) string {\n\treturn ValidateToken(token)\n}\n",
		"token.go": "package svc\n\n// ValidateToken checks a token signature and echoes it back.\nfunc ValidateToken(token string) string { return token }\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// TestContextBundleForSeedPath asserts the context op packs the changed file's
// symbols (with body excerpts — spans, not whole files), surfaces the seed file,
// and reports a mode.
func TestContextBundleForSeedPath(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t, false)
	repo := writeContextFixture(t)
	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}

	res, err := eng.Context(ctx, ContextInput{Paths: []string{"auth.go"}})
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if !ctxHasSymbol(res.Symbols, "AuthenticateUser") {
		t.Fatalf("context missing seed symbol AuthenticateUser; got %v", ctxSymbolNames(res.Symbols))
	}
	if !ctxHasFile(res.Files, "auth.go") {
		t.Fatalf("context missing seed file auth.go; got %v", ctxFilePaths(res.Files))
	}
	// The whole point of the context op: a packed symbol carries a bounded
	// body_excerpt span instead of the agent having to read the whole file.
	if !ctxAnyExcerpt(res.Symbols) {
		t.Error("no packed symbol carried a body_excerpt; context should pack spans, not whole files")
	}
	if res.Mode == "" {
		t.Error("Mode empty")
	}
	if res.SnapshotID == "" {
		t.Error("SnapshotID empty")
	}
}

// TestContextRespectsBudgets asserts the MaxFiles/Limit/MaxEdges budgets cap the
// bundle so an agent can bound the token cost.
func TestContextRespectsBudgets(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t, false)
	repo := writeContextFixture(t)
	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	res, err := eng.Context(ctx, ContextInput{Paths: []string{"auth.go"}, MaxFiles: 1, Limit: 1, MaxEdges: 1})
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(res.Files) > 1 {
		t.Errorf("MaxFiles=1 but got %d files", len(res.Files))
	}
	if len(res.Symbols) > 1 {
		t.Errorf("Limit=1 but got %d symbols", len(res.Symbols))
	}
	if len(res.Edges) > 1 {
		t.Errorf("MaxEdges=1 but got %d edges", len(res.Edges))
	}
}

// writeManySymbolsFixture writes one Go file with n exported functions so a
// budget cap is observable (the seed-path symbols alone exceed a small Limit).
func writeManySymbolsFixture(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("package big\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "// Func%d does thing %d.\nfunc Func%d() int { return %d }\n\n", i, i, i, i)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

// TestContextBudgetFromOption asserts a WithContextBudget default is applied when
// the request omits budgets, and that a per-request value still overrides it.
func TestContextBudgetFromOption(t *testing.T) {
	ctx := context.Background()
	repo := writeManySymbolsFixture(t, 8)
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	eng, err := New(ctx, WithSQLite(dbPath), WithContextBudget(ContextBudget{Limit: 3}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}

	res, err := eng.Context(ctx, ContextInput{Paths: []string{"big.go"}}) // no per-request budget
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(res.Symbols) > 3 {
		t.Errorf("configured Limit=3 not applied: got %d symbols", len(res.Symbols))
	}

	tighter, err := eng.Context(ctx, ContextInput{Paths: []string{"big.go"}, Limit: 1})
	if err != nil {
		t.Fatalf("Context(override): %v", err)
	}
	if len(tighter.Symbols) > 1 {
		t.Errorf("per-request Limit=1 should override configured 3: got %d", len(tighter.Symbols))
	}
}

// TestContextBudgetFromEnv asserts ATLAS_CONTEXT_LIMIT configures the default.
func TestContextBudgetFromEnv(t *testing.T) {
	t.Setenv("ATLAS_CONTEXT_LIMIT", "2")
	ctx := context.Background()
	repo := writeManySymbolsFixture(t, 8)
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	eng, err := New(ctx, WithSQLite(dbPath))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}

	res, err := eng.Context(ctx, ContextInput{Paths: []string{"big.go"}})
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(res.Symbols) > 2 {
		t.Errorf("ATLAS_CONTEXT_LIMIT=2 not applied: got %d symbols", len(res.Symbols))
	}
}

func ctxHasSymbol(syms []ContextSymbol, name string) bool {
	for _, s := range syms {
		if s.Name == name {
			return true
		}
	}
	return false
}

func ctxSymbolNames(syms []ContextSymbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}

func ctxHasFile(files []ContextFile, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}

func ctxFilePaths(files []ContextFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func ctxAnyExcerpt(syms []ContextSymbol) bool {
	for _, s := range syms {
		if ex, _ := s.Metadata["body_excerpt"].(string); ex != "" {
			return true
		}
	}
	return false
}
