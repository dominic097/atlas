// Package atlasclient is a hand-written HTTP client for the Atlas REST API
// (consumption surface S2, served by `atlas serve`). It is a thin, typed wrapper
// over net/http: one method per canonical operation, a base URL plus an optional
// bearer token, JSON decoding into the shared result types from
// github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas, and RFC 9457
// application/problem+json error parsing into a typed *APIError.
//
//	c, err := atlasclient.New("http://localhost:8080", atlasclient.WithToken("secret"))
//	if err != nil { ... }
//	st, err := c.Status(ctx, nil)
//	if err != nil {
//		var apiErr *atlasclient.APIError
//		if errors.As(err, &apiErr) { /* apiErr.Status, apiErr.Code, apiErr.Detail */ }
//	}
package atlasclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

// Client talks to an Atlas REST API server over HTTP. It is safe for concurrent
// use by multiple goroutines.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Option customizes a Client at construction time.
type Option func(*Client)

// WithToken sets the bearer token sent as `Authorization: Bearer <token>` on
// every /api/v1 request. Leave it unset for a local auth-disabled server.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithHTTPClient swaps in a custom *http.Client (timeouts, transport, proxy…).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// New builds a Client for the given base URL (e.g. "http://localhost:8080").
// A trailing slash is trimmed. The base URL must parse as an absolute URL.
func New(baseURL string, opts ...Option) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, errors.New("atlasclient: empty base URL")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("atlasclient: invalid base URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("atlasclient: base URL must be absolute, got %q", baseURL)
	}
	c := &Client{baseURL: trimmed, http: http.DefaultClient}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// APIError is the typed form of an RFC 9457 application/problem+json response.
// It is returned by every method on a non-2xx status.
type APIError struct {
	// Status is the HTTP status code (e.g. 404, 422, 500).
	Status int `json:"status"`
	// Code is the stable Atlas error code (e.g. "not_found", "unprocessable").
	Code string `json:"code"`
	// Detail is the human-readable explanation.
	Detail string `json:"detail"`
	// Title is the HTTP status text (e.g. "Not Found").
	Title string `json:"title"`
	// Type is the problem type URI (usually "about:blank").
	Type string `json:"type"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("atlas api error %d (%s): %s", e.Status, e.Code, e.Detail)
	}
	return fmt.Sprintf("atlas api error %d (%s)", e.Status, e.Code)
}

// envelope is the {"data": …} success wrapper every 2xx response uses.
type envelope[T any] struct {
	Data T `json:"data"`
}

// RepoStatus mirrors the engine's per-repo status row returned by GET
// /api/v1/repos. It is defined here because the bare slice is not re-exported
// from pkg/atlas; the JSON tags match the server wire format exactly.
type RepoStatus struct {
	RepoID    string `json:"repo_id"`
	FullName  string `json:"repo_full_name"`
	Snapshot  string `json:"snapshot_id"`
	CommitSHA string `json:"commit_sha"`
	Symbols   int    `json:"symbols"`
	Edges     int    `json:"edges"`
	IndexedAt string `json:"indexed_at"`
}

// ── public operations (one method per canonical op) ─────────────────────────

// StatusParams are the optional query parameters for Status.
type StatusParams struct {
	RepoID  string
	Verbose bool
}

// Status returns engine/tier status. Pass nil for defaults.
func (c *Client) Status(ctx context.Context, p *StatusParams) (*atlas.StatusResult, error) {
	q := url.Values{}
	if p != nil {
		setStr(q, "repo_id", p.RepoID)
		if p.Verbose {
			q.Set("verbose", "true")
		}
	}
	return getJSON[atlas.StatusResult](ctx, c, "/api/v1/status", q)
}

// IndexParams is the request body for Index.
type IndexParams struct {
	ProjectPath string `json:"project_path,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Reindex     bool   `json:"reindex,omitempty"`
}

// Index (re)indexes a project on the server. The server resolves paths relative
// to its own filesystem.
func (c *Client) Index(ctx context.Context, in IndexParams) (*atlas.IndexResult, error) {
	return postJSON[atlas.IndexResult](ctx, c, "/api/v1/index", in)
}

// SearchParams are the query parameters for Search.
type SearchParams struct {
	Query  string // the search query (?q=)
	RepoID string
	Kind   string
	Limit  int
	Mode   string // lexical (default) | semantic | hybrid
}

// Search runs a lexical (or semantic/hybrid) symbol search.
func (c *Client) Search(ctx context.Context, p SearchParams) (*atlas.SearchResult, error) {
	q := url.Values{}
	setStr(q, "q", p.Query)
	setStr(q, "repo_id", p.RepoID)
	setStr(q, "kind", p.Kind)
	setStr(q, "mode", p.Mode)
	setInt(q, "limit", p.Limit)
	return getJSON[atlas.SearchResult](ctx, c, "/api/v1/search", q)
}

// Symbol returns the definition(s) of a symbol, with callers and callees.
// repoID is optional (pass "").
func (c *Client) Symbol(ctx context.Context, name, repoID string) (*atlas.SymbolResult, error) {
	q := url.Values{}
	setStr(q, "repo_id", repoID)
	return getJSON[atlas.SymbolResult](ctx, c, "/api/v1/symbols/"+escapeSeg(name), q)
}

// Callers returns the resolved callers of a symbol.
func (c *Client) Callers(ctx context.Context, name, repoID string, limit int) (*atlas.CallersResult, error) {
	q := url.Values{}
	setStr(q, "repo_id", repoID)
	setInt(q, "limit", limit)
	return getJSON[atlas.CallersResult](ctx, c, "/api/v1/symbols/"+escapeSeg(name)+"/callers", q)
}

// Refs returns the call-site references to a symbol.
func (c *Client) Refs(ctx context.Context, name, repoID string) (*atlas.RefsResult, error) {
	q := url.Values{}
	setStr(q, "repo_id", repoID)
	return getJSON[atlas.RefsResult](ctx, c, "/api/v1/symbols/"+escapeSeg(name)+"/refs", q)
}

// Neighbors returns the depth-1 call neighborhood (callers + callees) of a symbol.
func (c *Client) Neighbors(ctx context.Context, name, repoID string) (*atlas.NeighborsResult, error) {
	q := url.Values{}
	setStr(q, "repo_id", repoID)
	return getJSON[atlas.NeighborsResult](ctx, c, "/api/v1/symbols/"+escapeSeg(name)+"/neighbors", q)
}

// Explain returns the deterministic context bundle for a symbol.
func (c *Client) Explain(ctx context.Context, name, repoID string) (*atlas.ExplainResult, error) {
	q := url.Values{}
	setStr(q, "repo_id", repoID)
	return getJSON[atlas.ExplainResult](ctx, c, "/api/v1/symbols/"+escapeSeg(name)+"/explain", q)
}

// Impact runs reverse-BFS blast-radius analysis from changed paths/symbols.
func (c *Client) Impact(ctx context.Context, in atlas.ImpactInput) (*atlas.ImpactResult, error) {
	body := map[string]any{
		"changed_paths": in.ChangedPaths,
		"symbols":       in.Symbols,
		"max_depth":     in.MaxDepth,
		"repo_id":       in.RepoID,
	}
	return postJSON[atlas.ImpactResult](ctx, c, "/api/v1/impact", body)
}

// Path returns the shortest forward call path from one symbol to another.
// maxDepth <= 0 lets the server pick its default.
func (c *Client) Path(ctx context.Context, from, to string, maxDepth int) (*atlas.PathResult, error) {
	return c.PathInRepo(ctx, from, to, "", maxDepth)
}

// PathInRepo is Path scoped to a specific repo (pass repoID "" for the default).
func (c *Client) PathInRepo(ctx context.Context, from, to, repoID string, maxDepth int) (*atlas.PathResult, error) {
	q := url.Values{}
	setStr(q, "from", from)
	setStr(q, "to", to)
	setStr(q, "repo_id", repoID)
	setInt(q, "max_depth", maxDepth)
	return getJSON[atlas.PathResult](ctx, c, "/api/v1/path", q)
}

// CoverageParams are the query parameters for Coverage.
type CoverageParams struct {
	Target    string
	RepoID    string
	Direction string // tests_for_symbol | symbols_for_test | "" (auto)
}

// Coverage returns static call-graph reachability coverage for a target.
func (c *Client) Coverage(ctx context.Context, p CoverageParams) (*atlas.CoverageResult, error) {
	q := url.Values{}
	setStr(q, "target", p.Target)
	setStr(q, "repo_id", p.RepoID)
	setStr(q, "direction", p.Direction)
	return getJSON[atlas.CoverageResult](ctx, c, "/api/v1/coverage", q)
}

// ExportParams are the query parameters for Export.
type ExportParams struct {
	RepoID string
	Symbol string
	Depth  int
	Format string // json | mermaid | dot
	All    bool
}

// Export renders the call graph (whole snapshot or a symbol neighborhood).
func (c *Client) Export(ctx context.Context, p ExportParams) (*atlas.GraphExportResult, error) {
	q := url.Values{}
	setStr(q, "repo_id", p.RepoID)
	setStr(q, "symbol", p.Symbol)
	setStr(q, "format", p.Format)
	setInt(q, "depth", p.Depth)
	if p.All {
		q.Set("all", "true")
	}
	return getJSON[atlas.GraphExportResult](ctx, c, "/api/v1/export", q)
}

// History returns the snapshot history for a repo. The HTTP layer keys this op
// on the `repo` query parameter (repo full_name or id); pass "" for the default.
func (c *Client) History(ctx context.Context, repo string, limit int) (*atlas.HistoryResult, error) {
	q := url.Values{}
	setStr(q, "repo", repo)
	setInt(q, "limit", limit)
	return getJSON[atlas.HistoryResult](ctx, c, "/api/v1/history", q)
}

// SnapshotDiff returns the structural delta between two snapshots of a repo.
// from/to accept a snapshot id or commit-SHA prefix; "" picks the server defaults
// (to = latest, from = the snapshot before it).
func (c *Client) SnapshotDiff(ctx context.Context, repo, from, to string) (*atlas.SnapshotDiffResult, error) {
	q := url.Values{}
	setStr(q, "repo", repo)
	setStr(q, "from", from)
	setStr(q, "to", to)
	return getJSON[atlas.SnapshotDiffResult](ctx, c, "/api/v1/snapshot-diff", q)
}

// Repos returns the indexed-repo list (a projection of Status.Repos). The HTTP
// layer returns the bare slice under {"data": [...]}, not a StatusResult.
func (c *Client) Repos(ctx context.Context) ([]RepoStatus, error) {
	res, err := getJSON[[]RepoStatus](ctx, c, "/api/v1/repos", nil)
	if err != nil {
		return nil, err
	}
	return *res, nil
}

// RouteContracts returns the producer route contracts a repo serves.
func (c *Client) RouteContracts(ctx context.Context, repo string) (*atlas.RouteContractsResult, error) {
	return getJSON[atlas.RouteContractsResult](ctx, c, "/api/v1/repos/"+escapeSeg(repo)+"/route-contracts", nil)
}

// Consumers returns the cross-repo consumers of a producer repo's routes.
func (c *Client) Consumers(ctx context.Context, repo string) (*atlas.ConsumersResult, error) {
	return getJSON[atlas.ConsumersResult](ctx, c, "/api/v1/repos/"+escapeSeg(repo)+"/consumers", nil)
}

// CrossRepoImpact returns the cross-repo blast radius of changes to a producer
// repo. changedPaths empty = the whole repo's route contract.
func (c *Client) CrossRepoImpact(ctx context.Context, repo string, changedPaths []string) (*atlas.CrossRepoImpactResult, error) {
	body := map[string]any{"changed_paths": changedPaths}
	return postJSON[atlas.CrossRepoImpactResult](ctx, c, "/api/v1/repos/"+escapeSeg(repo)+"/cross-repo-impact", body)
}

// ── transport helpers ───────────────────────────────────────────────────────

// getJSON performs a GET, decodes the {"data": T} envelope, and returns &data.
func getJSON[T any](ctx context.Context, c *Client, path string, q url.Values) (*T, error) {
	return do[T](ctx, c, http.MethodGet, path, q, nil)
}

// postJSON marshals body to JSON, performs a POST, and decodes the envelope.
func postJSON[T any](ctx context.Context, c *Client, path string, body any) (*T, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("atlasclient: marshal request body: %w", err)
	}
	return do[T](ctx, c, http.MethodPost, path, nil, raw)
}

// do is the single request path: build URL + headers, execute, then either parse
// the problem+json error or decode the success envelope.
func do[T any](ctx context.Context, c *Client, method, path string, q url.Values, body []byte) (*T, error) {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, fmt.Errorf("atlasclient: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("atlasclient: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("atlasclient: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(resp.StatusCode, data)
	}

	var env envelope[T]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("atlasclient: decode %s %s response: %w", method, path, err)
	}
	return &env.Data, nil
}

// parseAPIError turns a non-2xx body into a typed *APIError. When the body is not
// valid problem+json, it falls back to the HTTP status with the raw body as detail.
func parseAPIError(status int, body []byte) *APIError {
	apiErr := &APIError{Status: status}
	if len(body) > 0 && json.Unmarshal(body, apiErr) == nil && (apiErr.Code != "" || apiErr.Detail != "") {
		// Some servers omit "status" in the body; trust the HTTP status code.
		apiErr.Status = status
		return apiErr
	}
	apiErr.Code = http.StatusText(status)
	apiErr.Detail = strings.TrimSpace(string(body))
	return apiErr
}

// ── small query helpers ─────────────────────────────────────────────────────

func setStr(q url.Values, key, v string) {
	if v != "" {
		q.Set(key, v)
	}
}

func setInt(q url.Values, key string, v int) {
	if v > 0 {
		q.Set(key, strconv.Itoa(v))
	}
}

// escapeSeg percent-encodes a single path segment (the server PathUnescape-s it).
func escapeSeg(s string) string {
	return url.PathEscape(s)
}
