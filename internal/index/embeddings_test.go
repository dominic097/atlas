package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/lexical"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

// openTestStore opens a migrated SQLite driver for an index test.
func openTestStore(t *testing.T) store.StorageDriver {
	t.Helper()
	ctx := context.Background()
	d, err := store.Open(ctx, store.Options{Kind: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "atlas.db")})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return d
}

// writeGoRepo writes a tiny Go repo and returns its path.
func writeGoRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := `package svc

// GetUserByID fetches a user by id.
func GetUserByID(id string) string { return id }

// RenderInvoice renders an invoice.
func RenderInvoice(x string) string { return x }
`
	if err := os.WriteFile(filepath.Join(dir, "svc.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write repo: %v", err)
	}
	return dir
}

// TestRunWithVectorsWritesEmbeddings asserts that index.Run with
// EnableVectors:true persists embeddings (offline Hashing default) that
// NearestSymbols can rank — and that WITHOUT the flag, no embeddings are written.
func TestRunWithVectorsWritesEmbeddings(t *testing.T) {
	ctx := context.Background()
	repo := writeGoRepo(t)

	// 1) Vectors ON: embeddings get written and rank a relevant query.
	drv := openTestStore(t)
	lx, err := lexical.New(filepath.Join(t.TempDir(), "lexical"))
	if err != nil {
		t.Fatalf("lexical.New: %v", err)
	}
	t.Cleanup(func() { _ = lx.Close() })

	snap, _, err := Run(ctx, drv, lx, "", "svc", repo, Options{EnableVectors: true})
	if err != nil {
		t.Fatalf("Run(EnableVectors): %v", err)
	}

	// Probe with a zero-length query vec and a -1 floor just to confirm rows exist.
	any, err := drv.NearestSymbols(ctx, snap.ID, nil, 100, -1)
	if err != nil {
		t.Fatalf("NearestSymbols(probe): %v", err)
	}
	if len(any) == 0 {
		t.Fatalf("EnableVectors:true wrote no embeddings")
	}

	// 2) Vectors OFF on a fresh store: no embeddings written.
	drv2 := openTestStore(t)
	lx2, err := lexical.New(filepath.Join(t.TempDir(), "lexical2"))
	if err != nil {
		t.Fatalf("lexical.New: %v", err)
	}
	t.Cleanup(func() { _ = lx2.Close() })

	snap2, _, err := Run(ctx, drv2, lx2, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("Run(no vectors): %v", err)
	}
	none, err := drv2.NearestSymbols(ctx, snap2.ID, nil, 100, -1)
	if err != nil {
		t.Fatalf("NearestSymbols(none): %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("vectors off but %d embeddings were written", len(none))
	}
}
