// doclinks.go connects indexed DOCUMENTS (office files, images) to the in-repo
// CODE they reference, so a search/graph query can cross from a design deck or a
// spec to the package it describes — the "link docs ↔ code" half of document
// support. It runs after parsing, over the full symbol set, and emits
// EdgeReferences edges from each document symbol.
//
// Precision over recall: prose is noisy, so a link is created ONLY on a
// high-confidence signal — an exact in-repo file path appearing in the text, or a
// whole-word match of a DISTINCTIVE code symbol name (CamelCase/snake_case or a
// long mixed-case identifier — never a bare common word like "user" or "handler").
package index

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/dominic097/atlas/internal/graph"
)

var (
	// pathToken matches path-like substrings with an extension (a/b/c.go).
	pathToken = regexp.MustCompile(`[\w][\w./-]*\.[A-Za-z0-9]+`)
	// wordToken matches identifier-like words.
	wordToken = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)
)

// linkDocuments returns doc→code reference edges discovered in document symbols'
// text. It is O(total document text), not O(docs × files): it extracts candidate
// tokens from each document and looks them up, rather than scanning every file
// path against every document.
func linkDocuments(symbols []graph.CodeSymbol, files []graph.File) []graph.DependencyEdge {
	var docs []graph.CodeSymbol
	codeByName := map[string]string{} // distinctive symbol name -> its path (first wins)
	for i := range symbols {
		s := &symbols[i]
		if isDocumentSymbol(*s) {
			docs = append(docs, *s)
			continue
		}
		if _, dup := codeByName[s.Name]; !dup && isDistinctiveName(s.Name) {
			codeByName[s.Name] = s.Path
		}
	}
	if len(docs) == 0 {
		return nil
	}
	filePaths := make(map[string]struct{}, len(files))
	for _, f := range files {
		filePaths[f.Path] = struct{}{}
	}

	var edges []graph.DependencyEdge
	for _, d := range docs {
		seen := map[string]struct{}{} // per-document dedupe of targets
		text := d.Doc

		for _, m := range pathToken.FindAllString(text, -1) {
			cand := strings.TrimLeft(m, "./")
			if cand == d.Path {
				continue
			}
			if _, ok := filePaths[cand]; ok {
				if _, dup := seen["f:"+cand]; dup {
					continue
				}
				seen["f:"+cand] = struct{}{}
				edges = append(edges, docEdge(d, cand, cand, "file_path"))
			}
		}

		for _, w := range wordToken.FindAllString(text, -1) {
			if path, ok := codeByName[w]; ok {
				if _, dup := seen["s:"+w]; dup {
					continue
				}
				seen["s:"+w] = struct{}{}
				edges = append(edges, docEdge(d, w, path, "symbol_name"))
			}
		}
	}
	return edges
}

func docEdge(d graph.CodeSymbol, toRef, toPath, matchType string) graph.DependencyEdge {
	return graph.DependencyEdge{
		ID:         uuid.NewString(),
		FromFile:   d.Path,
		FromSymbol: d.ID,
		ToRef:      toRef,
		Kind:       graph.EdgeReferences,
		Language:   d.Language,
		Metadata: graph.JSONBMap{
			"source":     "document_mention",
			"match_type": matchType,
			"to_path":    toPath,
		},
	}
}

// isDocumentSymbol reports whether a symbol came from a document/image parser
// (it carries Metadata["document"] = true).
func isDocumentSymbol(s graph.CodeSymbol) bool {
	if s.Metadata == nil {
		return false
	}
	v, _ := s.Metadata["document"].(bool)
	return v
}

// commonIdentifiers are frequent code names that also occur in ordinary prose;
// matching them would produce noise, so they are never link targets on their own.
var commonIdentifiers = map[string]struct{}{
	"data": {}, "value": {}, "result": {}, "error": {}, "config": {}, "handler": {},
	"service": {}, "client": {}, "server": {}, "request": {}, "response": {}, "user": {},
	"list": {}, "item": {}, "name": {}, "type": {}, "model": {}, "view": {}, "index": {},
	"build": {}, "parse": {}, "format": {}, "create": {}, "update": {}, "delete": {},
	"main": {}, "init": {}, "test": {}, "router": {}, "context": {}, "manager": {},
	"options": {}, "status": {}, "message": {}, "content": {}, "default": {}, "common": {},
}

// isDistinctiveName reports whether a symbol name is specific enough that seeing it
// in prose is a meaningful reference: a CamelCase/snake_case compound, or a long
// mixed-case identifier — but never a bare common word.
func isDistinctiveName(name string) bool {
	if len(name) < 4 {
		return false
	}
	if _, common := commonIdentifiers[strings.ToLower(name)]; common {
		return false
	}
	var hasUpper, hasLower, hasUnderscore, camelHump bool
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c == '_':
			hasUnderscore = true
		}
		if i > 0 && name[i-1] >= 'a' && name[i-1] <= 'z' && c >= 'A' && c <= 'Z' {
			camelHump = true // "userService" / "UserService"
		}
	}
	switch {
	case camelHump:
		return true
	case hasUnderscore && len(name) >= 6:
		return true
	case len(name) >= 8 && hasUpper && hasLower:
		return true
	default:
		return false
	}
}
