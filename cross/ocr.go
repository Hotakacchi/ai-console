package main

// 画面の文字を読み取る (OCR)。OS に入っている仕組みを使うので、何もダウンロードしない:
//   Windows: Windows.Media.Ocr (PowerShell 5.1 から WinRT を呼ぶ)
//   Mac:     Vision (JavaScript for Automation から呼ぶ)
//   Linux:   tesseract (入っていれば)
// 画像を読めない AI (画像を読む部品がないローカルAIなど) でも、画面のエラーメッセージなどを読める。

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"

	"golang.org/x/image/draw"
)

// Windows の OCR が読める画像の大きさの上限 (OcrEngine.MaxImageDimension)
const ocrMaxSide = 2600

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
foreach ($line in $result.Lines) { $line.Text }
$stream.Dispose()
`

const ocrMacScript = `
ObjC.import('Foundation'); ObjC.import('Vision');
function run(argv) {
  const url = $.NSURL.fileURLWithPath(argv[0]);
  const req = $.VNRecognizeTextRequest.alloc.init;
  req.recognitionLevel = 0;
  req.usesLanguageCorrection = true;
  try { req.recognitionLanguages = $(argv[1] === 'ja' ? ['ja-JP', 'en-US'] : ['en-US']); } catch (e) {}
  const handler = $.VNImageRequestHandler.alloc.initWithURLOptions(url, $({}));
  if (!handler.performRequestsError($([req]), null)) throw new Error('Vision failed');
  const out = [];
  const results = req.results;
  for (let i = 0; i < results.count; i++) {
    const c = results.objectAtIndex(i).topCandidates(1);
    if (c.count > 0) out.push(c.objectAtIndex(0).string.js);
  }
  return out.join('\n');
}
`

// 画像の文字を読み取る (行ごと)
func ocrImage(img image.Image, lang string) (string, error) {
	tmp, err := os.MkdirTemp("", "lumi_ocr_")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	// 大きすぎる画像は縮める (Windows の OCR の上限)
	if b := img.Bounds(); max(b.Dx(), b.Dy()) > ocrMaxSide {
		scale := float64(ocrMaxSide) / float64(max(b.Dx(), b.Dy()))
		dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
	}
	file := filepath.Join(tmp, "screen.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		script := filepath.Join(tmp, "ocr.ps1")
		// BOM つきの UTF-8 で書く (Windows PowerShell 5.1 は BOM がないと日本語を読み違える)
		if err := os.WriteFile(script, append([]byte{0xEF, 0xBB, 0xBF}, ocrWindowsScript...), 0o600); err != nil {
			return "", err
		}
		// WinRT を呼べるのは Windows PowerShell (5.1) だけ (PowerShell 7 では使えない)
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
		cmd.Env = append(os.Environ(), "LUMI_OCR_FILE="+file, "LUMI_OCR_LANG="+lang)
	case "darwin":
		cmd = exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", ocrMacScript, file, lang)
	default:
		if _, err := exec.LookPath("tesseract"); err != nil {
			return "", errors.New(T("ocr.needTesseract"))
		}
		langs := "eng"
		if lang == "ja" {
			langs = "jpn+eng"
		}
		cmd = exec.CommandContext(ctx, "tesseract", file, "stdout", "-l", langs)
	}
	hideWindow(cmd)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if i := strings.Index(msg, "\n"); i >= 0 {
			msg = msg[:i]
		}
		return "", errors.New(strings.TrimSpace(err.Error() + " " + msg))
	}
	return tidyOCR(out.String()), nil
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
