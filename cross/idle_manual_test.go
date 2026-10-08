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
	l := &Lumi{s: &Settings{vals: map[string]any{"provider": "local", "model": "qwen3.5-0.8b", "idle_unload": 1, "keep_history": "off"}},
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
	go l.unloadIdle()

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

// VOICEVOX を止めても、次に使うときに起動し直せる: go test -tags manual -run VoicevoxRestart -v
func TestVoicevoxRestart(t *testing.T) {
	loadLocales()
	base := dataDir()
	defer voicevox.Stop()
	for i := 0; i < 2; i++ {
		start := time.Now()
		if err := voicevox.Start(base); err != nil {
			t.Fatal(err)
		}
		wav, _, err := voicevox.Synthesize("テストです。", voicevox.pickStyle(-1), 1)
		if err != nil || len(wav) < 1000 || !voicevox.Running() {
			t.Fatalf("round %d: %v %d", i, err, len(wav))
		}
		t.Logf("round %d: started and spoke in %s", i, time.Since(start).Round(time.Millisecond))
		voicevox.Stop()
		if voicevox.Running() {
			t.Fatal("still running after Stop")
		}
	}
}
