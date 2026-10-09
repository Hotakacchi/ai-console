package main

// 長期の記憶: 会話を archive.jsonl に残しておき (/cls では消えない。最大 2 万件)、
// 「前に話したあの店の名前なんだっけ？」と聞かれたら AI が <recall>店 名前</recall> で探す。
// 探し方は説明書 (lumidoc.go) と同じく、珍しい言葉ほど重く数えて関係するやりとりだけを渡す (重い仕組みは使わない)。
//   /recall <言葉> で自分でも探せる。/recall clear で全部消す。/set long_memory off で残さない。

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var nowFunc = time.Now // テストで日時を決めるため

const archiveKeep = 20000

var archiveMu sync.Mutex

func archivePath() string { return filepath.Join(dataDir(), "archive.jsonl") }

func (l *Lumi) longMemory() bool { return l.s.On("long_memory", "on") }

// 1 つ残す
func appendArchive(role, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 初めて残すときは、今までの会話の履歴 (history.jsonl、最大 500 件) を先に写しておく
	var seed []historyEntry
	if _, err := os.Stat(archivePath()); os.IsNotExist(err) {
		seed = readHistory(historyKeep)
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	os.MkdirAll(dataDir(), 0o755)
	f, err := os.OpenFile(archivePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		for _, e := range seed {
			line, _ := json.Marshal(e)
			f.Write(append(line, '\n'))
		}
	}
	if err != nil {
		return
	}
	line, _ := json.Marshal(historyEntry{Role: role, Text: text, At: nowFunc()})
	f.Write(append(line, '\n'))
	f.Close()
}

// 全部読む (多すぎたら古い分を捨てて書き直す)
func readArchive() []historyEntry {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	f, err := os.Open(archivePath())
	if err != nil {
		return nil
	}
	var all []historyEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var e historyEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Text != "" {
			all = append(all, e)
		}
	}
	f.Close()
	if len(all) > archiveKeep {
		all = all[len(all)-archiveKeep:]
		var b strings.Builder
		for _, e := range all {
			line, _ := json.Marshal(e)
			b.Write(append(line, '\n'))
		}
		os.WriteFile(archivePath(), []byte(b.String()), 0o644)
	}
	return all
}

// やりとり (話しかけた文と、そのあとのルミの返事) ごとにまとめる
func archiveItems(all []historyEntry) []string {
	var items []string
	for i := 0; i < len(all); i++ {
		e := all[i]
		if e.Role != "user" {
			continue
		}
		item := e.At.Format("2006-01-02") + " " + T("recall.you") + ": " + oneLine(e.Text, 300)
		if i+1 < len(all) && all[i+1].Role == "assistant" {
			item += " / " + T("recall.lumi") + ": " + oneLine(all[i+1].Text, 400)
		}
		items = append(items, item)
	}
	return items
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// 言葉に関係するやりとり (古い順、limit 文字まで)
func recall(query string, limit int) []string {
	return pickDoc(archiveItems(readArchive()), query, limit)
}

// AI の <recall>
func (l *Lumi) recallTool(r toolRequest) string {
	head := "\n[" + T("recall.label") + "] " + r.Command + "\n"
	if !l.longMemory() {
		return head + T("recall.off") + "\n"
	}
	limit := 5000
	if l.usingLocal() {
		limit = 2000
	}
	found := recall(r.Command, limit)
	l.write(T("recall.searching", len(found))+"\n", "cyan")
	if len(found) == 0 {
		return head + T("recall.none") + "\n"
	}
	return head + strings.Join(found, "\n") + "\n"
}

// /recall <言葉> | clear
func (l *Lumi) recallCommand(arg string) {
	arg = strings.TrimSpace(arg)
	switch {
	case arg == "":
		l.info(T("recall.how"))
	case strings.EqualFold(arg, "clear"):
		archiveMu.Lock()
		os.Remove(archivePath())
		archiveMu.Unlock()
		l.info(T("recall.cleared"))
	default:
		found := recall(arg, 3000)
		if len(found) == 0 {
			l.info(T("recall.none"))
			return
		}
		l.info(strings.Join(found, "\n"))
	}
}
