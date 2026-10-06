//go:build manual

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 聞き取りの確認用の WAV を VOICEVOX で作る: go test -tags manual -run LetterWavs
func TestLetterWavs(t *testing.T) {
	loadLocales()
	if err := voicevox.Start(dataDir()); err != nil {
		t.Fatal(err)
	}
	defer voicevox.Stop()
	style := voicevox.pickStyle(-1)
	for name, text := range map[string]string{
		// 確認の答え
		"conf_y1": "ワイ", "conf_y2": "ワイ。", "conf_a1": "エー", "conf_a2": "エイ", "conf_n1": "エヌ", "conf_n2": "ノー",
		"conf_f1": "えー、どうしようかな", "conf_r1": "実行して", "conf_s1": "やめて",
		// 固有名詞 (単語帳の効き目を見る)
		"vocab_1": "ほたかっちのフォルダを開いて", "vocab_2": "ずんだもんの声に変えて", "vocab_3": "春日部つむぎで読み上げて",
		"vocab_4": "ヴォイスボックスを再起動して", "vocab_5": "鳴海くんにメールを送る準備をして",
	} {
		wav, _, err := voicevox.Synthesize(text, style, 0)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(voiceDir(dataDir()), name+".wav"), wav, 0o644)
	}
}
