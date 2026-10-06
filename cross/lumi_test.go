package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestContainsWord(t *testing.T) {
	cases := []struct {
		s, w string
		want bool
	}{
		{"hi there", "hi", true},
		{"this is it", "hi", false},
		{"say hi!", "hi", true},
		{"what time is it", "what time", true},
		{"whatever", "what", false},
	}
	for _, c := range cases {
		if got := containsWord(c.s, c.w); got != c.want {
			t.Errorf("containsWord(%q, %q) = %v, want %v", c.s, c.w, got, c.want)
		}
	}
}

func TestSentenceSplitter(t *testing.T) {
	var sp sentenceSplitter
	var got []string
	for _, chunk := range []string{"「元気？", "」ですね。こんにち", "は！", "さようなら"} {
		got = append(got, sp.Push(chunk)...)
	}
	if rest := sp.Flush(); rest != "" {
		got = append(got, rest)
	}
	want := []string{"「元気？」", "ですね。", "こんにちは！", "さようなら"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestToolExtractor(t *testing.T) {
	var x toolExtractor
	var visible strings.Builder
	for _, c := range []string{"調べますね。<sea", "rch>天気</sea", "rch>それと<run adm", "in>whoami</run>a < b"} {
		visible.WriteString(x.Push(c))
	}
	visible.WriteString(x.Flush())
	if visible.String() != "調べますね。それとa < b" {
		t.Errorf("visible = %q", visible.String())
	}
	want := []toolRequest{{"search", "天気", false}, {"run", "whoami", true}}
	if !reflect.DeepEqual(x.Requests, want) {
		t.Errorf("requests = %+v", x.Requests)
	}
}

func TestVoiceCommand(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	if got := voiceCommand("ミュート。"); got != "/mute" {
		t.Errorf("ja mute = %q", got)
	}
	if got := voiceCommand("今何時"); got != "今何時" {
		t.Errorf("ja passthrough = %q", got)
	}
	setLanguage("en")
	if got := voiceCommand("Clear the screen"); got != "/cls" {
		t.Errorf("en cls = %q", got)
	}
}

// 壊れた settings.json は Set で上書きしない
func TestSetKeepsBrokenSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	broken := "{\n  \"provider\": \"anthropic\",\n  \"model\": oops\n}\n"
	os.WriteFile(path, []byte(broken), 0o644)
	s := &Settings{Path: path, vals: map[string]any{}}
	data, _ := os.ReadFile(path)
	s.Err = s.parse(data)
	if s.Err == nil {
		t.Fatal("expected a parse error")
	}
	if err := s.Set("voice_input", "on"); err == nil {
		t.Error("Set should refuse to write when the file could not be read")
	}
	if after, _ := os.ReadFile(path); string(after) != broken {
		t.Errorf("file was changed:\n%s", after)
	}
}

// 翻訳ファイルは日本語と英語で同じキーを持つ
func TestLocaleKeysMatch(t *testing.T) {
	loadLocales()
	for k := range messages["ja"] {
		if _, ok := messages["en"][k]; !ok {
			t.Errorf("en is missing %q", k)
		}
	}
	for k := range messages["en"] {
		if _, ok := messages["ja"][k]; !ok {
			t.Errorf("ja is missing %q", k)
		}
	}
}
