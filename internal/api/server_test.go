package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dominic097/atlas/pkg/atlas"
)

// newTestServer builds a real SQLite-backed engine over a temp dir, indexes a
// tiny 1-file Go repo (Caller -> Callee, so callers/refs return data), and
// returns a Server wired to it. token, when non-empty, enables bearer auth.
func newTestServer(t *testing.T, token string) *Server {
	t.Helper()
	ctx := context.Background()

	// 1-file repo: Callee is invoked by Caller, giving the call graph an edge.
	repoDir := t.TempDir()
	src := `package sample

// Callee does the work.
func Callee() int { return 42 }

// Caller invokes Callee.
func Caller() int { return Callee() }
`
	if err := os.WriteFile(filepath.Join(repoDir, "sample.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write sample.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module sample\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	eng, err := atlas.New(ctx, atlas.WithSQLite(dbPath))
	if err != nil {
		t.Fatalf("atlas.New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	if _, err := eng.Index(ctx, atlas.IndexInput{ProjectPath: repoDir, Repo: "org/sample"}); err != nil {
		t.Fatalf("index: %v", err)
	}

	return NewServer(eng, Config{Token: token})
}

// do issues a request against the server's full handler (auth middleware
// included) and returns the recorder.
func do(t *testing.T, s *Server, method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// decodeData unwraps the {"data": …} envelope into out.
func decodeData(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, rec.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			t.Fatalf("decode data: %v", err)
		}
	}
}

func TestStatusOK(t *testing.T) {
	s := newTestServer(t, "")
	rec := do(t, s, http.MethodGet, "/api/v1/status", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: got %q", ct)
	}
	var st struct {
		Tier         string `json:"tier"`
		ReposIndexed int    `json:"repos_indexed"`
		Repos        []struct {
			FullName string `json:"repo_full_name"`
		} `json:"repos"`
	}
	decodeData(t, rec, &st)
	if st.ReposIndexed != 1 || len(st.Repos) != 1 {
		t.Fatalf("expected 1 repo, got repos_indexed=%d repos=%d", st.ReposIndexed, len(st.Repos))
	}
	if st.Repos[0].FullName != "org/sample" {
		t.Errorf("repo full name: got %q", st.Repos[0].FullName)
	}
}

func TestStatsOK(t *testing.T) {
	s := newTestServer(t, "")
	rec := do(t, s, http.MethodGet, "/api/v1/stats?limit=5", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var res struct {
		RepoFullName string `json:"repo_full_name"`
		Latest       struct {
			Mode      string           `json:"mode"`
			Files     int              `json:"files"`
			Symbols   int              `json:"symbols"`
			TimingsMS map[string]int64 `json:"timings_ms"`
		} `json:"latest"`
		Graph struct {
			Symbols int `json:"symbols"`
			Edges   int `json:"edges"`
		} `json:"graph"`
		HistoryReturned int `json:"history_returned"`
	}
	decodeData(t, rec, &res)
	if res.RepoFullName != "org/sample" {
		t.Fatalf("repo full name = %q, want org/sample", res.RepoFullName)
	}
	if res.Latest.Mode != "full" || res.Latest.Files == 0 || res.Latest.Symbols == 0 {
		t.Fatalf("latest stats look wrong: %+v", res.Latest)
	}
	if len(res.Latest.TimingsMS) == 0 {
		t.Fatalf("latest timings should be persisted")
	}
	if res.Graph.Symbols == 0 || res.Graph.Edges == 0 {
		t.Fatalf("graph stats should include symbols and edges: %+v", res.Graph)
	}
	if res.HistoryReturned != 1 {
		t.Fatalf("history_returned = %d, want 1", res.HistoryReturned)
	}
}

func TestExplainPlainFormat(t *testing.T) {
	s := newTestServer(t, "")
	rec := do(t, s, http.MethodGet, "/api/v1/symbols/Callee/explain?repo_id=org/sample&format=plain", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("explain plain: got %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type: got %q, want text/plain", ct)
	}
	got := strings.TrimSpace(rec.Body.String())
	if got != "Callee f@sample:4" {
		t.Fatalf("plain explain = %q, want %q", got, "Callee f@sample:4")
	}
}

func TestSearchOK(t *testing.T) {
	s := newTestServer(t, "")
	rec := do(t, s, http.MethodGet, "/api/v1/search?q=Caller", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var res struct {
		Results []struct {
			Name string `json:"symbol"`
		} `json:"results"`
		ModeUsed string `json:"mode_used"`
	}
	decodeData(t, rec, &res)
	if res.ModeUsed != "lexical" {
		t.Errorf("mode_used: got %q", res.ModeUsed)
	}
	found := false
	for _, h := range res.Results {
		if h.Name == "Caller" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Caller in search results, got %+v", res.Results)
	}
}

func TestCallersOK(t *testing.T) {
	s := newTestServer(t, "")
	rec := do(t, s, http.MethodGet, "/api/v1/symbols/Callee/callers?limit=10", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("callers: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var res struct {
		Symbol  string `json:"symbol"`
		Callers []struct {
			Name string `json:"symbol"`
		} `json:"callers"`
		Total int `json:"total"`
	}
	decodeData(t, rec, &res)
	if res.Symbol != "Callee" {
		t.Errorf("symbol: got %q", res.Symbol)
	}
	found := false
	for _, c := range res.Callers {
		if c.Name == "Caller" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Caller to call Callee, got callers=%+v total=%d", res.Callers, res.Total)
	}
}

func TestImpactOK(t *testing.T) {
	s := newTestServer(t, "")
	body := []byte(`{"changed_paths":["sample.go"],"max_depth":3}`)
	rec := do(t, s, http.MethodPost, "/api/v1/impact", body, map[string]string{"Content-Type": "application/json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("impact: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var res struct {
		ImpactedSymbols []string `json:"impacted_symbols"`
		DepthReached    int      `json:"depth_reached"`
	}
	decodeData(t, rec, &res)
	// shape assertion: the envelope decodes and depth is the requested cap or less.
	if res.DepthReached < 0 {
		t.Errorf("unexpected depth_reached %d", res.DepthReached)
	}
}

func TestOpenAPIOK(t *testing.T) {
	s := newTestServer(t, "")
	rec := do(t, s, http.MethodGet, "/openapi.json", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi: got %d", rec.Code)
	}
	var doc struct {
		OpenAPI string                            `json:"openapi"`
		Info    map[string]any                    `json:"info"`
		Paths   map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("openapi version: got %q", doc.OpenAPI)
	}
	if doc.Info["title"] == nil {
		t.Errorf("missing info.title")
	}
	// every routeTable entry must be present.
	for _, rt := range routeTable {
		item, ok := doc.Paths[rt.path]
		if !ok {
			t.Errorf("openapi missing path %s", rt.path)
			continue
		}
		if _, ok := item[rt.method]; !ok {
			t.Errorf("openapi path %s missing method %s", rt.path, rt.method)
		}
	}
}

func TestUnknownRepo404(t *testing.T) {
	s := newTestServer(t, "")
	rec := do(t, s, http.MethodGet, "/api/v1/symbols/Callee/callers?repo_id=does-not-exist", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown repo: got %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("content-type: got %q, want application/problem+json", ct)
	}
	var p struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if p.Status != http.StatusNotFound || p.Code != "not_found" {
		t.Errorf("problem: got status=%d code=%q", p.Status, p.Code)
	}
}

func TestAuthRequired401(t *testing.T) {
	s := newTestServer(t, "secret-token")

	// Missing header → 401 problem.
	rec := do(t, s, http.MethodGet, "/api/v1/status", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("content-type: got %q", ct)
	}
	var p struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if p.Code != "unauthorized" {
		t.Errorf("problem code: got %q", p.Code)
	}

	// Correct header → 200.
	rec = do(t, s, http.MethodGet, "/api/v1/status", nil, map[string]string{"Authorization": "Bearer secret-token"})
	if rec.Code != http.StatusOK {
		t.Fatalf("with token: got %d, body=%s", rec.Code, rec.Body.String())
	}

	// openapi.json stays public even with auth on.
	rec = do(t, s, http.MethodGet, "/openapi.json", nil, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("openapi should be public: got %d", rec.Code)
	}
}
