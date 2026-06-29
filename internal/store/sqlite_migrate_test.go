package store

import (
	"context"
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
