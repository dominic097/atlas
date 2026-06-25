// Package graph holds the pure, persistence-agnostic data model for the Atlas
// code knowledge graph. These structs are ported from aziron-pulse
// internal/models/devops.go with SQL `db:` tags stripped — the StorageDriver
// maps them to SQLite or Postgres columns.
package graph

import "time"

// NodeKind enumerates the kinds of graph nodes.
type NodeKind string

const (
	NodeRepo   NodeKind = "repo"
	NodeFile   NodeKind = "file"
	NodeSymbol NodeKind = "symbol"
	NodeRoute  NodeKind = "route"
	NodeTest   NodeKind = "test"
)

// EdgeKind enumerates the kinds of graph edges.
type EdgeKind string

const (
	EdgeCalls      EdgeKind = "calls"
	EdgeImports    EdgeKind = "imports"
	EdgeReferences EdgeKind = "references"
	EdgeCovers     EdgeKind = "covers"
	EdgeServes     EdgeKind = "serves"
	EdgeConsumes   EdgeKind = "consumes"
)

// IndexStatus is the lifecycle state of a repo's index.
type IndexStatus string

const (
	StatusPending IndexStatus = "pending"
	StatusRunning IndexStatus = "running"
	StatusReady   IndexStatus = "ready"
	StatusFailed  IndexStatus = "failed"
)

// JSONBMap is a driver-portable JSON object. It serializes to JSONB on Postgres
// and TEXT on SQLite. (Scan must accept both []byte and string — see store impls.)
type JSONBMap map[string]any

// NodeID is a content-stable identity for a graph node, stable across snapshots
// so temporal diff (history / snapshot_diff) can compute set differences.
type NodeID string

// Repo is a unit of indexing.
type Repo struct {
	ID            string         `json:"repo_id"`
	FullName      string         `json:"repo_full_name"`
	Root          string         `json:"root"`
	DefaultBranch string         `json:"default_branch"`
	Status        IndexStatus    `json:"status"`
	Languages     map[string]int `json:"languages,omitempty"`
	LastCommit    string         `json:"last_commit,omitempty"`
	LastIndexedAt *time.Time     `json:"last_indexed_at,omitempty"`
	Scope         string         `json:"scope,omitempty"` // tenant/org scope on hosted; empty on local
}

// Snapshot is an immutable per-commit graph state (the temporal moat).
type Snapshot struct {
	ID          string    `json:"snapshot_id"`
	RepoID      string    `json:"repo_id"`
	CommitSHA   string    `json:"commit_sha"`
	Branch      string    `json:"branch,omitempty"`
	CommitRange string    `json:"commit_range,omitempty"`
	FileCount   int       `json:"file_count"`
	SymbolCount int       `json:"symbol_count"`
	EdgeCount   int       `json:"edge_count"`
	RouteCount  int       `json:"route_count"`
	Metadata    JSONBMap  `json:"metadata,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// File is an indexed source file.
type File struct {
	ID         string   `json:"id"`
	SnapshotID string   `json:"snapshot_id"`
	Path       string   `json:"path"`
	Language   string   `json:"language"`
	SizeBytes  int64    `json:"size_bytes"`
	Hash       string   `json:"hash"`
	Imports    []string `json:"imports,omitempty"`
	DocSummary string   `json:"doc_summary,omitempty"`
}

// CodeSymbol is a parsed symbol (function, method, type, ...).
type CodeSymbol struct {
	ID         string   `json:"symbol_id"`
	SnapshotID string   `json:"snapshot_id"`
	NodeID     NodeID   `json:"node_id"`
	RepoID     string   `json:"repo_id"`
	Path       string   `json:"path"`
	Language   string   `json:"language"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Signature  string   `json:"signature,omitempty"`
	Doc        string   `json:"doc,omitempty"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	Metadata   JSONBMap `json:"metadata,omitempty"`
}

// DependencyEdge is a graph edge (call, import, reference, ...).
type DependencyEdge struct {
	ID         string   `json:"id"`
	SnapshotID string   `json:"snapshot_id"`
	FromFile   string   `json:"from_file"`
	FromSymbol string   `json:"from_symbol,omitempty"`
	ToRef      string   `json:"to_ref"`
	Kind       EdgeKind `json:"kind"`
	Language   string   `json:"language,omitempty"`
	Line       int      `json:"line,omitempty"`
	Metadata   JSONBMap `json:"metadata,omitempty"`
}

// Route is a producer/consumer HTTP route contract (the cross-repo bridge node).
type Route struct {
	ID           string   `json:"id"`
	SnapshotID   string   `json:"snapshot_id"`
	RepoFullName string   `json:"repo_full_name"`
	Method       string   `json:"method"`
	PathPattern  string   `json:"path_pattern"`
	HandlerFile  string   `json:"handler_file,omitempty"`
	Role         string   `json:"role"` // producer | consumer
	Source       string   `json:"source,omitempty"`
	Confidence   string   `json:"confidence,omitempty"`
	Metadata     JSONBMap `json:"metadata,omitempty"`
}

// Coverage is a symbol<->test coverage edge (the test-intel substrate).
type Coverage struct {
	ID           string `json:"id"`
	SnapshotID   string `json:"snapshot_id,omitempty"`
	RepoFullName string `json:"repo_full_name"`
	SymbolRef    string `json:"symbol_ref"`
	TestID       string `json:"test_id"`
	TestFile     string `json:"test_file,omitempty"`
	CoverageType string `json:"coverage_type"` // ut | e2e | integration
	Strength     string `json:"strength,omitempty"`
}

// CrossDep is a detected consumer call-site that depends on another service.
type CrossDep struct {
	ID            string `json:"id"`
	SnapshotID    string `json:"snapshot_id"`
	SourceService string `json:"source_service"`
	TargetService string `json:"target_service"`
	Type          string `json:"type"` // http | grpc | relative_api
	Endpoint      string `json:"endpoint"`
	CallingSymbol string `json:"calling_symbol,omitempty"`
	CallingFile   string `json:"calling_file,omitempty"`
	Confidence    string `json:"confidence,omitempty"`
}
