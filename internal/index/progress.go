// progress.go carries live indexing progress from deep inside a Run (the parse
// loop) out to a caller that wants to render it (the CLI ticker), WITHOUT adding
// a parameter to Run/parseCandidates or a field to the public IndexInput. The
// counters travel on the context: a caller stuffs a *ProgressCounters in with
// WithProgress, Run/parseCandidates pull it out with ProgressFromContext and bump
// it, and the caller polls Snapshot() on its own cadence. Every method is
// nil-safe, so the non-progress path (tests, library callers) is unaffected.
//
// The per-repo counters (FilesParsed/FilesTotal/Symbols) RESET on SetRepo so a
// segmented multi-repo run shows progress for the repo currently being indexed,
// not a monotonic blur across all of them. The grand total for a segmented run is
// reported separately by the engine from each repo's final Stats.
package index

import (
	"context"
	"sync"
	"sync/atomic"
)

// progressKey is the unexported context key for *ProgressCounters.
type progressKey struct{}

// ProgressCounters is a concurrency-safe live view of one index Run. The hot
// counters (FilesParsed, Symbols) are atomics bumped once per parsed file by every
// parse worker; the cold fields (repo name/phase/index) are mutex-guarded and set
// at phase boundaries only.
type ProgressCounters struct {
	filesParsed atomic.Int64
	symbols     atomic.Int64
	filesTotal  atomic.Int64
	repoIndex   atomic.Int64
	repoTotal   atomic.Int64

	mu       sync.Mutex
	repoName string
	phase    string
}

// ProgressSnapshot is an immutable copy of the counters at one instant.
type ProgressSnapshot struct {
	RepoName    string
	RepoIndex   int // 1-based position in a segmented run (0 when single-repo)
	RepoTotal   int // number of repos in a segmented run (0/1 when single-repo)
	Phase       string
	FilesParsed int
	FilesTotal  int
	Symbols     int
}

// WithProgress returns a context carrying pc so a downstream Run can report into
// it. A nil pc is returned unchanged (the no-progress path).
func WithProgress(ctx context.Context, pc *ProgressCounters) context.Context {
	if pc == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, pc)
}

// ProgressFromContext returns the *ProgressCounters on ctx, or nil. All counter
// methods are nil-safe, so callers do not need to check the result.
func ProgressFromContext(ctx context.Context) *ProgressCounters {
	if ctx == nil {
		return nil
	}
	pc, _ := ctx.Value(progressKey{}).(*ProgressCounters)
	return pc
}

// AddParsed records one parsed file contributing nSymbols. Safe for concurrent
// callers and on a nil receiver.
func (p *ProgressCounters) AddParsed(nSymbols int) {
	if p == nil {
		return
	}
	p.filesParsed.Add(1)
	if nSymbols > 0 {
		p.symbols.Add(int64(nSymbols))
	}
}

// SetFilesTotal records how many candidate files this repo will parse.
func (p *ProgressCounters) SetFilesTotal(n int) {
	if p == nil {
		return
	}
	p.filesTotal.Store(int64(n))
}

// SetPhase records the current coarse phase ("discover", "parse", "go_types",
// "persist", "done").
func (p *ProgressCounters) SetPhase(phase string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.phase = phase
	p.mu.Unlock()
}

// SetRepo switches the live view to a new repo in a segmented run and resets the
// per-repo file/symbol counters. idx is 1-based; total is the repo count.
func (p *ProgressCounters) SetRepo(name string, idx, total int) {
	if p == nil {
		return
	}
	p.filesParsed.Store(0)
	p.symbols.Store(0)
	p.filesTotal.Store(0)
	p.repoIndex.Store(int64(idx))
	p.repoTotal.Store(int64(total))
	p.mu.Lock()
	p.repoName = name
	p.phase = "discover"
	p.mu.Unlock()
}

// Snapshot returns a consistent copy of the current counters.
func (p *ProgressCounters) Snapshot() ProgressSnapshot {
	if p == nil {
		return ProgressSnapshot{}
	}
	p.mu.Lock()
	name, phase := p.repoName, p.phase
	p.mu.Unlock()
	return ProgressSnapshot{
		RepoName:    name,
		RepoIndex:   int(p.repoIndex.Load()),
		RepoTotal:   int(p.repoTotal.Load()),
		Phase:       phase,
		FilesParsed: int(p.filesParsed.Load()),
		FilesTotal:  int(p.filesTotal.Load()),
		Symbols:     int(p.symbols.Load()),
	}
}
