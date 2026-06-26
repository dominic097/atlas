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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dominic097/atlas/internal/engine"
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
		{Name: "semantic_search", Description: "Optional vector nearest-neighbor search over indexed symbols. Degrades to lexical (degraded=true, mode_used=lexical) when vectors are off or the snapshot has no embeddings.",
			InputSchema: obj(map[string]any{"query": str, "repo_id": str, "limit": map[string]any{"type": "integer"}, "min_score": map[string]any{"type": "number"}}, "query")},
		{Name: "context", Description: "Bounded code-review context for changed paths: changed symbols, retrieval hits, impact files, and scoped edges.",
			InputSchema: obj(map[string]any{"changed_paths": map[string]any{"type": "array"}, "query": str, "repo_id": str, "limit": map[string]any{"type": "integer"}, "max_files": map[string]any{"type": "integer"}, "max_edges": map[string]any{"type": "integer"}, "max_depth": map[string]any{"type": "integer"}})},
		{Name: "symbol", Description: "Definition(s) of a symbol with its callers and callees.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "callers", Description: "Symbols that directly call a given symbol.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str, "limit": map[string]any{"type": "integer"}}, "symbol")},
		{Name: "neighbors", Description: "Depth-1 call neighborhood of a symbol: its direct callers and callees.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "path", Description: "Shortest forward call path from one symbol (from) to another (to).",
			InputSchema: obj(map[string]any{"from": str, "to": str, "repo_id": str, "max_depth": map[string]any{"type": "integer"}}, "from", "to")},
		{Name: "refs", Description: "Call-site and type-use references to a symbol (resolved callers over call edges, unioned with symbols that name it as a type).",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "explain", Description: "Deterministic context bundle for a symbol (no LLM): definitions, callers/callees, imports, served routes, cross-repo consumers.",
			InputSchema: obj(map[string]any{"symbol": str, "repo_id": str}, "symbol")},
		{Name: "coverage", Description: "Coverage for a symbol: real RUNTIME coverage (covered/total lines) when a profile was imported, else static call-graph reachability.",
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
		{Name: "communities", Description: "Deterministic graph communities: clusters of densely-connected symbols (label propagation), size-ranked with representative members.",
			InputSchema: obj(map[string]any{"repo_id": str, "limit": map[string]any{"type": "integer"}})},
		{Name: "hubs", Description: "Graph hubs (\"god nodes\"): top symbols by call-graph degree centrality (in/out/total).",
			InputSchema: obj(map[string]any{"repo_id": str, "limit": map[string]any{"type": "integer"}})},
		{Name: "report", Description: "Deterministic graph report: summary stats, top hubs (god nodes), and communities, with a ready-to-render Markdown document.",
			InputSchema: obj(map[string]any{"repo_id": str})},
		{Name: "status", Description: "Engine health and per-repo index freshness.",
			InputSchema: obj(map[string]any{"repo_id": str})},
		{Name: "link", Description: "Register a repo into the graph WITHOUT indexing it (path, git remote URL, or org/name), so it participates in cross-repo and shows in status.",
			InputSchema: obj(map[string]any{"repo": str, "branch": str}, "repo")},
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

// HTTPHandler returns an http.Handler implementing the MCP Streamable HTTP
// transport. A POST carries a single JSON-RPC request object or a batch array in
// the body; the response is the JSON-RPC response object (or array) as
// application/json. Notifications (requests without an id) produce no response:
// a lone notification yields 202 Accepted with no body. GET is answered with 405
// (Atlas does not push server-initiated SSE events on the GET stream).
//
// It reuses s.dispatch — the same catalog and op routing as the stdio transport.
func (s *Server) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.serveHTTPPost(w, r)
		case http.MethodGet:
			// Minimal Streamable HTTP: no standing SSE stream to offer.
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed: open a POST stream", http.StatusMethodNotAllowed)
		default:
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveHTTPPost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
	if err != nil {
		writeRPCError(w, nil, -32700, "parse error: "+err.Error())
		return
	}
	ctx := r.Context()

	// A JSON-RPC batch is a top-level array; a single call is an object.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []rpcRequest
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			writeRPCError(w, nil, -32700, "parse error: "+err.Error())
			return
		}
		responses := make([]rpcResponse, 0, len(batch))
		for i := range batch {
			if len(batch[i].ID) == 0 {
				continue // notification: no response
			}
			responses = append(responses, s.dispatch(ctx, &batch[i]))
		}
		if len(responses) == 0 {
			// Batch of only notifications: nothing to return.
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, http.StatusOK, responses)
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		writeRPCError(w, nil, -32700, "parse error: "+err.Error())
		return
	}
	if len(req.ID) == 0 {
		// Notification: acknowledge with no body.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, http.StatusOK, s.dispatch(ctx, &req))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	writeJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
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
	floatOr := func(k string, d float64) float64 {
		if v, ok := args[k].(float64); ok {
			return v
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
	case "semantic_search":
		payload, err = s.eng.SemanticSearch(ctx, engine.SemanticSearchInput{
			Query: str("query"), RepoID: str("repo_id"),
			Limit: intOr("limit", 20), MinScore: floatOr("min_score", 0),
		})
	case "context":
		payload, err = s.eng.Context(ctx, engine.ContextInput{
			Paths: strs("changed_paths"), Query: str("query"), RepoID: str("repo_id"),
			Limit: intOr("limit", 80), MaxFiles: intOr("max_files", 60), MaxEdges: intOr("max_edges", 500), MaxDepth: intOr("max_depth", 3),
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
	case "communities":
		payload, err = s.eng.Communities(ctx, engine.CommunitiesInput{RepoID: str("repo_id"), Limit: intOr("limit", 20)})
	case "hubs":
		payload, err = s.eng.Hubs(ctx, engine.HubsInput{RepoID: str("repo_id"), Limit: intOr("limit", 20)})
	case "report":
		payload, err = s.eng.Report(ctx, engine.ReportInput{RepoID: str("repo_id")})
	case "status":
		payload, err = s.eng.Status(ctx, engine.StatusInput{RepoID: str("repo_id")})
	case "link":
		payload, err = s.eng.Link(ctx, engine.LinkInput{Repo: str("repo"), Branch: str("branch")})
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

// mustJSON marshals a tool payload to COMPACT (un-indented) JSON. Agents don't
// need indentation, and compact output is a real token saving on the MCP surface
// — so this deliberately uses json.Marshal, never json.MarshalIndent.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ── Legacy HTTP+SSE transport (deprecated 2024-11-05) ───────────────────────
//
// Modern clients use the Streamable HTTP transport (HTTPHandler). The legacy SSE
// transport is a two-channel design for older clients:
//
//   1. GET /sse opens a long-lived text/event-stream. The server immediately
//      emits an `endpoint` event whose data is the relative POST-back URL
//      (/message?sessionId=<id>), then keeps the stream open with periodic ping
//      comment lines until the client disconnects.
//   2. POST /message?sessionId=<id> carries a single JSON-RPC request. The
//      handler routes it via s.dispatch (the same catalog/op routing as stdio
//      and Streamable HTTP) and delivers the JSON-RPC RESPONSE as a `message`
//      event on the GET stream that owns that session.
//
// Sessions live in an in-memory registry mapping sessionId to the GET stream's
// response channel, guarded by a mutex; entries are removed on disconnect.

// sseSession is one open GET /sse stream awaiting POST-delivered responses.
type sseSession struct {
	ch   chan []byte   // serialized JSON-RPC responses to emit as `message` events
	done chan struct{} // closed when the GET stream tears down
}

// sseRegistry maps sessionId -> session for cross-request response delivery.
type sseRegistry struct {
	mu       sync.Mutex
	sessions map[string]*sseSession
}

func (r *sseRegistry) add(id string, s *sseSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = make(map[string]*sseSession)
	}
	r.sessions[id] = s
}

func (r *sseRegistry) get(id string) (*sseSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	return s, ok
}

func (r *sseRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// sseHandler implements the legacy GET /sse + POST /message endpoints over a
// shared session registry. It reuses the Server's dispatch and rpc types — no
// catalog duplication.
type sseHandler struct {
	srv      *Server
	registry *sseRegistry
	ssePath  string // path of the GET stream endpoint, e.g. "/sse"
	msgPath  string // path of the POST endpoint, e.g. "/message"
}

// SSEHandler returns an http.Handler implementing the legacy MCP HTTP+SSE
// transport. Mount it so that GET /sse and POST /message both reach this handler
// (a single mux entry at "/" works, or two entries — the handler routes by
// method+path). ServeStdio and HTTPHandler are unaffected.
func (s *Server) SSEHandler() http.Handler {
	return &sseHandler{
		srv:      s,
		registry: &sseRegistry{sessions: make(map[string]*sseSession)},
		ssePath:  "/sse",
		msgPath:  "/message",
	}
}

func (h *sseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == h.ssePath:
		h.serveSSE(w, r)
	case r.Method == http.MethodPost && r.URL.Path == h.msgPath:
		h.serveMessage(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "not found: use GET "+h.ssePath+" or POST "+h.msgPath, http.StatusNotFound)
	}
}

// serveSSE opens the event-stream, registers the session, emits the `endpoint`
// event, and pumps `message` events (POST-delivered responses) plus periodic
// ping comments until the client disconnects.
func (h *sseHandler) serveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	id, err := newSessionID()
	if err != nil {
		http.Error(w, "session id generation failed", http.StatusInternalServerError)
		return
	}

	sess := &sseSession{ch: make(chan []byte, 16), done: make(chan struct{})}
	h.registry.add(id, sess)
	defer func() {
		h.registry.remove(id)
		close(sess.done)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Tell the client where to POST its JSON-RPC requests for this session.
	fmt.Fprintf(w, "event: endpoint\ndata: %s?sessionId=%s\n\n", h.msgPath, id)
	flusher.Flush()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return // client disconnected
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case payload := <-sess.ch:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

// serveMessage routes a POSTed JSON-RPC request through s.dispatch and delivers
// the response on the SSE stream owning the sessionId. The HTTP POST itself
// returns 202 Accepted with no body — the response is the SSE `message` event.
func (h *sseHandler) serveMessage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("sessionId")
	if id == "" {
		http.Error(w, "missing sessionId", http.StatusBadRequest)
		return
	}
	sess, ok := h.registry.get(id)
	if !ok {
		http.Error(w, "unknown sessionId", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
	if err != nil {
		http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(bytes.TrimSpace(body), &req); err != nil {
		// Deliver a parse-error response on the stream so the client sees it.
		h.deliver(sess, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Notifications (no id) get no response — just acknowledge the POST.
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := h.srv.dispatch(r.Context(), &req)
	h.deliver(sess, resp)
	w.WriteHeader(http.StatusAccepted)
}

// deliver pushes a JSON-RPC response onto the session's stream channel, dropping
// it only if the stream has already torn down (avoids blocking the POST).
func (h *sseHandler) deliver(sess *sseSession, resp rpcResponse) {
	payload, err := json.Marshal(resp)
	if err != nil {
		return
	}
	select {
	case sess.ch <- payload:
	case <-sess.done:
	}
}

// newSessionID returns a random 128-bit hex session identifier.
func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
