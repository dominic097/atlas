package analytics

import "sort"

// Community is one detected cluster of densely-connected symbol identities.
type Community struct {
	// ID is assigned by DESCENDING size; ties broken by the smallest member display
	// name. IDs are stable across runs for the same input.
	ID int `json:"id"`
	// Members are the qualified DISPLAY names of the identities in the community,
	// sorted. Distinct same-named methods appear as distinct members.
	Members []string `json:"members"`
	// Size == len(Members).
	Size int `json:"size"`
	// Representatives are the highest-degree members (up to a few), descending by
	// total degree, ties by display name — the symbols that best characterize the
	// cluster.
	Representatives []string `json:"representatives"`
}

// maxLabelPropIterations bounds the deterministic label-propagation sweep.
const maxLabelPropIterations = 20

// maxRepresentatives caps how many representative members a Community reports.
const maxRepresentatives = 3

// Communities partitions the nodes into communities using DETERMINISTIC label
// propagation over the UNDIRECTED identity-level call graph (in+out adjacency
// unioned):
//
//   - Every node starts labeled with its own identity key.
//   - Nodes are swept in sorted-key order. Each node adopts the label that occurs
//     most among its neighbors; on a tie it picks the lexicographically smallest
//     label. (Including the node's own current label among the candidates keeps
//     singletons and 2-cliques stable instead of oscillating.)
//   - The sweep repeats until no label changes or maxLabelPropIterations is hit.
//
// Because the sweep order, tie-break, and iteration cap are all fixed, the
// partition is identical across runs. Isolated nodes (no neighbors) each form a
// singleton community. Returned communities are ordered by descending size (ties
// by smallest member display name) and get stable integer IDs from that order.
// Members and Representatives are reported as qualified DISPLAY names.
func (g *Graph) Communities() []Community {
	// label[key] = current community label (a node key).
	label := make(map[string]string, len(g.keys))
	for _, key := range g.keys {
		label[key] = key
	}

	for iter := 0; iter < maxLabelPropIterations; iter++ {
		changed := false
		for _, key := range g.keys {
			neighbors := g.adj[key]
			if len(neighbors) == 0 {
				continue // isolated node keeps its own label
			}
			best := dominantLabel(label, key, neighbors)
			if best != label[key] {
				label[key] = best
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Group node keys by final label.
	groups := make(map[string][]string)
	for _, key := range g.keys {
		l := label[key]
		groups[l] = append(groups[l], key)
	}

	communities := make([]Community, 0, len(groups))
	for _, memberKeys := range groups {
		g.sortKeys(memberKeys)
		members := make([]string, 0, len(memberKeys))
		for _, k := range memberKeys {
			members = append(members, g.label[k])
		}
		communities = append(communities, Community{
			Members:         members,
			Size:            len(members),
			Representatives: g.topMembersByDegree(memberKeys, maxRepresentatives),
		})
	}

	// Order by descending size, ties by smallest member display name (members
	// already sorted, so index 0 is the smallest). Then assign stable IDs.
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

// topMembersByDegree returns up to `limit` member DISPLAY names with the highest
// total degree, descending; ties broken by ascending display name, then path. The
// input is a slice of node KEYS.
func (g *Graph) topMembersByDegree(memberKeys []string, limit int) []string {
	ranked := append([]string(nil), memberKeys...)
	sort.SliceStable(ranked, func(i, j int) bool {
		di, dj := g.totalDegree(ranked[i]), g.totalDegree(ranked[j])
		if di != dj {
			return di > dj
		}
		li, lj := g.label[ranked[i]], g.label[ranked[j]]
		if li != lj {
			return li < lj
		}
		return g.rep[ranked[i]].Path < g.rep[ranked[j]].Path
	})
	if limit > 0 && limit < len(ranked) {
		ranked = ranked[:limit]
	}
	out := make([]string, 0, len(ranked))
	for _, k := range ranked {
		out = append(out, g.label[k])
	}
	return out
}
