package store

import (
	"encoding/binary"
	"math"
	"sort"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// embeddingsCols is the column list shared by both drivers' embedding I/O.
const embeddingsCols = `snapshot_id, symbol_id, dim, vec`

// encodeVector serializes a []float32 to a little-endian byte blob (4 bytes per
// element). It is the storage encoding for the embeddings.vec column on both
// tiers (BLOB on sqlite, BYTEA on postgres).
func encodeVector(vec []float32) []byte {
	buf := make([]byte, 4*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector reverses encodeVector. A blob whose length is not a multiple of 4
// is truncated to the largest whole-float prefix (defensive; never expected).
func decodeVector(blob []byte) []float32 {
	n := len(blob) / 4
	vec := make([]float32, n)
	for i := 0; i < n; i++ {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec
}

// dotProduct is the cosine similarity of two EQUAL-LENGTH L2-normalized vectors
// (cosine == dot when both are unit length). Length-mismatched inputs (a stale
// embedding from a different model/dim) score 0 — they simply never rank.
func dotProduct(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// rankEmbeddings scores every (symbolID, vector) row against query by cosine,
// filters Score>=minScore, sorts descending (ties broken by symbol id for
// determinism) and returns the top-`limit`.
//
// This is a BRUTE-FORCE scan in Go — acceptable for both tiers at Atlas's
// per-snapshot symbol counts (an ANN index is a future optimization, not needed
// for correctness).
func rankEmbeddings(query []float32, ids []string, vecs [][]float32, limit int, minScore float64) []graph.ScoredSymbol {
	scored := make([]graph.ScoredSymbol, 0, len(ids))
	for i := range ids {
		s := dotProduct(query, vecs[i])
		if s < minScore {
			continue
		}
		scored = append(scored, graph.ScoredSymbol{SymbolID: ids[i], Score: s})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].SymbolID < scored[j].SymbolID
		}
		return scored[i].Score > scored[j].Score
	})
	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}
