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

func TestRepeatsEarlier(t *testing.T) {
	a := "ほたかっち、ルミの機能は「/help」コマンドを言ってくださいね！最新バージョンがあるかも「/update」で確認できますよ。"
	b := "ほたかっち、PC の名前が「ASUS-YUTO」って確認できましたね！ルミの新機能はさっき言った通り /help と言って教えてあげてください。"
	c := "ほたかっち、PC の名前はやっぱり「ASUS-YUTO」ですね！ルミの最新機能は /help で教えてもらうのが一番早いですよ。"
	if !repeatsEarlier([]string{a, b}, b) || !repeatsEarlier([]string{b}, c) {
		t.Error("repeated answers were not noticed")
	}
	if repeatsEarlier([]string{"Cドライブの空き容量を調べますね。"}, "空き容量は 22GB でした。少なめなので、いらないファイルを消すと安心です。") {
		t.Error("a real follow-up was treated as a repeat")
	}
	if repeatsEarlier(nil, a) || repeatsEarlier([]string{a}, "はい") {
		t.Error("short or first answers are never repeats")
	}
}

func TestAskingAboutLumi(t *testing.T) {
	for q, want := range map[string]bool{
		"ルミの新機能を教えて": true, "ルミってどんなことができるの？": true, "ルミの設定の変え方": true,
		"What can Lumi do? features": true, "今日の天気は？": false, "ルミ、こんにちは": false, "PC の設定を開いて": false,
	} {
		if got := askingAboutLumi(q); got != want {
			t.Errorf("askingAboutLumi(%q) = %v", q, got)
		}
	}
}

func TestNotesForLang(t *testing.T) {
	b, err := os.ReadFile("../release-notes/v1.6.0.md")
	if err != nil {
		t.Skip("no release notes")
	}
	loadLocales()
	setLanguage("ja")
	ja := notesForLang(string(b))
	setLanguage("en")
	en := notesForLang(string(b))
	setLanguage("ja")
	if !strings.Contains(ja, "設定画面") || strings.Contains(ja, "Settings window") || strings.Contains(ja, "インストール / Install") {
		t.Errorf("ja part:\n%s", ja)
	}
	if !strings.Contains(en, "Settings window") || strings.Contains(en, "設定画面") || strings.Contains(en, "Lumi-Windows-Setup") {
		t.Errorf("en part:\n%s", en)
	}
	if !asksWhatsNew("ルミの新機能を教えて") || asksWhatsNew("ルミの使い方") {
		t.Error("asksWhatsNew")
	}
}
