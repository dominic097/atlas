// Package routes is Atlas's content/regex-based HTTP route-contract extractor —
// the cross-repo moat's data producer. For every indexed source file it pulls
// two complementary facts:
//
//   - PRODUCER routes: the (method, path_pattern) -> handler the file SERVES
//     (gorilla/mux, chi, gin, echo, net/http). These say "this repo answers
//     these endpoints, and here is the handler symbol/file behind each".
//   - CONSUMER calls: the outbound HTTP calls the file MAKES (http.Get/Post,
//     http.NewRequest, client.Do, resty, ...). These say "this repo calls that
//     endpoint, from this file/function".
//
// Producer + consumer are persisted as graph.Route rows (Role "producer" /
// "consumer") so the matcher can answer the USP question: a handler changed in
// repo A -> which OTHER repos consume the routes A serves, and from which files.
//
// This is deliberately regex/content based (NOT tree-sitter): the indexer
// already holds the file content in hand during its WalkDir, so extraction is a
// cheap second pass over bytes already read. The Go implementation is ported
// from aziron-pulse internal/service/{route_contract_analyzer,
// cross_service_analyzer}.go; other languages are stubbed for a follow-up.
package routes

import (
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// Role values for a RawRoute / graph.Route.
const (
	RoleProducer = "producer"
	RoleConsumer = "consumer"
)

// RawRoute is one extracted route fact before the producer handler name is
// resolved to its defining symbol. It carries both producer and consumer shapes
// (distinguished by Role); the irrelevant fields are simply empty for each.
//
//   - PRODUCER: Method, Path, HandlerName, File, Role=producer.
//   - CONSUMER: Method (or "" if unknown), Path, File, CallingSymbol, RawURL,
//     Role=consumer.
type RawRoute struct {
	Method        string
	Path          string
	HandlerName   string // producer only: last segment of the handler expr, e.g. "getUser"
	File          string // file the route was found in (producer reg file / consumer calling file)
	Role          string // producer | consumer
	CallingSymbol string // consumer only: enclosing func/method name
	RawURL        string // consumer only: original URL string as written
}

// ExtractFile pulls every producer route and consumer call out of one source
// file's content. The language switch keeps the extractor pluggable; only Go is
// implemented today, the rest return nil until their follow-up lands.
//
// filePath is the repo-relative path (used verbatim as RawRoute.File so the
// resolved graph.Route points back at the right file); content is the file's
// bytes as a string (already read by the indexer's walk).
func ExtractFile(language, filePath, content string) []RawRoute {
	switch language {
	case "go":
		return goRoutes(filePath, content)
	case "javascript", "typescript":
		return jsRoutes(filePath, content)
	case "python":
		return pythonRoutes(filePath, content)
	case "java":
		return javaRoutes(filePath, content)
	default:
		return nil
	}
}

// Resolve turns the raw route facts of a whole repo into persistable graph.Route
// rows. Producer raws have their HandlerName resolved to the handler symbol's
// defining file (via a by-name index over the snapshot's symbols), so the
// route's HandlerFile points at where the handler actually lives — not merely
// where it was registered. Consumer raws keep their calling file as HandlerFile
// (the field doubles as "the relevant file" for both roles).
//
// Producer paths are normalized to {param} form (gin :id -> {id}, trailing
// slashes trimmed) so they match consumer endpoints through the fuzzy matcher.
// Resolution is fail-open: an unresolved handler name falls back to the
// registration file at lower confidence rather than dropping the route.
func Resolve(repoFullName string, raws []RawRoute, syms []graph.CodeSymbol) []graph.Route {
	byName := indexSymbolsByName(syms)
	out := make([]graph.Route, 0, len(raws))
	seen := map[string]bool{}

	for _, rr := range raws {
		switch rr.Role {
		case RoleConsumer:
			rt := graph.Route{
				RepoFullName: repoFullName,
				Method:       strings.ToUpper(strings.TrimSpace(rr.Method)),
				PathPattern:  rr.Path,
				HandlerFile:  rr.File,
				Role:         RoleConsumer,
				Source:       "outbound_call",
				Confidence:   "medium",
				Metadata: graph.JSONBMap{
					"calling_symbol": rr.CallingSymbol,
					"raw_url":        rr.RawURL,
				},
			}
			key := "C|" + rt.Method + "|" + rt.PathPattern + "|" + rt.HandlerFile
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, rt)

		default: // producer
			handlerFile := rr.File
			source, confidence := "route_table", "low"
			if matches := byName[rr.HandlerName]; len(matches) > 0 {
				best := pickHandlerSymbol(matches)
				handlerFile = best.Path
				confidence = "high"
				if len(matches) > 1 {
					confidence = "medium" // ambiguous handler name across files
				}
			}
			rt := graph.Route{
				RepoFullName: repoFullName,
				Method:       strings.ToUpper(strings.TrimSpace(rr.Method)),
				PathPattern:  normalizeRoutePath(rr.Path),
				HandlerFile:  handlerFile,
				Role:         RoleProducer,
				Source:       source,
				Confidence:   confidence,
				Metadata: graph.JSONBMap{
					"handler_symbol": rr.HandlerName,
				},
			}
			// Key on the handler too: two handlers on paths that normalize alike
			// (e.g. "/users/" prefix vs "/users" exact, both -> "/users") are
			// distinct routes and must not collapse into one.
			key := "P|" + rt.Method + "|" + rt.PathPattern + "|" + rt.HandlerFile + "|" + rr.HandlerName
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, rt)
		}
	}
	return out
}

// indexSymbolsByName buckets the snapshot's function/method symbols by name so a
// producer handler expression ("h.getUser" -> "getUser") resolves to its
// defining symbol(s). Only callable kinds are indexed; a name may map to several
// symbols across files (handled by pickHandlerSymbol).
func indexSymbolsByName(syms []graph.CodeSymbol) map[string][]graph.CodeSymbol {
	out := map[string][]graph.CodeSymbol{}
	for _, s := range syms {
		if s.Kind == "method" || s.Kind == "function" {
			out[s.Name] = append(out[s.Name], s)
		}
	}
	return out
}

// pickHandlerSymbol chooses the most likely handler when a name resolves to
// several symbols: prefer one whose file path looks like a handler/controller/
// route/api file, else the first.
func pickHandlerSymbol(matches []graph.CodeSymbol) graph.CodeSymbol {
	for _, s := range matches {
		lp := strings.ToLower(s.Path)
		if strings.Contains(lp, "handler") || strings.Contains(lp, "controller") ||
			strings.Contains(lp, "route") || strings.Contains(lp, "/api/") {
			return s
		}
	}
	return matches[0]
}

// normalizeRoutePath converts gin-style `:id` params to the `{id}` form used
// everywhere else. A trailing slash is DELIBERATELY KEPT: it marks a net/http
// subtree handler ("/users/" serves "/users/{id}"), and the matcher relies on it
// for prefix matching.
func normalizeRoutePath(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if strings.HasPrefix(seg, ":") && len(seg) > 1 {
			parts[i] = "{" + seg[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}
