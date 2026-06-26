package routes

import (
	"regexp"
	"strings"
)

// pythonRoutes is the Python-language extractor: PRODUCER route registrations +
// CONSUMER outbound HTTP calls from one file's content.
//
// PRODUCER frameworks:
//   - Flask/FastAPI decorators above a def:
//     @app.route("/path", methods=["GET","POST"]) — one producer per method,
//     default GET when methods= is absent.
//     @app.get("/path") / @app.post(...) / @router.get("/path") /
//     @blueprint.route(...) — method from the decorator verb. HandlerName is the
//     def name on the following (non-decorator) line.
//   - Django: path("route/", view) / re_path(r"^route/$", view) / url(...).
//     Path = the route string (regex anchors stripped, <int:id>/<id> -> {id});
//     HandlerName = the view reference (last dotted segment).
//
// CONSUMER (adapted from the original cross-service analyzer, Python patterns):
// requests.get/post/put/delete/patch(url), requests.request(method, url),
// httpx.get(...) / httpx.AsyncClient().get(...), aiohttp session.get(url),
// urllib.request.urlopen(url). Method from the verb (or the request() method
// arg); RawURL from the first url-ish literal on the line (reURLLiteral handles
// f-strings like f"http://svc/api/v1/users/{id}"). Each call is attributed to the
// nearest preceding def.
func pythonRoutes(filePath, content string) []RawRoute {
	out := extractPythonProducerRoutes(content, filePath)
	out = append(out, extractPythonConsumerCalls(content, filePath)...)
	return out
}

// ---------------------------------------------------------------------------
// PRODUCER
// ---------------------------------------------------------------------------

var (
	// Flask/FastAPI verb decorator: `@app.get("/path")` / `@router.post("/path")`
	// / `@blueprint.delete('/path')`. Group 1 = verb, group 2 = path.
	rePyVerbDecorator = regexp.MustCompile(`(?m)^\s*@\s*[\w.]+\.(get|post|put|delete|patch|head|options)\s*\(\s*` + "[\"'`]" + `([^"'` + "`]+)" + "[\"'`]")
	// Flask/blueprint generic route decorator: `@app.route("/path", methods=[...])`
	// / `@bp.route('/path')`. Group 1 = path. The methods= list (if any) is parsed
	// separately from the same matched line.
	rePyRouteDecorator = regexp.MustCompile(`(?m)^\s*@\s*[\w.]+\.route\s*\(\s*` + "[\"'`]" + `([^"'` + "`]+)" + "[\"'`]")
	// the methods=[...] keyword arg on a @route decorator line.
	rePyMethodsKwarg = regexp.MustCompile(`(?i)methods\s*=\s*[\[(]([^\])]*)[\])]`)
	// each quoted verb inside a methods=[...] list.
	rePyMethodToken = regexp.MustCompile(`(?i)` + "[\"'`]" + `(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)` + "[\"'`]")
	// `def handler_name(` — the function a decorator sits above.
	rePyDef = regexp.MustCompile(`^\s*(?:async\s+)?def\s+(\w+)\s*\(`)
	// a line that is itself a decorator (so we can skip stacked decorators when
	// looking for the def below a route decorator).
	rePyDecoratorLine = regexp.MustCompile(`^\s*@`)
	// Django URLConf entries: `path("route/", view ...)` / `re_path(r"^route/$", view)`
	// / `url(r"...", view)`. Group 1 = route string, group 2 = the view reference
	// (best-effort: a dotted identifier; views.foo / foo / module.foo).
	rePyDjangoRoute = regexp.MustCompile(`(?m)\b(?:re_path|path|url)\s*\(\s*r?` + "[\"'`]" + `([^"'` + "`]*)" + "[\"'`]" + `\s*,\s*([\w.]+)`)
)

// extractPythonProducerRoutes scans one Python file for Flask/FastAPI decorators
// and Django URLConf entries. Decorator paths are emitted with the def name on
// the following line as the handler; Django routes use the view reference. Paths
// are NOT normalized to {param} here beyond Django's <conv:name> rewrite — Resolve
// handles the common :id -> {id} pass (Python already uses {id}, so it is a no-op
// for FastAPI-style routes).
func extractPythonProducerRoutes(content, file string) []RawRoute {
	lines := strings.Split(content, "\n")
	var out []RawRoute

	// defNameAfter returns the `def` name on the first non-decorator,
	// non-blank line at or after index i+1 (i is the decorator's line index).
	defNameAfter := func(i int) string {
		for j := i + 1; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" {
				continue
			}
			if rePyDecoratorLine.MatchString(l) {
				continue // stacked decorator; keep scanning down
			}
			if m := rePyDef.FindStringSubmatch(l); m != nil {
				return m[1]
			}
			return "" // first real statement isn't a def — not a handler decorator
		}
		return ""
	}

	for i, line := range lines {
		// 1) @app.get("/path") / @router.post("/path") etc.
		if m := rePyVerbDecorator.FindStringSubmatch(line); m != nil {
			out = append(out, RawRoute{
				Method:      strings.ToUpper(m[1]),
				Path:        m[2],
				HandlerName: defNameAfter(i),
				File:        file,
				Role:        RoleProducer,
			})
			continue
		}
		// 2) @app.route("/path", methods=["GET","POST"]) — one producer per method.
		if m := rePyRouteDecorator.FindStringSubmatch(line); m != nil {
			handler := defNameAfter(i)
			for _, mt := range pythonRouteMethods(line) {
				out = append(out, RawRoute{
					Method:      mt,
					Path:        m[1],
					HandlerName: handler,
					File:        file,
					Role:        RoleProducer,
				})
			}
			continue
		}
		// 3) Django path("route/", view) / re_path(r"^route/$", view) / url(...).
		if m := rePyDjangoRoute.FindStringSubmatch(line); m != nil {
			out = append(out, RawRoute{
				Method:      "ANY",
				Path:        normalizeDjangoRoute(m[1]),
				HandlerName: lastSegment(m[2]),
				File:        file,
				Role:        RoleProducer,
			})
			continue
		}
	}
	return out
}

// pythonRouteMethods returns the HTTP methods declared on a @route decorator
// line: the verbs in methods=[...] when present, else a single default GET (Flask
// defaults to GET when methods= is omitted).
func pythonRouteMethods(line string) []string {
	kw := rePyMethodsKwarg.FindStringSubmatch(line)
	if kw == nil {
		return []string{"GET"}
	}
	var methods []string
	seen := map[string]bool{}
	for _, tok := range rePyMethodToken.FindAllStringSubmatch(kw[1], -1) {
		m := strings.ToUpper(tok[1])
		if !seen[m] {
			seen[m] = true
			methods = append(methods, m)
		}
	}
	if len(methods) == 0 {
		return []string{"GET"}
	}
	return methods
}

// normalizeDjangoRoute strips Django regex anchors and converts both Django path
// converters (<int:id>, <id>) and named regex groups ((?P<id>...)) to the {id}
// form. A leading slash is added so the path is absolute and comparable to other
// frameworks' served paths.
func normalizeDjangoRoute(route string) string {
	r := route
	r = strings.TrimPrefix(r, "^")
	r = strings.TrimSuffix(r, "$")
	// (?P<id>[0-9]+)  ->  {id}  (run BEFORE the converter rewrite so the inner
	// <id> of a named group isn't rewritten first, leaving a stray "(?P{id}...)").
	r = rePyNamedGroup.ReplaceAllString(r, "{$1}")
	// <int:id> / <slug:name> / <id>  ->  {id} / {name}
	r = rePyDjangoConverter.ReplaceAllString(r, "{$1}")
	if r != "" && !strings.HasPrefix(r, "/") {
		r = "/" + r
	}
	return r
}

var (
	// <int:id> / <slug:name> / <id> — capture the trailing identifier as the param.
	rePyDjangoConverter = regexp.MustCompile(`<(?:\w+:)?(\w+)>`)
	// (?P<name>...) named regex group — capture the name; drop the pattern body up
	// to the matching close paren (greedy-safe for the common simple case).
	rePyNamedGroup = regexp.MustCompile(`\(\?P<(\w+)>[^)]*\)`)
)

// ---------------------------------------------------------------------------
// CONSUMER
// ---------------------------------------------------------------------------

var (
	// requests/httpx/aiohttp/session verb calls: `requests.get(...)`,
	// `httpx.post(...)`, `session.put(...)`, `client.delete(...)`,
	// `httpx.AsyncClient().get(...)`. Group 1 = verb. The receiver is any dotted
	// chain (so AsyncClient() chains and `self.client` work).
	rePyHTTPVerb = regexp.MustCompile(`(?i)\b[\w.]*(?:requests|httpx|session|client|aiohttp)[\w.()]*\.(get|post|put|delete|patch|head|options)\s*\(`)
	// requests.request(method, url) / httpx.request("GET", url) / session.request(...)
	rePyHTTPRequest = regexp.MustCompile(`(?i)\b[\w.]*\.request\s*\(`)
	// the method string arg of a .request(...) call, when a literal.
	rePyRequestMethod = regexp.MustCompile(`(?i)\.request\s*\(\s*` + "[\"'`]" + `(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)` + "[\"'`]")
	// urllib.request.urlopen(url) / urlopen(url) — method unknown (defaults GET-ish).
	rePyURLOpen = regexp.MustCompile(`(?i)\burlopen\s*\(`)
	// enclosing `def name(` for consumer attribution.
	rePyDefAttr = regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+(\w+)\s*\(`)
)

// extractPythonConsumerCalls scans one Python file for outbound HTTP calls. Each
// call becomes a consumer RawRoute carrying the method, the called endpoint PATH
// (host stripped), the calling file, the enclosing def, and the raw URL.
//
// The URL is pulled from the FIRST url-ish string literal on the triggering line
// (reURLLiteral), so it survives f-string wrapping (e.g.
// requests.get(f"http://svc/api/v1/users/{id}")). A line with a verb/urlopen but
// no url-ish literal (URL held in a variable) is skipped — there is no endpoint
// to record.
func extractPythonConsumerCalls(content, file string) []RawRoute {
	lines := strings.Split(content, "\n")
	var out []RawRoute
	seen := map[string]bool{}

	emit := func(method, rawURL string) {
		path := extractPath(rawURL)
		if path == "" {
			return
		}
		method = strings.ToUpper(strings.TrimSpace(method))
		key := method + " " + rawURL
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, RawRoute{Method: method, Path: path, File: file, Role: RoleConsumer, RawURL: rawURL})
	}

	for _, line := range lines {
		// 1) requests/httpx/aiohttp/session .verb(...) — verb is the method.
		if v := firstSubmatch(rePyHTTPVerb, line); v != "" {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(v, u)
				continue
			}
		}
		// 2) .request(method, url) — method from the literal arg (if any).
		if rePyHTTPRequest.MatchString(line) {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(firstSubmatch(rePyRequestMethod, line), u)
				continue
			}
		}
		// 3) urllib.request.urlopen(url) — method unknown.
		if rePyURLOpen.MatchString(line) {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit("", u)
				continue
			}
		}
	}

	attributePythonConsumerSymbols(out, content, file)
	return out
}

// attributePythonConsumerSymbols sets each consumer RawRoute's CallingSymbol to
// the nearest preceding `def` declaration. It re-locates each call by its RawURL
// within the source and scans backwards for the enclosing def. Best-effort and
// order-stable (mirrors the Go attributor).
func attributePythonConsumerSymbols(raws []RawRoute, content, file string) {
	type fn struct {
		offset int
		name   string
	}
	var fns []fn
	for _, loc := range rePyDefAttr.FindAllStringSubmatchIndex(content, -1) {
		fns = append(fns, fn{offset: loc[0], name: content[loc[2]:loc[3]]})
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
		if raws[i].File != file || raws[i].RawURL == "" {
			continue
		}
		needle := raws[i].RawURL
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
