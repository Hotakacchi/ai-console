//go:build manual

package main

import (
	"testing"
	"time"
)

// SET_ACTIVITY への Discord の返事を見る (2 秒だけ出して消す): go test -tags manual -run DiscordReply -v
func TestDiscordReply(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	d := &discordRPC{appID: defaultDiscordAppID, on: true, started: time.Now()}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.connectLocked(); err != nil {
		t.Fatal(err)
	}
	activity := map[string]any{
		"details":    T("discord.talking"),
		"state":      "Lumi " + version + " (テスト)",
		"timestamps": map[string]any{"start": d.started.Unix()},
		"assets":     map[string]any{"large_image": discordIcon, "large_text": "Lumi"},
		"buttons":    []map[string]string{{"label": "GitHub", "url": discordRepo}},
	}
	op, reply, err := d.exchangeLocked(1, d.command("SET_ACTIVITY", activity))
	t.Logf("op=%d err=%v\n%s", op, err, reply)
	time.Sleep(2 * time.Second)
	d.clearLocked()
	d.dropLocked()
}
