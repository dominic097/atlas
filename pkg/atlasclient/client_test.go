package atlasclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeServer returns an httptest.Server that answers the handful of routes the
// tests exercise: status, search, a bearer-protected route, and a 404 problem.
func fakeServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			writeProblemJSON(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		writeDataJSON(w, map[string]any{
			"tier":           "local",
			"storage_driver": "sqlite",
			"vector_backend": "disabled",
			"repos_indexed":  2,
			"repos":          []any{},
		})
	})

	mux.HandleFunc("GET /api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "Handler" {
			t.Errorf("search: q = %q, want %q", got, "Handler")
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("search: limit = %q, want %q", got, "5")
		}
		writeDataJSON(w, map[string]any{
			"results": []any{
				map[string]any{"symbol": "Handler", "kind": "func", "path": "a.go", "line": 10, "score": 1.5},
			},
			"mode_used": "lexical",
			"total":     1,
		})
	})

	mux.HandleFunc("GET /api/v1/symbols/{name}", func(w http.ResponseWriter, _ *http.Request) {
		writeProblemJSON(w, http.StatusNotFound, "not_found", "atlas: no indexed repo found")
	})

	return httptest.NewServer(mux)
}

func writeDataJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func writeProblemJSON(w http.ResponseWriter, status int, code, detail string) {
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

func TestStatusDecodes(t *testing.T) {
	srv := fakeServer(t, "")
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	st, err := c.Status(context.Background(), nil)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Tier != "local" {
		t.Errorf("Tier = %q, want %q", st.Tier, "local")
	}
	if st.StorageDriver != "sqlite" {
		t.Errorf("StorageDriver = %q, want %q", st.StorageDriver, "sqlite")
	}
	if st.ReposIndexed != 2 {
		t.Errorf("ReposIndexed = %d, want 2", st.ReposIndexed)
	}
}

func TestSearchDecodesAndSendsParams(t *testing.T) {
	srv := fakeServer(t, "")
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := c.Search(context.Background(), SearchParams{Query: "Handler", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 1 || len(res.Results) != 1 {
		t.Fatalf("unexpected results: %+v", res)
	}
	if res.Results[0].Name != "Handler" {
		t.Errorf("Results[0].Name = %q, want %q", res.Results[0].Name, "Handler")
	}
	if res.ModeUsed != "lexical" {
		t.Errorf("ModeUsed = %q, want %q", res.ModeUsed, "lexical")
	}
}

func TestNotFoundReturnsAPIError(t *testing.T) {
	srv := fakeServer(t, "")
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = c.Symbol(context.Background(), "Nope", "")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *APIError: %T %v", err, err)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", apiErr.Status)
	}
	if apiErr.Code != "not_found" {
		t.Errorf("Code = %q, want %q", apiErr.Code, "not_found")
	}
	if !strings.Contains(apiErr.Detail, "no indexed repo") {
		t.Errorf("Detail = %q, want it to mention 'no indexed repo'", apiErr.Detail)
	}
	if !strings.Contains(apiErr.Error(), "404") {
		t.Errorf("Error() = %q, want it to contain status 404", apiErr.Error())
	}
}

func TestBearerTokenIsSent(t *testing.T) {
	srv := fakeServer(t, "s3cr3t")
	defer srv.Close()

	// Without a token the protected route 401s.
	noTok, _ := New(srv.URL)
	if _, err := noTok.Status(context.Background(), nil); err == nil {
		t.Fatal("expected 401 without token")
	} else {
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
			t.Fatalf("want 401 APIError, got %v", err)
		}
	}

	// With the right token it succeeds.
	withTok, _ := New(srv.URL, WithToken("s3cr3t"))
	if _, err := withTok.Status(context.Background(), nil); err != nil {
		t.Fatalf("Status with token: %v", err)
	}
}

func TestNewRejectsBadURL(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Error("expected error for empty URL")
	}
	if _, err := New("not-a-url"); err == nil {
		t.Error("expected error for relative URL")
	}
	if _, err := New("http://localhost:8080/"); err != nil {
		t.Errorf("trailing slash should be fine: %v", err)
	}
}
