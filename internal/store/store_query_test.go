package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// TestIndexedQueries opens a temp sqlite, saves a tiny 2-symbol/1-edge graph,
// and asserts the indexed read paths (SymbolsByName / CallEdgesByToRefs) return
// the same shape — node_id + decoded metadata — as the List* readers.
func TestIndexedQueries(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")

	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const snapID = "snap-1"
	snap := &graph.Snapshot{ID: snapID, RepoID: "repo-1", CommitSHA: "deadbeef"}

	symbols := []graph.CodeSymbol{
		{
			ID: "sym-app", SnapshotID: snapID, NodeID: "node-app",
			Path: "app.go", Language: "go", Kind: "method", Name: "addTask",
			Metadata: graph.JSONBMap{"recv_type": "TodoApp"},
		},
		{
			ID: "sym-engine", SnapshotID: snapID, NodeID: "node-engine",
			Path: "engine.go", Language: "go", Kind: "method", Name: "addTask",
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
	}
	files := []graph.File{
		{ID: "file-app", SnapshotID: snapID, Path: "app.go", Language: "go", Imports: []string{"context", "fmt"}},
		{ID: "file-engine", SnapshotID: snapID, Path: "engine.go", Language: "go", Imports: []string{"net/http"}},
	}
	routes := []graph.Route{
		{ID: "route-symbol", SnapshotID: snapID, Method: "GET", PathPattern: "/tasks", HandlerFile: "handler.go", Role: "producer", Metadata: graph.JSONBMap{"handler_symbol": "addTask"}},
		{ID: "route-file", SnapshotID: snapID, Method: "POST", PathPattern: "/tasks", HandlerFile: "engine.go", Role: "producer"},
		{ID: "route-consumer", SnapshotID: snapID, Method: "GET", PathPattern: "/external", HandlerFile: "engine.go", Role: "consumer"},
		{ID: "route-other", SnapshotID: snapID, Method: "GET", PathPattern: "/other", HandlerFile: "other.go", Role: "producer", Metadata: graph.JSONBMap{"handler_symbol": "otherTask"}},
	}

	if err := d.SaveSnapshot(ctx, snap, files, symbols, edges, routes); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// SymbolsByName: exact match returns both same-named methods, metadata intact.
	byName, err := d.SymbolsByName(ctx, snapID, "addTask")
	if err != nil {
		t.Fatalf("SymbolsByName: %v", err)
	}
	if len(byName) != 2 {
		t.Fatalf("SymbolsByName: got %d symbols, want 2", len(byName))
	}
	recvSeen := map[string]bool{}
	for _, s := range byName {
		if s.Name != "addTask" {
			t.Errorf("SymbolsByName: unexpected name %q", s.Name)
		}
		if s.NodeID == "" {
			t.Errorf("SymbolsByName: node_id not populated for %s", s.ID)
		}
		rt, _ := s.Metadata["recv_type"].(string)
		recvSeen[rt] = true
	}
	if !recvSeen["TodoApp"] || !recvSeen["Engine"] {
		t.Errorf("SymbolsByName: recv_type metadata missing, saw %v", recvSeen)
	}

	summaryReader, ok := d.(interface {
		SymbolSummaryByName(context.Context, string, string) (graph.CodeSymbol, int, bool, error)
		SymbolPathsByName(context.Context, string, string) ([]string, error)
	})
	if !ok {
		t.Fatal("sqlite driver does not expose symbol summary readers")
	}
	first, total, found, err := summaryReader.SymbolSummaryByName(ctx, snapID, "addTask")
	if err != nil {
		t.Fatalf("SymbolSummaryByName: %v", err)
	}
	if !found || first.ID != "sym-app" || total != 2 {
		t.Fatalf("SymbolSummaryByName = first:%+v total:%d found:%v, want sym-app/2/true", first, total, found)
	}
	paths, err := summaryReader.SymbolPathsByName(ctx, snapID, "addTask")
	if err != nil {
		t.Fatalf("SymbolPathsByName: %v", err)
	}
	if len(paths) != 2 || paths[0] != "app.go" || paths[1] != "engine.go" {
		t.Fatalf("SymbolPathsByName = %v, want [app.go engine.go]", paths)
	}

	// A miss returns nothing.
	none, err := d.SymbolsByName(ctx, snapID, "nope")
	if err != nil {
		t.Fatalf("SymbolsByName(miss): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("SymbolsByName(miss): got %d, want 0", len(none))
	}

	// SymbolsByPath: file-scoped.
	byPath, err := d.SymbolsByPath(ctx, snapID, "app.go")
	if err != nil {
		t.Fatalf("SymbolsByPath: %v", err)
	}
	if len(byPath) != 1 || byPath[0].ID != "sym-app" {
		t.Fatalf("SymbolsByPath: got %+v, want [sym-app]", byPath)
	}

	// FilesByPaths: file-scoped, dedupes duplicate input, preserves imports.
	byFilePath, err := d.FilesByPaths(ctx, snapID, []string{"engine.go", "app.go", "app.go", "missing.go"})
	if err != nil {
		t.Fatalf("FilesByPaths: %v", err)
	}
	if len(byFilePath) != 2 {
		t.Fatalf("FilesByPaths: got %d files, want 2", len(byFilePath))
	}
	filesSeen := map[string][]string{}
	for _, f := range byFilePath {
		filesSeen[f.Path] = f.Imports
	}
	if got := filesSeen["app.go"]; len(got) != 2 || got[0] != "context" || got[1] != "fmt" {
		t.Errorf("FilesByPaths(app.go): imports = %v, want [context fmt]", got)
	}
	if got := filesSeen["engine.go"]; len(got) != 1 || got[0] != "net/http" {
		t.Errorf("FilesByPaths(engine.go): imports = %v, want [net/http]", got)
	}
	emptyFiles, err := d.FilesByPaths(ctx, snapID, nil)
	if err != nil {
		t.Fatalf("FilesByPaths(nil): %v", err)
	}
	if len(emptyFiles) != 0 {
		t.Errorf("FilesByPaths(nil): got %d, want 0", len(emptyFiles))
	}

	routeReader, ok := d.(interface {
		RoutesForSymbol(context.Context, string, string, []string) ([]graph.Route, error)
		RouteCountForSymbol(context.Context, string, string, []string) (int, error)
	})
	if !ok {
		t.Fatal("sqlite driver does not expose RoutesForSymbol")
	}
	matchedRoutes, err := routeReader.RoutesForSymbol(ctx, snapID, "addTask", []string{"engine.go"})
	if err != nil {
		t.Fatalf("RoutesForSymbol: %v", err)
	}
	if len(matchedRoutes) != 2 {
		t.Fatalf("RoutesForSymbol: got %d routes, want 2 (%+v)", len(matchedRoutes), matchedRoutes)
	}
	routePaths := map[string]bool{}
	for _, route := range matchedRoutes {
		routePaths[route.PathPattern] = true
		if route.Role != "producer" {
			t.Errorf("RoutesForSymbol returned non-producer route: %+v", route)
		}
	}
	if !routePaths["/tasks"] {
		t.Errorf("RoutesForSymbol missing /tasks route: %+v", matchedRoutes)
	}
	routeCount, err := routeReader.RouteCountForSymbol(ctx, snapID, "addTask", []string{"engine.go"})
	if err != nil {
		t.Fatalf("RouteCountForSymbol: %v", err)
	}
	if routeCount != len(matchedRoutes) {
		t.Fatalf("RouteCountForSymbol = %d, want %d", routeCount, len(matchedRoutes))
	}

	// CallEdgesByToRefs: index hit returns the edge with metadata; a ref not in
	// the set is excluded.
	callEdges, err := d.CallEdgesByToRefs(ctx, snapID, []string{"app.addTask", "time.Parse"})
	if err != nil {
		t.Fatalf("CallEdgesByToRefs: %v", err)
	}
	if len(callEdges) != 1 {
		t.Fatalf("CallEdgesByToRefs: got %d edges, want 1", len(callEdges))
	}
	e := callEdges[0]
	if e.Kind != graph.EdgeCalls {
		t.Errorf("CallEdgesByToRefs: kind = %q, want calls", e.Kind)
	}
	if e.ToRef != "app.addTask" {
		t.Errorf("CallEdgesByToRefs: to_ref = %q, want app.addTask", e.ToRef)
	}
	if got := e.Metadata["recv_type"]; got != "TodoApp" {
		t.Errorf("CallEdgesByToRefs: recv_type metadata = %v, want TodoApp", got)
	}
	if got := e.Metadata["qualified_ref"]; got != "app.addTask" {
		t.Errorf("CallEdgesByToRefs: qualified_ref metadata = %v, want app.addTask", got)
	}

	// Empty input is a no-op.
	empty, err := d.CallEdgesByToRefs(ctx, snapID, nil)
	if err != nil {
		t.Fatalf("CallEdgesByToRefs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("CallEdgesByToRefs(nil): got %d, want 0", len(empty))
	}
}
