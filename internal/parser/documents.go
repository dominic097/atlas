package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

// documentFormats are the binary document/media formats Atlas extracts searchable
// text from (office OOXML packages plus images). They share the larger index size
// ceiling because the file carries embedded media but only its text is read.
var documentFormats = map[string]struct{}{
	"pptx": {}, "docx": {}, "xlsx": {}, "image": {},
}

// IsDocumentFormat reports whether a language is one of the binary document
// formats. The index walk uses this to apply a larger size ceiling — these files
// carry embedded media so they dwarf the source-file cap, yet only their (small)
// XML text is read.
func IsDocumentFormat(language string) bool {
	_, ok := documentFormats[language]
	return ok
}

// maxDocSymbolChars caps the searchable text stored on a single document/section
// symbol so one enormous deck cannot bloat the index; the head of the text (where
// titles and summaries live) is the most valuable for search.
const maxDocSymbolChars = 200_000

type docSection struct {
	kind string // "slide" | "sheet" | "section"
	name string // "Slide 3", "Cells", "Body"
	text string
}

// parseOfficeDocument extracts text from a pptx/docx/xlsx and emits searchable
// symbols: one file-level "document" symbol (Doc = the full text, so BM25 finds the
// file by any of its content) plus, when the document has natural parts, one symbol
// per slide (pptx) so a search hit points at the right place. A corrupt or non-zip
// payload is skipped (no error) rather than failing the whole index run.
func parseOfficeDocument(repoID, repoFullName, filePath, format string, content []byte) (Result, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return Result{}, nil
	}

	var sections []docSection
	switch format {
	case "pptx":
		sections = extractPptxSlides(zr)
	case "docx":
		if text := readZipText(zr, "word/document.xml"); text != "" {
			sections = []docSection{{kind: "section", name: "Body", text: text}}
		}
	case "xlsx":
		// Shared strings hold the bulk of a workbook's text in one part; that is
		// the most useful searchable surface without resolving per-cell references.
		// Sheet names (from the workbook part) are prepended so a tab name is found.
		var parts []string
		if names := extractXlsxSheetNames(zr); len(names) > 0 {
			parts = append(parts, "Sheets: "+strings.Join(names, ", "))
		}
		if text := readZipText(zr, "xl/sharedStrings.xml"); text != "" {
			parts = append(parts, text)
		}
		if len(parts) > 0 {
			sections = []docSection{{kind: "sheet", name: "Cells", text: strings.Join(parts, "\n")}}
		}
	}

	title := officeTitle(zr)
	if title == "" {
		title = filepath.Base(filePath)
	}

	full := joinSections(sections)
	if strings.TrimSpace(full) == "" && len(sections) == 0 {
		// Nothing extractable (e.g. an empty or image-only deck): still record the
		// file as a catalog entry so it is discoverable by name.
		full = title
	}

	syms := make([]graph.CodeSymbol, 0, len(sections)+1)
	syms = append(syms, newDocumentSymbol(repoID, repoFullName, filePath, format,
		"document", title, full, 0, len(sections)))

	// Per-section symbols give finer search hits — only worth it when there are
	// several (a single-section docx is fully covered by the document symbol).
	if len(sections) > 1 {
		for i, sec := range sections {
			syms = append(syms, newDocumentSymbol(repoID, repoFullName, filePath, format,
				sec.kind, sec.name, sec.text, i+1, len(sections)))
		}
	}
	return Result{Symbols: syms}, nil
}

// newDocumentSymbol builds one searchable document symbol. The extracted text goes
// in Doc (a BM25-indexed field) and a capped preview in Signature, so both terse
// and full search match. sectionIdx 0 marks the file-level document symbol.
func newDocumentSymbol(repoID, repoFullName, filePath, format, kind, name, text string, sectionIdx, sectionCount int) graph.CodeSymbol {
	text = capRunes(text, maxDocSymbolChars)
	meta := graph.JSONBMap{"format": format, "document": true}
	if sectionCount > 0 {
		meta["section_count"] = sectionCount
	}
	if sectionIdx > 0 {
		meta["section_index"] = sectionIdx
	}
	sig := capRunes(strings.Join(strings.Fields(text), " "), 200)
	sym := graph.CodeSymbol{
		ID:        newUUID(),
		RepoID:    repoID,
		Path:      filePath,
		Language:  format,
		Kind:      kind,
		Name:      name,
		Signature: sig,
		Doc:       text,
		StartLine: 1,
		EndLine:   1,
		Metadata:  meta,
	}
	sym.NodeID = ComputeNodeID(repoFullName, filePath, kind, name, fmt.Sprintf("%d", sectionIdx))
	return sym
}

// extractPptxSlides returns one section per slide, ordered by slide number, with
// the slide's visible text and its speaker notes appended (notes often carry the
// substantive talking points, so they are valuable to search).
func extractPptxSlides(zr *zip.Reader) []docSection {
	type numbered struct {
		n    int
		text string
	}
	notes := extractPptxNotes(zr)
	var slides []numbered
	for _, f := range zr.File {
		name := f.Name
		if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		base := strings.TrimSuffix(path.Base(name), ".xml") // slide12
		n, err := strconv.Atoi(strings.TrimPrefix(base, "slide"))
		if err != nil {
			continue
		}
		text := readZipText(zr, name)
		if note := notes[n]; note != "" {
			text = strings.TrimSpace(text + "\nNotes: " + note)
		}
		if text != "" {
			slides = append(slides, numbered{n: n, text: text})
		}
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
	out := make([]docSection, 0, len(slides))
	for _, s := range slides {
		out = append(out, docSection{kind: "slide", name: fmt.Sprintf("Slide %d", s.n), text: s.text})
	}
	return out
}

// extractPptxNotes returns speaker-notes text keyed by slide number. The mapping
// uses the conventional parallel numbering (notesSlideN ↔ slideN); a mismatch only
// attaches a note to a nearby slide and never drops it from the document text.
func extractPptxNotes(zr *zip.Reader) map[int]string {
	notes := map[int]string{}
	for _, f := range zr.File {
		name := f.Name
		if !strings.HasPrefix(name, "ppt/notesSlides/notesSlide") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		base := strings.TrimSuffix(path.Base(name), ".xml") // notesSlide3
		n, err := strconv.Atoi(strings.TrimPrefix(base, "notesSlide"))
		if err != nil {
			continue
		}
		if text := readZipText(zr, name); text != "" {
			notes[n] = text
		}
	}
	return notes
}

// extractXlsxSheetNames returns the worksheet (tab) names from xl/workbook.xml.
func extractXlsxSheetNames(zr *zip.Reader) []string {
	data := readZipBytes(zr, "xl/workbook.xml")
	if len(data) == 0 {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var names []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "sheet" {
			for _, a := range se.Attr {
				if a.Name.Local == "name" && strings.TrimSpace(a.Value) != "" {
					names = append(names, a.Value)
				}
			}
		}
	}
	return names
}

// officeTitle reads dc:title from docProps/core.xml, or "" when absent.
func officeTitle(zr *zip.Reader) string {
	data := readZipBytes(zr, "docProps/core.xml")
	if len(data) == 0 {
		return ""
	}
	return strings.TrimSpace(firstElementText(data, "title"))
}

// readZipText reads a named zip part and returns its OOXML visible text.
func readZipText(zr *zip.Reader, name string) string {
	data := readZipBytes(zr, name)
	if len(data) == 0 {
		return ""
	}
	return ooxmlVisibleText(data)
}

func readZipBytes(zr *zip.Reader, name string) []byte {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer rc.Close()
			data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
			if err != nil {
				return nil
			}
			return data
		}
	}
	return nil
}

// ooxmlVisibleText concatenates the character data inside every <*:t> element
// (a:t in pptx, w:t in docx, t in xlsx shared strings) — the runs that hold
// visible text in OOXML — separated by spaces.
func ooxmlVisibleText(data []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var b strings.Builder
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				depth++
			}
		case xml.EndElement:
			if t.Name.Local == "t" && depth > 0 {
				depth--
				if depth == 0 {
					b.WriteByte(' ')
				}
			}
		case xml.CharData:
			if depth > 0 {
				b.Write(t)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// firstElementText returns the character data of the first element with the given
// local name (used for dc:title without namespace gymnastics).
func firstElementText(data []byte, local string) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == local {
			var inner string
			for {
				t2, err := dec.Token()
				if err != nil {
					return inner
				}
				switch tt := t2.(type) {
				case xml.CharData:
					inner += string(tt)
				case xml.EndElement:
					return inner
				}
			}
		}
	}
}

func joinSections(sections []docSection) string {
	parts := make([]string, 0, len(sections))
	for _, s := range sections {
		if s.text != "" {
			parts = append(parts, s.text)
		}
	}
	return strings.Join(parts, "\n")
}

// capRunes truncates s to at most n runes (not bytes), preserving valid UTF-8.
func capRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
