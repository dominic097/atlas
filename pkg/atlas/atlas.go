// Package atlas is the public, import-stable SDK facade for the embeddable Atlas
// engine (consumption surface S4a). Third parties import this package and run
// Atlas in-process; the heavy internals live under internal/ and are not
// importable, so they can be refactored without breaking SemVer.
//
//	eng, err := atlas.New(context.Background(), atlas.WithSQLite("./.atlas/atlas.db"))
//	if err != nil { ... }
//	defer eng.Close()
//	res, err := eng.Index(ctx, atlas.IndexInput{Path: "."})
package atlas

import (
	"context"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/engine"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/query"
)

// Re-export the canonical engine types so SDK consumers never reach into internal/.
type (
	// Engine is the in-process Atlas code-knowledge engine. Every consumption
	// surface (CLI, HTTP, MCP) is a thin adapter over this interface.
	Engine = engine.Engine

	IndexInput  = engine.IndexInput
	IndexResult = engine.IndexResult

	SearchInput  = engine.SearchInput
	SearchResult = engine.SearchResult

	ImpactInput  = engine.ImpactInput
	ImpactResult = engine.ImpactResult

	StatusInput  = engine.StatusInput
	StatusResult = engine.StatusResult

	CallersInput  = engine.CallersInput
	CallersResult = engine.CallersResult
	SymbolInput   = engine.SymbolInput
	SymbolResult  = engine.SymbolResult
	SymbolDef     = engine.SymbolDef
	SymbolRef     = engine.SymbolRef

	// Local navigation ops (deterministic, single-repo).
	NeighborsInput       = engine.NeighborsInput
	NeighborsResult      = engine.NeighborsResult
	PathInput            = engine.PathInput
	PathResult           = engine.PathResult
	RefsInput            = engine.RefsInput
	RefsResult           = engine.RefsResult
	ExplainInput         = engine.ExplainInput
	ExplainResult        = engine.ExplainResult
	ExplainDef           = engine.ExplainDef
	ExplainRoute         = engine.ExplainRoute
	CoverageInput        = engine.CoverageInput
	CoverageResult       = engine.CoverageResult
	CoverageImportInput  = engine.CoverageImportInput
	CoverageImportResult = engine.CoverageImportResult

	GraphExportInput  = engine.GraphExportInput
	GraphExportResult = engine.GraphExportResult

	HistoryInput       = engine.HistoryInput
	HistoryResult      = engine.HistoryResult
	SnapshotInfo       = engine.SnapshotInfo
	SnapshotDiffInput  = engine.SnapshotDiffInput
	SnapshotDiffResult = engine.SnapshotDiffResult
	SymbolChange       = query.SymbolChange
	EdgeChange         = query.EdgeChange

	// Cross-repo (the USP): producer route contracts ↔ consumer HTTP calls.
	CrossRepoImpactInput  = engine.CrossRepoImpactInput
	CrossRepoImpactResult = engine.CrossRepoImpactResult
	ConsumersInput        = engine.ConsumersInput
	ConsumersResult       = engine.ConsumersResult
	RouteContractsInput   = engine.RouteContractsInput
	RouteContractsResult  = engine.RouteContractsResult
	RouteContract         = engine.RouteContract
	ConsumerHit           = engine.ConsumerHit
)

// ErrNotImplemented is returned by stub operations in this scaffold.
var ErrNotImplemented = engine.ErrNotImplemented

// Config carries construction options for an Engine.
type Config = engine.Config

// Option mutates a Config.
type Option = engine.Option

// WithSQLite selects the local SQLite StorageDriver (default tier).
func WithSQLite(path string) Option { return engine.WithSQLite(path) }

// WithPostgres selects the hosted Postgres StorageDriver.
func WithPostgres(dsn string) Option { return engine.WithPostgres(dsn) }

// WithScope isolates the engine to a tenant/org scope: every repo read and write
// is confined to that scope, and cross-repo matching only spans the same tenant.
// Empty scope ("") keeps single-tenant / all-repo behaviour (the local default).
// It is orthogonal to the tier — combine it with WithSQLite or WithPostgres.
func WithScope(scope string) Option { return engine.WithScope(scope) }

// New builds an Engine. With zero options it is the LOCAL tier: embedded SQLite
// under ./.atlas, lexical search on, vectors off, code never leaves the machine.
func New(ctx context.Context, opts ...Option) (Engine, error) {
	return engine.New(ctx, opts...)
}
