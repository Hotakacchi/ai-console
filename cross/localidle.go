package main

// ローカルAIは呼ばれたときだけ読み込む: 起動時には読み込まず、話しかけられたときに読み込み、
// local_unload 分使わなければ外してメモリ (と GPU のメモリ) を空ける。0 なら今までどおり読み込んだまま。

import (
	"strconv"
	"strings"
	"time"
)

const defaultLocalUnload = 5

// 使わない時間がこの分数続いたら外す (0 は外さない)
func (l *Lumi) localUnloadMinutes() int {
	n, err := strconv.Atoi(strings.TrimSpace(l.s.Get("local_unload", strconv.Itoa(defaultLocalUnload))))
	if err != nil || n < 0 {
		return defaultLocalUnload
	}
	return n
}

func (l *Lumi) usingLocal() bool { return strings.HasPrefix(l.ai.Label(), "local:") }

// 30 秒ごとに見て、返事の途中でなく、決めた時間使っていなければ外す
func (l *Lumi) unloadIdleLocal() {
	for range time.Tick(30 * time.Second) {
		mins := l.localUnloadMinutes()
		if mins == 0 || l.isBusy() || !localServer.Running() {
			continue
		}
		l.mu.Lock()
		idle := time.Since(l.lastUsed)
		l.mu.Unlock()
		if idle >= time.Duration(mins)*time.Minute {
			localServer.Stop()
		}
	}
}

// 読み込んでいなければ、待たせる理由を出す (読み込み自体は送る直前に provider が行う)
func (l *Lumi) noteLocalLoading() {
	if l.usingLocal() && !localServer.Running() && localInstalled(dataDir(), l.s.Get("model", "")) {
		l.write(T("local.loading")+"\n", "dim")
	}
}
