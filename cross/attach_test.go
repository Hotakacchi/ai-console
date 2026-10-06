package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, _ := zw.Create(name)
		io.WriteString(w, body)
	}
	zw.Close()
	f.Close()
}

// 文字だけの小さな PDF を作る (xref の位置も計算する)
func writePDF(t *testing.T, path, text string) {
	t.Helper()
	stream := "BT /F1 24 Tf 72 720 Td (" + text + ") Tj ET"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	var offs []int
	for i, o := range objs {
		offs = append(offs, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	os.WriteFile(path, b.Bytes(), 0o644)
}

func TestReadAttachment(t *testing.T) {
	loadLocales()
	dir := t.TempDir()
	p := func(name string) string { return filepath.Join(dir, name) }

	os.WriteFile(p("a.txt"), []byte("\xef\xbb\xbfこんにちは"), 0o644)
	sjis, _ := japanese.ShiftJIS.NewEncoder().Bytes([]byte("シフトJISの文"))
	os.WriteFile(p("b.csv"), sjis, 0o644)
	os.WriteFile(p("c.bin"), []byte{0, 1, 2, 3}, 0o644)
	writeZip(t, p("d.docx"), map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>一行目</w:t></w:r></w:p><w:p><w:r><w:t>二行</w:t></w:r><w:r><w:t>目</w:t></w:r></w:p></w:body></w:document>`})
	writeZip(t, p("e.xlsx"), map[string]string{
		"xl/sharedStrings.xml":     `<sst><si><t>名前</t></si><si><r><t>ル</t></r><r><t>ミ</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c t="s"><v>0</v></c><c><v>42</v></c></row><row><c t="s"><v>1</v></c><c t="inlineStr"><is><t>直接</t></is></c></row></sheetData></worksheet>`,
	})
	writeZip(t, p("f.pptx"), map[string]string{
		"ppt/slides/slide2.xml": `<p:sld xmlns:a="a" xmlns:p="p"><a:p><a:r><a:t>二枚目</a:t></a:r></a:p></p:sld>`,
		"ppt/slides/slide1.xml": `<p:sld xmlns:a="a" xmlns:p="p"><a:p><a:r><a:t>一枚目</a:t></a:r></a:p></p:sld>`,
	})
	writePDF(t, p("g.pdf"), "Hello PDF")
	img := image.NewRGBA(image.Rect(0, 0, 3000, 1500))
	img.Set(10, 10, color.White)
	f, _ := os.Create(p("h.png"))
	png.Encode(f, img)
	f.Close()

	cases := map[string]string{
		"a.txt":  "こんにちは",
		"b.csv":  "シフトJISの文",
		"d.docx": "一行目\n二行目",
		"e.xlsx": "--- sheet 1 ---\n名前\t42\nルミ\t直接",
		"f.pptx": "--- 1 ---\n一枚目\n--- 2 ---\n二枚目",
	}
	for name, want := range cases {
		a, err := readAttachment(p(name), 1000)
		if err != nil || a.Text != want {
			t.Errorf("%s: %q %v", name, a.Text, err)
		}
	}
	if a, err := readAttachment(p("g.pdf"), 1000); err != nil || !strings.Contains(a.Text, "Hello PDF") {
		t.Errorf("pdf: %q %v", a.Text, err)
	}
	if _, err := readAttachment(p("c.bin"), 1000); err == nil {
		t.Error("binary accepted")
	}
	if _, err := readAttachment(dir, 1000); err == nil {
		t.Error("directory accepted")
	}
	if a, _ := readAttachment(p("a.txt"), 3); a.Text != "こんに" || !a.Cut {
		t.Errorf("cut: %q %v", a.Text, a.Cut)
	}
	a, err := readAttachment(p("h.png"), 1000)
	if err != nil || a.Image == nil {
		t.Fatalf("image: %v", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(a.Image))
	if err != nil || cfg.Width != maxImageSide || cfg.Height != maxImageSide/2 {
		t.Errorf("resized to %dx%d %v", cfg.Width, cfg.Height, err)
	}
}

func TestWithAttachments(t *testing.T) {
	loadLocales()
	setLanguage("en")
	turn := withAttachments("summarize", []attachment{{Name: "a.txt", Text: "body", Cut: true}, {Name: "b.png", Image: []byte{1}}})
	if len(turn.Images) != 1 || !strings.Contains(turn.Text, "[Attached file: a.txt (truncated)]\nbody\n[End of attached file]") ||
		!strings.Contains(turn.Text, "[Attached image: b.png]") {
		t.Errorf("%q", turn.Text)
	}
}

func TestTrimHistory(t *testing.T) {
	h := []Message{
		{"role": "user", "content": strings.Repeat("a", 100)},
		{"role": "assistant", "content": strings.Repeat("b", 100)},
		{"role": "user", "content": strings.Repeat("c", 100)},
		{"role": "assistant", "content": "d"},
		{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": strings.Repeat("x", 100000)}}, map[string]any{"type": "text", "text": "q"}}},
	}
	got := trimHistory(h, 1800)
	// 画像は 1 枚 1500 と数えるので、最初の 2 つだけが落ちる (user から始まる)
	if len(got) != 3 || got[0]["role"] != "user" {
		t.Errorf("kept %d, first %v", len(got), got[0]["role"])
	}
	if len(trimHistory(h, 0)) != 5 {
		t.Error("budget 0 should keep all")
	}
	if len(trimHistory(h, 1)) != 1 {
		t.Error("the latest message must stay")
	}
}

// 画像つきの発言が OpenAI 互換の形で送られ、返事のあとは履歴から画像が外れる
func TestImageTurn(t *testing.T) {
	loadLocales()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"見えました\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	s := &Settings{vals: map[string]any{"endpoint": srv.URL}}
	p := newOpenAI(s)
	var reply strings.Builder
	p.Reply(Turn{Text: "これは何？", Images: [][]byte{{0xff, 0xd8}}}, func(s string) { reply.WriteString(s) })
	if reply.String() != "見えました" {
		t.Fatalf("reply %q", reply.String())
	}
	msgs := got["messages"].([]any)
	parts := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
	if url := parts[0].(map[string]any)["image_url"].(map[string]any)["url"].(string); url != "data:image/jpeg;base64,/9g=" {
		t.Errorf("image url %q", url)
	}
	if c, ok := p.history[0]["content"].(string); !ok || !strings.HasPrefix(c, "これは何？\n") {
		t.Errorf("history keeps the image: %#v", p.history[0]["content"])
	}
}
