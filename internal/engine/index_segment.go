package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dominic097/atlas/internal/index"
)

// RepoIndexSummary is one repo's outcome inside a segmented multi-repo index run.
// A non-empty Error means that repo failed but did not sink the others.
type RepoIndexSummary struct {
	Repo       string `json:"repo"`
	Root       string `json:"root"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	Files      int    `json:"files"`
	Symbols    int    `json:"symbols"`
	Edges      int    `json:"edges"`
	Routes     int    `json:"routes"`
	Mode       string `json:"mode,omitempty"`
	Error      string `json:"error,omitempty"`
}

// indexOne indexes a single repo rooted at abs and returns the standard result.
// It is the original engine.Index body, extracted so both the single-repo and the
// segmented paths share one implementation. fullNameOverride wins when set (the
// segmented path passes each nested repo's name); otherwise in.Repo, else the dir
// base name.
func (e *localEngine) indexOne(ctx context.Context, in IndexInput, abs, fullNameOverride string, skipPaths []string) (*IndexResult, error) {
	fullName := strings.TrimSpace(fullNameOverride)
	if fullName == "" {
		fullName = in.Repo
	}
	if fullName == "" {
		fullName = filepath.Base(abs)
	}
	enableVectors := in.EnableVectors || e.cfg.EnableVector
	lx, err := e.ensureLexical()
	if err != nil {
		return nil, err
	}
	snap, stats, err := index.Run(ctx, e.store, lx, "", fullName, abs,
		index.Options{Reindex: in.Reindex, Scope: e.cfg.Scope, EnableVectors: enableVectors, SkipPaths: skipPaths})
	if err != nil {
		return nil, err
	}
	return &IndexResult{
		RepoID:       snap.RepoID,
		RepoFullName: fullName,
		SnapshotID:   snap.ID,
		CommitSHA:    snap.CommitSHA,
		IndexedFiles: stats.Files,
		Symbols:      stats.Symbols,
		Edges:        stats.Edges,
		EdgeKinds:    stats.EdgeKinds,
		Routes:       stats.Routes,
		Languages:    stats.Languages,
		Mode:         stats.Mode,
		DurationMS:   stats.DurationMS,
		TimingsMS:    stats.TimingsMS,
	}, nil
}

// indexSegmented indexes each repo under root as its OWN Atlas repo, into the
// shared store. This is what makes pointing `atlas index` at a workspace (a parent
// of many repos) work instead of OOM-ing: each repo's parse + go/types pass runs
// and flushes independently, so peak memory is one repo's worth, not the whole
// tree's. Cross-repo links are preserved for free — producer route contracts and
// consumer calls match across repos at QUERY time from the shared store, so no
// index-time linking step is needed.
//
// A single repo failing is collected (RepoIndexSummary.Error) and the run
// continues; the call only errors if EVERY repo failed.
func (e *localEngine) indexSegmented(ctx context.Context, in IndexInput, root string, nested []string, includeRoot bool) (*IndexResult, error) {
	enableVectors := in.EnableVectors || e.cfg.EnableVector
	lx, err := e.ensureLexical()
	if err != nil {
		return nil, err
	}

	// One index job per nested repo, plus — when the workspace root is itself a git
	// repo — a final job for the root's OWN loose files with every nested repo
	// pruned, so the outer repo's top-level sources are covered exactly once.
	type segJob struct {
		root, name string
		skip       []string
	}
	jobs := make([]segJob, 0, len(nested)+1)
	for _, r := range nested {
		jobs = append(jobs, segJob{root: r, name: filepath.Base(r)})
	}
	if includeRoot {
		jobs = append(jobs, segJob{root: root, name: filepath.Base(root), skip: nested})
	}

	pc := index.ProgressFromContext(ctx)
	start := time.Now()
	agg := &IndexResult{
		RepoFullName: filepath.Base(root),
		Mode:         "segmented",
		EdgeKinds:    map[string]int{},
		Languages:    map[string]int{},
	}
	var failed []string
	for i, job := range jobs {
		if ctx.Err() != nil {
			break
		}
		name := job.name
		pc.SetRepo(name, i+1, len(jobs))
		snap, stats, rerr := index.Run(ctx, e.store, lx, "", name, job.root,
			index.Options{Reindex: in.Reindex, Scope: e.cfg.Scope, EnableVectors: enableVectors, SkipPaths: job.skip})
		summary := RepoIndexSummary{Repo: name, Root: job.root}
		if rerr != nil {
			summary.Error = rerr.Error()
			failed = append(failed, name)
			agg.Repos = append(agg.Repos, summary)
			continue
		}
		summary.SnapshotID = snap.ID
		summary.Files = stats.Files
		summary.Symbols = stats.Symbols
		summary.Edges = stats.Edges
		summary.Routes = stats.Routes
		summary.Mode = stats.Mode
		agg.Repos = append(agg.Repos, summary)

		// Last successful repo's identity stands in for the aggregate's RepoID/
		// SnapshotID/CommitSHA fields (which are single-repo by shape); the per-repo
		// breakdown in Repos is the authoritative record for a segmented run.
		agg.RepoID = snap.RepoID
		agg.SnapshotID = snap.ID
		agg.CommitSHA = snap.CommitSHA
		agg.IndexedFiles += stats.Files
		agg.Symbols += stats.Symbols
		agg.Edges += stats.Edges
		agg.Routes += stats.Routes
		for k, v := range stats.EdgeKinds {
			agg.EdgeKinds[k] += v
		}
		for k, v := range stats.Languages {
			agg.Languages[k] += v
		}
	}
	agg.DurationMS = time.Since(start).Milliseconds()
	pc.SetPhase("done")
	if len(jobs) > 0 && len(failed) == len(jobs) {
		return agg, fmt.Errorf("engine: all %d repos failed to index (first: %s)", len(jobs), failed[0])
	}
	return agg, nil
}

// isGitRepo reports whether dir is a git repository root. .git may be a directory
// (normal clone) or a file (worktree / submodule pointer), so a plain Stat covers
// both.
func isGitRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	return false
}

// repoDiscoveryMaxDepth bounds how deep below the root discoverRepos looks for
// nested repos, so a pathological tree cannot turn discovery into a full walk.
const repoDiscoveryMaxDepth = 5

// repoDiscoverySkip are directory names discoverRepos never descends into while
// hunting for repo roots: VCS internals, dependency caches, and agent scratch
// (.claude/worktrees holds duplicate checkouts that must not be re-indexed).
var repoDiscoverySkip = map[string]struct{}{
	".git": {}, "node_modules": {}, "vendor": {}, ".claude": {},
	".atlas": {}, ".hg": {}, ".svn": {}, ".venv": {}, "venv": {},
	"__pycache__": {}, ".idea": {}, ".vscode": {}, ".cache": {},
	".gradle": {}, ".mvn": {}, ".tox": {},
}

// discoverRepos returns the sorted roots of the git repositories nested under root
// (root itself is assumed NOT to be a repo — the caller checks that first). It
// records the first .git it finds on any branch and does not descend further, so
// each top-level repo is returned once and a repo's own submodules/vendored trees
// are left to that repo's own index pass.
func discoverRepos(root string) []string {
	var repos []string
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		if _, skip := repoDiscoverySkip[d.Name()]; skip {
			return filepath.SkipDir
		}
		if strings.Count(filepath.Clean(path), string(os.PathSeparator))-rootDepth > repoDiscoveryMaxDepth {
			return filepath.SkipDir
		}
		if isGitRepo(path) {
			repos = append(repos, path)
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(repos)
	return repos
}
