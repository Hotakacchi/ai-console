package main

// 最後の返事をコピーする: /copy と、AI の <copy/> (「今のをコピーして」)。
// <copy>文</copy> なら、その文 (コマンドなど) をコピーする。

import (
	"os/exec"
	"runtime"
	"strings"
)

// クリップボードに文字を入れる (窓のルミは Wails で、ターミナル版は OS のコマンドで)
func (l *Lumi) setClipboard(text string) bool {
	if l.gui() {
		return l.app.Clipboard.SetText(text)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// clip.exe は UTF-16 を受け取るので PowerShell で
		cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "[Console]::InputEncoding=[Text.Encoding]::UTF8; Set-Clipboard -Value ([Console]::In.ReadToEnd())")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		cmd = exec.Command("sh", "-c", "wl-copy 2>/dev/null || xclip -selection clipboard 2>/dev/null || xsel -bi")
	}
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run() == nil
}

// 返事の文から、画面に出す飾り (HTML のタグなど) を除いたもの
func plainReply(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

// /copy: 最後の返事をコピーする
func (l *Lumi) copyCommand() {
	l.mu.Lock()
	text := l.lastReply
	l.mu.Unlock()
	if text == "" {
		l.errorText(T("copy.none"))
		return
	}
	if !l.setClipboard(text) {
		l.errorText(T("copy.failed"))
		return
	}
	l.info(T("copy.done", len([]rune(text))))
}

// AI の <copy/> (前の返事) / <copy>文</copy> (その文)
func (l *Lumi) copyTool(r toolRequest) string {
	text := strings.TrimSpace(r.Command)
	if text == "" {
		l.mu.Lock()
		text = l.lastReply
		l.mu.Unlock()
	}
	if text == "" {
		return T("copy.none") + "\n"
	}
	if !l.setClipboard(text) {
		return T("copy.failed") + "\n"
	}
	msg := T("copy.done", len([]rune(text)))
	l.write(msg+"\n", "cyan")
	return msg + "\n"
}
