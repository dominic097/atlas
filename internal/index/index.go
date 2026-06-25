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
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/lexical"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/parser"
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
}

// skipDirs are directory names pruned wholesale during the walk: VCS metadata,
// dependency caches, build output, and Atlas's own on-disk state.
var skipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	".atlas":       {},
	".testdata":    {},
	"target":       {},
	".venv":        {},
	"__pycache__":  {},
	".next":        {},
	"out":          {},
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

	var (
		files     []graph.File
		symbols   []graph.CodeSymbol
		edges     []graph.DependencyEdge
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
		symbols = append(symbols, res.Symbols...)
		edges = append(edges, res.Edges...)
		return nil
	})
	if walkErr != nil {
		return nil, Stats{}, fmt.Errorf("index: walk %q: %w", absRoot, walkErr)
	}

	// Deterministic ordering so identical trees produce identical snapshots.
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].Path == symbols[j].Path {
			return symbols[i].StartLine < symbols[j].StartLine
		}
		return symbols[i].Path < symbols[j].Path
	})
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].FromFile == edges[j].FromFile {
			return edges[i].ToRef < edges[j].ToRef
		}
		return edges[i].FromFile < edges[j].FromFile
	})

	commitSHA := resolveCommitSHA(ctx, absRoot)

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
		RouteCount:  0,
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

	now := time.Now().UTC()
	repo := &graph.Repo{
		ID:            repoID,
		FullName:      repoFullName,
		Root:          absRoot,
		Status:        graph.StatusReady,
		Languages:     languages,
		LastCommit:    commitSHA,
		LastIndexedAt: &now,
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

	if err := drv.SaveSnapshot(ctx, snapshot, files, symbols, edges, nil); err != nil {
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
		Routes:     0,
		Languages:  languages,
		DurationMS: time.Since(start).Milliseconds(),
		Mode:       mode,
	}
	return snapshot, stats, nil
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
