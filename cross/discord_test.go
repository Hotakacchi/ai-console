package main

import (
	"bytes"
	"testing"
)

func TestDiscordFrame(t *testing.T) {
	f := discordFrame(1, []byte(`{"cmd":"SET_ACTIVITY"}`))
	if len(f) != 8+22 || f[0] != 1 || f[4] != 22 {
		t.Fatalf("% x", f[:8])
	}
	op, payload, err := readDiscordFrame(bytes.NewReader(f))
	if err != nil || op != 1 || string(payload) != `{"cmd":"SET_ACTIVITY"}` {
		t.Errorf("%d %q %v", op, payload, err)
	}
	if _, _, err := readDiscordFrame(bytes.NewReader(f[:5])); err == nil {
		t.Error("short frame accepted")
	}
}

func TestDiscordObserve(t *testing.T) {
	d := &discordRPC{}
	d.observe("running", map[string]any{"on": true, "cmd": "dir"})
	d.observe("activity", map[string]any{"name": "search"})
	d.observe("write", map[string]any{"text": "secret"}) // 会話の中身は見ない
	if !d.running || d.activity != "search" {
		t.Errorf("%+v", d)
	}
	d.observe("activity", nil)
	if d.activity != "" {
		t.Error("activity not cleared")
	}
}
