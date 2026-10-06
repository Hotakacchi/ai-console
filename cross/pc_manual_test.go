//go:build manual

package main

import (
	"strings"
	"testing"
	"time"
)

// PC の操作を頼んだときの返事を見る (コマンドは実行しない): go test -tags manual -run PCAsk -v
func TestPCAsk(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	memories.load()
	for _, q := range []string{"ipconfigを実行して", "Windowsのバージョンを調べて", "コマンドプロンプトでdirを実行して", "メモリの使用量を調べて", "デスクトップにあるファイルを一覧にして"} {
		s := &Settings{vals: map[string]any{"provider": "local"}}
		p := newLocal(s)
		var out strings.Builder
		p.Reply(Turn{Text: "[" + time.Now().Format("2006-01-02 15:04 (Mon)") + "] " + q}, func(s string) { out.WriteString(s) })
		t.Logf("Q: %s\nA: %s\n", q, out.String())
	}
	localServer.Stop()
}
