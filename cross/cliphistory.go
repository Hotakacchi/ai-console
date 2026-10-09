package main

// クリップボードの履歴: コピーした文字を最近 30 件まで覚えておく (ルミが動いている間だけ。ファイルには保存しない)。
//   /clips で一覧、/clips <番号> でそれをもう一度コピー。AI は <cliphistory/> で読む (clipboard の設定どおり確認する)。
// パスワードなどもコピーされることがあるので、ディスクには書かない。窓のルミのときだけ (ターミナル版は見ない)。

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const clipKeep = 30

type clipEntry struct {
	Text string
	At   time.Time
}

var clips struct {
	sync.Mutex
	list []clipEntry // 古い順
	last string
}

// 1 秒ごとにクリップボードを見て、変わっていたら覚える
func (l *Lumi) watchClipboard() {
	if !l.gui() {
		return
	}
	for range time.Tick(time.Second) {
		if l.s.Get("clipboard", "ask") == "off" {
			continue
		}
		text, ok := l.app.Clipboard.Text()
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		clips.Lock()
		if text != clips.last {
			clips.last = text
			// 同じものがあれば、新しい方に移す
			kept := clips.list[:0]
			for _, c := range clips.list {
				if c.Text != text {
					kept = append(kept, c)
				}
			}
			clips.list = append(kept, clipEntry{text, time.Now()})
			if len(clips.list) > clipKeep {
				clips.list = clips.list[len(clips.list)-clipKeep:]
			}
		}
		clips.Unlock()
	}
}

// 新しい順の一覧 (各行は短く)
func clipsText(width int) string {
	clips.Lock()
	defer clips.Unlock()
	var b strings.Builder
	for i := len(clips.list) - 1; i >= 0; i-- {
		c := clips.list[i]
		one := strings.Join(strings.Fields(c.Text), " ")
		if r := []rune(one); len(r) > width {
			one = string(r[:width]) + "…"
		}
		fmt.Fprintf(&b, "%d. %s  (%s)\n", len(clips.list)-i, one, c.At.Format("15:04"))
	}
	return b.String()
}

// /clips [番号]
func (l *Lumi) clipsCommand(args []string) {
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		clips.Lock()
		ok := err == nil && n >= 1 && n <= len(clips.list)
		text := ""
		if ok {
			text = clips.list[len(clips.list)-n].Text
		}
		clips.Unlock()
		if !ok || !l.gui() {
			l.errorText(T("clips.badNumber"))
			return
		}
		l.app.Clipboard.SetText(text)
		l.info(T("clips.copied", n))
		return
	}
	list := clipsText(70)
	if list == "" {
		list = T("clips.none") + "\n"
	}
	l.info(strings.TrimRight(list, "\n") + "\n\n  " + T("clips.how"))
}

// AI の <cliphistory/>: 確認してから (clipboard が ask のとき)、最近コピーしたものを渡す
func (l *Lumi) clipHistoryTool() string {
	head := "\n[" + T("clips.label") + "]\n"
	mode := strings.ToLower(l.s.Get("clipboard", "ask"))
	if mode == "off" {
		return head + T("clip.off") + "\n"
	}
	if mode == "ask" {
		if a := strings.ToLower(l.ask(T("clips.ask"))); a != "y" && a != "a" {
			l.write(T("web.stopped")+"\n", "dim")
			return head + T("web.denied") + "\n"
		}
	}
	list := clipsText(300)
	if list == "" {
		return head + T("clips.none") + "\n"
	}
	l.write(T("clips.read")+"\n\n", "cyan")
	l.taint() // コピーした文字は、外から来たものかもしれない
	return head + list
}
