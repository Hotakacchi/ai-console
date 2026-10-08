//go:build manual

package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// モデルを実際に入れて話してみる: LUMI_MODEL=qwen3.5-0.8b go test -tags manual -run LocalModelChat -v -timeout 60m
func TestLocalModelChat(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	id := os.Getenv("LUMI_MODEL")
	if id == "" {
		id = "qwen3.5-0.8b"
	}
	m, ok := findLocalModel(id)
	if !ok {
		t.Fatal("no model", id)
	}
	if os.Getenv("LUMI_TEMP") != "" {
		tempDataDir(t) // 本物のデータフォルダを汚さない (llama.cpp もダウンロードし直す)
	}
	base := dataDir()
	start := time.Now()
	last := -1
	if err := installLocal(base, m, func(step string, r float64) {
		if p := int(r * 100); p/20 != last/20 {
			t.Logf("%3d%% %s", p, step)
			last = p
		}
	}, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	t.Logf("installed %s in %s", m.Name, time.Since(start).Round(time.Second))
	s := &Settings{vals: map[string]any{"provider": "local", "model": m.ID}}
	p := newLocal(s)
	defer localServer.Stop()
	for _, q := range []string{"こんにちは！自己紹介して", "Cドライブの空き容量を調べて"} {
		var out strings.Builder
		start = time.Now()
		p.Reply(Turn{Text: q}, func(s string) { out.WriteString(s) })
		t.Logf("%s (%s)\nQ: %s\nA: %s", p.Label(), time.Since(start).Round(time.Millisecond), q, out.String())
		if strings.TrimSpace(out.String()) == "" {
			t.Error("no reply")
		}
	}
}
