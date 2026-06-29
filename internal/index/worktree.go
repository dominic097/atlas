// worktree.go implements working-tree-aware change detection for index.Run.
//
// The original delta path keyed change detection off a COMMIT diff
// (gitDiffNameStatus(base, head)). That misses the single most common per-edit
// update an AI agent runs: it edits a file in the working tree WITHOUT
// committing, then re-indexes. With no new commit, the commit diff is empty and
// the edit is invisible — the run noops and the graph goes stale.
//
// scanWorkTree fixes this at the source. It walks the working tree ONCE with the
// exact same prune/lang/size rules as the full index, content-hashes every
// supported file, and classifies each against the base snapshot's stored hashes
// (drv.ListFiles -> path+Hash):
//
//   - changed  = path present in both, hash differs
//   - new      = path present now, absent from base
//   - deleted  = path present in base, absent now
//
// This is git-independent (works on non-git dirs and untracked files) and is the
// correctness backstop: it catches uncommitted edits, new files, and deletions
// regardless of commit state. The scan materializes each file's bytes + hash so
// the delta path can re-parse changed files without a second read/hash pass.
package index

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/dominic097/atlas/internal/parser"
	"github.com/dominic097/atlas/internal/store"
)

// workTreeFile is one supported file found during the working-tree scan, with
// its content and hash already computed so the delta path need not re-read it.
type workTreeFile struct {
	absPath string
	relPath string // canonical (ToSlash) repo-relative path
	lang    string
	size    int64
	hash    string
	content []byte
}

// workTreeScan is the result of comparing the working tree against a base
// snapshot: the full present-file set plus the change classification. changed and
// new are disjoint sets of present rel paths; deleted are base rel paths no
// longer present. allEmpty reports a genuine no-op (tree matches the snapshot).
type workTreeScan struct {
	files   []workTreeFile      // every supported, present file (sorted by relPath)
	changed map[string]struct{} // present + hash differs from base
	added   map[string]struct{} // present + absent from base
	deleted map[string]struct{} // in base + absent now
}

// touchedSet returns changed ∪ added ∪ deleted as a canonical-path set: the
// files whose base rows must be dropped before the new/merged rows are folded in.
func (s *workTreeScan) touchedSet() map[string]struct{} {
	out := make(map[string]struct{}, len(s.changed)+len(s.added)+len(s.deleted))
	for p := range s.changed {
		out[p] = struct{}{}
	}
	for p := range s.added {
		out[p] = struct{}{}
	}
	for p := range s.deleted {
		out[p] = struct{}{}
	}
	return out
}

// changedSet returns the files that must be (re)parsed: changed ∪ added. Deleted
// files are handled purely by the keep-filter (their base rows are dropped and no
// new rows replace them).
func (s *workTreeScan) changedSet() map[string]struct{} {
	out := make(map[string]struct{}, len(s.changed)+len(s.added))
	for p := range s.changed {
		out[p] = struct{}{}
	}
	for p := range s.added {
		out[p] = struct{}{}
	}
	return out
}

// noChanges reports whether the working tree matches the base snapshot exactly —
// the only condition under which an index run is a genuine no-op.
func (s *workTreeScan) noChanges() bool {
	return len(s.changed) == 0 && len(s.added) == 0 && len(s.deleted) == 0
}

// scanWorkTree walks absRoot once (pruning skipDirs, honouring the lang/size
// rules), content-hashes every supported file, and classifies each against the
// base snapshot's stored file hashes. The base hashes are loaded via
// drv.ListFiles so detection works with no git history at all.
//
// It returns the full present-file set (with content+hash materialized for reuse
// by the delta path) and the changed/added/deleted classification. Any walk error
// propagates so the caller can fall back to a full index rather than index a
// partial tree.
func scanWorkTree(ctx context.Context, drv store.StorageDriver, baseSnapshotID, absRoot string) (*workTreeScan, error) {
	baseFiles, err := drv.ListFiles(ctx, baseSnapshotID)
	if err != nil {
		return nil, err
	}
	// path -> base content hash. canonicalPath keeps membership robust to any
	// stored backslash paths (mirrors the keep-filter's canonicalization).
	baseHash := make(map[string]string, len(baseFiles))
	for _, f := range baseFiles {
		baseHash[canonicalPath(f.Path)] = f.Hash
	}

	scan := &workTreeScan{
		changed: map[string]struct{}{},
		added:   map[string]struct{}{},
		deleted: map[string]struct{}{},
	}

	// Phase 1: WALK ONLY — collect the supported, in-size candidate files (no read,
	// no hash). The walk is inherently serial (directory tree traversal) but cheap;
	// the expensive per-file read+hash is deferred to the parallel phase below.
	var candidates []workTreeCandidate
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
			// A file that vanished mid-walk is not fatal to the whole scan.
			return nil
		}
		if info.Size() > maxFileBytes {
			return nil
		}

		candidates = append(candidates, workTreeCandidate{
			absPath: path,
			relPath: rel,
			lang:    lang,
			size:    info.Size(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// Phase 2: READ + HASH in parallel. Each candidate is independent (a file read
	// and a sha256), so a worker pool sized to GOMAXPROCS computes them concurrently.
	// Workers write into per-index result slots (no shared mutation), so the output
	// is order-independent; the present-file set is sorted below for determinism.
	hashed := hashWorkTreeCandidates(ctx, candidates)

	seen := make(map[string]struct{}, len(candidates))
	for i := range hashed {
		wf := hashed[i]
		if !wf.ok {
			// Unreadable single file: skip, don't abort the scan (parity with the
			// prior serial behavior, where a read error returned nil for that file).
			continue
		}
		scan.files = append(scan.files, wf.file)
		seen[wf.file.relPath] = struct{}{}

		prev, inBase := baseHash[wf.file.relPath]
		switch {
		case !inBase:
			scan.added[wf.file.relPath] = struct{}{}
		case prev != wf.file.hash:
			scan.changed[wf.file.relPath] = struct{}{}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Anything in the base but no longer present on disk is a deletion.
	for relPath := range baseHash {
		if _, present := seen[relPath]; !present {
			scan.deleted[relPath] = struct{}{}
		}
	}

	sortWorkTreeFiles(scan.files)
	return scan, nil
}

// workTreeCandidate is a file the walk selected for hashing: its path/lang/size are
// known but its content has not yet been read.
type workTreeCandidate struct {
	absPath string
	relPath string
	lang    string
	size    int64
}

// hashedWorkTreeFile is the parallel read+hash result for one candidate. ok is false
// when the file could not be read (it is then skipped, mirroring the serial path).
type hashedWorkTreeFile struct {
	file workTreeFile
	ok   bool
}

// hashWorkTreeCandidates reads and content-hashes every candidate concurrently
// across a worker pool sized to GOMAXPROCS (capped at the candidate count). Results
// are written to per-index slots so the output order is deterministic (it mirrors
// the candidate order, which the caller then sorts by rel path regardless). Honors
// ctx cancellation between files.
func hashWorkTreeCandidates(ctx context.Context, candidates []workTreeCandidate) []hashedWorkTreeFile {
	out := make([]hashedWorkTreeFile, len(candidates))
	if len(candidates) == 0 {
		return out
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}

	indexes := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range indexes {
				if ctx.Err() != nil {
					continue
				}
				c := candidates[i]
				content, readErr := os.ReadFile(c.absPath)
				if readErr != nil {
					continue // out[i].ok stays false
				}
				out[i] = hashedWorkTreeFile{
					ok: true,
					file: workTreeFile{
						absPath: c.absPath,
						relPath: c.relPath,
						lang:    c.lang,
						size:    c.size,
						hash:    hashContent(content),
						content: content,
					},
				}
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
	return out
}

// sortWorkTreeFiles orders the present-file set by rel path so the delta path
// builds rows in the same deterministic order a full index would, independent of
// filesystem iteration order.
func sortWorkTreeFiles(files []workTreeFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].relPath < files[j].relPath })
}
