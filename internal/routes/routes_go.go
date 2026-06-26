package routes

import (
	"regexp"
	"strings"
)

// goRoutes is the Go-language extractor: PRODUCER route registrations + CONSUMER
// outbound HTTP calls from one file's content.
//
// PRODUCER (adapted from the original route analyzer): net/http
// HandleFunc, gorilla/mux r.HandleFunc(...).Methods(...), chi/gin/echo method
// calls r.Get/Post/GET/POST(...). Subrouter prefixes declared in the same file
// (PathPrefix(...).Subrouter()) are chained back to a local mux.NewRouter() root
// so a route on a prefixed subrouter gets its full path. Atlas extracts
// per-file, so cross-file entry-prefix recovery (pulse's buildEntryPrefixIndex)
// is intentionally dropped — the in-file prefix chain is resolved with an empty
// seed.
//
// CONSUMER (adapted from the original cross-service analyzer, Go branch):
// http.Get/Post/Put/Delete/Patch/Do, http.NewRequest(method, url, ...),
// client.Do(...), and resty request builders. Each call's method + URL is
// captured; extractPath strips scheme/host to the path. Every consumer call is
// attributed to its enclosing func/method (best-effort: scan backwards for the
// nearest preceding func declaration).
func goRoutes(filePath, content string) []RawRoute {
	out := extractGoProducerRoutes(content, filePath)
	out = append(out, extractGoConsumerCalls(content, filePath)...)
	return out
}

// ---------------------------------------------------------------------------
// PRODUCER
// ---------------------------------------------------------------------------

var (
	// mux: `sub := router.PathPrefix("/api/v1").Subrouter()`  (also `=`, chained parent)
	reSubrouter = regexp.MustCompile(`(?m)(\w+)\s*:?=\s*(\w+)\.PathPrefix\(\s*` + "[\"`]" + `([^"` + "`]+)" + "[\"`]" + `\s*\)\.Subrouter\(\)`)
	// mux: `r := mux.NewRouter()`
	reNewRouter = regexp.MustCompile(`(?m)(\w+)\s*:?=\s*mux\.NewRouter\(\)`)
	// mux/net-http: `r.HandleFunc("/path", h.Handler).Methods("GET")`  (Methods optional)
	reHandleFunc = regexp.MustCompile(`(\w+)\.HandleFunc\(\s*` + "[\"`]" + `([^"` + "`]+)" + "[\"`]" + `\s*,\s*([\w.]+)\s*\)(?:\.Methods\(\s*` + "[\"`]" + `(\w+)` + "[\"`]" + `)?`)
	// chi/gin/echo: `r.Get("/path", h.Handler)` / `e.GET("/path", h.Handler)`
	reMethodCall = regexp.MustCompile(`(\w+)\.(Get|Post|Put|Delete|Patch|Head|Options|GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\(\s*` + "[\"`]" + `([^"` + "`]+)" + "[\"`]" + `\s*,\s*([\w.]+)`)
)

// extractGoProducerRoutes scans one Go file for route registrations, resolving
// gorilla/mux subrouter prefixes so a route on a prefixed subrouter gets its
// full path. The produced RawRoute paths are NOT yet normalized — Resolve
// normalizes them to {param} form.
func extractGoProducerRoutes(content, file string) []RawRoute {
	prefix := buildPrefixMap(content, "")
	var out []RawRoute

	for _, m := range reHandleFunc.FindAllStringSubmatch(content, -1) {
		routerVar, path, handlerExpr, method := m[1], m[2], m[3], m[4]
		full := joinRoute(prefixOr(prefix, routerVar, ""), path)
		mt := strings.ToUpper(method)
		if mt == "" {
			mt = "ANY"
		}
		out = append(out, RawRoute{
			Method:      mt,
			Path:        full,
			HandlerName: lastSegment(handlerExpr),
			File:        file,
			Role:        RoleProducer,
		})
	}
	for _, m := range reMethodCall.FindAllStringSubmatch(content, -1) {
		routerVar, method, path, handlerExpr := m[1], m[2], m[3], m[4]
		full := joinRoute(prefixOr(prefix, routerVar, ""), path)
		out = append(out, RawRoute{
			Method:      strings.ToUpper(method),
			Path:        full,
			HandlerName: lastSegment(handlerExpr),
			File:        file,
			Role:        RoleProducer,
		})
	}
	return out
}

func prefixOr(m map[string]string, v, seed string) string {
	if p, ok := m[v]; ok {
		return p
	}
	return seed
}

// buildPrefixMap resolves each router/subrouter variable to its full path prefix
// by chaining PathPrefix(...).Subrouter() declarations back to a local
// mux.NewRouter() (empty prefix). A variable that is neither a local root nor a
// tracked subrouter resolves to seed (empty here, since Atlas has no cross-file
// entry prefix).
func buildPrefixMap(content, seed string) map[string]string {
	type sub struct{ parent, prefix string }
	subs := map[string]sub{}
	roots := map[string]bool{}
	for _, m := range reNewRouter.FindAllStringSubmatch(content, -1) {
		roots[m[1]] = true
	}
	for _, m := range reSubrouter.FindAllStringSubmatch(content, -1) {
		subs[m[1]] = sub{parent: m[2], prefix: m[3]}
	}
	resolved := map[string]string{}
	for v := range roots {
		resolved[v] = ""
	}
	var resolve func(v string, depth int) string
	resolve = func(v string, depth int) string {
		if p, ok := resolved[v]; ok {
			return p
		}
		if depth > 12 {
			return seed
		}
		s, ok := subs[v]
		if !ok {
			return seed
		}
		full := joinRoute(resolve(s.parent, depth+1), s.prefix)
		resolved[v] = full
		return full
	}
	for v := range subs {
		resolve(v, 0)
	}
	return resolved
}

func joinRoute(prefix, path string) string {
	prefix = strings.TrimRight(prefix, "/")
	if path == "" {
		return prefix
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return prefix + path
}

func lastSegment(expr string) string {
	if i := strings.LastIndex(expr, "."); i >= 0 {
		return expr[i+1:]
	}
	return expr
}

// ---------------------------------------------------------------------------
// CONSUMER
// ---------------------------------------------------------------------------

var (
	// http.Get/Post/Put/Delete/Patch/Head( — the call TRIGGER; the verb is the method.
	reHTTPVerb = regexp.MustCompile(`(?i)\bhttp\.(Get|Post|Put|Delete|Patch|Head)\s*\(`)
	// http.NewRequest(...) / http.NewRequestWithContext(...) — the URL is an arg.
	reHTTPNewReq = regexp.MustCompile(`(?i)\bhttp\.NewRequest(?:WithContext)?\s*\(`)
	// the method string inside a NewRequest call, when present.
	reNewReqMethod = regexp.MustCompile(`(?i)\bhttp\.NewRequest(?:WithContext)?\s*\(\s*(?:[\w.]+\s*,\s*)?` + "[\"`]" + `(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)` + "[\"`]")
	// resty / generic builder `.Get("http://…")` — only a trigger when an ABSOLUTE
	// URL literal is also present (avoids matching unrelated `.Get("name")`).
	reBuilderVerb = regexp.MustCompile(`(?i)\.(Get|Post|Put|Delete|Patch|Head)\s*\(`)
	// client.Do(req) — outbound edge with unknown method/path; records the calling file.
	reClientDo = regexp.MustCompile(`(?i)\b\w+(?:\.\w+)*\.Do\s*\(`)
	// the FIRST URL-ish string literal on a line: an absolute URL or an absolute
	// path. Crucially this matches the literal even when it is wrapped in
	// fmt.Sprintf(...) or string concatenation (the verifier-found gap).
	reURLLiteral = regexp.MustCompile("[\"`]" + `((?:https?://|/)[^"` + "`]*)" + "[\"`]")
	// an ABSOLUTE URL literal (scheme + host), for the builder-verb path.
	reAbsURLLiteral = regexp.MustCompile("[\"`]" + `(https?://[^"` + "`]*)" + "[\"`]")
	// enclosing func/method declaration, for consumer attribution.
	reFuncDecl = regexp.MustCompile(`(?m)^\s*func\s+(?:\([^)]*\)\s*)?(\w+)\s*\(`)
)

func firstSubmatch(re *regexp.Regexp, line string) string {
	if m := re.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

// extractGoConsumerCalls scans one Go file for outbound HTTP calls. Each call
// becomes a consumer RawRoute carrying the method, the called endpoint PATH
// (host stripped), the calling file, the enclosing func/method, and the raw URL.
//
// The URL is pulled from the FIRST url-ish string literal on the triggering line,
// so it survives fmt.Sprintf / string-concat wrapping (e.g.
// http.Get(fmt.Sprintf("http://svc/api/v1/users/%s", id))).
func extractGoConsumerCalls(content, file string) []RawRoute {
	lines := strings.Split(content, "\n")
	var out []RawRoute
	seen := map[string]bool{}

	emit := func(method, rawURL string) {
		path := extractPath(rawURL)
		if path == "" {
			return
		}
		method = strings.ToUpper(method)
		key := method + " " + rawURL
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, RawRoute{Method: method, Path: path, File: file, Role: RoleConsumer, RawURL: rawURL})
	}

	for _, line := range lines {
		// 1) http.Verb(...) — verb is the method, URL is the first url literal anywhere on the line.
		if v := firstSubmatch(reHTTPVerb, line); v != "" {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(v, u)
				continue
			}
		}
		// 2) http.NewRequest(...) — method from the string arg (if any), URL is the first url literal.
		if reHTTPNewReq.MatchString(line) {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(firstSubmatch(reNewReqMethod, line), u)
				continue
			}
		}
		// 3) resty/builder .Verb("http://…") — require an absolute URL to avoid false positives.
		if v := firstSubmatch(reBuilderVerb, line); v != "" {
			if u := firstSubmatch(reAbsURLLiteral, line); u != "" {
				emit(v, u)
				continue
			}
		}
		// 4) bare client.Do(...) — no inline URL, but still an outbound edge from this file.
		if reClientDo.MatchString(line) {
			out = append(out, RawRoute{File: file, Role: RoleConsumer, RawURL: strings.TrimSpace(line)})
		}
	}

	attributeConsumerSymbols(out, content, file)
	return filterConsumerNoise(out)
}

// attributeConsumerSymbols sets each consumer RawRoute's CallingSymbol to the
// nearest preceding `func` declaration. It re-locates each call by its RawURL
// (or the trimmed Do(...) line) within the source and scans backwards for the
// enclosing func. Best-effort and order-stable.
func attributeConsumerSymbols(raws []RawRoute, content, file string) {
	// Precompute func-decl offsets in source order.
	type fn struct {
		offset int
		name   string
	}
	var fns []fn
	for _, loc := range reFuncDecl.FindAllStringSubmatchIndex(content, -1) {
		name := content[loc[2]:loc[3]]
		fns = append(fns, fn{offset: loc[0], name: name})
	}
	enclosing := func(at int) string {
		name := ""
		for _, f := range fns {
			if f.offset <= at {
				name = f.name
			} else {
				break
			}
		}
		return name
	}

	searchFrom := 0
	for i := range raws {
		if raws[i].File != file {
			continue
		}
		needle := raws[i].RawURL
		if needle == "" {
			continue
		}
		idx := strings.Index(content[searchFrom:], needle)
		at := 0
		if idx >= 0 {
			at = searchFrom + idx
			searchFrom = at + 1
		} else if idx = strings.Index(content, needle); idx >= 0 {
			at = idx
		}
		raws[i].CallingSymbol = enclosing(at)
	}
}

// filterConsumerNoise drops bare Do(...) edges that we could neither give a path
// nor attribute to an enclosing symbol — they carry no usable signal.
func filterConsumerNoise(raws []RawRoute) []RawRoute {
	out := raws[:0]
	for _, r := range raws {
		if r.Path == "" && r.CallingSymbol == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// extractPath strips scheme + host from a URL, leaving the path (and anything
// after it). A relative URL (already a path) is returned as-is. Env/format
// placeholders left in the host (e.g. "%s/api/...") are tolerated: the first "/"
// after the host marks the path.
func extractPath(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if strings.HasPrefix(u, "/") {
		return u // already a relative path
	}
	if idx := strings.Index(u, "/"); idx >= 0 {
		return u[idx:]
	}
	// No path component at all (bare host) — not a useful endpoint.
	return ""
}
