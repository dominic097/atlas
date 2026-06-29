package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestSQLiteMigrateVersionGate verifies Migrate stamps PRAGMA user_version and,
// once the schema is current, SKIPS the DDL replay on subsequent opens (the
// per-invocation startup optimization). It proves the skip directly: dropping a
// table and re-migrating must NOT recreate it, because the version gate short-
// circuits before the CREATE statements.
func TestSQLiteMigrateVersionGate(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	// First migrate on a fresh DB creates the schema and stamps the version.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	sd, ok := d.(*sqliteDriver)
	if !ok {
		t.Fatalf("expected *sqliteDriver, got %T", d)
	}
	var ver int
	if err := sd.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&ver); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if ver != sqliteSchemaVersion {
		t.Fatalf("user_version = %d, want %d", ver, sqliteSchemaVersion)
	}

	// Drop a table, then Migrate again. Because the version is already current,
	// the gate must skip DDL replay, so the table stays gone.
	if _, err := sd.db.ExecContext(ctx, "DROP TABLE coverage"); err != nil {
		t.Fatalf("drop coverage: %v", err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := sd.db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='coverage'").Scan(&n); err != nil {
		t.Fatalf("count coverage table: %v", err)
	}
	if n != 0 {
		t.Fatalf("version gate did NOT skip DDL: coverage table was recreated (n=%d)", n)
	}
}

// TestSQLiteMigrateCreatesOnStaleVersion verifies that when user_version is below
// the current schema version (e.g. a DB created before the gate existed), Migrate
// still applies the schema — backward compatibility for pre-gate databases.
func TestSQLiteMigrateCreatesOnStaleVersion(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	sd := d.(*sqliteDriver)

	// Simulate a pre-gate DB: schema absent, user_version 0.
	if _, err := sd.db.ExecContext(ctx, "PRAGMA user_version=0"); err != nil {
		t.Fatalf("reset version: %v", err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// The schema must now exist and the version must be stamped.
	var n int
	if err := sd.db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='symbols'").Scan(&n); err != nil {
		t.Fatalf("count symbols table: %v", err)
	}
	if n != 1 {
		t.Fatalf("Migrate did not create schema on stale version (symbols n=%d)", n)
	}
	var ver int
	sd.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&ver)
	if ver != sqliteSchemaVersion {
		t.Fatalf("user_version after migrate = %d, want %d", ver, sqliteSchemaVersion)
	}
}

// TestSQLiteMigrateDropsOnVersionBump verifies the dev-stage drop-fresh path: when
// an existing DB carries a non-zero version BELOW the current one (an incompatible
// older compact-schema shape), Migrate DROPS the old tables and recreates them
// clean, discarding stale rows. The local .atlas.db is a derived cache, so the
// next reindex repopulates — no in-place data migration.
func TestSQLiteMigrateDropsOnVersionBump(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	sd := d.(*sqliteDriver)

	// Simulate an older-but-gated DB: apply current schema, then stamp an EARLIER
	// version and seed a row that a fresh rebuild must discard.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("seed Migrate: %v", err)
	}
	if _, err := sd.db.ExecContext(ctx,
		"INSERT INTO repos (id, full_name) VALUES ('stale', 'old/repo')"); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if sqliteSchemaVersion < 2 {
		t.Skip("needs a non-zero prior version to exercise the drop branch")
	}
	if _, err := sd.db.ExecContext(ctx,
		fmt.Sprintf("PRAGMA user_version=%d", sqliteSchemaVersion-1)); err != nil {
		t.Fatalf("stamp prior version: %v", err)
	}

	// Re-migrate: ver (current-1) != 0 and != current -> drop + recreate fresh.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("upgrade Migrate: %v", err)
	}
	var n int
	if err := sd.db.QueryRowContext(ctx, "SELECT count(*) FROM repos").Scan(&n); err != nil {
		t.Fatalf("count repos after upgrade: %v", err)
	}
	if n != 0 {
		t.Fatalf("drop-fresh did not discard stale rows: repos count = %d, want 0", n)
	}
	var ver int
	sd.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&ver)
	if ver != sqliteSchemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", ver, sqliteSchemaVersion)
	}
	// The dead index must be gone after the fresh rebuild (proves new DDL applied).
	if err := sd.db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_symbols_node'").Scan(&n); err != nil {
		t.Fatalf("count idx_symbols_node: %v", err)
	}
	if n != 0 {
		t.Fatalf("compact schema not applied: idx_symbols_node still present")
	}
}
