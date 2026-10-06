//go:build manual

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 確認の答えを声で言った WAV を VOICEVOX で作る (聞き取りの確認用): go test -tags manual -run LetterWavs
func TestLetterWavs(t *testing.T) {
	loadLocales()
	if err := voicevox.Start(dataDir()); err != nil {
		t.Fatal(err)
	}
	defer voicevox.Stop()
	style := voicevox.pickStyle(-1)
	for name, text := range map[string]string{"y1": "ワイ", "y2": "ワイ。", "a1": "エー", "a2": "エイ", "n1": "エヌ", "n2": "ノー", "f1": "えー、どうしようかな", "r1": "実行して", "s1": "やめて"} {
		wav, _, err := voicevox.Synthesize(text, style, 0)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(voiceDir(dataDir()), "conf_"+name+".wav"), wav, 0o644)
	}
}
