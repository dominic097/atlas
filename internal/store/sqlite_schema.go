package store

// schemaSQLite is the LOCAL-tier DDL, adapted from the original storage schema.
// internal/repository/code_intelligence_repository.go (Postgres) and translated
// to SQLite: TEXT primary keys (no uuid type / no pq arrays), JSON stored as
// TEXT, time stored as RFC3339 TEXT. It is applied idempotently by Migrate on
// every open (CREATE TABLE/INDEX IF NOT EXISTS).
//
// Tables: repos, snapshots, files, symbols (with node_id), edges, routes.
const schemaSQLite = `
CREATE TABLE IF NOT EXISTS repos (
	id              TEXT PRIMARY KEY,
	full_name       TEXT NOT NULL,
	root            TEXT NOT NULL DEFAULT '',
	default_branch  TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'pending',
	languages       TEXT NOT NULL DEFAULT '{}',
	last_commit     TEXT NOT NULL DEFAULT '',
	last_indexed_at TEXT,
	scope           TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_repos_scope_fullname
	ON repos (scope, lower(full_name));

-- snapshots carries BOTH the public uuid (id) and a compact internal integer
-- surrogate (sid). Child rows (files/symbols/edges/routes) store the small sid in
-- their snapshot_id column instead of the 36B uuid — the surrogate never leaves the
-- store (readers re-attach the public uuid), so output is unchanged. id stays UNIQUE
-- so ON CONFLICT(id) upserts and uuid lookups keep working.
CREATE TABLE IF NOT EXISTS snapshots (
	sid          INTEGER PRIMARY KEY,
	id           TEXT NOT NULL UNIQUE,
	repo_id      TEXT NOT NULL,
	commit_sha   TEXT NOT NULL DEFAULT '',
	branch       TEXT NOT NULL DEFAULT '',
	commit_range TEXT NOT NULL DEFAULT '',
	file_count   INTEGER NOT NULL DEFAULT 0,
	symbol_count INTEGER NOT NULL DEFAULT 0,
	edge_count   INTEGER NOT NULL DEFAULT 0,
	route_count  INTEGER NOT NULL DEFAULT 0,
	metadata     TEXT NOT NULL DEFAULT '{}',
	created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_repo_created
	ON snapshots (repo_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_repo_commit
	ON snapshots (repo_id, commit_sha);

CREATE TABLE IF NOT EXISTS files (
	id          TEXT PRIMARY KEY,
	snapshot_id INTEGER NOT NULL,
	path        TEXT NOT NULL,
	language    TEXT NOT NULL DEFAULT '',
	size_bytes  INTEGER NOT NULL DEFAULT 0,
	hash        TEXT NOT NULL DEFAULT '',
	imports     TEXT NOT NULL DEFAULT '[]',
	doc_summary TEXT NOT NULL DEFAULT ''
);
-- idx_files_snapshot (snapshot_id) intentionally omitted: it is a strict prefix
-- of idx_files_snapshot_path, so the planner already serves bare snapshot_id
-- scans from the composite. (Audited: ListFiles binds idx_files_snapshot_path.)
CREATE INDEX IF NOT EXISTS idx_files_snapshot_path ON files (snapshot_id, path);

CREATE TABLE IF NOT EXISTS symbols (
	id          TEXT PRIMARY KEY,
	snapshot_id INTEGER NOT NULL,
	node_id     TEXT NOT NULL DEFAULT '',
	repo_id     TEXT NOT NULL DEFAULT '',
	path        TEXT NOT NULL DEFAULT '',
	language    TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT '',
	name        TEXT NOT NULL DEFAULT '',
	signature   TEXT NOT NULL DEFAULT '',
	doc         TEXT NOT NULL DEFAULT '',
	start_line  INTEGER NOT NULL DEFAULT 0,
	end_line    INTEGER NOT NULL DEFAULT 0,
	metadata    TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_symbols_snapshot_name ON symbols (snapshot_id, name);
CREATE INDEX IF NOT EXISTS idx_symbols_snapshot_path ON symbols (snapshot_id, path);
-- idx_symbols_node (snapshot_id, node_id) intentionally omitted: no query filters
-- on node_id (node_id is written + read as payload only, never a WHERE/JOIN key).
-- Re-add if a cross-snapshot node-identity lookup is introduced.

-- edges has NO uuid id column: it is the largest table and its surrogate id was
-- write-only (read by no query/consumer), so we use SQLite's implicit rowid and
-- drop the 36B/row + the uuid-PK autoindex. DependencyEdge.ID stays empty after a
-- round-trip; nothing reads it.
CREATE TABLE IF NOT EXISTS edges (
	snapshot_id INTEGER NOT NULL,
	from_file   TEXT NOT NULL DEFAULT '',
	from_symbol TEXT NOT NULL DEFAULT '',
	to_ref      TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT '',
	language    TEXT NOT NULL DEFAULT '',
	line        INTEGER NOT NULL DEFAULT 0,
	metadata    TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_edges_snapshot_toref ON edges (snapshot_id, to_ref);
CREATE INDEX IF NOT EXISTS idx_edges_snapshot_fromsymbol ON edges (snapshot_id, from_symbol);
CREATE INDEX IF NOT EXISTS idx_edges_snapshot_fromfile ON edges (snapshot_id, from_file);

CREATE TABLE IF NOT EXISTS routes (
	id             TEXT PRIMARY KEY,
	snapshot_id    INTEGER NOT NULL,
	repo_full_name TEXT NOT NULL DEFAULT '',
	method         TEXT NOT NULL DEFAULT '',
	path_pattern   TEXT NOT NULL DEFAULT '',
	handler_file   TEXT NOT NULL DEFAULT '',
	role           TEXT NOT NULL DEFAULT '',
	source         TEXT NOT NULL DEFAULT '',
	confidence     TEXT NOT NULL DEFAULT '',
	metadata       TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_routes_snapshot_role ON routes (snapshot_id, role);

CREATE TABLE IF NOT EXISTS coverage (
	id             TEXT PRIMARY KEY,
	snapshot_id    TEXT NOT NULL DEFAULT '',
	repo_full_name TEXT NOT NULL DEFAULT '',
	symbol_ref     TEXT NOT NULL DEFAULT '',
	test_id        TEXT NOT NULL DEFAULT '',
	test_file      TEXT NOT NULL DEFAULT '',
	coverage_type  TEXT NOT NULL DEFAULT '',
	strength       TEXT NOT NULL DEFAULT ''
);
-- idx_coverage_snapshot (snapshot_id) intentionally omitted: strict prefix of
-- idx_coverage_snapshot_symbol, which serves both ListCoverage variants.
CREATE INDEX IF NOT EXISTS idx_coverage_snapshot_symbol ON coverage (snapshot_id, symbol_ref);

CREATE TABLE IF NOT EXISTS embeddings (
	snapshot_id TEXT NOT NULL DEFAULT '',
	symbol_id   TEXT NOT NULL DEFAULT '',
	dim         INTEGER NOT NULL DEFAULT 0,
	vec         BLOB,
	PRIMARY KEY (snapshot_id, symbol_id)
);
-- idx_embeddings_snapshot (snapshot_id) intentionally omitted: snapshot_id is the
-- leading column of the (snapshot_id, symbol_id) PK, which already serves
-- NearestSymbols' bare snapshot_id filter.
`
