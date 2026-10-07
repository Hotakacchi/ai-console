//go:build manual

package main

import (
	"testing"
	"time"
)

// 実際に Discord のステータスに 60 秒だけ出して、消す: go test -tags manual -run DiscordShow -v
func TestDiscordShow(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	d := &discordRPC{appID: defaultDiscordAppID, on: true, started: time.Now()}
	d.update(T("discord.talking"), "Lumi "+version+" (テスト)")
	d.mu.Lock()
	connected := d.conn != nil && d.last != ""
	d.mu.Unlock()
	if !connected {
		t.Fatal("not shown")
	}
	t.Log("shown")
	time.Sleep(30 * time.Second)
	d.update(T("discord.running"), "Lumi "+version+" (テスト)")
	t.Log("changed")
	time.Sleep(30 * time.Second)
	d.stop()
	t.Log("cleared")
}
