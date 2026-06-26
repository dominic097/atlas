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

	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/store"
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
	// Bulk-resolve the seed names in one batched query before per-name reads.
	if err := cache.prefetchNames(changedSymbols); err != nil {
		return result, err
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

		// HOP PASS 1 — frontier-hit resolution. The lazy path, for EVERY edge,
		// fetches its to_ref (lowerMap) then resolveTargets, which only reaches a
		// receiver-type lookup (isRepoType) for the qualified-method-collision case.
		// We BATCH the to_refs of this hop in ONE query (the high-cardinality class,
		// and the latency cliff) before resolving, then resolve each edge exactly as
		// the lazy loop does — leaving the rare receiver-type lookups lazy so their
		// fetch order (and thus the first-casing-wins cache key) is byte-identical to
		// the lazy path. We collect the hitting edges so pass 2 resolves only their
		// callers, mirroring the lazy path's "resolveCaller only on hit".
		var toRefNames []string
		for _, e := range edges {
			if e.Kind != graph.EdgeCalls {
				continue
			}
			if r := strings.TrimSpace(e.ToRef); r != "" {
				toRefNames = append(toRefNames, r)
			}
		}
		if err := cache.prefetchNames(toRefNames); err != nil {
			return result, err
		}

		var hitting []graph.DependencyEdge
		for _, e := range edges {
			if e.Kind != graph.EdgeCalls {
				continue
			}
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
			if hits {
				hitting = append(hitting, e)
			}
		}

		// HOP PASS 2 — caller resolution. The lazy path calls resolveCaller (which
		// fetches from_symbol) ONLY for edges that hit the frontier, AFTER the hit
		// pass above. We BATCH those from_symbols in ONE query (in edge order, so the
		// first-casing-wins key matches lazy) before resolving the callers.
		var fromNames []string
		for _, e := range hitting {
			if fs := strings.TrimSpace(e.FromSymbol); fs != "" {
				fromNames = append(fromNames, fs)
			}
		}
		if err := cache.prefetchNames(fromNames); err != nil {
			return result, err
		}

		var nextSyms []graph.CodeSymbol
		for _, e := range hitting {
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

// prefetchNames bulk-loads every not-yet-fetched name in ONE chunked
// SymbolsByNames query, populating the by-name cache for each (so the whole BFS
// hop costs a single round-trip instead of one point query per distinct name).
// This is the latency keystone for hub symbols: thousands of distinct caller /
// callee / receiver-type names collapse into batched IN-list reads.
//
// It preserves byOriginalName's exact semantics: each requested-but-unfetched
// name gets a cache entry (empty slice when it has no symbols, so a later
// byOriginalName never re-queries it), keyed by the lowercased name — the same
// authoritative "fetched" marker. Names already cached are skipped; passing them
// through SymbolsByNames would not change the result, but skipping keeps reads
// minimal and idempotent across hops.
func (c *symbolCache) prefetchNames(names []string) error {
	// Distinct, not-yet-fetched original names (dedup on lowercase to match the
	// cache key while querying with the verbatim case the index expects).
	seen := map[string]bool{}
	var todo []string
	for _, n := range names {
		name := strings.TrimSpace(n)
		lower := strings.ToLower(name)
		if lower == "" || seen[lower] {
			continue
		}
		seen[lower] = true
		if _, ok := c.byLower[lower]; ok {
			continue // already fetched
		}
		todo = append(todo, name)
	}
	if len(todo) == 0 {
		return nil
	}
	// Seed empty entries so unmatched names are marked fetched (never re-queried),
	// matching byOriginalName's "present even if empty == fetched" contract.
	for _, n := range todo {
		c.byLower[strings.ToLower(n)] = nil
	}
	syms, err := c.drv.SymbolsByNames(c.ctx, c.snapshotID, todo)
	if err != nil {
		return err
	}
	for _, s := range syms {
		lower := strings.ToLower(strings.TrimSpace(s.Name))
		if lower == "" {
			continue
		}
		c.byLower[lower] = append(c.byLower[lower], s)
		c.put(s)
	}
	return nil
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

// ReferencesGraph returns the symbols that REFERENCE the type named `name` as a
// type-use (not a call), resolved through indexed store reads over the
// "references" edges emitted by the go/types analyzer. Each such edge carries
// FromSymbol/FromFile of the enclosing declaration, so the caller-resolution
// rule reuses resolveCaller exactly like CallersGraph. Deduped by identity,
// sorted. Type-use edges already point at a concrete type name, so — unlike
// CallersGraph — no resolveTargets receiver-type disambiguation is needed.
func ReferencesGraph(ctx context.Context, drv store.StorageDriver, snapshotID, name string) ([]graph.CodeSymbol, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	edges, err := drv.RefEdgesByToRefs(ctx, snapshotID, []string{name})
	if err != nil {
		return nil, err
	}
	cache := newSymbolCache(ctx, drv, snapshotID)
	target := strings.ToLower(strings.TrimSpace(name))
	seen := map[string]bool{}
	var out []graph.CodeSymbol
	for _, e := range edges {
		if e.Kind != graph.EdgeReferences {
			continue
		}
		if strings.ToLower(strings.TrimSpace(e.ToRef)) != target {
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

// Path finds the SHORTEST forward call path from a symbol named `from` to a
// symbol named `to`, walking FORWARD over resolved call edges (caller -> callee).
// It is a BFS over symbol identities seeded by every symbol named `from`; each
// hop expands the frontier's callees via CallEdgesByFromSymbols + resolveTargets
// (so the receiver-type / qualifier precision applies), recording a parent
// pointer per symbolKey. When a callee's Name == `to` (case-insensitive) the
// chain is reconstructed from parent pointers and returned, from-symbol first.
// Bounded by maxDepth (default 6). Returns nil when `to` is unreachable.
func Path(ctx context.Context, drv store.StorageDriver, snapshotID, from, to string, maxDepth int) ([]graph.CodeSymbol, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return nil, nil
	}
	if maxDepth < 1 {
		maxDepth = 6
	}
	target := strings.ToLower(to)

	cache := newSymbolCache(ctx, drv, snapshotID)
	seeds, err := cache.byOriginalName(from)
	if err != nil {
		return nil, err
	}
	if len(seeds) == 0 {
		return nil, nil
	}

	// parent[symbolKey] = the predecessor symbolKey on the discovered path; the
	// seeds map to "" (no predecessor). node[symbolKey] holds the symbol itself.
	parent := map[string]string{}
	node := map[string]graph.CodeSymbol{}
	visited := map[string]bool{}
	var frontier []graph.CodeSymbol
	for _, s := range seeds {
		k := symbolKey(s)
		if visited[k] {
			continue
		}
		visited[k] = true
		parent[k] = ""
		node[k] = s
		frontier = append(frontier, s)
		// A seed that is itself named `to` is a zero-length path to itself.
		if strings.EqualFold(s.Name, to) {
			return reconstructPath(k, parent, node), nil
		}
	}

	for d := 1; d <= maxDepth && len(frontier) > 0; d++ {
		// Distinct frontier names — the only from_symbols whose edges leave it.
		nameSet := map[string]bool{}
		var names []string
		for _, s := range frontier {
			if s.Name == "" || nameSet[strings.ToLower(s.Name)] {
				continue
			}
			nameSet[strings.ToLower(s.Name)] = true
			names = append(names, s.Name)
		}
		if len(names) == 0 {
			break
		}
		edges, err := drv.CallEdgesByFromSymbols(ctx, snapshotID, names)
		if err != nil {
			return nil, err
		}
		// Batch the to_refs of this hop in one query before resolving.
		var toRefNames []string
		for _, e := range edges {
			if e.Kind == graph.EdgeCalls {
				if r := strings.TrimSpace(e.ToRef); r != "" {
					toRefNames = append(toRefNames, r)
				}
			}
		}
		if err := cache.prefetchNames(toRefNames); err != nil {
			return nil, err
		}

		// frontierKeys maps a caller symbolKey present on the frontier so an edge's
		// caller can be attributed to the exact predecessor node.
		frontierKeys := make(map[string]bool, len(frontier))
		for _, s := range frontier {
			frontierKeys[symbolKey(s)] = true
		}

		var next []graph.CodeSymbol
		for _, e := range edges {
			if e.Kind != graph.EdgeCalls {
				continue
			}
			caller, err := cache.resolveCaller(e)
			if err != nil {
				return nil, err
			}
			if caller == nil {
				continue
			}
			ck := symbolKey(*caller)
			if !frontierKeys[ck] {
				continue // this edge's caller is not on the current frontier
			}
			byName, err := cache.lowerMap(e.ToRef)
			if err != nil {
				return nil, err
			}
			for _, t := range resolveTargets(e, byName, cache.isRepoType) {
				tk := symbolKey(t)
				if visited[tk] {
					continue
				}
				visited[tk] = true
				parent[tk] = ck
				node[tk] = t
				if strings.EqualFold(t.Name, to) || strings.ToLower(t.Name) == target {
					return reconstructPath(tk, parent, node), nil
				}
				next = append(next, t)
			}
		}
		frontier = next
	}
	return nil, nil
}

// reconstructPath walks parent pointers from `endKey` back to a seed and returns
// the symbol chain in forward order (seed first, target last).
func reconstructPath(endKey string, parent map[string]string, node map[string]graph.CodeSymbol) []graph.CodeSymbol {
	var rev []graph.CodeSymbol
	for k := endKey; k != ""; k = parent[k] {
		rev = append(rev, node[k])
		if parent[k] == "" {
			break
		}
	}
	// reverse
	out := make([]graph.CodeSymbol, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out
}

// isTestSymbol reports whether a symbol is a test: its file path matches a
// conventional test-file pattern (_test.go / *_test.py / test_*.py /
// *.test.{js,ts} / *.spec.{js,ts} / *Test.java / *Tests.java / *IT.java), OR its
// name has a Test / test_ prefix.
func isTestSymbol(s graph.CodeSymbol) bool {
	name := strings.TrimSpace(s.Name)
	if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "test_") {
		return true
	}
	return isTestPath(s.Path)
}

// isTestPath reports whether a file path matches a conventional test-file naming.
func isTestPath(path string) bool {
	p := canonicalPath(path)
	if p == "" {
		return false
	}
	base := p
	if i := strings.LastIndex(p, "/"); i >= 0 {
		base = p[i+1:]
	}
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, "_test.go"):
		return true
	case strings.HasSuffix(lower, "_test.py"):
		return true
	case strings.HasPrefix(lower, "test_") && strings.HasSuffix(lower, ".py"):
		return true
	case strings.HasSuffix(lower, ".test.js"), strings.HasSuffix(lower, ".test.ts"),
		strings.HasSuffix(lower, ".test.jsx"), strings.HasSuffix(lower, ".test.tsx"):
		return true
	case strings.HasSuffix(lower, ".spec.js"), strings.HasSuffix(lower, ".spec.ts"),
		strings.HasSuffix(lower, ".spec.jsx"), strings.HasSuffix(lower, ".spec.tsx"):
		return true
	case strings.HasSuffix(base, "Test.java"), strings.HasSuffix(base, "Tests.java"),
		strings.HasSuffix(base, "IT.java"):
		return true
	}
	return false
}

// CoverageRef is a lightweight pointer to a covering test / covered symbol.
type CoverageRef struct {
	SymbolID string `json:"symbol_id"`
	Name     string `json:"symbol"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
}

// CoverageResult is static call-graph reachability "coverage": for
// tests_for_symbol, the transitive test CALLERS reaching TARGET; for
// symbols_for_test, the transitive NON-test CALLEES TARGET exercises. This is
// STATIC call-graph reachability, not runtime coverage.
type CoverageResult struct {
	Target    string        `json:"target"`
	Direction string        `json:"direction"`
	Covered   bool          `json:"covered"`
	Tests     []CoverageRef `json:"tests,omitempty"`
	Symbols   []CoverageRef `json:"symbols,omitempty"`
}

const coverageCap = 200

// Coverage computes static call-graph reachability coverage for TARGET.
//
// direction is resolved as follows: "tests_for_symbol" walks the transitive
// CALLERS of TARGET (reverse reachability over call edges, bounded by maxDepth,
// default 8) and keeps those that are tests; "symbols_for_test" walks the
// transitive CALLEES of TARGET and keeps the non-test symbols it exercises. When
// direction is empty/"auto", it is inferred from TARGET: if every resolved
// TARGET symbol is a test -> symbols_for_test, else tests_for_symbol.
//
// The reachability BFS reuses resolveTargets/resolveCaller via the symbolCache
// (mirroring ImpactGraph/Subgraph) so receiver-type/qualifier precision applies.
func Coverage(ctx context.Context, drv store.StorageDriver, snapshotID, target, direction string, maxDepth int) (CoverageResult, error) {
	target = strings.TrimSpace(target)
	if maxDepth < 1 {
		maxDepth = 8
	}
	res := CoverageResult{Target: target}
	if target == "" {
		return res, nil
	}

	cache := newSymbolCache(ctx, drv, snapshotID)
	seeds, err := cache.byOriginalName(target)
	if err != nil {
		return res, err
	}

	dir := strings.ToLower(strings.TrimSpace(direction))
	if dir == "" || dir == "auto" {
		dir = "tests_for_symbol"
		if len(seeds) > 0 {
			allTests := true
			for _, s := range seeds {
				if !isTestSymbol(s) {
					allTests = false
					break
				}
			}
			if allTests {
				dir = "symbols_for_test"
			}
		}
	}
	res.Direction = dir
	if len(seeds) == 0 {
		return res, nil
	}

	if dir == "symbols_for_test" {
		reached, err := reachableCallees(ctx, drv, cache, seeds, maxDepth)
		if err != nil {
			return res, err
		}
		var out []graph.CodeSymbol
		for _, s := range reached {
			if !isTestSymbol(s) {
				out = append(out, s)
			}
		}
		sortSymbols(out)
		res.Symbols = coverageRefs(out, coverageCap)
		return res, nil
	}

	// tests_for_symbol: transitive callers filtered to tests.
	reached, err := reachableCallers(ctx, drv, cache, seeds, maxDepth)
	if err != nil {
		return res, err
	}
	var tests []graph.CodeSymbol
	for _, s := range reached {
		if isTestSymbol(s) {
			tests = append(tests, s)
		}
	}
	sortSymbols(tests)
	res.Tests = coverageRefs(tests, coverageCap)
	res.Covered = len(tests) > 0
	return res, nil
}

// reachableCallers returns every distinct symbol that transitively CALLS any of
// the seeds (reverse reachability over resolved call edges), bounded by maxDepth.
// Seeds themselves are excluded from the result.
func reachableCallers(ctx context.Context, drv store.StorageDriver, cache *symbolCache, seeds []graph.CodeSymbol, maxDepth int) ([]graph.CodeSymbol, error) {
	visited := map[string]bool{}
	var frontier []graph.CodeSymbol
	for _, s := range seeds {
		k := symbolKey(s)
		if !visited[k] {
			visited[k] = true
			frontier = append(frontier, s)
		}
	}
	out := map[string]graph.CodeSymbol{}
	for d := 1; d <= maxDepth && len(frontier) > 0; d++ {
		nameSet := map[string]bool{}
		var names []string
		for _, s := range frontier {
			if s.Name == "" || nameSet[s.Name] {
				continue
			}
			nameSet[s.Name] = true
			names = append(names, s.Name)
		}
		if len(names) == 0 {
			break
		}
		edges, err := drv.CallEdgesByToRefs(ctx, snapshotID(cache), names)
		if err != nil {
			return nil, err
		}
		var toRefNames []string
		for _, e := range edges {
			if e.Kind == graph.EdgeCalls {
				if r := strings.TrimSpace(e.ToRef); r != "" {
					toRefNames = append(toRefNames, r)
				}
			}
		}
		if err := cache.prefetchNames(toRefNames); err != nil {
			return nil, err
		}
		frontierSet := make(map[string]bool, len(frontier))
		for _, s := range frontier {
			frontierSet[symbolKey(s)] = true
		}
		var hitting []graph.DependencyEdge
		for _, e := range edges {
			if e.Kind != graph.EdgeCalls {
				continue
			}
			byName, err := cache.lowerMap(e.ToRef)
			if err != nil {
				return nil, err
			}
			for _, t := range resolveTargets(e, byName, cache.isRepoType) {
				if frontierSet[symbolKey(t)] {
					hitting = append(hitting, e)
					break
				}
			}
		}
		var fromNames []string
		for _, e := range hitting {
			if fs := strings.TrimSpace(e.FromSymbol); fs != "" {
				fromNames = append(fromNames, fs)
			}
		}
		if err := cache.prefetchNames(fromNames); err != nil {
			return nil, err
		}
		var next []graph.CodeSymbol
		for _, e := range hitting {
			caller, err := cache.resolveCaller(e)
			if err != nil {
				return nil, err
			}
			if caller == nil {
				continue
			}
			ck := symbolKey(*caller)
			if visited[ck] {
				continue
			}
			visited[ck] = true
			out[ck] = *caller
			next = append(next, *caller)
		}
		frontier = next
	}
	return mapValues(out), nil
}

// reachableCallees returns every distinct symbol any of the seeds transitively
// CALLS (forward reachability over resolved call edges), bounded by maxDepth.
// Seeds themselves are excluded from the result.
func reachableCallees(ctx context.Context, drv store.StorageDriver, cache *symbolCache, seeds []graph.CodeSymbol, maxDepth int) ([]graph.CodeSymbol, error) {
	visited := map[string]bool{}
	var frontier []graph.CodeSymbol
	for _, s := range seeds {
		k := symbolKey(s)
		if !visited[k] {
			visited[k] = true
			frontier = append(frontier, s)
		}
	}
	out := map[string]graph.CodeSymbol{}
	for d := 1; d <= maxDepth && len(frontier) > 0; d++ {
		nameSet := map[string]bool{}
		var names []string
		for _, s := range frontier {
			if s.Name == "" || nameSet[strings.ToLower(s.Name)] {
				continue
			}
			nameSet[strings.ToLower(s.Name)] = true
			names = append(names, s.Name)
		}
		if len(names) == 0 {
			break
		}
		edges, err := drv.CallEdgesByFromSymbols(ctx, snapshotID(cache), names)
		if err != nil {
			return nil, err
		}
		var toRefNames []string
		for _, e := range edges {
			if e.Kind == graph.EdgeCalls {
				if r := strings.TrimSpace(e.ToRef); r != "" {
					toRefNames = append(toRefNames, r)
				}
			}
		}
		if err := cache.prefetchNames(toRefNames); err != nil {
			return nil, err
		}
		frontierKeys := make(map[string]bool, len(frontier))
		for _, s := range frontier {
			frontierKeys[symbolKey(s)] = true
		}
		var next []graph.CodeSymbol
		for _, e := range edges {
			if e.Kind != graph.EdgeCalls {
				continue
			}
			caller, err := cache.resolveCaller(e)
			if err != nil {
				return nil, err
			}
			if caller == nil || !frontierKeys[symbolKey(*caller)] {
				continue
			}
			byName, err := cache.lowerMap(e.ToRef)
			if err != nil {
				return nil, err
			}
			for _, t := range resolveTargets(e, byName, cache.isRepoType) {
				tk := symbolKey(t)
				if visited[tk] {
					continue
				}
				visited[tk] = true
				out[tk] = t
				next = append(next, t)
			}
		}
		frontier = next
	}
	return mapValues(out), nil
}

// snapshotID exposes the cache's bound snapshot id for the reachability helpers.
func snapshotID(c *symbolCache) string { return c.snapshotID }

func mapValues(m map[string]graph.CodeSymbol) []graph.CodeSymbol {
	out := make([]graph.CodeSymbol, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func coverageRefs(syms []graph.CodeSymbol, limit int) []CoverageRef {
	out := make([]CoverageRef, 0, len(syms))
	for _, s := range syms {
		out = append(out, CoverageRef{SymbolID: s.ID, Name: s.Name, Kind: s.Kind, Path: s.Path, Line: s.StartLine})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// SymbolChange is one symbol added/removed/modified between two snapshots.
type SymbolChange struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Change string `json:"change"` // added | removed | modified
}

// EdgeChange is one call relationship added/removed between two snapshots.
type EdgeChange struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Change string `json:"change"` // added | removed
}

// DiffResult is the structural delta from a base snapshot to a head snapshot.
type DiffResult struct {
	Added        []SymbolChange `json:"added_symbols"`
	Removed      []SymbolChange `json:"removed_symbols"`
	Modified     []SymbolChange `json:"modified_symbols"`
	ChangedFiles []string       `json:"changed_files"`
	AddedEdges   []EdgeChange   `json:"added_edges"`
	RemovedEdges []EdgeChange   `json:"removed_edges"`
}

// Diff computes the structural delta from base (A) to head (B): symbols added /
// removed / modified, the distinct changed files, and added/removed call edges.
// Symbols are matched by their slot (path|kind|name); "modified" = same slot with
// a different NodeID (the content-stable identity changed, i.e. the signature/
// shape changed) — this is exactly what NodeID was designed for. Renames/moves
// surface as remove+add (P0). Edges are matched by caller|callee name.
func Diff(symsA, symsB []graph.CodeSymbol, edgesA, edgesB []graph.DependencyEdge) DiffResult {
	slotKey := func(s graph.CodeSymbol) string {
		return canonicalPath(s.Path) + "\x00" + strings.ToLower(strings.TrimSpace(s.Kind)) + "\x00" + strings.TrimSpace(s.Name)
	}
	a := make(map[string]graph.CodeSymbol, len(symsA))
	for _, s := range symsA {
		a[slotKey(s)] = s
	}
	b := make(map[string]graph.CodeSymbol, len(symsB))
	for _, s := range symsB {
		b[slotKey(s)] = s
	}

	var res DiffResult
	files := map[string]bool{}
	mark := func(p string) {
		if p = canonicalPath(p); p != "" {
			files[p] = true
		}
	}
	for k, sb := range b {
		if sa, ok := a[k]; !ok {
			res.Added = append(res.Added, symChange(sb, "added"))
			mark(sb.Path)
		} else if string(sa.NodeID) != string(sb.NodeID) {
			res.Modified = append(res.Modified, symChange(sb, "modified"))
			mark(sb.Path)
		}
	}
	for k, sa := range a {
		if _, ok := b[k]; !ok {
			res.Removed = append(res.Removed, symChange(sa, "removed"))
			mark(sa.Path)
		}
	}

	edgeKey := func(e graph.DependencyEdge) string {
		return strings.ToLower(strings.TrimSpace(e.FromSymbol)) + "\x00" + strings.ToLower(strings.TrimSpace(e.ToRef))
	}
	ea := map[string]graph.DependencyEdge{}
	for _, e := range edgesA {
		if e.Kind == graph.EdgeCalls && e.FromSymbol != "" && e.ToRef != "" {
			ea[edgeKey(e)] = e
		}
	}
	eb := map[string]graph.DependencyEdge{}
	for _, e := range edgesB {
		if e.Kind == graph.EdgeCalls && e.FromSymbol != "" && e.ToRef != "" {
			eb[edgeKey(e)] = e
		}
	}
	for k, e := range eb {
		if _, ok := ea[k]; !ok {
			res.AddedEdges = append(res.AddedEdges, EdgeChange{From: e.FromSymbol, To: e.ToRef, Change: "added"})
		}
	}
	for k, e := range ea {
		if _, ok := eb[k]; !ok {
			res.RemovedEdges = append(res.RemovedEdges, EdgeChange{From: e.FromSymbol, To: e.ToRef, Change: "removed"})
		}
	}

	for f := range files {
		res.ChangedFiles = append(res.ChangedFiles, f)
	}
	sort.Strings(res.ChangedFiles)
	sortChanges(res.Added)
	sortChanges(res.Removed)
	sortChanges(res.Modified)
	sortEdgeChanges(res.AddedEdges)
	sortEdgeChanges(res.RemovedEdges)
	return res
}

func symChange(s graph.CodeSymbol, ch string) SymbolChange {
	return SymbolChange{Name: s.Name, Path: s.Path, Kind: s.Kind, Change: ch}
}

func sortChanges(c []SymbolChange) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Path == c[j].Path {
			return c[i].Name < c[j].Name
		}
		return c[i].Path < c[j].Path
	})
}

func sortEdgeChanges(c []EdgeChange) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].From == c[j].From {
			return c[i].To < c[j].To
		}
		return c[i].From < c[j].From
	})
}

// SubEdge is a directed caller->callee relationship in a subgraph, by symbol name.
type SubEdge struct{ From, To string }

// SubgraphResult is the call-graph neighborhood around a symbol: representative
// symbols (one per distinct name) and the directed call edges among them.
type SubgraphResult struct {
	Nodes []graph.CodeSymbol
	Edges []SubEdge
}

// Subgraph builds the call-graph neighborhood around the symbol(s) named `name`:
// up to `depth` hops of BOTH callers and callees, bounded by maxNodes. Nodes are
// deduped by name (one representative symbol each); edges are caller->callee. It
// reuses the precise CallersGraph/CalleesGraph resolution, so external/mismatched
// calls never enter the subgraph.
func Subgraph(ctx context.Context, drv store.StorageDriver, snapshotID, name string, depth, maxNodes int) (SubgraphResult, error) {
	if depth < 1 {
		depth = 1
	}
	if maxNodes <= 0 {
		maxNodes = 200
	}
	res := SubgraphResult{}
	seed, err := drv.SymbolsByName(ctx, snapshotID, name)
	if err != nil {
		return res, err
	}
	if len(seed) == 0 {
		return res, nil
	}

	nodes := map[string]graph.CodeSymbol{} // lowername -> representative symbol
	addNode := func(s graph.CodeSymbol) {
		if k := strings.ToLower(strings.TrimSpace(s.Name)); k != "" {
			if _, ok := nodes[k]; !ok {
				nodes[k] = s
			}
		}
	}
	for _, s := range seed {
		addNode(s)
	}

	edgeSeen := map[string]bool{}
	addEdge := func(from, to string) {
		fk, tk := strings.ToLower(from), strings.ToLower(to)
		if fk == "" || tk == "" || fk == tk {
			return
		}
		if key := fk + "\x00" + tk; !edgeSeen[key] {
			edgeSeen[key] = true
			res.Edges = append(res.Edges, SubEdge{From: from, To: to})
		}
	}

	visited := map[string]bool{strings.ToLower(name): true}
	frontier := []string{name}
	for d := 1; d <= depth && len(frontier) > 0 && len(nodes) < maxNodes; d++ {
		var next []string
		for _, fn := range frontier {
			callees, err := CalleesGraph(ctx, drv, snapshotID, fn)
			if err != nil {
				return res, err
			}
			for _, c := range callees {
				addNode(c)
				addEdge(fn, c.Name)
				if lk := strings.ToLower(c.Name); !visited[lk] {
					visited[lk] = true
					next = append(next, c.Name)
				}
			}
			callers, err := CallersGraph(ctx, drv, snapshotID, fn)
			if err != nil {
				return res, err
			}
			for _, r := range callers {
				addNode(r)
				addEdge(r.Name, fn)
				if lk := strings.ToLower(r.Name); !visited[lk] {
					visited[lk] = true
					next = append(next, r.Name)
				}
			}
			if len(nodes) >= maxNodes {
				break
			}
		}
		frontier = next
	}
	for _, s := range nodes {
		res.Nodes = append(res.Nodes, s)
	}
	sortSymbols(res.Nodes)
	return res, nil
}
