package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// postgresDriver is the HOSTED tier StorageDriver, backed by lib/pq.
//
// It is the org-wide counterpart of sqliteDriver: the SAME StorageDriver
// contract over Postgres instead of SQLite. The schema (postgres_schema.go) keys
// repos by (scope, lower(full_name)) and snapshots by (repo_id, commit_sha) just
// like the local tier, so multi-tenant isolation falls out of scoping every
// repo-listing read. Unlike SQLite it advertises a durable queue, cross-scope
// reads and concurrent writes (Capabilities below), and lets the connection pool
// fan out (no single-writer mutex): Postgres serializes writes itself and
// SaveSnapshot's per-(repo,commit) transaction is idempotent under concurrency.
//
// Placeholders are $1,$2,... (NOT ?). JSONB columns scan as []byte and are
// decoded by the shared unmarshalJSONMap; TEXT[] imports use pq.Array; TIMESTAMPTZ
// columns bind/scan native time.Time so reads return the SAME graph shape as the
// SQLite tier (node_id + decoded Metadata + Imports + recv_type/qualified_ref
// preserved verbatim).
type postgresDriver struct {
	dsn string
	db  *sql.DB
}

func openPostgres(ctx context.Context, dsn string) (StorageDriver, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("store: postgres DSN is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open postgres: %w", err)
	}
	// Hosted tier fans writes out across the pool (ConcurrentWrite=true).
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping postgres: %w", err)
	}
	return &postgresDriver{dsn: dsn, db: db}, nil
}

func (d *postgresDriver) Migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, schemaPostgres); err != nil {
		return fmt.Errorf("store: migrate postgres: %w", err)
	}
	return nil
}

func (d *postgresDriver) Dialect() string { return "postgres" }

func (d *postgresDriver) Capabilities() Capabilities {
	return Capabilities{DurableQueue: true, CrossScope: true, ConcurrentWrite: true, PushReindex: true}
}

func (d *postgresDriver) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

// ---- repos -----------------------------------------------------------------

func (d *postgresDriver) EnsureRepo(ctx context.Context, r *graph.Repo) (*graph.Repo, error) {
	if r == nil {
		return nil, fmt.Errorf("store: repo is required")
	}

	scope := r.Scope
	// Resolve an existing repo by (scope, full_name) so callers without an ID still upsert.
	var existingID string
	err := d.db.QueryRowContext(ctx,
		`SELECT id FROM repos WHERE scope = $1 AND lower(full_name) = lower($2) LIMIT 1`,
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
	// already hold data — mirror the SQLite tier's COALESCE baseline-preservation:
	// an empty incoming value falls back to the stored one (NULLIF('',...) → NULL →
	// COALESCE picks the existing column). languages is JSONB, so compare its text
	// form against '' / '{}' to decide whether to keep the baseline.
	_, err = d.db.ExecContext(ctx, `
		INSERT INTO repos (id, full_name, root, default_branch, status, languages, last_commit, last_indexed_at, scope)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			full_name       = COALESCE(NULLIF(excluded.full_name, ''), repos.full_name),
			root            = COALESCE(NULLIF(excluded.root, ''), repos.root),
			default_branch  = COALESCE(NULLIF(excluded.default_branch, ''), repos.default_branch),
			status          = COALESCE(NULLIF(excluded.status, ''), repos.status),
			languages       = CASE WHEN excluded.languages::text IN ('', '{}') THEN repos.languages ELSE excluded.languages END,
			last_commit     = COALESCE(NULLIF(excluded.last_commit, ''), repos.last_commit),
			last_indexed_at = COALESCE(excluded.last_indexed_at, repos.last_indexed_at),
			scope           = excluded.scope
	`,
		id, r.FullName, r.Root, r.DefaultBranch, status, langs, r.LastCommit,
		timePtrUTC(r.LastIndexedAt), scope,
	)
	if err != nil {
		return nil, fmt.Errorf("store: ensure repo: %w", err)
	}

	out := *r
	out.ID = id
	out.Status = graph.IndexStatus(status)
	return &out, nil
}

func (d *postgresDriver) ListRepos(ctx context.Context, scope string) ([]graph.Repo, error) {
	var (
		rows *sql.Rows
		err  error
	)
	const cols = `id, full_name, root, default_branch, status, languages, last_commit, last_indexed_at, scope`
	if scope == "" {
		rows, err = d.db.QueryContext(ctx, `SELECT `+cols+` FROM repos ORDER BY lower(full_name)`)
	} else {
		rows, err = d.db.QueryContext(ctx, `SELECT `+cols+` FROM repos WHERE scope = $1 ORDER BY lower(full_name)`, scope)
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
			langs   []byte
			lastIdx sql.NullTime
		)
		if err := rows.Scan(&r.ID, &r.FullName, &r.Root, &r.DefaultBranch, &status, &langs, &r.LastCommit, &lastIdx, &r.Scope); err != nil {
			return nil, fmt.Errorf("store: scan repo: %w", err)
		}
		r.Status = graph.IndexStatus(status)
		if r.Languages, err = unmarshalLanguagesBytes(langs); err != nil {
			return nil, fmt.Errorf("store: unmarshal languages: %w", err)
		}
		r.LastIndexedAt = nullTimePtr(lastIdx)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- snapshots -------------------------------------------------------------

// SaveSnapshot inserts the snapshot row plus all files/symbols/edges/routes
// inside ONE transaction. It is idempotent on (repo_id, commit_sha): re-saving
// the same commit reuses the snapshot id and rebuilds its child rows.
func (d *postgresDriver) SaveSnapshot(ctx context.Context, s *graph.Snapshot, files []graph.File,
	symbols []graph.CodeSymbol, edges []graph.DependencyEdge, routes []graph.Route) error {
	if s == nil {
		return fmt.Errorf("store: snapshot is required")
	}

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
			`SELECT id FROM snapshots WHERE repo_id = $1 AND commit_sha = $2 LIMIT 1`,
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11)
		ON CONFLICT (id) DO UPDATE SET
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
		s.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("store: save snapshot: %w", err)
	}

	// Rebuild child rows from scratch.
	for _, table := range []string{"files", "symbols", "edges", "routes"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE snapshot_id = $1`, s.ID); err != nil {
			return fmt.Errorf("store: clear %s: %w", table, err)
		}
	}

	fileStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO files (id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)
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
		// pq.Array(nil) binds SQL NULL, which the TEXT[] NOT NULL column rejects;
		// coerce a nil/absent imports list to an empty (non-null) array, mirroring
		// the SQLite tier's "[]" default.
		imports := f.Imports
		if imports == nil {
			imports = []string{}
		}
		if _, err := fileStmt.ExecContext(ctx, id, s.ID, f.Path, f.Language, f.SizeBytes, f.Hash, pq.Array(imports), f.DocSummary); err != nil {
			return fmt.Errorf("store: save file %s: %w", f.Path, err)
		}
	}

	symStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO symbols (id, snapshot_id, node_id, repo_id, path, language, kind, name, signature, doc, start_line, end_line, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb)`)
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)`)
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)`)
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

func (d *postgresDriver) UpdateSnapshotMetadata(ctx context.Context, snapshotID string, metadata graph.JSONBMap) error {
	if strings.TrimSpace(snapshotID) == "" {
		return fmt.Errorf("store: snapshot id is required")
	}
	meta, err := marshalJSONMap(metadata)
	if err != nil {
		return fmt.Errorf("store: marshal snapshot metadata: %w", err)
	}
	res, err := d.db.ExecContext(ctx, `UPDATE snapshots SET metadata = $1::jsonb WHERE id = $2`, meta, snapshotID)
	if err != nil {
		return fmt.Errorf("store: update snapshot metadata: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: snapshot %q not found", snapshotID)
	}
	return nil
}

func scanSnapshotPG(sc interface{ Scan(...any) error }) (graph.Snapshot, error) {
	var (
		s         graph.Snapshot
		meta      []byte
		createdAt time.Time
	)
	if err := sc.Scan(&s.ID, &s.RepoID, &s.CommitSHA, &s.Branch, &s.CommitRange,
		&s.FileCount, &s.SymbolCount, &s.EdgeCount, &s.RouteCount, &meta, &createdAt); err != nil {
		return graph.Snapshot{}, err
	}
	m, err := unmarshalJSONMap(meta)
	if err != nil {
		return graph.Snapshot{}, err
	}
	s.Metadata = m
	s.CreatedAt = createdAt.UTC()
	return s, nil
}

func (d *postgresDriver) LatestSnapshot(ctx context.Context, repoID string) (*graph.Snapshot, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT `+snapshotCols+` FROM snapshots WHERE repo_id = $1 ORDER BY created_at DESC LIMIT 1`,
		repoID,
	)
	s, err := scanSnapshotPG(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: latest snapshot: %w", err)
	}
	return &s, nil
}

func (d *postgresDriver) ListSnapshots(ctx context.Context, repoID string, limit int) ([]graph.Snapshot, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT `+snapshotCols+` FROM snapshots WHERE repo_id = $1 ORDER BY created_at DESC LIMIT $2`,
		repoID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list snapshots: %w", err)
	}
	defer rows.Close()

	var out []graph.Snapshot
	for rows.Next() {
		s, err := scanSnapshotPG(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan snapshot: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- graph reads -----------------------------------------------------------

// scanSymbolRowPG decodes one symbols row into a graph.CodeSymbol (node_id +
// metadata included), matching ListSymbols exactly. JSONB metadata scans as
// []byte.
func scanSymbolRowPG(sc interface{ Scan(...any) error }) (graph.CodeSymbol, error) {
	var (
		sym    graph.CodeSymbol
		nodeID string
		meta   []byte
	)
	if err := sc.Scan(&sym.ID, &sym.SnapshotID, &nodeID, &sym.RepoID, &sym.Path, &sym.Language,
		&sym.Kind, &sym.Name, &sym.Signature, &sym.Doc, &sym.StartLine, &sym.EndLine, &meta); err != nil {
		return graph.CodeSymbol{}, err
	}
	sym.NodeID = graph.NodeID(nodeID)
	m, err := unmarshalJSONMap(meta)
	if err != nil {
		return graph.CodeSymbol{}, err
	}
	sym.Metadata = m
	return sym, nil
}

// scanEdgeRowPG decodes one edges row into a graph.DependencyEdge (metadata
// included), matching ListEdges exactly.
func scanEdgeRowPG(sc interface{ Scan(...any) error }) (graph.DependencyEdge, error) {
	var (
		e    graph.DependencyEdge
		kind string
		meta []byte
	)
	if err := sc.Scan(&e.ID, &e.SnapshotID, &e.FromFile, &e.FromSymbol, &e.ToRef, &kind, &e.Language, &e.Line, &meta); err != nil {
		return graph.DependencyEdge{}, err
	}
	e.Kind = graph.EdgeKind(kind)
	m, err := unmarshalJSONMap(meta)
	if err != nil {
		return graph.DependencyEdge{}, err
	}
	e.Metadata = m
	return e, nil
}

func (d *postgresDriver) ListSymbols(ctx context.Context, snapshotID string) ([]graph.CodeSymbol, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+symbolCols+`
		FROM symbols WHERE snapshot_id = $1
		ORDER BY path, start_line, name`,
		snapshotID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list symbols: %w", err)
	}
	defer rows.Close()

	var out []graph.CodeSymbol
	for rows.Next() {
		sym, err := scanSymbolRowPG(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol: %w", err)
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

func (d *postgresDriver) ListEdges(ctx context.Context, snapshotID string) ([]graph.DependencyEdge, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+edgeCols+`
		FROM edges WHERE snapshot_id = $1
		ORDER BY from_file, to_ref, kind`,
		snapshotID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list edges: %w", err)
	}
	defer rows.Close()

	var out []graph.DependencyEdge
	for rows.Next() {
		e, err := scanEdgeRowPG(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan edge: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SymbolsByName returns symbols whose name matches exactly, served by the
// idx_symbols_snapshot_name index — the indexed seed for impact's reverse-BFS.
func (d *postgresDriver) SymbolsByName(ctx context.Context, snapshotID, name string) ([]graph.CodeSymbol, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+symbolCols+`
		FROM symbols WHERE snapshot_id = $1 AND name = $2
		ORDER BY path, start_line`,
		snapshotID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("store: symbols by name: %w", err)
	}
	defer rows.Close()

	var out []graph.CodeSymbol
	for rows.Next() {
		sym, err := scanSymbolRowPG(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol: %w", err)
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

// pgChunk is the IN-list batch size for the chunked readers. Postgres allows up
// to 65535 bound params per statement; 1000 names/refs + the snapshot_id stays
// well under that while keeping round-trips low.
const pgChunk = 1000

// SymbolsByNames returns every symbol whose name is in `names`, served by
// idx_symbols_snapshot_name. The IN-list is chunked so a large blast radius never
// blows the bound-parameter limit; all matching rows are returned with node_id +
// decoded metadata (no dedupe), exactly like SymbolsByName.
func (d *postgresDriver) SymbolsByNames(ctx context.Context, snapshotID string, names []string) ([]graph.CodeSymbol, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var out []graph.CodeSymbol
	for start := 0; start < len(names); start += pgChunk {
		end := start + pgChunk
		if end > len(names) {
			end = len(names)
		}
		chunk := names[start:end]

		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, n := range chunk {
			args = append(args, n)
		}

		// ORDER BY name, path, start_line so that, within any single name, rows
		// arrive in the SAME order SymbolsByName (path, start_line) yields — the
		// per-name candidate ordering callers depend on for deterministic "first
		// candidate" selection. The leading name keeps each name's rows contiguous.
		query := `SELECT ` + symbolCols + `
			FROM symbols WHERE snapshot_id = $1 AND name IN (` + inPlaceholders(2, len(chunk)) + `)
			ORDER BY name, path, start_line`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: symbols by names: %w", err)
		}
		for rows.Next() {
			sym, err := scanSymbolRowPG(rows)
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
func (d *postgresDriver) SymbolsByPath(ctx context.Context, snapshotID, path string) ([]graph.CodeSymbol, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT `+symbolCols+`
		FROM symbols WHERE snapshot_id = $1 AND path = $2
		ORDER BY start_line, name`,
		snapshotID, path,
	)
	if err != nil {
		return nil, fmt.Errorf("store: symbols by path: %w", err)
	}
	defer rows.Close()

	var out []graph.CodeSymbol
	for rows.Next() {
		sym, err := scanSymbolRowPG(rows)
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
func (d *postgresDriver) SymbolsByIDs(ctx context.Context, snapshotID string, ids []string) ([]graph.CodeSymbol, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []graph.CodeSymbol
	for start := 0; start < len(ids); start += pgChunk {
		end := start + pgChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, id := range chunk {
			args = append(args, id)
		}

		query := `SELECT ` + symbolCols + `
			FROM symbols WHERE snapshot_id = $1 AND id IN (` + inPlaceholders(2, len(chunk)) + `)
			ORDER BY path, start_line, name`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: symbols by ids: %w", err)
		}
		for rows.Next() {
			sym, err := scanSymbolRowPG(rows)
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

// CallEdgesByToRefs returns every "calls" edge whose to_ref is in toRefs, served
// by idx_edges_snapshot_toref. The IN-list is chunked so a large blast radius
// never blows the bound-parameter limit; all matching edges are returned with
// Metadata populated (no dedupe).
func (d *postgresDriver) CallEdgesByToRefs(ctx context.Context, snapshotID string, toRefs []string) ([]graph.DependencyEdge, error) {
	return d.edgesIn(ctx, snapshotID, "calls", "to_ref", toRefs)
}

func (d *postgresDriver) CallEdgesByFromSymbols(ctx context.Context, snapshotID string, fromSymbols []string) ([]graph.DependencyEdge, error) {
	return d.edgesIn(ctx, snapshotID, "calls", "from_symbol", fromSymbols)
}

// RefEdgesByToRefs returns every "references" (type-use) edge whose to_ref is in
// toRefs, served by idx_edges_snapshot_toref. Identical to CallEdgesByToRefs
// except for the kind filter, so `refs` returns true type-use references.
func (d *postgresDriver) RefEdgesByToRefs(ctx context.Context, snapshotID string, toRefs []string) ([]graph.DependencyEdge, error) {
	return d.edgesIn(ctx, snapshotID, "references", "to_ref", toRefs)
}

// edgesIn is the shared chunked IN-list reader behind CallEdgesByToRefs /
// CallEdgesByFromSymbols / RefEdgesByToRefs. kind and column are fixed
// identifiers ("calls"|"references", "to_ref"|"from_symbol"), never user input,
// so they are interpolated directly.
func (d *postgresDriver) edgesIn(ctx context.Context, snapshotID, kind, column string, values []string) ([]graph.DependencyEdge, error) {
	if len(values) == 0 {
		return nil, nil
	}
	var out []graph.DependencyEdge
	for start := 0; start < len(values); start += pgChunk {
		end := start + pgChunk
		if end > len(values) {
			end = len(values)
		}
		chunk := values[start:end]

		args := make([]any, 0, len(chunk)+2)
		args = append(args, snapshotID, kind)
		for _, v := range chunk {
			args = append(args, v)
		}

		query := `SELECT ` + edgeCols + `
			FROM edges WHERE snapshot_id = $1 AND kind = $2 AND ` + column + ` IN (` + inPlaceholders(3, len(chunk)) + `)`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: %s edges by %s: %w", kind, column, err)
		}
		for rows.Next() {
			e, err := scanEdgeRowPG(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan edge: %w", err)
			}
			out = append(out, e)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: %s edges by %s: %w", kind, column, err)
		}
		rows.Close()
	}
	return out, nil
}

// EdgesByFromFiles returns every edge (any kind) whose from_file is in fromFiles,
// served by idx_edges_snapshot_fromfile. The IN-list is chunked so a large
// changed-path set never blows the bound-parameter limit; all matching edges are
// returned with Metadata populated (no dedupe). Unlike edgesIn there is no kind
// filter — the `context` op wants every edge leaving the changed paths.
func (d *postgresDriver) EdgesByFromFiles(ctx context.Context, snapshotID string, fromFiles []string) ([]graph.DependencyEdge, error) {
	fromFiles = uniqueNonEmpty(fromFiles)
	if len(fromFiles) == 0 {
		return nil, nil
	}
	var out []graph.DependencyEdge
	for start := 0; start < len(fromFiles); start += pgChunk {
		end := start + pgChunk
		if end > len(fromFiles) {
			end = len(fromFiles)
		}
		chunk := fromFiles[start:end]

		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, f := range chunk {
			args = append(args, f)
		}

		// ORDER BY from_file, to_ref, kind mirrors ListEdges so the targeted slice
		// arrives in the SAME order the load-all filter produced.
		query := `SELECT ` + edgeCols + `
			FROM edges WHERE snapshot_id = $1 AND from_file IN (` + inPlaceholders(2, len(chunk)) + `)
			ORDER BY from_file, to_ref, kind`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: edges by from_files: %w", err)
		}
		for rows.Next() {
			e, err := scanEdgeRowPG(rows)
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

func (d *postgresDriver) ListRoutes(ctx context.Context, snapshotID, role string) ([]graph.Route, error) {
	var (
		rows *sql.Rows
		err  error
	)
	const cols = `id, snapshot_id, repo_full_name, method, path_pattern, handler_file, role, source, confidence, metadata`
	if strings.TrimSpace(role) == "" {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+cols+` FROM routes WHERE snapshot_id = $1 ORDER BY method, path_pattern`,
			snapshotID)
	} else {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+cols+` FROM routes WHERE snapshot_id = $1 AND role = $2 ORDER BY method, path_pattern`,
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
			meta []byte
		)
		if err := rows.Scan(&rt.ID, &rt.SnapshotID, &rt.RepoFullName, &rt.Method, &rt.PathPattern,
			&rt.HandlerFile, &rt.Role, &rt.Source, &rt.Confidence, &meta); err != nil {
			return nil, fmt.Errorf("store: scan route: %w", err)
		}
		if rt.Metadata, err = unmarshalJSONMap(meta); err != nil {
			return nil, fmt.Errorf("store: unmarshal route metadata: %w", err)
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

// ListFiles returns the indexed file rows of a snapshot (path/language/imports).
func (d *postgresDriver) ListFiles(ctx context.Context, snapshotID string) ([]graph.File, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary
		 FROM files WHERE snapshot_id = $1 ORDER BY path`,
		snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: list files: %w", err)
	}
	defer rows.Close()

	var out []graph.File
	for rows.Next() {
		var (
			f       graph.File
			imports pq.StringArray
		)
		if err := rows.Scan(&f.ID, &f.SnapshotID, &f.Path, &f.Language, &f.SizeBytes, &f.Hash, &imports, &f.DocSummary); err != nil {
			return nil, fmt.Errorf("store: scan file: %w", err)
		}
		if len(imports) > 0 {
			f.Imports = []string(imports)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FilesByPaths returns the indexed file rows for the requested paths. It is the
// latency-sensitive counterpart to ListFiles for context/explain paths that only
// need imports and metadata for a few defining files.
func (d *postgresDriver) FilesByPaths(ctx context.Context, snapshotID string, paths []string) ([]graph.File, error) {
	paths = uniqueNonEmpty(paths)
	if len(paths) == 0 {
		return nil, nil
	}
	var out []graph.File
	for start := 0; start < len(paths); start += pgChunk {
		end := start + pgChunk
		if end > len(paths) {
			end = len(paths)
		}
		chunk := paths[start:end]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, snapshotID)
		for _, p := range chunk {
			args = append(args, p)
		}
		query := `SELECT id, snapshot_id, path, language, size_bytes, hash, imports, doc_summary
			FROM files WHERE snapshot_id = $1 AND path IN (` + inPlaceholders(2, len(chunk)) + `)
			ORDER BY path`
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: files by paths: %w", err)
		}
		for rows.Next() {
			var (
				f       graph.File
				imports pq.StringArray
			)
			if err := rows.Scan(&f.ID, &f.SnapshotID, &f.Path, &f.Language, &f.SizeBytes, &f.Hash, &imports, &f.DocSummary); err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: scan file: %w", err)
			}
			if len(imports) > 0 {
				f.Imports = []string(imports)
			}
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

// ---- postgres helpers ------------------------------------------------------

// inPlaceholders builds "$start,$start+1,...,$start+n-1" for an IN-list, given
// the 1-based index of the first placeholder.
func inPlaceholders(start, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('$')
		b.WriteString(strconv.Itoa(start + i))
	}
	return b.String()
}

// unmarshalLanguagesBytes parses the JSONB languages column ([]byte) back into a
// map[string]int, the Postgres counterpart of unmarshalLanguages (which takes the
// SQLite TEXT form). Empty / "{}" yields a nil map.
func unmarshalLanguagesBytes(b []byte) (map[string]int, error) {
	if len(b) == 0 || string(b) == "{}" {
		return nil, nil
	}
	out := map[string]int{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// timePtrUTC normalizes a *time.Time to a UTC time bind value, or nil.
func timePtrUTC(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

// nullTimePtr lifts a scanned sql.NullTime into a *time.Time (UTC) or nil.
func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	ts := v.Time.UTC()
	return &ts
}

// ---- coverage --------------------------------------------------------------

// SaveCoverage replaces the coverage rows of every snapshot referenced by `rows`
// and inserts the given runtime coverage facts, all inside one transaction so a
// re-import of the same snapshot is idempotent.
func (d *postgresDriver) SaveCoverage(ctx context.Context, rows []graph.Coverage) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin coverage tx: %w", err)
	}
	defer tx.Rollback()

	wiped := map[string]bool{}
	for i := range rows {
		sid := rows[i].SnapshotID
		if wiped[sid] {
			continue
		}
		wiped[sid] = true
		if _, err := tx.ExecContext(ctx, `DELETE FROM coverage WHERE snapshot_id = $1`, sid); err != nil {
			return fmt.Errorf("store: clear coverage: %w", err)
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO coverage (`+coverageCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)
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
func (d *postgresDriver) ListCoverage(ctx context.Context, snapshotID, symbolName string) ([]graph.Coverage, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(symbolName) == "" {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+coverageCols+` FROM coverage WHERE snapshot_id = $1 ORDER BY symbol_ref`,
			snapshotID)
	} else {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+coverageCols+` FROM coverage WHERE snapshot_id = $1 AND symbol_ref = $2 ORDER BY symbol_ref`,
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
// and persists the given per-symbol vectors in one transaction. Vectors are
// stored as BYTEA via the shared little-endian encodeVector — the SAME wire
// encoding as the SQLite tier, so the two drivers are interchangeable.
func (d *postgresDriver) SaveEmbeddings(ctx context.Context, snapshotID string, embs []graph.SymbolEmbedding) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin embeddings tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM embeddings WHERE snapshot_id = $1`, snapshotID); err != nil {
		return fmt.Errorf("store: clear embeddings: %w", err)
	}

	if len(embs) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO embeddings (`+embeddingsCols+`)
			VALUES ($1, $2, $3, $4)`)
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

// NearestSymbols loads the snapshot's embedding rows, decodes each BYTEA vector,
// and ranks them by cosine (== dot, both L2-normalized) against vec via the
// shared brute-force rankEmbeddings. (Brute-force is acceptable at Atlas's
// per-snapshot symbol counts on both tiers.)
func (d *postgresDriver) NearestSymbols(ctx context.Context, snapshotID string, vec []float32, limit int, minScore float64) ([]graph.ScoredSymbol, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT symbol_id, vec FROM embeddings WHERE snapshot_id = $1`, snapshotID)
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

// compile-time assertion that postgresDriver satisfies the contract.
var _ StorageDriver = (*postgresDriver)(nil)
