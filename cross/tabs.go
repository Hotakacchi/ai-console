package main

// タブ: ターミナルのように、シェルのタブを + で開いて × で閉じる。
// 最初のタブ (ルミ) は今までどおり AI と話す。シェルのタブは、それぞれ自分のシェル・場所 (cd)・実行中のコマンドを持ち、
// 打った行をそのままコマンドとして実行する (AI を通さず、確認もしない。exit で閉じる)。
//   画面 → Go: tabOpen {tab, shell} / tabSubmit {tab, text} / tabInterrupt {tab} / tabClose {tab}
//   Go → 画面: tabInfo {tab, title, prompt} / tabWrite {tab, text, color} / tabBusy {tab, on} / tabClear {tab} / tabClosed {tab}

import (
	"os"
	"strings"
	"sync"
)

type termTab struct {
	id      int
	sh      shellSpec
	dir     string
	cancel  chan struct{}
	running bool
}

var (
	termTabsMu sync.Mutex
	termTabs   = map[int]*termTab{}
)

func getTab(id int) *termTab {
	termTabsMu.Lock()
	defer termTabsMu.Unlock()
	return termTabs[id]
}

// + で開いたタブ。shell が空なら、今のシェル (設定 shell) で
func (l *Lumi) tabOpen(id int, kind string) {
	sh := l.shell()
	if kind != "" {
		if s, ok := pickShell(kind); ok {
			sh = s
		}
	}
	dir := l.currentDir()
	t := &termTab{id: id, sh: sh, dir: dir}
	termTabsMu.Lock()
	termTabs[id] = t
	termTabsMu.Unlock()
	l.tabWrite(t, T("tab.hello", sh.Label)+"\n\n", "dim")
	l.tabInfo(t)
}

func (l *Lumi) tabInfo(t *termTab) {
	l.emit("tabInfo", map[string]any{"tab": t.id, "title": t.sh.Label, "prompt": t.sh.prompt(t.dir)})
}

func (l *Lumi) tabWrite(t *termTab, text, color string) {
	l.emit("tabWrite", map[string]any{"tab": t.id, "text": text, "color": color})
}

func (l *Lumi) tabBusy(t *termTab, on bool) {
	l.emit("tabBusy", map[string]any{"tab": t.id, "on": on})
}

// タブで打たれた行を、そのタブのシェルで実行する
func (l *Lumi) tabSubmit(id int, text string) {
	t := getTab(id)
	if t == nil {
		return
	}
	termTabsMu.Lock()
	if t.running {
		termTabsMu.Unlock()
		return
	}
	text = strings.TrimSpace(text)
	switch strings.ToLower(text) {
	case "":
		termTabsMu.Unlock()
		l.tabBusy(t, false)
		return
	case "exit":
		termTabsMu.Unlock()
		l.emit("tabClosed", map[string]any{"tab": id})
		l.tabClose(id)
		return
	case "cls", "clear":
		termTabsMu.Unlock()
		l.emit("tabClear", map[string]any{"tab": id})
		l.tabBusy(t, false)
		return
	}
	t.running = true
	cancel := make(chan struct{})
	t.cancel = cancel
	termTabsMu.Unlock()

	l.tabBusy(t, true)
	l.emit("running", map[string]any{"on": true, "cmd": text})
	go func() {
		lines := 0
		dir, err := runShellWith(t.sh, text, t.dir, cancel, func(line string) {
			if lines++; lines <= maxShellLines {
				l.tabWrite(t, line+"\n", "fg")
			} else if lines == maxShellLines+1 {
				l.tabWrite(t, T("shell.tooMany", maxShellLines)+"\n", "dim")
			}
		})
		l.emit("running", map[string]any{"on": false})
		select {
		case <-cancel:
			l.tabWrite(t, "^C\n", "dim")
		default:
			if err != nil {
				l.tabWrite(t, T("shell.failed", err.Error())+"\n", "red")
			}
		}
		termTabsMu.Lock()
		if dir != "" {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				t.dir = dir
			}
		}
		t.running = false
		t.cancel = nil
		termTabsMu.Unlock()
		l.tabWrite(t, "\n", "fg")
		l.tabInfo(t)
		l.tabBusy(t, false)
	}()
}

// Ctrl+C: そのタブで動いているコマンドを止める
func (l *Lumi) tabInterrupt(id int) {
	termTabsMu.Lock()
	defer termTabsMu.Unlock()
	if t := termTabs[id]; t != nil && t.cancel != nil {
		select {
		case <-t.cancel:
		default:
			close(t.cancel)
		}
	}
}

// × で閉じた。動いているコマンドがあれば止める
func (l *Lumi) tabClose(id int) {
	l.tabInterrupt(id)
	termTabsMu.Lock()
	delete(termTabs, id)
	termTabsMu.Unlock()
}

// 画面の「▾」に出す、この PC のシェルの一覧
func (l *Lumi) sendShells() {
	var list []map[string]string
	for _, s := range availableShells() {
		list = append(list, map[string]string{"kind": s.Kind, "label": s.Label})
	}
	l.emit("shells", list)
}
