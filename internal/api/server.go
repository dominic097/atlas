// Package api implements consumption surface S2: the REST HTTP API exposed by
// `atlas serve`. It is a thin transport over the shared Engine — each handler
// parses a request, calls the matching engine method, and writes the JSON
// result under a {"data": …} envelope. Errors render as RFC 9457
// application/problem+json with a stable code and the correct status.
//
// Auth is optional and local-default-off: when the ATLAS_API_TOKEN env var is
// set, every /api/v1/* request must carry `Authorization: Bearer <token>`;
// when unset there is no auth (the local single-user default).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/engine"
)

// Config configures the HTTP server.
type Config struct {
	Addr     string
	MountMCP bool
	// Token is the bearer token required on /api/v1/*. When empty it falls back to
	// the ATLAS_API_TOKEN env var; when both are empty, auth is disabled.
	Token string
}

// Server is the Atlas HTTP API server.
type Server struct {
	eng   engine.Engine
	cfg   Config
	mux   *http.ServeMux
	token string
}

// NewServer builds the router and maps the canonical operation routes.
func NewServer(eng engine.Engine, cfg Config) *Server {
	token := cfg.Token
	if token == "" {
		token = os.Getenv("ATLAS_API_TOKEN")
	}
	s := &Server{eng: eng, cfg: cfg, mux: http.NewServeMux(), token: token}
	s.routes()
	return s
}

// Handler returns the fully-wired http.Handler (auth middleware + routes). It is
// exported so tests and embedders can drive the server without a live listener.
func (s *Server) Handler() http.Handler {
	return s.withAuth(s.mux)
}

// routes maps the canonical Atlas catalog onto /api/v1. Atlas is the
// deterministic code-intelligence layer: no agentic (rca/fix/review) or webhook
// routes — those live in Pulse, which consumes this API. Hosted-only ops
// (cross-repo) are registered too; they return honest-empty on a single-repo
// local engine.
func (s *Server) routes() {
	m := s.mux

	// infra (no auth, no /api/v1 prefix)
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	m.HandleFunc("GET /openapi.json", s.handleOpenAPI)

	// status / index
	m.HandleFunc("GET /api/v1/status", s.handleStatus)
	m.HandleFunc("POST /api/v1/index", s.handleIndex)

	// search
	m.HandleFunc("GET /api/v1/search", s.handleSearch)

	// symbol navigation
	m.HandleFunc("GET /api/v1/symbols/{name}", s.handleSymbol)
	m.HandleFunc("GET /api/v1/symbols/{name}/callers", s.handleCallers)
	m.HandleFunc("GET /api/v1/symbols/{name}/refs", s.handleRefs)
	m.HandleFunc("GET /api/v1/symbols/{name}/neighbors", s.handleNeighbors)
	m.HandleFunc("GET /api/v1/symbols/{name}/explain", s.handleExplain)

	// graph ops
	m.HandleFunc("POST /api/v1/impact", s.handleImpact)
	m.HandleFunc("GET /api/v1/path", s.handlePath)
	m.HandleFunc("GET /api/v1/coverage", s.handleCoverage)
	m.HandleFunc("GET /api/v1/export", s.handleExport)

	// temporal
	m.HandleFunc("GET /api/v1/history", s.handleHistory)
	m.HandleFunc("GET /api/v1/snapshot-diff", s.handleSnapshotDiff)

	// repos + cross-repo (hosted)
	m.HandleFunc("GET /api/v1/repos", s.handleRepos)
	m.HandleFunc("GET /api/v1/repos/{repo}/route-contracts", s.handleRouteContracts)
	m.HandleFunc("GET /api/v1/repos/{repo}/consumers", s.handleConsumers)
	m.HandleFunc("POST /api/v1/repos/{repo}/cross-repo-impact", s.handleCrossRepoImpact)
}

// ListenAndServe starts the server and shuts down on context cancel.
func (s *Server) ListenAndServe(ctx context.Context) error {
	httpSrv := &http.Server{Addr: s.cfg.Addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// ── auth middleware ───────────────────────────────────────────────────────────

// withAuth enforces a bearer token on /api/v1/* when one is configured. Infra
// and discovery routes (/healthz, /readyz, /openapi.json) are always open.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && strings.HasPrefix(r.URL.Path, "/api/v1/") {
			if !s.authorized(r) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="atlas"`)
				writeProblem(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// authorized reports whether the request carries the configured bearer token.
func (s *Server) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return false
	}
	return strings.TrimSpace(h[len(prefix):]) == s.token
}

// ── path/symbol handlers ─────────────────────────────────────────────────────

// symbolName extracts and percent-decodes the {name} path wildcard.
func symbolName(r *http.Request) (string, error) {
	name := r.PathValue("name")
	dec, err := url.PathUnescape(name)
	if err != nil {
		return "", err
	}
	return dec, nil
}

// repoName extracts and percent-decodes the {repo} path wildcard.
func repoName(r *http.Request) (string, error) {
	return url.PathUnescape(r.PathValue("repo"))
}

func (s *Server) handleSymbol(w http.ResponseWriter, r *http.Request) {
	name, err := symbolName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid symbol name: "+err.Error())
		return
	}
	res, err := s.eng.Symbol(r.Context(), engine.SymbolInput{Name: name, RepoID: r.URL.Query().Get("repo_id")})
	writeResult(w, res, err)
}

func (s *Server) handleCallers(w http.ResponseWriter, r *http.Request) {
	name, err := symbolName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid symbol name: "+err.Error())
		return
	}
	res, err := s.eng.Callers(r.Context(), engine.CallersInput{
		Name:   name,
		RepoID: r.URL.Query().Get("repo_id"),
		Limit:  queryInt(r, "limit", 0),
	})
	writeResult(w, res, err)
}

func (s *Server) handleRefs(w http.ResponseWriter, r *http.Request) {
	name, err := symbolName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid symbol name: "+err.Error())
		return
	}
	res, err := s.eng.Refs(r.Context(), engine.RefsInput{Name: name, RepoID: r.URL.Query().Get("repo_id")})
	writeResult(w, res, err)
}

func (s *Server) handleNeighbors(w http.ResponseWriter, r *http.Request) {
	name, err := symbolName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid symbol name: "+err.Error())
		return
	}
	res, err := s.eng.Neighbors(r.Context(), engine.NeighborsInput{Name: name, RepoID: r.URL.Query().Get("repo_id")})
	writeResult(w, res, err)
}

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	name, err := symbolName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid symbol name: "+err.Error())
		return
	}
	res, err := s.eng.Explain(r.Context(), engine.ExplainInput{Name: name, RepoID: r.URL.Query().Get("repo_id")})
	writeResult(w, res, err)
}

// ── index / search / status ──────────────────────────────────────────────────

type indexRequest struct {
	ProjectPath string `json:"project_path"`
	Repo        string `json:"repo"`
	Reindex     bool   `json:"reindex"`
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	var req indexRequest
	if err := decodeBody(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := s.eng.Index(r.Context(), engine.IndexInput{
		ProjectPath: req.ProjectPath,
		Repo:        req.Repo,
		Reindex:     req.Reindex,
	})
	writeResult(w, res, err)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.eng.Search(r.Context(), engine.SearchInput{
		Query:  q.Get("q"),
		RepoID: q.Get("repo_id"),
		Kind:   q.Get("kind"),
		Limit:  queryInt(r, "limit", 0),
		Mode:   orDefault(q.Get("mode"), "lexical"),
	})
	writeResult(w, res, err)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	res, err := s.eng.Status(r.Context(), engine.StatusInput{
		RepoID:  r.URL.Query().Get("repo_id"),
		Verbose: queryBool(r, "verbose"),
	})
	writeResult(w, res, err)
}

// handleRepos returns the indexed-repo list (a projection of Status.Repos).
func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	res, err := s.eng.Status(r.Context(), engine.StatusInput{})
	if err != nil {
		writeError(w, err)
		return
	}
	repos := res.Repos
	if repos == nil {
		repos = []engine.RepoStatus{}
	}
	writeResult(w, repos, nil)
}

// ── graph ops ─────────────────────────────────────────────────────────────────

type impactRequest struct {
	ChangedPaths []string `json:"changed_paths"`
	Symbols      []string `json:"symbols"`
	MaxDepth     int      `json:"max_depth"`
	RepoID       string   `json:"repo_id"`
}

func (s *Server) handleImpact(w http.ResponseWriter, r *http.Request) {
	var req impactRequest
	if err := decodeBody(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := s.eng.Impact(r.Context(), engine.ImpactInput{
		ChangedPaths: req.ChangedPaths,
		Symbols:      req.Symbols,
		MaxDepth:     req.MaxDepth,
		RepoID:       req.RepoID,
		IncludeTests: true,
	})
	writeResult(w, res, err)
}

func (s *Server) handlePath(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.eng.Path(r.Context(), engine.PathInput{
		From:     q.Get("from"),
		To:       q.Get("to"),
		RepoID:   q.Get("repo_id"),
		MaxDepth: queryInt(r, "max_depth", 0),
	})
	writeResult(w, res, err)
}

func (s *Server) handleCoverage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.eng.Coverage(r.Context(), engine.CoverageInput{
		Target:    q.Get("target"),
		RepoID:    q.Get("repo_id"),
		Direction: q.Get("direction"),
	})
	writeResult(w, res, err)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.eng.GraphExport(r.Context(), engine.GraphExportInput{
		RepoID: q.Get("repo_id"),
		Symbol: q.Get("symbol"),
		Depth:  queryInt(r, "depth", 0),
		Format: q.Get("format"),
		All:    queryBool(r, "all"),
	})
	writeResult(w, res, err)
}

// ── temporal ──────────────────────────────────────────────────────────────────

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.eng.History(r.Context(), engine.HistoryInput{
		RepoID: q.Get("repo"),
		Limit:  queryInt(r, "limit", 0),
	})
	writeResult(w, res, err)
}

func (s *Server) handleSnapshotDiff(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.eng.SnapshotDiff(r.Context(), engine.SnapshotDiffInput{
		RepoID: q.Get("repo"),
		From:   q.Get("from"),
		To:     q.Get("to"),
	})
	writeResult(w, res, err)
}

// ── cross-repo (hosted) ──────────────────────────────────────────────────────

func (s *Server) handleRouteContracts(w http.ResponseWriter, r *http.Request) {
	repo, err := repoName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid repo: "+err.Error())
		return
	}
	res, err := s.eng.RouteContracts(r.Context(), engine.RouteContractsInput{Repo: repo})
	writeResult(w, res, err)
}

func (s *Server) handleConsumers(w http.ResponseWriter, r *http.Request) {
	repo, err := repoName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid repo: "+err.Error())
		return
	}
	res, err := s.eng.Consumers(r.Context(), engine.ConsumersInput{Repo: repo})
	writeResult(w, res, err)
}

type crossRepoImpactRequest struct {
	ChangedPaths []string `json:"changed_paths"`
}

func (s *Server) handleCrossRepoImpact(w http.ResponseWriter, r *http.Request) {
	repo, err := repoName(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", "invalid repo: "+err.Error())
		return
	}
	var req crossRepoImpactRequest
	if err := decodeBody(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := s.eng.CrossRepoImpact(r.Context(), engine.CrossRepoImpactInput{
		Repo:         repo,
		ChangedPaths: req.ChangedPaths,
	})
	writeResult(w, res, err)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func queryBool(r *http.Request, key string) bool {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

// decodeBody decodes an optional JSON request body. An empty body is allowed
// (every POST op has sensible zero-value defaults). Unknown fields are rejected
// so typos surface as 400s rather than silent no-ops.
func decodeBody(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, errEOF) {
			return nil // empty body → defaults
		}
		return errors.New("invalid JSON body: " + err.Error())
	}
	return nil
}

// errEOF lets decodeBody treat an empty body as "no body" without importing io
// twice; json.Decode returns io.EOF on an empty stream.
var errEOF = errorString("EOF")

type errorString string

func (e errorString) Error() string { return string(e) }

func (e errorString) Is(target error) bool {
	return target != nil && target.Error() == string(e)
}

func writeResult(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

// writeError maps engine errors onto the right RFC 9457 problem response:
//   - ErrNoIndex / repo-or-snapshot-not-found → 404
//   - ErrNotImplemented                       → 501
//   - ErrTierUnsupported                      → 501
//   - everything else                         → 500
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, engine.ErrNoIndex):
		writeProblem(w, http.StatusNotFound, "not_found", err.Error())
	case isNotFound(err):
		writeProblem(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, engine.ErrNotImplemented):
		writeProblem(w, http.StatusNotImplemented, "not_implemented", err.Error())
	case errors.Is(err, engine.ErrTierUnsupported):
		writeProblem(w, http.StatusNotImplemented, "tier_unsupported", err.Error())
	case isUnprocessable(err):
		writeProblem(w, http.StatusUnprocessableEntity, "unprocessable", err.Error())
	default:
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

// isUnprocessable recognises engine precondition failures that reflect the
// caller's data state, not a server fault — e.g. a snapshot diff requested on a
// repo with only one snapshot. 422 is the honest status (not 500).
func isUnprocessable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "two snapshots")
}

// isNotFound recognises the engine's not-found errors that are plain fmt.Errorf
// values (no sentinel): `atlas: repo %q not found` and `atlas: snapshot %q not
// found`. These resolve a named repo/snapshot the caller asked for, so 404 is
// the honest status.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found")
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"code":   code,
		"detail": detail,
	})
}
