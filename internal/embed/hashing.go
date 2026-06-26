package embed

import (
	"context"
	"hash/fnv"
	"math"

	"github.com/dominic097/atlas/internal/lexical"
)

// hashingProvider is a deterministic, fully offline feature-hash embedder. Each
// text is tokenized with the SAME code-aware splitter the lexical index uses
// (lexical.TokenizeIdentifier: lowercase + camel/snake/kebab split, keeping the
// whole token), every token is hashed into one of Dim buckets, and the bucket
// counts are L2 normalized. The result is a sparse, signed bag-of-tokens vector:
// two texts that share tokens point in a similar direction, so cosine similarity
// is honest token-overlap — NOT a learned semantic space. It makes no external
// calls and is the zero-infra default.
type hashingProvider struct {
	dim int
}

// NewHashing builds the deterministic offline embedder with the given dimension.
// A non-positive dim falls back to the package default.
func NewHashing(dim int) Provider {
	if dim <= 0 {
		dim = defaultHashingDim
	}
	return &hashingProvider{dim: dim}
}

func (h *hashingProvider) Dim() int     { return h.dim }
func (h *hashingProvider) Name() string { return "hashing" }

// Embed feature-hashes each text into a Dim-length L2-normalized vector. It never
// errors (offline, pure function of the input), so the error is always nil — the
// signature matches Provider so the HTTP provider can share callers.
func (h *hashingProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = h.embedOne(t)
	}
	return out, nil
}

// embedOne hashes one text's tokens into Dim buckets and L2-normalizes. A sign
// derived from a second hash of each token keeps the projection roughly
// zero-mean (the standard signed feature-hashing trick), reducing collisions
// inflating the magnitude.
func (h *hashingProvider) embedOne(text string) []float32 {
	vec := make([]float32, h.dim)
	for _, tok := range lexical.TokenizeIdentifier(text) {
		bucket := hashBucket(tok, h.dim)
		if signBit(tok) {
			vec[bucket]++
		} else {
			vec[bucket]--
		}
	}
	l2Normalize(vec)
	return vec
}

// hashBucket maps a token to [0, dim) via FNV-1a.
func hashBucket(tok string, dim int) int {
	hh := fnv.New32a()
	_, _ = hh.Write([]byte(tok))
	return int(hh.Sum32() % uint32(dim))
}

// signBit derives a stable +/- for a token from a salted FNV-1a hash so two
// distinct tokens that collide in the same bucket tend to cancel rather than
// always add.
func signBit(tok string) bool {
	hh := fnv.New32a()
	_, _ = hh.Write([]byte("sign:" + tok))
	return hh.Sum32()&1 == 0
}

// l2Normalize scales vec to unit length in place. A zero vector (no tokens) is
// left as all-zeros — its cosine against anything is 0, which is the correct
// "no signal" answer.
func l2Normalize(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= inv
	}
}
