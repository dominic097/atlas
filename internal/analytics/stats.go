package analytics

import "sort"

// Stats is a deterministic summary of the snapshot graph: raw totals plus
// breakdowns by edge kind and language, derived community count, and the number of
// isolated nodes (symbol names with no resolved in or out call edge).
type Stats struct {
	// Files is the count of distinct symbol Paths.
	Files int `json:"files"`
	// Symbols is the count of distinct symbol NAMES (the node count).
	Symbols int `json:"symbols"`
	// Edges is the count of resolved name-level "calls" edges in this graph
	// (deduped FromSymbol->ToRef pairs where both endpoints are known symbols).
	Edges int `json:"edges"`
	// RawSymbols / RawEdges are the unfiltered input counts, for transparency.
	RawSymbols int `json:"raw_symbols"`
	RawEdges   int `json:"raw_edges"`
	// EdgeKinds counts the RAW input edges by Kind (calls, imports, ...).
	EdgeKinds map[string]int `json:"edge_kinds"`
	// Languages counts distinct symbol NAMES by their representative language.
	Languages map[string]int `json:"languages"`
	// Communities is the number of detected communities.
	Communities int `json:"communities"`
	// IsolatedNodes is the count of nodes with no resolved in or out call edge.
	IsolatedNodes int `json:"isolated_nodes"`
}

// Stats computes the summary. It is a pure function of the Graph and is stable
// across runs. Map fields are freshly allocated; the caller may mutate them.
func (g *Graph) Stats() Stats {
	s := Stats{
		Symbols:    len(g.names),
		RawSymbols: len(g.symbols),
		RawEdges:   len(g.edges),
		EdgeKinds:  make(map[string]int),
		Languages:  make(map[string]int),
	}

	// Distinct files across all input symbols.
	files := make(map[string]struct{})
	for _, sym := range g.symbols {
		if sym.Path != "" {
			files[sym.Path] = struct{}{}
		}
	}
	s.Files = len(files)

	// Raw edge-kind breakdown + resolved name-level edge count.
	for _, e := range g.edges {
		s.EdgeKinds[string(e.Kind)]++
	}
	for _, callees := range g.out {
		s.Edges += len(callees)
	}

	// Language breakdown over the node set (one count per distinct symbol name,
	// using the representative symbol's language).
	for _, name := range g.names {
		lang := g.rep[name].Language
		if lang == "" {
			lang = "unknown"
		}
		s.Languages[lang]++
	}

	// Isolated nodes: no resolved in or out call edge.
	for _, name := range g.names {
		if len(g.in[name]) == 0 && len(g.out[name]) == 0 {
			s.IsolatedNodes++
		}
	}

	s.Communities = len(g.Communities())
	return s
}

// EdgeKindsSorted returns the edge-kind breakdown as a deterministically ordered
// slice (descending count, ties by kind name) — convenient for stable reporting
// without leaking Go map iteration order.
func (s Stats) EdgeKindsSorted() []KindCount {
	return sortedKindCounts(s.EdgeKinds)
}

// LanguagesSorted returns the language breakdown as a deterministically ordered
// slice (descending count, ties by language name).
func (s Stats) LanguagesSorted() []KindCount {
	return sortedKindCounts(s.Languages)
}

// KindCount is a (key, count) pair used for deterministic breakdown ordering.
type KindCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func sortedKindCounts(m map[string]int) []KindCount {
	out := make([]KindCount, 0, len(m))
	for k, v := range m {
		out = append(out, KindCount{Key: k, Count: v})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}
