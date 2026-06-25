package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// newTestStore opens a fresh migrated SQLite driver in a temp dir.
func newTestStore(t *testing.T) StorageDriver {
	t.Helper()
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return d
}

// TestEmbeddingsRoundTripAndRanking persists three L2-normalized vectors and
// asserts NearestSymbols ranks them by cosine against a query, honors minScore
// and limit, and that a re-save for the same snapshot REPLACES (does not append).
func TestEmbeddingsRoundTripAndRanking(t *testing.T) {
	ctx := context.Background()
	d := newTestStore(t)

	const snap = "snap-emb"
	// Three unit vectors in a 4-D space. Query is exactly s1's direction, so:
	//   cosine(query, s1) = 1.0 (identical)
	//   cosine(query, s2) ~ 0.7 (partial overlap)
	//   cosine(query, s3) = 0.0 (orthogonal)
	s1 := []float32{1, 0, 0, 0}
	s2 := []float32{0.70710677, 0.70710677, 0, 0}
	s3 := []float32{0, 0, 1, 0}
	embs := []graph.SymbolEmbedding{
		{SnapshotID: snap, SymbolID: "s1", Dim: 4, Vector: s1},
		{SnapshotID: snap, SymbolID: "s2", Dim: 4, Vector: s2},
		{SnapshotID: snap, SymbolID: "s3", Dim: 4, Vector: s3},
	}
	if err := d.SaveEmbeddings(ctx, snap, embs); err != nil {
		t.Fatalf("SaveEmbeddings: %v", err)
	}

	query := []float32{1, 0, 0, 0}

	// Full ranking (limit 10, no floor): s1 then s2 then s3 (s3 score 0 included).
	got, err := d.NearestSymbols(ctx, snap, query, 10, -1)
	if err != nil {
		t.Fatalf("NearestSymbols: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3: %+v", len(got), got)
	}
	if got[0].SymbolID != "s1" || got[1].SymbolID != "s2" || got[2].SymbolID != "s3" {
		t.Fatalf("ranking = [%s %s %s], want [s1 s2 s3]", got[0].SymbolID, got[1].SymbolID, got[2].SymbolID)
	}
	if got[0].Score < 0.999 {
		t.Errorf("top score = %v, want ~1.0 (identical direction)", got[0].Score)
	}
	if got[1].Score <= got[2].Score {
		t.Errorf("expected s2 score %v > s3 score %v", got[1].Score, got[2].Score)
	}

	// minScore floor drops the orthogonal s3 (score 0).
	floored, err := d.NearestSymbols(ctx, snap, query, 10, 0.5)
	if err != nil {
		t.Fatalf("NearestSymbols(floor): %v", err)
	}
	if len(floored) != 2 {
		t.Fatalf("with minScore 0.5 got %d results, want 2: %+v", len(floored), floored)
	}
	for _, r := range floored {
		if r.SymbolID == "s3" {
			t.Errorf("s3 (orthogonal) should be filtered by minScore 0.5")
		}
	}

	// limit caps the result count.
	top1, err := d.NearestSymbols(ctx, snap, query, 1, -1)
	if err != nil {
		t.Fatalf("NearestSymbols(limit 1): %v", err)
	}
	if len(top1) != 1 || top1[0].SymbolID != "s1" {
		t.Fatalf("limit 1 = %+v, want exactly [s1]", top1)
	}

	// Re-save with a single vector REPLACES the prior three.
	if err := d.SaveEmbeddings(ctx, snap, []graph.SymbolEmbedding{
		{SnapshotID: snap, SymbolID: "only", Dim: 4, Vector: []float32{0, 1, 0, 0}},
	}); err != nil {
		t.Fatalf("SaveEmbeddings(replace): %v", err)
	}
	after, err := d.NearestSymbols(ctx, snap, []float32{0, 1, 0, 0}, 10, -1)
	if err != nil {
		t.Fatalf("NearestSymbols(after replace): %v", err)
	}
	if len(after) != 1 || after[0].SymbolID != "only" {
		t.Fatalf("after re-save = %+v, want exactly [only] (replace, not append)", after)
	}
}

// TestEmbeddingsEmptySnapshot asserts NearestSymbols on a snapshot with no
// embeddings returns no results and no error (the degrade-probe relies on this).
func TestEmbeddingsEmptySnapshot(t *testing.T) {
	ctx := context.Background()
	d := newTestStore(t)
	got, err := d.NearestSymbols(ctx, "no-such-snap", []float32{1, 0}, 5, -1)
	if err != nil {
		t.Fatalf("NearestSymbols(empty): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d results for empty snapshot, want 0", len(got))
	}
}

// TestEncodeDecodeVectorRoundTrip asserts the little-endian float32 encoding is
// lossless.
func TestEncodeDecodeVectorRoundTrip(t *testing.T) {
	in := []float32{0, 1, -1, 0.5, 3.14159, -2.71828}
	out := decodeVector(encodeVector(in))
	if len(out) != len(in) {
		t.Fatalf("decoded len %d != %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("element %d: decoded %v != %v", i, out[i], in[i])
		}
	}
}
