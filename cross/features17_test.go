package main

import (
	"testing"
	"time"
)

func TestTidyOCR(t *testing.T) {
	got := tidyOCR("エ ラ ー : フ ァ イ ル が 見 つ か り ま せ ん\r\n\r\n  Get-ChildItem : Cannot find path  \n保 存 し ま す か ？")
	want := "エラー : ファイルが見つかりません\nGet-ChildItem : Cannot find path\n保存しますか？"
	if got != want {
		t.Errorf("got %q", got)
	}
	if clipOCR("あいうえお", 3) != "あいう\n…" {
		t.Error("clipOCR")
	}
}

func TestFormatDuration(t *testing.T) {
	loadLocales()
	if got := formatDuration(80 * time.Second); got != T("done.minSec", 1, 20) {
		t.Errorf("got %q", got)
	}
	if got := formatDuration(45*time.Second + 400*time.Millisecond); got != T("done.sec", 45) {
		t.Errorf("got %q", got)
	}
}

func TestLateNight(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 12, h, 30, 0, 0, time.Local) }
	for h, want := range map[int]bool{22: false, 23: true, 0: true, 4: true, 5: false, 12: false} {
		if lateNight(at(h)) != want {
			t.Errorf("%d時: want %v", h, want)
		}
	}
}

func TestMoodStyle(t *testing.T) {
	v := &voicevoxServer{speakers: []vvSpeaker{
		{Name: "四国めたん", Styles: []struct {
			Name string `json:"name"`
			ID   int    `json:"id"`
		}{{"ノーマル", 2}, {"あまあま", 0}, {"ツンツン", 6}, {"ささやき", 36}}},
		{Name: "春日部つむぎ", Styles: []struct {
			Name string `json:"name"`
			ID   int    `json:"id"`
		}{{"ノーマル", 8}}},
	}}
	cases := []struct {
		style  int
		mood   string
		want   int
		styled bool
	}{
		{2, "happy", 0, true}, {2, "whisper", 36, true}, {2, "sad", 2, false}, {2, "", 2, false},
		{8, "happy", 8, false}, // スタイルがないキャラクターは元のまま (声の高さなどで出す)
	}
	for _, c := range cases {
		if got, styled := v.moodStyle(c.style, c.mood); got != c.want || styled != c.styled {
			t.Errorf("%d %s: got %d %v", c.style, c.mood, got, styled)
		}
	}
	q := map[string]any{"pitchScale": 0.0, "intonationScale": 1.0, "volumeScale": 1.0}
	if speed := moodProsody(q, "sad", 1); speed >= 1 || num(q["pitchScale"]) >= 0 || num(q["intonationScale"]) >= 1 {
		t.Errorf("sad: %v %v", speed, q)
	}
	q = map[string]any{"pitchScale": 0.0, "intonationScale": 1.0, "volumeScale": 1.0}
	if moodProsody(q, "whisper", 1); num(q["volumeScale"]) >= 1 {
		t.Errorf("whisper: %v", q)
	}
}

func TestFullscreenAppDoesNotPanic(t *testing.T) {
	t.Logf("fullscreen app: %v", fullscreenApp())
}

func TestPlainReply(t *testing.T) {
	if got := plainReply("  こんにちは<br>ルミです  "); got != "こんにちは ルミです" && got != "こんにちはルミです" {
		t.Errorf("got %q", got)
	}
}
