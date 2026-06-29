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
	"sort"

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
	seen := make(map[string]struct{}, len(baseFiles))

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

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			// Unreadable single file: skip, don't abort the scan.
			return nil
		}
		hash := hashContent(content)

		scan.files = append(scan.files, workTreeFile{
			absPath: path,
			relPath: rel,
			lang:    lang,
			size:    info.Size(),
			hash:    hash,
			content: content,
		})
		seen[rel] = struct{}{}

		prev, inBase := baseHash[rel]
		switch {
		case !inBase:
			scan.added[rel] = struct{}{}
		case prev != hash:
			scan.changed[rel] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
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

// sortWorkTreeFiles orders the present-file set by rel path so the delta path
// builds rows in the same deterministic order a full index would, independent of
// filesystem iteration order.
func sortWorkTreeFiles(files []workTreeFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].relPath < files[j].relPath })
}
