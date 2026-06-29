package index

import (
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

func TestIsDistinctiveName(t *testing.T) {
	distinctive := []string{"UserService", "AuthMiddleware", "parseConfig", "parse_config", "HTTPClient", "RenderInvoice"}
	for _, n := range distinctive {
		if !isDistinctiveName(n) {
			t.Errorf("isDistinctiveName(%q) = false, want true", n)
		}
	}
	common := []string{"User", "Handler", "data", "config", "main", "run", "id", "parse", "service"}
	for _, n := range common {
		if isDistinctiveName(n) {
			t.Errorf("isDistinctiveName(%q) = true, want false (too common/generic)", n)
		}
	}
}

func TestLinkDocumentsHighPrecision(t *testing.T) {
	files := []graph.File{
		{Path: "internal/auth/login.go"},
		{Path: "internal/billing/invoice.go"},
		{Path: "docs/design.pptx"},
	}
	symbols := []graph.CodeSymbol{
		// code symbols (targets)
		{ID: "c1", Name: "AuthMiddleware", Path: "internal/auth/login.go", Kind: "function"},
		{ID: "c2", Name: "RenderInvoice", Path: "internal/billing/invoice.go", Kind: "function"},
		{ID: "c3", Name: "User", Path: "internal/auth/login.go", Kind: "type"}, // too common -> not a target
		// a document symbol (source) that references code by name AND by file path
		{ID: "d1", Name: "design.pptx", Path: "docs/design.pptx", Kind: "document",
			Metadata: graph.JSONBMap{"document": true},
			Doc:      "The login flow uses AuthMiddleware (see internal/auth/login.go). Billing calls RenderInvoice. A plain User is mentioned but must not link."},
	}

	edges := linkDocuments(symbols, files)

	got := map[string]string{} // toRef -> match_type
	for _, e := range edges {
		if e.FromSymbol != "d1" || e.Kind != graph.EdgeReferences {
			t.Errorf("unexpected edge: %+v", e)
		}
		got[e.ToRef], _ = e.Metadata["match_type"].(string)
	}
	if got["AuthMiddleware"] != "symbol_name" {
		t.Errorf("missing AuthMiddleware symbol link, got %v", got)
	}
	if got["RenderInvoice"] != "symbol_name" {
		t.Errorf("missing RenderInvoice symbol link, got %v", got)
	}
	if got["internal/auth/login.go"] != "file_path" {
		t.Errorf("missing file-path link, got %v", got)
	}
	if _, linked := got["User"]; linked {
		t.Errorf("common name 'User' must NOT be linked, got %v", got)
	}
}

func TestLinkDocumentsNoDocsNoEdges(t *testing.T) {
	symbols := []graph.CodeSymbol{{ID: "c1", Name: "UserService", Path: "a.go", Kind: "function"}}
	if edges := linkDocuments(symbols, []graph.File{{Path: "a.go"}}); len(edges) != 0 {
		t.Errorf("no document symbols should yield no edges, got %d", len(edges))
	}
}
