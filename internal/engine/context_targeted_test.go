package engine

import (
	"context"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/dominic097/atlas/internal/store"
)

// countingStore wraps a real StorageDriver and counts the read calls the engine
// hot paths make, so a test can assert a cache hit issues NO new store reads and
// that the targeted readers are used instead of the load-all ones.
type countingStore struct {
	store.StorageDriver
	mu               sync.Mutex
	listSymbols      int
	listEdges        int
	symbolsByIDs     int
	edgesByFromFiles int
}

func (c *countingStore) ListSymbols(ctx context.Context, snapshotID string) ([]graph.CodeSymbol, error) {
	c.mu.Lock()
	c.listSymbols++
	c.mu.Unlock()
	return c.StorageDriver.ListSymbols(ctx, snapshotID)
}

func (c *countingStore) ListEdges(ctx context.Context, snapshotID string) ([]graph.DependencyEdge, error) {
	c.mu.Lock()
	c.listEdges++
	c.mu.Unlock()
	return c.StorageDriver.ListEdges(ctx, snapshotID)
}

func (c *countingStore) SymbolsByIDs(ctx context.Context, snapshotID string, ids []string) ([]graph.CodeSymbol, error) {
	c.mu.Lock()
	c.symbolsByIDs++
	c.mu.Unlock()
	return c.StorageDriver.SymbolsByIDs(ctx, snapshotID, ids)
}

func (c *countingStore) EdgesByFromFiles(ctx context.Context, snapshotID string, fromFiles []string) ([]graph.DependencyEdge, error) {
	c.mu.Lock()
	c.edgesByFromFiles++
	c.mu.Unlock()
	return c.StorageDriver.EdgesByFromFiles(ctx, snapshotID, fromFiles)
}

func (c *countingStore) snapshot() (ls, le, sid, eff int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listSymbols, c.listEdges, c.symbolsByIDs, c.edgesByFromFiles
}

// buildCountingEngine indexes the standard context fixture with a real engine,
// then returns a second engine over the SAME db + lexical dir whose store is the
// counting wrapper, so reads made during Context() are observable.
func buildCountingEngine(t *testing.T) (*localEngine, *countingStore) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "atlas.db")
	lexDir := filepath.Join(dir, "lexical")

	// Index once with a normal engine so the DB + lexical index are populated.
	writer, err := New(ctx, WithSQLite(dbPath), WithLexicalDir(lexDir))
	if err != nil {
		t.Fatalf("New(writer): %v", err)
	}
	repo := writeContextFixture(t)
	if _, err := writer.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(writer): %v", err)
	}

	// Open a fresh driver against the same DB and wrap it in the counter.
	drv, err := store.Open(ctx, store.Options{Kind: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := drv.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	cs := &countingStore{StorageDriver: drv}

	eng := &localEngine{
		cfg:   Config{Tier: "local", StorageKind: "sqlite", SQLitePath: dbPath, LexicalDir: lexDir},
		store: cs,
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng, cs
}

// TestContextCacheHitReturnsEqualBundleWithoutRequerying asserts the second
// identical Context() call is served from the in-process LRU: it returns a bundle
// deeply equal to the first, and issues NO new store reads.
func TestContextCacheHitReturnsEqualBundleWithoutRequerying(t *testing.T) {
	ctx := context.Background()
	eng, cs := buildCountingEngine(t)

	in := ContextInput{Paths: []string{"auth.go"}}
	first, err := eng.Context(ctx, in)
	if err != nil {
		t.Fatalf("Context(first): %v", err)
	}
	ls1, le1, sid1, eff1 := cs.snapshot()
	// The first call must use the TARGETED readers, never the load-all ones.
	if ls1 != 0 {
		t.Errorf("ListSymbols called %d times — load-all path still live", ls1)
	}
	if le1 != 0 {
		t.Errorf("ListEdges called %d times — load-all path still live", le1)
	}
	if eff1 == 0 {
		t.Error("EdgesByFromFiles never called — contextEdges not using the targeted reader")
	}

	second, err := eng.Context(ctx, in)
	if err != nil {
		t.Fatalf("Context(second): %v", err)
	}
	ls2, le2, sid2, eff2 := cs.snapshot()

	// Cache hit: counters must not advance on the second call.
	if ls2 != ls1 || le2 != le1 || sid2 != sid1 || eff2 != eff1 {
		t.Errorf("cache miss on repeat call: counts before {ls:%d le:%d sid:%d eff:%d} after {ls:%d le:%d sid:%d eff:%d}",
			ls1, le1, sid1, eff1, ls2, le2, sid2, eff2)
	}

	// The cached bundle must equal the first (deep) and be a distinct object.
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cached bundle differs from original:\n first=%+v\nsecond=%+v", first, second)
	}
	if first == second {
		t.Error("cache returned the SAME pointer; a caller mutation would corrupt the cache")
	}

	// A different budget is a different key → a real recompute (counters advance).
	if _, err := eng.Context(ctx, ContextInput{Paths: []string{"auth.go"}, MaxEdges: 1}); err != nil {
		t.Fatalf("Context(diff budget): %v", err)
	}
	ls3, _, _, eff3 := cs.snapshot()
	if eff3 == eff2 || ls3 != ls2 {
		t.Errorf("different budget should recompute via targeted readers: eff %d→%d, listSymbols %d→%d", eff2, eff3, ls2, ls3)
	}
}

// TestContextEdgesEqualsLoadAllFilter proves the targeted EdgesByFromFiles +
// in-memory cap produces the SAME ContextEdge slice the prior ListEdges +
// in-memory filter would have. It computes the load-all reference directly from
// the same fixture data and compares.
func TestContextEdgesEqualsLoadAllFilter(t *testing.T) {
	ctx := context.Background()
	eng, cs := buildCountingEngine(t)

	snap, err := eng.resolveSnapshot(ctx, "")
	if err != nil {
		t.Fatalf("resolveSnapshot: %v", err)
	}

	// Reference: the OLD approach — ListEdges over the whole snapshot, then filter
	// by the selected paths in memory, capped at maxEdges (mirrors the pre-change
	// contextEdges body exactly).
	pathSet := map[string]bool{"auth.go": true, "token.go": true}
	const maxEdges = 500
	allEdges, err := cs.StorageDriver.ListEdges(ctx, snap.ID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	want := make([]ContextEdge, 0, len(allEdges))
	for _, edge := range allEdges {
		if !pathSet[edge.FromFile] {
			continue
		}
		want = append(want, ContextEdge{
			FromFile:   edge.FromFile,
			FromSymbol: edge.FromSymbol,
			ToRef:      edge.ToRef,
			Kind:       string(edge.Kind),
			Language:   edge.Language,
			Line:       edge.Line,
			Metadata:   copyAnyMap(edge.Metadata),
		})
		if len(want) >= maxEdges {
			break
		}
	}

	// New approach: the targeted contextEdges.
	got, err := eng.contextEdges(ctx, snap.ID, pathSet, maxEdges)
	if err != nil {
		t.Fatalf("contextEdges: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("targeted contextEdges diverges from load-all filter:\n got=%+v\nwant=%+v", got, want)
	}
	if len(got) == 0 {
		t.Fatal("fixture produced no edges — test is vacuous")
	}
}

// TestLexicalSearchEqualsLoadAll proves lexicalSearch (now resolving hits via
// SymbolsByIDs) returns the SAME hits, in the same order, the prior ListSymbols
// byID approach produced. The reference reconstructs the old mapping from the
// same lexical hits + a whole-snapshot symbol map.
func TestLexicalSearchEqualsLoadAll(t *testing.T) {
	ctx := context.Background()
	eng, cs := buildCountingEngine(t)

	snap, err := eng.resolveSnapshot(ctx, "")
	if err != nil {
		t.Fatalf("resolveSnapshot: %v", err)
	}

	const (
		query = "token"
		limit = 20
	)
	got, err := eng.lexicalSearch(ctx, snap.ID, query, "", limit)
	if err != nil {
		t.Fatalf("lexicalSearch: %v", err)
	}

	// Reference: run the same BM25 hits, map them through a whole-snapshot byID
	// map (the OLD load-all behaviour), and project identically.
	lx, err := eng.ensureLexical()
	if err != nil {
		t.Fatalf("ensureLexical: %v", err)
	}
	hits, err := lx.Search(snap.ID, query, limit*2)
	if err != nil {
		t.Fatalf("lx.Search: %v", err)
	}
	all, err := cs.StorageDriver.ListSymbols(ctx, snap.ID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	byID := make(map[string]graph.CodeSymbol, len(all))
	for _, s := range all {
		byID[s.ID] = s
	}
	want := make([]SearchHit, 0, limit)
	for _, h := range hits {
		s, ok := byID[h.SymbolID]
		if !ok {
			continue
		}
		want = append(want, symbolToHit(s, h.Score))
		if len(want) >= limit {
			break
		}
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("targeted lexicalSearch diverges from load-all byID:\n got=%+v\nwant=%+v", got, want)
	}
	if len(got) == 0 {
		t.Fatal("fixture produced no lexical hits for query — test is vacuous")
	}
}
