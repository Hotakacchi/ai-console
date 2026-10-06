package main

// AI の返事に書かれた道具の呼び出し (<run> <search> <fetch>) の取り出しと、コマンドの実行。
// コマンドは実行前に必ずユーザーに確認する (確認は lumi.go 側)。

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

type toolRequest struct {
	Kind    string // run (コマンド) / search (Web 検索) / fetch (ページを読む)
	Command string // コマンド・検索語・URL
	Admin   bool
}

var toolTags = []string{"run", "search", "fetch"}

// ストリーミング中の返事からタグを取り出し、残りの (読み上げる) テキストを返す
type toolExtractor struct {
	buf      strings.Builder
	inside   string
	admin    bool
	Requests []toolRequest
}

func (x *toolExtractor) Push(chunk string) string {
	x.buf.WriteString(chunk)
	var visible strings.Builder
	for {
		s := x.buf.String()
		if x.inside == "" {
			i, tag := -1, ""
			for _, t := range toolTags {
				if at := strings.Index(s, "<"+t); at >= 0 && (i < 0 || at < i) {
					i, tag = at, t
				}
			}
			if i < 0 {
				// "<sea" のようにタグの途中で切れているかもしれない末尾は持ち越す
				keep := 0
				if lt := strings.LastIndex(s, "<"); lt >= 0 && !strings.Contains(s[lt:], ">") {
					for _, t := range toolTags {
						if strings.HasPrefix("<"+t, s[lt:]) {
							keep = len(s) - lt
							break
						}
					}
				}
				visible.WriteString(s[:len(s)-keep])
				x.reset(s[len(s)-keep:])
				return visible.String()
			}
			end := strings.Index(s[i:], ">")
			visible.WriteString(s[:i])
			if end < 0 {
				x.reset(s[i:])
				return visible.String()
			}
			x.admin = strings.Contains(s[i:i+end], "admin")
			x.inside = tag
			x.reset(s[i+end+1:])
		} else {
			endTag := "</" + x.inside + ">"
			end := strings.Index(s, endTag)
			if end < 0 {
				return visible.String()
			}
			if body := strings.TrimSpace(s[:end]); body != "" {
				x.Requests = append(x.Requests, toolRequest{x.inside, body, x.admin && x.inside == "run"})
			}
			x.inside = ""
			x.reset(s[end+len(endTag):])
		}
	}
}

func (x *toolExtractor) reset(s string) { x.buf.Reset(); x.buf.WriteString(s) }

func (x *toolExtractor) Flush() string {
	s := ""
	if x.inside == "" {
		s = x.buf.String()
	}
	x.buf.Reset()
	return s
}

// ---------------- コマンドの実行 ----------------

const (
	commandTimeout = 2 * time.Minute
	maxOutput      = 4000
)

// OS のシェルでコマンドを実行し、出力を返す。admin なら OS の管理者確認を通す
func runCommand(command string, admin bool, cancel <-chan struct{}) string {
	ctx, stop := context.WithTimeout(context.Background(), commandTimeout)
	defer stop()
	go func() {
		select {
		case <-cancel:
			stop()
		case <-ctx.Done():
		}
	}()

	var cmd *exec.Cmd
	var outFile string
	switch runtime.GOOS {
	case "windows":
		cmd, outFile = windowsCommand(ctx, command, admin)
	case "darwin":
		if admin {
			// macOS の管理者パスワードの確認画面が出る
			script := `do shell script "` + appleScriptEscape(command) + ` 2>&1" with administrator privileges`
			cmd = exec.CommandContext(ctx, "osascript", "-e", script)
		} else {
			cmd = exec.CommandContext(ctx, "/bin/zsh", "-c", command)
		}
	default:
		if admin {
			if _, err := exec.LookPath("pkexec"); err != nil {
				return "(管理者として実行するための pkexec が見つかりません)"
			}
			cmd = exec.CommandContext(ctx, "pkexec", "/bin/bash", "-c", command) // 認証画面が出る
		} else {
			cmd = exec.CommandContext(ctx, "/bin/bash", "-c", command)
		}
	}
	hideWindow(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()

	out := buf.String()
	if outFile != "" {
		if b, e := os.ReadFile(outFile); e == nil {
			out = string(b)
		}
		os.Remove(outFile)
	}
	out = strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("(%d 秒たっても終わらなかったので止めました)", int(commandTimeout.Seconds()))
	}
	if ctx.Err() == context.Canceled {
		return "(中断しました)"
	}
	if strings.Contains(out, "LUMI_UAC_DENIED") {
		return "(管理者権限の確認画面で許可されませんでした)"
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		return "(実行できませんでした: " + err.Error() + ")"
	}
	if len([]rune(out)) > maxOutput {
		out = string([]rune(out)[:maxOutput]) + "\n…(以下省略)"
	}
	if out == "" {
		out = "(出力なし)"
	}
	return fmt.Sprintf("終了コード %d\n%s", code, out)
}

// PowerShell で実行する。出力は UTF-8 で一時ファイルに書かせる (管理者として動かした PowerShell の出力は直接受け取れないため)
func windowsCommand(ctx context.Context, command string, admin bool) (*exec.Cmd, string) {
	out := filepath.Join(os.TempDir(), fmt.Sprintf("lumi_run_%d.txt", time.Now().UnixNano()))
	inner := "$ErrorActionPreference = 'Continue'\n" +
		"$o = & { " + command + "\n} 2>&1 | Out-String -Width 200\n" +
		"[IO.File]::WriteAllText('" + strings.ReplaceAll(out, "'", "''") + "', $o, (New-Object Text.UTF8Encoding $false))\n"
	script := inner
	if admin {
		// UAC の確認画面を出して、許可されたら管理者の PowerShell で inner を動かす
		script = "try { $p = Start-Process powershell -Verb RunAs -Wait -PassThru -WindowStyle Hidden " +
			"-ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-EncodedCommand','" + encodePS(inner) + "'; exit $p.ExitCode } " +
			"catch { Write-Output 'LUMI_UAC_DENIED' }"
	}
	// 確認を断ったときの目印は標準出力に出る (そのときは出力ファイルができない)
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePS(script))
	return cmd, out
}

func encodePS(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[i*2], b[i*2+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func appleScriptEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
