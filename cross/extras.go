package main

// 小さな機能: 新しいバージョンのお知らせ、呼び出し用のショートカットキー、クリップボード、見た目の設定。

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)


// ---- 新しいバージョンのお知らせ ----

// a が b より新しいか ("1.2.0" と "1.10.0" も数として比べる。-beta などは同じ番号なら古いとみなす)
func newerVersion(a, b string) bool {
	parse := func(v string) ([]int, bool) {
		pre := strings.Contains(v, "-")
		v = strings.SplitN(v, "-", 2)[0]
		var nums []int
		for _, p := range strings.Split(v, ".") {
			n, _ := strconv.Atoi(p)
			nums = append(nums, n)
		}
		for len(nums) < 3 {
			nums = append(nums, 0)
		}
		return nums, pre
	}
	na, preA := parse(a)
	nb, preB := parse(b)
	for i := 0; i < 3; i++ {
		if na[i] != nb[i] {
			return na[i] > nb[i]
		}
	}
	return preB && !preA
}

// URL を OS の既定のブラウザで開く
func openURL(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

// ---- 呼び出し用のショートカットキー ----

// hotkey の設定 (例: CmdOrCtrl+Alt+L) を登録し直す。押すとウィンドウを出す / 隠す
func (l *Lumi) applyHotkey() {
	gs := l.app.GlobalShortcut
	gs.UnregisterAll()
	key := l.s.Get("hotkey", "CmdOrCtrl+Alt+L")
	if strings.EqualFold(key, "off") {
		return
	}
	if err := gs.Register(key, l.toggleWindow); err != nil {
		l.errorText(T("hotkey.failed", key, err.Error()))
	}
}

func (l *Lumi) toggleWindow() {
	if l.win.IsVisible() && l.win.IsFocused() {
		l.win.Hide()
	} else {
		l.show()
	}
}

// ---- クリップボード ----

// AI の <clipboard/>: コピーされている文字を読んで返す (clipboard が ask なら毎回確認)
func (l *Lumi) clipboardTool() string {
	head := "\n[" + T("clip.label") + "]\n"
	mode := strings.ToLower(l.s.Get("clipboard", "ask"))
	if mode == "off" {
		return head + T("clip.off") + "\n"
	}
	if mode == "ask" {
		if a := strings.ToLower(l.ask(T("clip.ask"))); a != "y" && a != "a" {
			l.write(T("web.stopped")+"\n", "dim")
			return head + T("web.denied") + "\n"
		}
	}
	text, ok := l.app.Clipboard.Text()
	if !ok || strings.TrimSpace(text) == "" {
		l.write(T("clip.empty")+"\n", "dim")
		return head + T("clip.empty") + "\n"
	}
	if r := []rune(text); len(r) > 4000 {
		text = string(r[:4000]) + "\n" + T("run.truncated")
	}
	l.write(T("clip.read", len([]rune(text)))+"\n\n", "cyan")
	return head + text + "\n"
}

// ---- 見た目 ----

// 顔の色・大きさ、文字の大きさ・フォントを画面に伝える
func (l *Lumi) sendAppearance() {
	l.emit("appearance", map[string]any{
		"faceColor": l.s.Get("face_color", "cyan"),
		"faceSize":  l.s.GetInt("face_size", 60),
		"fontSize":  l.s.GetInt("font_size", 15),
		"font":      l.s.Get("font", ""),
	})
}
