//go:build manual

package main

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// 本物のローカルAIに 1 つ聞いて、画面に出る様子を見る (履歴・記録には残さない):
// LUMI_MODEL=qwen3.5-9b LUMI_Q=ルミの新機能を教えて go test -tags manual -run LocalAsk -v -timeout 10m
func TestLocalAsk(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	var out bytes.Buffer
	model := os.Getenv("LUMI_MODEL")
	l := &Lumi{s: &Settings{vals: map[string]any{"provider": "local", "model": model, "keep_history": "off", "long_memory": "off", "pc_control": "on"}},
		answers: make(chan string, 1), spoken: make(chan int, 8)}
	l.ai = newProvider(l.s)
	l.cli = &cliUI{l: l, out: &out, lineStart: true}
	defer localServer.Stop()
	go func() { // コマンドの確認には「いいえ」
		for {
			time.Sleep(200 * time.Millisecond)
			select {
			case l.answers <- "n":
			default:
			}
		}
	}()
	start := time.Now()
	if !l.begin() {
		t.Fatal("busy")
	}
	l.respond(os.Getenv("LUMI_Q"), false)
	t.Logf("%s\n%s", time.Since(start).Round(time.Second), out.String())
}
