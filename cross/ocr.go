package main

// 画面の文字を読み取る (OCR)。OS に入っている仕組みを使うので、何もダウンロードしない:
//   Windows: Windows.Media.Ocr (PowerShell 5.1 から WinRT を呼ぶ)
//   Mac:     Vision (JavaScript for Automation から呼ぶ)
//   Linux:   tesseract (入っていれば)
// 画像を読めない AI (画像を読む部品がないローカルAIなど) でも、画面のエラーメッセージなどを読める。
// 文字の場所もわかるので、画面操作 (uicontrol.go) で「〇〇をクリック」に使う。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/image/draw"
	"golang.org/x/text/unicode/norm"
)

// Windows の OCR が読める画像の大きさの上限 (OcrEngine.MaxImageDimension)
const ocrMaxSide = 2600

// 読み取った 1 行と、その中の単語 (場所は元の画像の座標)
type ocrWord struct {
	T string  `json:"t"`
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type ocrLine struct {
	T     string    `json:"t"`
	Words []ocrWord `json:"w"`
}

const ocrWindowsScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
$null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics, ContentType = WindowsRuntime]
$null = [Windows.Globalization.Language, Windows.Globalization, ContentType = WindowsRuntime]
$asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' } | Select-Object -First 1
function Await($op, $type) { $t = $asTask.MakeGenericMethod($type).Invoke($null, @($op)); $null = $t.Wait(-1); $t.Result }
$file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($env:LUMI_OCR_FILE)) ([Windows.Storage.StorageFile])
$stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
$decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
$bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
$engine = $null
if ($env:LUMI_OCR_LANG) { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage([Windows.Globalization.Language]::new($env:LUMI_OCR_LANG)) }
if (-not $engine) { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages() }
if (-not $engine) { throw 'no OCR language' }
$result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
$lines = @(foreach ($line in $result.Lines) {
  [pscustomobject]@{ t = $line.Text; w = @(foreach ($w in $line.Words) { $r = $w.BoundingRect; [pscustomobject]@{ t = $w.Text; x = $r.X; y = $r.Y; w = $r.Width; h = $r.Height } }) }
})
ConvertTo-Json -InputObject $lines -Depth 5 -Compress
$stream.Dispose()
`

const ocrMacScript = `
ObjC.import('Foundation'); ObjC.import('Vision'); ObjC.import('AppKit');
function run(argv) {
  const url = $.NSURL.fileURLWithPath(argv[0]);
  const rep = $.NSImage.alloc.initWithContentsOfFile(argv[0]).representations.objectAtIndex(0);
  const W = rep.pixelsWide, H = rep.pixelsHigh;
  const req = $.VNRecognizeTextRequest.alloc.init;
  req.recognitionLevel = 0;
  req.usesLanguageCorrection = true;
  try { req.recognitionLanguages = $(argv[1] === 'ja' ? ['ja-JP', 'en-US'] : ['en-US']); } catch (e) {}
  const handler = $.VNImageRequestHandler.alloc.initWithURLOptions(url, $({}));
  if (!handler.performRequestsError($([req]), null)) throw new Error('Vision failed');
  const out = [];
  const results = req.results;
  for (let i = 0; i < results.count; i++) {
    const o = results.objectAtIndex(i);
    const c = o.topCandidates(1);
    if (c.count === 0) continue;
    const t = c.objectAtIndex(0).string.js, b = o.boundingBox;
    // Vision の場所は 0〜1 で、原点が左下
    const box = { t, x: b.origin.x * W, y: (1 - b.origin.y - b.size.height) * H, w: b.size.width * W, h: b.size.height * H };
    out.push({ t, w: [box] });
  }
  return JSON.stringify(out);
}
`

// 画像の文字を、行ごとに場所つきで読み取る (場所は img の座標)
func ocrRead(img image.Image, lang string) ([]ocrLine, error) {
	tmp, err := os.MkdirTemp("", "lumi_ocr_")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	b := img.Bounds()
	// 大きすぎる画像は縮める (Windows の OCR の上限)。場所はあとで元に戻す
	scale := 1.0
	if max(b.Dx(), b.Dy()) > ocrMaxSide {
		scale = float64(ocrMaxSide) / float64(max(b.Dx(), b.Dy()))
		dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
	}
	file := filepath.Join(tmp, "screen.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return nil, err
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	var cmd *exec.Cmd
	tsv := false
	switch runtime.GOOS {
	case "windows":
		script := filepath.Join(tmp, "ocr.ps1")
		// BOM つきの UTF-8 で書く (Windows PowerShell 5.1 は BOM がないと日本語を読み違える)
		if err := os.WriteFile(script, append([]byte{0xEF, 0xBB, 0xBF}, ocrWindowsScript...), 0o600); err != nil {
			return nil, err
		}
		// WinRT を呼べるのは Windows PowerShell (5.1) だけ (PowerShell 7 では使えない)
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
		cmd.Env = append(os.Environ(), "LUMI_OCR_FILE="+file, "LUMI_OCR_LANG="+lang)
	case "darwin":
		cmd = exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", ocrMacScript, file, lang)
	default:
		if _, err := exec.LookPath("tesseract"); err != nil {
			return nil, errors.New(T("ocr.needTesseract"))
		}
		langs := "eng"
		if lang == "ja" {
			langs = "jpn+eng"
		}
		cmd = exec.CommandContext(ctx, "tesseract", file, "stdout", "-l", langs, "tsv")
		tsv = true
	}
	hideWindow(cmd)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if i := strings.Index(msg, "\n"); i >= 0 {
			msg = msg[:i]
		}
		return nil, errors.New(strings.TrimSpace(err.Error() + " " + msg))
	}
	var lines []ocrLine
	if tsv {
		lines = parseTesseractTSV(out.String())
	} else if s := strings.TrimSpace(out.String()); s != "" && s != "null" {
		if err := json.Unmarshal([]byte(strings.TrimPrefix(s, "\uFEFF")), &lines); err != nil {
			return nil, err
		}
	}
	// 場所を元の画像の座標に戻す
	for i := range lines {
		for j := range lines[i].Words {
			w := &lines[i].Words[j]
			w.X, w.Y, w.W, w.H = w.X/scale+float64(b.Min.X), w.Y/scale+float64(b.Min.Y), w.W/scale, w.H/scale
		}
	}
	return lines, nil
}

// tesseract の tsv (level page block par line word left top width height conf text) を行ごとにまとめる
func parseTesseractTSV(s string) []ocrLine {
	var lines []ocrLine
	key := ""
	for _, row := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		f := strings.Split(row, "\t")
		if len(f) < 12 || f[0] != "5" || strings.TrimSpace(f[11]) == "" {
			continue
		}
		num := func(i int) float64 { v, _ := strconv.ParseFloat(f[i], 64); return v }
		k := f[2] + "/" + f[3] + "/" + f[4]
		if k != key || len(lines) == 0 {
			lines = append(lines, ocrLine{})
			key = k
		}
		l := &lines[len(lines)-1]
		l.Words = append(l.Words, ocrWord{T: f[11], X: num(6), Y: num(7), W: num(8), H: num(9)})
		if l.T != "" {
			l.T += " "
		}
		l.T += f[11]
	}
	return lines
}

// 画像の文字を読み取る (行ごとの文)
func ocrImage(img image.Image, lang string) (string, error) {
	lines, err := ocrRead(img, lang)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.T + "\n")
	}
	return tidyOCR(b.String()), nil
}

// 比べるための形: 全角の英数字を半角に、小文字に、空白を除く
func ocrNorm(s string) string {
	s = strings.ToLower(norm.NFKC.String(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// 画面の文字 target がある場所 (真ん中)。いくつ見つかったかも返す (最初に見つかったものの場所)。
// 単語の中の一部に当たったときは、文字の数の割合で場所を見積もる
func findText(lines []ocrLine, target string) (x, y float64, count int) {
	want := []rune(ocrNorm(target))
	if len(want) == 0 {
		return 0, 0, 0
	}
	type ch struct {
		r        rune
		word     *ocrWord
		pos, len int
	}
	exact := -1.0
	var ex, ey float64
	for li := range lines {
		var chars []ch
		for wi := range lines[li].Words {
			w := &lines[li].Words[wi]
			rs := []rune(ocrNorm(w.T))
			for i, r := range rs {
				chars = append(chars, ch{r, w, i, len(rs)})
			}
		}
		for start := 0; start+len(want) <= len(chars); start++ {
			match := true
			for i, r := range want {
				if chars[start+i].r != r {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			count++
			minX, minY, maxX, maxY := 1e18, 1e18, -1e18, -1e18
			for _, c := range chars[start : start+len(want)] {
				w := c.word
				x0 := w.X + w.W*float64(c.pos)/float64(c.len)
				x1 := w.X + w.W*float64(c.pos+1)/float64(c.len)
				minX, maxX = min(minX, x0), max(maxX, x1)
				minY, maxY = min(minY, w.Y), max(maxY, w.Y+w.H)
			}
			cx, cy := (minX+maxX)/2, (minY+maxY)/2
			if count == 1 {
				x, y = cx, cy
			}
			// 行の文字がちょうどそれだけのもの (ボタンの名前など) を優先する
			if exact < 0 && len(want) == len(chars) {
				exact, ex, ey = 1, cx, cy
			}
		}
	}
	if exact > 0 {
		return ex, ey, count
	}
	return x, y, count
}

// 日本語の文字の間に入る空白 (Windows の OCR は 1 文字ずつ区切る)
var cjkSpaceRe = regexp.MustCompile(`([\p{Han}\p{Hiragana}\p{Katakana}ー、。「」・（）！？]) +([\p{Han}\p{Hiragana}\p{Katakana}ー、。「」・（）！？])`)

// 読み取った文字を整える: 日本語の間の空白を詰め、空の行を除く
func tidyOCR(s string) string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		line = strings.TrimFunc(line, unicode.IsSpace)
		if line == "" {
			continue
		}
		for i := 0; i < 2; i++ { // 「あ い う」のように 1 文字ずつのときは 2 回で詰まる
			line = cjkSpaceRe.ReplaceAllString(line, "$1$2")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// AI に渡す長さに収める (長いときは前の方を残す)
func clipOCR(s string, limit int) string {
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "\n…"
	}
	return s
}
