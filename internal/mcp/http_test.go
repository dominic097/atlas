package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/engine"
)

// stubEngine is a hermetic engine.Engine implementation for transport tests. Only
// Status returns a recognizable payload; every other op is a no-op so the MCP
// HTTP transport can be exercised without a real index, store, or CGO.
type stubEngine struct{}

func (stubEngine) Index(context.Context, engine.IndexInput) (*engine.IndexResult, error) {
	return &engine.IndexResult{}, nil
}
func (stubEngine) Search(context.Context, engine.SearchInput) (*engine.SearchResult, error) {
	return &engine.SearchResult{}, nil
}
func (stubEngine) Context(context.Context, engine.ContextInput) (*engine.ContextResult, error) {
	return &engine.ContextResult{}, nil
}
func (stubEngine) SemanticSearch(context.Context, engine.SemanticSearchInput) (*engine.SemanticSearchResult, error) {
	return &engine.SemanticSearchResult{}, nil
}
func (stubEngine) Impact(context.Context, engine.ImpactInput) (*engine.ImpactResult, error) {
	return &engine.ImpactResult{}, nil
}
func (stubEngine) Callers(context.Context, engine.CallersInput) (*engine.CallersResult, error) {
	return &engine.CallersResult{}, nil
}
func (stubEngine) Symbol(context.Context, engine.SymbolInput) (*engine.SymbolResult, error) {
	return &engine.SymbolResult{}, nil
}
func (stubEngine) Neighbors(context.Context, engine.NeighborsInput) (*engine.NeighborsResult, error) {
	return &engine.NeighborsResult{}, nil
}
func (stubEngine) Path(context.Context, engine.PathInput) (*engine.PathResult, error) {
	return &engine.PathResult{}, nil
}
func (stubEngine) Refs(context.Context, engine.RefsInput) (*engine.RefsResult, error) {
	return &engine.RefsResult{}, nil
}
func (stubEngine) Explain(context.Context, engine.ExplainInput) (*engine.ExplainResult, error) {
	return &engine.ExplainResult{}, nil
}
func (stubEngine) Coverage(context.Context, engine.CoverageInput) (*engine.CoverageResult, error) {
	return &engine.CoverageResult{}, nil
}
func (stubEngine) CoverageImport(context.Context, engine.CoverageImportInput) (*engine.CoverageImportResult, error) {
	return &engine.CoverageImportResult{}, nil
}
func (stubEngine) GraphExport(context.Context, engine.GraphExportInput) (*engine.GraphExportResult, error) {
	return &engine.GraphExportResult{}, nil
}
func (stubEngine) History(context.Context, engine.HistoryInput) (*engine.HistoryResult, error) {
	return &engine.HistoryResult{}, nil
}
func (stubEngine) SnapshotDiff(context.Context, engine.SnapshotDiffInput) (*engine.SnapshotDiffResult, error) {
	return &engine.SnapshotDiffResult{}, nil
}
func (stubEngine) CrossRepoImpact(context.Context, engine.CrossRepoImpactInput) (*engine.CrossRepoImpactResult, error) {
	return &engine.CrossRepoImpactResult{}, nil
}
func (stubEngine) Consumers(context.Context, engine.ConsumersInput) (*engine.ConsumersResult, error) {
	return &engine.ConsumersResult{}, nil
}
func (stubEngine) RouteContracts(context.Context, engine.RouteContractsInput) (*engine.RouteContractsResult, error) {
	return &engine.RouteContractsResult{}, nil
}
func (stubEngine) Status(context.Context, engine.StatusInput) (*engine.StatusResult, error) {
	return &engine.StatusResult{Tier: "test-tier", StorageDriver: "memory", ReposIndexed: 7}, nil
}
func (stubEngine) Link(context.Context, engine.LinkInput) (*engine.LinkResult, error) {
	return &engine.LinkResult{}, nil
}
func (stubEngine) Close() error { return nil }

// httpRPC posts a JSON-RPC body to the MCP HTTP handler and returns the raw
// response body and status code.
func httpRPC(t *testing.T, h http.Handler, body string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res.StatusCode, out
}

func newTestHandler() http.Handler {
	return NewServer(stubEngine{}).HTTPHandler()
}

func TestHTTPHandler_ToolsList(t *testing.T) {
	h := newTestHandler()
	status, body := httpRPC(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, body)
	}

	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != "1" {
		t.Errorf("id = %s, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if len(resp.Result.Tools) == 0 {
		t.Fatal("tools list is empty")
	}
	// status must be present in the advertised catalog.
	var found bool
	for _, tool := range resp.Result.Tools {
		if tool.Name == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("tools/list does not advertise the status tool; got %d tools", len(resp.Result.Tools))
	}
}

func TestHTTPHandler_ToolsCallStatus(t *testing.T) {
	h := newTestHandler()
	status, body := httpRPC(t, h, `{"jsonrpc":"2.0","id":"abc","method":"tools/call","params":{"name":"status","arguments":{}}}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, body)
	}

	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != `"abc"` {
		t.Errorf("id = %s, want \"abc\"", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result.IsError {
		t.Fatal("tools/call(status) returned isError=true")
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("tools/call(status) returned no content")
	}

	// The content text is the JSON-encoded StatusResult from the stub engine.
	var sr engine.StatusResult
	if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &sr); err != nil {
		t.Fatalf("status result unmarshal: %v; text=%s", err, resp.Result.Content[0].Text)
	}
	if sr.Tier != "test-tier" || sr.ReposIndexed != 7 {
		t.Errorf("status payload = %+v, want tier=test-tier repos_indexed=7", sr)
	}
}

func TestHTTPHandler_Batch(t *testing.T) {
	h := newTestHandler()
	body := `[{"jsonrpc":"2.0","id":1,"method":"tools/list"},{"jsonrpc":"2.0","id":2,"method":"initialize"}]`
	status, out := httpRPC(t, h, body)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, out)
	}
	var resps []rpcResponse
	if err := json.Unmarshal(out, &resps); err != nil {
		t.Fatalf("unmarshal batch: %v; body=%s", err, out)
	}
	if len(resps) != 2 {
		t.Fatalf("batch returned %d responses, want 2", len(resps))
	}
}

func TestHTTPHandler_Notification(t *testing.T) {
	h := newTestHandler()
	// No id => notification => 202 with empty body.
	status, out := httpRPC(t, h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", status, out)
	}
	if len(out) != 0 {
		t.Errorf("notification returned a body: %s", out)
	}
}

func TestHTTPHandler_GETNotAllowed(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", rec.Code)
	}
}
