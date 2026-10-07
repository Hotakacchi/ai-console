//go:build manual

package main

import (
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// VOICEVOX を実際に入れて声を作る確認 (CI では動かさない): go test -tags manual -run VV -timeout 60m
func TestVVInstallAndSynthesize(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	base := dataDir()
	start := time.Now()
	last := -1
	err := installVoicevox(base, func(step string, r float64) {
		if p := int(r * 100); p/10 != last/10 {
			t.Logf("%3d%% %s (%s)", p, step, time.Since(start).Round(time.Second))
			last = p
		}
	}, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if err := voicevox.Start(base); err != nil {
		t.Fatal(err)
	}
	defer voicevox.Stop()
	style := voicevox.pickStyle(-1)
	name, styleName := voicevox.styleName(style)
	t.Logf("voice: %d %s %s, speakers: %d", style, name, styleName, len(voicevox.speakers))
	wav, keys, err := voicevox.Synthesize("こんにちは、ルミです。", style, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) < 1000 || string(wav[:4]) != "RIFF" {
		t.Fatalf("not a wav: %d bytes", len(wav))
	}
	t.Logf("wav %d bytes, %d mouth keys, last at %.2fs", len(wav), len(keys), keys[len(keys)-1].T)
}

// Whisper (base) を実際に入れる確認: go test -tags manual -run Whisper -timeout 60m
func TestWhisperInstall(t *testing.T) {
	model := "whisper-base"
	if m := os.Getenv("WHISPER_MODEL"); m != "" {
		model = m
	}
	base := dataDir()
	last := -1
	err := installWhisper(base, model, func(step string, r float64) {
		if p := int(r * 100); p/10 != last/10 {
			t.Logf("%3d%% %s", p, step)
			last = p
		}
	}, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if !whisperInstalled(base, model) {
		t.Fatal("not installed")
	}
}

// 画面を撮れるか、ローカルAIが画像の文字を読めるかの確認: go test -tags manual -run Vision -timeout 60m
func TestVisionLocal(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	shot, err := captureDisplay()
	if err != nil {
		t.Fatal("capture:", err)
	}
	t.Logf("screen %v -> jpeg %d bytes", shot.Bounds().Size(), len(encodeJPEG(shot)))

	base := dataDir()
	if m := defaultLocalModel(); download(*m.Vision, filepath.Join(modelsDir(base), m.visionFile()), func(int64) {}, func() bool { return false }, nil) != nil {
		t.Fatal(err)
	}
	// 文字を描いた画像を読ませる
	img := image.NewRGBA(image.Rect(0, 0, 320, 80))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: img, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(20, 45)}
	d.DrawString("LUMI 2026 PASSWORD: ORANGE")
	big := image.NewRGBA(image.Rect(0, 0, 960, 240))
	xdraw.NearestNeighbor.Scale(big, big.Bounds(), img, img.Bounds(), draw.Src, nil)

	s := &Settings{vals: map[string]any{"provider": "local"}}
	p := newLocal(s)
	defer localServer.Stop()
	var out strings.Builder
	start := time.Now()
	p.Reply(Turn{Text: "画像に書いてある英語の文字をそのまま書き写して。", Images: [][]byte{encodeJPEG(big)}}, func(s string) { out.WriteString(s) })
	t.Logf("%s: %q", time.Since(start).Round(time.Millisecond), out.String())
	if !strings.Contains(strings.ToUpper(out.String()), "ORANGE") {
		t.Error("could not read the image")
	}
}
