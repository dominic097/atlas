package store

import (
	"context"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// sqliteDriver is the LOCAL tier StorageDriver.
//
// TODO(engine): back this with github.com/mattn/go-sqlite3 (CGO) — or
// modernc.org/sqlite (pure-Go, -tags purego) — opening <repo>/.atlas/atlas.db
// with PRAGMA journal_mode=WAL; synchronous=NORMAL; busy_timeout=5000. Schema is
// //go:embed schema.sql, applied by Migrate on first open. Writes serialize
// through a single process mutex (Capabilities.ConcurrentWrite=false).
type sqliteDriver struct {
	path string
}

func openSQLite(ctx context.Context, path string) (StorageDriver, error) {
	if path == "" {
		path = "./.atlas/atlas.db"
	}
	// TODO(engine): sql.Open("sqlite3", path), apply pragmas, run Migrate.
	return &sqliteDriver{path: path}, nil
}

func (d *sqliteDriver) Migrate(ctx context.Context) error { return ErrNotImplemented }
func (d *sqliteDriver) Dialect() string                   { return "sqlite" }
func (d *sqliteDriver) Capabilities() Capabilities {
	return Capabilities{DurableQueue: false, CrossScope: false, ConcurrentWrite: false, PushReindex: false}
}
func (d *sqliteDriver) Close() error { return nil }

func (d *sqliteDriver) EnsureRepo(ctx context.Context, r *graph.Repo) (*graph.Repo, error) {
	return nil, ErrNotImplemented
}
func (d *sqliteDriver) ListRepos(ctx context.Context, scope string) ([]graph.Repo, error) {
	return nil, ErrNotImplemented
}
func (d *sqliteDriver) SaveSnapshot(ctx context.Context, s *graph.Snapshot, files []graph.File,
	symbols []graph.CodeSymbol, edges []graph.DependencyEdge, routes []graph.Route) error {
	return ErrNotImplemented
}
func (d *sqliteDriver) LatestSnapshot(ctx context.Context, repoID string) (*graph.Snapshot, error) {
	return nil, ErrNotImplemented
}
func (d *sqliteDriver) ListSnapshots(ctx context.Context, repoID string, limit int) ([]graph.Snapshot, error) {
	return nil, ErrNotImplemented
}
func (d *sqliteDriver) ListSymbols(ctx context.Context, snapshotID string) ([]graph.CodeSymbol, error) {
	return nil, ErrNotImplemented
}
func (d *sqliteDriver) ListEdges(ctx context.Context, snapshotID string) ([]graph.DependencyEdge, error) {
	return nil, ErrNotImplemented
}
func (d *sqliteDriver) ListRoutes(ctx context.Context, snapshotID, role string) ([]graph.Route, error) {
	return nil, ErrNotImplemented
}
