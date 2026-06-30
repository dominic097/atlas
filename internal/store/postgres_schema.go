package store

// schemaPostgres is the HOSTED-tier DDL — the Postgres counterpart of
// schemaSQLite, lifting the same six tables (repos, snapshots, files,
// symbols-with-node_id, edges, routes) into native Postgres types: TEXT primary
// keys, JSONB for the metadata blobs and the languages map, TEXT[] for file
// imports, and TIMESTAMPTZ for timestamps. The index set mirrors the SQLite tier
// exactly — name/path-line and path on symbols, to_ref/from_symbol with call-kind
// composites on edges, (repo_id,created_at DESC) on snapshots, UNIQUE
// (repo_id,commit_sha) for snapshot idempotency, and UNIQUE (scope,lower(full_name))
// for the (scope, repo) upsert key — so the two drivers serve identical query
// plans. Applied idempotently by Migrate (IF NOT EXISTS).
const schemaPostgres = `
CREATE TABLE IF NOT EXISTS repos (
	id              TEXT PRIMARY KEY,
	full_name       TEXT NOT NULL,
	root            TEXT NOT NULL DEFAULT '',
	default_branch  TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'pending',
	languages       JSONB NOT NULL DEFAULT '{}'::jsonb,
	last_commit     TEXT NOT NULL DEFAULT '',
	last_indexed_at TIMESTAMPTZ,
	scope           TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_repos_scope_fullname
	ON repos (scope, lower(full_name));

CREATE TABLE IF NOT EXISTS snapshots (
	id           TEXT PRIMARY KEY,
	repo_id      TEXT NOT NULL,
	commit_sha   TEXT NOT NULL DEFAULT '',
	branch       TEXT NOT NULL DEFAULT '',
	commit_range TEXT NOT NULL DEFAULT '',
	file_count   INTEGER NOT NULL DEFAULT 0,
	symbol_count INTEGER NOT NULL DEFAULT 0,
	edge_count   INTEGER NOT NULL DEFAULT 0,
	route_count  INTEGER NOT NULL DEFAULT 0,
	metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_repo_created
	ON snapshots (repo_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_snapshots_created
	ON snapshots (created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_repo_commit
	ON snapshots (repo_id, commit_sha);

CREATE TABLE IF NOT EXISTS files (
	id          TEXT PRIMARY KEY,
	snapshot_id TEXT NOT NULL,
	path        TEXT NOT NULL,
	language    TEXT NOT NULL DEFAULT '',
	size_bytes  BIGINT NOT NULL DEFAULT 0,
	hash        TEXT NOT NULL DEFAULT '',
	imports     TEXT[] NOT NULL DEFAULT '{}',
	doc_summary TEXT NOT NULL DEFAULT ''
);
-- idx_files_snapshot omitted: strict prefix of idx_files_snapshot_path (see SQLite tier).
CREATE INDEX IF NOT EXISTS idx_files_snapshot_path ON files (snapshot_id, path);

CREATE TABLE IF NOT EXISTS symbols (
	id          TEXT PRIMARY KEY,
	snapshot_id TEXT NOT NULL,
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
	metadata    JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_symbols_snapshot_name_path_line
	ON symbols (snapshot_id, name, path, start_line);
CREATE INDEX IF NOT EXISTS idx_symbols_snapshot_path ON symbols (snapshot_id, path);
-- idx_symbols_node omitted: no query filters on node_id (see SQLite tier).

-- edges has NO uuid id (write-only, read by nothing); mirrors the SQLite tier's
-- rowid edges table. Postgres needs no PK here — edges are never looked up by id.
CREATE TABLE IF NOT EXISTS edges (
	snapshot_id TEXT NOT NULL,
	from_file   TEXT NOT NULL DEFAULT '',
	from_symbol TEXT NOT NULL DEFAULT '',
	to_ref      TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT '',
	language    TEXT NOT NULL DEFAULT '',
	line        INTEGER NOT NULL DEFAULT 0,
	metadata    JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_edges_snapshot_kind_toref
	ON edges (snapshot_id, kind, to_ref);
CREATE INDEX IF NOT EXISTS idx_edges_snapshot_kind_fromsymbol
	ON edges (snapshot_id, kind, from_symbol);
CREATE INDEX IF NOT EXISTS idx_edges_snapshot_fromfile ON edges (snapshot_id, from_file);

CREATE TABLE IF NOT EXISTS routes (
	id             TEXT PRIMARY KEY,
	snapshot_id    TEXT NOT NULL,
	repo_full_name TEXT NOT NULL DEFAULT '',
	method         TEXT NOT NULL DEFAULT '',
	path_pattern   TEXT NOT NULL DEFAULT '',
	handler_file   TEXT NOT NULL DEFAULT '',
	role           TEXT NOT NULL DEFAULT '',
	source         TEXT NOT NULL DEFAULT '',
	confidence     TEXT NOT NULL DEFAULT '',
	metadata       JSONB NOT NULL DEFAULT '{}'::jsonb
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
-- idx_coverage_snapshot omitted: strict prefix of idx_coverage_snapshot_symbol.
CREATE INDEX IF NOT EXISTS idx_coverage_snapshot_symbol ON coverage (snapshot_id, symbol_ref);

CREATE TABLE IF NOT EXISTS embeddings (
	snapshot_id TEXT NOT NULL DEFAULT '',
	symbol_id   TEXT NOT NULL DEFAULT '',
	dim         INTEGER NOT NULL DEFAULT 0,
	vec         BYTEA,
	PRIMARY KEY (snapshot_id, symbol_id)
);
-- idx_embeddings_snapshot omitted: snapshot_id is the leading column of the PK.
`
