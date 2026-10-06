//go:build manual

package main

import (
	"fmt"
	"testing"
	"time"
)

// スマホの画面を見るためにサーバーだけ動かす (90 秒): go test -tags manual -run PhonePage -v
func TestPhonePage(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	l := &Lumi{answers: make(chan string, 1)}
	if err := phone.start(l, 47811); err != nil {
		t.Fatal(err)
	}
	defer phone.stop()
	fmt.Printf("URL http://127.0.0.1:47811/?t=%s\n", phoneToken(false))
	time.Sleep(20 * time.Second)
	phone.publish("echo", "今日の天気は？  📱")
	phone.publish("thinking", true)
	time.Sleep(time.Second)
	phone.publish("thinking", false)
	phone.publish("speak", map[string]any{"text": "今日の東京は曇りで、最高 25 度です。"})
	phone.publish("speak", map[string]any{"text": "傘はいらなそうですよ。"})
	phone.publish("write", map[string]any{"text": "\n\n", "color": "fg"})
	phone.publish("write", map[string]any{"text": "\n実行してよいコマンド:\n  Get-PSDrive C\n", "color": "yellow"})
	phone.publish("ask", "実行しますか？ [y=実行 / a=管理者として実行 / N=やめる] ")
	time.Sleep(80 * time.Second)
}
