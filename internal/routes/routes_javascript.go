package routes

import (
	"regexp"
	"strings"
)

// jsRoutes extracts producer routes + consumer calls from a JavaScript/
// TypeScript source file.
//
// PRODUCER frameworks:
//   - Express / Fastify / Koa / generic router: app.get("/path", handler),
//     app.post / put / delete / patch, router.get(...), fastify.get("/path", ...).
//     Method = the verb; Path = the string literal; HandlerName = the 2nd-arg
//     identifier when it is a bare/named function reference (e.g. getUser), else "".
//   - NestJS decorators: @Get("/path") / @Post("/:id") above a method, with a
//     class-level @Controller("/base") prefix combined in when present.
//
// CONSUMER (HTTP-client patterns adapted from the original route analyzer
// cross_service_analyzer.go JS branch + extended): axios.get/post/put/delete/
// patch(url), axios({ method, url }), axios(url), fetch(url, {method}), got(url),
// superagent.get(url), http(s).request({...}). RawURL is the first url-ish literal
// on the line (REUSE reURLLiteral via firstSubmatch); the path keeps any ${id}
// template placeholders (the matcher treats them as wildcards). Each call is
// attributed to the nearest preceding function declaration.
func jsRoutes(filePath, content string) []RawRoute {
	out := extractJSProducerRoutes(content, filePath)
	out = append(out, extractJSConsumerCalls(content, filePath)...)
	return out
}

// ---------------------------------------------------------------------------
// PRODUCER
// ---------------------------------------------------------------------------

var (
	// Express/Fastify/Koa/router method registration:
	//   app.get("/path", getUser)  /  router.post('/path', ...)  /  fastify.get(`/p`, h)
	// Captures: 1=verb, 2=path literal, 3=rest-of-args (to mine a bare handler name).
	reJSAppRoute = regexp.MustCompile(`(?i)\b\w+\.(get|post|put|delete|patch|head|options|all)\s*\(\s*` + "[\"'`]" + `([^"'` + "`]+)" + "[\"'`]" + `\s*(,[^\n]*)?`)
	// A bare/named function reference passed as the (last) handler arg, e.g.
	//   , getUser);   ,  ctrl.getUser )   , [auth, getUser])
	// We grab the LAST dotted identifier before the route call's closing paren,
	// tolerating a trailing `)`, `]`, `;`, or whitespace; `, h.getUser)` ->
	// "getUser" via lastSegment.
	reJSHandlerArg = regexp.MustCompile(`([A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*)\s*[)\]; ]*$`)

	// NestJS class-level prefix: @Controller("/users") / @Controller('users')
	// (also bare @Controller() -> no prefix).
	reNestController = regexp.MustCompile(`(?m)^\s*@Controller\s*\(\s*(?:` + "[\"'`]" + `([^"'` + "`]*)" + "[\"'`]" + `)?\s*\)`)
	// NestJS method decorator: @Get("/path") / @Post('/:id') / @Patch() (no arg).
	// Captures: 1=verb, 2=path literal (may be empty / absent).
	reNestMethod = regexp.MustCompile(`(?m)^\s*@(Get|Post|Put|Delete|Patch|Head|Options|All)\s*\(\s*(?:` + "[\"'`]" + `([^"'` + "`]*)" + "[\"'`]" + `)?\s*\)`)
	// The method name on the line FOLLOWING a Nest decorator (used as HandlerName).
	//   async getUser(@Param('id') id: string) {  ->  getUser
	reNestMethodName = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|async\s+|static\s+)*([A-Za-z_$][\w$]*)\s*\(`)
)

// extractJSProducerRoutes scans one JS/TS file for served routes from both the
// Express-family (app/router.<verb>(...)) and NestJS decorator styles. Paths are
// returned RAW (Resolve normalizes :id -> {id}); HandlerName is best-effort.
func extractJSProducerRoutes(content, file string) []RawRoute {
	var out []RawRoute

	// --- Express / Fastify / Koa / router method registrations ---
	for _, m := range reJSAppRoute.FindAllStringSubmatch(content, -1) {
		verb, path, rest := m[1], m[2], m[3]
		// The same `\w+.<verb>("...")` shape also matches HTTP-CLIENT calls such as
		// axios.get(`http://svc/api/...`). A *served* route path is a relative path
		// (a leading "/"), never an absolute URL and never a `${...}` template
		// interpolation — those are consumer call URLs, so skip them here (the
		// consumer pass captures them instead).
		if !isJSServedRoutePath(path) {
			continue
		}
		out = append(out, RawRoute{
			Method:      strings.ToUpper(verb),
			Path:        path,
			HandlerName: jsHandlerName(rest),
			File:        file,
			Role:        RoleProducer,
		})
	}

	// --- NestJS decorators (controller prefix + method decorator) ---
	out = append(out, extractNestRoutes(content, file)...)

	return out
}

// isJSServedRoutePath reports whether a string literal passed to a
// `x.<verb>("...")` call is a SERVED route path (Express/router registration)
// rather than an outbound HTTP-client URL. A served path is relative — it starts
// with "/" — and carries no scheme or `${...}` template interpolation (both of
// which mark a consumer call like axios.get(`http://svc/...${id}`)).
func isJSServedRoutePath(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.Contains(path, "://") || strings.Contains(path, "${") {
		return false
	}
	return true
}

// jsHandlerName mines a bare/named handler identifier out of the args that
// follow the route path. It returns the LAST dotted identifier before the
// closing paren (so `, h.getUser)` -> "getUser"). An empty/anonymous handler
// (arrow fn, inline function, no further args) yields "".
func jsHandlerName(rest string) string {
	rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), ","))
	if rest == "" {
		return ""
	}
	// Anonymous handlers carry no resolvable symbol.
	if strings.Contains(rest, "=>") || strings.HasPrefix(rest, "function") ||
		strings.Contains(rest, "function(") || strings.Contains(rest, "function (") {
		return ""
	}
	if id := firstSubmatch(reJSHandlerArg, rest); id != "" {
		return lastSegment(id)
	}
	return ""
}

// extractNestRoutes walks the file line-by-line, tracking the most recent
// class-level @Controller("/base") prefix and combining it with each method
// decorator (@Get("/path")). The handler name is the method declared on the
// next non-decorator line.
func extractNestRoutes(content, file string) []RawRoute {
	lines := strings.Split(content, "\n")
	var out []RawRoute
	controllerPrefix := ""

	for i, line := range lines {
		if m := reNestController.FindStringSubmatch(line); m != nil {
			controllerPrefix = m[1]
			continue
		}
		m := reNestMethod.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		verb, path := m[1], m[2]
		full := joinNestPath(controllerPrefix, path)
		out = append(out, RawRoute{
			Method:      strings.ToUpper(verb),
			Path:        full,
			HandlerName: nestMethodNameAfter(lines, i),
			File:        file,
			Role:        RoleProducer,
		})
	}
	return out
}

// nestMethodNameAfter returns the method name declared just below a Nest method
// decorator, skipping intervening param/other decorators and blank lines.
func nestMethodNameAfter(lines []string, decoratorLine int) string {
	for j := decoratorLine + 1; j < len(lines) && j < decoratorLine+8; j++ {
		l := strings.TrimSpace(lines[j])
		if l == "" || strings.HasPrefix(l, "@") {
			continue
		}
		if m := reNestMethodName.FindStringSubmatch(lines[j]); m != nil {
			return m[1]
		}
		return ""
	}
	return ""
}

// joinNestPath combines a controller prefix with a method-decorator path into a
// single served path. Either side may be empty; missing leading slashes are
// added and duplicate slashes collapsed at the seam.
func joinNestPath(prefix, path string) string {
	prefix = strings.TrimSpace(prefix)
	path = strings.TrimSpace(path)
	if prefix != "" && !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	if path == "" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return prefix + path
}

// ---------------------------------------------------------------------------
// CONSUMER
// ---------------------------------------------------------------------------

var (
	// axios.get/post/put/delete/patch( — the verb is the method.
	reJSAxiosVerb = regexp.MustCompile(`(?i)\baxios\.(get|post|put|delete|patch|head|options)\s*\(`)
	// axios(...) / axios({...}) — bare client call (method comes from a `method:` option).
	reJSAxiosBare = regexp.MustCompile(`(?i)\baxios\s*\(`)
	// fetch(url, {...}) — the call trigger; method defaults to GET unless an option says otherwise.
	reJSFetch = regexp.MustCompile(`(?i)\bfetch\s*\(`)
	// got(url) / got.get(url) / got.post(url) — the `got` HTTP client.
	reJSGot = regexp.MustCompile(`(?i)\bgot(?:\.(get|post|put|delete|patch|head))?\s*\(`)
	// superagent.get(url) / request(...).post(url) style builder verbs (require a url literal).
	reJSSuperagentVerb = regexp.MustCompile(`(?i)\b(?:superagent|request)\.(get|post|put|delete|patch|head)\s*\(`)
	// http(s).request({...}) / http(s).get(url) — node core client.
	reJSHTTPModule = regexp.MustCompile(`(?i)\bhttps?\.(request|get)\s*\(`)
	// a `method: "POST"` option (axios config / fetch options).
	reJSMethodOpt = regexp.MustCompile(`(?i)\bmethod\s*:\s*` + "[\"'`]" + `(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)` + "[\"'`]")
	// the FIRST url-ish literal on a line, JS flavor: like the shared reURLLiteral
	// but ALSO accepting single-quoted strings (the JS norm). Matches an absolute
	// URL or an absolute path, surviving template-literal/concat wrapping; ${...}
	// placeholders inside the literal are preserved as path wildcards.
	reJSURLLiteral = regexp.MustCompile("[\"'`]" + `((?:https?://|/)[^"'` + "`]*)" + "[\"'`]")

	// Enclosing-function attribution (best-effort), any of:
	//   function name(..        /  async function name(..
	//   const name = (..  =>    /  let/var name = async (..  =>
	//   name(args) {  (method shorthand / object method)
	reJSFuncDecl = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`)
	reJSConstFn  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>`)
	reJSMethodFn = regexp.MustCompile(`(?m)^\s*(?:public\s+|private\s+|protected\s+|static\s+|async\s+|get\s+|set\s+)*([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*\{`)
)

// extractJSConsumerCalls scans one JS/TS file for outbound HTTP calls. Each call
// becomes a consumer RawRoute carrying the method, the endpoint PATH (host
// stripped, ${...} template placeholders preserved as wildcards), the calling
// file, the enclosing function, and the raw URL.
//
// The URL is pulled from the FIRST url-ish string literal on the triggering line
// (REUSE reURLLiteral), so it survives template-literal / concat wrapping such as
// axios.get(`http://svc/api/v1/users/${id}`).
func extractJSConsumerCalls(content, file string) []RawRoute {
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
		// Path is intentionally left empty per the consumer contract — the
		// resolver matches on RawURL, not Path.
		out = append(out, RawRoute{Method: method, File: file, Role: RoleConsumer, RawURL: rawURL})
	}

	for _, line := range lines {
		// 1) axios.<verb>(url) — verb is the method.
		if v := firstSubmatch(reJSAxiosVerb, line); v != "" {
			if u := jsURLLiteral(line); u != "" {
				emit(v, u)
				continue
			}
		}
		// 2) superagent/request.<verb>(url) — verb is the method.
		if v := firstSubmatch(reJSSuperagentVerb, line); v != "" {
			if u := jsURLLiteral(line); u != "" {
				emit(v, u)
				continue
			}
		}
		// 3) got / got.<verb>(url).
		if loc := reJSGot.FindStringSubmatch(line); loc != nil {
			if u := jsURLLiteral(line); u != "" {
				method := loc[1] // empty -> GET default
				if method == "" {
					method = "GET"
				}
				emit(method, u)
				continue
			}
		}
		// 4) http(s).request(...) / http(s).get(...) — node core client.
		if loc := reJSHTTPModule.FindStringSubmatch(line); loc != nil {
			if u := jsURLLiteral(line); u != "" {
				method := firstSubmatch(reJSMethodOpt, line)
				if method == "" {
					if strings.EqualFold(loc[1], "get") {
						method = "GET"
					}
				}
				emit(method, u)
				continue
			}
		}
		// 5) fetch(url, {method}) — method from options if present, else GET.
		if reJSFetch.MatchString(line) {
			if u := jsURLLiteral(line); u != "" {
				method := firstSubmatch(reJSMethodOpt, line)
				if method == "" {
					method = "GET"
				}
				emit(method, u)
				continue
			}
		}
		// 6) axios(url) / axios({ method, url }) — bare client; method from `method:` option, else GET.
		if reJSAxiosBare.MatchString(line) {
			if u := jsURLLiteral(line); u != "" {
				method := firstSubmatch(reJSMethodOpt, line)
				if method == "" {
					method = "GET"
				}
				emit(method, u)
				continue
			}
		}
	}

	attributeJSConsumerSymbols(out, content, file)
	return out
}

// jsURLLiteral pulls the first url-ish literal off a line. It reuses the shared
// reURLLiteral (double-quote / backtick) first — the common axios/fetch template
// and double-quoted forms — and falls back to the JS single-quote-aware variant
// so `got('http://svc/...')` style calls are not missed.
func jsURLLiteral(line string) string {
	if u := firstSubmatch(reURLLiteral, line); u != "" {
		return u
	}
	return firstSubmatch(reJSURLLiteral, line)
}

// attributeJSConsumerSymbols sets each consumer RawRoute's CallingSymbol to the
// nearest preceding function declaration (function/const-arrow/method-shorthand).
// It re-locates each call by its RawURL within the source and scans backwards for
// the enclosing function. Best-effort and order-stable.
func attributeJSConsumerSymbols(raws []RawRoute, content, file string) {
	type fn struct {
		offset int
		name   string
	}
	var fns []fn
	for _, re := range []*regexp.Regexp{reJSFuncDecl, reJSConstFn, reJSMethodFn} {
		for _, loc := range re.FindAllStringSubmatchIndex(content, -1) {
			fns = append(fns, fn{offset: loc[0], name: content[loc[2]:loc[3]]})
		}
	}
	// Order by source offset so "nearest preceding" is a simple scan.
	for i := 1; i < len(fns); i++ {
		for j := i; j > 0 && fns[j].offset < fns[j-1].offset; j-- {
			fns[j], fns[j-1] = fns[j-1], fns[j]
		}
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
		at := 0
		if idx := strings.Index(content[searchFrom:], needle); idx >= 0 {
			at = searchFrom + idx
			searchFrom = at + 1
		} else if idx := strings.Index(content, needle); idx >= 0 {
			at = idx
		}
		raws[i].CallingSymbol = enclosing(at)
	}
}
