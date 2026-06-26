// Package analytics implements DETERMINISTIC, LLM-FREE graph analytics over the
// snapshot's symbol call graph. Every analytic is a pure function of the indexed
// graph: no model calls, no randomness, no map-iteration-order dependence, and no
// heavy third-party deps (pure Go + stdlib). The same input always yields the same
// output — ties are broken by the qualified display name (and path) everywhere.
//
// NODE MODEL — IDENTITY-AWARE. The graph is keyed by SYMBOL IDENTITY (one node per
// DEFINITION), not by bare name. Two distinct definitions that merely share a name
// — e.g. every type's own Close method — are DISTINCT nodes, each with its own
// per-identity degree. This is the precision atlas already uses for impact/callers
// (a name-level graph collapses all "Close" methods into one fake god-node). A node
// carries a human DISPLAY label that qualifies the bare name with the receiver type
// when present ("localEngine.Close") plus the defining path, so distinct same-named
// methods are distinguishable in hubs / communities / report output.
//
// EDGE MODEL. A directed edge caller -> target is added for every EdgeCalls edge
// that RESOLVES to an in-repo target symbol via query.ResolveCallEdges, using the
// SAME signals query.resolveTargets uses (qualified_ref, recv_type, package/path,
// in-repo-type interface dispatch). Edges whose target leaves the repo (stdlib,
// third-party) resolve to nothing and are dropped, exactly as in impact/callers.
//
// USAGE:
//
//	g := analytics.Build(symbols, edges)
//	communities := g.Communities()  // []Community, ids stable & sorted by size
//	hubs        := g.Hubs(10)        // []Hub, top-N by total degree ("god nodes")
//	imp         := g.TopByPageRank(10) // []PageRankScore, deterministic PageRank
//	stats       := g.Stats()         // Stats: totals + breakdowns
//
// All slice results are freshly allocated and independently sorted, so callers may
// mutate them without affecting the Graph. Calling any accessor twice on the same
// Graph (or two Graphs built from equal input) yields deep-equal results.
package analytics

import (
	"sort"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/query"
)

// Graph is the immutable, IDENTITY-LEVEL call graph built from a snapshot's
// symbols and call edges. Build it once with Build and call the accessors; it
// holds no mutable analysis state, so accessors are safe to call repeatedly and in
// any order. Internally every node is keyed by a stable per-definition identity
// key (query.SymbolKey); the human display label is carried alongside.
type Graph struct {
	// keys is the sorted set of node identity keys (the nodes). Sorted by the node's
	// qualified display label, then by key, so iteration order is deterministic and
	// human-meaningful.
	keys []string
	// rep maps a node key to its defining CodeSymbol (path/kind/language reporting).
	rep map[string]graph.CodeSymbol
	// label maps a node key to its qualified human display name (e.g.
	// "localEngine.Close"). Two distinct Close methods get distinct keys but may
	// share a label only when on the same type+name in different files — disambiguated
	// further by path in the projection layer.
	label map[string]string

	// out[a] = sorted distinct callee KEYS reachable from a via a "calls" edge.
	out map[string][]string
	// in[b] = sorted distinct caller KEYS that call b.
	in map[string][]string
	// undirected adjacency (union of in+out, no self loops), sorted & distinct, by KEY.
	adj map[string][]string

	// raw inputs retained for Stats (distinct files / kinds / languages).
	symbols []graph.CodeSymbol
	edges   []graph.DependencyEdge
}

// Build constructs the deterministic IDENTITY-LEVEL call graph. Nodes are distinct
// symbol DEFINITIONS (keyed by query.SymbolKey); directed edges are the resolved
// "calls" edges, with each edge's callee resolved to the concrete in-repo target
// IDENTITY via query.ResolveCallEdges (external targets dropped). Build copies what
// it needs from the inputs, so the caller's slices may be mutated afterward without
// affecting the Graph.
func Build(symbols []graph.CodeSymbol, edges []graph.DependencyEdge) *Graph {
	g := &Graph{
		rep:     make(map[string]graph.CodeSymbol),
		label:   make(map[string]string),
		out:     make(map[string][]string),
		in:      make(map[string][]string),
		adj:     make(map[string][]string),
		symbols: append([]graph.CodeSymbol(nil), symbols...),
		edges:   append([]graph.DependencyEdge(nil), edges...),
	}

	// Node set = distinct symbol identities. Each definition is its own node, so a
	// name shared across types never collapses. The representative is the symbol
	// itself (keyed identity), and the display label qualifies the name.
	for _, s := range symbols {
		if s.Name == "" {
			continue
		}
		k := query.SymbolKey(s)
		if _, ok := g.rep[k]; !ok {
			g.rep[k] = s
			g.label[k] = qualifiedLabel(s)
		}
	}

	g.keys = make([]string, 0, len(g.rep))
	for k := range g.rep {
		g.keys = append(g.keys, k)
	}
	g.sortKeys(g.keys)

	// Build directed adjacency from RESOLVED call edges (per-identity). Dedupe with
	// per-source sets, then sort, so iteration order never leaks into the output.
	outSet := make(map[string]map[string]struct{})
	inSet := make(map[string]map[string]struct{})
	adjSet := make(map[string]map[string]struct{})
	add := func(m map[string]map[string]struct{}, k, v string) {
		s := m[k]
		if s == nil {
			s = make(map[string]struct{})
			m[k] = s
		}
		s[v] = struct{}{}
	}

	for _, re := range query.ResolveCallEdges(symbols, edges) {
		from := query.SymbolKey(re.Caller)
		to := query.SymbolKey(re.Target)
		// Both endpoints must be nodes (named symbols). A resolved target always is;
		// guard the caller (it could be an unnamed file-level fallback resolution).
		if _, ok := g.rep[from]; !ok {
			continue
		}
		if _, ok := g.rep[to]; !ok {
			continue
		}
		if from == to {
			// Self-recursion is a directed self-edge but not an undirected one.
			add(outSet, from, to)
			add(inSet, to, from)
			continue
		}
		add(outSet, from, to)
		add(inSet, to, from)
		add(adjSet, from, to)
		add(adjSet, to, from)
	}

	g.out = g.sortedSetMap(outSet)
	g.in = g.sortedSetMap(inSet)
	g.adj = g.sortedSetMap(adjSet)
	return g
}

// qualifiedLabel renders a symbol's human display name: "recvType.Name" for a
// method whose receiver type is known (the keystone disambiguation — two Close
// methods read as "localEngine.Close" vs "bleveIndex.Close"), else the bare Name.
func qualifiedLabel(s graph.CodeSymbol) string {
	if rt := symbolRecvType(s); rt != "" {
		return rt + "." + s.Name
	}
	return s.Name
}

// symbolRecvType returns the trimmed Metadata["recv_type"] of a symbol — the base
// type a method is declared on, or "" when absent.
func symbolRecvType(s graph.CodeSymbol) string {
	if s.Metadata == nil {
		return ""
	}
	rt, _ := s.Metadata["recv_type"].(string)
	return strings.TrimSpace(rt)
}

// sortKeys orders node keys deterministically by qualified display label, then by
// the underlying identity key (so same-labeled definitions in different files have
// a stable, reproducible order).
func (g *Graph) sortKeys(keys []string) {
	sort.SliceStable(keys, func(i, j int) bool {
		li, lj := g.label[keys[i]], g.label[keys[j]]
		if li != lj {
			return li < lj
		}
		pi, pj := g.rep[keys[i]].Path, g.rep[keys[j]].Path
		if pi != pj {
			return pi < pj
		}
		return keys[i] < keys[j]
	})
}

// sortedSetMap converts a map of key-sets into a map of slices sorted by the same
// deterministic node order (display label, then path, then key).
func (g *Graph) sortedSetMap(m map[string]map[string]struct{}) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, set := range m {
		vs := make([]string, 0, len(set))
		for v := range set {
			vs = append(vs, v)
		}
		g.sortKeys(vs)
		out[k] = vs
	}
	return out
}

// Names returns the sorted node DISPLAY labels (qualified names). The returned
// slice is a copy. Labels may repeat when two definitions share type+name across
// files; callers needing uniqueness should use the per-identity accessors.
func (g *Graph) Names() []string {
	out := make([]string, 0, len(g.keys))
	for _, k := range g.keys {
		out = append(out, g.label[k])
	}
	return out
}

// totalDegree returns the undirected total degree (in+out, counting a directed
// self-loop once) of a node KEY, used for hub ranking.
func (g *Graph) totalDegree(key string) int {
	return len(g.in[key]) + len(g.out[key])
}
