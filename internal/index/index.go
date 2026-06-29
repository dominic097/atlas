// Package index is Atlas's indexing pipeline: it walks a repository working
// tree, parses every supported source file into the shared graph model, derives
// the commit SHA, persists an immutable snapshot through the StorageDriver, and
// builds the lexical (BM25) symbol index for that snapshot.
//
// It is the orchestration seam that ties parser + store + lexical together. The
// walk/scan shape is adapted from the proven indexing engine
// (internal/service/code_intelligence_service.go: scanRepository ~1321 /
// parseRepoFile ~1403): a filepath.WalkDir that prunes vendored/build dirs,
// skips unsupported or oversized files, parses the rest, and accumulates
// files/symbols/edges before a single snapshot save.
package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dominic097/atlas/internal/embed"
	"github.com/dominic097/atlas/internal/gotypes"
	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/lexical"
	"github.com/dominic097/atlas/internal/parser"
	"github.com/dominic097/atlas/internal/routes"
	"github.com/dominic097/atlas/internal/store"
)

// maxFileBytes is the per-file size ceiling; files larger than this are skipped
// (generated bundles / vendored blobs blow up the parser for no graph value).
const maxFileBytes = 1 << 20 // 1 MB

// maxDocBytes is the larger ceiling for binary office documents: they embed media
// so the package is big, but only the (small) XML text is read, so the source-file
// cap would wrongly skip an ordinary slide deck.
const maxDocBytes = 64 << 20 // 64 MB

// fileSizeCap returns the size ceiling for a language: the document cap for office
// formats, otherwise the source-file cap.
func fileSizeCap(language string) int64 {
	if parser.IsDocumentFormat(language) {
		return maxDocBytes
	}
	return maxFileBytes
}

// Options configures a single indexing run.
type Options struct {
	// Reindex forces a full rebuild. The local SQLite tier already rebuilds the
	// snapshot's child rows idempotently per (repo_id, commit_sha), so this is a
	// hint surfaced in Stats.Mode rather than a divergent code path today.
	Reindex bool
	// Scope stamps the tenant/org id onto the indexed repo so EnsureRepo keys it by
	// (scope, full_name). Empty means single-tenant / no scope — the local default.
	Scope string
	// EnableVectors runs the OPTIONAL embedding pass after the snapshot is persisted:
	// embed each symbol's text with embed.NewProvider() and SaveEmbeddings. Off by
	// default — the deterministic lexical core is unchanged. A provider/embeddings
	// failure is non-fatal (logged, indexing still succeeds).
	EnableVectors bool
	// SkipPaths are absolute directory paths pruned from the walk, in addition to
	// the by-name skipDirs. The segmented (multi-repo) path uses this to index an
	// outer repo's OWN loose files without descending into the nested repos it also
	// indexes separately — so no file is indexed twice and memory stays bounded.
	SkipPaths []string
	// RespectGitignore prunes paths git considers ignored (build output, caches,
	// vendored runtimes) from the walk, so Atlas indexes source-of-truth files only.
	// It asks git itself (one `git ls-files` call), so .gitignore semantics — nested
	// rules, negation, exclude files — are exact; on a non-git tree or without a git
	// binary it is a silent no-op (everything is indexed). Off by default to keep
	// library/SDK snapshots stable; the CLI turns it on.
	RespectGitignore bool
}

// Stats is the human-facing summary of an indexing run.
type Stats struct {
	Files      int            `json:"files"`
	Symbols    int            `json:"symbols"`
	Edges      int            `json:"edges"`
	EdgeKinds  map[string]int `json:"edge_kinds,omitempty"`
	Routes     int            `json:"routes"`
	Languages  map[string]int `json:"languages"`
	DurationMS int64          `json:"duration_ms"`
	Mode       string         `json:"mode"`
	// ChangedFiles is the number of files re-parsed on a delta run (0 on full /
	// reindex). It is purely additive — the engine maps Stats field-by-field, so a
	// new field is safe and simply unmapped until the engine opts in.
	ChangedFiles int              `json:"changed_files"`
	TimingsMS    map[string]int64 `json:"timings_ms,omitempty"`
}

// skipDirs are directory names pruned wholesale during the walk: VCS metadata,
// third-party dependency caches, and Atlas's own on-disk state.
//
// Build-output names ("build"/"out"/"target"/"dist") are deliberately NOT
// skipped: they collide with real SOURCE-package directories (e.g. bazel's whole
// Java tree lives under com/google/devtools/build/...), and genuine build
// artifacts are non-source extensions the parser already ignores. Correctness of
// the graph beats a marginal walk-speed win.
var skipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	".venv":        {},
	"venv":         {},
	"__pycache__":  {},
	".next":        {},
	".nuxt":        {},
	".svelte-kit":  {},
	".atlas":       {},
	"graphify-out": {},
	".testdata":    {},
	// Agent scratch: .claude/worktrees holds dozens of full duplicate repo
	// checkouts. Walking them indexes every tracked repo many times over (and is
	// a primary cause of OOM when a workspace root is indexed). Never source.
	".claude": {},
	// VCS metadata and editor/build caches — never source, frequently enormous.
	".hg":           {},
	".svn":          {},
	".idea":         {},
	".vscode":       {},
	".gradle":       {},
	".mvn":          {},
	".tox":          {},
	".cache":        {},
	".pytest_cache": {},
	".mypy_cache":   {},
	".ruff_cache":   {},
	".turbo":        {},
	".parcel-cache": {},
}

// Run indexes the repository rooted at root and persists a snapshot.
//
// It walks root (pruning skipDirs), parses every parser-supported file under the
// size ceiling, accumulates graph.File/CodeSymbol/DependencyEdge with the file
// path RELATIVE to root, derives the commit SHA (git rev-parse if available,
// else "working-tree"), stamps the new snapshot's ID onto every child row,
// ensures the repo, saves the snapshot in one transaction, and finally builds
// the lexical index for the snapshot's symbols.
func Run(ctx context.Context, drv store.StorageDriver, lx *lexical.Index, repoID, repoFullName, root string, opts Options) (*graph.Snapshot, Stats, error) {
	start := time.Now()
	timings := map[string]int64{}
	phase := func(name string, since time.Time) {
		timings[name] += time.Since(since).Milliseconds()
	}

	if drv == nil {
		return nil, Stats{}, fmt.Errorf("index: storage driver is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, Stats{}, fmt.Errorf("index: repo root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: resolve root %q: %w", root, err)
	}
	if info, statErr := os.Stat(absRoot); statErr != nil {
		return nil, Stats{}, fmt.Errorf("index: stat root %q: %w", absRoot, statErr)
	} else if !info.IsDir() {
		return nil, Stats{}, fmt.Errorf("index: root %q is not a directory", absRoot)
	}

	phaseStart := time.Now()
	head := resolveCommitSHA(ctx, absRoot)
	phase("resolve_head", phaseStart)

	// Try an incremental delta. Change detection is WORKING-TREE-aware: it compares
	// the working tree (content-hashed) against the base snapshot's stored hashes,
	// so an uncommitted edit, an untracked new file, or a deletion is detected even
	// when no new commit exists. This is the per-edit update an agent runs after
	// every task; a commit-only diff would miss it and noop a stale graph.
	//
	// The run is a genuine no-op ONLY when the working tree matches the base
	// snapshot exactly. On any error we fall through to the full walk below, so a
	// delta never fails the run.
	if !opts.Reindex {
		phaseStart = time.Now()
		if base, baseErr := resolveDeltaBase(ctx, drv, repoFullName); baseErr == nil && base != nil {
			if scan, scanErr := scanWorkTree(ctx, drv, base.snapshot.ID, absRoot); scanErr == nil {
				// The scan (walk + hash + classify) is the delta_check cost; record it
				// once here so neither the noop, delta, nor fall-through exit double-counts.
				phase("delta_check", phaseStart)
				if scan.noChanges() {
					stats := Stats{
						Files:      base.snapshot.FileCount,
						Symbols:    base.snapshot.SymbolCount,
						Edges:      base.snapshot.EdgeCount,
						Routes:     base.snapshot.RouteCount,
						Languages:  languagesFromSnapshot(base.snapshot),
						DurationMS: time.Since(start).Milliseconds(),
						Mode:       "noop",
						TimingsMS:  timings,
					}
					if err := persistIndexTelemetry(ctx, drv, base.snapshot, stats); err != nil {
						return nil, Stats{}, err
					}
					return base.snapshot, stats, nil
				}
				snap, stats, derr := runDelta(ctx, drv, lx, base, scan, repoFullName, absRoot, head, opts, start, timings)
				if derr == nil {
					return snap, stats, nil
				}
				// Delta failed mid-flight (store hiccup): fall back to full. The
				// delta_check phase is already recorded above.
			} else {
				// No usable base scan (base resolved but scan errored): record the
				// time spent before falling through to the full walk.
				phase("delta_check", phaseStart)
			}
		} else {
			// No delta base (first index, or base lookup failed): record the probe time.
			phase("delta_check", phaseStart)
		}
	}

	var (
		candidates []indexCandidate
		languages  = map[string]int{}
	)

	// Absolute paths to prune from the walk: the segmented path's nested-repo roots,
	// plus (when enabled) everything git ignores. Fully-ignored directories collapse
	// to one entry so a large ignored tree is skipped at its boundary, not walked.
	skipPathSet := make(map[string]struct{}, len(opts.SkipPaths))
	for _, p := range opts.SkipPaths {
		if ap, aerr := filepath.Abs(p); aerr == nil {
			skipPathSet[filepath.Clean(ap)] = struct{}{}
		}
	}
	gitHandledIgnore := false
	if opts.RespectGitignore {
		phaseStart = time.Now()
		gi := gitIgnoredPaths(ctx, absRoot)
		for p := range gi {
			skipPathSet[p] = struct{}{}
		}
		gitHandledIgnore = gi != nil // non-nil only inside a real git repo
		phase("gitignore_scan", phaseStart)
	}
	// `.atlasignore` is honored in ANY folder (git or not) — this is how a plain
	// documents directory excludes files. It inherits `.gitignore` only for a
	// non-git folder under RespectGitignore; inside a git repo the exact,
	// tracked-file-aware gitIgnoredPaths above already covered .gitignore.
	ignorer := loadIgnoreMatcher(absRoot, opts.RespectGitignore && !gitHandledIgnore)

	phaseStart = time.Now()
	walkErr := filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Honour cancellation between files.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if entry.IsDir() {
			if len(skipPathSet) > 0 {
				if _, skip := skipPathSet[filepath.Clean(path)]; skip {
					return filepath.SkipDir
				}
			}
			if _, skip := skipDirs[entry.Name()]; skip {
				return filepath.SkipDir
			}
			if ignorer.ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinks and other non-regular files.
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		// A git-ignored FILE inside an otherwise-tracked directory (the dir-level
		// prune above only catches fully-ignored directories).
		if len(skipPathSet) > 0 {
			if _, skip := skipPathSet[filepath.Clean(path)]; skip {
				return nil
			}
		}
		// An .atlasignore/.gitignore file rule (git-independent).
		if ignorer.ignored(rel, false) {
			return nil
		}

		lang := parser.LanguageForPath(rel)
		if !parser.Supported(lang) {
			return nil
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			// A file that vanished mid-walk is not fatal to the whole index.
			return nil
		}
		if info.Size() > fileSizeCap(lang) {
			return nil
		}

		candidates = append(candidates, indexCandidate{
			absPath: path,
			relPath: rel,
			lang:    lang,
			size:    info.Size(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, Stats{}, fmt.Errorf("index: walk %q: %w", absRoot, walkErr)
	}
	// discover_files: the working-tree walk that selects parseable candidates. This
	// is the canonical phase name shared with the delta path's scan (delta_check).
	phase("walk", phaseStart)
	phase("discover_files", phaseStart)

	// Live progress (nil-safe): the count of candidate files is now known, so a
	// watcher can render "parsed K / N" instead of a bare elapsed timer.
	pc := ProgressFromContext(ctx)
	pc.SetFilesTotal(len(candidates))
	pc.SetPhase("parse")

	phaseStart = time.Now()
	files, symbols, edges, rawRoutes, goFiles := parseCandidates(ctx, repoID, repoFullName, candidates, languages)
	if ctx.Err() != nil {
		return nil, Stats{}, ctx.Err()
	}
	phase("parse", phaseStart)
	pc.SetPhase("go_types")

	// build_symbols_edges spans every post-parse graph-construction step: precise
	// go/types enrichment, deterministic ordering, and route resolution. It is the
	// canonical "turn parsed facts into the final graph" phase a profile reads.
	buildStart := time.Now()

	// Precise Go analysis (go/types): refine heuristic recv_type on call edges and
	// add real type-use reference edges. Non-regressing — on any miss the heuristic
	// edges stand untouched (see enrichGoTypes).
	phaseStart = time.Now()
	edges = enrichGoTypes(ctx, absRoot, goFiles, edges)
	phase("go_types", phaseStart)

	// Link indexed documents (decks, specs, images) to the in-repo code they
	// reference, so a query can cross from a document to the package it describes.
	phaseStart = time.Now()
	edges = append(edges, linkDocuments(symbols, files)...)
	phase("doc_links", phaseStart)

	// Deterministic ordering so identical trees produce identical snapshots. The
	// same helpers order the delta path's merged rows, guaranteeing a delta
	// snapshot equals a full reindex of the same HEAD.
	phaseStart = time.Now()
	sortFiles(files)
	sortSymbols(symbols)
	sortEdges(edges)
	phase("sort", phaseStart)

	// Resolve raw route facts now that the full symbol set is available: producer
	// handler names bind to their defining file, consumer calls keep their calling
	// file. Sorted for deterministic snapshots.
	phaseStart = time.Now()
	graphRoutes := routes.Resolve(repoFullName, rawRoutes, symbols)
	sortRoutes(graphRoutes)
	phase("routes", phaseStart)

	phase("build_symbols_edges", buildStart)

	commitSHA := head

	mode := "full"
	if opts.Reindex {
		mode = "reindex"
	}

	snapshot := &graph.Snapshot{
		ID:          uuid.NewString(),
		RepoID:      repoID,
		CommitSHA:   commitSHA,
		FileCount:   len(files),
		SymbolCount: len(symbols),
		EdgeCount:   len(edges),
		RouteCount:  len(graphRoutes),
		Metadata: graph.JSONBMap{
			"languages": languages,
			"mode":      mode,
			"root":      absRoot,
		},
		CreatedAt: time.Now().UTC(),
	}

	// Stamp the snapshot id onto every child row before persisting.
	for i := range files {
		files[i].SnapshotID = snapshot.ID
	}
	for i := range symbols {
		symbols[i].SnapshotID = snapshot.ID
	}
	for i := range edges {
		edges[i].SnapshotID = snapshot.ID
	}
	for i := range graphRoutes {
		graphRoutes[i].SnapshotID = snapshot.ID
	}

	now := time.Now().UTC()
	repo := &graph.Repo{
		ID:            repoID,
		FullName:      repoFullName,
		Root:          absRoot,
		Status:        graph.StatusReady,
		Languages:     languages,
		LastCommit:    commitSHA,
		LastIndexedAt: &now,
		Scope:         opts.Scope,
	}
	phaseStart = time.Now()
	ensured, err := drv.EnsureRepo(ctx, repo)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: ensure repo: %w", err)
	}
	phase("ensure_repo", phaseStart)
	// EnsureRepo may resolve a pre-existing repo id (lookup by scope+full_name);
	// adopt it so the snapshot binds to the canonical repo row and every symbol's
	// RepoID stays consistent.
	if ensured != nil && ensured.ID != "" && ensured.ID != repoID {
		snapshot.RepoID = ensured.ID
		for i := range symbols {
			symbols[i].RepoID = ensured.ID
		}
	}

	// Persist the snapshot and build the lexical index. These two writes are
	// independent (disjoint stores, read-only shared symbols), so they run
	// CONCURRENTLY (Phase 1C) — the slower hides behind the faster. The persisted
	// snapshot and lexical index are identical to running them sequentially. The
	// combined wall-clock spans the overlapped block; persist/lexical are reported as
	// the same combined duration since they overlap.
	var lexicalFn func() error
	if lx != nil {
		lexicalFn = func() error {
			// Build the lexical index against the persisted snapshot id. The symbols now
			// carry the snapshot id; pass them straight through.
			if err := lx.BuildForSnapshot(snapshot.ID, symbols); err != nil {
				return fmt.Errorf("index: build lexical index: %w", err)
			}
			return nil
		}
	}
	pc.SetPhase("persist")
	phaseStart = time.Now()
	if err := runPersistAndLexical(ctx, func() error {
		if err := drv.SaveSnapshot(ctx, snapshot, files, symbols, edges, graphRoutes); err != nil {
			return fmt.Errorf("index: save snapshot: %w", err)
		}
		return nil
	}, lexicalFn); err != nil {
		return nil, Stats{}, err
	}
	phase("persist", phaseStart)
	phase("write_sqlite", phaseStart)
	if lx != nil {
		phase("lexical", phaseStart)
	}

	// OPTIONAL, gated semantic-search pass. Off by default; only runs with
	// --enable-vectors. Non-fatal by design — a provider/embeddings hiccup must
	// never fail the deterministic index.
	if opts.EnableVectors {
		phaseStart = time.Now()
		buildEmbeddings(ctx, drv, snapshot.ID, symbols)
		phase("embeddings", phaseStart)
	}

	stats := Stats{
		Files:      len(files),
		Symbols:    len(symbols),
		Edges:      len(edges),
		EdgeKinds:  countEdgeKinds(edges),
		Routes:     len(graphRoutes),
		Languages:  languages,
		DurationMS: time.Since(start).Milliseconds(),
		Mode:       mode,
		TimingsMS:  timings,
	}
	if err := persistIndexTelemetry(ctx, drv, snapshot, stats); err != nil {
		return nil, Stats{}, err
	}
	pc.SetPhase("done")
	return snapshot, stats, nil
}

func persistIndexTelemetry(ctx context.Context, drv store.StorageDriver, snapshot *graph.Snapshot, stats Stats) error {
	if snapshot == nil {
		return nil
	}
	if snapshot.Metadata == nil {
		snapshot.Metadata = graph.JSONBMap{}
	}
	if stats.Mode != "noop" {
		snapshot.Metadata["mode"] = stats.Mode
		snapshot.Metadata["duration_ms"] = stats.DurationMS
		snapshot.Metadata["timings_ms"] = stats.TimingsMS
		snapshot.Metadata["changed_files"] = stats.ChangedFiles
		snapshot.Metadata["edge_kinds"] = stats.EdgeKinds
		snapshot.Metadata["languages"] = stats.Languages
	}
	snapshot.Metadata["last_index_mode"] = stats.Mode
	snapshot.Metadata["last_index_duration_ms"] = stats.DurationMS
	snapshot.Metadata["last_index_timings_ms"] = stats.TimingsMS
	snapshot.Metadata["last_index_changed_files"] = stats.ChangedFiles
	if err := drv.UpdateSnapshotMetadata(ctx, snapshot.ID, snapshot.Metadata); err != nil {
		return fmt.Errorf("index: update snapshot telemetry: %w", err)
	}
	return nil
}

func languagesFromSnapshot(snap *graph.Snapshot) map[string]int {
	if snap == nil || snap.Metadata == nil {
		return nil
	}
	raw, ok := snap.Metadata["languages"]
	if !ok {
		return nil
	}
	switch langs := raw.(type) {
	case map[string]int:
		if len(langs) == 0 {
			return nil
		}
		out := make(map[string]int, len(langs))
		for k, v := range langs {
			out[k] = v
		}
		return out
	case map[string]any:
		out := make(map[string]int, len(langs))
		for k, v := range langs {
			switch n := v.(type) {
			case int:
				out[k] = n
			case int64:
				out[k] = int(n)
			case float64:
				out[k] = int(n)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

type indexCandidate struct {
	absPath string
	relPath string
	lang    string
	size    int64
}

type parseCandidateResult struct {
	file      graph.File
	symbols   []graph.CodeSymbol
	edges     []graph.DependencyEdge
	routes    []routes.RawRoute
	goFile    string
	language  string
	indexable bool
}

func parseCandidates(ctx context.Context, repoID, repoFullName string, candidates []indexCandidate, languages map[string]int) (
	[]graph.File, []graph.CodeSymbol, []graph.DependencyEdge, []routes.RawRoute, []string,
) {
	if len(candidates) == 0 {
		return nil, nil, nil, nil, nil
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}

	// Parse each candidate concurrently, writing into a per-index result slot. Each
	// parser.Parse call constructs its OWN parsing state — Go uses go/parser (per
	// call) and the tree-sitter path builds a fresh tree_sitter.NewParser per Parse
	// (see internal/parser parseTreeSitter), so no parser is shared across goroutines.
	// Writing to out[i] (not a shared append) keeps the pre-sort accumulation order
	// equal to the candidate order, so the snapshot is byte-identical to a serial run
	// after the deterministic sort the caller applies.
	out := make([]parseCandidateResult, len(candidates))
	indexes := make(chan int)
	pc := ProgressFromContext(ctx) // nil-safe; bumped once per parsed file below
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for idx := range indexes {
				if ctx.Err() != nil {
					continue
				}
				out[idx] = parseCandidate(ctx, repoID, repoFullName, candidates[idx])
				pc.AddParsed(len(out[idx].symbols))
			}
		}()
	}
	for i := range candidates {
		if ctx.Err() != nil {
			break
		}
		indexes <- i
	}
	close(indexes)
	wg.Wait()

	// Reduce in candidate order (deterministic) for stable accumulation.
	var (
		files     []graph.File
		symbols   []graph.CodeSymbol
		edges     []graph.DependencyEdge
		rawRoutes []routes.RawRoute
		goFiles   []string
	)
	for i := range out {
		r := &out[i]
		if !r.indexable {
			continue
		}
		files = append(files, r.file)
		languages[r.language]++
		if r.goFile != "" {
			goFiles = append(goFiles, r.goFile)
		}
		symbols = append(symbols, r.symbols...)
		edges = append(edges, r.edges...)
		rawRoutes = append(rawRoutes, r.routes...)
	}
	return files, symbols, edges, rawRoutes, goFiles
}

func parseCandidate(ctx context.Context, repoID, repoFullName string, c indexCandidate) parseCandidateResult {
	if ctx.Err() != nil {
		return parseCandidateResult{}
	}
	content, readErr := os.ReadFile(c.absPath)
	if readErr != nil {
		// Unreadable single file: skip, don't abort the scan.
		return parseCandidateResult{}
	}
	res, parseErr := parser.Parse(repoID, repoFullName, c.relPath, c.lang, content)
	if parseErr != nil {
		// A parse failure on one file should not sink the whole snapshot.
		return parseCandidateResult{}
	}
	out := parseCandidateResult{
		file: graph.File{
			ID:        uuid.NewString(),
			Path:      c.relPath,
			Language:  c.lang,
			SizeBytes: c.size,
			Hash:      hashContent(content),
			Imports:   res.Imports,
		},
		symbols:   res.Symbols,
		edges:     res.Edges,
		routes:    routes.ExtractFile(c.lang, c.relPath, string(content)),
		language:  c.lang,
		indexable: true,
	}
	if c.lang == "go" {
		out.goFile = c.relPath
	}
	return out
}

func countEdgeKinds(edges []graph.DependencyEdge) map[string]int {
	counts := make(map[string]int)
	for _, edge := range edges {
		kind := string(edge.Kind)
		if kind == "" {
			kind = "unknown"
		}
		counts[kind]++
	}
	return counts
}

// buildEmbeddings runs the optional embedding pass: it builds embed.NewProvider()
// (offline Hashing by default, HTTP when ATLAS_EMBED_URL is set), embeds each
// symbol's text (Name + " " + Signature + " " + Doc), and persists the vectors
// via SaveEmbeddings. Every failure mode is non-fatal — it logs to stderr and
// returns, leaving the deterministic snapshot intact. Symbols with no id are
// skipped (they have no stable key to retrieve them by).
func buildEmbeddings(ctx context.Context, drv store.StorageDriver, snapshotID string, symbols []graph.CodeSymbol) {
	if len(symbols) == 0 {
		return
	}
	provider := embed.NewProvider()

	texts := make([]string, 0, len(symbols))
	ids := make([]string, 0, len(symbols))
	for i := range symbols {
		s := &symbols[i]
		if strings.TrimSpace(s.ID) == "" {
			continue
		}
		texts = append(texts, symbolText(s))
		ids = append(ids, s.ID)
	}
	if len(texts) == 0 {
		return
	}

	vecs, err := provider.Embed(ctx, texts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "index: embeddings skipped (provider %q failed): %v\n", provider.Name(), err)
		return
	}
	if len(vecs) != len(ids) {
		fmt.Fprintf(os.Stderr, "index: embeddings skipped (provider %q returned %d vectors for %d symbols)\n", provider.Name(), len(vecs), len(ids))
		return
	}

	embs := make([]graph.SymbolEmbedding, 0, len(ids))
	for i := range ids {
		embs = append(embs, graph.SymbolEmbedding{
			SnapshotID: snapshotID,
			SymbolID:   ids[i],
			Dim:        len(vecs[i]),
			Vector:     vecs[i],
		})
	}
	if err := drv.SaveEmbeddings(ctx, snapshotID, embs); err != nil {
		fmt.Fprintf(os.Stderr, "index: embeddings skipped (save failed): %v\n", err)
	}
}

// symbolText is the document an embedder sees for a symbol: its name, signature,
// and doc joined with spaces. It mirrors the lexical index's searched fields so
// semantic and lexical search rank over comparable content.
func symbolText(s *graph.CodeSymbol) string {
	return strings.TrimSpace(s.Name + " " + s.Signature + " " + s.Doc)
}

// enrichGoTypes runs the precise go/types analyzer over the repo and folds its
// results into the accumulated edge set:
//
//   - recv_type refinement: every result CallRecv (file\x00line\x00callee ->
//     precise receiver base type) overwrites the heuristic recv_type on the
//     matching Go EdgeCalls edge. Edges with no precise match keep their heuristic
//     value, so this is a pure refinement — never a downgrade.
//   - reference edges: each RefEdge (a type-use, not a call) is appended as a new
//     graph.EdgeReferences edge so `refs` returns true references, not just
//     callers.
//
// If the analyzer declines (oversized repo, load error, panic, timeout) it
// returns OK:false and we return the edges unchanged — the heuristic stands and
// there is no regression. Returns the (possibly augmented) edge slice.
func enrichGoTypes(ctx context.Context, absRoot string, goFiles []string, edges []graph.DependencyEdge) []graph.DependencyEdge {
	if len(goFiles) == 0 {
		return edges
	}
	res := gotypes.Analyze(ctx, absRoot, len(goFiles))
	if !res.OK {
		return edges
	}
	return applyGoTypesResult(res, edges)
}

// applyGoTypesResult folds a gotypes.Result into the edge set: it refines recv_type
// on matching Go call edges (a pure refinement — a miss leaves the heuristic value
// untouched) and appends one type-use reference edge per RefEdge. It is shared by
// the whole-module (enrichGoTypes) and scoped (enrichGoTypesScoped) paths so both
// produce byte-identical edge shapes for the files they cover.
func applyGoTypesResult(res gotypes.Result, edges []graph.DependencyEdge) []graph.DependencyEdge {
	// Index precise receiver types by (relfile\x00line\x00callee). The AST call
	// edge carries the same file (repo-relative), Line, and ToRef (bare callee),
	// so this key joins them exactly.
	recvByCall := make(map[string]string, len(res.CallRecvs))
	for _, cr := range res.CallRecvs {
		if cr.Type == "" {
			continue
		}
		recvByCall[cr.File+"\x00"+strconv.Itoa(cr.Line)+"\x00"+cr.Callee] = cr.Type
	}

	if len(recvByCall) > 0 {
		for i := range edges {
			e := &edges[i]
			if e.Language != "go" || e.Kind != graph.EdgeCalls {
				continue
			}
			key := e.FromFile + "\x00" + strconv.Itoa(e.Line) + "\x00" + e.ToRef
			precise, ok := recvByCall[key]
			if !ok {
				continue
			}
			if e.Metadata == nil {
				e.Metadata = graph.JSONBMap{}
			}
			// Record whether go/types changed the heuristic value (a true
			// refinement) or merely confirmed it. Either way recv_source marks
			// this receiver as type-checker-grounded, not heuristic — so the
			// precision of any given edge is auditable after the fact.
			if prev, _ := e.Metadata["recv_type"].(string); prev != precise {
				e.Metadata["recv_type_heuristic"] = prev
			}
			e.Metadata["recv_type"] = precise
			e.Metadata["recv_source"] = "go_types"
		}
	}

	// Append type-use reference edges. These have no caller-side counterpart in the
	// AST parser, so there is nothing to dedup against the call edges.
	for _, r := range res.RefEdges {
		edges = append(edges, graph.DependencyEdge{
			ID:         uuid.NewString(),
			FromFile:   r.FromFile,
			FromSymbol: r.FromSymbol,
			ToRef:      r.ToRef,
			Kind:       graph.EdgeReferences,
			Language:   "go",
			Line:       r.Line,
			Metadata: graph.JSONBMap{
				"qualified_ref":  r.Qualified,
				"source":         "go_types",
				"analysis_level": "type_use",
			},
		})
	}
	return edges
}

// enrichGoTypesScoped is the INCREMENTAL Go enrichment for the delta path. It
// type-checks only the changed packages + their in-module reverse-deps (via
// gotypes.AnalyzeScoped) instead of the whole module, then refreshes the go/types
// edges for exactly the analyzed files — recv_type overrides and regenerated
// type-use reference edges — while every untouched file's carried-forward edges are
// left intact.
//
// It returns (edges, true) on success and (edges-unchanged, false) when the scoped
// analyzer declines (OK:false). On false the caller MUST fall back to the
// whole-module enrichGoTypes so precision is never regressed.
//
// changedFiles are the canonical (ToSlash) repo-relative paths of the re-parsed
// files (changed ∪ added). The reference-edge drop is scoped to res.AnalyzedFiles —
// the full set the analyzer is authoritative for (changed packages + reverse-deps) —
// so reverse-dep files whose refs could have shifted are regenerated, and no other
// file's refs are touched.
func enrichGoTypesScoped(ctx context.Context, absRoot string, goFiles []string, changedFiles map[string]struct{}, edges []graph.DependencyEdge) ([]graph.DependencyEdge, bool) {
	if len(goFiles) == 0 || len(changedFiles) == 0 {
		return edges, false
	}
	res := gotypes.AnalyzeScoped(ctx, absRoot, len(goFiles), changedFiles)
	if !res.OK {
		return edges, false
	}
	// Drop only the analyzed files' prior type-use refs; applyGoTypesResult then
	// regenerates exactly those plus refreshing their recv_type overrides.
	edges = dropTypeUseRefs(edges, res.AnalyzedFiles)
	return applyGoTypesResult(res, edges), true
}

// hashContent returns the lowercase sha256 hex digest of a file's bytes.
func hashContent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// resolveCommitSHA returns the working tree's HEAD commit. The hot path reads
// .git/HEAD directly to keep no-change reindex cheap; it falls back to
// `git rev-parse HEAD` for layouts the direct reader does not understand.
// gitIgnoredPaths returns the set of ABSOLUTE paths under absRoot that git
// considers ignored. `--directory` collapses a fully-ignored directory to one
// entry, so a 3GB ignored tree is pruned at its boundary instead of walked. It
// shells out to git — the same dependency the index already uses for HEAD — and
// returns nil when absRoot is not a git repo or git is unavailable, so indexing
// degrades to "index everything" rather than failing. Tracked files are never
// returned (git does not ignore tracked paths), matching git's own model.
func gitIgnoredPaths(ctx context.Context, absRoot string) map[string]struct{} {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, gitBin,
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	cmd.Dir = absRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	set := make(map[string]struct{})
	for _, rel := range strings.Split(string(out), "\x00") {
		rel = strings.TrimSuffix(strings.TrimSpace(rel), "/")
		if rel == "" {
			continue
		}
		set[filepath.Clean(filepath.Join(absRoot, rel))] = struct{}{}
	}
	return set
}

func resolveCommitSHA(ctx context.Context, root string) string {
	if sha := readGitHead(root); sha != "" {
		return sha
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return workingTreeSHA
	}
	cmd := exec.CommandContext(ctx, gitBin, "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return workingTreeSHA
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return workingTreeSHA
	}
	return sha
}

func readGitHead(root string) string {
	gitDir := findGitDir(root)
	if gitDir == "" {
		return ""
	}
	headBytes, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(headBytes))
	if isHexSHA(head) {
		return head
	}
	const refPrefix = "ref:"
	if !strings.HasPrefix(head, refPrefix) {
		return ""
	}
	ref := strings.TrimSpace(strings.TrimPrefix(head, refPrefix))
	if ref == "" || strings.Contains(ref, "..") || filepath.IsAbs(ref) {
		return ""
	}
	if b, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(ref))); err == nil {
		if sha := strings.TrimSpace(string(b)); isHexSHA(sha) {
			return sha
		}
	}
	return readPackedRef(filepath.Join(gitDir, "packed-refs"), ref)
}

func findGitDir(root string) string {
	dir, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		info, statErr := os.Stat(gitPath)
		if statErr == nil {
			if info.IsDir() {
				return gitPath
			}
			if b, err := os.ReadFile(gitPath); err == nil {
				text := strings.TrimSpace(string(b))
				const gitdirPrefix = "gitdir:"
				if strings.HasPrefix(text, gitdirPrefix) {
					p := strings.TrimSpace(strings.TrimPrefix(text, gitdirPrefix))
					if p == "" {
						return ""
					}
					if !filepath.IsAbs(p) {
						p = filepath.Join(dir, p)
					}
					return filepath.Clean(p)
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func readPackedRef(path, ref string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref && isHexSHA(fields[0]) {
			return fields[0]
		}
	}
	return ""
}

func isHexSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}
