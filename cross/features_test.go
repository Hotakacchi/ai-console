package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNotes(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	tempDataDir(t)
	notes.list, notes.loaded = nil, false
	l := &Lumi{s: &Settings{vals: map[string]any{}}}
	l.noteTag(toolRequest{Kind: "note", Command: "牛乳を買う"})
	l.notesCommand([]string{"歯医者", "の予約"})
	if got := notesText(); !strings.Contains(got, "1. 牛乳を買う") || !strings.Contains(got, "2. 歯医者 の予約") {
		t.Fatalf("notes:\n%s", got)
	}
	if p := notesPrompt(); !strings.Contains(p, "- 牛乳を買う") {
		t.Errorf("prompt: %s", p)
	}
	// 保存して、読み直しても残っている
	notes.list, notes.loaded = nil, false
	if !strings.Contains(notesText(), "牛乳を買う") {
		t.Error("not saved")
	}
	l.notesCommand([]string{"delete", "1"})
	if got := notesText(); strings.Contains(got, "牛乳") || !strings.HasPrefix(got, "1. 歯医者") {
		t.Errorf("after delete:\n%s", got)
	}
	l.notesCommand([]string{"clear"})
	if notesText() != "" {
		t.Error("clear")
	}
}

func TestFindFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("OneDrive", "")
	mk := func(rel string, age time.Duration) {
		p := filepath.Join(home, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
		ts := time.Now().Add(-age)
		os.Chtimes(p, ts, ts)
	}
	mk("Documents/見積書_2026.xlsx", 2*24*time.Hour)
	mk("Documents/old/見積書_2020.xlsx", 400*24*time.Hour)
	mk("Desktop/見積書メモ.txt", time.Hour)
	mk("Downloads/node_modules/見積書.xlsx", time.Hour) // 飛ばすフォルダ
	mk("Documents/写真.png", time.Hour)

	got := findFiles([]string{"見積"}, []string{"xlsx"}, 7)
	if len(got) != 1 || !strings.HasSuffix(got[0].Path, "見積書_2026.xlsx") {
		t.Errorf("xlsx in 7 days: %+v", got)
	}
	got = findFiles([]string{"見積"}, nil, 0)
	if len(got) != 3 || !strings.HasSuffix(got[0].Path, "見積書メモ.txt") { // 新しい順、node_modules は飛ばす
		t.Errorf("all: %+v", got)
	}
	if got := findFiles(nil, []string{".png"}, 0); len(got) != 1 {
		t.Errorf("png: %+v", got)
	}
}

func TestMediaAndClips(t *testing.T) {
	for in, want := range map[string]string{"pause": "playpause", "Next": "next", "previous": "prev", "louder": "volup", "mute": "mute"} {
		if got := normalizeMedia(in); got != want || !validMedia(got) {
			t.Errorf("normalizeMedia(%q) = %q", in, got)
		}
	}
	if validMedia("shutdown") {
		t.Error("unknown action accepted")
	}
	clips.list = []clipEntry{{"https://example.com/a", time.Now()}, {"二つ目の\nコピー", time.Now()}}
	got := clipsText(70)
	if !strings.HasPrefix(got, "1. 二つ目の コピー") || !strings.Contains(got, "2. https://example.com/a") {
		t.Errorf("clips (newest first):\n%s", got)
	}
	clips.list = nil
}
