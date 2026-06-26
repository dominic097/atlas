package engine

import (
	"context"
	"os"
	"path/filepath"
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
