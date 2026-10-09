package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTriggers(t *testing.T) {
	for in, want := range map[string]string{
		"USB": "drive", "ドライブ": "drive", "充電": "charge", "起動": "startup",
		"終了:Chrome.exe": "exit:chrome", "開始：node": "start:node", `フォルダ:C:\Users\a\Downloads`: `folder:C:\Users\a\Downloads`,
		"exit:": "", "そのうち": "",
	} {
		if got := parseTrigger(in); got != want {
			t.Errorf("parseTrigger(%q) = %q, want %q", in, got, want)
		}
	}
	list := parseRoutines("[おはよう @07:30]\n天気\n[写真 @USB]\n実行: copy {drive}\\DCIM x\n[再起動 @終了:chrome]\n開く: chrome\n")
	if len(list) != 3 || list[0].At != "07:30" || list[0].When != "" || list[1].When != "drive" || list[2].When != "exit:chrome" {
		t.Errorf("routines: %+v", list)
	}
	for _, n := range []string{"a.crdownload", "b.part", "~$c.docx", ".hidden"} {
		if !partialDownload(n) {
			t.Errorf("%s should be skipped", n)
		}
	}
	if partialDownload("report.pdf") {
		t.Error("report.pdf skipped")
	}
}

// フォルダに新しいファイルが来たら、そのきっかけの自動化が {file} つきで動く
func TestFolderTrigger(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	base := tempDataDir(t)
	watched := t.TempDir()
	os.WriteFile(filepath.Join(watched, "old.txt"), []byte("x"), 0o644) // 前からあるものでは動かない
	os.MkdirAll(base, 0o755)
	os.WriteFile(filepath.Join(base, "routines.txt"), []byte("[新着 @フォルダ:"+watched+"]\n言う: 来たよ {file}\n"), 0o644)
	var out bytes.Buffer
	l := &Lumi{s: &Settings{vals: map[string]any{}}, answers: make(chan string, 1), spoken: make(chan int, 8)}
	l.ai = newProvider(l.s)
	l.cli = &cliUI{l: l, out: &out, lineStart: true}
	go l.watchTriggers()
	time.Sleep(12 * time.Second) // 最初の 1 回で今あるものを覚える
	os.WriteFile(filepath.Join(watched, "new.pdf"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(watched, "half.crdownload"), []byte("x"), 0o644)
	for i := 0; i < 20 && !strings.Contains(out.String(), "来たよ"); i++ {
		time.Sleep(time.Second)
	}
	got := out.String()
	if !strings.Contains(got, "来たよ "+filepath.Join(watched, "new.pdf")) || strings.Contains(got, "old.txt") || strings.Contains(got, "half") {
		t.Errorf("output:\n%s", got)
	}
}
