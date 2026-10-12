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
	text, err := ocrImage(img, "ja")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s\n%s", time.Since(start).Round(time.Millisecond), text)
}
