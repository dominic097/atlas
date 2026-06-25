package routes

import (
	"regexp"
	"strings"
)

// javaRoutes is the Java-language extractor: PRODUCER route declarations +
// CONSUMER outbound HTTP calls from one file's content.
//
// PRODUCER frameworks:
//   - Spring MVC annotations: @GetMapping/@PostMapping/@PutMapping/
//     @DeleteMapping/@PatchMapping (the verb IS the method) and the generic
//     @RequestMapping(value="/path", method=RequestMethod.GET) /
//     @RequestMapping("/path") (method from `method=`, else "ANY"). A class-level
//     @RequestMapping("/base") prefix is concatenated onto each method-level
//     mapping path. HandlerName = the method name on the signature line that
//     follows the annotation (public ... name(...)).
//   - JAX-RS: class @Path("/base") + method @Path("/sub") + @GET/@POST/... .
//
// CONSUMER (per-language patterns aligned with aziron-pulse
// cross_service_analyzer.go): Spring RestTemplate (getForObject/getForEntity/
// postForObject/.../exchange), WebClient (.get()/.post()....uri(url)), OkHttp
// (Request.Builder().url(url)), java.net.http (HttpRequest.newBuilder().uri(...))
// and HttpURLConnection. The method comes from the verb / HttpMethod token; the
// URL is the first url-ish string literal on the line (reURLLiteral, shared with
// routes_go.go), so it survives string concatenation like
// "http://svc/api/v1/users/" + id (the literal prefix is captured). Each call is
// attributed to the nearest preceding method signature.
func javaRoutes(filePath, content string) []RawRoute {
	out := extractJavaProducerRoutes(content, filePath)
	out = append(out, extractJavaConsumerCalls(content, filePath)...)
	return out
}

// ---------------------------------------------------------------------------
// PRODUCER
// ---------------------------------------------------------------------------

var (
	// Class-level @RequestMapping prefix: @RequestMapping("/base") or
	// @RequestMapping(value = "/base", ...). Captured as the controller base path
	// that is prepended to every method mapping in the file.
	reJavaClassRequestMapping = regexp.MustCompile(`@RequestMapping\s*\(\s*(?:value\s*=\s*)?` + "[\"`]" + `([^"` + "`]*)" + "[\"`]")
	// Class-level JAX-RS base: @Path("/base").
	reJavaClassPath = regexp.MustCompile(`@Path\s*\(\s*` + "[\"`]" + `([^"` + "`]*)" + "[\"`]" + `\s*\)`)
	// A `class Foo` declaration — marks where class-level annotations stop applying
	// to method mappings vs. where method-level ones begin.
	reJavaClassDecl = regexp.MustCompile(`(?m)^\s*(?:public\s+|final\s+|abstract\s+)*class\s+\w+`)

	// Spring verb-specific mappings: @GetMapping("/p") / @GetMapping(value="/p").
	// The verb is captured so the HTTP method is known without a method= arg.
	reJavaVerbMapping = regexp.MustCompile(`@(Get|Post|Put|Delete|Patch)Mapping\s*\(\s*(?:value\s*=\s*|path\s*=\s*)?` + "[\"`]" + `([^"` + "`]*)" + "[\"`]")
	// Spring verb mapping with NO path arg: @GetMapping or @GetMapping() — maps the
	// bare class base path.
	reJavaVerbMappingBare = regexp.MustCompile(`@(Get|Post|Put|Delete|Patch)Mapping\s*(?:\(\s*\))?\s*$`)
	// Generic @RequestMapping on a method, with an optional path and optional
	// method=RequestMethod.VERB.
	reJavaMethodRequestMapping = regexp.MustCompile(`@RequestMapping\s*\(`)
	reJavaRequestMappingPath   = regexp.MustCompile(`@RequestMapping\s*\([^)]*?(?:value\s*=\s*|path\s*=\s*)?` + "[\"`]" + `([^"` + "`]*)" + "[\"`]")
	reJavaRequestMappingMethod = regexp.MustCompile(`method\s*=\s*(?:RequestMethod\.)?(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)`)

	// JAX-RS HTTP verb annotation on a method: @GET / @POST / ... .
	reJavaJaxrsVerb = regexp.MustCompile(`(?m)^\s*@(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\b`)
	// JAX-RS method-level sub-path: @Path("/sub").
	reJavaMethodPath = regexp.MustCompile(`@Path\s*\(\s*` + "[\"`]" + `([^"` + "`]*)" + "[\"`]" + `\s*\)`)

	// A method signature line: `public User getUser(...)`. Captures the method
	// name so the producer HandlerName / consumer CallingSymbol can be resolved.
	reJavaMethodSig = regexp.MustCompile(`(?m)^\s*(?:@\w+\s*(?:\([^)]*\))?\s*)*(?:public|protected|private)?\s*(?:static\s+|final\s+|abstract\s+|synchronized\s+|default\s+)*[\w.<>\[\],?\s]+?\s+(\w+)\s*\([^;]*\)\s*(?:throws[\w.,\s]+)?\{?`)
)

// extractJavaProducerRoutes scans one Java file for Spring MVC and JAX-RS route
// declarations. The class-level @RequestMapping / @Path prefix is concatenated
// onto each method-level mapping path; HandlerName is taken from the nearest
// following method signature. Paths are NOT yet normalized — Resolve handles
// that.
func extractJavaProducerRoutes(content, file string) []RawRoute {
	lines := strings.Split(content, "\n")
	base := javaClassBasePath(content)
	var out []RawRoute

	emit := func(method, path string, lineIdx int) {
		full := joinRoute(base, path)
		mt := strings.ToUpper(strings.TrimSpace(method))
		if mt == "" {
			mt = "ANY"
		}
		out = append(out, RawRoute{
			Method:      mt,
			Path:        full,
			HandlerName: javaHandlerNameAfter(lines, lineIdx),
			File:        file,
			Role:        RoleProducer,
		})
	}

	for i, line := range lines {
		// Spring @VerbMapping("/path")
		if m := reJavaVerbMapping.FindStringSubmatch(line); m != nil {
			emit(m[1], m[2], i)
			continue
		}
		// Spring @VerbMapping (no path) -> the class base path.
		if m := reJavaVerbMappingBare.FindStringSubmatch(line); m != nil {
			emit(m[1], "", i)
			continue
		}
		// Spring generic @RequestMapping on a method.
		if reJavaMethodRequestMapping.MatchString(line) && !isJavaClassAnnotationLine(lines, i) {
			path := firstSubmatch(reJavaRequestMappingPath, line)
			method := firstSubmatch(reJavaRequestMappingMethod, line)
			emit(method, path, i)
			continue
		}
		// JAX-RS @GET/@POST/... on a method; the sub-path is the @Path on a nearby
		// line of the same method (this annotation or an adjacent one).
		if m := reJavaJaxrsVerb.FindStringSubmatch(line); m != nil {
			sub := javaJaxrsMethodPath(lines, i)
			emit(m[1], sub, i)
			continue
		}
	}
	return out
}

// javaClassBasePath returns the controller base path: the FIRST class-level
// @RequestMapping(...) or @Path(...) that appears BEFORE the `class` declaration.
// Method-level annotations of the same shape come after the class decl and are
// not mistaken for the base.
func javaClassBasePath(content string) string {
	classAt := len(content)
	if loc := reJavaClassDecl.FindStringIndex(content); loc != nil {
		classAt = loc[0]
	}
	head := content[:classAt]
	if m := reJavaClassRequestMapping.FindStringSubmatch(head); m != nil {
		return m[1]
	}
	if m := reJavaClassPath.FindStringSubmatch(head); m != nil {
		return m[1]
	}
	return ""
}

// isJavaClassAnnotationLine reports whether the @RequestMapping on lines[idx] is
// a CLASS-level annotation (i.e. it sits before the `class` declaration). Such a
// line is the base path, not a route — extractJavaProducerRoutes must not emit a
// route for it.
func isJavaClassAnnotationLine(lines []string, idx int) bool {
	for i := idx; i < len(lines); i++ {
		if reJavaClassDecl.MatchString(lines[i]) {
			return true
		}
		// A method signature before any class decl means this is method-level.
		if reJavaMethodSig.MatchString(lines[i]) {
			return false
		}
	}
	return false
}

// javaHandlerNameAfter finds the method name on the first method-signature line
// at or after lineIdx (skipping intervening annotation lines). Returns "" if none
// is found within a small window.
func javaHandlerNameAfter(lines []string, lineIdx int) string {
	for i := lineIdx; i < len(lines) && i < lineIdx+8; i++ {
		if m := reJavaMethodSig.FindStringSubmatch(lines[i]); m != nil {
			return m[1]
		}
	}
	return ""
}

// javaJaxrsMethodPath finds the @Path("/sub") that belongs to the JAX-RS method
// whose @GET/@POST sits on lines[verbIdx] — scanning a small window around it
// (annotations can precede or follow the verb) up to the method signature. The
// backward scan stops at the `class` declaration so the class-level @Path base
// (same annotation shape) is never mistaken for the method sub-path.
func javaJaxrsMethodPath(lines []string, verbIdx int) string {
	lo := verbIdx - 4
	if lo < 0 {
		lo = 0
	}
	// Don't scan back across the class declaration — the @Path before it is the base.
	for j := verbIdx; j >= lo; j-- {
		if reJavaClassDecl.MatchString(lines[j]) {
			lo = j + 1
			break
		}
	}
	for i := lo; i < len(lines) && i < verbIdx+6; i++ {
		if m := reJavaMethodPath.FindStringSubmatch(lines[i]); m != nil {
			return m[1]
		}
		if i > verbIdx && reJavaMethodSig.MatchString(lines[i]) {
			break
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// CONSUMER
// ---------------------------------------------------------------------------

var (
	// Spring RestTemplate verb methods. The method is derived from the prefix
	// (getForObject -> GET, postForEntity -> POST, ...).
	reJavaRestTemplate = regexp.MustCompile(`(?i)\.(getForObject|getForEntity|postForObject|postForEntity|put|delete|patchForObject)\s*\(`)
	// RestTemplate.exchange(url, HttpMethod.GET, ...) — method from the HttpMethod arg.
	reJavaExchange = regexp.MustCompile(`(?i)\.exchange\s*\(`)
	// WebClient verb: webClient.get().uri(...) / .post().uri(...). The verb chained
	// before .uri/.retrieve is the method.
	reJavaWebClientVerb = regexp.MustCompile(`(?i)\.(get|post|put|delete|patch)\s*\(\s*\)\s*\.(?:uri|retrieve)`)
	// OkHttp / java.net.http builders: .url(url) (OkHttp) or .uri(URI.create(url)) /
	// .uri(url) (java.net.http) — the URL is the literal on the line; method is
	// resolved from a HttpMethod / .method("VERB") / verb token if present, else "".
	reJavaBuilderURL = regexp.MustCompile(`(?i)\.(?:url|uri)\s*\(`)
	// java.net.http HttpRequest.newBuilder().<verb>(...) e.g. .GET() / .POST(...) /
	// .PUT(...) / .DELETE() / .method("PATCH", ...).
	reJavaHttpReqVerb = regexp.MustCompile(`\.(GET|POST|PUT|DELETE|PATCH)\s*\(`)
	// .method("VERB", ...) (java.net.http / generic).
	reJavaMethodCall = regexp.MustCompile(`\.method\s*\(\s*` + "[\"`]" + `(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)` + "[\"`]")
	// HttpMethod.VERB anywhere on the line (RestTemplate.exchange, HttpEntity, ...).
	reJavaHttpMethodEnum = regexp.MustCompile(`HttpMethod\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)`)
	// A bare URL connection open — outbound edge even without an inline verb.
	reJavaOpenConn = regexp.MustCompile(`(?i)\.openConnection\s*\(`)
)

// restTemplateMethod maps a RestTemplate verb-method name to its HTTP verb.
func restTemplateMethod(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "get"):
		return "GET"
	case strings.HasPrefix(n, "post"):
		return "POST"
	case strings.HasPrefix(n, "put"):
		return "PUT"
	case strings.HasPrefix(n, "delete"):
		return "DELETE"
	case strings.HasPrefix(n, "patch"):
		return "PATCH"
	}
	return ""
}

// extractJavaConsumerCalls scans one Java file for outbound HTTP calls across the
// common Java HTTP clients. Each call becomes a consumer RawRoute carrying the
// method (when determinable), the endpoint PATH (host stripped), the calling
// file, the enclosing method, and the raw URL literal. The URL is the first
// url-ish literal on the triggering line so it survives string concatenation
// ("http://svc/api/v1/users/" + id captures "/api/v1/users/").
func extractJavaConsumerCalls(content, file string) []RawRoute {
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
		// 1) RestTemplate getForObject/postForEntity/... — verb from the method name.
		if m := reJavaRestTemplate.FindStringSubmatch(line); m != nil {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(restTemplateMethod(m[1]), u)
				continue
			}
		}
		// 2) RestTemplate.exchange(url, HttpMethod.VERB, ...) — verb from HttpMethod.
		if reJavaExchange.MatchString(line) {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(firstSubmatch(reJavaHttpMethodEnum, line), u)
				continue
			}
		}
		// 3) WebClient .get().uri(url) — verb from the chained method.
		if m := reJavaWebClientVerb.FindStringSubmatch(line); m != nil {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(m[1], u)
				continue
			}
		}
		// 4) OkHttp .url(url) / java.net.http .uri(url) — verb from any verb/method
		//    token on the line (best-effort), else unknown.
		if reJavaBuilderURL.MatchString(line) {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				method := firstSubmatch(reJavaHttpReqVerb, line)
				if method == "" {
					method = firstSubmatch(reJavaMethodCall, line)
				}
				if method == "" {
					method = firstSubmatch(reJavaHttpMethodEnum, line)
				}
				emit(method, u)
				continue
			}
		}
		// 5) HttpURLConnection url.openConnection() — outbound edge; URL literal if any.
		if reJavaOpenConn.MatchString(line) {
			if u := firstSubmatch(reURLLiteral, line); u != "" {
				emit(firstSubmatch(reJavaHttpMethodEnum, line), u)
				continue
			}
		}
	}

	attributeJavaConsumerSymbols(out, content, lines, file)
	return out
}

// attributeJavaConsumerSymbols sets each consumer RawRoute's CallingSymbol to the
// nearest preceding Java method signature. It re-locates each call by its RawURL
// within the source and scans backwards for the enclosing method. Best-effort
// and order-stable (mirrors the Go attributor).
func attributeJavaConsumerSymbols(raws []RawRoute, content string, lines []string, file string) {
	type fn struct {
		offset int
		name   string
	}
	var fns []fn
	for _, loc := range reJavaMethodSig.FindAllStringSubmatchIndex(content, -1) {
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
