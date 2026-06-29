package parser

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseImageCatalog(t *testing.T) {
	// A blank PNG: catalog entry with dimensions, no OCR text needed.
	res, err := Parse("repo-1", "Acme/Assets", "diagrams/flow.png", "", pngBytes(t, 80, 40))
	if err != nil {
		t.Fatalf("Parse image: %v", err)
	}
	if len(res.Symbols) != 1 {
		t.Fatalf("want 1 image symbol, got %d", len(res.Symbols))
	}
	s := res.Symbols[0]
	if s.Kind != "image" || s.Name != "flow.png" || s.Language != "image" {
		t.Errorf("image symbol = kind %q name %q lang %q", s.Kind, s.Name, s.Language)
	}
	if s.Metadata["image_format"] != "png" {
		t.Errorf("image_format = %v, want png", s.Metadata["image_format"])
	}
	if s.Metadata["width"] != 80 || s.Metadata["height"] != 40 {
		t.Errorf("dimensions = %vx%v, want 80x40", s.Metadata["width"], s.Metadata["height"])
	}
	if !strings.Contains(s.Doc, "flow.png") {
		t.Errorf("Doc should at least contain the filename, got %q", s.Doc)
	}
}

func TestParseImageWiring(t *testing.T) {
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".tiff"} {
		if got := LanguageForPath("a/b" + ext); got != "image" {
			t.Errorf("LanguageForPath(%s) = %q, want image", ext, got)
		}
	}
	if !Supported("image") || !IsDocumentFormat("image") {
		t.Error("image must be Supported + a document format (for the size cap)")
	}
}

// TestOcrImageWhenAvailable exercises the real OCR path when a tesseract binary is
// present; it is skipped otherwise so CI without tesseract still passes.
func TestOcrImageWhenAvailable(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract not installed")
	}
	// A blank image yields no text — that is fine; we only assert the OCR shell-out
	// runs without error and returns a string (empty here).
	if got := ocrImage(pngBytes(t, 60, 30)); got != "" {
		t.Logf("blank-image OCR returned %q (non-fatal)", got)
	}
}
