package watch

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// skipDirs mirrors the indexer's pruned directory set (internal/index.skipDirs):
// VCS metadata, dependency caches, and Atlas's own on-disk state. The indexer's
// set is package-private, so we keep an independent copy here. Keeping them in
// sync matters: if the watcher registered .git or node_modules it would wake on
// churn the indexer never reads, burning CPU for no graph change. Build-output
// names ("build"/"dist"/...) are deliberately NOT skipped, matching the indexer,
// because they collide with real source-package directory names.
var skipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	".venv":        {},
	"__pycache__":  {},
	".next":        {},
	".atlas":       {},
	"graphify-out": {},
	".testdata":    {},
}

// addTree registers root and every non-pruned subdirectory with the fsnotify
// watcher. fsnotify only delivers events for directories explicitly added (it is
// not recursive), so we walk once and add each directory. Pruned directories are
// skipped wholesale via fs.SkipDir, so their entire subtrees are never watched.
//
// A per-directory Add failure (e.g. a directory removed between WalkDir reading
// it and Add) is non-fatal: we skip that directory and continue, so a racing
// delete cannot abort the whole registration.
func addTree(fsw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable entry: skip it rather than abort the whole walk. The root
			// itself is stat-checked in New, so a root error here is already rare.
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root {
			if _, skip := skipDirs[entry.Name()]; skip {
				return filepath.SkipDir
			}
		}
		// Best-effort: a directory that vanished between WalkDir and Add is fine to
		// drop; its contents are gone too.
		_ = fsw.Add(path)
		return nil
	})
}

// isUnderSkipped reports whether path lies inside a pruned directory relative to
// root. fsnotify can deliver an event for a pruned path in two ways: a watched
// parent emits a Create for a new pruned dir, or a brief race adds churn before
// we prune. Filtering here keeps those events from arming the debounce timer.
func isUnderSkipped(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if _, skip := skipDirs[part]; skip {
			return true
		}
	}
	return false
}

// isWatchableLeaf reports whether a Write event's path is worth a refresh. It
// accepts paths that have any extension (source files) and paths with no
// extension (could be a directory or an extensionless file the indexer ignores
// cheaply). The indexer's own LanguageForPath/Supported/size filter is the
// precise gate — a false positive here only costs a working-tree scan that
// noops, never a wrong graph. Editor swap/temp files are dropped so a save does
// not double-fire on the temp then the rename.
func isWatchableLeaf(path string) bool {
	base := filepath.Base(path)
	// Common editor atomic-save temporaries: skip the transient, the real write
	// arrives as a Rename/Create on the final name.
	if strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".tmp") {
		return false
	}
	if strings.HasPrefix(base, ".#") || strings.HasPrefix(base, "#") {
		return false
	}
	return true
}
