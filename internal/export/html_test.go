package export

import (
	"strings"
	"testing"
)

// sampleGraph is a small deterministic graph used across the HTML tests: a hub
// (main) calling three helpers, plus an isolated node.
func sampleGraph() Graph {
	return Graph{
		Nodes: []Node{
			{ID: "s1", Name: "main", Kind: "func", Path: "cmd/app/main.go", Line: 10, Language: "go"},
			{ID: "s2", Name: "Serve", Kind: "func", Path: "internal/server/server.go", Line: 42, Language: "go"},
			{ID: "s3", Name: "Handle", Kind: "func", Path: "internal/server/handler.go", Line: 7, Language: "go"},
			{ID: "s4", Name: "Logf", Kind: "func", Path: "internal/log/log.go", Line: 3, Language: "go"},
			{ID: "s5", Name: "Orphan", Kind: "func", Path: "internal/x/x.go", Line: 1, Language: "go"},
		},
		Edges: []Edge{
			{From: "s1", To: "s2", Kind: "calls"},
			{From: "s1", To: "s3", Kind: "calls"},
			{From: "s2", To: "s4", Kind: "calls"},
			{From: "s3", To: "s4", Kind: "calls"},
		},
	}
}

func TestHTML_BasicStructure(t *testing.T) {
	g := sampleGraph()
	out, err := g.HTML(HTMLOptions{Title: "Test graph"})
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if out == "" {
		t.Fatal("HTML returned empty string")
	}
	if !strings.HasPrefix(out, "<!DOCTYPE html>") {
		t.Errorf("HTML must start with <!DOCTYPE html>, got prefix %q", out[:min(40, len(out))])
	}
	// Self-contained: no external network/CDN resources. The only allowed URL is
	// the SVG XML namespace literal (a constant identifier browsers never fetch).
	scrubbed := strings.ReplaceAll(out, "http://www.w3.org/2000/svg", "")
	for _, bad := range []string{"http://", "https://", "src=\"//", "href=\"//", "@import", "<link"} {
		if strings.Contains(scrubbed, bad) {
			t.Errorf("HTML must be self-contained, found external reference %q", bad)
		}
	}
	// Embedded data block + renderer present.
	if !strings.Contains(out, `id="atlas-graph"`) {
		t.Error("HTML missing embedded graph data block")
	}
	if !strings.Contains(out, "mulberry32") {
		t.Error("HTML missing the seeded PRNG (deterministic layout)")
	}
}

func TestHTML_EmbedsNodeData(t *testing.T) {
	g := sampleGraph()
	out, err := g.HTML(HTMLOptions{})
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	// Node identity + metadata must be embedded as JSON in the page.
	for _, want := range []string{`"id":"s1"`, `"name":"main"`, `"path":"cmd/app/main.go"`, `"degree":`, `"community":`} {
		if !strings.Contains(out, want) {
			t.Errorf("embedded node data missing %q", want)
		}
	}
	// Edges embedded too.
	if !strings.Contains(out, `"from":"s1"`) {
		t.Error("embedded edge data missing from:s1")
	}
}

func TestHTML_TitleWithCounts(t *testing.T) {
	g := sampleGraph()
	out, err := g.HTML(HTMLOptions{Title: "My repo"})
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	// 5 nodes, 4 edges, no cap -> "My repo — 5 nodes, 4 edges".
	if !strings.Contains(out, "My repo — 5 nodes, 4 edges") {
		t.Errorf("title with counts not found in output")
	}
	if !strings.Contains(out, "<title>") {
		t.Error("missing <title> element")
	}
}

func TestHTML_TopNCap(t *testing.T) {
	// 5 nodes, cap to 2 -> only the two highest-degree nodes survive, title notes it.
	g := sampleGraph()
	out, err := g.HTML(HTMLOptions{Title: "Big", TopN: 2})
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if !strings.Contains(out, "showing top 2 of 5 nodes") {
		t.Errorf("top-N cap not reflected in title")
	}
	// s4 (Logf) has degree 2 (called by Serve+Handle) and s1 (main) has degree 2;
	// the orphan s5 has degree 0 and must be dropped.
	if strings.Contains(out, `"id":"s5"`) {
		t.Error("capped output should not contain the lowest-degree orphan node s5")
	}
}

func TestHTML_Deterministic(t *testing.T) {
	g := sampleGraph()
	a, err := g.HTML(HTMLOptions{Title: "Det"})
	if err != nil {
		t.Fatalf("HTML a: %v", err)
	}
	b, err := g.HTML(HTMLOptions{Title: "Det"})
	if err != nil {
		t.Fatalf("HTML b: %v", err)
	}
	if a != b {
		t.Error("HTML must be deterministic: two renders differ")
	}

	// Determinism must not depend on input node/edge ordering. Reverse both slices
	// and re-render; the page must be byte-identical.
	shuffled := Graph{
		Nodes: append([]Node(nil), g.Nodes...),
		Edges: append([]Edge(nil), g.Edges...),
	}
	for i, j := 0, len(shuffled.Nodes)-1; i < j; i, j = i+1, j-1 {
		shuffled.Nodes[i], shuffled.Nodes[j] = shuffled.Nodes[j], shuffled.Nodes[i]
	}
	for i, j := 0, len(shuffled.Edges)-1; i < j; i, j = i+1, j-1 {
		shuffled.Edges[i], shuffled.Edges[j] = shuffled.Edges[j], shuffled.Edges[i]
	}
	c, err := shuffled.HTML(HTMLOptions{Title: "Det"})
	if err != nil {
		t.Fatalf("HTML c: %v", err)
	}
	if a != c {
		t.Error("HTML must be order-independent: reordered input yields different output")
	}
}

func TestHTML_ViaRender(t *testing.T) {
	g := sampleGraph()
	out, err := g.Render("html")
	if err != nil {
		t.Fatalf("Render(html): %v", err)
	}
	if !strings.HasPrefix(out, "<!DOCTYPE html>") {
		t.Error("Render(html) must produce the HTML page")
	}
}

// TestRender_UnchangedFormats guards that adding html did not alter json/mermaid/dot.
func TestRender_UnchangedFormats(t *testing.T) {
	g := sampleGraph()
	js, err := g.Render("json")
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(js), "{") || strings.Contains(js, "<!DOCTYPE") {
		t.Error("json output changed")
	}
	mm := g.Mermaid()
	if !strings.HasPrefix(mm, "flowchart LR") {
		t.Error("mermaid output changed")
	}
	dot := g.DOT()
	if !strings.HasPrefix(dot, "digraph atlas") {
		t.Error("dot output changed")
	}
}
