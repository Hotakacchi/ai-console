package main

// 長期の記憶。AI が <remember>…</remember> で覚え、<forget>…</forget> で忘れる。
// データフォルダの memory.json に保存し、毎回 AI への指示に入れる。

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

const maxMemories = 100

type memoryItem struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

type memoryStore struct {
	mu    sync.Mutex
	items []memoryItem
}

var memories = &memoryStore{}

func memoryPath() string { return filepath.Join(dataDir(), "memory.json") }

func (m *memoryStore) load() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = nil
	if data, err := os.ReadFile(memoryPath()); err == nil {
		json.Unmarshal(data, &m.items)
	}
}

func (m *memoryStore) saveLocked() error {
	data, _ := json.MarshalIndent(m.items, "", "  ")
	os.MkdirAll(dataDir(), 0o755)
	return os.WriteFile(memoryPath(), data, 0o644)
}

// 同じことは 2 回覚えない。多すぎたら古いものから忘れる
func (m *memoryStore) add(text string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.items {
		if it.Text == text {
			return false
		}
	}
	m.items = append(m.items, memoryItem{text, time.Now()})
	if len(m.items) > maxMemories {
		m.items = m.items[len(m.items)-maxMemories:]
	}
	m.saveLocked()
	return true
}

// keyword を含む記憶を消し、消した数を返す
func (m *memoryStore) forget(keyword string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.items[:0]
	n := 0
	for _, it := range m.items {
		if strings.Contains(strings.ToLower(it.Text), strings.ToLower(keyword)) {
			n++
		} else {
			kept = append(kept, it)
		}
	}
	m.items = kept
	if n > 0 {
		m.saveLocked()
	}
	return n
}

func (m *memoryStore) removeAt(i int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.items) {
		return false
	}
	m.items = append(m.items[:i], m.items[i+1:]...)
	m.saveLocked()
	return true
}

func (m *memoryStore) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = nil
	m.saveLocked()
}

func (m *memoryStore) list() []memoryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]memoryItem(nil), m.items...)
}

// AI への指示に入れる「覚えていること」
func (m *memoryStore) prompt() string {
	items := m.list()
	if len(items) == 0 {
		return T("prompt.memoryEmpty")
	}
	var b strings.Builder
	b.WriteString(T("prompt.memoryList"))
	for _, it := range items {
		b.WriteString("\n- " + it.Text)
	}
	return b.String()
}

// AI の <remember> / <forget>
func (l *Lumi) memoryTag(r toolRequest) {
	if r.Kind == "remember" {
		if memories.add(r.Command) {
			l.fx("bulb", 1.8) // ピカッ
			l.write("  "+T("memory.added", r.Command)+"\n", "dim")
		}
		return
	}
	if n := memories.forget(r.Command); n > 0 {
		l.write("  "+T("memory.forgot", n)+"\n", "dim")
	}
}

// /memory, /memory delete <番号>, /memory clear
func (l *Lumi) memoryCommand(args []string) {
	switch {
	case len(args) >= 1 && args[0] == "clear":
		memories.clear()
		l.info(T("memory.cleared"))
	case len(args) >= 2 && args[0] == "delete":
		n, err := strconv.Atoi(args[1])
		if err != nil || !memories.removeAt(n-1) {
			l.errorText(T("memory.noSuch", args[1]))
			return
		}
		l.info(T("memory.deleted", n))
	default:
		items := memories.list()
		if len(items) == 0 {
			l.info(T("memory.none"))
			return
		}
		var b strings.Builder
		for i, it := range items {
			fmt.Fprintf(&b, "  %2d. %s\n", i+1, it.Text)
		}
		b.WriteString("\n  " + T("memory.howto"))
		l.info(b.String())
	}
}
