package main

// 使っていない部品は休ませて軽くする: ローカルAI・VOICEVOX・Whisper のモデルは、
// 使うときに読み込み、idle_unload 分 (返事も呼びかけもない時間) が続いたら外してメモリ (と GPU のメモリ) を空ける。
// 0 なら外さない (ローカルAIも起動時に読み込んでおく)。

import (
	"strconv"
	"strings"
	"time"
)

const defaultIdleUnload = 1

// 使わない時間がこの分数続いたら外す (0 は外さない)
func (l *Lumi) idleUnloadMinutes() int {
	n, err := strconv.Atoi(strings.TrimSpace(l.s.Get("idle_unload", strconv.Itoa(defaultIdleUnload))))
	if err != nil || n < 0 {
		return defaultIdleUnload
	}
	return n
}

func (l *Lumi) usingLocal() bool { return strings.HasPrefix(l.ai.Label(), "local:") }

// 30 秒ごとに見て、返事の途中でなく、決めた時間使っていなければ外す (外すのは使わない時間 1 回につき 1 回)
func (l *Lumi) unloadIdle() {
	done := false
	for range time.Tick(30 * time.Second) {
		mins := l.idleUnloadMinutes()
		l.mu.Lock()
		idle := time.Since(l.lastUsed)
		l.mu.Unlock()
		if mins == 0 || l.isBusy() || idle < time.Duration(mins)*time.Minute {
			done = false
			continue
		}
		if done {
			continue
		}
		done = true
		localServer.Stop()
		voicevox.Stop()
		l.emit("whisperUnload", nil) // Whisper は画面側で動いているので、画面に外してもらう
	}
}

// ローカルAIを読み込んでいなければ、待たせる理由を出す (読み込み自体は送る直前に provider が行う)
func (l *Lumi) noteLocalLoading() {
	if l.usingLocal() && !localServer.Running() && localInstalled(dataDir(), l.s.Get("model", "")) {
		l.write(T("local.loading")+"\n", "dim")
	}
}
