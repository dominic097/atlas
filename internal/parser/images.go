package parser

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"  // register decoders for DecodeConfig (dimensions/format)
	_ "image/jpeg" // ...
	_ "image/png"  // ...
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dominic097/atlas/internal/graph"
)

// imageExts are the raster image extensions Atlas catalogs and (optionally) OCRs.
var imageExts = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {},
	".bmp": {}, ".tif": {}, ".tiff": {}, ".webp": {},
}

// parseImage records an image as a searchable symbol (so it is discoverable by
// name and dimensions) and, when a `tesseract` binary is present, OCRs its text
// into the BM25-indexed Doc field. OCR is best-effort and optional: no tesseract,
// an unsupported image, or an OCR failure simply yields the catalog entry — Atlas
// stays local-first with zero build-time OCR dependency (it shells out to the
// tesseract CLI if installed, the same pattern the index uses for git).
func parseImage(repoID, repoFullName, filePath string, content []byte) (Result, error) {
	name := filepath.Base(filePath)
	meta := graph.JSONBMap{"format": "image", "document": true}
	parts := []string{name}

	if cfg, format, err := image.DecodeConfig(bytes.NewReader(content)); err == nil {
		meta["image_format"] = format
		meta["width"] = cfg.Width
		meta["height"] = cfg.Height
	}
	if ocr := ocrImage(content); ocr != "" {
		meta["ocr"] = true
		parts = append(parts, ocr)
	}

	text := capRunes(strings.Join(parts, "\n"), maxDocSymbolChars)
	sym := graph.CodeSymbol{
		ID:        newUUID(),
		RepoID:    repoID,
		Path:      filePath,
		Language:  "image",
		Kind:      "image",
		Name:      name,
		Signature: capRunes(strings.Join(strings.Fields(text), " "), 200),
		Doc:       text,
		StartLine: 1,
		EndLine:   1,
		Metadata:  meta,
	}
	sym.NodeID = ComputeNodeID(repoFullName, filePath, "image", name, "")
	return Result{Symbols: []graph.CodeSymbol{sym}}, nil
}

// tesseractBin resolves the tesseract binary once per process (it never changes
// mid-run); an empty result means OCR is unavailable and is skipped.
var tesseractBin = struct {
	once sync.Once
	path string
}{}

func ocrImage(content []byte) string {
	tesseractBin.once.Do(func() {
		tesseractBin.path, _ = exec.LookPath("tesseract")
	})
	if tesseractBin.path == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// `tesseract - stdout` reads the image from stdin and writes recognized text
	// to stdout, so no temp file is needed.
	cmd := exec.CommandContext(ctx, tesseractBin.path, "-", "stdout", "-l", "eng")
	cmd.Stdin = bytes.NewReader(content)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}
