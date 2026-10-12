package main

// 時間帯で様子が変わる (time_faces): 朝は元気、夜は眠そう、深夜は「そろそろ寝よう」。
// 顔の変化は画面側 (face.js) が時計を見て決める。ここは夜遅いかどうかと、寝ようの声かけ。

import (
	"sync"
	"time"
)

// 夜遅いか (23 時〜朝 5 時前)。VOICEVOX は小声になる
func lateNight(t time.Time) bool {
	h := t.Hour()
	return h >= 23 || h < 5
}

var bedtime struct {
	mu   sync.Mutex
	last string // 声をかけた夜 (その夜の始まりの日付)
}

// 深夜 (1 時〜5 時前) に話しかけられたら、返事のあとに一晩 1 回だけ「そろそろ寝よう」と言う
func (l *Lumi) bedtimeNudge() {
	now := time.Now()
	if h := now.Hour(); h < 1 || h >= 5 || !l.s.On("time_faces", "on") {
		return
	}
	night := now.Add(-12 * time.Hour).Format("2006-01-02")
	bedtime.mu.Lock()
	if bedtime.last == night {
		bedtime.mu.Unlock()
		return
	}
	bedtime.last = night
	bedtime.mu.Unlock()
	l.mu.Lock()
	l.feeling = "" // 返事の気持ちは持ち越さず、夜の小声で
	l.mu.Unlock()
	l.emit("flash", map[string]any{"expr": "sleep", "seconds": 3})
	l.speak(T("night.bedtime"))
	l.write("\n\n", "fg")
}
