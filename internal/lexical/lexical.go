// Package lexical implements an on-disk bleve (BM25) symbol index with a
// code-aware analyzer. It is adapted from the proven code search
// service (internal/service/code_search_service.go: BuildIndex / bm25Search),
// adapted to the Atlas graph types and extended with a code-aware analyzer that
// splits camelCase / snake_case / kebab-case identifiers while preserving the
// original whole token — so "GetUserById" is retrievable by "user", "get",
// "getuserbyid", etc.
package lexical

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	unicodetok "github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/registry"

	"github.com/dominic097/atlas/internal/graph"
)

const (
	// codeIdentifierFilter is the registered name of the token filter that
	// applies TokenizeIdentifier semantics (camel/snake/kebab split + keep the
	// original whole token, lowercased).
	codeIdentifierFilter = "atlas_code_identifier"
	// codeAnalyzer is the registered name of the code-aware analyzer.
	codeAnalyzer = "atlas_code"

	// fieldSnapshotID scopes every doc to its snapshot so Search can filter.
	fieldSnapshotID = "snapshot_id"
	fieldName       = "name"
	fieldKind       = "kind"
	fieldSignature  = "signature"
	fieldDoc        = "doc"
	fieldPath       = "path"
	fieldLanguage   = "language"

	// nameBoost ranks a name-field match above signature/doc/path matches.
	nameBoost = 3.0

	indexBatchSize = 256
)

// registerOnce guards the global bleve registry registration of the code
// identifier token filter (the registry is process-global; double registration
// panics).
var registerOnce sync.Once

func init() {
	registerOnce.Do(func() {
		_ = registry.RegisterTokenFilter(codeIdentifierFilter, codeIdentifierFilterConstructor)
	})
}

// Hit is a single search result: the symbol id and its BM25 score.
type Hit struct {
	SymbolID string  `json:"symbol_id"`
	Score    float64 `json:"score"`
}

// Index is a persistent, code-aware bleve index over code symbols. A single
// physical index holds symbols from many snapshots; Search scopes by
// snapshot_id. It is safe for concurrent use.
type Index struct {
	mu  sync.RWMutex
	dir string
	idx bleve.Index
}

// New creates or opens a persistent bleve index under dir. The directory is
// created if missing; an existing index at that path is reopened.
func New(dir string) (*Index, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("lexical: index dir is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, fmt.Errorf("lexical: create index parent dir: %w", err)
	}

	var (
		idx bleve.Index
		err error
	)
	if _, statErr := os.Stat(dir); statErr == nil {
		// Reopen an existing on-disk index.
		idx, err = bleve.Open(dir)
		if err != nil {
			return nil, fmt.Errorf("lexical: open index at %s: %w", dir, err)
		}
	} else {
		m, mErr := buildIndexMapping()
		if mErr != nil {
			return nil, mErr
		}
		idx, err = bleve.New(dir, m)
		if err != nil {
			return nil, fmt.Errorf("lexical: create index at %s: %w", dir, err)
		}
	}

	return &Index{dir: dir, idx: idx}, nil
}

// buildIndexMapping wires the code-aware analyzer onto the searched text fields
// (name/signature/doc/path) with name boosted, and a keyword analyzer on the
// snapshot_id/kind/language fields so they match exactly for filtering.
func buildIndexMapping() (mapping.IndexMapping, error) {
	im := bleve.NewIndexMapping()

	// Code-aware analyzer: unicode tokenizer (splits on whitespace/punctuation
	// into identifier-ish tokens) followed by the code identifier filter that
	// applies camel/snake/kebab splitting + keeps the original token, lowercased.
	if err := im.AddCustomAnalyzer(codeAnalyzer, map[string]interface{}{
		"type":          custom.Name,
		"tokenizer":     unicodetok.Name,
		"token_filters": []string{codeIdentifierFilter},
	}); err != nil {
		return nil, fmt.Errorf("lexical: register code analyzer: %w", err)
	}
	im.DefaultAnalyzer = keyword.Name

	doc := bleve.NewDocumentMapping()

	codeField := func() *mapping.FieldMapping {
		fm := bleve.NewTextFieldMapping()
		fm.Analyzer = codeAnalyzer
		fm.Store = true
		fm.IncludeInAll = false
		fm.IncludeTermVectors = false
		return fm
	}

	keywordField := func() *mapping.FieldMapping {
		fm := bleve.NewTextFieldMapping()
		fm.Analyzer = keyword.Name
		fm.Store = true
		fm.IncludeInAll = false
		fm.IncludeTermVectors = false
		return fm
	}

	doc.AddFieldMappingsAt(fieldName, codeField())
	doc.AddFieldMappingsAt(fieldSignature, codeField())
	doc.AddFieldMappingsAt(fieldDoc, codeField())
	doc.AddFieldMappingsAt(fieldPath, codeField())
	doc.AddFieldMappingsAt(fieldKind, keywordField())
	doc.AddFieldMappingsAt(fieldLanguage, keywordField())
	doc.AddFieldMappingsAt(fieldSnapshotID, keywordField())

	im.DefaultMapping = doc
	return im, nil
}

// symbolDoc is the indexed representation of a CodeSymbol.
type symbolDoc struct {
	SnapshotID string `json:"snapshot_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Signature  string `json:"signature"`
	Doc        string `json:"doc"`
	Path       string `json:"path"`
	Language   string `json:"language"`
}

// BuildForSnapshot indexes the given symbols under snapshotID. Each symbol is a
// doc keyed by Symbol.ID. Re-indexing the same symbol ids overwrites them, so a
// rebuild for the same snapshot is idempotent.
func (ix *Index) BuildForSnapshot(snapshotID string, syms []graph.CodeSymbol) error {
	if ix == nil || ix.idx == nil {
		return fmt.Errorf("lexical: index is nil")
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()

	batch := ix.idx.NewBatch()
	n := 0
	for i := range syms {
		s := &syms[i]
		if strings.TrimSpace(s.ID) == "" {
			continue
		}
		d := symbolDoc{
			SnapshotID: snapshotID,
			Name:       s.Name,
			Kind:       s.Kind,
			Signature:  s.Signature,
			Doc:        s.Doc,
			Path:       s.Path,
			Language:   s.Language,
		}
		if err := batch.Index(s.ID, d); err != nil {
			// Skip a single bad doc rather than abort the whole snapshot.
			continue
		}
		n++
		if n%indexBatchSize == 0 {
			if err := ix.idx.Batch(batch); err != nil {
				return fmt.Errorf("lexical: batch index: %w", err)
			}
			batch = ix.idx.NewBatch()
		}
	}
	if batch.Size() > 0 {
		if err := ix.idx.Batch(batch); err != nil {
			return fmt.Errorf("lexical: batch index (final): %w", err)
		}
	}
	return nil
}

// UpdateForSnapshot incrementally updates the bleve index for snapshotID in
// place, WITHOUT rebuilding the whole snapshot. It is the delta-path counterpart
// to BuildForSnapshot: when an index run reuses an existing snapshot id (an
// uncommitted edit re-indexed against the same commit — see SaveSnapshot's
// (repo_id, commit_sha) idempotency), only the changed files' docs need to move,
// so a full BuildForSnapshot over every merged symbol is wasted work.
//
// It does two things, in one batch, under the write lock:
//
//   - DELETE: every doc id in removeSymbolIDs is removed. The caller passes the
//     BASE snapshot's symbol ids for the touched files (changed ∪ added ∪
//     deleted) — those are exactly the docs that were indexed under this snapshot
//     id and must go. The doc-id scheme is the symbol id (matching
//     BuildForSnapshot's batch.Index(s.ID, …)), so a base symbol id deletes the
//     base doc it created.
//   - INDEX: every symbol in newSymbols is (re)indexed under its id, identically
//     to BuildForSnapshot (same symbolDoc shape, same analyzer, same batching).
//     The caller passes the freshly-parsed symbols for the changed/added files.
//
// The net effect is byte-for-byte equivalent — for search purposes — to a full
// BuildForSnapshot of the merged symbol set, because the carried-forward
// (untouched-file) docs were never disturbed and remain keyed by their original
// (preserved) symbol ids, while the touched files' docs are swapped wholesale.
// The caller MUST preserve the carried-forward symbols' ids across the reused
// snapshot (no re-stamp) so the kept docs still map to the persisted rows.
//
// Re-indexing a doc id that does not yet exist is an insert; deleting a doc id
// that is absent is a harmless no-op — so an "added" file (no base doc) and a
// "deleted" file (no new symbol) are both handled by passing the right ids.
func (ix *Index) UpdateForSnapshot(snapshotID string, removeSymbolIDs []string, newSymbols []graph.CodeSymbol) error {
	if ix == nil || ix.idx == nil {
		return fmt.Errorf("lexical: index is nil")
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()

	batch := ix.idx.NewBatch()
	n := 0
	flush := func() error {
		if batch.Size() == 0 {
			return nil
		}
		if err := ix.idx.Batch(batch); err != nil {
			return fmt.Errorf("lexical: update batch: %w", err)
		}
		batch = ix.idx.NewBatch()
		return nil
	}

	for _, id := range removeSymbolIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		batch.Delete(id)
		n++
		if n%indexBatchSize == 0 {
			if err := flush(); err != nil {
				return err
			}
		}
	}

	for i := range newSymbols {
		s := &newSymbols[i]
		if strings.TrimSpace(s.ID) == "" {
			continue
		}
		d := symbolDoc{
			SnapshotID: snapshotID,
			Name:       s.Name,
			Kind:       s.Kind,
			Signature:  s.Signature,
			Doc:        s.Doc,
			Path:       s.Path,
			Language:   s.Language,
		}
		if err := batch.Index(s.ID, d); err != nil {
			// Skip a single bad doc rather than abort the whole update.
			continue
		}
		n++
		if n%indexBatchSize == 0 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// Search runs a code-aware disjunction query scoped to snapshotID and returns
// the top-N hits by BM25 score (descending). The query is matched against the
// name (boosted) / signature / doc / path fields, with each query term passed
// through the same code-aware analyzer so "GetUserById" indexed tokens are
// reachable by "user".
func (ix *Index) Search(snapshotID, query string, limit int) (hits []Hit, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			hits = nil
			err = fmt.Errorf("lexical: search panic: %v", recovered)
		}
	}()
	if ix == nil || ix.idx == nil {
		return nil, fmt.Errorf("lexical: index is nil")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	ix.mu.RLock()
	defer ix.mu.RUnlock()

	// Disjunction across the code-analyzed fields; a MatchQuery analyzes the
	// query string through the field's mapped (code-aware) analyzer.
	disj := bleve.NewDisjunctionQuery()
	for _, f := range []struct {
		field string
		boost float64
	}{
		{fieldName, nameBoost},
		{fieldSignature, 1.0},
		{fieldDoc, 1.0},
		{fieldPath, 1.0},
	} {
		mq := bleve.NewMatchQuery(query)
		mq.SetField(f.field)
		mq.SetBoost(f.boost)
		disj.AddQuery(mq)
	}
	disj.SetMin(1)

	// Scope to the snapshot via an exact (keyword) term match.
	scope := bleve.NewTermQuery(strings.ToLower(snapshotID))
	scope.SetField(fieldSnapshotID)

	conj := bleve.NewConjunctionQuery(scope, disj)

	// Pull extra candidates then trim to limit, matching pulse bm25Search.
	size := limit
	if size < 50 {
		size = 50
	}
	req := bleve.NewSearchRequest(conj)
	req.Size = size
	req.Fields = []string{}

	res, err := ix.idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("lexical: search: %w", err)
	}

	out := make([]Hit, 0, len(res.Hits))
	for _, h := range res.Hits {
		out = append(out, Hit{SymbolID: h.ID, Score: h.Score})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Close releases the underlying bleve index.
func (ix *Index) Close() error {
	if ix == nil || ix.idx == nil {
		return nil
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	err := ix.idx.Close()
	ix.idx = nil
	return err
}

// TokenizeIdentifier splits an identifier on camelCase boundaries,
// snake_case (_), kebab-case (-), dots, slashes and other non-alphanumeric
// separators, lowercases every piece, AND keeps the original whole token
// (lowercased). So "GetUserById" -> ["getuserbyid", "get", "user", "by", "id"]
// and "user_profile-v2" -> ["user_profile-v2", "user", "profile", "v2"].
//
// The order is deterministic: the whole token first, then the split pieces in
// source order, de-duplicated.
func TokenizeIdentifier(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	out := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	add := func(tok string) {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			return
		}
		if _, ok := seen[tok]; ok {
			return
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}

	// Keep the original whole token first (lowercased).
	add(s)

	// Split on separators into "words", then split each word on camelCase.
	for _, word := range splitOnSeparators(s) {
		for _, piece := range splitCamelCase(word) {
			add(piece)
		}
	}
	return out
}

// splitOnSeparators breaks a string on any non-alphanumeric rune (covers _, -,
// ., /, spaces, etc.), discarding empty pieces.
func splitOnSeparators(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// splitCamelCase splits a single separator-free word on camelCase boundaries,
// keeping runs of uppercase (acronyms) together until a lowercase letter starts
// a new word, and breaking letter<->digit transitions. E.g. "GetUserByID" ->
// ["Get","User","By","ID"], "HTTPServer" -> ["HTTP","Server"], "v2Beta" ->
// ["v","2","Beta"]... we keep digit runs attached to the preceding letters only
// at word level; here we split letter/digit transitions so "id2" -> ["id","2"].
func splitCamelCase(word string) []string {
	if word == "" {
		return nil
	}
	runes := []rune(word)
	var words []string
	start := 0

	classify := func(r rune) int {
		switch {
		case unicode.IsUpper(r):
			return 1 // upper
		case unicode.IsLower(r):
			return 2 // lower
		case unicode.IsDigit(r):
			return 3 // digit
		default:
			return 0
		}
	}

	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		cur := runes[i]
		boundary := false

		switch {
		// lower/digit -> upper : userName | id2X (id|2 handled by class change)
		case classify(cur) == 1 && (classify(prev) == 2 || classify(prev) == 3):
			boundary = true
		// UPPER run followed by Upper+lower : HTTPServer -> HTTP | Server
		case classify(cur) == 2 && classify(prev) == 1 && i >= 2 && classify(runes[i-2]) == 1:
			// Break before the previous uppercase letter (it starts the new word).
			words = append(words, string(runes[start:i-1]))
			start = i - 1
			continue
		// letter <-> digit transitions
		case classify(cur) == 3 && (classify(prev) == 1 || classify(prev) == 2):
			boundary = true
		case (classify(cur) == 1 || classify(cur) == 2) && classify(prev) == 3:
			boundary = true
		}

		if boundary {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	words = append(words, string(runes[start:]))
	return words
}

// codeIdentifierFilter applies TokenizeIdentifier to each incoming token,
// emitting the original + split sub-tokens as a single re-numbered stream. This
// is the bleve token filter behind the code-aware analyzer.
type codeIdentifierTokenFilter struct{}

func codeIdentifierFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return &codeIdentifierTokenFilter{}, nil
}

func (f *codeIdentifierTokenFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	out := make(analysis.TokenStream, 0, len(input)*2)
	pos := 1
	for _, tok := range input {
		pieces := TokenizeIdentifier(string(tok.Term))
		if len(pieces) == 0 {
			continue
		}
		for _, p := range pieces {
			out = append(out, &analysis.Token{
				Term:     []byte(p),
				Start:    tok.Start,
				End:      tok.End,
				Position: pos,
				Type:     tok.Type,
			})
			pos++
		}
	}
	return out
}
