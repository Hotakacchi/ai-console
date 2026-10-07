package main

// Discord のステータス (Rich Presence) に、ルミの様子を出す。
//   /discord on|off   (設定 discord。最初はオフ)
// 出すのは「お話し中」「コマンドを実行中」などの様子だけで、会話の中身は出さない。
// Discord のアプリの ID は discord_app_id (空なら標準の ID)。PC の Discord とは IPC (名前付きパイプ / UNIX ソケット) でつなぐ。

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// 標準の Discord アプリの ID (Discord Developer Portal で作った「Lumi」)。空なら discord_app_id が必要
const defaultDiscordAppID = ""

const (
	discordIcon = "https://raw.githubusercontent.com/Hotakacchi/ai-console/main/lumi.png"
	discordRepo = "https://github.com/Hotakacchi/ai-console"
)

type discordRPC struct {
	mu      sync.Mutex
	conn    io.ReadWriteCloser
	appID   string
	on      bool
	started time.Time
	last    string // 最後に送った内容 (同じなら送らない)
	nonce   int

	// 画面に送ったイベントから分かる様子
	running  bool
	activity string
	thinking bool
}

var discord = &discordRPC{started: time.Now()}

// 画面へのイベントを見て、様子を覚えておく (lumi.emit から呼ぶ)
func (d *discordRPC) observe(name string, data any) {
	switch name {
	case "running", "activity", "thinking":
	default:
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	switch name {
	case "running":
		m, _ := data.(map[string]any)
		d.running, _ = m["on"].(bool)
	case "activity":
		d.activity = ""
		if m, ok := data.(map[string]any); ok {
			d.activity, _ = m["name"].(string)
		}
	case "thinking":
		d.thinking, _ = data.(bool)
	}
}

// 設定に合わせて始める / やめる
func (l *Lumi) applyDiscord() {
	on := l.s.Get("discord", "off") == "on"
	id := strings.TrimSpace(l.s.Get("discord_app_id", ""))
	if id == "" {
		id = defaultDiscordAppID
	}
	discord.mu.Lock()
	changed := discord.appID != id
	discord.on, discord.appID = on && id != "", id
	if (!discord.on || changed) && discord.conn != nil {
		discord.clearLocked()
		discord.conn.Close()
		discord.conn = nil
	}
	discord.last = ""
	discord.mu.Unlock()
	if on && id == "" {
		l.errorText(T("discord.noAppID"))
	}
}

// /discord [on|off]
func (l *Lumi) discordCommand(arg string) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on", "off":
		if l.s.Err == nil {
			l.s.Set("discord", strings.ToLower(strings.TrimSpace(arg)))
		}
		l.applyDiscord()
	}
	discord.mu.Lock()
	on, connected := discord.on, discord.conn != nil
	discord.mu.Unlock()
	switch {
	case !on:
		l.info(T("discord.off"))
	case connected:
		l.info(T("discord.connected"))
	default:
		l.info(T("discord.waiting"))
	}
}

// 数秒ごとに様子を確かめ、変わっていたら Discord に送る (Discord の制限: 20 秒に 5 回まで)
func (l *Lumi) runDiscord() {
	for range time.Tick(5 * time.Second) {
		discord.mu.Lock()
		on := discord.on
		discord.mu.Unlock()
		if !on {
			continue
		}
		details, state := l.discordStatus()
		discord.update(details, state)
	}
}

// 今の様子 (1 行目・2 行目)
func (l *Lumi) discordStatus() (string, string) {
	discord.mu.Lock()
	running, activity, thinking := discord.running, discord.activity, discord.thinking
	discord.mu.Unlock()
	details := T("discord.idle")
	switch {
	case running:
		details = T("discord.running")
	case activity == "search":
		details = T("discord.searching")
	case activity == "download":
		details = T("discord.downloading")
	case thinking:
		details = T("discord.thinking")
	case l.isBusy():
		details = T("discord.talking")
	case l.shellOn:
		details = T("discord.shell")
	}
	return details, "Lumi " + version
}

func (d *discordRPC) update(details, state string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		if err := d.connectLocked(); err != nil {
			return // Discord が動いていない。次の回にまたつなぎに行く
		}
		d.last = ""
	}
	activity := map[string]any{
		"details":    details,
		"state":      state,
		"timestamps": map[string]any{"start": d.started.Unix()},
		"assets":     map[string]any{"large_image": discordIcon, "large_text": "Lumi"},
		"buttons":    []map[string]string{{"label": "GitHub", "url": discordRepo}},
	}
	b, _ := json.Marshal(activity)
	if string(b) == d.last {
		return
	}
	if err := d.sendLocked(1, d.command("SET_ACTIVITY", activity)); err != nil {
		d.conn.Close()
		d.conn = nil
		return
	}
	d.last = string(b)
}

func (d *discordRPC) command(cmd string, activity any) map[string]any {
	d.nonce++
	return map[string]any{
		"cmd":   cmd,
		"args":  map[string]any{"pid": os.Getpid(), "activity": activity},
		"nonce": fmt.Sprint(d.nonce),
	}
}

// 終わるときなどに、ステータスを消す
func (d *discordRPC) clearLocked() {
	if d.conn != nil {
		d.sendLocked(1, d.command("SET_ACTIVITY", nil))
	}
}

func (d *discordRPC) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		d.clearLocked()
		d.conn.Close()
		d.conn = nil
	}
}

// Discord につないで、あいさつ (handshake) する
func (d *discordRPC) connectLocked() error {
	var conn io.ReadWriteCloser
	var err error
	for i := 0; i < 10; i++ {
		if conn, err = dialDiscord(i); err == nil {
			break
		}
	}
	if conn == nil {
		return errors.New("discord is not running")
	}
	d.conn = conn
	if err := d.sendLocked(0, map[string]any{"v": 1, "client_id": d.appID}); err != nil {
		conn.Close()
		d.conn = nil
		return err
	}
	// 返事 (READY か、ID が違うときのエラー) を読む
	op, payload, err := readDiscordFrame(conn)
	if err != nil || op != 1 || !strings.Contains(string(payload), `"READY"`) {
		conn.Close()
		d.conn = nil
		return fmt.Errorf("handshake failed: %s", payload)
	}
	// 以降の返事は読み捨てる (読まないとパイプが詰まる)
	go func() {
		for {
			if _, _, err := readDiscordFrame(conn); err != nil {
				return
			}
		}
	}()
	return nil
}

func (d *discordRPC) sendLocked(op uint32, v any) error {
	payload, _ := json.Marshal(v)
	_, err := d.conn.Write(discordFrame(op, payload))
	return err
}

// Discord の IPC の 1 通: 種類 (4 バイト) + 長さ (4 バイト) + JSON
func discordFrame(op uint32, payload []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, op)
	binary.Write(&b, binary.LittleEndian, uint32(len(payload)))
	b.Write(payload)
	return b.Bytes()
}

func readDiscordFrame(r io.Reader) (uint32, []byte, error) {
	var head [8]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	op, n := binary.LittleEndian.Uint32(head[:4]), binary.LittleEndian.Uint32(head[4:])
	if n > 1<<20 {
		return 0, nil, errors.New("frame too large")
	}
	payload := make([]byte, n)
	_, err := io.ReadFull(r, payload)
	return op, payload, err
}
