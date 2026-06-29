package parser

import (
	"embed"
	"sort"
	"strings"
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// tagsquery.go is Atlas's GENERIC tree-sitter "tags query" symbol extractor —
// the reusable engine that turns ANY grammar's tree-sitter tags query
// (queries/<lang>.scm, normally the grammar's official queries/tags.scm) into
// Atlas symbolDrafts via real AST matching. It replaces the per-language regex
// fallback for languages routed through tagsSymbols (rust, ruby, csharp, php in
// this batch; designed so later batches add a grammar + a .scm and route the
// language with NO new Go code).
//
// Tags-query convention (shared across tree-sitter grammars): a definition site
// captures the whole def node as @definition.<kind> and its name node as @name.
// We emit one symbolDraft per @definition.* capture, taking the kind from the
// capture suffix (mapped to Atlas kinds) and the name from the @name capture in
// the SAME match. References (@reference.*) and bookkeeping captures (@doc,
// @ignore, bare @module) are ignored here — only @definition.* produces symbols.

//go:embed queries/*.scm
var tagsQueryFS embed.FS

// tagsQueryForLanguage returns the embedded tags query (.scm) source for a
// language, or "" when none is registered. Centralizes the lang→file mapping so
// adding a language is a one-line addition plus the .scm file.
func tagsQueryForLanguage(language string) string {
	data, err := tagsQueryFS.ReadFile("queries/" + language + ".scm")
	if err != nil {
		return ""
	}
	return string(data)
}

// compiledTagsQuery caches the compiled tree_sitter.Query per language so the
// (relatively expensive) query compilation runs exactly once. The grammar
// pointer is stable per language, so the cache key is the language name.
var compiledTagsQuery sync.Map // map[string]*tagsQueryEntry

type tagsQueryEntry struct {
	query *tree_sitter.Query
	// nameIdx / defKinds are precomputed from the compiled query's capture
	// names so the hot path avoids string work per match:
	//   nameIdx        — capture index of @name (the def's name node)
	//   defKinds[i]    — the Atlas kind for capture index i when it is a
	//                    @definition.<suffix> capture, else "" (not a def).
	nameIdx  uint
	hasName  bool
	defKinds []string
}

// getCompiledTagsQuery returns (and memoizes) the compiled query + capture
// metadata for a language/grammar/source triple. ok is false when the query
// fails to compile (left to the caller to fall back gracefully).
func getCompiledTagsQuery(language string, grammar *tree_sitter.Language, source string) (*tagsQueryEntry, bool) {
	if cached, ok := compiledTagsQuery.Load(language); ok {
		entry := cached.(*tagsQueryEntry)
		return entry, entry != nil
	}
	entry := buildTagsQueryEntry(grammar, source)
	// Store even a nil entry so a permanently-broken query is not recompiled on
	// every file. LoadOrStore guards against a benign compile race.
	actual, _ := compiledTagsQuery.LoadOrStore(language, entry)
	resolved := actual.(*tagsQueryEntry)
	return resolved, resolved != nil
}

func buildTagsQueryEntry(grammar *tree_sitter.Language, source string) *tagsQueryEntry {
	if grammar == nil || source == "" {
		return nil
	}
	q, qErr := tree_sitter.NewQuery(grammar, source)
	if qErr != nil || q == nil {
		return nil
	}
	names := q.CaptureNames()
	entry := &tagsQueryEntry{
		query:    q,
		defKinds: make([]string, len(names)),
	}
	for i, name := range names {
		switch {
		case name == "name":
			entry.nameIdx = uint(i)
			entry.hasName = true
		case strings.HasPrefix(name, "definition."):
			entry.defKinds[i] = atlasKindForDefinition(strings.TrimPrefix(name, "definition."))
		}
	}
	return entry
}

// atlasKindForDefinition maps a tags-query @definition.<suffix> to the Atlas
// symbol kind. Suffixes follow the cross-grammar tags convention; unknown
// suffixes pass through verbatim so a grammar that invents a definition kind
// (e.g. @definition.macro) still yields a usefully-typed symbol rather than
// being dropped.
func atlasKindForDefinition(suffix string) string {
	switch suffix {
	case "function":
		return "function"
	case "method":
		return "method"
	case "class":
		return "class"
	case "interface":
		return "interface"
	case "struct":
		return "struct"
	case "module":
		return "module"
	case "type":
		return "type"
	case "constant":
		return "constant"
	case "enum":
		return "enum"
	case "constructor":
		return "constructor"
	case "field":
		return "field"
	default:
		return suffix
	}
}

// tagsSymbols is the GENERIC native-AST symbol extractor. It parses content with
// the given grammar (mirroring parseTreeSitter's parse/cleanup lifecycle), runs
// the compiled tags query over the tree via a QueryCursor, and emits one
// symbolDraft per @definition.* capture:
//
//   - kind: the Atlas kind mapped from the capture suffix (definition.function →
//     function, definition.class → class, …);
//   - name: the text of the @name capture in the same match (fallback: the def
//     node's own name field / first identifier child);
//   - startLine/endLine: the def node's span (1-based);
//   - signature: the first source line of the def node.
//
// Output is deterministic: matches are collected then sorted by
// (startLine, kind, name, endLine) and identical (name,kind,startLine) drafts
// are de-duplicated.
func tagsSymbols(language string, grammar *tree_sitter.Language, tagsQuery string, content []byte) []symbolDraft {
	if grammar == nil || len(content) == 0 {
		return nil
	}
	entry, ok := getCompiledTagsQuery(language, grammar, tagsQuery)
	if !ok {
		return nil
	}

	p := tree_sitter.NewParser()
	if err := p.SetLanguage(grammar); err != nil {
		p.Close()
		return nil
	}
	defer p.Close()

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil
	}

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	var drafts []symbolDraft
	matches := cursor.Matches(entry.query, root, content)
	for match := matches.Next(); match != nil; match = matches.Next() {
		// Find the @name node (if any) for this match once, then emit a draft for
		// each @definition.* capture in the match. Most tags patterns have exactly
		// one of each, but alternations (e.g. ruby's class/singleton_class) can
		// carry a single name with one def capture per match.
		var nameNode *tree_sitter.Node
		if entry.hasName {
			for i := range match.Captures {
				c := &match.Captures[i]
				if uint(c.Index) == entry.nameIdx {
					n := c.Node
					nameNode = &n
					break
				}
			}
		}
		for i := range match.Captures {
			c := &match.Captures[i]
			kind := entry.defKinds[c.Index]
			if kind == "" {
				continue
			}
			defNode := c.Node
			name := ""
			if nameNode != nil {
				name = strings.TrimSpace(nodeText(nameNode, content))
			}
			if name == "" {
				name = strings.TrimSpace(tagsDefFallbackName(&defNode, content))
			}
			if name == "" {
				continue
			}
			drafts = append(drafts, symbolDraft{
				name:      name,
				kind:      kind,
				signature: tagsFirstLine(&defNode, content),
				startLine: int(defNode.StartPosition().Row) + 1,
				endLine:   int(defNode.EndPosition().Row) + 1,
			})
		}
	}
	return sortDedupDrafts(drafts)
}

// tagsDefFallbackName derives a name for a definition node when the tags query
// supplied no @name capture (rare — some grammars capture only the def node).
// It prefers the conventional `name` field, then the first identifier-like
// child. Empty when none is found (the draft is then dropped).
func tagsDefFallbackName(def *tree_sitter.Node, src []byte) string {
	if def == nil {
		return ""
	}
	if n := def.ChildByFieldName("name"); n != nil {
		if t := strings.TrimSpace(nodeText(n, src)); t != "" {
			return t
		}
	}
	for i := uint(0); i < def.ChildCount(); i++ {
		child := def.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "identifier", "type_identifier", "name", "constant",
			"property_identifier", "field_identifier":
			if t := strings.TrimSpace(nodeText(child, src)); t != "" {
				return t
			}
		}
	}
	return ""
}

// tagsFirstLine returns the trimmed first source line spanned by a def node, the
// signature used for tags-extracted symbols (mirrors the one-line signatures the
// hand-written walkers produce). Bounded to keep signatures compact.
func tagsFirstLine(def *tree_sitter.Node, src []byte) string {
	sig := nodeText(def, src)
	if nl := strings.IndexByte(sig, '\n'); nl >= 0 {
		sig = sig[:nl]
	}
	sig = strings.TrimSpace(sig)
	const maxLen = 160
	if len(sig) > maxLen {
		return sig[:maxLen] + "..."
	}
	return sig
}

// sortDedupDrafts returns drafts in a stable, deterministic order and collapses
// drafts that name the SAME definition (same name+kind+startLine), which a tags
// query can produce when overlapping patterns both match a def site.
func sortDedupDrafts(in []symbolDraft) []symbolDraft {
	if len(in) == 0 {
		return nil
	}
	sort.SliceStable(in, func(a, b int) bool {
		if in[a].startLine != in[b].startLine {
			return in[a].startLine < in[b].startLine
		}
		if in[a].kind != in[b].kind {
			return in[a].kind < in[b].kind
		}
		if in[a].name != in[b].name {
			return in[a].name < in[b].name
		}
		return in[a].endLine < in[b].endLine
	})
	seen := make(map[string]struct{}, len(in))
	out := make([]symbolDraft, 0, len(in))
	for _, d := range in {
		k := d.name + "\x00" + d.kind + "\x00" + itoa(d.startLine)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, d)
	}
	return out
}
