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
			InputSchema: obj(map[string]any{"query": str, "repo_id": str, "kind": str}, "query")},
		{Name: "symbol", Description: "Full context bundle for one symbol.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "impact", Description: "Single-repo blast radius for a change.",
			InputSchema: obj(map[string]any{"changed_paths": map[string]any{"type": "array"}, "repo_id": str})},
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

	var (
		payload any
		err     error
	)
	switch p.Name {
	case "search":
		payload, err = s.eng.Search(ctx, engine.SearchInput{Mode: "lexical"})
	case "impact":
		payload, err = s.eng.Impact(ctx, engine.ImpactInput{MaxDepth: 3, IncludeTests: true})
	case "status":
		payload, err = s.eng.Status(ctx, engine.StatusInput{})
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
