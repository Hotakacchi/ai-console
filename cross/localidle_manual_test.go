//go:build manual

package main

import (
	"strings"
	"testing"
	"time"
)

// 呼ばれたときだけ読み込み、使わなければ外す: go test -tags manual -run LocalUnload -v -timeout 10m
func TestLocalUnload(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	l := &Lumi{s: &Settings{vals: map[string]any{"provider": "local", "model": "qwen3.5-0.8b", "local_unload": 1, "keep_history": "off"}},
		answers: make(chan string, 1), spoken: make(chan int, 8)}
	l.ai = newProvider(l.s)
	defer localServer.Stop()
	localServer.Stop()

	l.warmupLocal() // 先には読み込まない
	if localServer.Running() {
		t.Fatal("loaded at startup")
	}
	var log strings.Builder
	l.cli = &cliUI{l: l, out: &log, lineStart: true}
	go l.unloadIdleLocal()

	start := time.Now()
	if !l.begin() {
		t.Fatal("busy")
	}
	l.respond("こんにちは", false)
	t.Logf("first reply in %s", time.Since(start).Round(time.Millisecond))
	if !strings.Contains(log.String(), T("local.loading")) {
		t.Error("no loading notice")
	}
	if !localServer.Running() {
		t.Fatal("not loaded after the reply")
	}
	for i := 0; i < 8 && localServer.Running(); i++ {
		time.Sleep(15 * time.Second)
	}
	if localServer.Running() {
		t.Fatal("still loaded after the idle time")
	}
	t.Logf("unloaded after %s", time.Since(start).Round(time.Second))
	t.Log(log.String())
}
