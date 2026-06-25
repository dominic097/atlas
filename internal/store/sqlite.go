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

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
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
	// _pragma URL params apply per-connection on the modernc driver.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
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

func (d *sqliteDriver) Migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, schemaSQLite); err != nil {
		return fmt.Errorf("store: migrate sqlite: %w", err)
	}
	return nil
}

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

	fileStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO files (id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare file insert: %w", err)
	}
	defer fileStmt.Close()
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
		if _, err := fileStmt.ExecContext(ctx, id, s.ID, f.Path, f.Language, f.SizeBytes, f.Hash, imp, f.DocSummary); err != nil {
			return fmt.Errorf("store: save file %s: %w", f.Path, err)
		}
	}

	symStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO symbols (id, snapshot_id, node_id, repo_id, path, language, kind, name, signature, doc, start_line, end_line, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare symbol insert: %w", err)
	}
	defer symStmt.Close()
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
		if _, err := symStmt.ExecContext(ctx, id, s.ID, string(sym.NodeID), sym.RepoID, sym.Path, sym.Language,
			sym.Kind, sym.Name, sym.Signature, sym.Doc, sym.StartLine, sym.EndLine, m); err != nil {
			return fmt.Errorf("store: save symbol %s: %w", sym.Name, err)
		}
	}

	edgeStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO edges (id, snapshot_id, from_file, from_symbol, to_ref, kind, language, line, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare edge insert: %w", err)
	}
	defer edgeStmt.Close()
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
		if _, err := edgeStmt.ExecContext(ctx, id, s.ID, e.FromFile, e.FromSymbol, e.ToRef, string(e.Kind), e.Language, e.Line, m); err != nil {
			return fmt.Errorf("store: save edge %s -> %s: %w", e.FromFile, e.ToRef, err)
		}
	}

	routeStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO routes (id, snapshot_id, repo_full_name, method, path_pattern, handler_file, role, source, confidence, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare route insert: %w", err)
	}
	defer routeStmt.Close()
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
		if _, err := routeStmt.ExecContext(ctx, id, s.ID, rt.RepoFullName, rt.Method, rt.PathPattern,
			rt.HandlerFile, rt.Role, rt.Source, rt.Confidence, m); err != nil {
			return fmt.Errorf("store: save route %s %s: %w", rt.Method, rt.PathPattern, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit snapshot: %w", err)
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

// compile-time assertion that sqliteDriver satisfies the contract.
var _ StorageDriver = (*sqliteDriver)(nil)
