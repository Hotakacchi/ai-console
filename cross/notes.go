package main

// メモ: 「メモして: 牛乳を買う」で書き留める。/notes で一覧、/notes delete <番号>、/notes clear。
// AI は <note>内容</note> で書き留め、指示文に入っている一覧から「メモ見せて」に答える。
// データフォルダの notes.json に保存する。

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

type note struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

var notes struct {
	sync.Mutex
	list   []note
	loaded bool
}

func notesFile() string { return filepath.Join(dataDir(), "notes.json") }

func loadNotes() {
	if notes.loaded {
		return
	}
	notes.loaded = true
	if b, err := os.ReadFile(notesFile()); err == nil {
		json.Unmarshal(b, &notes.list)
	}
}

func saveNotes() {
	b, _ := json.MarshalIndent(notes.list, "", "  ")
	os.MkdirAll(filepath.Dir(notesFile()), 0o755)
	os.WriteFile(notesFile(), b, 0o644)
}

func addNote(text string) {
	notes.Lock()
	defer notes.Unlock()
	loadNotes()
	notes.list = append(notes.list, note{Text: strings.TrimSpace(text), At: time.Now()})
	saveNotes()
}

// 一覧 (番号つき)
func notesText() string {
	notes.Lock()
	defer notes.Unlock()
	loadNotes()
	var b strings.Builder
	for i, n := range notes.list {
		fmt.Fprintf(&b, "%d. %s  (%s)\n", i+1, n.Text, n.At.Format("1/2 15:04"))
	}
	return b.String()
}

// AI の <note>: 書き留める
func (l *Lumi) noteTag(r toolRequest) {
	if strings.TrimSpace(r.Command) == "" {
		return
	}
	addNote(r.Command)
	l.write("  📝 "+T("notes.added", strings.TrimSpace(r.Command))+"\n", "cyan")
}

// /notes [delete <番号> | clear]
func (l *Lumi) notesCommand(args []string) {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "clear":
			notes.Lock()
			loadNotes()
			n := len(notes.list)
			notes.list = nil
			saveNotes()
			notes.Unlock()
			l.info(T("notes.cleared", n))
			return
		case "delete", "del", "rm":
			if len(args) > 1 {
				i, err := strconv.Atoi(args[1])
				notes.Lock()
				loadNotes()
				ok := err == nil && i >= 1 && i <= len(notes.list)
				if ok {
					notes.list = append(notes.list[:i-1], notes.list[i:]...)
					saveNotes()
				}
				notes.Unlock()
				if ok {
					l.info(T("notes.deleted", i))
					return
				}
			}
			l.errorText(T("notes.badNumber"))
			return
		}
		// /notes 内容 … そのまま書き留める
		addNote(strings.Join(args, " "))
		l.info(T("notes.added", strings.Join(args, " ")))
		return
	}
	list := notesText()
	if list == "" {
		list = T("notes.none") + "\n"
	}
	l.info(strings.TrimRight(list, "\n") + "\n\n  " + T("notes.how"))
}

// AI に渡すメモの一覧 (新しい 20 件まで)
func notesPrompt() string {
	notes.Lock()
	defer notes.Unlock()
	loadNotes()
	list := notes.list
	if len(list) > 20 {
		list = list[len(list)-20:]
	}
	var b strings.Builder
	b.WriteString(T("prompt.notes"))
	if len(list) == 0 {
		b.WriteString("\n" + T("notes.none"))
	}
	for _, n := range list {
		b.WriteString("\n- " + n.Text + " (" + n.At.Format("2006-01-02") + ")")
	}
	return b.String()
}
