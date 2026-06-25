// Package mcp implements consumption surface S3: an MCP server exposing Atlas
// graph/search/impact as tools to LLM agents (Claude Code, Cursor, Codex,
// Gemini, Copilot) over stdio (and, in the full build, HTTP + legacy SSE).
//
// It speaks JSON-RPC 2.0. The scaffold implements initialize / tools/list /
// tools/call dispatch with a tool catalog mapping each tool to an Engine op;
// op bodies return the not-implemented sentinel as a (non-error) degrade result
// so agents self-correct rather than abort.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/engine"
)

// Server is the Atlas MCP server.
type Server struct {
	eng   engine.Engine
	tools []Tool
}

// Tool is one advertised MCP tool.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// NewServer builds the server and registers the core tool catalog.
func NewServer(eng engine.Engine) *Server {
	return &Server{eng: eng, tools: coreTools()}
}

// coreTools advertises the tier-agnostic tools. The hosted/cross-scope ops
// (cross_repo_impact, consumers, route_contracts) are added by the full build
// when the backend reports the capability. Atlas exposes deterministic
// intelligence only; agentic tools (rca/fix/review) live in Pulse.
func coreTools() []Tool {
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	str := map[string]any{"type": "string"}
	return []Tool{
		{Name: "search", Description: "Code-aware lexical search over indexed symbols.",
			InputSchema: obj(map[string]any{"query": str, "repo_id": str, "kind": str, "limit": map[string]any{"type": "integer"}}, "query")},
		{Name: "symbol", Description: "Definition(s) of a symbol with its callers and callees.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "callers", Description: "Symbols that directly call a given symbol.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str, "limit": map[string]any{"type": "integer"}}, "symbol")},
		{Name: "neighbors", Description: "Depth-1 call neighborhood of a symbol: its direct callers and callees.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "path", Description: "Shortest forward call path from one symbol (from) to another (to).",
			InputSchema: obj(map[string]any{"from": str, "to": str, "repo_id": str, "max_depth": map[string]any{"type": "integer"}}, "from", "to")},
		{Name: "refs", Description: "Call-site references to a symbol (Atlas has call + import edges; reference edges land later).",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "explain", Description: "Deterministic context bundle for a symbol (no LLM): definitions, callers/callees, imports, served routes, cross-repo consumers.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "coverage", Description: "Static call-graph reachability coverage: tests reaching a symbol, or non-test symbols a test exercises. Not runtime coverage.",
			InputSchema: obj(map[string]any{"target": str, "repo_id": str, "direction": str}, "target")},
		{Name: "impact", Description: "Single-repo blast radius: reverse call-graph BFS from changed paths/symbols.",
			InputSchema: obj(map[string]any{"changed_paths": map[string]any{"type": "array"}, "symbols": map[string]any{"type": "array"}, "max_depth": map[string]any{"type": "integer"}, "repo_id": str})},
		{Name: "graph_export", Description: "Export the call-graph neighborhood around a symbol (json|mermaid|dot).",
			InputSchema: obj(map[string]any{"symbol": str, "depth": map[string]any{"type": "integer"}, "format": str, "repo_id": str}, "symbol")},
		{Name: "history", Description: "Per-commit snapshot timeline for a repo (temporal).",
			InputSchema: obj(map[string]any{"repo_id": str, "limit": map[string]any{"type": "integer"}})},
		{Name: "snapshot_diff", Description: "Structural diff between two snapshots: symbols/edges added/removed/modified.",
			InputSchema: obj(map[string]any{"from": str, "to": str, "repo_id": str})},
		{Name: "route_contracts", Description: "Producer HTTP routes a repo serves (its public contract: method/path/handler).",
			InputSchema: obj(map[string]any{"repo": str})},
		{Name: "consumers", Description: "Other repos that call any route this repo serves (cross-repo dependents).",
			InputSchema: obj(map[string]any{"repo": str})},
		{Name: "cross_repo_impact", Description: "Cross-repo blast radius (the USP): which OTHER repos call routes that the changed handler files serve.",
			InputSchema: obj(map[string]any{"repo": str, "changed_paths": map[string]any{"type": "array"}})},
		{Name: "status", Description: "Engine health and per-repo index freshness.",
			InputSchema: obj(map[string]any{"repo_id": str})},
	}
}

// ── JSON-RPC framing ────────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeStdio reads line-delimited JSON-RPC from r and writes responses to w.
// IMPORTANT: in stdio mode, stdout is the protocol channel — all logging must go
// to stderr (the full server enforces this).
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var req rpcRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = enc.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			continue // notification: no response
		}
		_ = enc.Encode(s.dispatch(ctx, &req))
	}
	return sc.Err()
}

func (s *Server) dispatch(ctx context.Context, req *rpcRequest) rpcResponse {
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo":      map[string]any{"name": "atlas", "version": "scaffold"},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": s.tools}
	case "tools/call":
		resp.Result = s.callTool(ctx, req.Params)
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

// callTool routes a tools/call to the Engine. On a not-implemented op it returns
// a degrade result (isError:false) so the agent reads the hint and recovers.
func (s *Server) callTool(ctx context.Context, params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(params, &p)

	args := map[string]any{}
	if len(p.Arguments) > 0 {
		_ = json.Unmarshal(p.Arguments, &args)
	}
	str := func(k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	strs := func(k string) []string {
		raw, ok := args[k].([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	intOr := func(k string, d int) int {
		if v, ok := args[k].(float64); ok {
			return int(v)
		}
		return d
	}

	var (
		payload any
		err     error
	)
	switch p.Name {
	case "search":
		payload, err = s.eng.Search(ctx, engine.SearchInput{
			Query: str("query"), RepoID: str("repo_id"), Kind: str("kind"),
			Limit: intOr("limit", 20), Mode: "lexical",
		})
	case "impact":
		payload, err = s.eng.Impact(ctx, engine.ImpactInput{
			ChangedPaths: strs("changed_paths"), Symbols: strs("symbols"),
			RepoID: str("repo_id"), MaxDepth: intOr("max_depth", 3), IncludeTests: true,
		})
	case "callers":
		payload, err = s.eng.Callers(ctx, engine.CallersInput{Name: str("symbol"), RepoID: str("repo_id"), Limit: intOr("limit", 50)})
	case "symbol":
		payload, err = s.eng.Symbol(ctx, engine.SymbolInput{Name: str("symbol"), RepoID: str("repo_id")})
	case "neighbors":
		payload, err = s.eng.Neighbors(ctx, engine.NeighborsInput{Name: str("symbol"), RepoID: str("repo_id")})
	case "path":
		payload, err = s.eng.Path(ctx, engine.PathInput{From: str("from"), To: str("to"), RepoID: str("repo_id"), MaxDepth: intOr("max_depth", 6)})
	case "refs":
		payload, err = s.eng.Refs(ctx, engine.RefsInput{Name: str("symbol"), RepoID: str("repo_id")})
	case "explain":
		payload, err = s.eng.Explain(ctx, engine.ExplainInput{Name: str("symbol"), RepoID: str("repo_id")})
	case "coverage":
		payload, err = s.eng.Coverage(ctx, engine.CoverageInput{Target: str("target"), RepoID: str("repo_id"), Direction: str("direction")})
	case "graph_export":
		f := str("format")
		if f == "" {
			f = "mermaid"
		}
		payload, err = s.eng.GraphExport(ctx, engine.GraphExportInput{Symbol: str("symbol"), RepoID: str("repo_id"), Depth: intOr("depth", 2), Format: f, MaxNodes: 200})
	case "history":
		payload, err = s.eng.History(ctx, engine.HistoryInput{RepoID: str("repo_id"), Limit: intOr("limit", 50)})
	case "snapshot_diff":
		payload, err = s.eng.SnapshotDiff(ctx, engine.SnapshotDiffInput{From: str("from"), To: str("to"), RepoID: str("repo_id")})
	case "route_contracts":
		payload, err = s.eng.RouteContracts(ctx, engine.RouteContractsInput{Repo: str("repo")})
	case "consumers":
		payload, err = s.eng.Consumers(ctx, engine.ConsumersInput{Repo: str("repo")})
	case "cross_repo_impact":
		payload, err = s.eng.CrossRepoImpact(ctx, engine.CrossRepoImpactInput{Repo: str("repo"), ChangedPaths: strs("changed_paths")})
	case "status":
		payload, err = s.eng.Status(ctx, engine.StatusInput{RepoID: str("repo_id")})
	default:
		err = engine.ErrNotImplemented
	}

	text := mustJSON(payload)
	if err != nil {
		text = mustJSON(map[string]any{"status": "not_implemented", "hint": err.Error()})
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": false,
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
