// Package index is Atlas's indexing pipeline: it walks a repository working
// tree, parses every supported source file into the shared graph model, derives
// the commit SHA, persists an immutable snapshot through the StorageDriver, and
// builds the lexical (BM25) symbol index for that snapshot.
//
// It is the orchestration seam that ties parser + store + lexical together. The
// walk/scan shape is ported from the proven aziron-pulse engine
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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/gotypes"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/lexical"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/parser"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/routes"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

// maxFileBytes is the per-file size ceiling; files larger than this are skipped
// (generated bundles / vendored blobs blow up the parser for no graph value).
const maxFileBytes = 1 << 20 // 1 MB

// Options configures a single indexing run.
type Options struct {
	// Reindex forces a full rebuild. The local SQLite tier already rebuilds the
	// snapshot's child rows idempotently per (repo_id, commit_sha), so this is a
	// hint surfaced in Stats.Mode rather than a divergent code path today.
	Reindex bool
	// Scope stamps the tenant/org id onto the indexed repo so EnsureRepo keys it by
	// (scope, full_name). Empty means single-tenant / no scope — the local default.
	Scope string
}

// Stats is the human-facing summary of an indexing run.
type Stats struct {
	Files      int            `json:"files"`
	Symbols    int            `json:"symbols"`
	Edges      int            `json:"edges"`
	Routes     int            `json:"routes"`
	Languages  map[string]int `json:"languages"`
	DurationMS int64          `json:"duration_ms"`
	Mode       string         `json:"mode"`
	// ChangedFiles is the number of files re-parsed on a delta run (0 on full /
	// reindex). It is purely additive — the engine maps Stats field-by-field, so a
	// new field is safe and simply unmapped until the engine opts in.
	ChangedFiles int `json:"changed_files"`
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
	"__pycache__":  {},
	".next":        {},
	".atlas":       {},
	".testdata":    {},
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

	head := resolveCommitSHA(ctx, absRoot)

	// Try an incremental delta: re-parse only the files that changed since the
	// latest snapshot's commit and carry the rest of the graph forward. Eligibility
	// is conservative (see deltaEligible); on any miss or error we fall through to
	// the full walk below, so a delta never fails the run.
	if !opts.Reindex {
		if base, baseErr := resolveDeltaBase(ctx, drv, repoFullName); baseErr == nil && base != nil {
			if deltaEligible(ctx, absRoot, base, head) {
				snap, stats, derr := runDelta(ctx, drv, lx, base, repoFullName, absRoot, head, opts, start)
				if derr == nil {
					return snap, stats, nil
				}
				// Delta failed mid-flight (git/diff/store hiccup): fall back to full.
			}
		}
	}

	var (
		files     []graph.File
		symbols   []graph.CodeSymbol
		edges     []graph.DependencyEdge
		rawRoutes []routes.RawRoute
		goFiles   []string // repo-relative paths of indexed Go files, for the go/types pass
		languages = map[string]int{}
	)

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
			if _, skip := skipDirs[entry.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinks and other non-regular files.
		if entry.Type()&fs.ModeSymlink != 0 {
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
		if info.Size() > maxFileBytes {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			// Unreadable single file: skip, don't abort the scan.
			return nil
		}

		res, parseErr := parser.Parse(repoID, repoFullName, rel, lang, content)
		if parseErr != nil {
			// A parse failure on one file should not sink the whole snapshot.
			return nil
		}

		files = append(files, graph.File{
			ID:        uuid.NewString(),
			Path:      rel,
			Language:  lang,
			SizeBytes: info.Size(),
			Hash:      hashContent(content),
			Imports:   res.Imports,
		})
		languages[lang]++
		if lang == "go" {
			goFiles = append(goFiles, rel)
		}
		symbols = append(symbols, res.Symbols...)
		edges = append(edges, res.Edges...)
		// Cross-repo moat: pull producer routes + consumer calls from the same
		// content the parser just consumed (cheap — the bytes are already in hand).
		// Handler resolution is deferred to routes.Resolve below, once the full
		// symbol set is known.
		rawRoutes = append(rawRoutes, routes.ExtractFile(lang, rel, string(content))...)
		return nil
	})
	if walkErr != nil {
		return nil, Stats{}, fmt.Errorf("index: walk %q: %w", absRoot, walkErr)
	}

	// Precise Go analysis (go/types): refine heuristic recv_type on call edges and
	// add real type-use reference edges. Non-regressing — on any miss the heuristic
	// edges stand untouched (see enrichGoTypes).
	edges = enrichGoTypes(ctx, absRoot, goFiles, edges)

	// Deterministic ordering so identical trees produce identical snapshots. The
	// same helpers order the delta path's merged rows, guaranteeing a delta
	// snapshot equals a full reindex of the same HEAD.
	sortFiles(files)
	sortSymbols(symbols)
	sortEdges(edges)

	// Resolve raw route facts now that the full symbol set is available: producer
	// handler names bind to their defining file, consumer calls keep their calling
	// file. Sorted for deterministic snapshots.
	graphRoutes := routes.Resolve(repoFullName, rawRoutes, symbols)
	sortRoutes(graphRoutes)

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
	ensured, err := drv.EnsureRepo(ctx, repo)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: ensure repo: %w", err)
	}
	// EnsureRepo may resolve a pre-existing repo id (lookup by scope+full_name);
	// adopt it so the snapshot binds to the canonical repo row and every symbol's
	// RepoID stays consistent.
	if ensured != nil && ensured.ID != "" && ensured.ID != repoID {
		snapshot.RepoID = ensured.ID
		for i := range symbols {
			symbols[i].RepoID = ensured.ID
		}
	}

	if err := drv.SaveSnapshot(ctx, snapshot, files, symbols, edges, graphRoutes); err != nil {
		return nil, Stats{}, fmt.Errorf("index: save snapshot: %w", err)
	}

	// Build the lexical index against the persisted snapshot id. The symbols now
	// carry the snapshot id; pass them straight through.
	if lx != nil {
		if err := lx.BuildForSnapshot(snapshot.ID, symbols); err != nil {
			return nil, Stats{}, fmt.Errorf("index: build lexical index: %w", err)
		}
	}

	stats := Stats{
		Files:      len(files),
		Symbols:    len(symbols),
		Edges:      len(edges),
		Routes:     len(graphRoutes),
		Languages:  languages,
		DurationMS: time.Since(start).Milliseconds(),
		Mode:       mode,
	}
	return snapshot, stats, nil
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

// hashContent returns the lowercase sha256 hex digest of a file's bytes.
func hashContent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// resolveCommitSHA returns the working tree's HEAD commit via `git rev-parse
// HEAD`, falling back to the sentinel "working-tree" when root is not a git
// checkout or git is unavailable.
func resolveCommitSHA(ctx context.Context, root string) string {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return "working-tree"
	}
	cmd := exec.CommandContext(ctx, gitBin, "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "working-tree"
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "working-tree"
	}
	return sha
}
