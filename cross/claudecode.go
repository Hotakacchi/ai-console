package main

// Claude Code (claude -p) を AI として使う (provider claude-code)。
// Claude Pro / Max の契約があれば、API キーなしで (定額の範囲で) Claude が答える。
// Claude Code 自身の道具 (ファイルの編集・コマンドの実行など)・MCP・スラッシュコマンド・設定ファイルは使わせず、
// 返事を作るだけにする。PC の操作は、いつもどおりルミのタグ (確認してから実行) で行う。
// 発言ごとに claude を起動し、それまでの会話を文章にして一緒に渡す (claude 側には会話を保存しない)。

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type claudeCodeProvider struct {
	s       *Settings
	history []Message
	mu      sync.Mutex
	cmd     *exec.Cmd
	aborted bool
}

// 渡す会話の長さの上限 (文字数のめやす)
const claudeCodeBudget = 60000

// claude に渡すモデル名 (ローカルAIの .gguf の名前が残っていたら使わない)
func claudeCodeModel(s *Settings) string {
	m := strings.TrimSpace(s.Get("model", ""))
	if strings.HasSuffix(strings.ToLower(m), ".gguf") {
		return ""
	}
	return m
}

func (c *claudeCodeProvider) Label() string {
	if m := claudeCodeModel(c.s); m != "" {
		return "claude-code: " + m
	}
	return "claude-code"
}
func (c *claudeCodeProvider) Clear()              { c.history = nil }
func (c *claudeCodeProvider) Seed(msgs []Message) { c.history = append([]Message(nil), msgs...) }

func (c *claudeCodeProvider) Abort() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aborted = true
	if c.cmd != nil && c.cmd.Process != nil {
		killTree(c.cmd)
	}
}

// claude の場所: PATH か、ふつうに入れたときの場所
func claudeCodeExe() string {
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	var tries []string
	if runtime.GOOS == "windows" {
		tries = []string{filepath.Join(home, ".local", "bin", "claude.exe"), filepath.Join(os.Getenv("APPDATA"), "npm", "claude.cmd")}
	} else {
		tries = []string{filepath.Join(home, ".local", "bin", "claude"), filepath.Join(home, ".claude", "local", "claude"), "/opt/homebrew/bin/claude", "/usr/local/bin/claude"}
	}
	for _, p := range tries {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// claude に渡す引数 (道具・MCP・スラッシュコマンド・設定ファイル・会話の保存はなし)
func claudeCodeArgs(promptFile, model string) []string {
	args := []string{"-p",
		"--input-format", "stream-json", "--output-format", "stream-json", "--include-partial-messages", "--verbose",
		"--tools", "", "--strict-mcp-config", "--disable-slash-commands", "--setting-sources", "",
		"--no-session-persistence", "--system-prompt-file", promptFile}
	if model != "" {
		args = append(args, "--model", model)
	}
	return args
}

// それまでの会話と今の発言を、1 つの発言にまとめる
// それまでの会話と今の発言を、1 つの文章にまとめる (Claude Code・Codex に渡す)
func conversationText(history []Message, text string) string {
	var b strings.Builder
	if len(history) > 0 {
		b.WriteString(T("claudecode.historyStart") + "\n")
		for _, m := range history {
			who := T("claudecode.user")
			if m["role"] == "assistant" {
				who = T("claudecode.assistant")
			}
			t, _ := m["content"].(string)
			b.WriteString(who + ": " + strings.TrimSpace(t) + "\n")
		}
		b.WriteString(T("claudecode.historyEnd") + "\n\n")
	}
	b.WriteString(text)
	return b.String()
}

func claudeCodeMessage(history []Message, t Turn) []byte {
	content := []map[string]any{{"type": "text", "text": conversationText(history, t.Text)}}
	for _, img := range t.Images {
		content = append(content, map[string]any{"type": "image", "source": map[string]any{
			"type": "base64", "media_type": "image/jpeg", "data": base64.StdEncoding.EncodeToString(img)}})
	}
	line, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
	return append(line, '\n')
}

// claude の出力 (stream-json) を読む: 文の断片は onText に渡し、全体の返事と、失敗したときの理由を返す
func readClaudeStream(r io.Reader, onText func(string)) (reply string, failure string) {
	var b strings.Builder
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var ev struct {
			Type  string `json:"type"`
			Event struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			} `json:"event"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
			Error   string `json:"error"`
			IsError bool   `json:"is_error"`
			Result  string `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "stream_event":
			if ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta" && ev.Event.Delta.Text != "" {
				b.WriteString(ev.Event.Delta.Text)
				onText(ev.Event.Delta.Text)
			}
		case "assistant":
			// 断片が来なかったとき (エラーの文など) は、まとまった文を使う
			if ev.Error != "" && b.Len() == 0 {
				for _, c := range ev.Message.Content {
					failure += c.Text
				}
			}
		case "result":
			if ev.IsError && failure == "" {
				failure = ev.Result
			}
		}
	}
	return b.String(), strings.TrimSpace(failure)
}

func (c *claudeCodeProvider) Reply(t Turn, onText func(string)) {
	exe := claudeCodeExe()
	if exe == "" {
		onText(T("claudecode.notFound"))
		return
	}
	// 指示文はファイルで渡す (長いので、コマンドラインの長さの上限に当たらないように)
	tmp, err := os.MkdirTemp("", "lumi_claude_")
	if err != nil {
		onText(T("command.failed", err.Error()))
		return
	}
	defer os.RemoveAll(tmp)
	promptFile := filepath.Join(tmp, "system.md")
	if err := os.WriteFile(promptFile, []byte(c.s.SystemPrompt()), 0o600); err != nil {
		onText(T("command.failed", err.Error()))
		return
	}
	c.history = trimHistory(c.history, claudeCodeBudget)
	input := claudeCodeMessage(c.history, t)

	cmd := exec.Command(exe, claudeCodeArgs(promptFile, claudeCodeModel(c.s))...)
	cmd.Dir = tmp // 作業フォルダの CLAUDE.md などを読ませない
	cmd.Stdin = strings.NewReader(string(input))
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
	reply, failure := readClaudeStream(out, onText)
	waitErr := cmd.Wait()
	c.mu.Lock()
	aborted := c.aborted
	c.cmd = nil
	c.mu.Unlock()
	if aborted {
		return
	}
	switch {
	case failure != "" && strings.TrimSpace(reply) == "":
		onText(claudeCodeFailure(failure))
		return
	case strings.TrimSpace(reply) == "":
		msg := strings.TrimSpace(stderr.String())
		if waitErr != nil && msg == "" {
			msg = waitErr.Error()
		}
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		onText(T("command.errorExit", msg))
		return
	}
	// 画像は履歴には残さず、印だけ
	text := t.Text
	if len(t.Images) > 0 {
		text += imagePlaceholder(len(t.Images))
	}
	c.history = append(c.history, Message{"role": "user", "content": text}, Message{"role": "assistant", "content": strings.TrimSpace(reply)})
}

// 失敗の理由を、どうすればよいかの説明にする
func claudeCodeFailure(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not logged in") || strings.Contains(lower, "/login"):
		return T("claudecode.login")
	case strings.Contains(lower, "limit"):
		return T("claudecode.limit", msg)
	}
	return T("claudecode.failed", msg)
}
