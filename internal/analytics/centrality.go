package analytics

import "sort"

// Hub is a high-degree node — a "god node" that many symbols call or that calls
// many symbols. Degrees are over the resolved IDENTITY-level call graph, so each
// distinct definition (e.g. each type's own Close) has its own per-identity degree
// rather than collapsing every same-named method into one inflated node.
type Hub struct {
	// Name is the qualified DISPLAY name ("localEngine.Close") — the receiver type
	// plus the method name when known, else the bare name. Distinct same-named
	// methods are distinguishable here (and fully by Path below).
	Name string `json:"name"`
	// BareName is the unqualified symbol name ("Close"), retained for callers that
	// key off the raw name.
	BareName    string `json:"bare_name,omitempty"`
	Path        string `json:"path,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Language    string `json:"language,omitempty"`
	InDegree    int    `json:"in_degree"`
	OutDegree   int    `json:"out_degree"`
	TotalDegree int    `json:"total_degree"`
}

// degreeByKey reports the in/out/total degree of a single node KEY.
func (g *Graph) degreeByKey(key string) (in, out, total int) {
	in = len(g.in[key])
	out = len(g.out[key])
	return in, out, in + out
}

// Degree reports the in/out/total degree of the node(s) whose qualified display
// label or bare name equals `name`. When several distinct definitions match (e.g.
// the bare name "Close"), their degrees are SUMMED — so the name-level view is
// still available, but the identity split lives in Hubs/Communities. Unknown names
// report zero. Total is in+out (a directed self-loop counts once on each side).
func (g *Graph) Degree(name string) (in, out, total int) {
	for _, k := range g.keys {
		if g.label[k] == name || g.rep[k].Name == name {
			ki, ko, _ := g.degreeByKey(k)
			in += ki
			out += ko
		}
	}
	return in, out, in + out
}

// Hubs returns the topN nodes by total degree, descending; ties broken by
// ascending qualified display name, then path. A non-positive topN returns every
// node (still ranked). The result is deterministic and freshly allocated. Each hub
// is a distinct IDENTITY, so two same-named methods on different types appear as
// two rows with their own (smaller, real) degrees.
func (g *Graph) Hubs(topN int) []Hub {
	hubs := make([]Hub, 0, len(g.keys))
	for _, key := range g.keys {
		in, out, total := g.degreeByKey(key)
		r := g.rep[key]
		hubs = append(hubs, Hub{
			Name:        g.label[key],
			BareName:    r.Name,
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
		if hubs[i].Name != hubs[j].Name {
			return hubs[i].Name < hubs[j].Name
		}
		return hubs[i].Path < hubs[j].Path
	})
	if topN > 0 && topN < len(hubs) {
		hubs = hubs[:topN]
	}
	return hubs
}

// PageRankScore is one node's deterministic PageRank importance, carried by its
// qualified display name.
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

// PageRank computes a deterministic PageRank over the DIRECTED identity-level call
// graph: a fixed 20 iterations at damping 0.85, with dangling-node mass
// redistributed uniformly. Because the node order, iteration count, and arithmetic
// are fixed, the result is bit-for-bit stable across runs. Scores sum to ~1.0
// across all nodes. Scores are reported by qualified display name.
func (g *Graph) PageRank() []PageRankScore {
	n := len(g.keys)
	out := make([]PageRankScore, 0, n)
	if n == 0 {
		return out
	}

	// Index nodes by their position in the sorted key list for stable iteration.
	idx := make(map[string]int, n)
	for i, key := range g.keys {
		idx[key] = i
	}

	rank := make([]float64, n)
	init := 1.0 / float64(n)
	for i := range rank {
		rank[i] = init
	}

	// Precompute out-degree per node (over the directed graph).
	outDeg := make([]int, n)
	for i, key := range g.keys {
		outDeg[i] = len(g.out[key])
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
		for i, key := range g.keys {
			if outDeg[i] == 0 {
				continue
			}
			share := pageRankDamping * rank[i] / float64(outDeg[i])
			for _, callee := range g.out[key] {
				next[idx[callee]] += share
			}
		}
		rank, next = next, rank
	}

	for i, key := range g.keys {
		out = append(out, PageRankScore{Name: g.label[key], Score: rank[i]})
	}
	return out
}

// TopByPageRank returns the topN nodes by PageRank score, descending; ties broken
// by ascending display name. A non-positive topN returns every node (ranked).
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
