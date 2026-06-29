package parser

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/dominic097/atlas/internal/graph"
)

// parsePDF extracts text from a PDF into searchable document symbols: one
// "document" symbol with the whole text and, for a multi-page PDF, one "page"
// symbol per page so a search hit points at the right page. It reuses the office
// document-symbol builder. The ledongthuc/pdf reader can panic on some malformed
// PDFs, so a recover keeps one bad file from sinking the whole index run.
func parsePDF(repoID, repoFullName, filePath string, content []byte) (res Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = Result{}, nil
		}
	}()

	reader, perr := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if perr != nil {
		return Result{}, nil
	}

	var sections []docSection
	fonts := make(map[string]*pdf.Font)
	pages := reader.NumPage()
	for i := 1; i <= pages; i++ {
		p := reader.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, terr := p.GetPlainText(fonts)
		if terr != nil {
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			sections = append(sections, docSection{kind: "page", name: fmt.Sprintf("Page %d", i), text: text})
		}
	}

	title := filepath.Base(filePath)
	full := joinSections(sections)
	if strings.TrimSpace(full) == "" {
		full = title // image-only / unextractable PDF: still cataloged by name
	}

	syms := make([]graph.CodeSymbol, 0, len(sections)+1)
	syms = append(syms, newDocumentSymbol(repoID, repoFullName, filePath, "pdf",
		"document", title, full, 0, len(sections)))
	if len(sections) > 1 {
		for i, sec := range sections {
			syms = append(syms, newDocumentSymbol(repoID, repoFullName, filePath, "pdf",
				sec.kind, sec.name, sec.text, i+1, len(sections)))
		}
	}
	return Result{Symbols: syms}, nil
}
