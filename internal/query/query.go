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
	"context"
	"sort"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
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
	repoType := repoTypePredicate(syms)
	radj := map[string][]graph.CodeSymbol{}
	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			continue
		}
		caller := resolveCaller(e, symsByName, symsByFile)
		if caller == nil {
			continue
		}
		for _, t := range resolveTargets(e, symsByName, repoType) {
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

// ImpactGraph computes the SAME reverse blast radius as Impact, but driven by
// INDEXED store queries so a query touches only the blast radius — never the
// whole snapshot. It resolves the seed via SymbolsByPath/SymbolsByName, then per
// hop collects the distinct callee names of the current frontier, fetches only
// the matching call edges via CallEdgesByToRefs, and resolves each edge's
// targets/caller with the SAME resolveTargets/resolveCaller logic Impact uses
// (so the receiver-type precision fix applies identically). Symbols fetched by
// name are cached so each name is queried at most once across the whole BFS.
func ImpactGraph(ctx context.Context, drv store.StorageDriver, snapshotID string, changedPaths, changedSymbols []string, maxDepth int) (ImpactResult, error) {
	if maxDepth < 1 {
		maxDepth = 1
	}
	result := ImpactResult{ImpactedSymbols: []string{}, ImpactedFiles: []string{}}

	cache := newSymbolCache(ctx, drv, snapshotID)

	// Seed identities + seed files, resolved through indexed reads only.
	seed := map[string]bool{}      // symbol identity -> true
	seedFiles := map[string]bool{} // canonical seed file path -> true
	// frontierSyms holds the actual CodeSymbols on the current frontier so we can
	// read their (callee) Name to find callers in the next hop.
	var frontierSyms []graph.CodeSymbol

	addSeed := func(s graph.CodeSymbol) {
		k := symbolKey(s)
		if seed[k] {
			return
		}
		seed[k] = true
		if fp := canonicalPath(s.Path); fp != "" {
			seedFiles[fp] = true
		}
		frontierSyms = append(frontierSyms, s)
	}

	for _, p := range changedPaths {
		cp := canonicalPath(p)
		if cp == "" {
			continue
		}
		syms, err := drv.SymbolsByPath(ctx, snapshotID, p)
		if err != nil {
			return result, err
		}
		// SymbolsByPath matches the stored path verbatim; also try the canonical
		// form when it differs (e.g. a leading "./" the caller passed).
		if len(syms) == 0 && cp != p {
			if syms, err = drv.SymbolsByPath(ctx, snapshotID, cp); err != nil {
				return result, err
			}
		}
		for _, s := range syms {
			cache.put(s)
			addSeed(s)
		}
	}
	for _, n := range changedSymbols {
		name := strings.TrimSpace(n)
		if name == "" {
			continue
		}
		syms, err := cache.byOriginalName(name)
		if err != nil {
			return result, err
		}
		for _, s := range syms {
			addSeed(s)
		}
	}
	if len(seed) == 0 {
		return result, nil
	}

	impactedNames := map[string]bool{}
	impactedFiles := map[string]bool{}
	visited := map[string]bool{}
	for k := range seed {
		visited[k] = true
	}

	depthReached := 0
	for d := 1; d <= maxDepth && len(frontierSyms) > 0; d++ {
		// Distinct callee names of the current frontier — the only to_refs whose
		// edges can point AT something on the frontier.
		nameSet := map[string]bool{}
		var names []string
		for _, s := range frontierSyms {
			if s.Name == "" || nameSet[s.Name] {
				continue
			}
			nameSet[s.Name] = true
			names = append(names, s.Name)
		}
		if len(names) == 0 {
			break
		}
		edges, err := drv.CallEdgesByToRefs(ctx, snapshotID, names)
		if err != nil {
			return result, err
		}

		// Frontier identities, so we only advance edges that actually resolve to
		// the frontier (not merely share a bare callee name).
		frontierSet := make(map[string]bool, len(frontierSyms))
		for _, s := range frontierSyms {
			frontierSet[symbolKey(s)] = true
		}

		var nextSyms []graph.CodeSymbol
		for _, e := range edges {
			if e.Kind != graph.EdgeCalls {
				continue
			}
			// Does this edge resolve to a symbol on the current frontier?
			byName, err := cache.lowerMap(e.ToRef)
			if err != nil {
				return result, err
			}
			hits := false
			for _, t := range resolveTargets(e, byName, cache.isRepoType) {
				if frontierSet[symbolKey(t)] {
					hits = true
					break
				}
			}
			if !hits {
				continue
			}
			caller, err := cache.resolveCaller(e)
			if err != nil {
				return result, err
			}
			if caller == nil {
				continue
			}
			ck := symbolKey(*caller)
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
			nextSyms = append(nextSyms, *caller)
		}
		frontierSyms = nextSyms
	}

	result.ImpactedSymbols = sortedKeys(impactedNames)
	result.ImpactedFiles = sortedKeys(impactedFiles)
	result.DepthReached = depthReached
	return result, nil
}

// symbolCache fetches symbols-by-name from the store on demand and memoizes the
// result, so each distinct name is queried at most once across a whole BFS. It
// maintains a lowercase-keyed view for the resolveTargets/resolveCaller helpers
// (which key by lowercased name) alongside a canonical-path index for caller
// file resolution.
type symbolCache struct {
	ctx        context.Context
	drv        store.StorageDriver
	snapshotID string
	// byLower[lowercased name] = symbols with that name (the form resolveTargets
	// and resolveCaller expect). A present (even if empty) entry means "fetched".
	byLower map[string][]graph.CodeSymbol
	// byFile[canonical path] = symbols in that file, populated opportunistically
	// from every symbol we observe; feeds resolveCaller's file fallback.
	byFile      map[string][]graph.CodeSymbol
	fileSeen    map[string]map[string]bool // path -> symbolKey set, for byFile dedup
	fileFetched map[string]bool            // canonical path -> SymbolsByPath already run
	// repoType[lowered name] = is it an in-repo type (memoized for isRepoType).
	repoType map[string]bool
}

func newSymbolCache(ctx context.Context, drv store.StorageDriver, snapshotID string) *symbolCache {
	return &symbolCache{
		ctx:         ctx,
		drv:         drv,
		snapshotID:  snapshotID,
		byLower:     map[string][]graph.CodeSymbol{},
		byFile:      map[string][]graph.CodeSymbol{},
		fileSeen:    map[string]map[string]bool{},
		fileFetched: map[string]bool{},
		repoType:    map[string]bool{},
	}
}

// symbolsInFile returns all symbols in canonical path `fp`, fetching them once
// via the indexed SymbolsByPath read and memoizing. The store matches the path
// verbatim; we query with the canonical form (paths are stored canonically).
func (c *symbolCache) symbolsInFile(fp string) ([]graph.CodeSymbol, error) {
	if fp == "" {
		return nil, nil
	}
	if c.fileFetched[fp] {
		return c.byFile[fp], nil
	}
	c.fileFetched[fp] = true
	syms, err := c.drv.SymbolsByPath(c.ctx, c.snapshotID, fp)
	if err != nil {
		return nil, err
	}
	for _, s := range syms {
		c.put(s)
	}
	return c.byFile[fp], nil
}

// put records a symbol into the by-file index (and, since it's already in hand,
// pre-warms the by-name cache for its lowercased name without a store round-trip
// only when that name was not yet fetched — otherwise leaves the authoritative
// fetched set untouched).
func (c *symbolCache) put(s graph.CodeSymbol) {
	if fp := canonicalPath(s.Path); fp != "" {
		k := symbolKey(s)
		seen := c.fileSeen[fp]
		if seen == nil {
			seen = map[string]bool{}
			c.fileSeen[fp] = seen
		}
		if !seen[k] {
			seen[k] = true
			c.byFile[fp] = append(c.byFile[fp], s)
		}
	}
}

// byOriginalName fetches symbols whose stored name == name (verbatim case, as
// the store's idx_symbols_snapshot_name index expects), memoized by lowercased
// name. Returns the fetched symbols.
func (c *symbolCache) byOriginalName(name string) ([]graph.CodeSymbol, error) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return nil, nil
	}
	if got, ok := c.byLower[lower]; ok {
		return got, nil
	}
	syms, err := c.drv.SymbolsByName(c.ctx, c.snapshotID, name)
	if err != nil {
		return nil, err
	}
	c.byLower[lower] = syms
	for _, s := range syms {
		c.put(s)
	}
	return syms, nil
}

// lowerMap returns a transient map[lowername][]symbol covering the single name
// `name` (lowercased), shaped exactly like indexSymbols' byName output, so
// resolveTargets can be reused unchanged. Fetched-and-cached on first use.
func (c *symbolCache) lowerMap(name string) (map[string][]graph.CodeSymbol, error) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return map[string][]graph.CodeSymbol{}, nil
	}
	if _, ok := c.byLower[lower]; !ok {
		if _, err := c.byOriginalName(name); err != nil {
			return nil, err
		}
	}
	return map[string][]graph.CodeSymbol{lower: c.byLower[lower]}, nil
}

// isRepoType reports whether `name` is an in-repo type/interface, enabling
// interface-dispatch resolution (a call on an in-repo interface variable resolves
// to any implementor's method). Cache-backed; false on lookup error.
func (c *symbolCache) isRepoType(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	if v, ok := c.repoType[lower]; ok {
		return v
	}
	v := false
	if syms, err := c.byOriginalName(name); err == nil {
		for _, s := range syms {
			if isTypeKind(s.Kind) {
				v = true
				break
			}
		}
	}
	c.repoType[lower] = v
	return v
}

// resolveCaller maps an edge's caller side back to a concrete symbol using the
// same prefer-same-file/then-any-same-name/then-first-in-file rule as the
// in-memory resolveCaller, but fetching candidates through the cache.
func (c *symbolCache) resolveCaller(e graph.DependencyEdge) (*graph.CodeSymbol, error) {
	fromFile := canonicalPath(e.FromFile)
	fromName := strings.TrimSpace(e.FromSymbol)
	if fromName != "" {
		cands, err := c.byOriginalName(fromName)
		if err != nil {
			return nil, err
		}
		for i := range cands {
			if fromFile != "" && canonicalPath(cands[i].Path) == fromFile {
				return &cands[i], nil
			}
		}
		if len(cands) > 0 {
			return &cands[0], nil
		}
	}
	if fromFile != "" {
		fileSyms, err := c.symbolsInFile(fromFile)
		if err != nil {
			return nil, err
		}
		if len(fileSyms) > 0 {
			return &fileSyms[0], nil
		}
	}
	return nil, nil
}

// resolveTargets returns the indexed symbol(s) an EdgeCalls edge plausibly
// targets, using the qualified_ref to disambiguate name collisions:
//   - callee name not indexed at all            -> external, nothing
//   - unqualified call (foo())                   -> same-package candidate(s), else any
//   - qualified pkg.Foo() where pkg is a package -> that package's candidate(s)
//   - qualified x.Foo() with no package match    -> indexed METHOD(s) named Foo,
//     filtered by RECEIVER TYPE: when the edge carries Metadata["recv_type"], only
//     methods declared on that exact type match (so bleve.Index() does NOT attribute
//     to our localEngine.Index method); when the receiver type is unknown ("") we
//     keep the best-effort set of all same-named methods. No method match (or a
//     known receiver type with no matching method) -> external (dropped).
func resolveTargets(e graph.DependencyEdge, symsByName map[string][]graph.CodeSymbol, isRepoType func(string) bool) []graph.CodeSymbol {
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
	if len(methods) == 0 {
		return nil // qualified, non-method, non-matching package -> external
	}
	// Method call on a value. Use the statically inferred receiver type to defeat
	// method-name collisions across unrelated types (the keystone precision fix).
	callRecv := edgeRecvType(e)
	if callRecv == "" {
		return methods // receiver type unknown -> best-effort over all same-named methods
	}
	var typed []graph.CodeSymbol
	for _, m := range methods {
		if symbolRecvType(m) == callRecv {
			typed = append(typed, m)
		}
	}
	if len(typed) > 0 {
		return typed // exact concrete receiver-type match
	}
	// No concrete match. If the receiver names an IN-REPO type (typically an
	// interface, e.g. `drv StorageDriver`), this is interface dispatch -> resolve
	// to any implementor's method. If the receiver is EXTERNAL (bleve.Batch,
	// sql.Rows), the call leaves the repo -> drop it (the collision fix still holds).
	if isRepoType != nil && isRepoType(callRecv) {
		return methods
	}
	return nil
}

// edgeRecvType returns the trimmed Metadata["recv_type"] of a call edge — the
// statically inferred base type of the call's receiver, or "" when unknown / a
// package call (per the SHARED METADATA CONTRACT).
func edgeRecvType(e graph.DependencyEdge) string {
	if e.Metadata == nil {
		return ""
	}
	rt, _ := e.Metadata["recv_type"].(string)
	return strings.TrimSpace(rt)
}

// symbolRecvType returns the trimmed Metadata["recv_type"] of a method symbol —
// the base type it is declared on, or "" when absent.
func symbolRecvType(s graph.CodeSymbol) string {
	if s.Metadata == nil {
		return ""
	}
	rt, _ := s.Metadata["recv_type"].(string)
	return strings.TrimSpace(rt)
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
	repoType := repoTypePredicate(syms)

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
			for _, t := range resolveTargets(e, symsByName, repoType) {
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

// isTypeKind reports whether a symbol kind denotes a type declaration (struct,
// interface, class, ...) rather than a function/method/variable.
func isTypeKind(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "type", "interface", "struct", "class", "enum", "trait":
		return true
	}
	return false
}

// repoTypePredicate returns a predicate reporting whether a name is an in-repo
// type — used to allow interface-dispatch method calls. Built once from a loaded
// symbol set; the ImpactGraph path uses symbolCache.isRepoType instead.
func repoTypePredicate(syms []graph.CodeSymbol) func(string) bool {
	set := map[string]bool{}
	for _, s := range syms {
		if isTypeKind(s.Kind) {
			set[strings.ToLower(strings.TrimSpace(s.Name))] = true
		}
	}
	return func(n string) bool { return set[strings.ToLower(strings.TrimSpace(n))] }
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

// CallersGraph returns the symbols that directly call the symbol(s) named `name`,
// resolved through indexed store reads (one hop). Receiver-type / qualifier
// precision applies via resolveTargets. Deduped by identity, sorted.
func CallersGraph(ctx context.Context, drv store.StorageDriver, snapshotID, name string) ([]graph.CodeSymbol, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	edges, err := drv.CallEdgesByToRefs(ctx, snapshotID, []string{name})
	if err != nil {
		return nil, err
	}
	cache := newSymbolCache(ctx, drv, snapshotID)
	byName, err := cache.lowerMap(name)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []graph.CodeSymbol
	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			continue
		}
		resolved := false
		for _, t := range resolveTargets(e, byName, cache.isRepoType) {
			if strings.EqualFold(t.Name, name) {
				resolved = true
				break
			}
		}
		if !resolved {
			continue
		}
		caller, err := cache.resolveCaller(e)
		if err != nil {
			return nil, err
		}
		if caller == nil {
			continue
		}
		if k := symbolKey(*caller); !seen[k] {
			seen[k] = true
			out = append(out, *caller)
		}
	}
	sortSymbols(out)
	return out, nil
}

// CalleesGraph returns the symbols the symbol(s) named `fromName` directly call,
// resolved through indexed store reads (one hop). Deduped by identity, sorted.
func CalleesGraph(ctx context.Context, drv store.StorageDriver, snapshotID, fromName string) ([]graph.CodeSymbol, error) {
	if strings.TrimSpace(fromName) == "" {
		return nil, nil
	}
	edges, err := drv.CallEdgesByFromSymbols(ctx, snapshotID, []string{fromName})
	if err != nil {
		return nil, err
	}
	cache := newSymbolCache(ctx, drv, snapshotID)
	seen := map[string]bool{}
	var out []graph.CodeSymbol
	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			continue
		}
		byName, err := cache.lowerMap(e.ToRef)
		if err != nil {
			return nil, err
		}
		for _, t := range resolveTargets(e, byName, cache.isRepoType) {
			if k := symbolKey(t); !seen[k] {
				seen[k] = true
				out = append(out, t)
			}
		}
	}
	sortSymbols(out)
	return out, nil
}

func sortSymbols(s []graph.CodeSymbol) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Path == s[j].Path {
			return s[i].Name < s[j].Name
		}
		return s[i].Path < s[j].Path
	})
}
