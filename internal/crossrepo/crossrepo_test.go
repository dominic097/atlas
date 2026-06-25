package crossrepo

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

func TestEndpointMatch(t *testing.T) {
	cases := []struct {
		name                 string
		prodMethod, prodPath string
		consMethod, consPath string
		want                 bool
	}{
		// keystone: concrete id matches the {id} placeholder.
		{"param vs concrete", "GET", "/api/v1/users/{id}", "GET", "/api/v1/users/1", true},
		// consumer placeholder {} matches producer placeholder.
		{"empty-brace placeholder", "GET", "/api/v1/users/{id}", "GET", "/api/v1/users/{}", true},
		// chi/gin :id producer style.
		{"colon param", "GET", "/api/v1/users/:id", "GET", "/api/v1/users/42", true},
		// trailing slash + query string ignored.
		{"trailing slash + query", "POST", "/api/v1/orders", "POST", "/api/v1/orders/?q=1", true},
		// full URL with host on the consumer side.
		{"host stripped", "GET", "/api/v1/users/{id}", "GET", "https://svc.internal/api/v1/users/7", true},
		// unknown consumer method matches any producer method.
		{"unknown method", "DELETE", "/api/v1/users/{id}", "", "/api/v1/users/9", true},
		// consumer-subtree: a concat URL "…/orders/" + id keeps only the trailing-slash
		// base literal; it must still hit a producer {id} route (cross-lang Go->Python).
		{"consumer concat trailing-slash vs param", "GET", "/api/v1/orders/{id}", "GET", "/api/v1/orders/", true},
		// but a consumer base must not match an unrelated longer LITERAL producer path.
		{"consumer subtree literal mismatch", "GET", "/api/v1/orders/export", "GET", "/api/v1/orders/", false},

		// clear non-matches:
		{"different path", "GET", "/api/v1/users/{id}", "GET", "/api/v1/orders/1", false},
		{"different method", "GET", "/api/v1/users/{id}", "POST", "/api/v1/users/1", false},
		{"different segment count", "GET", "/api/v1/users/{id}", "GET", "/api/v1/users/1/roles", false},
		{"literal segment mismatch", "GET", "/api/v1/users/{id}/roles", "GET", "/api/v1/users/1/teams", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EndpointMatch(c.prodMethod, c.prodPath, c.consMethod, c.consPath); got != c.want {
				t.Errorf("EndpointMatch(%q,%q,%q,%q) = %v, want %v",
					c.prodMethod, c.prodPath, c.consMethod, c.consPath, got, c.want)
			}
		})
	}
}

// TestImpactCrossRepo persists a producer repo (serving GET /api/v1/users/{id})
// and a consumer repo (calling GET /api/v1/users/1), then asserts Impact links
// them through the matcher.
func TestImpactCrossRepo(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	d, err := store.Open(ctx, store.Options{Kind: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Producer repo A: serves GET /api/v1/users/{id} from handlers/users.go.
	repoA, err := d.EnsureRepo(ctx, &graph.Repo{FullName: "org/service-a"})
	if err != nil {
		t.Fatalf("EnsureRepo A: %v", err)
	}
	snapA := &graph.Snapshot{ID: "snap-a", RepoID: repoA.ID, CommitSHA: "aaa"}
	prodRoutes := []graph.Route{{
		SnapshotID: snapA.ID, RepoFullName: "org/service-a",
		Method: "GET", PathPattern: "/api/v1/users/{id}",
		HandlerFile: "handlers/users.go", Role: "producer",
		Metadata: graph.JSONBMap{"handler_symbol": "GetUser"},
	}}
	if err := d.SaveSnapshot(ctx, snapA, nil, nil, nil, prodRoutes); err != nil {
		t.Fatalf("SaveSnapshot A: %v", err)
	}

	// Consumer repo B: calls GET /api/v1/users/1 from client/users_client.go.
	repoB, err := d.EnsureRepo(ctx, &graph.Repo{FullName: "org/service-b"})
	if err != nil {
		t.Fatalf("EnsureRepo B: %v", err)
	}
	snapB := &graph.Snapshot{ID: "snap-b", RepoID: repoB.ID, CommitSHA: "bbb"}
	consRoutes := []graph.Route{{
		SnapshotID: snapB.ID, RepoFullName: "org/service-b",
		Method: "GET", PathPattern: "/api/v1/users/1",
		HandlerFile: "client/users_client.go", Role: "consumer",
		Metadata: graph.JSONBMap{"calling_symbol": "FetchUser", "raw_url": "http://service-a/api/v1/users/1"},
	}}
	if err := d.SaveSnapshot(ctx, snapB, nil, nil, nil, consRoutes); err != nil {
		t.Fatalf("SaveSnapshot B: %v", err)
	}

	// Change the handler file in A → B must surface as an impacted consumer.
	res, err := Impact(ctx, d, "org/service-a", []string{"handlers/users.go"})
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
	if len(res.ServedRoutes) != 1 {
		t.Fatalf("ServedRoutes = %d, want 1", len(res.ServedRoutes))
	}
	if len(res.Impacted) != 1 {
		t.Fatalf("Impacted = %d, want 1: %+v", len(res.Impacted), res.Impacted)
	}
	hit := res.Impacted[0]
	if hit.Repo != "org/service-b" {
		t.Errorf("hit.Repo = %q, want org/service-b", hit.Repo)
	}
	if hit.CallingFile != "client/users_client.go" {
		t.Errorf("hit.CallingFile = %q", hit.CallingFile)
	}
	if hit.CallingSymbol != "FetchUser" {
		t.Errorf("hit.CallingSymbol = %q, want FetchUser", hit.CallingSymbol)
	}
	if len(res.ConsumerRepos) != 1 || res.ConsumerRepos[0] != "org/service-b" {
		t.Errorf("ConsumerRepos = %v, want [org/service-b]", res.ConsumerRepos)
	}

	// A changed path that touches no handler → no served routes, no impact.
	none, err := Impact(ctx, d, "org/service-a", []string{"README.md"})
	if err != nil {
		t.Fatalf("Impact(no-handler): %v", err)
	}
	if len(none.ServedRoutes) != 0 || len(none.Impacted) != 0 {
		t.Errorf("expected empty impact for non-handler change, got served=%d impacted=%d",
			len(none.ServedRoutes), len(none.Impacted))
	}

	// Empty changedPaths → use ALL producer routes (whole-repo contract).
	all, err := Consumers(ctx, d, "org/service-a")
	if err != nil {
		t.Fatalf("Consumers: %v", err)
	}
	if len(all.Impacted) != 1 {
		t.Errorf("Consumers Impacted = %d, want 1", len(all.Impacted))
	}

	// RouteContracts returns the producer routes.
	rc, err := RouteContracts(ctx, d, "org/service-a")
	if err != nil {
		t.Fatalf("RouteContracts: %v", err)
	}
	if len(rc) != 1 || rc[0].PathPattern != "/api/v1/users/{id}" {
		t.Errorf("RouteContracts = %+v", rc)
	}

	// Unknown repo → ErrRepoNotFound.
	if _, err := Impact(ctx, d, "org/does-not-exist", nil); err != ErrRepoNotFound {
		t.Errorf("Impact(unknown) err = %v, want ErrRepoNotFound", err)
	}
}
