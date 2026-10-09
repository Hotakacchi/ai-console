package main

// 自動化: ルーティン (routines.txt) の [名前 @きっかけ] で、時刻以外のきっかけでも動かす。
//   @USB (@ドライブ)        ドライブをつないだとき       {drive} にドライブ (E: など)
//   @充電                    充電を始めたとき
//   @起動                    ルミが起動したとき
//   @終了:chrome             そのアプリが終わったとき     {app}
//   @開始:chrome             そのアプリが起動したとき     {app}
//   @フォルダ:C:\…\Downloads そのフォルダに新しいファイル {file} にそのファイル
// アプリは 5 秒ごと、フォルダは 10 秒ごとに見る (きっかけを書いたものだけ)。
// AI は <automation>[名前 @きっかけ]\n手順…</automation> で自動化を足せる (足す前に必ず内容を見せて確認する)。

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// @ の後ろを、きっかけの種類にする ("" は分からない)
func parseTrigger(s string) string {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	switch low {
	case "usb", "ドライブ", "drive":
		return "drive"
	case "充電", "charge", "charging":
		return "charge"
	case "起動", "startup", "start":
		return "startup"
	}
	for kind, heads := range map[string][]string{
		"exit":   {"終了", "exit", "quit"},
		"start":  {"開始", "start", "launch"},
		"folder": {"フォルダ", "folder"},
	} {
		for _, h := range heads {
			for _, sep := range []string{":", "："} {
				if rest, ok := strings.CutPrefix(s, h+sep); ok && strings.TrimSpace(rest) != "" {
					rest = strings.TrimSpace(rest)
					if kind != "folder" {
						rest = strings.ToLower(strings.TrimSuffix(rest, ".exe"))
					}
					return kind + ":" + rest
				}
			}
		}
	}
	return ""
}

// きっかけが起きた: それをきっかけにしているルーティンを動かす
func (l *Lumi) fireTrigger(event string, vars map[string]string) {
	for _, r := range loadRoutines() {
		if r.When == "" || !strings.EqualFold(r.When, event) {
			continue
		}
		go func(r routine) {
			for !l.begin() { // 返事の途中なら終わるまで待つ
				time.Sleep(500 * time.Millisecond)
			}
			if l.gui() && !l.win.IsVisible() {
				l.peek.pop(r.Name)
				defer func() {
					time.Sleep(2 * time.Second)
					l.peek.retract(false, "")
				}()
			}
			l.write("  "+T("automation.trigger", r.Name)+"\n", "cyan")
			l.runRoutineWith(r, vars)
		}(r)
	}
}

// アプリ (@終了・@開始) とフォルダ (@フォルダ) のきっかけを見張る。きっかけを書いたものだけ
func (l *Lumi) watchTriggers() {
	running := map[string]bool{}       // アプリの名前 → 動いているか
	known := map[string]map[string]bool{} // フォルダ → 前に見たファイル
	tick := 0
	for range time.Tick(5 * time.Second) {
		tick++
		apps, folders := map[string]bool{}, map[string]bool{}
		for _, r := range loadRoutines() {
			kind, arg, _ := strings.Cut(r.When, ":")
			switch kind {
			case "exit", "start":
				apps[arg] = true
			case "folder":
				folders[arg] = true
			}
		}
		if len(apps) > 0 {
			now := map[string]bool{}
			for _, p := range listProcs(func(procInfo) bool { return false }) {
				if n := strings.ToLower(p.Name); apps[n] {
					now[n] = true
				}
			}
			for name := range apps {
				was, seen := running[name]
				switch {
				case seen && was && !now[name]:
					l.fireTrigger("exit:"+name, map[string]string{"app": name})
				case seen && !was && now[name]:
					l.fireTrigger("start:"+name, map[string]string{"app": name})
				}
				running[name] = now[name]
			}
		}
		if tick%2 == 0 { // フォルダは 10 秒ごと
			for dir := range folders {
				entries, err := os.ReadDir(dir)
				if err != nil {
					continue
				}
				cur := map[string]bool{}
				for _, e := range entries {
					if !e.IsDir() && !partialDownload(e.Name()) {
						cur[e.Name()] = true
					}
				}
				if prev, ok := known[dir]; ok { // 最初の 1 回は、今あるものを覚えるだけ
					for name := range cur {
						if !prev[name] {
							l.fireTrigger("folder:"+dir, map[string]string{"file": filepath.Join(dir, name)})
						}
					}
				}
				known[dir] = cur
			}
		}
	}
}

// ダウンロード途中のファイル (終わってから動かす)
func partialDownload(name string) bool {
	low := strings.ToLower(name)
	for _, ext := range []string{".crdownload", ".part", ".tmp", ".download", ".partial"} {
		if strings.HasSuffix(low, ext) {
			return true
		}
	}
	return strings.HasPrefix(low, "~$") || strings.HasPrefix(low, ".")
}

// AI の <automation>: 内容を見せて確認してから routines.txt に足す
func (l *Lumi) automationTool(r toolRequest) string {
	head := "\n[" + T("automation.label") + "]\n"
	block := strings.TrimSpace(r.Command)
	list := parseRoutines(block)
	if len(list) != 1 || (list[0].When == "" && list[0].At == "") || len(list[0].Steps) == 0 {
		return head + T("automation.bad") + "\n"
	}
	l.write("\n"+T("automation.addTitle")+"\n", "yellow")
	for _, line := range strings.Split(block, "\n") {
		l.write("  "+strings.TrimRight(line, "\r")+"\n", "white")
	}
	if a := strings.ToLower(l.ask(T("automation.addAsk"))); a != "y" && a != "a" {
		l.write(T("tool.notRun")+"\n", "dim")
		return head + T("tool.declined") + "\n"
	}
	p := routinesPath()
	data, err := os.ReadFile(p)
	if err != nil {
		data = []byte(T("routines.template"))
	}
	text := strings.TrimRight(string(data), "\r\n") + "\n\n" + block + "\n"
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return head + err.Error() + "\n"
	}
	l.write(T("automation.added", list[0].Name)+"\n\n", "cyan")
	return head + T("automation.added", list[0].Name) + "\n"
}
