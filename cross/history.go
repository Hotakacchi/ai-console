package main

// 会話の履歴。話しかけた文とルミの返事をデータフォルダの history.jsonl に残し、
// 次に起動したときは続きから話せるよう AI に渡す (keep_history が on のとき)。

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	historySeed = 20  // 起動時に AI に渡す発言の数
	historyKeep = 500 // ファイルに残す発言の数
)

type historyEntry struct {
	Role string    `json:"role"` // user / assistant
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

var historyMu sync.Mutex

func historyPath() string { return filepath.Join(dataDir(), "history.jsonl") }

func appendHistory(role, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	historyMu.Lock()
	defer historyMu.Unlock()
	os.MkdirAll(dataDir(), 0o755)
	f, err := os.OpenFile(historyPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	line, _ := json.Marshal(historyEntry{role, text, time.Now()})
	f.Write(append(line, '\n'))
	f.Close()
}

// 新しいほうから n 個 (古い順に並べて返す)
func readHistory(n int) []historyEntry {
	historyMu.Lock()
	defer historyMu.Unlock()
	f, err := os.Open(historyPath())
	if err != nil {
		return nil
	}
	defer f.Close()
	var all []historyEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var e historyEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Text != "" {
			all = append(all, e)
		}
	}
	// 大きくなりすぎたら古い分を捨てる
	if len(all) > historyKeep {
		all = all[len(all)-historyKeep:]
		var b strings.Builder
		for _, e := range all {
			line, _ := json.Marshal(e)
			b.Write(append(line, '\n'))
		}
		os.WriteFile(historyPath(), []byte(b.String()), 0o644)
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

func clearHistory() {
	historyMu.Lock()
	defer historyMu.Unlock()
	os.Remove(historyPath())
}

// AI に渡す形 (user から始まり、user / assistant が交互)
func historyMessages(entries []historyEntry) []Message {
	var msgs []Message
	for _, e := range entries {
		if len(msgs) == 0 && e.Role != "user" {
			continue
		}
		if len(msgs) > 0 && msgs[len(msgs)-1]["role"] == e.Role {
			msgs[len(msgs)-1]["content"] = msgs[len(msgs)-1]["content"].(string) + "\n" + e.Text
			continue
		}
		msgs = append(msgs, Message{"role": e.Role, "content": e.Text})
	}
	// 最後が user だと、次の発言と続いてしまうので落とす
	if len(msgs) > 0 && msgs[len(msgs)-1]["role"] == "user" {
		msgs = msgs[:len(msgs)-1]
	}
	return msgs
}

// 起動時や AI を切り替えたときに、前の会話を AI に渡す
func (l *Lumi) restoreHistory(show bool) {
	if !l.s.On("keep_history", "on") {
		return
	}
	entries := readHistory(historySeed)
	msgs := historyMessages(entries)
	if len(msgs) == 0 {
		return
	}
	l.ai.Seed(msgs)
	if show {
		l.write(T("history.restored")+"\n", "dim")
		start := len(entries) - 4
		if start < 0 {
			start = 0
		}
		for _, e := range entries[start:] {
			prefix := "  > "
			if e.Role == "assistant" {
				prefix = "    "
			}
			l.write(prefix+firstLine(e.Text, 80)+"\n", "dim")
		}
		l.write("\n", "dim")
	}
}

// /history [件数]
func (l *Lumi) historyCommand(args []string) {
	n := 20
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	entries := readHistory(n)
	if len(entries) == 0 {
		l.info(T("history.none"))
		return
	}
	var b strings.Builder
	for _, e := range entries {
		who := "  > "
		if e.Role == "assistant" {
			who = "    "
		}
		b.WriteString(who + e.At.Format("01-02 15:04") + "  " + firstLine(e.Text, 90) + "\n")
	}
	b.WriteString("\n  " + T("history.howto"))
	l.info(b.String())
}

func firstLine(s string, max int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
