package main

// 監視モード: 指定したプロセス (アプリ) を見張って、何かあったら知らせる。
//   /watch <名前|PID>   … 見張る (名前は chrome・node などの実行ファイル名。.exe はなくてよい)
//   /watch              … 見張っているものの一覧
//   /watch stop <名前|PID|all> … やめる
// AI も <watch>名前</watch> で見張りを始められる (「Chrome が落ちたら教えて」など)。
// 知らせること: 終わった (落ちた・閉じた) / また起動した / CPU を 1 分くらい使い続けている / メモリが急に増えた。
// 名前で見張るものは watch.json に覚えておき、ルミを起動し直しても続ける (PID は変わるので覚えない)。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type procInfo struct {
	PID  int
	Name string  // 実行ファイル名 (Windows は .exe なし)
	Mem  uint64  // メモリ (調べたときだけ)
	CPU  float64 // これまでに使った CPU の時間 (秒、調べたときだけ)
}

const (
	procWatchEvery = 5 * time.Second
	procBusyPct    = 90 // CPU 1 コアぶんのうちこれだけ使い続けたら
	procBusyTicks  = 12 // 1 分ぶん
)

type watchItem struct {
	Name    string // 名前で見張るとき (小文字)
	PID     int    // PID で見張るとき
	label   string // 画面に出す名前
	running bool
	baseMem uint64 // 見張り始めた (起動した) ときのメモリ
	memTold bool
	cpuPrev float64
	cpuAt   time.Time
	busy    int
	cpuTold time.Time
}

var procWatch struct {
	sync.Mutex
	items  []*watchItem
	loaded bool
}

func watchFile() string { return filepath.Join(dataDir(), "watch.json") }

func (w *watchItem) matches(p procInfo) bool {
	if w.PID != 0 {
		return p.PID == w.PID
	}
	return strings.EqualFold(p.Name, w.Name)
}

// 名前で見張っているものを覚える
func saveWatches() {
	var names []string
	for _, w := range procWatch.items {
		if w.PID == 0 {
			names = append(names, w.Name)
		}
	}
	b, _ := json.MarshalIndent(names, "", "  ")
	os.WriteFile(watchFile(), b, 0o644)
}

func loadWatches() {
	if procWatch.loaded {
		return
	}
	procWatch.loaded = true
	var names []string
	if b, err := os.ReadFile(watchFile()); err == nil && json.Unmarshal(b, &names) == nil {
		for _, n := range names {
			procWatch.items = append(procWatch.items, &watchItem{Name: n, label: n})
		}
	}
}

// 見張りを足して、その結果の文を返す
func (l *Lumi) addWatch(target string) string {
	target = strings.TrimSuffix(strings.TrimSpace(target), ".exe")
	if target == "" {
		return T("watch.how")
	}
	w := &watchItem{Name: strings.ToLower(target), label: target}
	if pid, err := strconv.Atoi(target); err == nil && pid > 0 {
		w = &watchItem{PID: pid, label: "PID " + target}
	}
	procWatch.Lock()
	defer procWatch.Unlock()
	loadWatches()
	for _, x := range procWatch.items {
		if x.Name == w.Name && x.PID == w.PID {
			return T("watch.already", x.label)
		}
	}
	// 今動いているか (PID なら名前も分かる)
	count := 0
	for _, p := range listProcs(w.matches) {
		if w.matches(p) {
			count++
			w.baseMem += p.Mem
			if w.PID != 0 {
				w.label = fmt.Sprintf("%s (PID %d)", p.Name, p.PID)
			}
		}
	}
	if count == 0 && w.PID != 0 {
		return T("watch.noPID", w.PID)
	}
	w.running = count > 0
	procWatch.items = append(procWatch.items, w)
	saveWatches()
	if !w.running {
		return T("watch.addedNotRunning", w.label)
	}
	return T("watch.added", w.label, count)
}

// /watch [名前|PID] / /watch stop <名前|PID|all>
func (l *Lumi) watchCommand(arg string) {
	f := strings.Fields(arg)
	switch {
	case len(f) == 0:
		procWatch.Lock()
		loadWatches()
		var b strings.Builder
		for _, w := range procWatch.items {
			state := T("watch.stateStopped")
			if w.running {
				state = T("watch.stateRunning")
			}
			b.WriteString("  " + padRight(w.label, 24) + state + "\n")
		}
		procWatch.Unlock()
		if b.Len() == 0 {
			b.WriteString("  " + T("watch.none") + "\n")
		}
		l.info(b.String() + "\n  " + T("watch.how"))
	case strings.EqualFold(f[0], "stop") || strings.EqualFold(f[0], "off"):
		target := ""
		if len(f) > 1 {
			target = strings.ToLower(strings.TrimSuffix(f[1], ".exe"))
		}
		procWatch.Lock()
		loadWatches()
		kept := procWatch.items[:0]
		removed := 0
		for _, w := range procWatch.items {
			if target == "all" || target == w.Name || target == strconv.Itoa(w.PID) {
				removed++
				continue
			}
			kept = append(kept, w)
		}
		procWatch.items = kept
		saveWatches()
		procWatch.Unlock()
		l.info(T("watch.stopped", removed))
	default:
		l.info(l.addWatch(arg))
	}
}

// AI の <watch>: 見張りを始めて、結果を AI に返す
func (l *Lumi) watchTool(r toolRequest) string {
	msg := l.addWatch(r.Command)
	l.write(msg+"\n", "cyan")
	return "\n[" + T("watch.label") + "] " + r.Command + "\n" + msg + "\n"
}

// 5 秒ごとに見張っているプロセスを確かめる
func (l *Lumi) runProcWatch() {
	procWatch.Lock()
	loadWatches()
	procWatch.Unlock()
	for range time.Tick(procWatchEvery) {
		procWatch.Lock()
		items := append([]*watchItem(nil), procWatch.items...)
		procWatch.Unlock()
		if len(items) == 0 {
			continue
		}
		procs := listProcs(func(p procInfo) bool {
			for _, w := range items {
				if w.matches(p) {
					return true
				}
			}
			return false
		})
		procWatch.Lock() // 一覧の表示 (/watch) と同時に書き換えないように
		var gone []*watchItem
		for _, w := range items {
			count, mem, cpu := 0, uint64(0), 0.0
			for _, p := range procs {
				if w.matches(p) {
					count++
					mem += p.Mem
					cpu += p.CPU
				}
			}
			switch {
			case count == 0 && w.running:
				w.running, w.cpuAt, w.busy = false, time.Time{}, 0
				go l.reportPC(T("watch.exited", w.label), "sad")
				if w.PID != 0 {
					gone = append(gone, w) // PID は二度と戻らないので、見張りも終わり
				}
				continue
			case count == 0:
				continue
			case !w.running:
				w.running, w.baseMem, w.memTold = true, mem, false
				go l.reportPC(T("watch.started", w.label), "happy")
			}
			// CPU: 前回からの増え方 (1 コアぶんを 100%)
			now := time.Now()
			if !w.cpuAt.IsZero() && cpu >= w.cpuPrev {
				pct := (cpu - w.cpuPrev) / now.Sub(w.cpuAt).Seconds() * 100
				if pct >= procBusyPct {
					w.busy++
				} else {
					w.busy = 0
				}
				if w.busy >= procBusyTicks && time.Since(w.cpuTold) > 30*time.Minute {
					w.busy, w.cpuTold = 0, now
					go l.reportPC(T("watch.cpuBusy", w.label, pct), "sad")
				}
			}
			w.cpuPrev, w.cpuAt = cpu, now
			// メモリ: 見張り始めの 2 倍以上、かつ 1GB 以上増えたら (半分くらいまで戻ったら、また知らせられる)
			if w.baseMem == 0 {
				w.baseMem = mem
			}
			if !w.memTold && mem > w.baseMem*2 && mem-w.baseMem > gb {
				w.memTold = true
				go l.reportPC(T("watch.memUp", w.label, fmt.Sprintf("%.1fGB", float64(w.baseMem)/gb), fmt.Sprintf("%.1fGB", float64(mem)/gb)), "sad")
			} else if w.memTold && mem < w.baseMem*3/2 {
				w.memTold = false
			}
		}
		if len(gone) > 0 {
			kept := procWatch.items[:0]
			for _, w := range procWatch.items {
				drop := false
				for _, g := range gone {
					drop = drop || g == w
				}
				if !drop {
					kept = append(kept, w)
				}
			}
			procWatch.items = kept
		}
		procWatch.Unlock()
	}
}
