package main

import (
	"os"
	"strings"
	"testing"
)

func TestLumiDoc(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	md, err := os.ReadFile("../README.md")
	if err != nil {
		t.Skip("no README")
	}
	items := docItems(string(md))
	if len(items) < 20 {
		t.Fatalf("only %d items", len(items))
	}
	for _, c := range []struct{ q, want string }{
		{"ルミでスマホから話しかけるには？", "/phone"},
		{"ルミのプラグインの作り方", "plugins"},
		{"ルミの自動モードって何", "/auto"},
		{"ルミのローカルAIのモデルを変えたい", "/install-local"},
		{"ルミで USB メモリをつないだらどうなる？", "USB"},
	} {
		picked := pickDoc(items, c.q, 2500)
		joined := strings.Join(picked, "\n")
		if !strings.Contains(joined, c.want) {
			t.Errorf("%s: %q not in\n%s", c.q, c.want, joined)
		}
		if n := len([]rune(joined)); n > 2600 {
			t.Errorf("%s: too long (%d)", c.q, n)
		}
	}
	if !aboutLumi("ルミ アプリ 使い方") || !aboutLumi("Lumi assistant features") || aboutLumi("今日の天気") {
		t.Error("aboutLumi")
	}
	// 英語のときは English の部分だけ
	setLanguage("en")
	for _, it := range docItems(string(md)) {
		if strings.Contains(it, "インストール") {
			t.Errorf("Japanese item in English: %s", it)
			break
		}
	}
}
