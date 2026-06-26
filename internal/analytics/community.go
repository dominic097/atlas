package analytics

import "sort"

// Community is one detected cluster of densely-connected symbol names.
type Community struct {
	// ID is assigned by DESCENDING size; ties broken by the smallest member name.
	// IDs are stable across runs for the same input.
	ID int `json:"id"`
	// Members are the symbol names in the community, sorted.
	Members []string `json:"members"`
	// Size == len(Members).
	Size int `json:"size"`
	// Representatives are the highest-degree members (up to a few), descending by
	// total degree, ties by name — the symbols that best characterize the cluster.
	Representatives []string `json:"representatives"`
}

// maxLabelPropIterations bounds the deterministic label-propagation sweep.
const maxLabelPropIterations = 20

// maxRepresentatives caps how many representative members a Community reports.
const maxRepresentatives = 3

// Communities partitions the nodes into communities using DETERMINISTIC label
// propagation over the UNDIRECTED call graph (in+out adjacency unioned):
//
//   - Every node starts labeled with its own name.
//   - Nodes are swept in sorted-name order. Each node adopts the label that occurs
//     most among its neighbors; on a tie it picks the lexicographically smallest
//     label. (Including the node's own current label among the candidates keeps
//     singletons and 2-cliques stable instead of oscillating.)
//   - The sweep repeats until no label changes or maxLabelPropIterations is hit.
//
// Because the sweep order, tie-break, and iteration cap are all fixed, the
// partition is identical across runs. Isolated nodes (no neighbors) each form a
// singleton community. Returned communities are ordered by descending size (ties
// by smallest member name) and get stable integer IDs from that order.
func (g *Graph) Communities() []Community {
	// label[name] = current community label (a node name).
	label := make(map[string]string, len(g.names))
	for _, name := range g.names {
		label[name] = name
	}

	for iter := 0; iter < maxLabelPropIterations; iter++ {
		changed := false
		for _, name := range g.names {
			neighbors := g.adj[name]
			if len(neighbors) == 0 {
				continue // isolated node keeps its own label
			}
			best := dominantLabel(label, name, neighbors)
			if best != label[name] {
				label[name] = best
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Group nodes by final label.
	groups := make(map[string][]string)
	for _, name := range g.names {
		l := label[name]
		groups[l] = append(groups[l], name)
	}

	communities := make([]Community, 0, len(groups))
	for _, members := range groups {
		sort.Strings(members)
		communities = append(communities, Community{
			Members:         members,
			Size:            len(members),
			Representatives: g.topMembersByDegree(members, maxRepresentatives),
		})
	}

	// Order by descending size, ties by smallest member name (members already
	// sorted, so index 0 is the smallest). Then assign stable IDs.
	sort.SliceStable(communities, func(i, j int) bool {
		if communities[i].Size != communities[j].Size {
			return communities[i].Size > communities[j].Size
		}
		return communities[i].Members[0] < communities[j].Members[0]
	})
	for i := range communities {
		communities[i].ID = i
	}
	return communities
}

// dominantLabel returns the label to adopt for `self`: the most frequent label
// among its neighbors plus its own current label, ties broken by smallest label.
func dominantLabel(label map[string]string, self string, neighbors []string) string {
	counts := make(map[string]int, len(neighbors)+1)
	// Include the node's own current label so stable pairs don't oscillate.
	counts[label[self]]++
	for _, nb := range neighbors {
		counts[label[nb]]++
	}

	// Deterministic argmax: iterate candidate labels in sorted order.
	cands := make([]string, 0, len(counts))
	for l := range counts {
		cands = append(cands, l)
	}
	sort.Strings(cands)

	best := cands[0]
	bestCount := counts[best]
	for _, l := range cands[1:] {
		if counts[l] > bestCount {
			best, bestCount = l, counts[l]
		}
	}
	return best
}

// topMembersByDegree returns up to `limit` members with the highest total degree,
// descending; ties broken by ascending name.
func (g *Graph) topMembersByDegree(members []string, limit int) []string {
	ranked := append([]string(nil), members...)
	sort.SliceStable(ranked, func(i, j int) bool {
		di, dj := g.totalDegree(ranked[i]), g.totalDegree(ranked[j])
		if di != dj {
			return di > dj
		}
		return ranked[i] < ranked[j]
	})
	if limit > 0 && limit < len(ranked) {
		ranked = ranked[:limit]
	}
	return ranked
}
