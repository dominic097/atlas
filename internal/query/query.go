// Package query implements pure, in-memory graph queries over a loaded
// snapshot — the slices of symbols and edges the engine passes in. It has no
// persistence dependency: callers load a snapshot from the StorageDriver and
// hand the symbol/edge slices here.
//
// Call edges are symbol-granular: an EdgeCalls edge has FromSymbol = the
// enclosing caller's bare name, FromFile = the caller's file, ToRef = the bare
// callee name, and (for Go) Metadata["qualified_ref"] = the qualified callee
// like "parser.Parse" / "time.Parse" / "p.Parse".
//
// PRECISION: matching callers by bare name alone collides unrelated symbols
// (our parser.Parse vs stdlib time.Parse). So every call edge is RESOLVED to the
// indexed symbol(s) it actually targets using the qualifier + package/method
// rules in resolveTargets — an edge to an external symbol (time.Parse) resolves
// to nothing and is dropped. Impact is then a reverse-BFS over resolved symbol
// identities, not names. (The cross-repo / route / risk machinery is Pulse.)
package query

import (
	"sort"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// Callers returns the symbols that call the symbol(s) named `name` — i.e. the
// FromSymbol of every EdgeCalls edge that RESOLVES to an indexed symbol named
// `name`. Deduped by symbol identity, sorted deterministically.
func Callers(edges []graph.DependencyEdge, syms []graph.CodeSymbol, name string) []graph.CodeSymbol {
	return callersMatching(edges, syms, name, map[graph.EdgeKind]bool{graph.EdgeCalls: true})
}

// References is Callers across both EdgeCalls and EdgeReferences edges.
func References(edges []graph.DependencyEdge, syms []graph.CodeSymbol, name string) []graph.CodeSymbol {
	return callersMatching(edges, syms, name, map[graph.EdgeKind]bool{
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

// Impact computes the reverse blast radius of a change. The seed is every symbol
// in a changedPath OR named in changedSymbols. It walks the RESOLVED caller graph
// (resolved callee identity -> caller) up to maxDepth hops, over symbol
// identities (not names), so a call to an external same-named symbol never
// connects. Returns impacted symbol names, distinct impacted files, and the
// deepest hop reached. Seed files are not reported as impacted — only callers.
func Impact(syms []graph.CodeSymbol, edges []graph.DependencyEdge, changedPaths, changedSymbols []string, maxDepth int) ImpactResult {
	if maxDepth < 1 {
		maxDepth = 1
	}

	pathSet := map[string]bool{}
	for _, p := range changedPaths {
		if p = canonicalPath(p); p != "" {
			pathSet[p] = true
		}
	}
	nameSet := map[string]bool{}
	for _, n := range changedSymbols {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			nameSet[n] = true
		}
	}

	symsByName, symsByFile := indexSymbols(syms)

	// Seed identities + seed files.
	seed := map[string]bool{}
	seedFiles := map[string]bool{}
	for i := range syms {
		s := syms[i]
		if (len(pathSet) > 0 && pathSet[canonicalPath(s.Path)]) ||
			nameSet[strings.ToLower(strings.TrimSpace(s.Name))] {
			seed[symbolKey(s)] = true
			if fp := canonicalPath(s.Path); fp != "" {
				seedFiles[fp] = true
			}
		}
	}
	result := ImpactResult{ImpactedSymbols: []string{}, ImpactedFiles: []string{}}
	if len(seed) == 0 {
		return result
	}

	// Reverse adjacency: resolved-target-identity -> caller symbols.
	radj := map[string][]graph.CodeSymbol{}
	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			continue
		}
		caller := resolveCaller(e, symsByName, symsByFile)
		if caller == nil {
			continue
		}
		for _, t := range resolveTargets(e, symsByName) {
			tk := symbolKey(t)
			radj[tk] = append(radj[tk], *caller)
		}
	}

	impactedNames := map[string]bool{}
	impactedFiles := map[string]bool{}
	visited := map[string]bool{}
	for k := range seed {
		visited[k] = true
	}
	frontier := make([]string, 0, len(seed))
	for k := range seed {
		frontier = append(frontier, k)
	}

	depthReached := 0
	for d := 1; d <= maxDepth && len(frontier) > 0; d++ {
		var next []string
		for _, tk := range frontier {
			for _, caller := range radj[tk] {
				ck := symbolKey(caller)
				if visited[ck] {
					continue
				}
				visited[ck] = true
				depthReached = d
				if caller.Name != "" {
					impactedNames[caller.Name] = true
				}
				if fp := canonicalPath(caller.Path); fp != "" && !seedFiles[fp] {
					impactedFiles[fp] = true
				}
				next = append(next, ck)
			}
		}
		frontier = next
	}

	result.ImpactedSymbols = sortedKeys(impactedNames)
	result.ImpactedFiles = sortedKeys(impactedFiles)
	result.DepthReached = depthReached
	return result
}

// resolveTargets returns the indexed symbol(s) an EdgeCalls edge plausibly
// targets, using the qualified_ref to disambiguate name collisions:
//   - callee name not indexed at all            -> external, nothing
//   - unqualified call (foo())                   -> same-package candidate(s), else any
//   - qualified pkg.Foo() where pkg is a package -> that package's candidate(s)
//   - qualified x.Foo() with no package match    -> indexed METHOD(s) named Foo
//     (a method call on a value), else external (dropped)
func resolveTargets(e graph.DependencyEdge, symsByName map[string][]graph.CodeSymbol) []graph.CodeSymbol {
	bare := strings.ToLower(strings.TrimSpace(e.ToRef))
	if bare == "" {
		return nil
	}
	cands := symsByName[bare]
	if len(cands) == 0 {
		return nil // target is not an indexed symbol (e.g. stdlib)
	}
	qualifier := edgeQualifier(e)
	if qualifier == "" {
		callerPkg := pkgDir(e.FromFile)
		var local []graph.CodeSymbol
		for _, c := range cands {
			if pkgDir(c.Path) == callerPkg {
				local = append(local, c)
			}
		}
		if len(local) > 0 {
			return local
		}
		return cands // best-effort when we can't localize
	}
	var pkgMatch, methods []graph.CodeSymbol
	for _, c := range cands {
		if pkgBase(c.Path) == qualifier {
			pkgMatch = append(pkgMatch, c)
		}
		if strings.EqualFold(c.Kind, "method") {
			methods = append(methods, c)
		}
	}
	if len(pkgMatch) > 0 {
		return pkgMatch
	}
	if len(methods) > 0 {
		return methods // method call on a variable of unknown type
	}
	return nil // qualified, non-method, non-matching package -> external
}

// edgeQualifier returns the lowercased segment immediately before the final name
// in Metadata["qualified_ref"] ("parser.Parse" -> "parser", "r.Context" -> "r"),
// or "" when the call is unqualified or carries no qualified_ref.
func edgeQualifier(e graph.DependencyEdge) string {
	if e.Metadata == nil {
		return ""
	}
	qr, _ := e.Metadata["qualified_ref"].(string)
	qr = strings.TrimSpace(qr)
	if qr == "" {
		return ""
	}
	parts := strings.Split(qr, ".")
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[len(parts)-2]))
}

// callersMatching returns caller symbols of every indexed symbol named `name`,
// considering only edges of the supplied kinds and only when the edge RESOLVES
// to a symbol named `name`.
func callersMatching(edges []graph.DependencyEdge, syms []graph.CodeSymbol, name string, kinds map[graph.EdgeKind]bool) []graph.CodeSymbol {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil
	}
	symsByName, symsByFile := indexSymbols(syms)

	seen := map[string]bool{}
	var out []graph.CodeSymbol
	for _, e := range edges {
		if !kinds[e.Kind] {
			continue
		}
		if strings.ToLower(strings.TrimSpace(e.ToRef)) != target {
			continue
		}
		// For EdgeCalls, demand a real resolution to a symbol named `target`.
		if e.Kind == graph.EdgeCalls {
			resolved := false
			for _, t := range resolveTargets(e, symsByName) {
				if strings.EqualFold(t.Name, name) {
					resolved = true
					break
				}
			}
			if !resolved {
				continue
			}
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

func indexSymbols(syms []graph.CodeSymbol) (byName, byFile map[string][]graph.CodeSymbol) {
	byName = make(map[string][]graph.CodeSymbol)
	byFile = make(map[string][]graph.CodeSymbol)
	for i := range syms {
		s := syms[i]
		if key := strings.ToLower(strings.TrimSpace(s.Name)); key != "" {
			byName[key] = append(byName[key], s)
		}
		if fp := canonicalPath(s.Path); fp != "" {
			byFile[fp] = append(byFile[fp], s)
		}
	}
	return byName, byFile
}

// resolveCaller maps an edge's caller side (FromFile + FromSymbol) back to a
// concrete CodeSymbol: prefer same file+name, then any same-name, then the first
// symbol in the file. Returns nil when nothing resolves.
func resolveCaller(e graph.DependencyEdge, symsByName, symsByFile map[string][]graph.CodeSymbol) *graph.CodeSymbol {
	fromFile := canonicalPath(e.FromFile)
	fromName := strings.ToLower(strings.TrimSpace(e.FromSymbol))
	if fromName != "" {
		candidates := symsByName[fromName]
		for i := range candidates {
			if fromFile != "" && canonicalPath(candidates[i].Path) == fromFile {
				return &candidates[i]
			}
		}
		if len(candidates) > 0 {
			return &candidates[0]
		}
	}
	if fromFile != "" {
		if fileSyms := symsByFile[fromFile]; len(fileSyms) > 0 {
			return &fileSyms[0]
		}
	}
	return nil
}

// pkgDir is the directory portion of a path: the Go-style "package" key.
// "internal/parser/parser.go" -> "internal/parser".
func pkgDir(p string) string {
	p = canonicalPath(p)
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// pkgBase is the last directory component (lowercased) — the bareword a
// qualified call would use. "internal/parser/parser.go" -> "parser".
func pkgBase(p string) string {
	d := pkgDir(p)
	if i := strings.LastIndex(d, "/"); i >= 0 {
		return strings.ToLower(d[i+1:])
	}
	return strings.ToLower(d)
}

// symbolKey is a stable identity for dedup: prefer ID, then NodeID, else
// file+name+start-line.
func symbolKey(s graph.CodeSymbol) string {
	if s.ID != "" {
		return "id:" + s.ID
	}
	if s.NodeID != "" {
		return "node:" + string(s.NodeID)
	}
	return "fnl:" + canonicalPath(s.Path) + "\x00" + s.Name + "\x00" + itoa(s.StartLine)
}

// canonicalPath normalizes a file path for comparison.
func canonicalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.TrimPrefix(p, "./")
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
