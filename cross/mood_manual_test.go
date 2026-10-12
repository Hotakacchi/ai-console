//go:build manual

package main

import "testing"

// VOICEVOX で気持ちの声色を作れるか (音は鳴らさない): go test -tags manual -run MoodVoice -v
func TestMoodVoice(t *testing.T) {
	loadLocales()
	if !voicevoxInstalled(dataDir()) {
		t.Skip("VOICEVOX is not installed")
	}
	if err := voicevox.Start(dataDir()); err != nil {
		t.Fatal(err)
	}
	defer voicevox.Stop()
	for _, s := range voicevox.speakers {
		names := ""
		for _, st := range s.Styles {
			names += st.Name + " "
		}
		t.Logf("%s: %s", s.Name, names)
	}
	base := voicevox.pickStyle(-1)
	for _, mood := range []string{"", "happy", "sad", "surprised", "whisper"} {
		style, styled := voicevox.moodStyle(base, mood)
		wav, _, err := voicevox.Synthesize("今日はいい天気ですね。", base, 1, mood)
		if err != nil {
			t.Fatal(mood, err)
		}
		t.Logf("mood %-9q style %d (styled %v) wav %d bytes", mood, style, styled, len(wav))
	}
}
