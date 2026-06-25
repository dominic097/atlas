// Package api implements consumption surface S2: the REST HTTP API exposed by
// `atlas serve`. It is a thin transport over the shared Engine. The full server
// adds auth, tenancy, rate limiting, idempotency, SSE, and OpenAPI; the
// skeleton wires the canonical route table over stdlib net/http.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/engine"
)

// Config configures the HTTP server.
type Config struct {
	Addr     string
	MountMCP bool
}

// Server is the Atlas HTTP API server.
type Server struct {
	eng engine.Engine
	cfg Config
	mux *http.ServeMux
}

// NewServer builds the router and maps the canonical operation routes.
func NewServer(eng engine.Engine, cfg Config) *Server {
	s := &Server{eng: eng, cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

// routes maps the canonical Atlas catalog onto /api/v1. Atlas is the
// deterministic code-intelligence layer: no agentic (rca/fix/review) or webhook
// routes — those live in Pulse, which consumes this API. Hosted-only ops
// (cross-repo) are registered too; they return honest-empty on a single-repo
// local engine.
func (s *Server) routes() {
	m := s.mux

	// infra
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// core (both tiers)
	m.HandleFunc("POST /api/v1/repos/{repo_id}/index", s.handleIndex)
	m.HandleFunc("GET /api/v1/search", s.handleSearch)
	m.HandleFunc("POST /api/v1/repos/{repo_id}/impact", s.handleImpact)
	m.HandleFunc("GET /api/v1/status", s.handleStatus)

	// remaining canonical ops are registered as not-implemented placeholders so
	// the route table is complete and self-documenting.
	for _, p := range []string{
		"GET /api/v1/symbols/{symbol_id}",
		"GET /api/v1/symbols/{symbol_id}/callers",
		"GET /api/v1/symbols/{symbol_id}/references",
		"GET /api/v1/graph/neighbors",
		"GET /api/v1/graph/path",
		"POST /api/v1/explain",
		"GET /api/v1/repos/{repo_id}/graph",
		"POST /api/v1/repos/{repo_id}/cross-repo-impact", // hosted
		"GET /api/v1/repos/{repo_id}/consumers",          // hosted
		"GET /api/v1/route-contracts",                    // hosted
		"GET /api/v1/history",
		"GET /api/v1/repos/{repo_id}/snapshots/diff",
		"GET /api/v1/coverage",
		"GET /api/v1/repos",
		"POST /api/v1/repos", // link (hosted)
	} {
		m.HandleFunc(p, s.handleNotImplemented)
	}
}

// ListenAndServe starts the server and shuts down on context cancel.
func (s *Server) ListenAndServe(ctx context.Context) error {
	httpSrv := &http.Server{Addr: s.cfg.Addr, Handler: s.mux}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// ── handlers ────────────────────────────────────────────────────────────────

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	in := engine.IndexInput{Repo: r.PathValue("repo_id")}
	res, err := s.eng.Index(r.Context(), in)
	writeResult(w, res, err)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	in := engine.SearchInput{
		Query:  q.Get("q"),
		RepoID: q.Get("repo_id"),
		Kind:   q.Get("kind"),
		Mode:   orDefault(q.Get("mode"), "lexical"),
	}
	res, err := s.eng.Search(r.Context(), in)
	writeResult(w, res, err)
}

func (s *Server) handleImpact(w http.ResponseWriter, r *http.Request) {
	in := engine.ImpactInput{RepoID: r.PathValue("repo_id"), MaxDepth: 3, IncludeTests: true}
	res, err := s.eng.Impact(r.Context(), in)
	writeResult(w, res, err)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	res, err := s.eng.Status(r.Context(), engine.StatusInput{})
	writeResult(w, res, err)
}

func (s *Server) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	writeProblem(w, http.StatusNotImplemented, "not_implemented", engine.ErrNotImplemented.Error())
}

// ── helpers ─────────────────────────────────────────────────────────────────

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func writeResult(w http.ResponseWriter, v any, err error) {
	if err != nil {
		if errors.Is(err, engine.ErrNotImplemented) {
			writeProblem(w, http.StatusNotImplemented, "not_implemented", err.Error())
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status, "code": code, "detail": detail,
	})
}
