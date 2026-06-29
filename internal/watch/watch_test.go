package watch_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dominic097/atlas/internal/watch"
	"github.com/dominic097/atlas/pkg/atlas"
)

// safeBuf is a goroutine-safe io.Writer the watcher can log into from its loop
// goroutine while the test reads the accumulated text — without a data race.
type safeBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// newEngine builds a real Atlas engine backed by a throwaway SQLite db under the
// test's temp dir, so the watcher exercises the genuine index path end to end.
func newEngine(t *testing.T) atlas.Engine {
	t.Helper()
	dbDir := t.TempDir()
	// A unique SQLite path per engine; the lexical index defaults to a sibling
	// directory of the db, so each test is automatically isolated on disk.
	dbPath := filepath.Join(dbDir, "atlas.db")
	eng, err := atlas.New(context.Background(), atlas.WithSQLite(dbPath))
	if err != nil {
		t.Fatalf("atlas.New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

// symbolKnown reports whether the engine can resolve a symbol by name — i.e. the
// graph already contains it. Used to poll for the background update landing.
func symbolKnown(t *testing.T, eng atlas.Engine, name string) bool {
	t.Helper()
	res, err := eng.Symbol(context.Background(), atlas.SymbolInput{Name: name})
	if err != nil {
		return false
	}
	for _, m := range res.Matches {
		if m.Name == name {
			return true
		}
	}
	return false
}

// waitFor polls cond until it is true or the deadline elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// TestWatcherAutoIndexesNewSymbol is the headline: with the watcher running, a
// brand-new source file written into the tree becomes queryable through the
// engine WITHOUT any manual index call — the background update did it.
func TestWatcherAutoIndexesNewSymbol(t *testing.T) {
	repo := t.TempDir()
	// Seed one file so the initial index has a non-empty base snapshot to delta
	// against (the working-tree-aware path needs a base).
	seed := "package svc\n\n// Seed exists from the start.\nfunc Seed() string { return \"seed\" }\n"
	if err := os.WriteFile(filepath.Join(repo, "seed.go"), []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Index(ctx, atlas.IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("initial Index: %v", err)
	}
	// Sanity: the new symbol is NOT present before we write it.
	if symbolKnown(t, eng, "WatcherAddedFunc") {
		t.Fatalf("WatcherAddedFunc present before it was written")
	}

	var logs safeBuf
	w, err := watch.New(eng, repo, watch.Options{
		Debounce: 60 * time.Millisecond,
		Logger:   &logs,
	})
	if err != nil {
		t.Fatalf("watch.New: %v", err)
	}
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Edit the tree AFTER the watcher is live: append a new file with a new symbol.
	added := "package svc\n\n// WatcherAddedFunc is written while the watcher runs.\nfunc WatcherAddedFunc() int { return 42 }\n"
	if err := os.WriteFile(filepath.Join(repo, "added.go"), []byte(added), 0o644); err != nil {
		t.Fatalf("write added: %v", err)
	}

	// The background update should pick it up; allow generous slack for CI.
	if !waitFor(t, 8*time.Second, func() bool { return symbolKnown(t, eng, "WatcherAddedFunc") }) {
		t.Fatalf("WatcherAddedFunc never became queryable; watcher updates=%d logs=%q",
			w.Updates(), logs.String())
	}
	// The symbol becoming queryable and the update counter being bumped are two
	// observable effects of the SAME refresh: the snapshot is persisted (symbol
	// visible) a hair before the counter/log are published. Poll the counter so we
	// assert the watcher's own accounting without racing that publish window.
	if !waitFor(t, 2*time.Second, func() bool { return w.Updates() > 0 }) {
		t.Fatalf("watcher reported 0 updates despite the new symbol landing; logs=%q", logs.String())
	}
}

// TestWatcherCleanStop asserts Stop fully releases the watcher: it returns
// promptly (the loop exits and the fsnotify watcher is closed) and a second Stop
// is a harmless no-op. A hung Stop here would mean the goroutine leaked.
func TestWatcherCleanStop(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package p\nfunc A(){}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Index(ctx, atlas.IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("initial Index: %v", err)
	}

	w, err := watch.New(eng, repo, watch.Options{Debounce: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("watch.New: %v", err)
	}
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return within 3s — the watch goroutine leaked")
	}

	// Second Stop must be a no-op, not a panic or a hang.
	w.Stop()
}

// TestWatcherStopsOnContextCancel asserts the loop exits when its context is
// cancelled even without an explicit Stop, so a parent (serve/mcp) shutdown
// tears the watcher down cleanly.
func TestWatcherStopsOnContextCancel(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package p\nfunc A(){}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	eng := newEngine(t)
	if _, err := eng.Index(context.Background(), atlas.IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("initial Index: %v", err)
	}

	w, err := watch.New(eng, repo, watch.Options{Debounce: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("watch.New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Cancel the context; Stop should then return immediately because the loop has
	// already exited (Stop still joins the done channel).
	cancel()
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not exit after context cancel")
	}
}

// TestStartTwiceFails guards the documented contract: a Watcher is single-use
// per Start.
func TestStartTwiceFails(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package p\nfunc A(){}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	eng := newEngine(t)
	w, err := watch.New(eng, repo, watch.Options{})
	if err != nil {
		t.Fatalf("watch.New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer w.Stop()
	if err := w.Start(ctx); err == nil {
		t.Fatal("second Start should have failed")
	}
}

// TestNewRejectsBadRoot covers construction validation.
func TestNewRejectsBadRoot(t *testing.T) {
	eng := newEngine(t)
	if _, err := watch.New(eng, filepath.Join(t.TempDir(), "does-not-exist"), watch.Options{}); err == nil {
		t.Fatal("New should reject a non-existent root")
	}
	if _, err := watch.New(nil, t.TempDir(), watch.Options{}); err == nil {
		t.Fatal("New should reject a nil engine")
	}
}
