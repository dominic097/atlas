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
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/query"
)

// Re-export the canonical engine types so SDK consumers never reach into internal/.
type (
	// Engine is the in-process Atlas code-knowledge engine. Every consumption
	// surface (CLI, HTTP, MCP) is a thin adapter over this interface.
	Engine = engine.Engine

	IndexInput  = engine.IndexInput
	IndexResult = engine.IndexResult

	SearchInput   = engine.SearchInput
	SearchResult  = engine.SearchResult
	SearchHit     = engine.SearchHit
	ContextInput  = engine.ContextInput
	ContextResult = engine.ContextResult
	ContextFile   = engine.ContextFile
	ContextSymbol = engine.ContextSymbol
	ContextEdge   = engine.ContextEdge
	ContextBudget = engine.ContextBudget

	// Optional, gated semantic search (off by default; needs vectors enabled).
	SemanticSearchInput  = engine.SemanticSearchInput
	SemanticSearchResult = engine.SemanticSearchResult
	SymbolEmbedding      = graph.SymbolEmbedding
	ScoredSymbol         = graph.ScoredSymbol

	ImpactInput  = engine.ImpactInput
	ImpactResult = engine.ImpactResult
	FileImpact   = engine.FileImpact

	StatusInput  = engine.StatusInput
	StatusResult = engine.StatusResult

	LinkInput  = engine.LinkInput
	LinkResult = engine.LinkResult

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

// WithVectors enables the OPTIONAL semantic layer: the index pass builds
// per-symbol embeddings and query-time SemanticSearch runs vector nearest-neighbor
// instead of degrading to lexical. Off by default — the deterministic core is
// unchanged. ATLAS_ENABLE_VECTORS=1 sets the same flag from the environment.
func WithVectors(enabled bool) Option { return engine.WithVectors(enabled) }

// WithContextBudget overrides the default `context` op budgets (Limit/MaxFiles/
// MaxEdges/MaxDepth). Only non-zero fields are applied. The ATLAS_CONTEXT_* env
// vars override this, and a per-request ContextInput field overrides both.
func WithContextBudget(b ContextBudget) Option { return engine.WithContextBudget(b) }

// New builds an Engine. With zero options it is the LOCAL tier: embedded SQLite
// under ./.atlas, lexical search on, vectors off, code never leaves the machine.
func New(ctx context.Context, opts ...Option) (Engine, error) {
	return engine.New(ctx, opts...)
}
