package main

// タイマー・リマインダー。AI が <remind in="300">…</remind> や <remind at="15:00">…</remind> で予約する。
// データフォルダの reminders.json に保存するので、再起動しても残る。
// 時間になると、トレイにいても右下から顔が出て、声で知らせる。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type reminder struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

type reminderStore struct {
	mu    sync.Mutex
	items []reminder
}

var reminders = &reminderStore{}

func remindersPath() string { return filepath.Join(dataDir(), "reminders.json") }

func (r *reminderStore) load() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = nil
	if data, err := os.ReadFile(remindersPath()); err == nil {
		json.Unmarshal(data, &r.items)
	}
}

func (r *reminderStore) saveLocked() {
	sort.Slice(r.items, func(i, j int) bool { return r.items[i].At.Before(r.items[j].At) })
	if r.items == nil {
		r.items = []reminder{} // 空でも null ではなく [] と書く
	}
	data, _ := json.MarshalIndent(r.items, "", "  ")
	os.MkdirAll(dataDir(), 0o755)
	os.WriteFile(remindersPath(), data, 0o644)
}

func (r *reminderStore) add(at time.Time, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, reminder{at, text})
	r.saveLocked()
}

// 時間が来たものを取り出す
func (r *reminderStore) due(now time.Time) []reminder {
	r.mu.Lock()
	defer r.mu.Unlock()
	var due, rest []reminder
	for _, it := range r.items {
		if !it.At.After(now) {
			due = append(due, it)
		} else {
			rest = append(rest, it)
		}
	}
	if len(due) > 0 {
		r.items = rest
		r.saveLocked()
	}
	return due
}

func (r *reminderStore) list() []reminder {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]reminder(nil), r.items...)
}

func (r *reminderStore) removeAt(i int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < 0 || i >= len(r.items) {
		return false
	}
	r.items = append(r.items[:i], r.items[i+1:]...)
	r.saveLocked()
	return true
}

func (r *reminderStore) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = nil
	r.saveLocked()
}

var durationRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(s|sec|秒|m|min|分|h|hour|時間)?$`)

// in="300" / "5m" / "1h"、at="15:00" / "2026-10-07 09:00" を時刻にする
func reminderTime(attrs map[string]string, now time.Time) (time.Time, bool) {
	if in := strings.TrimSpace(strings.ToLower(attrs["in"])); in != "" {
		m := durationRe.FindStringSubmatch(in)
		if m == nil {
			return time.Time{}, false
		}
		n, _ := strconv.ParseFloat(m[1], 64)
		unit := time.Second
		switch m[2] {
		case "m", "min", "分":
			unit = time.Minute
		case "h", "hour", "時間":
			unit = time.Hour
		}
		return now.Add(time.Duration(n * float64(unit))), n > 0
	}
	at := strings.TrimSpace(attrs["at"])
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006/01/02 15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, at, now.Location()); err == nil {
			return t, t.After(now)
		}
	}
	if t, err := time.ParseInLocation("15:04", at, now.Location()); err == nil {
		// 時刻だけなら、今日のその時刻 (過ぎていれば明日)
		t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		return t, true
	}
	return time.Time{}, false
}

func formatWhen(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	return t.Format("2006-01-02 15:04")
}

// AI の <remind>
func (l *Lumi) remindTag(r toolRequest) {
	at, ok := reminderTime(r.Attrs, time.Now())
	if !ok {
		l.write("  "+T("remind.badTime")+"\n", "red")
		return
	}
	reminders.add(at, r.Command)
	l.write("  "+T("remind.added", formatWhen(at), r.Command)+"\n", "cyan")
}

// 1 秒ごとに時間が来たリマインダーを知らせる
func (l *Lumi) runReminders() {
	lastMinute := ""
	for now := range time.Tick(time.Second) {
		for _, it := range reminders.due(now) {
			l.announce(T("remind.fire", it.Text))
		}
		// 毎分 1 回、時刻つきのルーティンを確かめる
		if m := now.Format("15:04"); m != lastMinute {
			lastMinute = m
			l.checkScheduledRoutines(now)
		}
	}
}

// 返事の途中でなければ (待ってから) 知らせる。隠れていれば右下から顔を出す
func (l *Lumi) announce(text string) {
	// 返事が終わるまで待つ (待つ間に別の返事が始まっても重ならないよう、begin で確かめる)
	for !l.begin() {
		time.Sleep(500 * time.Millisecond)
	}
	defer l.setBusy(false)
	popped := false
	if !l.win.IsVisible() {
		l.peek.pop(text)
		popped = true
	}
	l.emit("flash", map[string]any{"expr": "happy", "seconds": 3})
	l.write("\n", "fg")
	l.speak(text)
	l.write("\n\n", "fg")
	if popped {
		time.Sleep(2 * time.Second)
		l.peek.retract(false, "")
	}
}

// /reminders, /reminders cancel <番号>, /reminders clear
func (l *Lumi) remindersCommand(args []string) {
	switch {
	case len(args) >= 1 && args[0] == "clear":
		reminders.clear()
		l.info(T("remind.cleared"))
	case len(args) >= 2 && args[0] == "cancel":
		n, err := strconv.Atoi(args[1])
		if err != nil || !reminders.removeAt(n-1) {
			l.errorText(T("remind.noSuch", args[1]))
			return
		}
		l.info(T("remind.cancelled", n))
	default:
		items := reminders.list()
		if len(items) == 0 {
			l.info(T("remind.none"))
			return
		}
		var b strings.Builder
		for i, it := range items {
			fmt.Fprintf(&b, "  %2d. %s  %s\n", i+1, formatWhen(it.At), it.Text)
		}
		b.WriteString("\n  " + T("remind.howto"))
		l.info(b.String())
	}
}
