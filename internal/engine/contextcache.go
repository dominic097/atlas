package engine

import (
	"container/list"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// contextCacheMaxEntries bounds the in-process context-pack LRU. 64 distinct
// (snapshot, paths, query, budget) bundles is generous for the warm watch/serve
// loop, which re-asks for the same handful of contexts against one snapshot,
// while keeping the retained footprint small.
const contextCacheMaxEntries = 64

// contextCache is a bounded, thread-safe LRU of assembled Context() bundles. The
// key is a fully-resolved (snapshot, sorted paths, query, budget) string; a new
// snapshot yields a new key, so the cache invalidates naturally as the index
// advances. Hits and misses both return/store DEEP COPIES so a caller mutating
// the returned bundle never corrupts a cached one.
//
// The watch/serve warm process benefits most: it keeps the engine resident and
// repeatedly asks for context over the same snapshot, where ListEdges/ListSymbols
// (now SymbolsByIDs/EdgesByFromFiles) plus impact traversal would otherwise rerun
// on every call.
type contextCache struct {
	mu      sync.Mutex
	max     int
	ll      *list.List               // front = most-recently used
	entries map[string]*list.Element // key -> *list element holding *contextCacheEntry
}

type contextCacheEntry struct {
	key    string
	bundle *ContextResult // canonical stored copy; never handed out directly
}

func newContextCache(max int) *contextCache {
	if max < 1 {
		max = 1
	}
	return &contextCache{
		max:     max,
		ll:      list.New(),
		entries: make(map[string]*list.Element, max),
	}
}

// get returns a fresh deep copy of the cached bundle for key, or (nil,false).
func (c *contextCache) get(key string) (*ContextResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	ent := el.Value.(*contextCacheEntry)
	return cloneContextResult(ent.bundle), true
}

// put stores a deep copy of bundle under key, evicting the LRU entry if full.
func (c *contextCache) put(key string, bundle *ContextResult) {
	if bundle == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*contextCacheEntry).bundle = cloneContextResult(bundle)
		return
	}
	el := c.ll.PushFront(&contextCacheEntry{key: key, bundle: cloneContextResult(bundle)})
	c.entries[key] = el
	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.entries, oldest.Value.(*contextCacheEntry).key)
	}
}

// contextCacheGet / contextCachePut are the engine-side lazy-init wrappers.
func (e *localEngine) contextCacheGet(key string) (*ContextResult, bool) {
	c := e.ctxCacheRef()
	if c == nil {
		return nil, false
	}
	return c.get(key)
}

func (e *localEngine) contextCachePut(key string, bundle *ContextResult) {
	e.ctxCacheRef().put(key, bundle)
}

// ctxCacheRef lazily initializes the cache under the lexical mutex (reused as the
// engine's general init guard) and returns it.
func (e *localEngine) ctxCacheRef() *contextCache {
	e.lexicalMu.Lock()
	defer e.lexicalMu.Unlock()
	if e.ctxCache == nil {
		e.ctxCache = newContextCache(contextCacheMaxEntries)
	}
	return e.ctxCache
}

// contextCacheKey folds the snapshot id, sorted seed paths, query, and resolved
// budget into a single stable string. Sorting paths makes the key order-
// independent; the unit separators avoid collisions between fields.
func contextCacheKey(snapshotID string, paths []string, query string, limit, maxFiles, maxEdges, depth int) string {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	var b strings.Builder
	b.WriteString(snapshotID)
	b.WriteByte('\x1f')
	b.WriteString(strings.Join(sorted, "\x1e"))
	b.WriteByte('\x1f')
	b.WriteString(query)
	b.WriteByte('\x1f')
	b.WriteString(strconv.Itoa(limit))
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(maxFiles))
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(maxEdges))
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(depth))
	return b.String()
}

// cloneContextResult deep-copies a ContextResult so the cache never shares mutable
// slices/maps with a caller. Scalar fields copy by value; slices are reallocated;
// the per-symbol/per-edge Metadata maps are cloned.
func cloneContextResult(in *ContextResult) *ContextResult {
	if in == nil {
		return nil
	}
	out := &ContextResult{
		RepoID:     in.RepoID,
		SnapshotID: in.SnapshotID,
		CommitSHA:  in.CommitSHA,
		Mode:       in.Mode,
	}
	// Each slice is reallocated only when the source is non-nil, preserving the
	// nil-vs-empty distinction so the clone is reflect.DeepEqual to the original.
	if in.Files != nil {
		out.Files = make([]ContextFile, len(in.Files))
		for i, f := range in.Files {
			cf := f
			cf.Imports = cloneStrings(f.Imports)
			out.Files[i] = cf
		}
	}
	if in.Symbols != nil {
		out.Symbols = make([]ContextSymbol, len(in.Symbols))
		for i, s := range in.Symbols {
			cs := s
			cs.Metadata = cloneStringAnyMap(s.Metadata)
			out.Symbols[i] = cs
		}
	}
	if in.Edges != nil {
		out.Edges = make([]ContextEdge, len(in.Edges))
		for i, ed := range in.Edges {
			ce := ed
			ce.Metadata = cloneStringAnyMap(ed.Metadata)
			out.Edges[i] = ce
		}
	}
	if in.SearchHits != nil {
		out.SearchHits = make([]SearchHit, len(in.SearchHits))
		copy(out.SearchHits, in.SearchHits)
	}
	if in.ImpactedFiles != nil {
		out.ImpactedFiles = make([]FileImpact, len(in.ImpactedFiles))
		copy(out.ImpactedFiles, in.ImpactedFiles)
	}
	return out
}

// cloneStrings copies a []string, preserving the nil-vs-empty distinction (a nil
// source clones to nil; a non-nil source — even length 0 — clones to a non-nil
// slice) so cloned bundles stay reflect.DeepEqual to their originals.
func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// cloneStringAnyMap shallow-copies a map[string]any (values are JSON scalars /
// nested maps that the engine treats as read-only facts). A nil/empty map clones
// to nil to match the producer paths (copyAnyMap), keeping bundles equal.
func cloneStringAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
