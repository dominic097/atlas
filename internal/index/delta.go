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
	"os"
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
func runDelta(ctx context.Context, drv store.StorageDriver, lx *lexical.Index, base *deltaBase, scan *workTreeScan, repoFullName, absRoot, head string, opts Options, start time.Time, seedTimings map[string]int64) (*graph.Snapshot, Stats, error) {
	baseCommit := base.commit

	// reuseSnapshotID predicts SaveSnapshot's (repo_id, commit_sha) idempotency:
	// when the working tree is re-indexed against the SAME commit (the common
	// uncommitted-edit case), SaveSnapshot reuses the base snapshot's id and wipes
	// + rebuilds its child rows. We mirror that decision here so the whole delta —
	// id stamping and the lexical update — is consistent with what gets persisted.
	// A NEW commit means a brand-new snapshot id, which takes the full lexical
	// rebuild path below (correct: there is no prior doc set to update in place).
	reuseSnapshotID := head != "" && head == baseCommit

	// FAST SQL-LEVEL PATH (Phase 1B): when the base snapshot id is being REUSED (the
	// common uncommitted-edit case), the base rows already live under that id, so we
	// can replace ONLY the affected files' rows in-place via ReplaceFileRows instead
	// of loading + re-saving the whole graph. runDeltaSQL guarantees byte-identical
	// parity with a full reindex for the self-contained (non-Go) case and the scoped
	// Go case; it returns ok=false (declines) for the whole-module Go case, a Go
	// deletion, or any situation where the targeted replace cannot be proven exact —
	// in which case we fall back to the whole-graph merge below (never a parity risk).
	//
	// On a NEW commit there is no existing snapshot whose rows we can replace
	// incrementally, so we always take the whole-graph path (it builds a fresh full
	// snapshot). That is rare for the per-edit agent loop this optimizes.
	if reuseSnapshotID {
		snap, stats, ok, err := runDeltaSQL(ctx, drv, lx, base, scan, repoFullName, absRoot, head, opts, start, seedTimings)
		if err != nil {
			return nil, Stats{}, err
		}
		if ok {
			return snap, stats, nil
		}
	}
	return runDeltaWholeGraph(ctx, drv, lx, base, scan, repoFullName, absRoot, head, reuseSnapshotID, opts, start, seedTimings)
}

// runDeltaSQL is the SQL-level incremental delta: it re-parses only the changed
// files, computes the affected file set (changed/added + Go reverse-deps), runs the
// scoped go/types enrichment over a TARGETED edge slice (the changed files' fresh
// edges plus the reverse-dep files' base edges loaded via EdgesByFromFiles — never
// the whole graph), and persists the result with StorageDriver.ReplaceFileRows,
// which deletes only the affected ∪ deleted files' rows and inserts the fresh ones.
// The unchanged base graph is NEVER loaded or rewritten.
//
// It returns ok=false (decline, fall back to the whole-graph merge) when:
//   - any touched Go file forces the WHOLE-MODULE go/types path (small module below
//     the size gate, a Go deletion, or the scoped analyzer declines): that path
//     refines recv_type and regenerates reference edges across EVERY file, so a
//     targeted file-scoped replace cannot reproduce it without the whole graph.
//
// PARITY: for the cases it accepts, the persisted snapshot is byte-identical to a
// full reindex of the same working tree (same rows, same content) — symbols/edges
// for the affected files are re-emitted exactly as a full index would, and every
// unchanged file's rows are left verbatim.
func runDeltaSQL(ctx context.Context, drv store.StorageDriver, lx *lexical.Index, base *deltaBase, scan *workTreeScan, repoFullName, absRoot, head string, opts Options, start time.Time, seedTimings map[string]int64) (*graph.Snapshot, Stats, bool, error) {
	baseCommit := base.commit
	snapshotID := base.snapshot.ID

	timings := seedDeltaTimings(seedTimings)
	phase := func(name string, since time.Time) {
		timings[name] += time.Since(since).Milliseconds()
	}

	changedSet := scan.changedSet() // changed ∪ added — the files to (re)parse
	touched := scan.touchedSet()    // changed ∪ added ∪ deleted — base rows to drop

	goTouched := false
	for p := range touched {
		if parser.LanguageForPath(p) == "go" {
			goTouched = true
			break
		}
	}

	// Re-parse ONLY the changed/added files. Their fresh symbols/edges/routes replace
	// the dropped base rows for the same files; unchanged files are not parsed.
	var (
		newFiles     []graph.File
		newSymbols   []graph.CodeSymbol
		newEdges     []graph.DependencyEdge
		newRawRoutes []routes.RawRoute
	)
	parseStart := time.Now()
	for i := range scan.files {
		if ctx.Err() != nil {
			return nil, Stats{}, false, ctx.Err()
		}
		wf := &scan.files[i]
		if _, isChanged := changedSet[wf.relPath]; !isChanged {
			continue
		}
		f := graph.File{
			ID:        uuid.NewString(),
			Path:      wf.relPath,
			Language:  wf.lang,
			SizeBytes: wf.size,
			Hash:      wf.hash,
		}
		res, parseErr := parser.Parse("", repoFullName, wf.relPath, wf.lang, wf.content)
		if parseErr == nil {
			f.Imports = res.Imports
			newSymbols = append(newSymbols, res.Symbols...)
			newEdges = append(newEdges, res.Edges...)
			newRawRoutes = append(newRawRoutes, routes.ExtractFile(wf.lang, wf.relPath, string(wf.content))...)
		}
		newFiles = append(newFiles, f)
	}
	phase("parse", parseStart)

	// affectedEdgeFiles is the set of files whose EDGE rows are being replaced. It
	// starts as the changed files (their AST edges are freshly parsed) and grows to
	// include Go reverse-dep files when the scoped go/types pass refreshes their
	// edges. Symbols/files/routes are only ever replaced for `touched` — a reverse-dep
	// file's SYMBOLS do not change (only its go_types edges do).
	affectedEdgeFiles := map[string]struct{}{}
	for p := range touched {
		affectedEdgeFiles[p] = struct{}{}
	}

	// Go precision. The scoped path type-checks only the changed packages + their
	// in-module reverse-deps, then refreshes exactly those files' go/types edges. To
	// apply recv_type refinement to a reverse-dep file's (unchanged, base) call edges,
	// we load JUST those files' base edges via EdgesByFromFiles — a targeted read, not
	// the whole graph. The whole-module path cannot be done file-scoped, so we DECLINE
	// (ok=false) and let the caller fall back to the whole-graph merge.
	goTypesMode := ""
	if goTouched {
		goTypesStart := time.Now()

		var goFiles []string // every present Go file (for the size gate + analyzer)
		for i := range scan.files {
			if scan.files[i].lang == "go" {
				goFiles = append(goFiles, scan.files[i].relPath)
			}
		}
		changedGoFiles := map[string]struct{}{}
		for p := range changedSet {
			if parser.LanguageForPath(p) == "go" {
				changedGoFiles[canonicalPath(p)] = struct{}{}
			}
		}
		goDeleted := false
		for p := range scan.deleted {
			if parser.LanguageForPath(p) == "go" {
				goDeleted = true
				break
			}
		}
		bigEnough := len(goFiles) >= gotypes.ScopedMinGoFiles

		if len(changedGoFiles) == 0 || goDeleted || !bigEnough {
			// Whole-module path required — cannot be done as a targeted file replace.
			phase("go_types", goTypesStart)
			return nil, Stats{}, false, nil
		}

		res := gotypes.AnalyzeScoped(ctx, absRoot, len(goFiles), changedGoFiles)
		if !res.OK {
			// Scoped analyzer declined: the whole-module fallback regenerates edges
			// across all files, which the targeted path cannot reproduce.
			phase("go_types", goTypesStart)
			return nil, Stats{}, false, nil
		}

		// Reverse-dep files = analyzed files that were NOT re-parsed (changed). Their
		// edge rows must be refreshed too, so add them to the affected edge set and load
		// their base edges to receive recv_type refinement + regenerated references.
		var reverseDepFiles []string
		for f := range res.AnalyzedFiles {
			cf := canonicalPath(f)
			if _, isChanged := changedSet[cf]; isChanged {
				continue
			}
			if _, alreadyAffected := affectedEdgeFiles[cf]; alreadyAffected {
				continue
			}
			affectedEdgeFiles[cf] = struct{}{}
			reverseDepFiles = append(reverseDepFiles, cf)
		}

		// Load ONLY the reverse-dep files' base edges (targeted), then fold the scoped
		// go/types result over (new changed-file edges + reverse-dep base edges). The
		// drop is scoped to AnalyzedFiles so exactly those files' prior go_types refs are
		// regenerated; every other file's edges are untouched (and never loaded).
		var revBaseEdges []graph.DependencyEdge
		if len(reverseDepFiles) > 0 {
			var lerr error
			revBaseEdges, lerr = drv.EdgesByFromFiles(ctx, snapshotID, reverseDepFiles)
			if lerr != nil {
				return nil, Stats{}, false, fmt.Errorf("index: delta load reverse-dep edges: %w", lerr)
			}
		}
		combined := make([]graph.DependencyEdge, 0, len(newEdges)+len(revBaseEdges))
		combined = append(combined, newEdges...)
		combined = append(combined, revBaseEdges...)
		combined = dropTypeUseRefs(combined, res.AnalyzedFiles)
		newEdges = applyGoTypesResult(res, combined)
		goTypesMode = "scoped"
		phase("go_types", goTypesStart)
	}

	// Routes: resolve the changed files' raw routes against the FULL symbol set so a
	// handler defined in an unchanged file still binds to its file. We do NOT load all
	// symbols — we resolve handler names via the targeted SymbolsByNames read and union
	// with the freshly-parsed symbols. The result matches routes.Resolve over the whole
	// merged symbol set for these raws.
	resolveStart := time.Now()
	resolvedNewRoutes, rerr := resolveDeltaRoutes(ctx, drv, snapshotID, repoFullName, newRawRoutes, newSymbols)
	if rerr != nil {
		return nil, Stats{}, false, fmt.Errorf("index: delta resolve routes: %w", rerr)
	}
	phase("routes", resolveStart)

	// Deterministic ordering so the inserted rows match a full reindex's ordering for
	// the affected files (ListSymbols/ListEdges re-sort by their own ORDER BY, so this
	// is belt-and-suspenders, but keeps in-memory shapes identical to the full path).
	sortStart := time.Now()
	sortFiles(newFiles)
	sortSymbols(newSymbols)
	sortEdges(newEdges)
	sortRoutes(resolvedNewRoutes)
	phase("sort", sortStart)

	// Stamp the (reused) snapshot id + fresh per-row ids onto every inserted row. The
	// row-level ID is a global primary key, so a fresh uuid avoids any collision. The
	// content-stable identity (NodeID) is preserved.
	for i := range newFiles {
		newFiles[i].SnapshotID = snapshotID
		newFiles[i].ID = uuid.NewString()
	}
	for i := range newSymbols {
		newSymbols[i].SnapshotID = snapshotID
		// Reused-snapshot path: the incremental lexical update keys carried-forward
		// docs by their original symbol ids, and a newly-parsed symbol gets a fresh id
		// here that the lexical update will index. The base rows for these files are
		// being DELETED + replaced, so a fresh id cannot collide.
		newSymbols[i].ID = uuid.NewString()
		if base.repoID != "" {
			newSymbols[i].RepoID = base.repoID
		}
	}
	for i := range newEdges {
		newEdges[i].SnapshotID = snapshotID
		newEdges[i].ID = uuid.NewString()
	}
	for i := range resolvedNewRoutes {
		resolvedNewRoutes[i].SnapshotID = snapshotID
		resolvedNewRoutes[i].ID = uuid.NewString()
	}

	// Compute the NEW snapshot totals from the base counts and the per-file deltas.
	// We need the counts of the rows being DELETED (the affected files' base rows) to
	// compute new totals without re-counting the whole snapshot. These are targeted
	// reads (SymbolsByPath / EdgesByFromFiles / routes by handler_file), not whole-graph.
	countStart := time.Now()
	newFileCount, newSymbolCount, newEdgeCount, newRouteCount, cerr := deltaNewCounts(
		ctx, drv, base.snapshot, touched, affectedEdgeFiles, newFiles, newSymbols, newEdges, resolvedNewRoutes)
	if cerr != nil {
		return nil, Stats{}, false, fmt.Errorf("index: delta count: %w", cerr)
	}
	phase("count", countStart)

	// Two drop scopes: fileScope = touched (changed ∪ added ∪ deleted) drives the
	// files/symbols/routes deletes; edgeScope = touched ∪ reverse-deps drives the edges
	// delete. A Go reverse-dep file is in edgeScope but NOT fileScope, so only its
	// (refreshed) edge rows are replaced while its symbol/file rows are preserved. The
	// fresh rows passed in match: files/symbols/routes cover the changed files only,
	// newEdges covers changed + reverse-dep files.
	fileScope := keysOf(touched)
	edgeScope := unionKeys(touched, affectedEdgeFiles)

	forceFull := lx != nil && os.Getenv("ATLAS_LEXICAL_FULL_DELTA") == "1"

	// Lexical inputs MUST be read BEFORE ReplaceFileRows deletes the base rows they
	// reference: the incremental remove-ids and the forceFull full-symbol load both
	// query rows that the replace will drop. Reading them up front also lets persist
	// and lexical run concurrently below (they then share no store reads).
	var (
		lexicalRemoveIDs []string
		lexicalAllSyms   []graph.CodeSymbol
	)
	if lx != nil {
		if forceFull {
			// Escape hatch: rebuild the full lexical index for this id from the full
			// (kept + new) symbol set. This requires the kept symbols, so load them once
			// — only taken when the operator explicitly distrusts the incremental path.
			var lerr error
			lexicalAllSyms, lerr = drv.ListSymbols(ctx, snapshotID)
			if lerr != nil {
				return nil, Stats{}, false, fmt.Errorf("index: delta lexical load: %w", lerr)
			}
		} else {
			var lerr error
			lexicalRemoveIDs, lerr = deltaLexicalRemoveIDs(ctx, drv, snapshotID, touched)
			if lerr != nil {
				return nil, Stats{}, false, fmt.Errorf("index: delta lexical remove ids: %w", lerr)
			}
		}
	}

	// Persist (ReplaceFileRows) and lexical update are independent writers (SQLite db
	// vs Bleve index) — run them CONCURRENTLY (Phase 1C). Both their store reads are
	// already done, so there is no read-after-delete hazard. Output is identical to the
	// sequential order.
	var lexicalFn func() error
	if lx != nil {
		lexicalFn = func() error {
			if forceFull {
				if err := lx.BuildForSnapshot(snapshotID, lexicalAllSyms); err != nil {
					return fmt.Errorf("index: delta build lexical index: %w", err)
				}
				return nil
			}
			// INCREMENTAL: remove the touched files' base docs (by their base symbol
			// ids) and index the new symbols. Carried-forward docs keep their original
			// keys, exactly as the whole-graph delta did.
			if err := lx.UpdateForSnapshot(snapshotID, lexicalRemoveIDs, newSymbols); err != nil {
				return fmt.Errorf("index: delta update lexical index: %w", err)
			}
			return nil
		}
	}

	persistStart := time.Now()
	if err := runPersistAndLexical(ctx, func() error {
		if err := drv.ReplaceFileRows(ctx, snapshotID, fileScope, edgeScope,
			newFiles, newSymbols, newEdges, resolvedNewRoutes,
			newFileCount, newSymbolCount, newEdgeCount, newRouteCount); err != nil {
			return fmt.Errorf("index: delta replace rows: %w", err)
		}
		return nil
	}, lexicalFn); err != nil {
		return nil, Stats{}, false, err
	}
	phase("persist", persistStart)
	phase("write_sqlite", persistStart)
	if lx != nil {
		phase("lexical", persistStart)
	}
	timings["build_symbols_edges"] = timings["go_types"] + timings["routes"] + timings["sort"] + timings["count"]

	languages := languagesFromSnapshot(base.snapshot)
	now := time.Now().UTC()
	snapshot := &graph.Snapshot{
		ID:          snapshotID,
		RepoID:      base.repoID,
		CommitSHA:   head,
		CommitRange: baseCommit + ".." + head,
		FileCount:   newFileCount,
		SymbolCount: newSymbolCount,
		EdgeCount:   newEdgeCount,
		RouteCount:  newRouteCount,
		Metadata: graph.JSONBMap{
			"languages":     languages,
			"mode":          "delta",
			"root":          absRoot,
			"base_commit":   baseCommit,
			"changed_files": len(changedSet),
			"delta_path":    "sql",
		},
		CreatedAt: now,
	}
	if goTypesMode != "" {
		snapshot.Metadata["go_types_mode"] = goTypesMode
	}
	if lx != nil {
		if os.Getenv("ATLAS_LEXICAL_FULL_DELTA") == "1" {
			snapshot.Metadata["lexical_mode"] = "full_rebuild"
		} else {
			snapshot.Metadata["lexical_mode"] = "incremental"
		}
	}

	// Keep the repo row's last-indexed bookkeeping current (matches the whole-graph
	// path's EnsureRepo). Adopt any canonical repo id it resolves.
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
	if _, err := drv.EnsureRepo(ctx, repo); err != nil {
		return nil, Stats{}, false, fmt.Errorf("index: delta ensure repo: %w", err)
	}

	stats := Stats{
		Files:        newFileCount,
		Symbols:      newSymbolCount,
		Edges:        newEdgeCount,
		Routes:       newRouteCount,
		Languages:    languages,
		DurationMS:   time.Since(start).Milliseconds(),
		Mode:         "delta",
		ChangedFiles: len(changedSet),
		TimingsMS:    timings,
	}
	// EdgeKinds requires the full edge set; the SQL path never loads it, so it is
	// reported from a single targeted aggregate rather than a whole-graph scan would.
	stats.EdgeKinds = countEdgeKinds(newEdges)
	if err := persistIndexTelemetry(ctx, drv, snapshot, stats); err != nil {
		return nil, Stats{}, false, err
	}
	return snapshot, stats, true, nil
}

// seedDeltaTimings copies the caller-measured pre-delta phases (resolve_head,
// delta_check) into a fresh timings map and surfaces delta_check under the canonical
// discover_files name, mirroring the whole-graph path.
func seedDeltaTimings(seedTimings map[string]int64) map[string]int64 {
	timings := map[string]int64{}
	for k, v := range seedTimings {
		timings[k] = v
	}
	if dc, ok := seedTimings["delta_check"]; ok {
		timings["discover_files"] = dc
	}
	return timings
}

// resolveDeltaRoutes resolves the changed files' raw routes against the FULL symbol
// set WITHOUT loading every symbol: it resolves producer handler names via the
// targeted SymbolsByNames index read and unions them with the freshly-parsed
// symbols, then runs routes.Resolve. routes.Resolve only consults symbols by name
// (indexSymbolsByName), so this targeted context produces the same resolution a
// whole-symbol-set resolve would for these raws.
func resolveDeltaRoutes(ctx context.Context, drv store.StorageDriver, snapshotID, repoFullName string, raws []routes.RawRoute, newSymbols []graph.CodeSymbol) ([]graph.Route, error) {
	if len(raws) == 0 {
		return nil, nil
	}
	// Collect producer handler names to resolve against the persisted symbol index.
	nameSet := map[string]struct{}{}
	for i := range raws {
		if n := strings.TrimSpace(raws[i].HandlerName); n != "" {
			nameSet[n] = struct{}{}
		}
	}
	syms := make([]graph.CodeSymbol, 0, len(newSymbols))
	syms = append(syms, newSymbols...)
	if len(nameSet) > 0 {
		names := make([]string, 0, len(nameSet))
		for n := range nameSet {
			names = append(names, n)
		}
		base, err := drv.SymbolsByNames(ctx, snapshotID, names)
		if err != nil {
			return nil, err
		}
		// A handler may resolve to a symbol in a CHANGED file. The base index still
		// holds that file's OLD symbols (rows not yet replaced), which could pick a
		// stale file. To match a full reindex, prefer the freshly-parsed symbols: drop
		// base symbols whose owning file is among the changed files (they are superseded
		// by newSymbols already in `syms`).
		changedFiles := map[string]struct{}{}
		for i := range newSymbols {
			changedFiles[canonicalPath(newSymbols[i].Path)] = struct{}{}
		}
		for i := range base {
			if _, isChanged := changedFiles[canonicalPath(base[i].Path)]; isChanged {
				continue
			}
			syms = append(syms, base[i])
		}
	}
	return routes.Resolve(repoFullName, raws, syms), nil
}

// deltaNewCounts computes the snapshot's new total counts (file/symbol/edge/route)
// after a targeted replace, WITHOUT loading the whole graph: new total = base total
// − (rows being deleted for the affected files) + (rows being inserted). The deleted
// counts come from targeted reads scoped to the affected files only.
func deltaNewCounts(ctx context.Context, drv store.StorageDriver, base *graph.Snapshot, touched, affectedEdgeFiles map[string]struct{}, newFiles []graph.File, newSymbols []graph.CodeSymbol, newEdges []graph.DependencyEdge, newRoutes []graph.Route) (int, int, int, int, error) {
	snapshotID := base.ID

	// Files + symbols deleted = base rows whose path ∈ touched.
	delFiles, delSymbols := 0, 0
	for p := range touched {
		fs, err := drv.FilesByPaths(ctx, snapshotID, []string{p})
		if err != nil {
			return 0, 0, 0, 0, err
		}
		delFiles += len(fs)
		ss, err := drv.SymbolsByPath(ctx, snapshotID, p)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		delSymbols += len(ss)
	}

	// Edges deleted = base rows whose from_file ∈ affectedEdgeFiles.
	delEdges := 0
	if len(affectedEdgeFiles) > 0 {
		edges, err := drv.EdgesByFromFiles(ctx, snapshotID, keysOf(affectedEdgeFiles))
		if err != nil {
			return 0, 0, 0, 0, err
		}
		delEdges = len(edges)
	}

	// Routes deleted = base rows whose handler_file ∈ touched. There is no
	// handler-file-indexed reader, so load the snapshot's routes once (route counts are
	// tiny relative to symbols/edges) and count those whose handler is touched.
	delRoutes := 0
	if base.RouteCount > 0 {
		allRoutes, err := drv.ListRoutes(ctx, snapshotID, "")
		if err != nil {
			return 0, 0, 0, 0, err
		}
		for i := range allRoutes {
			if _, hit := touched[canonicalPath(allRoutes[i].HandlerFile)]; hit {
				delRoutes++
			}
		}
	}

	newFileCount := base.FileCount - delFiles + len(newFiles)
	newSymbolCount := base.SymbolCount - delSymbols + len(newSymbols)
	newEdgeCount := base.EdgeCount - delEdges + len(newEdges)
	newRouteCount := base.RouteCount - delRoutes + len(newRoutes)
	return newFileCount, newSymbolCount, newEdgeCount, newRouteCount, nil
}

// deltaLexicalRemoveIDs returns the base symbol ids of every touched file — the docs
// the incremental lexical update must delete before indexing the new symbols. Loaded
// via the targeted SymbolsByPath index, never the whole symbol set.
func deltaLexicalRemoveIDs(ctx context.Context, drv store.StorageDriver, snapshotID string, touched map[string]struct{}) ([]string, error) {
	var ids []string
	for p := range touched {
		syms, err := drv.SymbolsByPath(ctx, snapshotID, p)
		if err != nil {
			return nil, err
		}
		for i := range syms {
			if id := strings.TrimSpace(syms[i].ID); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

// keysOf returns the keys of a set as a slice (order unspecified).
func keysOf(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// unionKeys returns the union of two sets as a slice (deduped, order unspecified).
func unionKeys(a, b map[string]struct{}) []string {
	out := make([]string, 0, len(a)+len(b))
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, m := range []map[string]struct{}{a, b} {
		for k := range m {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// runDeltaWholeGraph is the original whole-graph merge delta: it loads the entire
// base snapshot, drops the touched files' rows, merges the freshly-parsed rows, runs
// the (scoped or whole-module) go/types enrichment over the merged edge set, and
// SaveSnapshots the whole thing. It is the SAFE fallback runDelta takes when the
// SQL-level path declines (whole-module Go, Go deletion, scoped decline) or on a new
// commit (no existing snapshot to replace incrementally). reuseSnapshotID is passed
// in (computed by runDelta) so the id-stamping + lexical decisions match.
func runDeltaWholeGraph(ctx context.Context, drv store.StorageDriver, lx *lexical.Index, base *deltaBase, scan *workTreeScan, repoFullName, absRoot, head string, reuseSnapshotID bool, opts Options, start time.Time, seedTimings map[string]int64) (*graph.Snapshot, Stats, error) {
	baseCommit := base.commit

	// Seed with the pre-delta phases the caller already measured (resolve_head,
	// delta_check) so the delta's timings_ms is a complete profile rather than only
	// the in-runDelta work. delta_check is the working-tree discover+hash+classify
	// scan — also surfaced under the canonical discover_files name.
	timings := seedDeltaTimings(seedTimings)
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

	// For the incremental lexical update (reused-snapshot path only): the docs to
	// delete are the BASE snapshot's symbols whose owning file is touched (changed
	// ∪ added ∪ deleted). Those docs were indexed under this snapshot id keyed by
	// their (base) symbol id; deleting them and re-indexing newSymbols swaps exactly
	// the touched files' docs while every carried-forward doc is left in place. We
	// snapshot these ids BEFORE the re-stamp loop, which would otherwise overwrite
	// them. (Harmless to compute even when reuse is false — it's then unused.)
	var lexicalRemoveIDs []string
	if reuseSnapshotID && lx != nil {
		for i := range baseSymbols {
			if _, hit := touched[canonicalPath(baseSymbols[i].Path)]; hit {
				if id := strings.TrimSpace(baseSymbols[i].ID); id != "" {
					lexicalRemoveIDs = append(lexicalRemoveIDs, id)
				}
			}
		}
	}
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

	// build_symbols_edges is the canonical "construct the final graph from parsed
	// facts" phase, shared with the full path: here it spans loading the base graph,
	// the go/types enrichment, and the deterministic re-sort of the merged rows.
	timings["build_symbols_edges"] = timings["base_load"] + timings["go_types"] + timings["sort"]

	// When the commit is unchanged, SaveSnapshot will reuse the base snapshot id;
	// stamp it here so every child row's SnapshotID and the lexical update key the
	// SAME id SaveSnapshot persists under. On a new commit, mint a fresh id (the
	// full lexical rebuild path handles it).
	snapshotID := uuid.NewString()
	if reuseSnapshotID {
		snapshotID = base.snapshot.ID
	}

	now := time.Now().UTC()
	snapshot := &graph.Snapshot{
		ID:          snapshotID,
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
			"delta_path":    "whole_graph",
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
		// On the reused-snapshot path the lexical index is updated INCREMENTALLY:
		// carried-forward docs are keyed by their original symbol ids and left in
		// place, so the persisted symbol rows MUST keep those same ids or a kept doc
		// would no longer map to its row (engine.lexicalSearch maps a hit's symbol id
		// back through ListSymbols). SaveSnapshot deletes this snapshot's child rows
		// before re-inserting, so reusing an original id cannot collide. On a new
		// snapshot id we re-stamp a fresh id (the full lexical rebuild keys it).
		if !reuseSnapshotID {
			mergedSymbols[i].ID = uuid.NewString()
		}
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

	// Decide the lexical path BEFORE launching the concurrent block so the lexical
	// goroutine never reads snapshot.ID/Metadata while SaveSnapshot mutates them.
	// snapshot.ID is already the reused base id here (stamped above when
	// reuseSnapshotID), so SaveSnapshot's idempotency lookup resolves the same id and
	// does not change it. ATLAS_LEXICAL_FULL_DELTA=1 forces the legacy full-rebuild
	// path even when the incremental one is eligible — an escape hatch / A-B lever.
	forceFull := os.Getenv("ATLAS_LEXICAL_FULL_DELTA") == "1"
	lexicalIncremental := reuseSnapshotID && snapshot.ID == base.snapshot.ID && !forceFull
	persistID := snapshot.ID

	var lexicalFn func() error
	if lx != nil {
		lexicalFn = func() error {
			if lexicalIncremental {
				// INCREMENTAL: delete only the touched files' base docs and index only the
				// changed/added files' fresh symbols; every untouched file's doc is left in
				// place. Search-equivalent to a full rebuild of mergedSymbols because the
				// carried-forward docs keep their original symbol-id keys, which the persisted
				// rows preserve above — at a fraction of the work.
				if err := lx.UpdateForSnapshot(persistID, lexicalRemoveIDs, newSymbols); err != nil {
					return fmt.Errorf("index: delta update lexical index: %w", err)
				}
				return nil
			}
			// NEW snapshot id (new commit, or an unexpected id): no prior doc set to update
			// in place, so build the full lexical index for the persisted id.
			if err := lx.BuildForSnapshot(persistID, mergedSymbols); err != nil {
				return fmt.Errorf("index: delta build lexical index: %w", err)
			}
			return nil
		}
	}

	// Persist + lexical are independent writers — run them CONCURRENTLY (Phase 1C).
	persistStart := time.Now()
	if err := runPersistAndLexical(ctx, func() error {
		if err := drv.SaveSnapshot(ctx, snapshot, files, mergedSymbols, mergedEdges, mergedRoutes); err != nil {
			return fmt.Errorf("index: delta save snapshot: %w", err)
		}
		return nil
	}, lexicalFn); err != nil {
		return nil, Stats{}, err
	}
	phase("persist", persistStart)
	phase("write_sqlite", persistStart)
	if lx != nil {
		phase("lexical", persistStart)
		if lexicalIncremental {
			snapshot.Metadata["lexical_mode"] = "incremental"
		} else {
			snapshot.Metadata["lexical_mode"] = "full_rebuild"
		}
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
	if err := persistIndexTelemetry(ctx, drv, snapshot, stats); err != nil {
		return nil, Stats{}, err
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
