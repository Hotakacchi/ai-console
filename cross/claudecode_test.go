package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// 偽の claude を作って PATH の先頭に置く
func fakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", filepath.Join(dir, name), "./testdata/fakeclaude").CombinedOutput()
	if err != nil {
		t.Fatalf("build fake claude: %v %s", err, out)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func newTestSettings(t *testing.T, kv map[string]any) *Settings {
	s := &Settings{vals: map[string]any{}}
	for k, v := range kv {
		s.vals[k] = v
	}
	return s
}

func TestClaudeCodeProvider(t *testing.T) {
	loadLocales()
	dir := fakeClaude(t)
	argsFile := filepath.Join(dir, "args.json")
	t.Setenv("FAKE_CLAUDE_ARGS", argsFile)
	p := &claudeCodeProvider{s: newTestSettings(t, map[string]any{"model": "my-model.gguf"})}

	var got strings.Builder
	p.Reply(Turn{Text: "こんにちは", Images: [][]byte{{1, 2, 3}}}, func(s string) { got.WriteString(s) })
	if got.String() != "受け取りました: こんにちは (画像 1)" {
		t.Fatalf("reply %q", got.String())
	}
	var args []string
	b, _ := os.ReadFile(argsFile)
	json.Unmarshal(b, &args)
	i := slices.Index(args, "--tools")
	if i < 0 || args[i+1] != "" || !slices.Contains(args, "--strict-mcp-config") || !slices.Contains(args, "--no-session-persistence") || !slices.Contains(args, "--system-prompt-file") {
		t.Errorf("args %q", args)
	}
	if slices.Contains(args, "--model") {
		t.Errorf(".gguf のモデル名は渡さない: %q", args)
	}
	// 履歴: 画像は印だけ、返事も残る
	if len(p.history) != 2 || !strings.HasPrefix(p.history[0]["content"].(string), "こんにちは\n") || p.history[1]["content"] != "受け取りました: こんにちは (画像 1)" {
		t.Errorf("history %v", p.history)
	}

	// ログインしていない
	t.Setenv("FAKE_CLAUDE_MODE", "login")
	got.Reset()
	p.Reply(Turn{Text: "やあ"}, func(s string) { got.WriteString(s) })
	if got.String() != T("claudecode.login") || len(p.history) != 2 {
		t.Errorf("login: %q (history %d)", got.String(), len(p.history))
	}

	// 中断
	t.Setenv("FAKE_CLAUDE_MODE", "slow")
	start := time.Now()
	go func() { time.Sleep(500 * time.Millisecond); p.Abort() }()
	got.Reset()
	p.Reply(Turn{Text: "待って"}, func(s string) { got.WriteString(s) })
	if d := time.Since(start); d > 10*time.Second || got.String() != "" {
		t.Errorf("abort took %v, text %q", d, got.String())
	}
}

func TestClaudeCodeMessage(t *testing.T) {
	loadLocales()
	line := claudeCodeMessage([]Message{{"role": "user", "content": "前の質問"}, {"role": "assistant", "content": "前の答え"}}, Turn{Text: "今の質問"})
	var m struct {
		Type    string `json:"type"`
		Message struct {
			Content []map[string]any `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &m); err != nil || m.Type != "user" {
		t.Fatal(err, string(line))
	}
	text := m.Message.Content[0]["text"].(string)
	for _, want := range []string{T("claudecode.user") + ": 前の質問", T("claudecode.assistant") + ": 前の答え", "今の質問"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %q", want, text)
		}
	}
	if !strings.HasSuffix(text, "今の質問") {
		t.Errorf("今の発言は最後に: %q", text)
	}
}

func TestClaudeCodeFailure(t *testing.T) {
	loadLocales()
	if claudeCodeFailure("Not logged in · Please run /login") != T("claudecode.login") {
		t.Error("login")
	}
	if !strings.Contains(claudeCodeFailure("Claude usage limit reached"), "usage limit") {
		t.Error("limit")
	}
}
