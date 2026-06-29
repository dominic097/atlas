// Package watch keeps an Atlas graph fresh automatically. It watches a repo's
// working tree for file changes and, on any edit, runs the SAME incremental
// index path the `atlas index` command uses (engine.Index) — which is
// working-tree-aware, so an uncommitted edit, a new file, or a deletion all
// trigger a real delta update with no manual command.
//
// The design goal is the warm-process integration: the long-lived MCP/HTTP
// server an agent launches can keep the graph live in the background, so every
// query hits a fresh graph. A burst of edits (a multi-file save, a branch
// switch, a code-gen pass) is coalesced by a debounce window into ONE update.
//
// File watching prefers github.com/fsnotify/fsnotify (real inotify/kqueue
// events, no polling cost at rest). The watcher prunes the same dependency /
// VCS / build-cache directories the indexer prunes, so it never wakes on
// node_modules / .git / .atlas churn.
package watch

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/dominic097/atlas/pkg/atlas"
)

// DefaultDebounce coalesces a burst of file events into a single index run. A
// multi-file save or a code-gen pass fires many events within a few
// milliseconds; waiting this long after the LAST event collapses them into one
// update instead of re-indexing per file.
const DefaultDebounce = 250 * time.Millisecond

// Engine is the slice of the Atlas engine the watcher needs: it only ever
// triggers an incremental index. Narrowing to this interface keeps the watcher
// trivially testable with a fake and documents that watching never reads the
// graph, only refreshes it.
type Engine interface {
	Index(ctx context.Context, in atlas.IndexInput) (*atlas.IndexResult, error)
}

// Options configures a Watcher.
type Options struct {
	// Repo is the full_name passed through to Index. Empty lets the engine derive
	// it from the path (filepath.Base), matching `atlas index` with no --repo.
	Repo string
	// Debounce overrides DefaultDebounce. Zero uses the default.
	Debounce time.Duration
	// Logger receives one concise line per triggered update (and per error). Nil
	// discards all output — useful in tests and when the caller wants silence.
	Logger io.Writer
	// EnableVectors forwards to Index so a watch session can keep the optional
	// embedding layer fresh too. Off by default.
	EnableVectors bool
}

// Watcher watches a repository tree and incrementally refreshes the graph on
// every change. Construct with New, then Start(ctx); call Stop to shut down
// cleanly (it also exits on ctx cancellation).
type Watcher struct {
	eng  Engine
	root string
	repo string

	debounce      time.Duration
	logger        io.Writer
	enableVectors bool

	// fsw is the underlying file-system notifier. It is created in Start so a
	// Watcher can be constructed cheaply and a creation failure surfaces at Start.
	fsw *fsnotify.Watcher

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}

	// updates counts completed (non-noop) refreshes; tests assert progress.
	updateMu sync.Mutex
	updates  int
}

// New builds a Watcher for the repo rooted at root, refreshing eng on change.
// root must be a directory. The Watcher does not touch the filesystem until
// Start.
func New(eng Engine, root string, opts Options) (*Watcher, error) {
	if eng == nil {
		return nil, fmt.Errorf("watch: engine is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("watch: resolve root %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("watch: stat root %q: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("watch: root %q is not a directory", abs)
	}
	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	return &Watcher{
		eng:           eng,
		root:          abs,
		repo:          opts.Repo,
		debounce:      debounce,
		logger:        opts.Logger,
		enableVectors: opts.EnableVectors,
	}, nil
}

// Start begins watching in a background goroutine and returns immediately. The
// watch loop runs until ctx is cancelled or Stop is called. Calling Start twice
// is an error. The fsnotify watcher and the initial directory registration
// happen synchronously here so a setup failure is returned to the caller rather
// than swallowed by the goroutine.
func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return fmt.Errorf("watch: already started")
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.mu.Unlock()
		return fmt.Errorf("watch: create fsnotify watcher: %w", err)
	}
	// Register the whole tree up front. fsnotify watches a single directory per
	// Add, so we walk and add every non-pruned directory; new directories created
	// later are added on the fly in the event loop.
	if err := addTree(fsw, w.root); err != nil {
		_ = fsw.Close()
		w.mu.Unlock()
		return fmt.Errorf("watch: register tree: %w", err)
	}

	loopCtx, cancel := context.WithCancel(ctx)
	w.fsw = fsw
	w.cancel = cancel
	w.done = make(chan struct{})
	w.started = true
	w.mu.Unlock()

	go w.loop(loopCtx)
	return nil
}

// Stop cancels the watch loop and blocks until it has fully exited (the
// fsnotify watcher is closed and the goroutine has returned). Safe to call once;
// calling it when not started, or twice, is a no-op.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	cancel := w.cancel
	done := w.done
	w.started = false
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Updates reports how many non-noop refreshes have completed. Used by tests to
// confirm the background update actually ran; also handy for diagnostics.
func (w *Watcher) Updates() int {
	w.updateMu.Lock()
	defer w.updateMu.Unlock()
	return w.updates
}

// loop is the event pump. It coalesces fsnotify events with a debounce timer and
// fires one index per quiet window. It owns fsw and closes it on exit, so Stop's
// done-channel close signals a fully released watcher.
func (w *Watcher) loop(ctx context.Context) {
	defer close(w.done)
	defer w.fsw.Close()

	// A single reusable timer; we Reset it on each event and drain on fire. nil
	// channel until the first relevant event so a quiet watcher never wakes.
	var (
		timer   *time.Timer
		timerC  <-chan time.Time
		pending bool
	)
	stopTimer := func() {
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	arm := func() {
		pending = true
		if timer == nil {
			timer = time.NewTimer(w.debounce)
		} else {
			stopTimer()
			timer.Reset(w.debounce)
		}
		timerC = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			stopTimer()
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if !w.relevant(event) {
				continue
			}
			// A newly created directory must itself be watched, else edits inside it
			// are invisible. Adding it here (best-effort) keeps the tree live.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addTree(w.fsw, event.Name)
				}
			}
			arm()

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.logf("watch: fsnotify error: %v", err)

		case <-timerC:
			timerC = nil
			if !pending {
				continue
			}
			pending = false
			w.refresh(ctx)
		}
	}
}

// refresh runs one incremental index. It is working-tree-aware, so it updates
// the graph for uncommitted edits and noops only when the tree already matches
// the snapshot. Errors and noops are logged but never stop the loop.
func (w *Watcher) refresh(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	start := time.Now()
	res, err := w.eng.Index(ctx, atlas.IndexInput{
		ProjectPath:   w.root,
		Repo:          w.repo,
		EnableVectors: w.enableVectors,
	})
	if err != nil {
		// A transient index failure (e.g. a file mid-write) must not kill the
		// watcher; the next event re-triggers a fresh attempt.
		w.logf("watch: index error after %s: %v", time.Since(start).Round(time.Millisecond), err)
		return
	}
	if res.Mode == "noop" {
		// The tree already matched the snapshot (e.g. an editor touched mtime
		// without changing bytes, or a save inside a pruned dir slipped through).
		// Log at a lower volume so a noisy editor does not spam.
		w.logf("watch: no change (%s, %s)", res.Mode, time.Since(start).Round(time.Millisecond))
		return
	}
	w.updateMu.Lock()
	w.updates++
	w.updateMu.Unlock()
	w.logf("watch: updated mode=%s files=%d symbols=%d edges=%d in %s",
		res.Mode, res.IndexedFiles, res.Symbols, res.Edges, time.Since(start).Round(time.Millisecond))
}

// relevant filters fsnotify events down to ones that can change the graph:
// edits to supported source files, and directory creates (so new dirs get
// watched). Chmod-only events and changes under pruned directories are dropped.
func (w *Watcher) relevant(event fsnotify.Event) bool {
	// Chmod alone never changes content; ignore it to avoid spurious wakeups.
	if event.Op == fsnotify.Chmod {
		return false
	}
	if isUnderSkipped(w.root, event.Name) {
		return false
	}
	// A directory create is relevant (we add it to the watch set), but we cannot
	// stat reliably on a Remove/Rename. Treat creates/removes/renames of any
	// entry as potentially graph-affecting; the index's own lang/size filter is
	// the precise gate, so a false positive just yields a cheap noop.
	if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		return true
	}
	// Write: only wake for paths that look like (or could be) a directory or a
	// supported source file. A directory check requires a stat; cheaper to accept
	// and let the indexer's filter decide.
	return isWatchableLeaf(event.Name)
}

func (w *Watcher) logf(format string, args ...any) {
	if w.logger == nil {
		return
	}
	fmt.Fprintf(w.logger, format+"\n", args...)
}
