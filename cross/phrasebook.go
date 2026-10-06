package main

// よく使うコマンドの単語帳。AI への指示に「やりたいこと → コマンド」の一覧を入れて、
// 小さなローカルAIでも正しいコマンドを書けるようにする。
//  - 標準の一覧は locales の pcbook.<OS>
//  - 自分で足す分はデータフォルダの commands.txt (/commands で開く)

import (
	"os"
	"path/filepath"
	"strings"
)

const maxUserCommands = 4000 // 指示文が長くなりすぎないように (文字数)

func userCommandsPath() string { return filepath.Join(dataDir(), "commands.txt") }

// commands.txt の中身 (# で始まる行と空行は除く)
func userCommands() string {
	data, err := os.ReadFile(userCommandsPath())
	if err != nil {
		return ""
	}
	return parseUserCommands(string(data))
}

func parseUserCommands(data string) string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r", ""), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	s := strings.Join(lines, "\n")
	if r := []rune(s); len(r) > maxUserCommands {
		s = string(r[:maxUserCommands])
	}
	return s
}

// /commands: commands.txt を開く (なければ書き方の例を入れて作る)
func (l *Lumi) commandsFile() {
	p := userCommandsPath()
	if _, err := os.Stat(p); err != nil {
		os.MkdirAll(dataDir(), 0o755)
		os.WriteFile(p, []byte(T("commands.template")), 0o644)
	}
	openFile(p)
	l.info(T("commands.opened", p))
}
