// delta.go implements incremental (delta) indexing for index.Run.
//
// A delta reindex avoids re-parsing the whole working tree on a per-edit update:
// only the files the working tree shows as changed/added (vs. the base snapshot)
// are re-parsed, and the rest of the graph (symbols, edges, routes for untouched
// files) is carried forward from the base snapshot. Change detection itself lives
// in worktree.go (scanWorkTree, a git-independent content-hash compare) so an
// UNCOMMITTED edit, an untracked new file, or a deletion is detected even with no
// new commit. The merged result is sorted identically to a full index so a delta
// snapshot is byte-for-byte equivalent to a full reindex of the same tree state.
//
// Everything here is built on the EXISTING StorageDriver methods (ListRepos,
// LatestSnapshot, ListFiles, ListSymbols, ListEdges, ListRoutes); the delta logic
// derives its base snapshot itself and needs no store-interface or engine change.
package index

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dominic097/atlas/internal/gotypes"
	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/lexical"
	"github.com/dominic097/atlas/internal/parser"
	"github.com/dominic097/atlas/internal/routes"
	"github.com/dominic097/atlas/internal/store"
)

// workingTreeSHA is the sentinel resolveCommitSHA returns for a non-git tree. The
// commit field is now informational only — change detection is content-hash based
// (scanWorkTree), so a working-tree sentinel base is a perfectly valid delta base.
const workingTreeSHA = "working-tree"

// deltaBase is the previous-snapshot context a delta reindex builds on.
type deltaBase struct {
	repoID   string          // canonical repo id resolved from the store
	snapshot *graph.Snapshot // the base (latest) snapshot
	commit   string          // base commit sha (== snapshot.CommitSHA)
}

// resolveDeltaBase finds the canonical repo (case-insensitive full_name match)
// and its latest snapshot. It returns (nil, nil) when there is no prior repo or
// snapshot — the caller then takes the full path. Lookup errors propagate so the
// caller can decide to fall back rather than fail.
func resolveDeltaBase(ctx context.Context, drv store.StorageDriver, repoFullName string) (*deltaBase, error) {
	repos, err := drv.ListRepos(ctx, "")
	if err != nil {
		return nil, err
	}
	var repoID string
	for _, r := range repos {
		if strings.EqualFold(r.FullName, repoFullName) {
			repoID = r.ID
			break
		}
	}
	if repoID == "" {
		return nil, nil
	}
	snap, err := drv.LatestSnapshot(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, nil
	}
	return &deltaBase{repoID: repoID, snapshot: snap, commit: snap.CommitSHA}, nil
}

// canonicalPath normalizes a path for set membership: trim, ToSlash.
func canonicalPath(p string) string {
	return toSlash(strings.TrimSpace(p))
}

// toSlash converts OS separators to forward slashes without importing path
// machinery into the hot membership checks.
func toSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// keepBaseSymbols returns the base snapshot's symbols whose owning file is NOT in
// touched (changed ∪ deleted). Dropped symbols belong to files that were either
// re-parsed (replaced by fresh rows) or deleted.
func keepBaseSymbols(base []graph.CodeSymbol, touched map[string]struct{}) []graph.CodeSymbol {
	out := make([]graph.CodeSymbol, 0, len(base))
	for _, s := range base {
		if _, hit := touched[canonicalPath(s.Path)]; hit {
			continue
		}
		out = append(out, s)
	}
	return out
}

// keepBaseEdges returns the base snapshot's edges whose FromFile is NOT in
// touched. An edge is owned by the file it originates from.
func keepBaseEdges(base []graph.DependencyEdge, touched map[string]struct{}) []graph.DependencyEdge {
	out := make([]graph.DependencyEdge, 0, len(base))
	for _, e := range base {
		if _, hit := touched[canonicalPath(e.FromFile)]; hit {
			continue
		}
		out = append(out, e)
	}
	return out
}

// dropTypeUseRefs removes the go/types-sourced type-use reference edges from a
// merged edge set so a re-enrichment (enrichGoTypes) can regenerate them without
// duplicating the copies kept from the base snapshot. Call edges and any
// non-go_types edges are preserved untouched.
//
// When scope is nil, EVERY go_types reference edge is dropped (the whole-module
// re-enrichment regenerates them all). When scope is non-nil, only reference edges
// whose FromFile is in scope are dropped — the incremental (scoped) re-enrichment
// regenerates exactly those, and every untouched file's carried-forward reference
// edges are preserved. scope holds canonical (ToSlash, trimmed) paths.
func dropTypeUseRefs(edges []graph.DependencyEdge, scope map[string]struct{}) []graph.DependencyEdge {
	out := make([]graph.DependencyEdge, 0, len(edges))
	for _, e := range edges {
		if e.Kind == graph.EdgeReferences {
			if src, _ := e.Metadata["source"].(string); src == "go_types" {
				if scope == nil {
					continue
				}
				if _, inScope := scope[canonicalPath(e.FromFile)]; inScope {
					continue
				}
			}
		}
		out = append(out, e)
	}
	return out
}

// keepBaseRoutes returns the base snapshot's routes whose HandlerFile is NOT in
// touched. A route is owned by its handler/calling file (HandlerFile doubles as
// "the relevant file" for both producer and consumer roles).
func keepBaseRoutes(base []graph.Route, touched map[string]struct{}) []graph.Route {
	out := make([]graph.Route, 0, len(base))
	for _, r := range base {
		if _, hit := touched[canonicalPath(r.HandlerFile)]; hit {
			continue
		}
		out = append(out, r)
	}
	return out
}

// makeSet builds a canonical-path set from one or more path slices.
func makeSet(slices ...[]string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, s := range slices {
		for _, p := range s {
			if cp := canonicalPath(p); cp != "" {
				set[cp] = struct{}{}
			}
		}
	}
	return set
}

// runDelta executes the incremental index path from a PRECOMPUTED working-tree
// scan. Preconditions (checked by the caller): base != nil and scan != nil with
// at least one change (a no-change scan is the caller's noop). It re-parses only
// changed/added files, carries the rest of the base graph forward, merges +
// re-sorts everything to match a full index, persists a new snapshot, and
// rebuilds the lexical index.
//
// The scan is the single source of change truth: it already content-hashed every
// present file and classified it against the base snapshot's stored hashes, so
// uncommitted edits, untracked new files, and deletions are all reflected here
// regardless of commit state. runDelta re-uses the scan's materialized content +
// hashes — it does not re-walk or re-hash the tree.
//
// Any error here is the caller's signal to fall back to a full index — runDelta
// must not have side effects before SaveSnapshot, which it doesn't (all work is
// in-memory until the single save).
func runDelta(ctx context.Context, drv store.StorageDriver, lx *lexical.Index, base *deltaBase, scan *workTreeScan, repoFullName, absRoot, head string, opts Options, start time.Time) (*graph.Snapshot, Stats, error) {
	baseCommit := base.commit

	timings := map[string]int64{}
	phase := func(name string, since time.Time) {
		timings[name] += time.Since(since).Milliseconds()
	}

	changedSet := scan.changedSet() // changed ∪ added — the files to (re)parse
	touched := scan.touchedSet()    // changed ∪ added ∪ deleted — base rows to drop

	// Whether any touched (changed/added/deleted) file is Go — gates the go/types
	// re-enrichment below. No Go change means the kept base Go edges are already
	// precise, so the whole-repo type pass can be skipped (delta stays fast).
	goTouched := false
	for p := range touched {
		if parser.LanguageForPath(p) == "go" {
			goTouched = true
			break
		}
	}

	// Build rows from the precomputed scan with the SAME prune/size/lang rules the
	// full path used (the scan already applied them). Every present supported file
	// contributes a graph.File row (content+hash already computed) so FileCount
	// matches a full index of HEAD; only changed/added files are (re)parsed.
	var (
		files        []graph.File
		newSymbols   []graph.CodeSymbol
		newEdges     []graph.DependencyEdge
		newRawRoutes []routes.RawRoute
		languages    = map[string]int{}
	)

	parseStart := time.Now()
	for i := range scan.files {
		if ctx.Err() != nil {
			return nil, Stats{}, ctx.Err()
		}
		wf := &scan.files[i]

		// File row for every supported, present file (keeps FileCount correct).
		files = append(files, graph.File{
			ID:        uuid.NewString(),
			Path:      wf.relPath,
			Language:  wf.lang,
			SizeBytes: wf.size,
			Hash:      wf.hash,
		})
		languages[wf.lang]++

		// Only changed/added files are (re)parsed; their fresh rows replace the
		// dropped base rows for the same file.
		if _, isChanged := changedSet[wf.relPath]; !isChanged {
			continue
		}
		res, parseErr := parser.Parse("", repoFullName, wf.relPath, wf.lang, wf.content)
		if parseErr != nil {
			continue
		}
		// Restore imports onto the file row now that we parsed it.
		files[len(files)-1].Imports = res.Imports
		newSymbols = append(newSymbols, res.Symbols...)
		newEdges = append(newEdges, res.Edges...)
		newRawRoutes = append(newRawRoutes, routes.ExtractFile(wf.lang, wf.relPath, string(wf.content))...)
	}
	phase("parse", parseStart)

	// Load the base graph and keep only the rows whose owning file is untouched.
	baseLoadStart := time.Now()
	baseSymbols, err := drv.ListSymbols(ctx, base.snapshot.ID)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: delta load base symbols: %w", err)
	}
	baseEdges, err := drv.ListEdges(ctx, base.snapshot.ID)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: delta load base edges: %w", err)
	}
	baseRoutes, err := drv.ListRoutes(ctx, base.snapshot.ID, "")
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: delta load base routes: %w", err)
	}
	phase("base_load", baseLoadStart)

	mergedSymbols := append(keepBaseSymbols(baseSymbols, touched), newSymbols...)
	mergedEdges := append(keepBaseEdges(baseEdges, touched), newEdges...)
	// Resolve the changed-file raw routes against the FULL merged symbol set so a
	// handler defined in an untouched file still resolves to its file, then append
	// to the kept base routes.
	resolvedNewRoutes := routes.Resolve(repoFullName, newRawRoutes, mergedSymbols)
	mergedRoutes := append(keepBaseRoutes(baseRoutes, touched), resolvedNewRoutes...)

	// Precise Go analysis parity with the full path: when a Go file changed or was
	// deleted, refresh the go/types recv_type refinement + type-use reference edges
	// so the delta carries the same precision a full reindex would. Touched Go files
	// would otherwise keep only heuristic edges, and a deleted file would leave stale
	// references behind.
	//
	// FAST PATH (incremental): type-check only the changed packages + their in-module
	// reverse-deps and refresh exactly those files' edges, carrying every untouched
	// file's go/types edges forward. This is byte-identical to the whole-module pass
	// for the affected files (see enrichGoTypesScoped / gotypes.AnalyzeScoped) but
	// skips re-type-checking packages the edit cannot affect.
	//
	// SAFE PATH (whole-module): when the scoped analyzer declines (OK:false) — e.g. a
	// new file whose package the metadata load can't yet see, an oversized repo, or a
	// load error — fall back to the whole-repo pass so precision is never regressed.
	// A pure deletion (no changed/added Go file) also takes the safe path, since
	// there is no changed package to scope to but stale refs must still be dropped.
	goTypesMode := ""
	if goTouched {
		goTypesStart := time.Now()
		var goFiles []string
		for i := range files {
			if files[i].Language == "go" {
				goFiles = append(goFiles, files[i].Path)
			}
		}

		// changedGoFiles = the re-parsed Go files (changed ∪ added) that scope the
		// incremental type-check. Deletions are not here (they have no file on disk);
		// when only deletions touched Go, this set is empty and we take the safe path.
		changedGoFiles := map[string]struct{}{}
		for p := range changedSet {
			if parser.LanguageForPath(p) == "go" {
				changedGoFiles[canonicalPath(p)] = struct{}{}
			}
		}

		// A Go DELETION can remove a type that a reverse-dep file references, leaving
		// a stale carried-forward reference edge that only the whole-module pass would
		// drop. The deleted file is gone from disk, so the scoped metadata load cannot
		// seed its package as a reverse-dep root. Rather than risk a stale ref, any Go
		// deletion forces the safe whole-module path. (Plain edits/additions, the
		// common per-edit case, still take the fast scoped path.)
		goDeleted := false
		for p := range scan.deleted {
			if parser.LanguageForPath(p) == "go" {
				goDeleted = true
				break
			}
		}

		// SIZE GATE: the scoped path adds a fixed metadata `go list ./...` cost (to
		// discover the package graph for reverse-dep expansion) and only wins when the
		// whole-module type-check it replaces is much larger than that discovery cost.
		// On a SMALL module the discovery `go list` is comparable to a full typed load,
		// so scoping is break-even-to-slower (measured: logrus ~47 Go files, scoped
		// ≈ whole-module). Above gotypes.ScopedMinGoFiles the full type-check dominates
		// and scoping pays off (measured: x/tools ~1268 Go files, scoped ~30% faster on
		// a low-fan-out edit). Below the gate we keep the whole-module pass — never a
		// speed regression on the small repos that are the common case.
		bigEnough := len(goFiles) >= gotypes.ScopedMinGoFiles

		scoped := false
		if len(changedGoFiles) > 0 && !goDeleted && bigEnough {
			mergedEdges, scoped = enrichGoTypesScoped(ctx, absRoot, goFiles, changedGoFiles, mergedEdges)
		}
		if scoped {
			goTypesMode = "scoped"
		} else {
			mergedEdges = dropTypeUseRefs(mergedEdges, nil)
			mergedEdges = enrichGoTypes(ctx, absRoot, goFiles, mergedEdges)
			goTypesMode = "whole_module"
		}
		phase("go_types", goTypesStart)
	}

	// Re-sort everything identically to the full path so a delta snapshot equals a
	// full reindex of the same HEAD.
	sortStart := time.Now()
	sortFiles(files)
	sortSymbols(mergedSymbols)
	sortEdges(mergedEdges)
	sortRoutes(mergedRoutes)
	phase("sort", sortStart)

	now := time.Now().UTC()
	snapshot := &graph.Snapshot{
		ID:          uuid.NewString(),
		RepoID:      base.repoID,
		CommitSHA:   head,
		CommitRange: baseCommit + ".." + head,
		FileCount:   len(files),
		SymbolCount: len(mergedSymbols),
		EdgeCount:   len(mergedEdges),
		RouteCount:  len(mergedRoutes),
		Metadata: graph.JSONBMap{
			"languages":     languages,
			"mode":          "delta",
			"root":          absRoot,
			"base_commit":   baseCommit,
			"changed_files": len(changedSet),
		},
		CreatedAt: now,
	}
	// Record which go/types path ran (scoped fast path vs whole-module safe path) so
	// the precision provenance of a delta snapshot is auditable.
	if goTypesMode != "" {
		snapshot.Metadata["go_types_mode"] = goTypesMode
	}

	// Re-stamp the new snapshot id onto every merged row, and mint a FRESH per-row
	// primary-key ID for each. The row-level ID column is a GLOBAL primary key
	// (unique across every snapshot, not scoped per-snapshot), so a carried-forward
	// base row reused verbatim would collide with the row it was copied from. The
	// content-stable cross-snapshot identity lives in NodeID, which is preserved —
	// only the surrogate ID and SnapshotID are reassigned. (Newly-parsed rows
	// already have unique IDs; regenerating uniformly is simplest and harmless.)
	for i := range files {
		files[i].SnapshotID = snapshot.ID
		files[i].ID = uuid.NewString()
	}
	for i := range mergedSymbols {
		mergedSymbols[i].SnapshotID = snapshot.ID
		mergedSymbols[i].ID = uuid.NewString()
	}
	for i := range mergedEdges {
		mergedEdges[i].SnapshotID = snapshot.ID
		mergedEdges[i].ID = uuid.NewString()
	}
	for i := range mergedRoutes {
		mergedRoutes[i].SnapshotID = snapshot.ID
		mergedRoutes[i].ID = uuid.NewString()
	}

	repo := &graph.Repo{
		ID:            base.repoID,
		FullName:      repoFullName,
		Root:          absRoot,
		Status:        graph.StatusReady,
		Languages:     languages,
		LastCommit:    head,
		LastIndexedAt: &now,
		Scope:         opts.Scope,
	}
	ensured, err := drv.EnsureRepo(ctx, repo)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: delta ensure repo: %w", err)
	}
	if ensured != nil && ensured.ID != "" && ensured.ID != snapshot.RepoID {
		snapshot.RepoID = ensured.ID
		for i := range mergedSymbols {
			mergedSymbols[i].RepoID = ensured.ID
		}
	}

	persistStart := time.Now()
	if err := drv.SaveSnapshot(ctx, snapshot, files, mergedSymbols, mergedEdges, mergedRoutes); err != nil {
		return nil, Stats{}, fmt.Errorf("index: delta save snapshot: %w", err)
	}
	phase("persist", persistStart)

	if lx != nil {
		// Full lexical rebuild for the new snapshot id (correct + fine for v1).
		lexicalStart := time.Now()
		if err := lx.BuildForSnapshot(snapshot.ID, mergedSymbols); err != nil {
			return nil, Stats{}, fmt.Errorf("index: delta build lexical index: %w", err)
		}
		phase("lexical", lexicalStart)
	}

	stats := Stats{
		Files:        len(files),
		Symbols:      len(mergedSymbols),
		Edges:        len(mergedEdges),
		EdgeKinds:    countEdgeKinds(mergedEdges),
		Routes:       len(mergedRoutes),
		Languages:    languages,
		DurationMS:   time.Since(start).Milliseconds(),
		Mode:         "delta",
		ChangedFiles: len(changedSet),
		TimingsMS:    timings,
	}
	return snapshot, stats, nil
}

// --- shared deterministic ordering (used by both the full and delta paths) ---

func sortFiles(files []graph.File) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
}

func sortSymbols(symbols []graph.CodeSymbol) {
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].Path == symbols[j].Path {
			return symbols[i].StartLine < symbols[j].StartLine
		}
		return symbols[i].Path < symbols[j].Path
	})
}

func sortEdges(edges []graph.DependencyEdge) {
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].FromFile == edges[j].FromFile {
			return edges[i].ToRef < edges[j].ToRef
		}
		return edges[i].FromFile < edges[j].FromFile
	})
}

func sortRoutes(graphRoutes []graph.Route) {
	sort.SliceStable(graphRoutes, func(i, j int) bool {
		if graphRoutes[i].Role != graphRoutes[j].Role {
			return graphRoutes[i].Role < graphRoutes[j].Role
		}
		if graphRoutes[i].PathPattern != graphRoutes[j].PathPattern {
			return graphRoutes[i].PathPattern < graphRoutes[j].PathPattern
		}
		if graphRoutes[i].Method != graphRoutes[j].Method {
			return graphRoutes[i].Method < graphRoutes[j].Method
		}
		return graphRoutes[i].HandlerFile < graphRoutes[j].HandlerFile
	})
}
