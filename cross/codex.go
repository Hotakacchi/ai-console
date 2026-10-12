package main

// Codex CLI (codex exec) を AI として使う (provider codex)。
// ChatGPT Plus / Pro でログインしていれば、API キーなしで (定額の範囲で) ChatGPT のモデルが答える。
// Codex は自分でコマンドを動かす仕組みを持っていて完全には外せないので、読み取り専用のサンドボックス・空の作業フォルダで動かし、
// ブラウザ・PC の操作・アプリ連携などの機能は切る。ルミの指示文で、PC の操作はルミのタグ (確認してから実行) で書かせる。
// 発言ごとに codex を起動し、それまでの会話を文章にして一緒に渡す (codex 側には会話を保存しない)。

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type codexProvider struct {
	s       *Settings
	history []Message
	mu      sync.Mutex
	cmd     *exec.Cmd
	aborted bool
}

func (c *codexProvider) Label() string {
	if m := claudeCodeModel(c.s); m != "" {
		return "codex: " + m
	}
	return "codex"
}
func (c *codexProvider) Clear()              { c.history = nil }
func (c *codexProvider) Seed(msgs []Message) { c.history = append([]Message(nil), msgs...) }

func (c *codexProvider) Abort() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aborted = true
	if c.cmd != nil && c.cmd.Process != nil {
		killTree(c.cmd)
	}
}

// codex の場所: PATH か、ふつうに入れたときの場所
func codexExe() string {
	if p, err := exec.LookPath("codex"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	var tries []string
	if runtime.GOOS == "windows" {
		tries = []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "OpenAI", "Codex", "bin", "codex.exe"), filepath.Join(os.Getenv("APPDATA"), "npm", "codex.cmd")}
	} else {
		tries = []string{filepath.Join(home, ".local", "bin", "codex"), "/opt/homebrew/bin/codex", "/usr/local/bin/codex"}
	}
	for _, p := range tries {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// Codex の機能のうち、ルミでは使わせないもの (知らない名前は Codex が無視する)
var codexOffFeatures = []string{"apps", "browser_use", "browser_use_external", "computer_use", "image_generation", "hooks", "goals", "in_app_browser", "in_app_local_automation"}

// codex exec に渡す引数 (発言は stdin から)
func codexArgs(workDir, promptFile, model string, images []string) []string {
	args := []string{"exec", "--json", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only",
		"--ignore-user-config", "--ignore-rules", "--color", "never", "-C", workDir,
		"-c", "model_instructions_file=" + tomlString(filepath.ToSlash(promptFile))}
	for _, f := range codexOffFeatures {
		args = append(args, "-c", "features."+f+"=false")
	}
	if model != "" {
		args = append(args, "-m", model)
	}
	for _, img := range images {
		args = append(args, "--image", img)
	}
	return append(args, "-")
}

// TOML の文字列 (設定の上書き -c key=value の値)
func tomlString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// codex の出力 (JSONL) を読む: できた返事の文を onText に渡し、全体の返事と、失敗したときの理由を返す
func readCodexStream(r io.Reader, onText func(string)) (reply string, failure string) {
	var b strings.Builder
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var ev struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "item.completed":
			if ev.Item.Type == "agent_message" && strings.TrimSpace(ev.Item.Text) != "" {
				piece := ev.Item.Text
				if b.Len() > 0 {
					piece = "\n\n" + piece
				}
				b.WriteString(piece)
				onText(piece)
			}
		case "turn.failed":
			if failure == "" {
				failure = ev.Error.Message
			}
		case "error":
			if failure == "" {
				failure = ev.Message
			}
		}
	}
	return b.String(), strings.TrimSpace(failure)
}

func (c *codexProvider) Reply(t Turn, onText func(string)) {
	exe := codexExe()
	if exe == "" {
		onText(T("codex.notFound"))
		return
	}
	tmp, err := os.MkdirTemp("", "lumi_codex_")
	if err != nil {
		onText(T("command.failed", err.Error()))
		return
	}
	defer os.RemoveAll(tmp)
	// 作業フォルダは空のフォルダ (読み取り専用のサンドボックスでも、PC のファイルを見て回らないように)
	work := filepath.Join(tmp, "work")
	os.MkdirAll(work, 0o755)
	promptFile := filepath.Join(tmp, "instructions.md")
	if err := os.WriteFile(promptFile, []byte(c.s.SystemPrompt()), 0o600); err != nil {
		onText(T("command.failed", err.Error()))
		return
	}
	var images []string
	for i, img := range t.Images {
		f := filepath.Join(tmp, fmt.Sprintf("image%d.jpg", i+1))
		if os.WriteFile(f, img, 0o600) == nil {
			images = append(images, f)
		}
	}
	c.history = trimHistory(c.history, claudeCodeBudget)

	cmd := exec.Command(exe, codexArgs(work, promptFile, claudeCodeModel(c.s), images)...)
	cmd.Dir = work
	cmd.Stdin = strings.NewReader(conversationText(c.history, t.Text))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, _ := cmd.StdoutPipe()
	hideWindow(cmd)
	newGroup(cmd)
	c.mu.Lock()
	c.aborted = false
	c.cmd = cmd
	c.mu.Unlock()
	if err := cmd.Start(); err != nil {
		onText(T("command.failed", err.Error()))
		return
	}
	reply, failure := readCodexStream(out, onText)
	waitErr := cmd.Wait()
	c.mu.Lock()
	aborted := c.aborted
	c.cmd = nil
	c.mu.Unlock()
	if aborted {
		return
	}
	if strings.TrimSpace(reply) == "" {
		msg := failure
		if msg == "" {
			msg = strings.TrimSpace(stderr.String())
			if i := strings.LastIndex(msg, "\n"); i >= 0 {
				msg = msg[i+1:]
			}
		}
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		onText(codexFailure(msg))
		return
	}
	text := t.Text
	if len(t.Images) > 0 {
		text += imagePlaceholder(len(t.Images))
	}
	c.history = append(c.history, Message{"role": "user", "content": text}, Message{"role": "assistant", "content": strings.TrimSpace(reply)})
}

// 失敗の理由を、どうすればよいかの説明にする
func codexFailure(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not logged in") || strings.Contains(lower, "login") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401"):
		return T("codex.login")
	case strings.Contains(lower, "limit"):
		return T("claudecode.limit", msg)
	}
	return T("codex.failed", msg)
}
