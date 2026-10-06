package main

// 画面を見せる: スクリーンショットを撮って AI に見せる (<screen/> か /screen)。
// ローカルAI (Qwen3.5-4B) は画像を読む部品 (mmproj) を足すと画像も読める (/install-vision)。

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kbinani/screenshot"
	"golang.org/x/image/draw"
)

// 標準モデル用の画像を読む部品
const visionFile = "mmproj-Qwen3.5-4B-F16.gguf"

var visionAsset = asset{
	"https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/resolve/e87f176479d0855a907a41277aca2f8ee7a09523/mmproj-F16.gguf",
	672423616,
	"cd88edcf8d031894960bb0c9c5b9b7e1fea6ebee02b9f7ce925a00d12891f864",
}

// 標準モデルを使っていて部品が入っていれば、その場所 (なければ "")
func visionPath(base, modelPath string) string {
	if modelPath != localModelPath(base, "") {
		return ""
	}
	p := filepath.Join(modelsDir(base), visionFile)
	if st, err := os.Stat(p); err == nil && st.Size() == visionAsset.Size {
		return p
	}
	return ""
}

// ローカルAIのコンテキストの長さ (トークン)
func localContext(s *Settings) int {
	return max(12288, s.GetInt("max_tokens", 0)+4096)
}

// 今の AI に画像を見せられるか。だめなら理由
func (l *Lumi) canSeeImages() (bool, string) {
	switch strings.ToLower(l.s.Get("provider", "offline")) {
	case "offline":
		return false, T("vision.offline")
	case "local":
		dir := dataDir()
		if visionPath(dir, localModelPath(dir, l.s.Get("model", ""))) == "" {
			return false, T("vision.needInstall", float64(visionAsset.Size)/1e6)
		}
	}
	return true, ""
}

// /install-vision: ローカルAIに画像を読む部品を足す
func (l *Lumi) installVisionCmd() {
	dir := dataDir()
	if !localInstalled(dir) {
		l.errorText(T("local.notInstalledHint", float64(localTotalSize())/1e9))
		return
	}
	l.setBusy(true)
	cancel := make(chan struct{})
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	l.write(T("vision.installStart", float64(visionAsset.Size)/1e6)+"\n", "dim")
	go func() {
		defer l.setBusy(false)
		lastPct := -1
		err := download(visionAsset, filepath.Join(modelsDir(dir), visionFile), func(n int64) {
			if pct := int(float64(n) / float64(visionAsset.Size) * 100); pct/10 != lastPct/10 {
				l.write(fmt.Sprintf("  [%3d%%] %s\n", pct, T("vision.downloading")), "dim")
				lastPct = pct
			}
		}, func() bool {
			select {
			case <-cancel:
				return true
			default:
				return false
			}
		}, func() { l.write("  "+T("local.verifying")+"\n", "dim") })
		switch {
		case err == errCancelled:
			l.write("^C\n"+T("local.installCancelled", "/install-vision")+"\n\n", "dim")
		case err != nil:
			l.errorText(T("download.failed", err.Error()))
		default:
			l.info(T("vision.installed"))
			l.warmupLocal() // 部品つきで起動し直す
		}
	}()
}

// ---- スクリーンショット ----

// AI の <screen/> と /screen: 画面を撮って JPEG で返す (screen が ask なら毎回確認)。
// だめなときは AI に返す説明を返す
func (l *Lumi) screenTool() ([]byte, string) {
	if ok, why := l.canSeeImages(); !ok {
		l.write(why+"\n", "yellow")
		return nil, why
	}
	switch strings.ToLower(l.s.Get("screen", "ask")) {
	case "off":
		return nil, T("screen.off")
	case "ask":
		if a := strings.ToLower(l.ask(T("screen.ask"))); a != "y" && a != "a" {
			l.write(T("web.stopped")+"\n", "dim")
			return nil, T("web.denied")
		}
	}
	img, err := l.captureScreen()
	if err != nil {
		l.errorText(T("screen.failed", err.Error()))
		return nil, T("screen.failed", err.Error())
	}
	l.write(T("screen.taken", img.Bounds().Dx(), img.Bounds().Dy())+"\n\n", "cyan")
	return encodeJPEG(img), ""
}

// ルミの窓が写り込まないよう、少しの間隠してから撮る
func (l *Lumi) captureScreen() (image.Image, error) {
	visible := l.win.IsVisible()
	if visible {
		l.win.Hide()
		time.Sleep(400 * time.Millisecond)
		defer l.show()
	}
	return captureDisplay()
}

func captureDisplay() (image.Image, error) {
	if screenshot.NumActiveDisplays() > 0 {
		if img, err := screenshot.CaptureDisplay(0); err == nil {
			return img, nil
		} else if runtime.GOOS == "windows" {
			return nil, err
		}
	}
	// Mac・Linux で撮れなかったときは OS のコマンドで
	tmp, err := os.MkdirTemp("", "lumi_screen_")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "screen.png")
	var tries [][]string
	if runtime.GOOS == "darwin" {
		tries = [][]string{{"screencapture", "-x", "-m", out}}
	} else {
		tries = [][]string{{"grim", out}, {"gnome-screenshot", "-f", out}, {"spectacle", "-b", "-n", "-f", "-o", out}, {"scrot", "-o", out}, {"import", "-window", "root", out}}
	}
	for _, t := range tries {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		if exec.Command(t[0], t[1:]...).Run() == nil {
			if f, err := os.Open(out); err == nil {
				img, _, err := image.Decode(f)
				f.Close()
				if err == nil {
					return img, nil
				}
			}
		}
	}
	return nil, errors.New(T("screen.noTool"))
}

// 長い辺が maxImageSide を超えないよう縮めて JPEG にする (AI が読める大きさ・速さにする)
const maxImageSide = 1280

func encodeJPEG(img image.Image) []byte {
	b := img.Bounds()
	if w, h := b.Dx(), b.Dy(); w > maxImageSide || h > maxImageSide {
		scale := float64(maxImageSide) / float64(max(w, h))
		dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}
