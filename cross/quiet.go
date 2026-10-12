package main

// ゲーム中・全画面で動画を見ているときは静かにする (game_quiet。Windows):
// 喋らずに文字だけ出し、隠れているときに小窓を出さない。確かめるのは喋る・知らせるときだけ (見張り続けない)。
// 長いコマンドが終わったら知らせる (done_notice): 30 秒以上かかったコマンドが終わったとき、窓を見ていなければ。

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var quietCache struct {
	mu   sync.Mutex
	at   time.Time
	full bool
}

// 今は静かにするときか (全画面のアプリが前にある)。続けて呼ばれても 2 秒は確かめ直さない
func (l *Lumi) quietNow() bool {
	if !l.s.On("game_quiet", "on") {
		return false
	}
	quietCache.mu.Lock()
	defer quietCache.mu.Unlock()
	if time.Since(quietCache.at) > 2*time.Second {
		quietCache.full, quietCache.at = fullscreenApp(), time.Now()
	}
	return quietCache.full
}

// 知らせるほど長いコマンドの時間
const longCommand = 30 * time.Second

var runningSince struct {
	mu    sync.Mutex
	start time.Time
	cmd   string
}

// 画面へのイベントから、コマンドの始まりと終わりを見る (lumi.emit から呼ぶ)
func (l *Lumi) observeRunning(name string, data any) {
	if name != "running" {
		return
	}
	m, _ := data.(map[string]any)
	on, _ := m["on"].(bool)
	runningSince.mu.Lock()
	if on {
		runningSince.start = time.Now()
		runningSince.cmd, _ = m["cmd"].(string)
		runningSince.mu.Unlock()
		return
	}
	start, cmd := runningSince.start, runningSince.cmd
	runningSince.start = time.Time{}
	runningSince.mu.Unlock()
	if !start.IsZero() {
		if d := time.Since(start); d >= longCommand {
			go l.commandDone(cmd, d)
		}
	}
}

// 長いコマンドが終わった: 窓を見ていなければ、顔を出して知らせる。
// AI が実行したコマンドなら、このあと AI が結果を話すので知らせない
func (l *Lumi) commandDone(cmd string, d time.Duration) {
	if !l.gui() || !l.s.On("done_notice", "on") {
		return
	}
	// シェルモードのコマンドは、終わるとすぐ手が空く。AI のコマンドなら AI の返事が続くので空かない
	for i := 0; i < 20 && l.isBusy(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if l.isBusy() {
		return
	}
	if l.win.IsVisible() && l.win.IsFocused() {
		return
	}
	if r := []rune(strings.TrimSpace(cmd)); len(r) > 30 {
		cmd = string(r[:30]) + "…"
	}
	l.announce(T("done.notice", cmd, formatDuration(d)))
}

// 「1分20秒」「45秒」のような長さ
func formatDuration(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 60 {
		return fmt.Sprintf(T("done.minSec"), s/60, s%60)
	}
	return fmt.Sprintf(T("done.sec"), s)
}
