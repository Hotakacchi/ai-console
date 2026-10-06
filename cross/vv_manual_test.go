//go:build manual

package main

import (
	"testing"
	"time"
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
