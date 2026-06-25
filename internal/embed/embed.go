// Package embed is Atlas's OPTIONAL, gated embedding layer for semantic_search.
// It is OFF by default: the deterministic lexical (BM25) core never touches it,
// and an engine only builds a Provider when vectors are explicitly enabled.
//
// Two implementations satisfy Provider:
//
//   - Hashing — a deterministic, fully OFFLINE feature-hash embedder over code
//     tokens (the same camel/snake/kebab split the lexical analyzer uses). It
//     makes NO external calls; its "similarity" is honest token-overlap, not a
//     learned semantic space. It is the default so `atlas index --enable-vectors`
//     and semantic-search work with zero infra and zero data egress.
//   - HTTP — POSTs to an OpenAI/Ollama-compatible embeddings endpoint named by
//     ATLAS_EMBED_URL, for a REAL embedding model when one is configured.
//
// NewProvider() returns HTTP when ATLAS_EMBED_URL is set, else Hashing.
package embed

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// Provider turns texts into fixed-dimension vectors. Every returned vector is L2
// normalized so a dot product equals cosine similarity (see store.NearestSymbols).
type Provider interface {
	// Embed returns one vector per input text, each of length Dim(). The vectors
	// are L2 normalized.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dim is the fixed dimensionality of every vector this provider emits.
	Dim() int
	// Name is a short, stable provider id for status/audit (e.g. "hashing").
	Name() string
}

// Environment knobs (all optional; absence keeps the offline default).
const (
	// EnvEmbedURL, when set, selects the HTTP provider and is the embeddings
	// endpoint to POST to (OpenAI: /v1/embeddings; Ollama: /api/embeddings).
	EnvEmbedURL = "ATLAS_EMBED_URL"
	// EnvEmbedModel is the model name sent in the request body (HTTP provider).
	EnvEmbedModel = "ATLAS_EMBED_MODEL"
	// EnvEmbedDim overrides the Hashing provider's dimension (default 256).
	EnvEmbedDim = "ATLAS_EMBED_DIM"
)

// defaultHashingDim is the Hashing provider's fixed dimensionality. 256 keeps the
// per-symbol BLOB small (1 KiB at float32) while leaving room for token overlap.
const defaultHashingDim = 256

// NewProvider is the factory the index/query paths use. It returns the HTTP
// provider when ATLAS_EMBED_URL is set (a real embedding model is configured),
// otherwise the deterministic, offline Hashing provider. It never fails: a
// missing/blank URL simply yields Hashing.
func NewProvider() Provider {
	if url := strings.TrimSpace(os.Getenv(EnvEmbedURL)); url != "" {
		return NewHTTP(url, strings.TrimSpace(os.Getenv(EnvEmbedModel)))
	}
	return NewHashing(hashingDimFromEnv())
}

// hashingDimFromEnv reads ATLAS_EMBED_DIM, falling back to the default for an
// unset or non-positive value.
func hashingDimFromEnv() int {
	if v := strings.TrimSpace(os.Getenv(EnvEmbedDim)); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultHashingDim
}
