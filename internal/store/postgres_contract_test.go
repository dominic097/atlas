package store

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
	_ "github.com/lib/pq"
)

// TestPostgresContract exercises the full hosted StorageDriver round-trip against
// a LIVE Postgres. It is gated on ATLAS_PG_DSN so plain `go test ./internal/store/`
// (the SQLite suite) never touches a database. To run it:
//
//	ATLAS_PG_DSN="postgres://atlas:atlas123@localhost:5432/atlas_hosted?sslmode=disable" \
//	  CGO_ENABLED=1 go test ./internal/store/ -run TestPostgres -v
//
// It drops + recreates the atlas tables, saves a 2-repo / 2-snapshot graph with
// edges and routes across two tenant scopes, then asserts: EnsureRepo upsert
// (baseline preservation + stable id), ListRepos(scope) tenant isolation, and
// that every indexed/list read path round-trips node_id + decoded metadata +
// imports verbatim — the same shape the SQLite tier returns.
func TestPostgresContract(t *testing.T) {
	dsn := os.Getenv("ATLAS_PG_DSN")
	if dsn == "" {
		t.Skip("ATLAS_PG_DSN not set; skipping hosted Postgres contract test")
	}
	ctx := context.Background()

	// Drop atlas tables first so Migrate starts from a clean slate.
	dropPGTables(t, dsn)

	d, err := Open(ctx, Options{Kind: "postgres", PostgresDSN: dsn})
	if err != nil {
		t.Fatalf("Open(postgres): %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	if got := d.Dialect(); got != "postgres" {
		t.Fatalf("Dialect = %q, want postgres", got)
	}
	if caps := d.Capabilities(); !caps.DurableQueue || !caps.CrossScope || !caps.ConcurrentWrite || !caps.PushReindex {
		t.Fatalf("Capabilities = %+v, want all true", caps)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Idempotent re-migrate must be a no-op.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate (re-run): %v", err)
	}

	// ---- EnsureRepo upsert + tenant scoping --------------------------------

	const scopeA, scopeB = "tenant-A", "tenant-B"

	// First ensure: full payload in tenant A.
	repoA1, err := d.EnsureRepo(ctx, &graph.Repo{
		FullName: "org/web", Root: "/srv/web", DefaultBranch: "main",
		Status: graph.StatusReady, Scope: scopeA,
		Languages: map[string]int{"go": 10, "ts": 5},
	})
	if err != nil {
		t.Fatalf("EnsureRepo(A1): %v", err)
	}
	if repoA1.ID == "" {
		t.Fatal("EnsureRepo(A1): empty id")
	}

	// Re-ensure same (scope, full_name) with a SPARSE payload: must reuse the id
	// and must NOT clobber default_branch / languages (baseline preservation).
	repoA2, err := d.EnsureRepo(ctx, &graph.Repo{FullName: "org/web", Scope: scopeA})
	if err != nil {
		t.Fatalf("EnsureRepo(A2): %v", err)
	}
	if repoA2.ID != repoA1.ID {
		t.Fatalf("EnsureRepo upsert: id changed %q -> %q", repoA1.ID, repoA2.ID)
	}

	// A second repo in tenant A, and one in tenant B (same full_name, different scope).
	if _, err := d.EnsureRepo(ctx, &graph.Repo{
		FullName: "org/api", Root: "/srv/api", DefaultBranch: "main",
		Status: graph.StatusReady, Scope: scopeA,
	}); err != nil {
		t.Fatalf("EnsureRepo(A api): %v", err)
	}
	repoB_web, err := d.EnsureRepo(ctx, &graph.Repo{
		FullName: "org/web", Root: "/srv/web", DefaultBranch: "develop",
		Status: graph.StatusReady, Scope: scopeB,
	})
	if err != nil {
		t.Fatalf("EnsureRepo(B web): %v", err)
	}
	// Same full_name across scopes must be DISTINCT repos.
	if repoB_web.ID == repoA1.ID {
		t.Fatalf("cross-tenant repos collapsed to one id %q", repoB_web.ID)
	}

	// ListRepos(scopeA) sees exactly the 2 tenant-A repos; baseline preserved.
	reposA, err := d.ListRepos(ctx, scopeA)
	if err != nil {
		t.Fatalf("ListRepos(A): %v", err)
	}
	if len(reposA) != 2 {
		t.Fatalf("ListRepos(A): got %d repos, want 2 (%+v)", len(reposA), reposA)
	}
	var webA *graph.Repo
	for i := range reposA {
		if reposA[i].FullName == "org/web" {
			webA = &reposA[i]
		}
		if reposA[i].Scope != scopeA {
			t.Errorf("ListRepos(A): leaked scope %q", reposA[i].Scope)
		}
	}
	if webA == nil {
		t.Fatal("ListRepos(A): org/web missing")
	}
	if webA.DefaultBranch != "main" {
		t.Errorf("baseline preservation: default_branch = %q, want main (sparse re-ensure clobbered it)", webA.DefaultBranch)
	}
	if webA.Languages["go"] != 10 || webA.Languages["ts"] != 5 {
		t.Errorf("baseline preservation: languages = %v, want {go:10 ts:5}", webA.Languages)
	}

	// ListRepos(scopeB) is isolated to tenant B.
	reposB, err := d.ListRepos(ctx, scopeB)
	if err != nil {
		t.Fatalf("ListRepos(B): %v", err)
	}
	if len(reposB) != 1 || reposB[0].ID != repoB_web.ID {
		t.Fatalf("ListRepos(B): got %+v, want [%s]", reposB, repoB_web.ID)
	}

	// Empty scope = cross-scope (all tenants) — hosted CrossScope capability.
	reposAll, err := d.ListRepos(ctx, "")
	if err != nil {
		t.Fatalf("ListRepos(all): %v", err)
	}
	if len(reposAll) != 3 {
		t.Fatalf("ListRepos(all): got %d, want 3", len(reposAll))
	}

	// ---- SaveSnapshot graph round-trip -------------------------------------

	const snapID = "snap-pg-1"
	snap := &graph.Snapshot{
		ID: snapID, RepoID: repoA1.ID, CommitSHA: "deadbeef", Branch: "main",
		Metadata: graph.JSONBMap{"indexer": "atlas", "version": 2},
	}
	files := []graph.File{
		{ID: "file-app", SnapshotID: snapID, Path: "app.go", Language: "go", SizeBytes: 1234, Hash: "h1", Imports: []string{"fmt", "context"}},
		{ID: "file-engine", SnapshotID: snapID, Path: "engine.go", Language: "go", SizeBytes: 99, Hash: "h2"},
	}
	symbols := []graph.CodeSymbol{
		{
			ID: "sym-app", SnapshotID: snapID, NodeID: "node-app", RepoID: repoA1.ID,
			Path: "app.go", Language: "go", Kind: "method", Name: "addTask",
			StartLine: 10, EndLine: 20,
			Metadata: graph.JSONBMap{"recv_type": "TodoApp"},
		},
		{
			ID: "sym-engine", SnapshotID: snapID, NodeID: "node-engine", RepoID: repoA1.ID,
			Path: "engine.go", Language: "go", Kind: "method", Name: "addTask",
			StartLine: 5, EndLine: 9,
			Metadata: graph.JSONBMap{"recv_type": "Engine"},
		},
	}
	edges := []graph.DependencyEdge{
		{
			ID: "edge-1", SnapshotID: snapID,
			FromFile: "main.go", FromSymbol: "main", ToRef: "app.addTask",
			Kind: graph.EdgeCalls, Language: "go", Line: 7,
			Metadata: graph.JSONBMap{"qualified_ref": "app.addTask", "recv_type": "TodoApp"},
		},
		{
			ID: "edge-2", SnapshotID: snapID,
			FromFile: "app.go", FromSymbol: "app.addTask", ToRef: "engine.addTask",
			Kind: graph.EdgeCalls, Language: "go", Line: 15,
			Metadata: graph.JSONBMap{"qualified_ref": "engine.addTask"},
		},
	}
	routes := []graph.Route{
		{ID: "route-1", SnapshotID: snapID, RepoFullName: "org/web", Method: "POST", PathPattern: "/tasks", HandlerFile: "app.go", Role: "producer", Source: "ast", Confidence: "high", Metadata: graph.JSONBMap{"auth": "jwt"}},
		{ID: "route-2", SnapshotID: snapID, RepoFullName: "org/web", Method: "GET", PathPattern: "/health", Role: "consumer"},
	}

	if err := d.SaveSnapshot(ctx, snap, files, symbols, edges, routes); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Second snapshot on the SAME repo (different commit) so ListSnapshots / Latest
	// can be exercised.
	snap2 := &graph.Snapshot{ID: "snap-pg-2", RepoID: repoA1.ID, CommitSHA: "cafef00d", Branch: "main"}
	if err := d.SaveSnapshot(ctx, snap2, nil, nil, nil, nil); err != nil {
		t.Fatalf("SaveSnapshot(2): %v", err)
	}

	// Idempotency: re-saving the SAME (repo_id, commit_sha) must reuse the id and
	// rebuild children (count stays 2, not 4). Carry the same metadata so the
	// upsert (metadata = excluded.metadata) preserves it for the round-trip check.
	reSnap := &graph.Snapshot{RepoID: repoA1.ID, CommitSHA: "deadbeef", Metadata: graph.JSONBMap{"indexer": "atlas", "version": 2}}
	if err := d.SaveSnapshot(ctx, reSnap, files, symbols, edges, routes); err != nil {
		t.Fatalf("SaveSnapshot(idempotent): %v", err)
	}
	if reSnap.ID != snapID {
		t.Fatalf("idempotent SaveSnapshot: id = %q, want %q", reSnap.ID, snapID)
	}

	// LatestSnapshot picks the most recent created_at (snap2, saved last among the
	// two distinct commits — but the idempotent re-save of snap touched only its
	// children, not created_at). Just assert a snapshot comes back and counts are sane.
	latest, err := d.LatestSnapshot(ctx, repoA1.ID)
	if err != nil {
		t.Fatalf("LatestSnapshot: %v", err)
	}
	if latest == nil {
		t.Fatal("LatestSnapshot: nil")
	}

	snaps, err := d.ListSnapshots(ctx, repoA1.ID, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("ListSnapshots: got %d, want 2 (idempotent re-save must not create a 3rd)", len(snaps))
	}

	// Metadata round-trips on the original snapshot.
	var origSnap *graph.Snapshot
	for i := range snaps {
		if snaps[i].ID == snapID {
			origSnap = &snaps[i]
		}
	}
	if origSnap == nil {
		t.Fatal("ListSnapshots: snap-pg-1 missing")
	}
	if origSnap.FileCount != 2 || origSnap.SymbolCount != 2 || origSnap.EdgeCount != 2 || origSnap.RouteCount != 2 {
		t.Errorf("snapshot counts = files:%d sym:%d edge:%d route:%d, want 2/2/2/2",
			origSnap.FileCount, origSnap.SymbolCount, origSnap.EdgeCount, origSnap.RouteCount)
	}
	if origSnap.Metadata["indexer"] != "atlas" {
		t.Errorf("snapshot metadata = %v, want indexer=atlas", origSnap.Metadata)
	}

	// ---- ListSymbols / ListEdges / ListRoutes ------------------------------

	allSyms, err := d.ListSymbols(ctx, snapID)
	if err != nil {
		t.Fatalf("ListSymbols: %v", err)
	}
	if len(allSyms) != 2 {
		t.Fatalf("ListSymbols: got %d, want 2", len(allSyms))
	}
	for _, s := range allSyms {
		if s.NodeID == "" {
			t.Errorf("ListSymbols: node_id empty for %s", s.ID)
		}
		if _, ok := s.Metadata["recv_type"]; !ok {
			t.Errorf("ListSymbols: recv_type metadata missing for %s", s.ID)
		}
	}

	allEdges, err := d.ListEdges(ctx, snapID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(allEdges) != 2 {
		t.Fatalf("ListEdges: got %d, want 2", len(allEdges))
	}

	producerRoutes, err := d.ListRoutes(ctx, snapID, "producer")
	if err != nil {
		t.Fatalf("ListRoutes(producer): %v", err)
	}
	if len(producerRoutes) != 1 || producerRoutes[0].PathPattern != "/tasks" {
		t.Fatalf("ListRoutes(producer): got %+v, want [/tasks]", producerRoutes)
	}
	if producerRoutes[0].Metadata["auth"] != "jwt" {
		t.Errorf("ListRoutes: route metadata = %v, want auth=jwt", producerRoutes[0].Metadata)
	}
	allRoutes, err := d.ListRoutes(ctx, snapID, "")
	if err != nil {
		t.Fatalf("ListRoutes(all): %v", err)
	}
	if len(allRoutes) != 2 {
		t.Fatalf("ListRoutes(all): got %d, want 2", len(allRoutes))
	}

	// ---- ListFiles (TEXT[] imports verbatim) -------------------------------

	fileRows, err := d.ListFiles(ctx, snapID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(fileRows) != 2 {
		t.Fatalf("ListFiles: got %d, want 2", len(fileRows))
	}
	var appFile *graph.File
	for i := range fileRows {
		if fileRows[i].Path == "app.go" {
			appFile = &fileRows[i]
		}
	}
	if appFile == nil {
		t.Fatal("ListFiles: app.go missing")
	}
	gotImports := append([]string(nil), appFile.Imports...)
	sort.Strings(gotImports)
	if len(gotImports) != 2 || gotImports[0] != "context" || gotImports[1] != "fmt" {
		t.Errorf("ListFiles: imports = %v, want [context fmt]", gotImports)
	}

	// ---- SymbolsByName / SymbolsByNames / SymbolsByPath --------------------

	byName, err := d.SymbolsByName(ctx, snapID, "addTask")
	if err != nil {
		t.Fatalf("SymbolsByName: %v", err)
	}
	if len(byName) != 2 {
		t.Fatalf("SymbolsByName: got %d, want 2", len(byName))
	}
	recvSeen := map[string]bool{}
	for _, s := range byName {
		if s.NodeID == "" {
			t.Errorf("SymbolsByName: node_id empty for %s", s.ID)
		}
		rt, _ := s.Metadata["recv_type"].(string)
		recvSeen[rt] = true
	}
	if !recvSeen["TodoApp"] || !recvSeen["Engine"] {
		t.Errorf("SymbolsByName: recv_type metadata missing, saw %v", recvSeen)
	}

	if miss, err := d.SymbolsByName(ctx, snapID, "nope"); err != nil || len(miss) != 0 {
		t.Errorf("SymbolsByName(miss): got %d err=%v, want 0/nil", len(miss), err)
	}

	byNames, err := d.SymbolsByNames(ctx, snapID, []string{"addTask", "ghost"})
	if err != nil {
		t.Fatalf("SymbolsByNames: %v", err)
	}
	if len(byNames) != 2 {
		t.Fatalf("SymbolsByNames: got %d, want 2", len(byNames))
	}
	if empty, err := d.SymbolsByNames(ctx, snapID, nil); err != nil || len(empty) != 0 {
		t.Errorf("SymbolsByNames(nil): got %d err=%v, want 0/nil", len(empty), err)
	}

	byPath, err := d.SymbolsByPath(ctx, snapID, "app.go")
	if err != nil {
		t.Fatalf("SymbolsByPath: %v", err)
	}
	if len(byPath) != 1 || byPath[0].ID != "sym-app" {
		t.Fatalf("SymbolsByPath: got %+v, want [sym-app]", byPath)
	}

	// ---- CallEdgesByToRefs / CallEdgesByFromSymbols ------------------------

	callTo, err := d.CallEdgesByToRefs(ctx, snapID, []string{"app.addTask", "time.Parse"})
	if err != nil {
		t.Fatalf("CallEdgesByToRefs: %v", err)
	}
	if len(callTo) != 1 || callTo[0].ToRef != "app.addTask" {
		t.Fatalf("CallEdgesByToRefs: got %+v, want [app.addTask]", callTo)
	}
	if callTo[0].Metadata["recv_type"] != "TodoApp" || callTo[0].Metadata["qualified_ref"] != "app.addTask" {
		t.Errorf("CallEdgesByToRefs: metadata = %v, want recv_type=TodoApp qualified_ref=app.addTask", callTo[0].Metadata)
	}
	if empty, err := d.CallEdgesByToRefs(ctx, snapID, nil); err != nil || len(empty) != 0 {
		t.Errorf("CallEdgesByToRefs(nil): got %d err=%v, want 0/nil", len(empty), err)
	}

	callFrom, err := d.CallEdgesByFromSymbols(ctx, snapID, []string{"app.addTask"})
	if err != nil {
		t.Fatalf("CallEdgesByFromSymbols: %v", err)
	}
	if len(callFrom) != 1 || callFrom[0].ToRef != "engine.addTask" {
		t.Fatalf("CallEdgesByFromSymbols: got %+v, want [edge-2 -> engine.addTask]", callFrom)
	}
	if empty, err := d.CallEdgesByFromSymbols(ctx, snapID, nil); err != nil || len(empty) != 0 {
		t.Errorf("CallEdgesByFromSymbols(nil): got %d err=%v, want 0/nil", len(empty), err)
	}
}

// dropPGTables tears down the atlas tables in the dedicated atlas_hosted database
// so each contract run Migrates from a clean slate. It opens its own connection
// (independent of the driver under test) and ignores "table does not exist".
func dropPGTables(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("dropPGTables: open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE IF EXISTS routes, edges, symbols, files, snapshots, repos CASCADE`); err != nil {
		t.Fatalf("dropPGTables: drop: %v", err)
	}
}
