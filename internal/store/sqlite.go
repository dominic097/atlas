package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// sqliteDriver is the LOCAL tier StorageDriver, backed by modernc.org/sqlite
// (pure-Go, no CGO). It opens <repo>/.atlas/atlas.db with WAL journaling and a
// busy timeout, applies the schema on first open, and serializes writes through
// a single process mutex (Capabilities.ConcurrentWrite=false).
type sqliteDriver struct {
	path string
	db   *sql.DB
	mu   sync.Mutex // serializes writes; ConcurrentWrite=false
}

func openSQLite(ctx context.Context, path string) (StorageDriver, error) {
	if path == "" {
		path = "./.atlas/atlas.db"
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: create sqlite dir: %w", err)
		}
	}
	// _pragma URL params apply per-connection on the modernc driver. The cache,
	// mmap, and temp-store settings reduce local bulk-index overhead without
	// weakening WAL durability or foreign-key enforcement.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=cache_size(-65536)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	// SQLite tolerates a single writer; cap the pool so WAL stays happy.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping sqlite: %w", err)
	}
	return &sqliteDriver{path: path, db: db}, nil
}

// sqliteSchemaVersion is bumped whenever schemaSQLite changes shape. Migrate
// stamps it into PRAGMA user_version after applying the schema, so subsequent
// opens (the common case — every read query) skip the DDL replay entirely and pay
// only one pragma read. This is the dominant per-invocation startup cost for query
// ops, where the actual query is a few ms but schema replay was not.
//
// v2 = compact schema (redundant indexes removed; the incremental column
// compactions ride later bumps). Because a local .atlas.db is a DERIVED cache (the
// git working tree is the source of truth), a version MISMATCH does a clean
// DROP+recreate rather than an in-place data migration: the caller (atlas index /
// atlas watch) reindexes from the working tree afterward. No migration code, no
// partial-state risk.
const sqliteSchemaVersion = 2

func (d *sqliteDriver) Migrate(ctx context.Context) error {
	var ver int
	err := d.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&ver)
	if err == nil && ver == sqliteSchemaVersion {
		return nil // schema already current — skip all DDL replay
	}
	// Any non-zero, non-current version (older OR a downgraded future one) is an
	// INCOMPATIBLE on-disk shape. Drop every Atlas table so the fresh DDL below
	// recreates them clean; the snapshot data is rebuilt by the next reindex.
	// ver==0 is a fresh / pre-gate DB with nothing (compatible) to drop.
	if err == nil && ver != 0 {
		if _, derr := d.db.ExecContext(ctx, dropAllSQLite); derr != nil {
			return fmt.Errorf("store: drop stale sqlite schema (v%d): %w", ver, derr)
		}
	}
	if _, err := d.db.ExecContext(ctx, schemaSQLite); err != nil {
		return fmt.Errorf("store: migrate sqlite: %w", err)
	}
	// PRAGMA user_version takes no bound parameter; the version is a constant.
	if _, err := d.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", sqliteSchemaVersion)); err != nil {
		return fmt.Errorf("store: stamp sqlite schema version: %w", err)
	}
	return nil
}

// dropAllSQLite removes every Atlas table (indexes drop with their tables) so a
// version-mismatched DB can be rebuilt fresh. SQLite DROP TABLE takes one table
// per statement; order is free because schemaSQLite declares no inter-table FKs.
// Safe under MaxOpenConns(1): no concurrent reader can observe the gap.
const dropAllSQLite = `
DROP TABLE IF EXISTS embeddings;
DROP TABLE IF EXISTS coverage;
DROP TABLE IF EXISTS routes;
DROP TABLE IF EXISTS edges;
DROP TABLE IF EXISTS symbols;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS snapshots;
DROP TABLE IF EXISTS repos;
`

func (d *sqliteDriver) Dialect() string { return "sqlite" }

func (d *sqliteDriver) Capabilities() Capabilities {
	return Capabilities{DurableQueue: false, CrossScope: false, ConcurrentWrite: false, PushReindex: false}
}

func (d *sqliteDriver) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

// ---- helpers ---------------------------------------------------------------

const rfc3339 = time.RFC3339Nano

// marshalJSONMap serializes a JSONBMap to TEXT, never returning NULL (defaults to "{}").
func marshalJSONMap(m graph.JSONBMap) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalJSONMap parses TEXT (or []byte) JSON back into a JSONBMap.
func unmarshalJSONMap(v any) (graph.JSONBMap, error) {
	var raw []byte
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		if t == "" {
			return nil, nil
		}
		raw = []byte(t)
	case []byte:
		if len(t) == 0 {
			return nil, nil
		}
		raw = t
	default:
		return nil, fmt.Errorf("store: cannot scan %T as json map", v)
	}
	m := graph.JSONBMap{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// marshalStrings serializes []string to a JSON TEXT array, never NULL.
func marshalStrings(s []string) (string, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalStrings(v sql.NullString) ([]string, error) {
	if !v.Valid || v.String == "" || v.String == "[]" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func marshalLanguages(m map[string]int) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalLanguages(v sql.NullString) (map[string]int, error) {
	if !v.Valid || v.String == "" {
		return nil, nil
	}
	out := map[string]int{}
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func timePtrToText(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(rfc3339)
}

func textToTimePtr(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	if ts, err := time.Parse(rfc3339, v.String); err == nil {
		return &ts
	}
	if ts, err := time.Parse(time.RFC3339, v.String); err == nil {
		return &ts
	}
	return nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(rfc3339, s); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts
	}
	return time.Time{}
}

// ---- repos -----------------------------------------------------------------

func (d *sqliteDriver) EnsureRepo(ctx context.Context, r *graph.Repo) (*graph.Repo, error) {
	if r == nil {
		return nil, fmt.Errorf("store: repo is required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	scope := r.Scope
	// Resolve an existing repo by (scope, full_name) so callers without an ID still upsert.
	var existingID string
	err := d.db.QueryRowContext(ctx,
		`SELECT id FROM repos WHERE scope = ? AND lower(full_name) = lower(?) LIMIT 1`,
		scope, r.FullName,
	).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: lookup repo: %w", err)
	}

	id := r.ID
	if existingID != "" {
		id = existingID
	}
	if id == "" {
		id = uuid.NewString()
	}

	langs, err := marshalLanguages(r.Languages)
	if err != nil {
		return nil, fmt.Errorf("store: marshal languages: %w", err)
	}
	status := string(r.Status)
	if status == "" {
		status = string(graph.StatusPending)
	}

	// On re-ensure, a sparsely-populated input must not clobber columns that
	// already hold data — mirror Pulse's COALESCE baseline-preservation: an
	// empty incoming value falls back to the stored one (NULLIF('',...) → NULL
	// → COALESCE picks the existing column).
	_, err = d.db.ExecContext(ctx, `
		INSERT INTO repos (id, full_name, root, default_branch, status, languages, last_commit, last_indexed_at, scope)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			full_name       = COALESCE(NULLIF(excluded.full_name, ''), repos.full_name),
			root            = COALESCE(NULLIF(excluded.root, ''), repos.root),
			default_branch  = COALESCE(NULLIF(excluded.default_branch, ''), repos.default_branch),
			status          = COALESCE(NULLIF(excluded.status, ''), repos.status),
			languages       = CASE WHEN excluded.languages IN ('', '{}') THEN repos.languages ELSE excluded.languages END,
			last_commit     = COALESCE(NULLIF(excluded.last_commit, ''), repos.last_commit),
			last_indexed_at = COALESCE(excluded.last_indexed_at, repos.last_indexed_at),
			scope           = excluded.scope
	`,
		id, r.FullName, r.Root, r.DefaultBranch, status, langs, r.LastCommit,
		timePtrToText(r.LastIndexedAt), scope,
	)
	if err != nil {
		return nil, fmt.Errorf("store: ensure repo: %w", err)
	}

	out := *r
	out.ID = id
	out.Status = graph.IndexStatus(status)
	return &out, nil
}

func (d *sqliteDriver) ListRepos(ctx context.Context, scope string) ([]graph.Repo, error) {
	var (
		rows *sql.Rows
		err  error
	)
	const cols = `id, full_name, root, default_branch, status, languages, last_commit, last_indexed_at, scope`
	if scope == "" {
		rows, err = d.db.QueryContext(ctx, `SELECT `+cols+` FROM repos ORDER BY lower(full_name)`)
	} else {
		rows, err = d.db.QueryContext(ctx, `SELECT `+cols+` FROM repos WHERE scope = ? ORDER BY lower(full_name)`, scope)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list repos: %w", err)
	}
	defer rows.Close()

	var out []graph.Repo
	for rows.Next() {
		var (
			r       graph.Repo
			status  string
			langs   sql.NullString
			lastIdx sql.NullString
		)
		if err := rows.Scan(&r.ID, &r.FullName, &r.Root, &r.DefaultBranch, &status, &langs, &r.LastCommit, &lastIdx, &r.Scope); err != nil {
			return nil, fmt.Errorf("store: scan repo: %w", err)
		}
		r.Status = graph.IndexStatus(status)
		if r.Languages, err = unmarshalLanguages(langs); err != nil {
			return nil, fmt.Errorf("store: unmarshal languages: %w", err)
		}
		r.LastIndexedAt = textToTimePtr(lastIdx)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- snapshots -------------------------------------------------------------

// SaveSnapshot inserts the snapshot row plus all files/symbols/edges/routes
// inside ONE transaction. It is idempotent on (repo_id, commit_sha): re-saving
// the same commit replaces the snapshot's child rows.
func (d *sqliteDriver) SaveSnapshot(ctx context.Context, s *graph.Snapshot, files []graph.File,
	symbols []graph.CodeSymbol, edges []graph.DependencyEdge, routes []graph.Route) error {
	if s == nil {
		return fmt.Errorf("store: snapshot is required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	s.FileCount = len(files)
	s.SymbolCount = len(symbols)
	s.EdgeCount = len(edges)
	s.RouteCount = len(routes)

	meta, err := marshalJSONMap(s.Metadata)
	if err != nil {
		return fmt.Errorf("store: marshal snapshot metadata: %w", err)
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Idempotency: if a snapshot for this (repo_id, commit_sha) already exists,
	// reuse its id and wipe its child rows so we rebuild cleanly.
	if s.CommitSHA != "" {
		var existingID string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM snapshots WHERE repo_id = ? AND commit_sha = ? LIMIT 1`,
			s.RepoID, s.CommitSHA,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: lookup snapshot: %w", err)
		}
		if existingID != "" {
			s.ID = existingID
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO snapshots (id, repo_id, commit_sha, branch, commit_range, file_count, symbol_count, edge_count, route_count, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			repo_id      = excluded.repo_id,
			commit_sha   = excluded.commit_sha,
			branch       = excluded.branch,
			commit_range = excluded.commit_range,
			file_count   = excluded.file_count,
			symbol_count = excluded.symbol_count,
			edge_count   = excluded.edge_count,
			route_count  = excluded.route_count,
			metadata     = excluded.metadata
	`,
		s.ID, s.RepoID, s.CommitSHA, s.Branch, s.CommitRange,
		s.FileCount, s.SymbolCount, s.EdgeCount, s.RouteCount, meta,
		s.CreatedAt.UTC().Format(rfc3339),
	)
	if err != nil {
		return fmt.Errorf("store: save snapshot: %w", err)
	}

	// Rebuild child rows from scratch.
	for _, table := range []string{"files", "symbols", "edges", "routes"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE snapshot_id = ?`, s.ID); err != nil {
			return fmt.Errorf("store: clear %s: %w", table, err)
		}
	}

	fileRows := make([][]any, 0, len(files))
	for i := range files {
		f := &files[i]
		id := f.ID
		if id == "" {
			id = uuid.NewString()
		}
		imp, err := marshalStrings(f.Imports)
		if err != nil {
			return fmt.Errorf("store: marshal imports for %s: %w", f.Path, err)
		}
		fileRows = append(fileRows, []any{id, s.ID, f.Path, f.Language, f.SizeBytes, f.Hash, imp, f.DocSummary})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO files (id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary) VALUES `,
		fileRows); err != nil {
		return fmt.Errorf("store: save files: %w", err)
	}

	symbolRows := make([][]any, 0, len(symbols))
	for i := range symbols {
		sym := &symbols[i]
		id := sym.ID
		if id == "" {
			id = uuid.NewString()
		}
		m, err := marshalJSONMap(sym.Metadata)
		if err != nil {
			return fmt.Errorf("store: marshal symbol metadata for %s: %w", sym.Name, err)
		}
		symbolRows = append(symbolRows, []any{id, s.ID, string(sym.NodeID), sym.RepoID, sym.Path, sym.Language,
			sym.Kind, sym.Name, sym.Signature, sym.Doc, sym.StartLine, sym.EndLine, m})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO symbols (id, snapshot_id, node_id, repo_id, path, language, kind, name, signature, doc, start_line, end_line, metadata) VALUES `,
		symbolRows); err != nil {
		return fmt.Errorf("store: save symbols: %w", err)
	}

	edgeRows := make([][]any, 0, len(edges))
	for i := range edges {
		e := &edges[i]
		id := e.ID
		if id == "" {
			id = uuid.NewString()
		}
		m, err := marshalJSONMap(e.Metadata)
		if err != nil {
			return fmt.Errorf("store: marshal edge metadata: %w", err)
		}
		edgeRows = append(edgeRows, []any{id, s.ID, e.FromFile, e.FromSymbol, e.ToRef, string(e.Kind), e.Language, e.Line, m})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO edges (id, snapshot_id, from_file, from_symbol, to_ref, kind, language, line, metadata) VALUES `,
		edgeRows); err != nil {
		return fmt.Errorf("store: save edges: %w", err)
	}

	routeRows := make([][]any, 0, len(routes))
	for i := range routes {
		rt := &routes[i]
		id := rt.ID
		if id == "" {
			id = uuid.NewString()
		}
		m, err := marshalJSONMap(rt.Metadata)
		if err != nil {
			return fmt.Errorf("store: marshal route metadata: %w", err)
		}
		routeRows = append(routeRows, []any{id, s.ID, rt.RepoFullName, rt.Method, rt.PathPattern,
			rt.HandlerFile, rt.Role, rt.Source, rt.Confidence, m})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO routes (id, snapshot_id, repo_full_name, method, path_pattern, handler_file, role, source, confidence, metadata) VALUES `,
		routeRows); err != nil {
		return fmt.Errorf("store: save routes: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit snapshot: %w", err)
	}
	return nil
}

// ReplaceFileRows is the SQL-level incremental delta: it deletes only the rows
// owned by the affected ∪ deleted files and bulk-inserts the supplied fresh rows
// for the affected files, then updates the snapshot's counts — all in ONE tx.
// Unchanged files' rows are never touched, so a small edit re-saves only its blast
// radius rather than the whole graph (the byte-identical-to-full-reindex parity is
// guaranteed by the caller, which re-emits exactly the affected files' rows).
func (d *sqliteDriver) ReplaceFileRows(ctx context.Context, snapshotID string, fileScope, edgeScope []string,
	files []graph.File, symbols []graph.CodeSymbol, edges []graph.DependencyEdge, routes []graph.Route,
	newFileCount, newSymbolCount, newEdgeCount, newRouteCount int) error {
	if strings.TrimSpace(snapshotID) == "" {
		return fmt.Errorf("store: snapshot id is required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin replace tx: %w", err)
	}
	defer tx.Rollback()

	// fileScope drives the files/symbols/routes deletes (keyed by path/path/handler_file);
	// edgeScope (a superset including Go reverse-dep files) drives the edges delete
	// (keyed by from_file). A reverse-dep file is in edgeScope but NOT fileScope, so only
	// its edge rows are replaced while its symbol/file rows are preserved.
	fileScope = uniqueNonEmpty(fileScope)
	edgeScope = uniqueNonEmpty(edgeScope)
	if len(fileScope) > 0 {
		for _, del := range []struct{ table, col string }{
			{"files", "path"},
			{"symbols", "path"},
			{"routes", "handler_file"},
		} {
			if err := sqliteDeleteByFileColumn(ctx, tx, del.table, del.col, snapshotID, fileScope); err != nil {
				return fmt.Errorf("store: replace clear %s: %w", del.table, err)
			}
		}
	}
	if len(edgeScope) > 0 {
		if err := sqliteDeleteByFileColumn(ctx, tx, "edges", "from_file", snapshotID, edgeScope); err != nil {
			return fmt.Errorf("store: replace clear edges: %w", err)
		}
	}

	// Bulk-insert the fresh rows for the affected files. These reuse the SAME row
	// marshaling SaveSnapshot uses, so a row written here is byte-identical to the row
	// a full reindex would write for the same file.
	fileRows := make([][]any, 0, len(files))
	for i := range files {
		f := &files[i]
		id := f.ID
		if id == "" {
			id = uuid.NewString()
		}
		imp, err := marshalStrings(f.Imports)
		if err != nil {
			return fmt.Errorf("store: marshal imports for %s: %w", f.Path, err)
		}
		fileRows = append(fileRows, []any{id, snapshotID, f.Path, f.Language, f.SizeBytes, f.Hash, imp, f.DocSummary})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO files (id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary) VALUES `,
		fileRows); err != nil {
		return fmt.Errorf("store: replace save files: %w", err)
	}

	symbolRows := make([][]any, 0, len(symbols))
	for i := range symbols {
		sym := &symbols[i]
		id := sym.ID
		if id == "" {
			id = uuid.NewString()
		}
		m, err := marshalJSONMap(sym.Metadata)
		if err != nil {
			return fmt.Errorf("store: marshal symbol metadata for %s: %w", sym.Name, err)
		}
		symbolRows = append(symbolRows, []any{id, snapshotID, string(sym.NodeID), sym.RepoID, sym.Path, sym.Language,
			sym.Kind, sym.Name, sym.Signature, sym.Doc, sym.StartLine, sym.EndLine, m})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO symbols (id, snapshot_id, node_id, repo_id, path, language, kind, name, signature, doc, start_line, end_line, metadata) VALUES `,
		symbolRows); err != nil {
		return fmt.Errorf("store: replace save symbols: %w", err)
	}

	edgeRows := make([][]any, 0, len(edges))
	for i := range edges {
		e := &edges[i]
		id := e.ID
		if id == "" {
			id = uuid.NewString()
		}
		m, err := marshalJSONMap(e.Metadata)
		if err != nil {
			return fmt.Errorf("store: marshal edge metadata: %w", err)
		}
		edgeRows = append(edgeRows, []any{id, snapshotID, e.FromFile, e.FromSymbol, e.ToRef, string(e.Kind), e.Language, e.Line, m})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO edges (id, snapshot_id, from_file, from_symbol, to_ref, kind, language, line, metadata) VALUES `,
		edgeRows); err != nil {
		return fmt.Errorf("store: replace save edges: %w", err)
	}

	routeRows := make([][]any, 0, len(routes))
	for i := range routes {
		rt := &routes[i]
		id := rt.ID
		if id == "" {
			id = uuid.NewString()
		}
		m, err := marshalJSONMap(rt.Metadata)
		if err != nil {
			return fmt.Errorf("store: marshal route metadata: %w", err)
		}
		routeRows = append(routeRows, []any{id, snapshotID, rt.RepoFullName, rt.Method, rt.PathPattern,
			rt.HandlerFile, rt.Role, rt.Source, rt.Confidence, m})
	}
	if err := sqliteBulkInsert(ctx, tx,
		`INSERT INTO routes (id, snapshot_id, repo_full_name, method, path_pattern, handler_file, role, source, confidence, metadata) VALUES `,
		routeRows); err != nil {
		return fmt.Errorf("store: replace save routes: %w", err)
	}

	// Update the snapshot's counts to the new totals (kept + replaced). The caller
	// computes these; ReplaceFileRows never re-counts the whole snapshot.
	if _, err := tx.ExecContext(ctx,
		`UPDATE snapshots SET file_count = ?, symbol_count = ?, edge_count = ?, route_count = ? WHERE id = ?`,
		newFileCount, newSymbolCount, newEdgeCount, newRouteCount, snapshotID); err != nil {
		return fmt.Errorf("store: replace update counts: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit replace: %w", err)
	}
	return nil
}

// sqliteDeleteByFileColumn deletes every row of `table` whose `col` (a file-path
// column) is in `paths`, scoped to snapshotID, using a chunked IN-list so a large
// affected set never blows SQLite's bound-parameter limit. col is a fixed
// identifier ("path"|"from_file"|"handler_file"), never user input.
func sqliteDeleteByFileColumn(ctx context.Context, tx *sql.Tx, table, col, snapshotID string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	for start := 0; start < len(paths); start += symbolsChunk {
		end := start + symbolsChunk
		if end > len(paths) {
			end = len(paths)
		}
		chunk := paths[start:end]
		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, p := range chunk {
			args = append(args, p)
		}
		query := `DELETE FROM ` + table + ` WHERE snapshot_id = ? AND ` + col + ` IN (` + placeholders + `)`
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

const sqliteBulkInsertMaxVars = 900

func sqliteBulkInsert(ctx context.Context, tx *sql.Tx, prefix string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	cols := len(rows[0])
	if cols == 0 {
		return nil
	}
	chunkSize := sqliteBulkInsertMaxVars / cols
	if chunkSize < 1 {
		chunkSize = 1
	}
	for start := 0; start < len(rows); start += chunkSize {
		end := start + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var b strings.Builder
		b.Grow(len(prefix) + len(chunk)*cols*2)
		b.WriteString(prefix)
		args := make([]any, 0, len(chunk)*cols)
		for i, row := range chunk {
			if len(row) != cols {
				return fmt.Errorf("bulk insert row has %d columns, want %d", len(row), cols)
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('(')
			for c := 0; c < cols; c++ {
				if c > 0 {
					b.WriteByte(',')
				}
				b.WriteByte('?')
				args = append(args, row[c])
			}
			b.WriteByte(')')
		}
		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func (d *sqliteDriver) UpdateSnapshotMetadata(ctx context.Context, snapshotID string, metadata graph.JSONBMap) error {
	if strings.TrimSpace(snapshotID) == "" {
		return fmt.Errorf("store: snapshot id is required")
	}
	meta, err := marshalJSONMap(metadata)
	if err != nil {
		return fmt.Errorf("store: marshal snapshot metadata: %w", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE snapshots SET metadata = ? WHERE id = ?`, meta, snapshotID)
	if err != nil {
		return fmt.Errorf("store: update snapshot metadata: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: snapshot %q not found", snapshotID)
	}
	return nil
}

const snapshotCols = `id, repo_id, commit_sha, branch, commit_range, file_count, symbol_count, edge_count, route_count, metadata, created_at`

func scanSnapshot(sc interface{ Scan(...any) error }) (graph.Snapshot, error) {
	var (
		s         graph.Snapshot
		meta      sql.NullString
		createdAt string
	)
	if err := sc.Scan(&s.ID, &s.RepoID, &s.CommitSHA, &s.Branch, &s.CommitRange,
		&s.FileCount, &s.SymbolCount, &s.EdgeCount, &s.RouteCount, &meta, &createdAt); err != nil {
		return graph.Snapshot{}, err
	}
	m, err := unmarshalJSONMap(meta.String)
	if err != nil {
		return graph.Snapshot{}, err
	}
	s.Metadata = m
	s.CreatedAt = parseTime(createdAt)
	return s, nil
}

func (d *sqliteDriver) LatestSnapshot(ctx context.Context, repoID string) (*graph.Snapshot, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT `+snapshotCols+` FROM snapshots WHERE repo_id = ? ORDER BY created_at DESC LIMIT 1`,
		repoID,
	)
	s, err := scanSnapshot(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: latest snapshot: %w", err)
	}
	return &s, nil
}

func (d *sqliteDriver) ListSnapshots(ctx context.Context, repoID string, limit int) ([]graph.Snapshot, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT `+snapshotCols+` FROM snapshots WHERE repo_id = ? ORDER BY created_at DESC LIMIT ?`,
		repoID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list snapshots: %w", err)
	}
	defer rows.Close()

	var out []graph.Snapshot
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan snapshot: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- graph reads -----------------------------------------------------------

// symbolCols / edgeCols are the canonical column lists; SymbolsByName,
// SymbolsByPath, CallEdgesByToRefs and the List* readers all share them so every
// path returns the SAME graph shape (node_id + decoded metadata).
const symbolCols = `id, snapshot_id, node_id, repo_id, path, language, kind, name, signature, doc, start_line, end_line, metadata`
const edgeCols = `id, snapshot_id, from_file, from_symbol, to_ref, kind, language, line, metadata`

// scanSymbolRow decodes one symbols row into a graph.CodeSymbol (node_id +
// metadata included), matching ListSymbols exactly.
func scanSymbolRow(sc interface{ Scan(...any) error }) (graph.CodeSymbol, error) {
	var (
		sym    graph.CodeSymbol
		nodeID string
		meta   sql.NullString
	)
	if err := sc.Scan(&sym.ID, &sym.SnapshotID, &nodeID, &sym.RepoID, &sym.Path, &sym.Language,
		&sym.Kind, &sym.Name, &sym.Signature, &sym.Doc, &sym.StartLine, &sym.EndLine, &meta); err != nil {
		return graph.CodeSymbol{}, err
	}
	sym.NodeID = graph.NodeID(nodeID)
	m, err := unmarshalJSONMap(meta.String)
	if err != nil {
		return graph.CodeSymbol{}, err
	}
	sym.Metadata = m
	return sym, nil
}

// scanEdgeRow decodes one edges row into a graph.DependencyEdge (metadata
// included), matching ListEdges exactly.
func scanEdgeRow(sc interface{ Scan(...any) error }) (graph.DependencyEdge, error) {
	var (
		e    graph.DependencyEdge
		kind string
		meta sql.NullString
	)
	if err := sc.Scan(&e.ID, &e.SnapshotID, &e.FromFile, &e.FromSymbol, &e.ToRef, &kind, &e.Language, &e.Line, &meta); err != nil {
		return graph.DependencyEdge{}, err
	}
	e.Kind = graph.EdgeKind(kind)
	m, err := unmarshalJSONMap(meta.String)
	if err != nil {
		return graph.DependencyEdge{}, err
	}
	e.Metadata = m
	return e, nil
}

func (d *sqliteDriver) ListSymbols(ctx context.Context, snapshotID string) ([]graph.CodeSymbol, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+symbolCols+`
		FROM symbols WHERE snapshot_id = ?
		ORDER BY path, start_line, name`,
		snapshotID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list symbols: %w", err)
	}
	defer rows.Close()

	var out []graph.CodeSymbol
	for rows.Next() {
		sym, err := scanSymbolRow(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol: %w", err)
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

func (d *sqliteDriver) ListEdges(ctx context.Context, snapshotID string) ([]graph.DependencyEdge, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+edgeCols+`
		FROM edges WHERE snapshot_id = ?
		ORDER BY from_file, to_ref, kind`,
		snapshotID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list edges: %w", err)
	}
	defer rows.Close()

	var out []graph.DependencyEdge
	for rows.Next() {
		e, err := scanEdgeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan edge: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// streamBatch is the default StreamSymbols/StreamEdges batch size.
const streamBatch = 4096

func (d *sqliteDriver) StreamSymbols(ctx context.Context, snapshotID string, batch int, fn func([]graph.CodeSymbol) error) error {
	if batch <= 0 {
		batch = streamBatch
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+symbolCols+`
		FROM symbols WHERE snapshot_id = ?
		ORDER BY path, start_line, name`,
		snapshotID,
	)
	if err != nil {
		return fmt.Errorf("store: stream symbols: %w", err)
	}
	defer rows.Close()
	buf := make([]graph.CodeSymbol, 0, batch)
	for rows.Next() {
		sym, err := scanSymbolRow(rows)
		if err != nil {
			return fmt.Errorf("store: scan symbol: %w", err)
		}
		buf = append(buf, sym)
		if len(buf) >= batch {
			if err := fn(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(buf) > 0 {
		return fn(buf)
	}
	return nil
}

func (d *sqliteDriver) StreamEdges(ctx context.Context, snapshotID string, batch int, fn func([]graph.DependencyEdge) error) error {
	if batch <= 0 {
		batch = streamBatch
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+edgeCols+`
		FROM edges WHERE snapshot_id = ?
		ORDER BY from_file, to_ref, kind`,
		snapshotID,
	)
	if err != nil {
		return fmt.Errorf("store: stream edges: %w", err)
	}
	defer rows.Close()
	buf := make([]graph.DependencyEdge, 0, batch)
	for rows.Next() {
		e, err := scanEdgeRow(rows)
		if err != nil {
			return fmt.Errorf("store: scan edge: %w", err)
		}
		buf = append(buf, e)
		if len(buf) >= batch {
			if err := fn(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(buf) > 0 {
		return fn(buf)
	}
	return nil
}

// SymbolsByName returns symbols whose name matches exactly, served by the
// idx_symbols_snapshot_name index — the indexed seed for impact's reverse-BFS.
func (d *sqliteDriver) SymbolsByName(ctx context.Context, snapshotID, name string) ([]graph.CodeSymbol, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+symbolCols+`
		FROM symbols WHERE snapshot_id = ? AND name = ?
		ORDER BY path, start_line`,
		snapshotID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("store: symbols by name: %w", err)
	}
	defer rows.Close()

	var out []graph.CodeSymbol
	for rows.Next() {
		sym, err := scanSymbolRow(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol: %w", err)
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

// symbolsChunk is the IN-list batch size for SymbolsByNames; same rationale as
// callEdgesChunk — 400 names + the snapshot_id stays under SQLite's bound-param cap.
const symbolsChunk = 400

// SymbolsByNames returns every symbol whose name is in `names`, served by
// idx_symbols_snapshot_name. The IN-list is chunked so a large blast radius never
// blows the bound-parameter limit; all matching rows are returned with node_id +
// decoded metadata (no dedupe), exactly like SymbolsByName. This batched form
// replaces the per-name point queries the impact reverse-BFS used to issue.
func (d *sqliteDriver) SymbolsByNames(ctx context.Context, snapshotID string, names []string) ([]graph.CodeSymbol, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var out []graph.CodeSymbol
	for start := 0; start < len(names); start += symbolsChunk {
		end := start + symbolsChunk
		if end > len(names) {
			end = len(names)
		}
		chunk := names[start:end]

		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, n := range chunk {
			args = append(args, n)
		}

		// ORDER BY name, path, start_line so that, within any single name, rows
		// arrive in the SAME order SymbolsByName (path, start_line) yields — the
		// per-name candidate ordering resolveTargets/resolveCaller depend on for
		// deterministic "first candidate" selection. The leading name keeps each
		// name's rows contiguous.
		query := `SELECT ` + symbolCols + `
			FROM symbols WHERE snapshot_id = ? AND name IN (` + placeholders + `)
			ORDER BY name, path, start_line`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: symbols by names: %w", err)
		}
		for rows.Next() {
			sym, err := scanSymbolRow(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan symbol: %w", err)
			}
			out = append(out, sym)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: symbols by names: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// SymbolsByPath returns symbols in a given file, served by the
// idx_symbols_snapshot_path index.
func (d *sqliteDriver) SymbolsByPath(ctx context.Context, snapshotID, path string) ([]graph.CodeSymbol, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+symbolCols+`
		FROM symbols WHERE snapshot_id = ? AND path = ?
		ORDER BY start_line, name`,
		snapshotID, path,
	)
	if err != nil {
		return nil, fmt.Errorf("store: symbols by path: %w", err)
	}
	defer rows.Close()

	var out []graph.CodeSymbol
	for rows.Next() {
		sym, err := scanSymbolRow(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol: %w", err)
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

// SymbolsByIDs returns every symbol whose id is in `ids`, served by the symbols
// primary key (id) scoped to snapshot_id. The IN-list is chunked so a large hit
// set never blows the bound-parameter limit; all matching rows are returned with
// node_id + decoded metadata (no dedupe), exactly like SymbolsByName. This
// targeted form replaces the whole-snapshot ListSymbols scan the search/context
// paths used only to resolve a handful of known hit ids.
func (d *sqliteDriver) SymbolsByIDs(ctx context.Context, snapshotID string, ids []string) ([]graph.CodeSymbol, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []graph.CodeSymbol
	for start := 0; start < len(ids); start += symbolsChunk {
		end := start + symbolsChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, id := range chunk {
			args = append(args, id)
		}

		query := `SELECT ` + symbolCols + `
			FROM symbols WHERE snapshot_id = ? AND id IN (` + placeholders + `)
			ORDER BY path, start_line, name`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: symbols by ids: %w", err)
		}
		for rows.Next() {
			sym, err := scanSymbolRow(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan symbol: %w", err)
			}
			out = append(out, sym)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: symbols by ids: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// callEdgesChunk is the IN-list batch size: SQLite caps bound parameters
// (default 999); 400 to_refs + the snapshot_id stays comfortably under it.
const callEdgesChunk = 400

// CallEdgesByToRefs returns every "calls" edge whose to_ref is in toRefs, served
// by idx_edges_snapshot_toref. The IN-list is chunked so a large blast radius
// never blows the bound-parameter limit; all matching edges are returned with
// Metadata populated (no dedupe).
func (d *sqliteDriver) CallEdgesByToRefs(ctx context.Context, snapshotID string, toRefs []string) ([]graph.DependencyEdge, error) {
	if len(toRefs) == 0 {
		return nil, nil
	}
	var out []graph.DependencyEdge
	for start := 0; start < len(toRefs); start += callEdgesChunk {
		end := start + callEdgesChunk
		if end > len(toRefs) {
			end = len(toRefs)
		}
		chunk := toRefs[start:end]

		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, ref := range chunk {
			args = append(args, ref)
		}

		query := `SELECT ` + edgeCols + `
			FROM edges WHERE snapshot_id = ? AND kind = 'calls' AND to_ref IN (` + placeholders + `)`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: call edges by to_refs: %w", err)
		}
		for rows.Next() {
			e, err := scanEdgeRow(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan edge: %w", err)
			}
			out = append(out, e)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: call edges by to_refs: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

func (d *sqliteDriver) CallEdgesByFromSymbols(ctx context.Context, snapshotID string, fromSymbols []string) ([]graph.DependencyEdge, error) {
	if len(fromSymbols) == 0 {
		return nil, nil
	}
	var out []graph.DependencyEdge
	for start := 0; start < len(fromSymbols); start += callEdgesChunk {
		end := start + callEdgesChunk
		if end > len(fromSymbols) {
			end = len(fromSymbols)
		}
		chunk := fromSymbols[start:end]
		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, fs := range chunk {
			args = append(args, fs)
		}
		query := `SELECT ` + edgeCols + `
			FROM edges WHERE snapshot_id = ? AND kind = 'calls' AND from_symbol IN (` + placeholders + `)`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: call edges by from_symbols: %w", err)
		}
		for rows.Next() {
			e, err := scanEdgeRow(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan edge: %w", err)
			}
			out = append(out, e)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: call edges by from_symbols: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// RefEdgesByToRefs returns every "references" (type-use) edge whose to_ref is in
// toRefs, served by idx_edges_snapshot_toref. It is identical to
// CallEdgesByToRefs except for the kind filter ('references' instead of 'calls'),
// so `refs` returns true type-use references alongside call-site callers.
func (d *sqliteDriver) RefEdgesByToRefs(ctx context.Context, snapshotID string, toRefs []string) ([]graph.DependencyEdge, error) {
	if len(toRefs) == 0 {
		return nil, nil
	}
	var out []graph.DependencyEdge
	for start := 0; start < len(toRefs); start += callEdgesChunk {
		end := start + callEdgesChunk
		if end > len(toRefs) {
			end = len(toRefs)
		}
		chunk := toRefs[start:end]

		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, ref := range chunk {
			args = append(args, ref)
		}

		query := `SELECT ` + edgeCols + `
			FROM edges WHERE snapshot_id = ? AND kind = 'references' AND to_ref IN (` + placeholders + `)`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: ref edges by to_refs: %w", err)
		}
		for rows.Next() {
			e, err := scanEdgeRow(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan edge: %w", err)
			}
			out = append(out, e)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: ref edges by to_refs: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// EdgesByFromFiles returns every edge (any kind) whose from_file is in fromFiles,
// served by idx_edges_snapshot_fromfile. The IN-list is chunked so a large
// changed-path set never blows the bound-parameter limit; all matching edges are
// returned with Metadata populated (no dedupe). This is the targeted counterpart
// to ListEdges for the `context` op's changed-path edge slice.
func (d *sqliteDriver) EdgesByFromFiles(ctx context.Context, snapshotID string, fromFiles []string) ([]graph.DependencyEdge, error) {
	fromFiles = uniqueNonEmpty(fromFiles)
	if len(fromFiles) == 0 {
		return nil, nil
	}
	var out []graph.DependencyEdge
	for start := 0; start < len(fromFiles); start += callEdgesChunk {
		end := start + callEdgesChunk
		if end > len(fromFiles) {
			end = len(fromFiles)
		}
		chunk := fromFiles[start:end]

		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, f := range chunk {
			args = append(args, f)
		}

		// ORDER BY from_file, to_ref, kind mirrors ListEdges so the targeted slice
		// arrives in the SAME order the load-all filter produced.
		query := `SELECT ` + edgeCols + `
			FROM edges WHERE snapshot_id = ? AND from_file IN (` + placeholders + `)
			ORDER BY from_file, to_ref, kind`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: edges by from_files: %w", err)
		}
		for rows.Next() {
			e, err := scanEdgeRow(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan edge: %w", err)
			}
			out = append(out, e)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: edges by from_files: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

func (d *sqliteDriver) ListRoutes(ctx context.Context, snapshotID, role string) ([]graph.Route, error) {
	var (
		rows *sql.Rows
		err  error
	)
	const cols = `id, snapshot_id, repo_full_name, method, path_pattern, handler_file, role, source, confidence, metadata`
	if strings.TrimSpace(role) == "" {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+cols+` FROM routes WHERE snapshot_id = ? ORDER BY method, path_pattern`,
			snapshotID)
	} else {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+cols+` FROM routes WHERE snapshot_id = ? AND role = ? ORDER BY method, path_pattern`,
			snapshotID, role)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list routes: %w", err)
	}
	defer rows.Close()

	var out []graph.Route
	for rows.Next() {
		var (
			rt   graph.Route
			meta sql.NullString
		)
		if err := rows.Scan(&rt.ID, &rt.SnapshotID, &rt.RepoFullName, &rt.Method, &rt.PathPattern,
			&rt.HandlerFile, &rt.Role, &rt.Source, &rt.Confidence, &meta); err != nil {
			return nil, fmt.Errorf("store: scan route: %w", err)
		}
		if rt.Metadata, err = unmarshalJSONMap(meta.String); err != nil {
			return nil, fmt.Errorf("store: unmarshal route metadata: %w", err)
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

// ListFiles returns the indexed file rows of a snapshot (path/language/imports).
func (d *sqliteDriver) ListFiles(ctx context.Context, snapshotID string) ([]graph.File, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary
		 FROM files WHERE snapshot_id = ? ORDER BY path`,
		snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: list files: %w", err)
	}
	defer rows.Close()

	var out []graph.File
	for rows.Next() {
		var (
			f    graph.File
			imp  sql.NullString
			docS sql.NullString
		)
		if err := rows.Scan(&f.ID, &f.SnapshotID, &f.Path, &f.Language, &f.SizeBytes, &f.Hash, &imp, &docS); err != nil {
			return nil, fmt.Errorf("store: scan file: %w", err)
		}
		if f.Imports, err = unmarshalStrings(imp); err != nil {
			return nil, fmt.Errorf("store: unmarshal file imports: %w", err)
		}
		f.DocSummary = docS.String
		out = append(out, f)
	}
	return out, rows.Err()
}

// FilesByPaths returns the indexed file rows for the requested paths. It is the
// latency-sensitive counterpart to ListFiles for context/explain paths that only
// need imports and metadata for a few defining files.
func (d *sqliteDriver) FilesByPaths(ctx context.Context, snapshotID string, paths []string) ([]graph.File, error) {
	paths = uniqueNonEmpty(paths)
	if len(paths) == 0 {
		return nil, nil
	}
	var out []graph.File
	for start := 0; start < len(paths); start += symbolsChunk {
		end := start + symbolsChunk
		if end > len(paths) {
			end = len(paths)
		}
		chunk := paths[start:end]
		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, p := range chunk {
			args = append(args, p)
		}
		query := `SELECT id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary
			FROM files WHERE snapshot_id = ? AND path IN (` + placeholders + `)
			ORDER BY path`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: files by paths: %w", err)
		}
		for rows.Next() {
			var (
				f    graph.File
				imp  sql.NullString
				docS sql.NullString
			)
			if err := rows.Scan(&f.ID, &f.SnapshotID, &f.Path, &f.Language, &f.SizeBytes, &f.Hash, &imp, &docS); err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan file: %w", err)
			}
			if f.Imports, err = unmarshalStrings(imp); err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: unmarshal file imports: %w", err)
			}
			f.DocSummary = docS.String
			out = append(out, f)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: files by paths: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// ---- coverage --------------------------------------------------------------

const coverageCols = `id, snapshot_id, repo_full_name, symbol_ref, test_id, test_file, coverage_type, strength`

// SaveCoverage replaces the coverage rows of every snapshot referenced by `rows`
// and inserts the given runtime coverage facts, all inside one transaction so a
// re-import of the same snapshot is idempotent (the prior runtime facts for that
// snapshot are wiped and rebuilt).
func (d *sqliteDriver) SaveCoverage(ctx context.Context, rows []graph.Coverage) error {
	if len(rows) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin coverage tx: %w", err)
	}
	defer tx.Rollback()

	// Clear existing rows for each distinct snapshot before rebuilding.
	wiped := map[string]bool{}
	for i := range rows {
		sid := rows[i].SnapshotID
		if wiped[sid] {
			continue
		}
		wiped[sid] = true
		if _, err := tx.ExecContext(ctx, `DELETE FROM coverage WHERE snapshot_id = ?`, sid); err != nil {
			return fmt.Errorf("store: clear coverage: %w", err)
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO coverage (`+coverageCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare coverage insert: %w", err)
	}
	defer stmt.Close()
	for i := range rows {
		c := &rows[i]
		id := c.ID
		if id == "" {
			id = uuid.NewString()
		}
		if _, err := stmt.ExecContext(ctx, id, c.SnapshotID, c.RepoFullName, c.SymbolRef,
			c.TestID, c.TestFile, c.CoverageType, c.Strength); err != nil {
			return fmt.Errorf("store: save coverage for %s: %w", c.SymbolRef, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit coverage: %w", err)
	}
	return nil
}

// ListCoverage returns the coverage rows for a snapshot, optionally filtered to a
// single symbol name (empty symbolName = all rows).
func (d *sqliteDriver) ListCoverage(ctx context.Context, snapshotID, symbolName string) ([]graph.Coverage, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(symbolName) == "" {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+coverageCols+` FROM coverage WHERE snapshot_id = ? ORDER BY symbol_ref`,
			snapshotID)
	} else {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+coverageCols+` FROM coverage WHERE snapshot_id = ? AND symbol_ref = ? ORDER BY symbol_ref`,
			snapshotID, symbolName)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list coverage: %w", err)
	}
	defer rows.Close()

	var out []graph.Coverage
	for rows.Next() {
		var c graph.Coverage
		if err := rows.Scan(&c.ID, &c.SnapshotID, &c.RepoFullName, &c.SymbolRef,
			&c.TestID, &c.TestFile, &c.CoverageType, &c.Strength); err != nil {
			return nil, fmt.Errorf("store: scan coverage: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- embeddings (optional semantic-search substrate) -----------------------

// SaveEmbeddings replaces every embedding row for snapshotID (delete-then-insert)
// and persists the given per-symbol vectors, all in one transaction so a re-index
// with vectors enabled is idempotent. Vectors are stored little-endian via the
// shared encodeVector.
func (d *sqliteDriver) SaveEmbeddings(ctx context.Context, snapshotID string, embs []graph.SymbolEmbedding) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin embeddings tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM embeddings WHERE snapshot_id = ?`, snapshotID); err != nil {
		return fmt.Errorf("store: clear embeddings: %w", err)
	}

	if len(embs) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO embeddings (`+embeddingsCols+`)
			VALUES (?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("store: prepare embeddings insert: %w", err)
		}
		defer stmt.Close()
		for i := range embs {
			e := &embs[i]
			if _, err := stmt.ExecContext(ctx, snapshotID, e.SymbolID, e.Dim, encodeVector(e.Vector)); err != nil {
				return fmt.Errorf("store: save embedding for %s: %w", e.SymbolID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit embeddings: %w", err)
	}
	return nil
}

// NearestSymbols loads the snapshot's embedding rows, decodes each vector, and
// ranks them by cosine (== dot, both L2-normalized) against vec via the shared
// brute-force rankEmbeddings. (Brute-force is acceptable at Atlas's per-snapshot
// symbol counts on both tiers.)
func (d *sqliteDriver) NearestSymbols(ctx context.Context, snapshotID string, vec []float32, limit int, minScore float64) ([]graph.ScoredSymbol, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT symbol_id, vec FROM embeddings WHERE snapshot_id = ?`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: list embeddings: %w", err)
	}
	defer rows.Close()

	var (
		ids  []string
		vecs [][]float32
	)
	for rows.Next() {
		var (
			id   string
			blob []byte
		)
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("store: scan embedding: %w", err)
		}
		ids = append(ids, id)
		vecs = append(vecs, decodeVector(blob))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rankEmbeddings(vec, ids, vecs, limit, minScore), nil
}

// compile-time assertion that sqliteDriver satisfies the contract.
var _ StorageDriver = (*sqliteDriver)(nil)
