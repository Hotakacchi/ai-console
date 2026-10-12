//go:build manual

package main

import (
	"strings"
	"testing"
	"time"
)

// 本物の codex (ChatGPT でログイン済み) で返事をもらう: go test -tags manual -run CodexReal -v -timeout 10m
func TestCodexReal(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	if codexExe() == "" {
		t.Skip("codex is not installed")
	}
	s := &Settings{vals: map[string]any{"provider": "codex", "pc_control": "on"}}
	p := &codexProvider{s: s}
	for _, q := range []string{"こんにちは。一言で自己紹介して。", "Cドライブの空き容量を調べて", "さっき私が最初に言ったことは？"} {
		start := time.Now()
		var got strings.Builder
		p.Reply(Turn{Text: q}, func(s string) { got.WriteString(s) })
		t.Logf("%s (%s)\n→ %s", q, time.Since(start).Round(100*time.Millisecond), got.String())
		if got.Len() == 0 {
			t.Fatal("no reply")
		}
	}
	if !strings.Contains(p.history[3]["content"].(string), "<run") {
		t.Errorf("PC の操作は <run> タグで: %v", p.history[3]["content"])
	}
}
