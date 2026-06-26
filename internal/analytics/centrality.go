package analytics

import "sort"

// Hub is a high-degree node — a "god node" that many symbols call or that calls
// many symbols. Degrees are over the resolved name-level call graph.
type Hub struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Language    string `json:"language,omitempty"`
	InDegree    int    `json:"in_degree"`
	OutDegree   int    `json:"out_degree"`
	TotalDegree int    `json:"total_degree"`
}

// Degree reports the in/out/total degree of a single node. Unknown names report
// zero. Total is in+out (a directed self-loop counts once on each side).
func (g *Graph) Degree(name string) (in, out, total int) {
	in = len(g.in[name])
	out = len(g.out[name])
	return in, out, in + out
}

// Hubs returns the topN nodes by total degree, descending; ties broken by
// ascending name. A non-positive topN returns every node (still ranked). The
// result is deterministic and freshly allocated.
func (g *Graph) Hubs(topN int) []Hub {
	hubs := make([]Hub, 0, len(g.names))
	for _, name := range g.names {
		in, out, total := g.Degree(name)
		r := g.rep[name]
		hubs = append(hubs, Hub{
			Name:        name,
			Path:        r.Path,
			Kind:        r.Kind,
			Language:    r.Language,
			InDegree:    in,
			OutDegree:   out,
			TotalDegree: total,
		})
	}
	sort.SliceStable(hubs, func(i, j int) bool {
		if hubs[i].TotalDegree != hubs[j].TotalDegree {
			return hubs[i].TotalDegree > hubs[j].TotalDegree
		}
		return hubs[i].Name < hubs[j].Name
	})
	if topN > 0 && topN < len(hubs) {
		hubs = hubs[:topN]
	}
	return hubs
}

// PageRankScore is one node's deterministic PageRank importance.
type PageRankScore struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

// pageRankIterations and pageRankDamping are fixed so PageRank is fully
// deterministic — no convergence threshold, no randomness.
const (
	pageRankIterations = 20
	pageRankDamping    = 0.85
)

// PageRank computes a deterministic PageRank over the DIRECTED call graph: a fixed
// 20 iterations at damping 0.85, with dangling-node mass redistributed uniformly.
// Because the node order, iteration count, and arithmetic are fixed, the result is
// bit-for-bit stable across runs. Scores sum to ~1.0 across all nodes.
func (g *Graph) PageRank() []PageRankScore {
	n := len(g.names)
	out := make([]PageRankScore, 0, n)
	if n == 0 {
		return out
	}

	// Index nodes by their position in the sorted name list for stable iteration.
	idx := make(map[string]int, n)
	for i, name := range g.names {
		idx[name] = i
	}

	rank := make([]float64, n)
	init := 1.0 / float64(n)
	for i := range rank {
		rank[i] = init
	}

	// Precompute out-degree per node (over the directed graph).
	outDeg := make([]int, n)
	for i, name := range g.names {
		outDeg[i] = len(g.out[name])
	}

	base := (1.0 - pageRankDamping) / float64(n)
	next := make([]float64, n)
	for iter := 0; iter < pageRankIterations; iter++ {
		// Dangling mass: nodes with no out-edges spread their rank uniformly.
		var dangling float64
		for i := 0; i < n; i++ {
			if outDeg[i] == 0 {
				dangling += rank[i]
			}
		}
		danglingShare := pageRankDamping * dangling / float64(n)

		for i := 0; i < n; i++ {
			next[i] = base + danglingShare
		}
		// Iterate nodes in sorted order; for each, push rank to its sorted callees.
		for i, name := range g.names {
			if outDeg[i] == 0 {
				continue
			}
			share := pageRankDamping * rank[i] / float64(outDeg[i])
			for _, callee := range g.out[name] {
				next[idx[callee]] += share
			}
		}
		rank, next = next, rank
	}

	for i, name := range g.names {
		out = append(out, PageRankScore{Name: name, Score: rank[i]})
	}
	return out
}

// TopByPageRank returns the topN nodes by PageRank score, descending; ties broken
// by ascending name. A non-positive topN returns every node (ranked).
func (g *Graph) TopByPageRank(topN int) []PageRankScore {
	scores := g.PageRank()
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].Name < scores[j].Name
	})
	if topN > 0 && topN < len(scores) {
		scores = scores[:topN]
	}
	return scores
}
