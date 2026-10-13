//go:build manual

package main

import (
	"os"
	"strings"
	"testing"
)

// AI の返事を、タグも含めてそのまま見る (何も実行しない):
// LUMI_PROVIDER=local LUMI_MODEL=qwen3.5-9b LUMI_Q=... go test -tags manual -run RawAsk -v -timeout 10m
func TestRawAsk(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	s := &Settings{vals: map[string]any{"provider": os.Getenv("LUMI_PROVIDER"), "model": os.Getenv("LUMI_MODEL"), "pc_control": "on", "auto_run": os.Getenv("LUMI_AUTO")}}
	p := newProvider(s)
	defer localServer.Stop()
	var b strings.Builder
	p.Reply(Turn{Text: os.Getenv("LUMI_Q")}, func(x string) { b.WriteString(x) })
	t.Logf("\n%s", b.String())
}
