//go:build manual

package main

import (
	"image/png"
	"os"
	"testing"
	"time"
)

// OCR_PNG=画像.png go test -tags manual -run OCRFile -v
func TestOCRFile(t *testing.T) {
	loadLocales()
	f, err := os.Open(os.Getenv("OCR_PNG"))
	if err != nil {
		t.Skip("OCR_PNG is not set")
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	lines, err := ocrRead(img, "ja")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", time.Since(start).Round(time.Millisecond))
	for _, l := range lines {
		t.Logf("%q (%d words, first at %.0f,%.0f)", l.T, len(l.Words), l.Words[0].X, l.Words[0].Y)
	}
	for _, q := range []string{"保存しますか", "いいえ", "Cannot find path", "report", "ありません"} {
		x, y, n := findText(lines, q)
		t.Logf("find %q → (%.0f, %.0f) ×%d", q, x, y, n)
	}
}
