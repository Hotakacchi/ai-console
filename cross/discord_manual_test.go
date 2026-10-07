//go:build manual

package main

import "testing"

// PC の Discord につながるかだけ確かめる (あいさつだけで、ステータスは変えない):
// go test -tags manual -run DiscordPipe -v
func TestDiscordPipe(t *testing.T) {
	conn, err := dialDiscord(0)
	if err != nil {
		t.Skip("Discord is not running")
	}
	defer conn.Close()
	conn.Write(discordFrame(0, []byte(`{"v":1,"client_id":"`+defaultDiscordAppID+`"}`)))
	op, payload, err := readDiscordFrame(conn)
	t.Logf("op=%d payload=%s err=%v", op, payload, err)
	if err != nil {
		t.Fatal(err)
	}
}
