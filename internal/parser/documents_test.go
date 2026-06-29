package parser

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func buildZip(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParsePptxSlides(t *testing.T) {
	pptx := buildZip(t, map[string]string{
		"docProps/core.xml":      `<cp:coreProperties xmlns:cp="x" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Quarterly Review</dc:title></cp:coreProperties>`,
		"ppt/slides/slide1.xml":  `<p:sld xmlns:a="a"><a:t>Revenue</a:t><a:t>grew 20%</a:t></p:sld>`,
		"ppt/slides/slide2.xml":  `<p:sld xmlns:a="a"><a:t>Roadmap for auth service</a:t></p:sld>`,
		"ppt/slides/_rels/x.xml": `<ignore/>`,
	})

	// Go through the public dispatch to prove LanguageForPath + Parse wiring.
	res, err := Parse("repo-1", "Acme/Deck", "decks/q3.pptx", "", pptx)
	if err != nil {
		t.Fatalf("Parse pptx: %v", err)
	}
	if len(res.Symbols) != 3 {
		t.Fatalf("want 3 symbols (1 document + 2 slides), got %d", len(res.Symbols))
	}
	doc := res.Symbols[0]
	if doc.Kind != "document" || doc.Name != "Quarterly Review" {
		t.Errorf("document symbol = kind %q name %q, want document/Quarterly Review", doc.Kind, doc.Name)
	}
	if !strings.Contains(doc.Doc, "Revenue") || !strings.Contains(doc.Doc, "Roadmap") {
		t.Errorf("document Doc missing slide text: %q", doc.Doc)
	}
	if doc.Language != "pptx" {
		t.Errorf("language = %q, want pptx", doc.Language)
	}
	// Slide-level symbols with the right text for precise search hits.
	var foundSlide2 bool
	for _, s := range res.Symbols[1:] {
		if s.Kind != "slide" {
			t.Errorf("sub-symbol kind = %q, want slide", s.Kind)
		}
		if s.Name == "Slide 2" && strings.Contains(s.Doc, "auth service") {
			foundSlide2 = true
		}
	}
	if !foundSlide2 {
		t.Error("no Slide 2 symbol with its text")
	}
}

func TestParsePptxSpeakerNotes(t *testing.T) {
	pptx := buildZip(t, map[string]string{
		"ppt/slides/slide1.xml":           `<p:sld xmlns:a="a"><a:t>Title slide</a:t></p:sld>`,
		"ppt/notesSlides/notesSlide1.xml":  `<p:notes xmlns:a="a"><a:t>Remember to mention the migration deadline</a:t></p:notes>`,
	})
	res, err := Parse("r", "Acme/Deck", "deck.pptx", "", pptx)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// The slide's text must include its speaker note.
	if !strings.Contains(res.Symbols[0].Doc, "migration deadline") {
		t.Errorf("document text missing speaker note: %q", res.Symbols[0].Doc)
	}
}

func TestParseXlsxSheetNames(t *testing.T) {
	xlsx := buildZip(t, map[string]string{
		"xl/workbook.xml":      `<workbook xmlns="w"><sheets><sheet name="Q3 Forecast"/><sheet name="Headcount"/></sheets></workbook>`,
		"xl/sharedStrings.xml": `<sst xmlns="s"><si><t>Revenue</t></si></sst>`,
	})
	res, err := Parse("r", "Acme/Sheets", "budget.xlsx", "", xlsx)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	d := res.Symbols[0].Doc
	if !strings.Contains(d, "Q3 Forecast") || !strings.Contains(d, "Headcount") {
		t.Errorf("xlsx text missing sheet names: %q", d)
	}
	if !strings.Contains(d, "Revenue") {
		t.Errorf("xlsx text missing cell content: %q", d)
	}
}

func TestParseDocxSingleBody(t *testing.T) {
	docx := buildZip(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>Design</w:t></w:r><w:r><w:t xml:space="preserve"> doc for the billing module</w:t></w:r></w:p></w:body></w:document>`,
	})
	res, err := Parse("repo-1", "Acme/Docs", "docs/spec.docx", "", docx)
	if err != nil {
		t.Fatalf("Parse docx: %v", err)
	}
	// A single-section docx is fully covered by the one document symbol.
	if len(res.Symbols) != 1 {
		t.Fatalf("want 1 document symbol, got %d", len(res.Symbols))
	}
	d := res.Symbols[0]
	if d.Kind != "document" || d.Name != "spec.docx" {
		t.Errorf("document = kind %q name %q, want document/spec.docx (filename fallback)", d.Kind, d.Name)
	}
	if !strings.Contains(d.Doc, "Design") || !strings.Contains(d.Doc, "billing module") {
		t.Errorf("docx Doc missing body text: %q", d.Doc)
	}
}

func TestParseXlsxSharedStrings(t *testing.T) {
	xlsx := buildZip(t, map[string]string{
		"xl/sharedStrings.xml": `<sst xmlns="s"><si><t>Total Revenue</t></si><si><t>Q3 forecast</t></si></sst>`,
	})
	res, err := Parse("repo-1", "Acme/Sheets", "sheets/budget.xlsx", "", xlsx)
	if err != nil {
		t.Fatalf("Parse xlsx: %v", err)
	}
	if len(res.Symbols) != 1 {
		t.Fatalf("want 1 document symbol, got %d", len(res.Symbols))
	}
	if d := res.Symbols[0]; !strings.Contains(d.Doc, "Total Revenue") || !strings.Contains(d.Doc, "Q3 forecast") {
		t.Errorf("xlsx Doc missing cell text: %q", d.Doc)
	}
}

func TestParseCorruptOfficeIsSkippedNotFatal(t *testing.T) {
	// Not a zip — must be skipped (no symbols, no error), never crash the index.
	res, err := Parse("repo-1", "Acme/Docs", "broken.docx", "", []byte("not a zip file"))
	if err != nil {
		t.Fatalf("corrupt office should not error, got %v", err)
	}
	if len(res.Symbols) != 0 {
		t.Errorf("corrupt office should yield no symbols, got %d", len(res.Symbols))
	}
}

func TestLanguageAndDocFormatWiring(t *testing.T) {
	for ext, want := range map[string]string{".pptx": "pptx", ".docx": "docx", ".xlsx": "xlsx"} {
		if got := LanguageForPath("a/b" + ext); got != want {
			t.Errorf("LanguageForPath(%s) = %q, want %q", ext, got, want)
		}
		if !Supported(want) {
			t.Errorf("Supported(%q) = false", want)
		}
		if !IsDocumentFormat(want) {
			t.Errorf("IsDocumentFormat(%q) = false", want)
		}
	}
	if IsDocumentFormat("go") {
		t.Error("go must not be a document format")
	}
}
