// Package analytics implements DETERMINISTIC, LLM-FREE graph analytics over the
// snapshot's symbol call graph. Every analytic is a pure function of the indexed
// graph: no model calls, no randomness, no map-iteration-order dependence, and no
// heavy third-party deps (pure Go + stdlib). The same input always yields the same
// output — ties are broken by lexicographic symbol name everywhere.
//
// NODE MODEL. The graph is NAME-LEVEL (not per-definition), matching how graphify
// nodes work: a node per distinct symbol NAME. A directed edge FromSymbol -> ToRef
// is added for every edge whose Kind is graph.EdgeCalls and whose ToRef names a
// known symbol (external/unresolved refs whose ToRef is not in the symbol-name set
// are dropped). FromSymbol must also name a known symbol. This is intentionally
// simple and deterministic; precision-resolved per-identity impact lives in the
// query package, not here.
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

	"github.com/dominic097/atlas/internal/graph"
)

// Graph is the immutable node-level call graph built from a snapshot's symbols and
// call edges. Build it once with Build and call the accessors; it holds no mutable
// analysis state, so accessors are safe to call repeatedly and in any order.
type Graph struct {
	// names is the sorted set of distinct symbol names (the nodes).
	names []string
	// rep maps a symbol name to a representative CodeSymbol (the lexicographically
	// smallest ID among same-named symbols) for path/kind/language reporting.
	rep map[string]graph.CodeSymbol

	// out[a] = sorted distinct callee names reachable from a via a "calls" edge.
	out map[string][]string
	// in[b] = sorted distinct caller names that call b.
	in map[string][]string
	// undirected adjacency (union of in+out, no self loops), sorted & distinct.
	adj map[string][]string

	// raw inputs retained for Stats (distinct files / kinds / languages).
	symbols []graph.CodeSymbol
	edges   []graph.DependencyEdge
}

// Build constructs the deterministic node-level call graph. Nodes are distinct
// symbol NAMES; directed edges are the resolved "calls" edges (both endpoints must
// name a known symbol). Build copies what it needs from the inputs, so the caller's
// slices may be mutated afterward without affecting the Graph.
func Build(symbols []graph.CodeSymbol, edges []graph.DependencyEdge) *Graph {
	g := &Graph{
		rep:     make(map[string]graph.CodeSymbol),
		out:     make(map[string][]string),
		in:      make(map[string][]string),
		adj:     make(map[string][]string),
		symbols: append([]graph.CodeSymbol(nil), symbols...),
		edges:   append([]graph.DependencyEdge(nil), edges...),
	}

	// Node set = distinct symbol names. Representative = smallest-ID symbol so the
	// reported path/kind/language is deterministic across runs.
	nameSet := make(map[string]struct{})
	for _, s := range symbols {
		if s.Name == "" {
			continue
		}
		nameSet[s.Name] = struct{}{}
		if existing, ok := g.rep[s.Name]; !ok || s.ID < existing.ID {
			g.rep[s.Name] = s
		}
	}

	g.names = make([]string, 0, len(nameSet))
	for n := range nameSet {
		g.names = append(g.names, n)
	}
	sort.Strings(g.names)

	// Build directed adjacency from resolved "calls" edges. Dedupe with per-source
	// sets, then sort, so iteration order never leaks into the output.
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

	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			continue
		}
		from, to := e.FromSymbol, e.ToRef
		if from == "" || to == "" {
			continue
		}
		if _, ok := nameSet[from]; !ok {
			continue
		}
		if _, ok := nameSet[to]; !ok {
			// External / unresolved callee — drop it.
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

	g.out = sortedSetMap(outSet)
	g.in = sortedSetMap(inSet)
	g.adj = sortedSetMap(adjSet)
	return g
}

// sortedSetMap converts a map of sets into a map of sorted distinct slices.
func sortedSetMap(m map[string]map[string]struct{}) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, set := range m {
		vs := make([]string, 0, len(set))
		for v := range set {
			vs = append(vs, v)
		}
		sort.Strings(vs)
		out[k] = vs
	}
	return out
}

// Names returns the sorted node (symbol-name) set. The returned slice is a copy.
func (g *Graph) Names() []string {
	return append([]string(nil), g.names...)
}

// totalDegree returns the undirected total degree (in+out, counting a directed
// self-loop once) used for hub ranking.
func (g *Graph) totalDegree(name string) int {
	return len(g.in[name]) + len(g.out[name])
}
