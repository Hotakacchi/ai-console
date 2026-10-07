package main

// コンソールとして使う: 打ったコマンドを AI を通さずにそのまま実行する。
//  - !コマンド       … 1 回だけ実行する (例: !ipconfig)
//  - /shell          … シェルモードの切り替え。打った行がそのままコマンドになる (exit で戻る)
// 出力は 1 行ずつその場で出し、Ctrl+C で止められる。cd で移った場所は次のコマンドにも引き継ぐ。
// 自分で打ったコマンドなので確認はしない (声で話しかけた内容はコマンドとしては実行しない)。

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

const (
	cwdMarker     = "__LUMI_CWD__"
	maxShellLines = 5000 // これより多い行は画面に出さない (止まらずに最後まで読む)
)

// 画面から打ち込まれた行 (と --script)。シェルモードや ! ならコマンドとして実行する
func (l *Lumi) submitTyped(text string) {
	t := strings.TrimSpace(text)
	if l.shellOn && !strings.HasPrefix(t, "/") {
		switch {
		case t == "":
		case t == "exit":
			l.toggleShell()
		default:
			l.startShell(t)
		}
		return
	}
	if cmd, ok := strings.CutPrefix(t, "!"); ok && strings.TrimSpace(cmd) != "" {
		l.startShell(strings.TrimSpace(cmd))
		return
	}
	l.submit(text)
}

// /shell
func (l *Lumi) toggleShell() {
	l.shellOn = !l.shellOn
	if l.shellOn {
		l.info(T("shell.on", shellName()))
	} else {
		l.info(T("shell.off"))
	}
	l.emit("prompt", l.promptText())
	l.emit("shellMode", l.shellOn)
}

func (l *Lumi) currentDir() string {
	if l.shellDir == "" {
		l.shellDir = homeDir()
	}
	return l.shellDir
}

// ふだんのプロンプト (その OS のコンソールらしく。PC やユーザーの本当の名前は出さない)
func basePrompt() string {
	switch runtime.GOOS {
	case "windows":
		return `C:\Lumi> `
	case "darwin":
		return "lumi@Mac ~ % "
	}
	return "lumi@linux:~$ "
}

// シェルモードのプロンプト ("" なら普段のプロンプト)
func (l *Lumi) promptText() string {
	if !l.shellOn {
		return ""
	}
	dir := l.currentDir()
	if runtime.GOOS == "windows" {
		return "PS " + dir + "> "
	}
	if home := homeDir(); dir == home || strings.HasPrefix(dir, home+"/") {
		dir = "~" + strings.TrimPrefix(dir, home)
	}
	return dir + " $ "
}

func (l *Lumi) startShell(command string) {
	if !l.begin() {
		return
	}
	go func() {
		lines := 0
		l.emit("running", map[string]any{"on": true, "cmd": command})
		defer l.emit("running", map[string]any{"on": false})
		dir, err := runShell(command, l.currentDir(), l.cancelChan(), func(line string) {
			if lines++; lines <= maxShellLines {
				l.write(line+"\n", "fg")
			} else if lines == maxShellLines+1 {
				l.write(T("shell.tooMany", maxShellLines)+"\n", "dim")
			}
		})
		switch {
		case l.cancelled():
			l.write("^C\n", "dim")
		case err != nil:
			l.errorText(T("shell.failed", err.Error()))
		}
		if dir != "" {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				l.shellDir = dir
			}
		}
		l.write("\n", "fg")
		l.emit("prompt", l.promptText())
		l.setBusy(false)
	}()
}

// command を dir で実行し、出力を 1 行ずつ渡す。終わったときの場所 (cd したあと) を返す
func runShell(command, dir string, cancel <-chan struct{}, onLine func(string)) (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// 出力は UTF-8 のバイトで書き出させる (コンソールの文字コードに左右されないように)。
		// *>&1 で Write-Host やエラーも含めて受け取る
		// 進み具合の表示は、出力を受け取る側では読めない形 (CLIXML) になるので出さない
		script := "$ErrorActionPreference = 'Continue'; $ProgressPreference = 'SilentlyContinue'\n" +
			"$__o = [Console]::OpenStandardOutput(); $__e = New-Object Text.UTF8Encoding $false\n" +
			"function __w($s) { $b = $__e.GetBytes([string]$s + \"`n\"); $__o.Write($b, 0, $b.Length); $__o.Flush() }\n" +
			"Set-Location -LiteralPath '" + strings.ReplaceAll(dir, "'", "''") + "' -ErrorAction SilentlyContinue\n" +
			"try { & {\n" + command + "\n} *>&1 | Out-String -Stream -Width 200 | ForEach-Object { __w $_ } } catch { __w $_ }\n" +
			"__w ('" + cwdMarker + "' + (Get-Location).Path)\n"
		cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	default:
		sh := "/bin/bash"
		if runtime.GOOS == "darwin" {
			sh = "/bin/zsh"
		}
		script := "cd '" + strings.ReplaceAll(dir, "'", `'\''`) + "' 2>/dev/null\n" + command + "\nprintf '\\n" + cwdMarker + "%s\\n' \"$PWD\"\n"
		cmd = exec.Command(sh, "-c", script)
	}
	cmd.Dir = dir
	hideWindow(cmd)
	newGroup(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		return "", err
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		select {
		case <-cancel:
			killTree(cmd)
		case <-ctx.Done():
		}
	}()
	go func() { pw.CloseWithError(cmd.Wait()) }()

	newDir := ""
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		// 表の右側の詰め物の空白は取る
		line := strings.TrimRight(decodeOutput(sc.Bytes()), "\r \t")
		if d, ok := strings.CutPrefix(line, cwdMarker); ok {
			newDir = filepath.Clean(strings.TrimSpace(d))
			continue
		}
		if strings.HasPrefix(line, "#< CLIXML") || strings.HasPrefix(line, "<Objs Version=") {
			continue // PowerShell の内部向けの出力
		}
		onLine(line)
	}
	err := sc.Err()
	if _, ok := err.(*exec.ExitError); ok {
		err = nil // 終了コードが 0 でないだけなら、出力を見ればわかる
	}
	return newDir, err
}

// UTF-8 でなければ Shift_JIS として読む (日本語の Windows の古いコマンドの出力など)
func decodeOutput(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	if s, err := japanese.ShiftJIS.NewDecoder().Bytes(b); err == nil {
		return string(s)
	}
	return string(b)
}
