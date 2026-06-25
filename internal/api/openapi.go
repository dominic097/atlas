package api

import (
	"encoding/json"
	"net/http"
)

// route is one entry in the canonical route table that the OpenAPI document is
// generated from. Keeping the table in one place keeps the spec and the router
// honest about each other.
type route struct {
	method  string
	path    string // OpenAPI-style path ({name} templating)
	opID    string
	summary string
}

// routeTable is the authoritative list of /api/v1 operations. handleOpenAPI
// renders it into an OpenAPI 3.1 paths object; the strings here mirror the
// patterns registered in routes().
var routeTable = []route{
	{"get", "/api/v1/status", "getStatus", "Engine and storage status, including the indexed-repo list"},
	{"post", "/api/v1/index", "index", "Index (or re-index) a project path into the code graph"},
	{"get", "/api/v1/search", "search", "Lexical (BM25) symbol search across the indexed graph"},
	{"get", "/api/v1/symbols/{name}", "getSymbol", "Resolve a symbol by name with its definitions, callers, and callees"},
	{"get", "/api/v1/symbols/{name}/callers", "getCallers", "List the resolved callers of a symbol"},
	{"get", "/api/v1/symbols/{name}/refs", "getRefs", "List the call-site references to a symbol"},
	{"get", "/api/v1/symbols/{name}/neighbors", "getNeighbors", "Depth-1 caller/callee neighborhood of a symbol"},
	{"get", "/api/v1/symbols/{name}/explain", "explain", "Deterministic context bundle for a symbol (defs, imports, routes, consumers)"},
	{"post", "/api/v1/impact", "impact", "Reverse blast-radius of changed paths/symbols across the call graph"},
	{"get", "/api/v1/path", "getPath", "Shortest forward call path between two symbols"},
	{"get", "/api/v1/coverage", "getCoverage", "Static call-graph reachability coverage for a symbol or test"},
	{"get", "/api/v1/export", "exportGraph", "Export a symbol neighborhood or the whole graph (json|mermaid|dot)"},
	{"get", "/api/v1/history", "getHistory", "Snapshot history (temporal index) for a repo"},
	{"get", "/api/v1/snapshot-diff", "getSnapshotDiff", "Symbol/edge diff between two snapshots of a repo"},
	{"get", "/api/v1/repos", "listRepos", "List the indexed repositories"},
	{"get", "/api/v1/repos/{repo}/route-contracts", "getRouteContracts", "Producer HTTP route contracts a repo serves"},
	{"get", "/api/v1/repos/{repo}/consumers", "getConsumers", "Cross-repo consumers of a repo's route contracts"},
	{"post", "/api/v1/repos/{repo}/cross-repo-impact", "crossRepoImpact", "Cross-repo blast-radius of a repo's changed handler files"},
}

// handleOpenAPI serves a real OpenAPI 3.1 document generated from routeTable.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	doc := s.openAPISpec()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

// openAPISpec builds the OpenAPI 3.1 document programmatically. It declares the
// info block, the bearer security scheme (active only when a token is set), and
// one path item per route with method, summary, operationId, and path
// parameters inferred from {name}/{repo} templating.
func (s *Server) openAPISpec() map[string]any {
	paths := map[string]any{}
	for _, rt := range routeTable {
		item, _ := paths[rt.path].(map[string]any)
		if item == nil {
			item = map[string]any{}
		}
		op := map[string]any{
			"operationId": rt.opID,
			"summary":     rt.summary,
			"responses": map[string]any{
				"200": map[string]any{
					"description": "Success — result wrapped in a {\"data\": …} envelope",
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"type": "object", "properties": map[string]any{"data": map[string]any{}}},
						},
					},
				},
				"400": problemResponse("Bad request"),
				"401": problemResponse("Unauthorized (set Authorization: Bearer when ATLAS_API_TOKEN is configured)"),
				"404": problemResponse("Repo/snapshot not found, or nothing indexed yet"),
				"500": problemResponse("Internal error"),
			},
		}
		if params := pathParams(rt.path); len(params) > 0 {
			op["parameters"] = params
		}
		item[rt.method] = op
		paths[rt.path] = item
	}

	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Atlas Code-Intelligence API",
			"version":     "1.0.0",
			"description": "Deterministic code-intelligence engine: search, symbol navigation, impact, coverage, temporal diff, and cross-repo route contracts. Every operation is a pure function of the indexed graph.",
		},
		"servers": []any{map[string]any{"url": "/"}},
		"paths":   paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"description":  "Optional. Required on /api/v1/* only when ATLAS_API_TOKEN is set on the server.",
					"bearerFormat": "opaque",
				},
			},
			"schemas": map[string]any{
				"Problem": map[string]any{
					"type":        "object",
					"description": "RFC 9457 problem detail.",
					"properties": map[string]any{
						"type":   map[string]any{"type": "string"},
						"title":  map[string]any{"type": "string"},
						"status": map[string]any{"type": "integer"},
						"code":   map[string]any{"type": "string"},
						"detail": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	if s.token != "" {
		doc["security"] = []any{map[string]any{"bearerAuth": []any{}}}
	}
	return doc
}

// problemResponse is an OpenAPI response object referencing the Problem schema.
func problemResponse(desc string) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/problem+json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Problem"},
			},
		},
	}
}

// pathParams returns OpenAPI parameter objects for each {name} template segment
// in an OpenAPI-style path.
func pathParams(p string) []any {
	var out []any
	seg := ""
	in := false
	for _, c := range p {
		switch {
		case c == '{':
			in, seg = true, ""
		case c == '}':
			if in && seg != "" {
				out = append(out, map[string]any{
					"name":     seg,
					"in":       "path",
					"required": true,
					"schema":   map[string]any{"type": "string"},
				})
			}
			in = false
		case in:
			seg += string(c)
		}
	}
	return out
}
