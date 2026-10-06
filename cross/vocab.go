package main

// 音声の単語帳: Whisper に「こういう言葉が出てくる」と見せて、名前や専門用語を正しく書き起こしやすくする。
//  - 呼びかけの言葉
//  - データフォルダの words.txt (/words で開く。1 行に 1 語)
//  - commands.txt に書いた「やりたいこと」
//  - 覚えていること (/memory。名前など)
// Vosk には単語を足せないので、Whisper (/install-whisper) を使っているときだけ効く。

import (
	"os"
	"path/filepath"
	"strings"
)

const maxVocab = 600 // Whisper に見せる長さ (文字数)。長すぎると逆に間違えやすくなる

func wordsPath() string { return filepath.Join(dataDir(), "words.txt") }

func (l *Lumi) voiceVocab() string {
	var parts []string
	seen := map[string]bool{}
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" && !seen[s] {
			seen[s] = true
			parts = append(parts, s)
		}
	}
	add(l.s.WakeWord())
	if data, err := os.ReadFile(wordsPath()); err == nil {
		for _, line := range strings.Split(parseUserCommands(string(data)), "\n") {
			add(line)
		}
	}
	for _, line := range strings.Split(userCommands(), "\n") {
		if task, _, ok := strings.Cut(line, "→"); ok {
			add(task)
		}
	}
	for _, m := range memories.list() {
		add(m.Text)
	}
	v := strings.Join(parts, "、")
	if r := []rune(v); len(r) > maxVocab {
		v = string(r[:maxVocab])
	}
	return v
}

// /words: words.txt を開く (なければ書き方の例を入れて作る)
func (l *Lumi) wordsFile() {
	p := wordsPath()
	if _, err := os.Stat(p); err != nil {
		os.MkdirAll(dataDir(), 0o755)
		os.WriteFile(p, []byte(T("words.template")), 0o644)
	}
	openFile(p)
	msg := T("words.opened", p)
	if l.s.Get("stt", "vosk") != "whisper" {
		msg += "\n" + T("words.needWhisper")
	}
	l.info(msg)
}
