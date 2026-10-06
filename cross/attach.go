package main

// ファイルを読ませる: 窓にファイルをドラッグ＆ドロップ (か /attach <パス>) すると、
// 次に話しかけたときに中身を一緒に AI に渡す。
// 文字のファイル・PDF・Word・Excel・PowerPoint は文字を取り出し、画像はそのまま見せる。

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	"golang.org/x/text/encoding/japanese"
)

const maxAttachBytes = 50 << 20 // これより大きいファイルは読まない

type attachment struct {
	Name  string
	Text  string // 取り出した文字 (画像なら空)
	Image []byte // 画像なら JPEG
	Cut   bool   // 長すぎて途中で切った
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true}

// ファイルを読んで、AI に渡せる形にする。maxChars を超える文字は切る
func readAttachment(path string, maxChars int) (attachment, error) {
	a := attachment{Name: filepath.Base(path)}
	st, err := os.Stat(path)
	if err != nil {
		return a, err
	}
	if st.IsDir() {
		return a, errors.New(T("attach.isDir"))
	}
	if st.Size() > maxAttachBytes {
		return a, errors.New(T("attach.tooBig", maxAttachBytes>>20))
	}
	ext := strings.ToLower(filepath.Ext(path))
	if imageExts[ext] {
		f, err := os.Open(path)
		if err != nil {
			return a, err
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return a, err
		}
		a.Image = encodeJPEG(img)
		return a, nil
	}
	var text string
	switch ext {
	case ".pdf":
		text, err = pdfText(path)
	case ".docx":
		text, err = officeText(path, "word/document.xml", "p", "t")
	case ".pptx":
		text, err = pptxText(path)
	case ".xlsx":
		text, err = xlsxText(path)
	default:
		text, err = fileText(path)
	}
	if err != nil {
		return a, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return a, errors.New(T("attach.noText"))
	}
	if r := []rune(text); len(r) > maxChars {
		text, a.Cut = string(r[:maxChars]), true
	}
	a.Text = text
	return a, nil
}

// 文字のファイル。UTF-8 でなければ Shift_JIS として読む (Windows の日本語のファイル)
func fileText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return "", errors.New(T("attach.unsupported"))
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	if s, err := japanese.ShiftJIS.NewDecoder().Bytes(data); err == nil {
		return string(s), nil
	}
	return "", errors.New(T("attach.unsupported"))
}

func pdfText(path string) (text string, err error) {
	defer func() { // 壊れた・変わった形の PDF で落ちないように
		if r := recover(); r != nil {
			err = fmt.Errorf("PDF: %v", r)
		}
	}()
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, w := range row.Content {
				b.WriteString(w.S)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// ---- Office (中身は zip の XML) ----

func zipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 64<<20))
		}
	}
	return nil, os.ErrNotExist
}

// XML から、textTag の中の文字を取り出し、paraTag の終わりで改行する
func xmlText(data []byte, paraTag, textTag string) string {
	var b strings.Builder
	d := xml.NewDecoder(bytes.NewReader(data))
	inText := false
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == textTag {
				inText = true
			} else if t.Name.Local == "tab" {
				b.WriteString("\t")
			}
		case xml.EndElement:
			if t.Name.Local == textTag {
				inText = false
			} else if t.Name.Local == paraTag {
				b.WriteString("\n")
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
	return b.String()
}

func officeText(path, part, paraTag, textTag string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	data, err := zipFile(&zr.Reader, part)
	if err != nil {
		return "", errors.New(T("attach.unsupported"))
	}
	return xmlText(data, paraTag, textTag), nil
}

var slideRe = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

func pptxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	type slide struct {
		n    int
		name string
	}
	var slides []slide
	for _, f := range zr.File {
		if m := slideRe.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, slide{n, f.Name})
		}
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
	var b strings.Builder
	for _, s := range slides {
		data, err := zipFile(&zr.Reader, s.name)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "--- %d ---\n%s\n", s.n, strings.TrimSpace(xmlText(data, "p", "t")))
	}
	return b.String(), nil
}

// Excel: シートごとに、行をタブ区切りで並べる
func xlsxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	// 共通の文字列 (セルには番号だけが入っている)
	var shared []string
	if data, err := zipFile(&zr.Reader, "xl/sharedStrings.xml"); err == nil {
		var sst struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		xml.Unmarshal(data, &sst)
		for _, si := range sst.SI {
			s := si.T
			for _, r := range si.R {
				s += r.T
			}
			shared = append(shared, s)
		}
	}
	sheetRe := regexp.MustCompile(`^xl/worksheets/sheet(\d+)\.xml$`)
	type sheet struct {
		n    int
		name string
	}
	var sheets []sheet
	for _, f := range zr.File {
		if m := sheetRe.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			sheets = append(sheets, sheet{n, f.Name})
		}
	}
	sort.Slice(sheets, func(i, j int) bool { return sheets[i].n < sheets[j].n })
	var b strings.Builder
	for _, sh := range sheets {
		data, err := zipFile(&zr.Reader, sh.name)
		if err != nil {
			continue
		}
		var ws struct {
			Rows []struct {
				Cells []struct {
					Type   string `xml:"t,attr"`
					V      string `xml:"v"`
					Inline string `xml:"is>t"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if xml.Unmarshal(data, &ws) != nil {
			continue
		}
		fmt.Fprintf(&b, "--- sheet %d ---\n", sh.n)
		for _, row := range ws.Rows {
			var cells []string
			for _, c := range row.Cells {
				v := c.V
				switch c.Type {
				case "s":
					if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(shared) {
						v = shared[i]
					}
				case "inlineStr":
					v = c.Inline
				}
				cells = append(cells, v)
			}
			b.WriteString(strings.Join(cells, "\t") + "\n")
		}
	}
	return b.String(), nil
}

// ---- 渡す前のファイル ----

// 1 つの発言に入れる文字の上限 (ローカルAIはコンテキストが小さいので少なめ)
func (l *Lumi) attachLimit() int {
	if strings.ToLower(l.s.Get("provider", "offline")) == "local" {
		return max(3000, localContext(l.s)-5000)
	}
	return 100000
}

// ドロップされたファイル (や /attach) を読んで、次の発言に付ける
func (l *Lumi) attachFiles(paths []string) {
	for _, p := range paths {
		a, err := readAttachment(p, l.attachLimit())
		if err != nil {
			l.errorText(T("attach.failed", filepath.Base(p), err.Error()))
			continue
		}
		if a.Image != nil {
			if ok, why := l.canSeeImages(); !ok {
				l.errorText(T("attach.failed", a.Name, why))
				continue
			}
		}
		l.mu.Lock()
		l.pending = append(l.pending, a)
		l.mu.Unlock()
		l.fx("eat", 1.8) // もぐもぐ
		switch {
		case a.Image != nil:
			l.write("  "+T("attach.addedImage", a.Name)+"\n", "cyan")
		case a.Cut:
			l.write("  "+T("attach.addedCut", a.Name, len([]rune(a.Text)))+"\n", "cyan")
		default:
			l.write("  "+T("attach.added", a.Name, len([]rune(a.Text)))+"\n", "cyan")
		}
	}
	if l.pendingCount() > 0 {
		l.write("  "+T("attach.hint")+"\n\n", "dim")
	}
}

func (l *Lumi) pendingCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pending)
}

// 付けていたファイルを取り出す (取り出したら空にする)
func (l *Lumi) takePending() []attachment {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.pending
	l.pending = nil
	return p
}

// 発言にファイルの中身をつなげる
func withAttachments(text string, list []attachment) Turn {
	t := Turn{Text: text}
	for _, a := range list {
		if a.Image != nil {
			t.Images = append(t.Images, a.Image)
			t.Text += "\n\n" + T("attach.imageBlock", a.Name)
			continue
		}
		note := ""
		if a.Cut {
			note = T("attach.cutNote")
		}
		t.Text += "\n\n" + T("attach.block", a.Name, note) + "\n" + a.Text + "\n" + T("attach.blockEnd")
	}
	return t
}

// /attach <パス>  /attach (一覧)  /detach (付けたファイルを外す)
func (l *Lumi) attachCommand(arg string) {
	if arg = strings.Trim(strings.TrimSpace(arg), `"'`); arg != "" {
		l.attachFiles([]string{arg})
		return
	}
	l.mu.Lock()
	list := append([]attachment(nil), l.pending...)
	l.mu.Unlock()
	if len(list) == 0 {
		l.info(T("attach.none"))
		return
	}
	var b strings.Builder
	for _, a := range list {
		b.WriteString("  " + a.Name + "\n")
	}
	b.WriteString("\n  " + T("attach.hint"))
	l.info(b.String())
}

func (l *Lumi) detach() {
	if n := len(l.takePending()); n > 0 {
		l.info(T("attach.detached", n))
	} else {
		l.info(T("attach.none"))
	}
}
