package parser

import "testing"

func TestParsePDFWiring(t *testing.T) {
	if got := LanguageForPath("docs/whitepaper.pdf"); got != "pdf" {
		t.Errorf("LanguageForPath(.pdf) = %q, want pdf", got)
	}
	if !Supported("pdf") || !IsDocumentFormat("pdf") {
		t.Error("pdf must be Supported + a document format (for the size cap)")
	}
}

func TestParseCorruptPDFSkipped(t *testing.T) {
	// Garbage bytes must not panic the parser (ledongthuc/pdf can panic) and must
	// yield no symbols rather than failing the index.
	res, err := Parse("r", "Acme/Docs", "broken.pdf", "", []byte("%PDF-1.4 not really a pdf"))
	if err != nil {
		t.Fatalf("corrupt PDF should not error, got %v", err)
	}
	if len(res.Symbols) != 0 {
		t.Errorf("corrupt PDF should yield no symbols, got %d", len(res.Symbols))
	}
}
