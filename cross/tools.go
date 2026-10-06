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
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

type toolRequest struct {
	Kind    string            // run / search / fetch / face / remind / remember / forget / clipboard / screen
	Command string            // 中身 (コマンド・検索語・URL・表情・覚えること など)
	Admin   bool              // <run admin>
	Attrs   map[string]string // <remind in="300"> などの属性
}

var toolTags = []string{"run", "search", "fetch", "face", "remind", "remember", "forget", "clipboard", "screen"}

// 中身が空でも意味のあるタグ
var emptyOK = map[string]bool{"clipboard": true, "screen": true}

var attrRe = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)

// ストリーミング中の返事からタグを取り出す。
// OnText には読み上げる文字が、OnTag には取り出したタグが、返事に出てきた順に渡る
type toolExtractor struct {
	buf     strings.Builder
	inside  string
	openTag string
	OnText  func(string)
	OnTag   func(toolRequest)
}

func (x *toolExtractor) text(s string) {
	if s != "" && x.OnText != nil {
		x.OnText(s)
	}
}

func (x *toolExtractor) Push(chunk string) {
	x.buf.WriteString(chunk)
	for {
		s := x.buf.String()
		if x.inside == "" {
			i, tag := -1, ""
			for _, t := range toolTags {
				// "<run" が "<runtime" などに当たらないよう、タグ名の直後も確かめる
				for from := 0; ; {
					at := strings.Index(s[from:], "<"+t)
					if at < 0 {
						break
					}
					at += from
					next := at + 1 + len(t)
					if next >= len(s) || strings.ContainsRune(" >/\t\n", rune(s[next])) {
						if i < 0 || at < i {
							i, tag = at, t
						}
						break
					}
					from = at + 1
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
				x.text(s[:len(s)-keep])
				x.reset(s[len(s)-keep:])
				return
			}
			end := strings.Index(s[i:], ">")
			x.text(s[:i])
			if end < 0 {
				x.reset(s[i:])
				return
			}
			x.openTag = s[i : i+end]
			x.reset(s[i+end+1:])
			if strings.HasSuffix(x.openTag, "/") { // <clipboard/> のような閉じタグなしの形
				x.tag(tag, "")
				continue
			}
			x.inside = tag
		} else {
			endTag := "</" + x.inside + ">"
			end := strings.Index(s, endTag)
			if end < 0 {
				return
			}
			x.tag(x.inside, strings.TrimSpace(s[:end]))
			x.inside = ""
			x.reset(s[end+len(endTag):])
		}
	}
}

func (x *toolExtractor) tag(kind, body string) {
	if (body == "" && !emptyOK[kind]) || x.OnTag == nil {
		return
	}
	attrs := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(x.openTag, -1) {
		attrs[strings.ToLower(m[1])] = m[2]
	}
	head := strings.TrimSuffix(strings.TrimPrefix(x.openTag, "<"+kind), "/")
	admin := kind == "run" && strings.Contains(attrRe.ReplaceAllString(head, ""), "admin")
	x.OnTag(toolRequest{Kind: kind, Command: body, Admin: admin, Attrs: attrs})
}

func (x *toolExtractor) reset(s string) { x.buf.Reset(); x.buf.WriteString(s) }

func (x *toolExtractor) Flush() {
	if x.inside == "" {
		x.text(x.buf.String())
	}
	x.buf.Reset()
}

// ---------------- コマンドの実行 ----------------

const (
	commandTimeout = 2 * time.Minute
	maxOutput      = 20000 // AI に返す出力の上限 (ローカルAIはさらに短くする: localOutputLimit)
)

// OS のシェルでコマンドを実行し、出力と成功したかを返す。admin なら OS の管理者確認を通す。
// 管理者として動き始めたら (確認で許可されたら) onElevated を 1 回呼ぶ
func runCommand(command string, admin bool, cancel <-chan struct{}, onElevated func()) (string, bool) {
	ctx, stop := context.WithTimeout(context.Background(), commandTimeout)
	defer stop()
	go func() {
		select {
		case <-cancel:
			stop()
		case <-ctx.Done():
		}
	}()

	// 管理者として動く側が最初にこのファイルを作る (許可された合図)
	// (自分で作ったフォルダの中に置くので、管理者が作ったファイルでも後で消せる)
	marker := ""
	if admin {
		if dir, err := os.MkdirTemp("", "lumi_admin_"); err == nil {
			marker = filepath.Join(dir, "elevated")
			defer os.RemoveAll(dir)
		}
	}
	touch := ""
	if marker != "" {
		touch = "touch '" + strings.ReplaceAll(marker, "'", `'\''`) + "'; "
	}
	// どこから起動されても、コマンドはホームフォルダで動かす (管理者として動くシェルは別の場所から始まるので cd も付ける)
	home := homeDir()
	cdHome := "cd '" + strings.ReplaceAll(home, "'", `'\''`) + "' 2>/dev/null; "

	var cmd *exec.Cmd
	var outFile string
	switch runtime.GOOS {
	case "windows":
		cmd, outFile = windowsCommand(ctx, command, admin, marker, home)
	case "darwin":
		if admin {
			// macOS の管理者パスワードの確認画面が出る
			script := `do shell script "` + appleScriptEscape(touch+cdHome+command) + ` 2>&1" with administrator privileges`
			cmd = exec.CommandContext(ctx, "osascript", "-e", script)
		} else {
			cmd = exec.CommandContext(ctx, "/bin/zsh", "-c", command)
		}
	default:
		if admin {
			if _, err := exec.LookPath("pkexec"); err != nil {
				return T("run.noPkexec"), false
			}
			cmd = exec.CommandContext(ctx, "pkexec", "/bin/bash", "-c", touch+cdHome+command) // 認証画面が出る
		} else {
			cmd = exec.CommandContext(ctx, "/bin/bash", "-c", command)
		}
	}
	cmd.Dir = home
	hideWindow(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	done := make(chan struct{})
	if marker != "" && onElevated != nil {
		go func() {
			for {
				select {
				case <-done:
					return
				case <-time.After(150 * time.Millisecond):
					if _, err := os.Stat(marker); err == nil {
						onElevated()
						return
					}
				}
			}
		}()
	}
	err := cmd.Run()
	close(done)

	out := buf.String()
	if outFile != "" {
		if b, e := os.ReadFile(outFile); e == nil {
			out = string(b)
		}
		os.Remove(outFile)
	}
	out = strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
	if ctx.Err() == context.DeadlineExceeded {
		return T("run.timeout", int(commandTimeout.Seconds())), false
	}
	if ctx.Err() == context.Canceled {
		return T("run.cancelled"), false
	}
	if strings.Contains(out, "LUMI_UAC_DENIED") {
		return T("run.uacDenied"), false
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		return T("run.failed", err.Error()), false
	}
	if len([]rune(out)) > maxOutput {
		out = string([]rune(out)[:maxOutput]) + "\n" + T("run.truncated")
	}
	if out == "" {
		out = T("run.noOutput")
	}
	return T("run.exitCode", code) + "\n" + out, code == 0
}

// PowerShell で実行する。出力は UTF-8 で一時ファイルに書かせる (管理者として動かした PowerShell の出力は直接受け取れないため)
func windowsCommand(ctx context.Context, command string, admin bool, marker, home string) (*exec.Cmd, string) {
	out := filepath.Join(os.TempDir(), fmt.Sprintf("lumi_run_%d.txt", time.Now().UnixNano()))
	inner := ""
	if marker != "" {
		inner = "New-Item -ItemType File -Force -Path '" + strings.ReplaceAll(marker, "'", "''") + "' | Out-Null\n"
	}
	inner += "Set-Location -LiteralPath '" + strings.ReplaceAll(home, "'", "''") + "' -ErrorAction SilentlyContinue\n"
	inner += "$ErrorActionPreference = 'Continue'\n" +
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

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.TempDir()
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
