package engine

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// newTestEngine builds a real local engine (SQLite + lexical) under a temp dir,
// with vectors optionally enabled. It is hermetic: no network, the offline
// Hashing embedder backs any embedding pass.
func newTestEngine(t *testing.T, vectors bool) Engine {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	opts := []Option{WithSQLite(dbPath)}
	if vectors {
		opts = append(opts, WithVectors(true))
	}
	eng, err := New(ctx, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

func TestSymbolToHitSanitizesNonFiniteScores(t *testing.T) {
	sym := graph.CodeSymbol{
		ID:        "sym-1",
		Name:      "ReviewContext",
		Kind:      "function",
		RepoID:    "repo-1",
		Path:      "internal/service/review.go",
		StartLine: 42,
	}

	cases := []float64{math.Inf(1), math.Inf(-1), math.NaN(), 12.5}
	for _, score := range cases {
		hit := symbolToHit(sym, score)
		if math.IsInf(hit.Score, 0) || math.IsNaN(hit.Score) {
			t.Fatalf("symbolToHit(%v) produced non-finite score %v", score, hit.Score)
		}
	}
}

// writeFixtureRepo writes a tiny Go package whose symbol names/docs make token
// overlap meaningful for the ranking smoke test, and returns its path.
func writeFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := `package shop

// GetUserByID fetches a user record by its identifier.
func GetUserByID(id string) string { return id }

// RenderInvoiceTemplate renders the printable invoice HTML layout.
func RenderInvoiceTemplate(data string) string { return data }

// LoadUserProfile loads the profile for a user.
func LoadUserProfile(user string) string { return user }
`
	if err := os.WriteFile(filepath.Join(dir, "shop.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

// TestSemanticSearchDegradesWithoutVectors asserts that with vectors OFF (the
// default), SemanticSearch runs the lexical path and reports the degrade honestly
// instead of erroring.
func TestSemanticSearchDegradesWithoutVectors(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t, false) // vectors OFF
	repo := writeFixtureRepo(t)

	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}

	res, err := eng.SemanticSearch(ctx, SemanticSearchInput{Query: "user", Limit: 10})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if !res.Degraded {
		t.Errorf("Degraded = false, want true (vectors off)")
	}
	if res.ModeUsed != "lexical" {
		t.Errorf("ModeUsed = %q, want lexical", res.ModeUsed)
	}
	if len(res.Results) == 0 {
		t.Errorf("degraded lexical search returned no hits for 'user'")
	}
}

// TestSemanticSearchDegradesWhenNoEmbeddings asserts that even with vectors
// ENABLED, a snapshot indexed without embeddings degrades to lexical (the
// embeddings table is empty for that snapshot).
func TestSemanticSearchDegradesWhenNoEmbeddings(t *testing.T) {
	ctx := context.Background()
	// Vectors enabled at query time, but index WITHOUT the embedding pass by
	// using a separate engine that has vectors off for indexing. Simpler: index
	// with a vectors-off engine, then open a vectors-on engine on the same db.
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	repo := writeFixtureRepo(t)

	indexEng, err := New(ctx, WithSQLite(dbPath)) // vectors off -> no embeddings written
	if err != nil {
		t.Fatalf("New(index): %v", err)
	}
	if _, err := indexEng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	_ = indexEng.Close()

	queryEng, err := New(ctx, WithSQLite(dbPath), WithVectors(true)) // vectors on at query time
	if err != nil {
		t.Fatalf("New(query): %v", err)
	}
	t.Cleanup(func() { _ = queryEng.Close() })

	res, err := queryEng.SemanticSearch(ctx, SemanticSearchInput{Query: "user", Limit: 10})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if !res.Degraded || res.ModeUsed != "lexical" {
		t.Errorf("expected degrade to lexical (no embeddings), got Degraded=%v ModeUsed=%q", res.Degraded, res.ModeUsed)
	}
}

// TestSemanticSearchRankingWithVectors is the ranking smoke test: index with
// vectors ON (offline Hashing embedder), then a semantic query for a user-related
// term must run in "semantic" mode and rank the user-named symbols above the
// disjoint invoice-template symbol.
func TestSemanticSearchRankingWithVectors(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t, true) // vectors ON
	repo := writeFixtureRepo(t)

	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}

	res, err := eng.SemanticSearch(ctx, SemanticSearchInput{Query: "GetUserByID user identifier", Limit: 10})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if res.Degraded {
		t.Fatalf("Degraded = true, want false (vectors on, embeddings present)")
	}
	if res.ModeUsed != "semantic" {
		t.Fatalf("ModeUsed = %q, want semantic", res.ModeUsed)
	}
	if len(res.Results) == 0 {
		t.Fatalf("semantic search returned no hits")
	}

	// Rank check: the best hit must be a user-related symbol, and it must outrank
	// the disjoint RenderInvoiceTemplate.
	rank := map[string]int{}
	for i, h := range res.Results {
		rank[h.Name] = i
	}
	getUser, hasGetUser := rank["GetUserByID"]
	invoice, hasInvoice := rank["RenderInvoiceTemplate"]
	if !hasGetUser {
		t.Fatalf("GetUserByID not in results: %+v", res.Results)
	}
	if hasInvoice && getUser >= invoice {
		t.Errorf("expected GetUserByID (rank %d) to outrank RenderInvoiceTemplate (rank %d)", getUser, invoice)
	}
	if res.Results[0].Score <= 0 {
		t.Errorf("top semantic score = %v, want > 0", res.Results[0].Score)
	}
}

// TestSearchSemanticModeDelegates asserts the catalog Search(mode=semantic)
// delegates to SemanticSearch and reports the degraded lexical mode when vectors
// are off — proving the previously-hard-error branch now degrades instead.
func TestSearchSemanticModeDelegates(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t, false)
	repo := writeFixtureRepo(t)
	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}

	res, err := eng.Search(ctx, SearchInput{Query: "user", Mode: "semantic", Limit: 5})
	if err != nil {
		t.Fatalf("Search(semantic) errored, want graceful degrade: %v", err)
	}
	if res.ModeUsed != "lexical" {
		t.Errorf("ModeUsed = %q, want lexical (degraded)", res.ModeUsed)
	}
}
