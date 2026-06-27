package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunSkipsGeneratedGraphifyOutput(t *testing.T) {
	ctx := context.Background()
	repo := writeGoRepo(t)

	graphDir := filepath.Join(repo, "graphify-out")
	if err := os.MkdirAll(graphDir, 0o755); err != nil {
		t.Fatalf("mkdir graphify-out: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "graph.json"), []byte(`{"nodes":[{"id":"generated"}]}`), 0o644); err != nil {
		t.Fatalf("write generated graph: %v", err)
	}

	drv := openTestStore(t)
	_, stats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Files != 1 {
		t.Fatalf("indexed %d files, want only the source file (stats=%+v)", stats.Files, stats)
	}
	if got := stats.Languages["json"]; got != 0 {
		t.Fatalf("indexed generated graphify JSON count %d, want 0 (stats=%+v)", got, stats)
	}
}
