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

	GraphExportInput  = engine.GraphExportInput
	GraphExportResult = engine.GraphExportResult
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

// New builds an Engine. With zero options it is the LOCAL tier: embedded SQLite
// under ./.atlas, lexical search on, vectors off, code never leaves the machine.
func New(ctx context.Context, opts ...Option) (Engine, error) {
	return engine.New(ctx, opts...)
}
