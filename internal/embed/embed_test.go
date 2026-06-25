package embed

import (
	"context"
	"math"
	"testing"
)

// TestHashingDeterministic asserts the offline Hashing embedder is a pure
// function of its input: the same text always yields the identical vector.
func TestHashingDeterministic(t *testing.T) {
	p := NewHashing(0) // 0 -> default dim
	ctx := context.Background()

	a, err := p.Embed(ctx, []string{"GetUserByID"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	b, err := p.Embed(ctx, []string{"GetUserByID"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("want 1 vector each, got %d / %d", len(a), len(b))
	}
	if len(a[0]) != len(b[0]) {
		t.Fatalf("dim mismatch %d != %d", len(a[0]), len(b[0]))
	}
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("non-deterministic at %d: %v != %v", i, a[0][i], b[0][i])
		}
	}
}

// TestHashingDim asserts Dim() is the fixed configured dimension and that every
// emitted vector has exactly that length.
func TestHashingDim(t *testing.T) {
	const dim = 128
	p := NewHashing(dim)
	if p.Dim() != dim {
		t.Fatalf("Dim() = %d, want %d", p.Dim(), dim)
	}
	if p.Name() != "hashing" {
		t.Errorf("Name() = %q, want hashing", p.Name())
	}
	vecs, err := p.Embed(context.Background(), []string{"alpha beta", "gamma"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, v := range vecs {
		if len(v) != dim {
			t.Errorf("vector %d len = %d, want %d", i, len(v), dim)
		}
	}
}

// TestHashingL2Norm asserts every non-empty embedding is L2-normalized (unit
// length) so cosine similarity equals the dot product downstream.
func TestHashingL2Norm(t *testing.T) {
	p := NewHashing(256)
	vecs, err := p.Embed(context.Background(), []string{
		"CreateOrder placeOrder checkout",
		"x",
		"MultiWord identifier with_snake and-kebab",
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, v := range vecs {
		var sum float64
		for _, f := range v {
			sum += float64(f) * float64(f)
		}
		norm := math.Sqrt(sum)
		if math.Abs(norm-1.0) > 1e-5 {
			t.Errorf("vector %d L2 norm = %v, want 1.0", i, norm)
		}
	}
}

// TestHashingEmptyTextIsZero asserts a text with no tokens yields an all-zero
// vector (the honest "no signal" answer — not a NaN from dividing by zero).
func TestHashingEmptyTextIsZero(t *testing.T) {
	p := NewHashing(64)
	vecs, err := p.Embed(context.Background(), []string{"   "})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for _, f := range vecs[0] {
		if f != 0 {
			t.Fatalf("empty text should embed to all-zeros, got non-zero %v", f)
		}
	}
}

// TestHashingTokenOverlapRanks asserts the embedder captures token overlap:
// two texts that share tokens are more similar (higher cosine) than two that
// share none. This is the honest property the Hashing default provides.
func TestHashingTokenOverlapRanks(t *testing.T) {
	p := NewHashing(256)
	ctx := context.Background()
	vecs, err := p.Embed(ctx, []string{
		"GetUser fetch user by id",        // 0: query-ish
		"GetUserByID load user record",    // 1: shares user/get
		"RenderTemplate html layout pass", // 2: disjoint
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	dot := func(a, b []float32) float64 {
		var s float64
		for i := range a {
			s += float64(a[i]) * float64(b[i])
		}
		return s
	}
	related := dot(vecs[0], vecs[1])
	unrelated := dot(vecs[0], vecs[2])
	if related <= unrelated {
		t.Fatalf("expected token-overlap similarity %.4f > disjoint %.4f", related, unrelated)
	}
}

// TestNewProviderDefaultsToHashing asserts the factory returns the offline
// Hashing provider when ATLAS_EMBED_URL is unset.
func TestNewProviderDefaultsToHashing(t *testing.T) {
	t.Setenv(EnvEmbedURL, "")
	if got := NewProvider().Name(); got != "hashing" {
		t.Fatalf("NewProvider().Name() = %q, want hashing (no ATLAS_EMBED_URL)", got)
	}
}

// TestNewProviderHTTPWhenURLSet asserts the factory selects the HTTP provider
// when ATLAS_EMBED_URL is set (it does NOT make a call here — just type/name).
func TestNewProviderHTTPWhenURLSet(t *testing.T) {
	t.Setenv(EnvEmbedURL, "http://localhost:11434/api/embeddings")
	if got := NewProvider().Name(); got != "http" {
		t.Fatalf("NewProvider().Name() = %q, want http (ATLAS_EMBED_URL set)", got)
	}
}
