package main

import (
	"strings"
	"testing"
)

func TestParseUIActions(t *testing.T) {
	loadLocales()
	acts, err := parseUIActions("\n1. click 「保存」\n- type レポート 2026\nkey ctrl+s\nスクロール down 3\n右クリック @640, 360\nwait\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []uiAction{{"click", "保存"}, {"type", "レポート 2026"}, {"key", "ctrl+s"}, {"scroll", "down 3"}, {"rightclick", "@640, 360"}, {"wait", "1"}}
	if len(acts) != len(want) {
		t.Fatalf("got %v", acts)
	}
	for i := range want {
		if acts[i] != want[i] {
			t.Errorf("%d: got %v want %v", i, acts[i], want[i])
		}
	}
	if x, y, ok := parsePoint(acts[4].Arg); !ok || x != 640 || y != 360 {
		t.Errorf("point %v %v %v", x, y, ok)
	}
	for _, bad := range []string{"", "jump 3", "click", "type"} {
		if _, err := parseUIActions(bad); err == nil {
			t.Errorf("should fail: %q", bad)
		}
	}
}

func TestFindText(t *testing.T) {
	lines := []ocrLine{
		{T: "保 存 し ま す か ? は い / い い え", Words: []ocrWord{
			{"保", 100, 10, 20, 20}, {"存", 120, 10, 20, 20}, {"し", 140, 10, 20, 20}, {"ま", 160, 10, 20, 20}, {"す", 180, 10, 20, 20}, {"か", 200, 10, 20, 20},
			{"は", 260, 10, 20, 20}, {"い", 280, 10, 20, 20}, {"い", 320, 10, 20, 20}, {"い", 340, 10, 20, 20}, {"え", 360, 10, 20, 20}}},
		{T: "Save As", Words: []ocrWord{{"Save", 100, 100, 80, 20}, {"As", 190, 100, 30, 20}}},
		{T: "Save", Words: []ocrWord{{"Save", 400, 200, 80, 20}}},
		{T: "ファイル名", Words: []ocrWord{{"ファイル名", 100, 300, 100, 20}}}, // Mac のように行で 1 つのとき
	}
	check := func(q string, wx, wy float64, wn int) {
		t.Helper()
		x, y, n := findText(lines, q)
		if n != wn || x != wx || y != wy {
			t.Errorf("%q: got (%v,%v)×%d want (%v,%v)×%d", q, x, y, n, wx, wy, wn)
		}
	}
	check("いいえ", 350, 20, 1)
	check("保存", 120, 20, 1)
	check("save", 440, 210, 2) // 「Save」だけの行 (ボタン) を優先
	check("Ｓａｖｅ　Ａｓ", 160, 110, 1)
	check("イル", 160, 310, 1) // 単語の一部は文字の割合で見積もる
	check("ない", 0, 0, 0)
}

// 記録の間に実行したコマンドが入っても、話しかけた文と返事をまとめられる
func TestArchiveItemsSkipCommands(t *testing.T) {
	loadLocales()
	items := archiveItems([]historyEntry{
		{Role: "user", Text: "メモ帳を開いて"}, {Role: "command", Text: "Start-Process notepad"}, {Role: "assistant", Text: "開きました"},
		{Role: "user", Text: "ありがとう"},
	})
	if len(items) != 2 || !strings.Contains(items[0], "開きました") || strings.Contains(items[1], "開きました") {
		t.Errorf("%q", items)
	}
}

// 小さいモデルが改行を落としても読める
func TestParseUIGlued(t *testing.T) {
	loadLocales()
	for _, body := range []string{"wait 2type こんにちは", "wait 2; type こんにちは", "scroll down 3click 保存"} {
		acts, err := parseUIActions(body)
		if err != nil || len(acts) != 2 {
			t.Errorf("%q: %v %v", body, acts, err)
		}
	}
	acts, _ := parseUIActions("wait 2type こんにちは")
	if acts[0] != (uiAction{"wait", "2"}) || acts[1] != (uiAction{"type", "こんにちは"}) {
		t.Errorf("%v", acts)
	}
}
