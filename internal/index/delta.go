// delta.go implements incremental (delta) indexing for index.Run.
//
// A delta reindex avoids re-parsing the whole working tree when the previous
// snapshot's commit is an ancestor of the new HEAD: only files that changed
// between the two commits are re-parsed, and the rest of the graph (symbols,
// edges, routes for untouched files) is carried forward from the base snapshot.
// The merged result is sorted identically to a full index so a delta snapshot is
// byte-for-byte equivalent to a full reindex of the same HEAD.
//
// Everything here is built on the EXISTING StorageDriver methods (ListRepos,
// LatestSnapshot, ListSymbols, ListEdges, ListRoutes); the delta logic derives
// its base snapshot itself and needs no store-interface or engine change.
package index

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/lexical"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/parser"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/routes"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

// workingTreeSHA is the sentinel resolveCommitSHA returns for a non-git tree; a
// snapshot stamped with it can never be a delta base.
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

// deltaEligible reports whether a delta reindex is safe given the base snapshot
// and the new HEAD. It requires: a base commit that is a real, non-sentinel sha
// distinct from head; git available; and the base being an ancestor of head
// (which rules out force-pushes / unrelated history that would invalidate the
// carry-forward).
func deltaEligible(ctx context.Context, root string, base *deltaBase, head string) bool {
	if base == nil {
		return false
	}
	baseCommit := strings.TrimSpace(base.commit)
	if baseCommit == "" || baseCommit == workingTreeSHA {
		return false
	}
	if head == "" || head == workingTreeSHA {
		return false
	}
	if baseCommit == head {
		return false
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	// `merge-base --is-ancestor A B` exits 0 iff A is an ancestor of B.
	cmd := exec.CommandContext(ctx, gitBin, "-C", root, "merge-base", "--is-ancestor", baseCommit, head)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// gitDiffNameStatus returns the changed and deleted repo-root-relative paths
// between baseCommit and head, parsed from `git diff --name-status --no-renames
// -z`. With --no-renames, a rename is reported as a delete + an add, so renamed
// files land in both sets correctly. Status codes A/M/C/T/R map to changed; D
// maps to deleted. Paths are ToSlash-normalized.
func gitDiffNameStatus(ctx context.Context, root, baseCommit, head string) (changed, deleted []string, err error) {
	gitBin, lookErr := exec.LookPath("git")
	if lookErr != nil {
		return nil, nil, lookErr
	}
	cmd := exec.CommandContext(ctx, gitBin, "-C", root,
		"diff", "--name-status", "--no-renames", "-z", baseCommit, head)
	out, runErr := cmd.Output()
	if runErr != nil {
		return nil, nil, runErr
	}
	changed, deleted = parseNameStatusZ(out)
	return changed, deleted, nil
}

// parseNameStatusZ parses the NUL-delimited output of `git diff --name-status
// -z`. The stream is a flat sequence of records where each entry is a status
// token (e.g. "A", "M", "D") followed by its path, both NUL-terminated:
//
//	"M\0path1\0A\0path2\0D\0path3\0"
//
// (With --no-renames there are no two-path R/C records to special-case.)
func parseNameStatusZ(out []byte) (changed, deleted []string) {
	fields := strings.Split(string(out), "\x00")
	i := 0
	for i < len(fields) {
		status := strings.TrimSpace(fields[i])
		if status == "" {
			i++
			continue
		}
		if i+1 >= len(fields) {
			break
		}
		path := canonicalPath(fields[i+1])
		i += 2
		if path == "" {
			continue
		}
		switch status[0] {
		case 'D':
			deleted = append(deleted, path)
		case 'A', 'M', 'C', 'T', 'R':
			changed = append(changed, path)
		default:
			// Unknown status (e.g. U for unmerged): treat as changed so the file
			// is re-parsed rather than silently carried forward stale.
			changed = append(changed, path)
		}
	}
	return changed, deleted
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
// merged edge set so a delta re-enrichment (enrichGoTypes) can regenerate them
// for the whole repo without duplicating the copies kept from the base snapshot.
// Call edges and any non-go_types edges are preserved untouched.
func dropTypeUseRefs(edges []graph.DependencyEdge) []graph.DependencyEdge {
	out := make([]graph.DependencyEdge, 0, len(edges))
	for _, e := range edges {
		if e.Kind == graph.EdgeReferences {
			if src, _ := e.Metadata["source"].(string); src == "go_types" {
				continue
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

// runDelta executes the incremental index path. Preconditions (checked by the
// caller): base != nil, deltaEligible == true. It re-parses only changed files,
// carries the rest of the base graph forward, merges + re-sorts everything to
// match a full index, persists a new snapshot, and rebuilds the lexical index.
//
// Any error here is the caller's signal to fall back to a full index — runDelta
// must not have side effects before SaveSnapshot, which it doesn't (all work is
// in-memory until the single save).
func runDelta(ctx context.Context, drv store.StorageDriver, lx *lexical.Index, base *deltaBase, repoFullName, absRoot, head string, opts Options, start time.Time) (*graph.Snapshot, Stats, error) {
	baseCommit := base.commit

	changedPaths, deletedPaths, err := gitDiffNameStatus(ctx, absRoot, baseCommit, head)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("index: delta diff: %w", err)
	}
	changedSet := makeSet(changedPaths)
	deletedSet := makeSet(deletedPaths)
	touched := makeSet(changedPaths, deletedPaths)

	// Whether any touched (changed or deleted) file is Go — gates the go/types
	// re-enrichment below. No Go change means the kept base Go edges are already
	// precise, so the whole-repo type pass can be skipped (delta stays fast).
	goTouched := false
	for p := range touched {
		if parser.LanguageForPath(p) == "go" {
			goTouched = true
			break
		}
	}

	// Re-walk the tree with the SAME prune/size/lang rules as the full path, but
	// only PARSE files whose rel path changed. Every supported file still present
	// contributes a graph.File row (stat+hash only, no parse) so FileCount matches
	// a full index of HEAD.
	var (
		files        []graph.File
		newSymbols   []graph.CodeSymbol
		newEdges     []graph.DependencyEdge
		newRawRoutes []routes.RawRoute
		languages    = map[string]int{}
	)

	walkErr := filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
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
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		lang := parser.LanguageForPath(rel)
		if !parser.Supported(lang) {
			return nil
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		if info.Size() > maxFileBytes {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		// File row for every supported, present file (keeps FileCount correct).
		files = append(files, graph.File{
			ID:        uuid.NewString(),
			Path:      rel,
			Language:  lang,
			SizeBytes: info.Size(),
			Hash:      hashContent(content),
		})
		languages[lang]++

		// Only changed files are (re)parsed; their fresh rows replace the dropped
		// base rows for the same file.
		if _, isChanged := changedSet[rel]; !isChanged {
			return nil
		}
		res, parseErr := parser.Parse("", repoFullName, rel, lang, content)
		if parseErr != nil {
			return nil
		}
		// Restore imports onto the file row now that we parsed it.
		files[len(files)-1].Imports = res.Imports
		newSymbols = append(newSymbols, res.Symbols...)
		newEdges = append(newEdges, res.Edges...)
		newRawRoutes = append(newRawRoutes, routes.ExtractFile(lang, rel, string(content))...)
		return nil
	})
	if walkErr != nil {
		return nil, Stats{}, fmt.Errorf("index: delta walk %q: %w", absRoot, walkErr)
	}
	_ = deletedSet // deletions are handled purely via the `touched` keep-filter

	// Load the base graph and keep only the rows whose owning file is untouched.
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

	mergedSymbols := append(keepBaseSymbols(baseSymbols, touched), newSymbols...)
	mergedEdges := append(keepBaseEdges(baseEdges, touched), newEdges...)
	// Resolve the changed-file raw routes against the FULL merged symbol set so a
	// handler defined in an untouched file still resolves to its file, then append
	// to the kept base routes.
	resolvedNewRoutes := routes.Resolve(repoFullName, newRawRoutes, mergedSymbols)
	mergedRoutes := append(keepBaseRoutes(baseRoutes, touched), resolvedNewRoutes...)

	// Precise Go analysis parity with the full path: when a Go file changed or was
	// deleted, re-run the whole-repo go/types pass over the merged graph so the
	// delta carries the same recv_type refinement + type-use reference edges a full
	// reindex would. Touched Go files would otherwise keep only heuristic edges, and
	// a deleted file would leave stale references behind. The prior type-use refs
	// are dropped first since enrichGoTypes regenerates them for the whole repo.
	if goTouched {
		mergedEdges = dropTypeUseRefs(mergedEdges)
		var goFiles []string
		for i := range files {
			if files[i].Language == "go" {
				goFiles = append(goFiles, files[i].Path)
			}
		}
		mergedEdges = enrichGoTypes(ctx, absRoot, goFiles, mergedEdges)
	}

	// Re-sort everything identically to the full path so a delta snapshot equals a
	// full reindex of the same HEAD.
	sortFiles(files)
	sortSymbols(mergedSymbols)
	sortEdges(mergedEdges)
	sortRoutes(mergedRoutes)

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

	if err := drv.SaveSnapshot(ctx, snapshot, files, mergedSymbols, mergedEdges, mergedRoutes); err != nil {
		return nil, Stats{}, fmt.Errorf("index: delta save snapshot: %w", err)
	}

	if lx != nil {
		// Full lexical rebuild for the new snapshot id (correct + fine for v1).
		if err := lx.BuildForSnapshot(snapshot.ID, mergedSymbols); err != nil {
			return nil, Stats{}, fmt.Errorf("index: delta build lexical index: %w", err)
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
