// Package query implements pure, in-memory graph queries over a loaded
// snapshot — the slices of symbols and edges the engine passes in. It has no
// persistence dependency: callers load a snapshot from the StorageDriver and
// hand the symbol/edge slices here.
//
// The traversal idea is ported from the proven aziron-pulse engine
// (internal/service/code_search_service.go: GetCallers / GetReferences /
// GetImpactForChangedSymbols + blastRadius). Atlas edges are symbol-granular:
// an EdgeCalls edge has FromSymbol = the enclosing caller's bare name,
// FromFile = the caller's file, and ToRef = the bare callee name. So "callers
// of X" = the FromSymbol side of every EdgeCalls edge whose ToRef matches X's
// name, and the blast radius is a reverse-BFS from the changed symbols up the
// callee->caller direction.
package query

import (
	"sort"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// Callers returns the symbols that call the symbol(s) named `name`. A symbol is
// a caller when it is the FromSymbol of an EdgeCalls edge whose ToRef matches
// `name`. Results are deduped (by symbol identity) and sorted deterministically.
func Callers(edges []graph.DependencyEdge, syms []graph.CodeSymbol, name string) []graph.CodeSymbol {
	return callersByKinds(edges, syms, name, map[graph.EdgeKind]bool{graph.EdgeCalls: true})
}

// References returns the symbols that reference the symbol(s) named `name`,
// across both EdgeCalls and EdgeReferences edges (a superset of Callers).
func References(edges []graph.DependencyEdge, syms []graph.CodeSymbol, name string) []graph.CodeSymbol {
	return callersByKinds(edges, syms, name, map[graph.EdgeKind]bool{
		graph.EdgeCalls:      true,
		graph.EdgeReferences: true,
	})
}

// ImpactResult is the blast radius of a change: the names of the symbols whose
// behavior may be affected, the distinct files they live in, and the deepest
// reverse-BFS hop that contributed an impacted symbol.
type ImpactResult struct {
	ImpactedSymbols []string `json:"impacted_symbols"`
	ImpactedFiles   []string `json:"impacted_files"`
	DepthReached    int      `json:"depth_reached"`
}

// Impact computes the reverse blast radius of a change. The seed is every
// symbol that lives in one of changedPaths OR is named in changedSymbols. From
// the seed it walks the caller graph (callee ToRef -> caller FromSymbol) over
// EdgeCalls edges, up to maxDepth hops. It returns the impacted symbol names,
// the distinct impacted file paths, and the deepest hop reached.
//
// Name ambiguity is handled simply: edges match targets by name (case-fold),
// and every symbol/file is recorded once (deduped). The changed (seed) files
// are not themselves reported as impacted — only their upstream callers are.
//
// Ported from aziron-pulse GetImpactForChangedSymbols + blastRadiusGuardedCtx,
// with the cross-repo / route / risk machinery stripped (Atlas P0 in-memory).
func Impact(syms []graph.CodeSymbol, edges []graph.DependencyEdge, changedPaths, changedSymbols []string, maxDepth int) ImpactResult {
	if maxDepth < 1 {
		maxDepth = 1
	}

	// Build the seed set: symbols in a changed path OR named in changedSymbols.
	pathSet := make(map[string]bool, len(changedPaths))
	for _, p := range changedPaths {
		if p = canonicalPath(p); p != "" {
			pathSet[p] = true
		}
	}
	nameSet := make(map[string]bool, len(changedSymbols))
	for _, n := range changedSymbols {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			nameSet[n] = true
		}
	}

	var frontier []graph.CodeSymbol
	seedFiles := map[string]bool{}
	for _, s := range syms {
		matched := false
		if len(pathSet) > 0 && pathSet[canonicalPath(s.Path)] {
			matched = true
		}
		if !matched && nameSet[strings.ToLower(strings.TrimSpace(s.Name))] {
			matched = true
		}
		if matched {
			frontier = append(frontier, s)
			if fp := canonicalPath(s.Path); fp != "" {
				seedFiles[fp] = true
			}
		}
	}

	result := ImpactResult{
		ImpactedSymbols: []string{},
		ImpactedFiles:   []string{},
	}
	if len(frontier) == 0 {
		return result
	}

	// Index symbols by (lowercased) name and by file so each reverse-BFS hop can
	// (a) match callee names against the frontier and (b) expand a newly impacted
	// symbol's whole file into the next frontier.
	symsByName := make(map[string][]graph.CodeSymbol)
	symsByFile := make(map[string][]graph.CodeSymbol)
	for _, s := range syms {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if key != "" {
			symsByName[key] = append(symsByName[key], s)
		}
		if fp := canonicalPath(s.Path); fp != "" {
			symsByFile[fp] = append(symsByFile[fp], s)
		}
	}

	// Pre-index call edges by lowercased ToRef so each hop is O(frontier) lookups
	// rather than O(frontier * edges).
	callEdgesByRef := make(map[string][]graph.DependencyEdge)
	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			continue
		}
		ref := strings.ToLower(strings.TrimSpace(e.ToRef))
		if ref == "" {
			continue
		}
		callEdgesByRef[ref] = append(callEdgesByRef[ref], e)
	}

	impactedSymKeys := map[string]bool{} // symbol identity -> recorded
	impactedNames := map[string]bool{}   // distinct impacted symbol name -> present
	impactedFiles := map[string]bool{}   // distinct impacted file -> present
	depthReached := 0

	for d := 1; d <= maxDepth && len(frontier) > 0; d++ {
		// Distinct callee names present in the current frontier.
		frontierNames := map[string]bool{}
		for _, t := range frontier {
			if n := strings.ToLower(strings.TrimSpace(t.Name)); n != "" {
				frontierNames[n] = true
			}
		}

		var next []graph.CodeSymbol
		hopAddedSomething := false
		for name := range frontierNames {
			for _, e := range callEdgesByRef[name] {
				caller := resolveCaller(e, symsByName, symsByFile)
				if caller == nil {
					continue
				}
				fp := canonicalPath(caller.Path)
				if fp != "" && seedFiles[fp] {
					// Never report the changed files themselves as impacted; but
					// still let them expand outward (a seed file's callers count).
				}
				key := symbolKey(*caller)
				if impactedSymKeys[key] {
					continue
				}
				impactedSymKeys[key] = true
				hopAddedSomething = true
				if caller.Name != "" {
					impactedNames[caller.Name] = true
				}
				if fp != "" && !seedFiles[fp] {
					impactedFiles[fp] = true
				}
				// Seed the next hop with this caller ITSELF (not its whole file):
				// the BFS must walk callers-of-callers precisely. Expanding the
				// whole file pulls in every unrelated symbol declared alongside the
				// caller, which detonates the blast radius into the whole graph.
				next = append(next, *caller)
			}
		}
		if hopAddedSomething {
			depthReached = d
		}
		frontier = dedupeSymbols(next)
	}

	result.ImpactedSymbols = sortedKeys(impactedNames)
	result.ImpactedFiles = sortedKeys(impactedFiles)
	result.DepthReached = depthReached
	return result
}

// callersByKinds returns the caller symbols of every symbol named `name`,
// considering only edges of the supplied kinds. The caller is the edge's
// FromSymbol resolved back to a CodeSymbol (by file+name, falling back to name).
func callersByKinds(edges []graph.DependencyEdge, syms []graph.CodeSymbol, name string, kinds map[graph.EdgeKind]bool) []graph.CodeSymbol {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil
	}

	symsByName := make(map[string][]graph.CodeSymbol)
	symsByFile := make(map[string][]graph.CodeSymbol)
	for _, s := range syms {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if key != "" {
			symsByName[key] = append(symsByName[key], s)
		}
		if fp := canonicalPath(s.Path); fp != "" {
			symsByFile[fp] = append(symsByFile[fp], s)
		}
	}

	seen := map[string]bool{}
	var out []graph.CodeSymbol
	for _, e := range edges {
		if !kinds[e.Kind] {
			continue
		}
		if strings.ToLower(strings.TrimSpace(e.ToRef)) != target {
			continue
		}
		caller := resolveCaller(e, symsByName, symsByFile)
		if caller == nil {
			continue
		}
		key := symbolKey(*caller)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, *caller)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// resolveCaller maps an edge's caller side (FromFile + FromSymbol) back to a
// concrete CodeSymbol. It prefers the symbol that matches BOTH the caller's file
// and name (the precise hit), then falls back to any symbol with the caller's
// name, then to the first symbol declared in the caller's file. Returns nil when
// nothing resolves.
func resolveCaller(e graph.DependencyEdge, symsByName, symsByFile map[string][]graph.CodeSymbol) *graph.CodeSymbol {
	fromFile := canonicalPath(e.FromFile)
	fromName := strings.ToLower(strings.TrimSpace(e.FromSymbol))

	if fromName != "" {
		candidates := symsByName[fromName]
		// Prefer the same-file candidate (disambiguates a name defined in many files).
		for i := range candidates {
			if fromFile != "" && canonicalPath(candidates[i].Path) == fromFile {
				return &candidates[i]
			}
		}
		if len(candidates) > 0 {
			return &candidates[0]
		}
	}

	// No usable FromSymbol — fall back to the first symbol declared in the file.
	if fromFile != "" {
		if fileSyms := symsByFile[fromFile]; len(fileSyms) > 0 {
			return &fileSyms[0]
		}
	}
	return nil
}

// symbolKey is a stable identity for dedup: prefer the explicit ID/NodeID, else
// fall back to file+name+start-line.
func symbolKey(s graph.CodeSymbol) string {
	if s.ID != "" {
		return "id:" + s.ID
	}
	if s.NodeID != "" {
		return "node:" + string(s.NodeID)
	}
	return "fnl:" + canonicalPath(s.Path) + "\x00" + s.Name + "\x00" + itoa(s.StartLine)
}

func dedupeSymbols(in []graph.CodeSymbol) []graph.CodeSymbol {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		key := symbolKey(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

// canonicalPath normalizes a file path for comparison: trim space, drop a single
// leading "./", and convert backslashes to forward slashes.
func canonicalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return p
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
