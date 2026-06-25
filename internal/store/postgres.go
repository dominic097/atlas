package store

import (
	"context"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// postgresDriver is the HOSTED tier StorageDriver.
//
// TODO(engine): in the real build this is //go:build hosted and backed by
// github.com/jmoiron/sqlx + github.com/lib/pq, lifting aziron-pulse
// internal/repository/code_intelligence_repository.go nearly verbatim (schema
// renamed pulse->atlas). It provides a durable SKIP LOCKED job queue,
// org-wide cross-repo scope, and concurrent writes
// (Capabilities.ConcurrentWrite=true). The pq.Array / ANY($n) idioms must be
// abstracted behind a driver IN-clause helper so the SQLite driver can use a
// temp-table join instead.
type postgresDriver struct {
	dsn string
}

func openPostgres(ctx context.Context, dsn string) (StorageDriver, error) {
	// TODO(engine): sqlx.Open("postgres", dsn), run Migrate against schema atlas.
	return &postgresDriver{dsn: dsn}, nil
}

func (d *postgresDriver) Migrate(ctx context.Context) error { return ErrNotImplemented }
func (d *postgresDriver) Dialect() string                   { return "postgres" }
func (d *postgresDriver) Capabilities() Capabilities {
	return Capabilities{DurableQueue: true, CrossScope: true, ConcurrentWrite: true, PushReindex: true}
}
func (d *postgresDriver) Close() error { return nil }

func (d *postgresDriver) EnsureRepo(ctx context.Context, r *graph.Repo) (*graph.Repo, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) ListRepos(ctx context.Context, scope string) ([]graph.Repo, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) SaveSnapshot(ctx context.Context, s *graph.Snapshot, files []graph.File,
	symbols []graph.CodeSymbol, edges []graph.DependencyEdge, routes []graph.Route) error {
	return ErrNotImplemented
}
func (d *postgresDriver) LatestSnapshot(ctx context.Context, repoID string) (*graph.Snapshot, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) ListSnapshots(ctx context.Context, repoID string, limit int) ([]graph.Snapshot, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) ListSymbols(ctx context.Context, snapshotID string) ([]graph.CodeSymbol, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) ListEdges(ctx context.Context, snapshotID string) ([]graph.DependencyEdge, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) ListRoutes(ctx context.Context, snapshotID, role string) ([]graph.Route, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) SymbolsByName(ctx context.Context, snapshotID, name string) ([]graph.CodeSymbol, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) SymbolsByPath(ctx context.Context, snapshotID, path string) ([]graph.CodeSymbol, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) CallEdgesByToRefs(ctx context.Context, snapshotID string, toRefs []string) ([]graph.DependencyEdge, error) {
	return nil, ErrNotImplemented
}
func (d *postgresDriver) CallEdgesByFromSymbols(ctx context.Context, snapshotID string, fromSymbols []string) ([]graph.DependencyEdge, error) {
	return nil, ErrNotImplemented
}
