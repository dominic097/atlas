package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAnalyticsFixture writes a small multi-symbol Go package whose call graph
// has a clear hub: Dispatch calls three handlers, each of which calls a shared
// Helper. So Helper (3 callers) and Dispatch (3 callees) are the high-degree
// "god nodes", and there is at least one connected community.
func writeAnalyticsFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := `package svc

// Helper is the shared leaf every handler calls.
func Helper(x int) int { return x + 1 }

// HandleA calls Helper.
func HandleA() int { return Helper(1) }

// HandleB calls Helper.
func HandleB() int { return Helper(2) }

// HandleC calls Helper.
func HandleC() int { return Helper(3) }

// Dispatch fans out to the three handlers — a hub on the out-degree side.
func Dispatch() int { return HandleA() + HandleB() + HandleC() }
`
	if err := os.WriteFile(filepath.Join(dir, "svc.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

func indexAnalyticsFixture(t *testing.T) Engine {
	t.Helper()
	ctx := context.Background()
	eng := newTestEngine(t, false)
	repo := writeAnalyticsFixture(t)
	if _, err := eng.Index(ctx, IndexInput{ProjectPath: repo}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	return eng
}

// TestCommunitiesReturnsClusters asserts the communities op returns a non-empty,
// size-ranked partition over the indexed fixture, with stable IDs and a Total.
func TestCommunitiesReturnsClusters(t *testing.T) {
	ctx := context.Background()
	eng := indexAnalyticsFixture(t)

	res, err := eng.Communities(ctx, CommunitiesInput{})
	if err != nil {
		t.Fatalf("Communities: %v", err)
	}
	if len(res.Communities) == 0 {
		t.Fatalf("expected at least one community, got none")
	}
	if res.Total < len(res.Communities) {
		t.Errorf("Total (%d) should be >= shown communities (%d)", res.Total, len(res.Communities))
	}
	// The connected call graph (Helper/Dispatch/HandleA/B/C) should land in one
	// community: the largest cluster must contain Helper and Dispatch.
	top := res.Communities[0]
	if top.ID != 0 {
		t.Errorf("first community should have stable ID 0, got %d", top.ID)
	}
	if top.Size != len(top.Members) {
		t.Errorf("community Size (%d) != len(Members) (%d)", top.Size, len(top.Members))
	}
	if !containsStr(top.Members, "Helper") || !containsStr(top.Members, "Dispatch") {
		t.Errorf("largest community should contain Helper and Dispatch; got %v", top.Members)
	}
	if len(top.Representatives) == 0 {
		t.Errorf("largest community should have representatives; got none")
	}
}

// TestHubsRanksGodNodes asserts the hubs op ranks Helper and Dispatch (the
// high-degree symbols) at the top by total degree.
func TestHubsRanksGodNodes(t *testing.T) {
	ctx := context.Background()
	eng := indexAnalyticsFixture(t)

	res, err := eng.Hubs(ctx, HubsInput{Limit: 3})
	if err != nil {
		t.Fatalf("Hubs: %v", err)
	}
	if len(res.Hubs) == 0 {
		t.Fatalf("expected hubs, got none")
	}
	if len(res.Hubs) > 3 {
		t.Errorf("Limit=3 not honored: got %d hubs", len(res.Hubs))
	}
	// Helper (3 in-edges) and Dispatch (3 out-edges) are the god nodes; both must
	// rank in the top 3.
	top := make([]string, 0, len(res.Hubs))
	for _, h := range res.Hubs {
		top = append(top, h.Name)
		if h.TotalDegree <= 0 {
			t.Errorf("hub %s has non-positive total degree %d", h.Name, h.TotalDegree)
		}
	}
	if !containsStr(top, "Helper") {
		t.Errorf("Helper should rank as a hub; top hubs were %v", top)
	}
	if !containsStr(top, "Dispatch") {
		t.Errorf("Dispatch should rank as a hub; top hubs were %v", top)
	}
	// Hubs must be sorted by total degree descending.
	for i := 1; i < len(res.Hubs); i++ {
		if res.Hubs[i-1].TotalDegree < res.Hubs[i].TotalDegree {
			t.Errorf("hubs not sorted by total degree desc at %d: %d < %d",
				i, res.Hubs[i-1].TotalDegree, res.Hubs[i].TotalDegree)
		}
	}
}

// TestReportComposesStatsAndMarkdown asserts the report op returns sensible
// Stats + hubs + communities and that the Markdown carries the summary stats and
// at least one hub name.
func TestReportComposesStatsAndMarkdown(t *testing.T) {
	ctx := context.Background()
	eng := indexAnalyticsFixture(t)

	res, err := eng.Report(ctx, ReportInput{})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if res.Stats.Symbols == 0 {
		t.Errorf("report stats should count symbols; got 0")
	}
	if res.Stats.Edges == 0 {
		t.Errorf("report stats should count call edges; got 0")
	}
	if len(res.Hubs) == 0 {
		t.Fatalf("report should include hubs; got none")
	}
	if len(res.Communities) == 0 {
		t.Errorf("report should include communities; got none")
	}
	md := res.Markdown
	if md == "" {
		t.Fatalf("report Markdown is empty")
	}
	// The markdown must carry the summary stats and the headline sections.
	for _, want := range []string{"# Graph report", "## Summary", "## Top hubs (god nodes)", "## Communities"} {
		if !strings.Contains(md, want) {
			t.Errorf("report Markdown missing %q", want)
		}
	}
	// The symbol count is part of the summary.
	if !strings.Contains(md, "Symbols (nodes):") {
		t.Errorf("report Markdown missing the symbol-count stat line")
	}
	// A real hub name from the structured result must appear in the rendered table.
	hub := res.Hubs[0].Name
	if !strings.Contains(md, hub) {
		t.Errorf("report Markdown should mention hub %q; markdown was:\n%s", hub, md)
	}
}

func TestStatsReturnsPersistedIndexTelemetry(t *testing.T) {
	ctx := context.Background()
	eng := indexAnalyticsFixture(t)

	res, err := eng.Stats(ctx, StatsInput{Limit: 5})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if res.RepoFullName == "" {
		t.Fatalf("Stats should include repo full name")
	}
	if res.Latest.Mode != "full" {
		t.Fatalf("Latest mode = %q, want full", res.Latest.Mode)
	}
	if res.Latest.Files == 0 || res.Latest.Symbols == 0 || res.Latest.Edges == 0 {
		t.Fatalf("latest counts should be populated: %+v", res.Latest)
	}
	if len(res.Latest.TimingsMS) == 0 {
		t.Fatalf("latest timings should be persisted")
	}
	if res.Graph.Symbols == 0 || res.Graph.Edges == 0 {
		t.Fatalf("graph stats should be populated: %+v", res.Graph)
	}
	if res.HistoryReturned != 1 || len(res.History) != 1 {
		t.Fatalf("history returned mismatch: %d len=%d", res.HistoryReturned, len(res.History))
	}
}

func containsStr(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
