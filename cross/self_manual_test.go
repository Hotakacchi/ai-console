//go:build manual

package main

import (
	"strings"
	"testing"
	"time"
)

// 自分のことを聞いたときの返事を見る: go test -tags manual -run SelfAsk -v
func TestSelfAsk(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	memories.load()
	for _, q := range []string{"今のルミのバージョンは？", "あなたは何ができるの？", "今どの AI を使ってるの？"} {
		s := &Settings{vals: map[string]any{"provider": "local"}}
		p := newLocal(s)
		var out strings.Builder
		p.Reply(Turn{Text: "[" + time.Now().Format("2006-01-02 15:04 (Mon)") + "] " + q}, func(s string) { out.WriteString(s) })
		t.Logf("Q: %s\nA: %s\n", q, out.String())
	}
	var off strings.Builder
	offlineProvider{}.Reply(Turn{Text: "バージョンを教えて"}, func(s string) { off.WriteString(s) })
	t.Logf("offline: %s", off.String())
	localServer.Stop()
}
