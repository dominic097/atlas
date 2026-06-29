// concurrent.go overlaps the two INDEPENDENT terminal writes of an index run —
// persisting the snapshot to the StorageDriver and (re)building the lexical (BM25)
// index — so they execute at the same time instead of strictly back-to-back.
//
// Phase 1C rationale: the pipeline used to run SaveSnapshot (write_sqlite ~1.9s on
// a large repo) and THEN BuildForSnapshot (lexical ~2.5s) sequentially, even though
// they consume the SAME in-memory symbol slice (read-only) and write to two
// completely separate stores (the SQLite db file vs the Bleve index directory). The
// two have no data dependency, so running them concurrently hides one behind the
// other. The output is identical to the sequential order because:
//
//   - Both functions only READ the shared symbols slice; neither mutates it.
//   - They write disjoint storage (different files), so there is no write contention.
//   - errgroup propagates the first error and waits for both, so failure semantics
//     match "do persist; if ok do lexical" closely enough for our callers (both
//     errors are fatal to the run either way).
package index

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// concurrentPersistLexical, when true, runs the persist and lexical phases
// concurrently (the production path). Tests flip it to false (via
// withSequentialPersistLexical) to assert the concurrent path yields a result
// identical to the sequential one. It is a package-level constant-style toggle, not
// a runtime flag — the production value never changes.
var concurrentPersistLexical = true

// withSequentialPersistLexical forces the sequential persist→lexical order for the
// duration of a test, restoring the concurrent default on cleanup. Used by the 1C
// equivalence test to A/B the two orderings against the same input.
func withSequentialPersistLexical(t interface{ Cleanup(func()) }) {
	prev := concurrentPersistLexical
	concurrentPersistLexical = false
	t.Cleanup(func() { concurrentPersistLexical = prev })
}

// runPersistAndLexical executes the persist closure and the lexical closure. When
// concurrentPersistLexical is true (production) they run in parallel via errgroup;
// otherwise they run sequentially (persist first), which is the behavior tests pin
// as the reference. lexical may be nil (no lexical index configured), in which case
// only persist runs. The first error from either is returned; both always complete
// (or are waited on) before return so no goroutine outlives the call.
//
// Because the two closures write independent stores and only read shared in-memory
// state, the persisted snapshot and the lexical index are byte-for-byte the same
// regardless of which ordering ran.
func runPersistAndLexical(ctx context.Context, persist func() error, lexical func() error) error {
	if lexical == nil {
		return persist()
	}
	if !concurrentPersistLexical {
		if err := persist(); err != nil {
			return err
		}
		return lexical()
	}
	g, _ := errgroup.WithContext(ctx)
	g.Go(persist)
	g.Go(lexical)
	return g.Wait()
}
