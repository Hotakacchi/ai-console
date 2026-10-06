package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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
	var visible strings.Builder
	var got []string
	x := toolExtractor{
		OnText: func(s string) { visible.WriteString(s) },
		OnTag: func(r toolRequest) {
			got = append(got, r.Kind+":"+r.Command+":"+r.Attrs["in"]+":"+map[bool]string{true: "admin", false: ""}[r.Admin])
		},
	}
	for _, c := range []string{"<face>happy</face>調べますね。<sea", "rch>天気</sea", "rch>それと<run adm", "in>whoami</run>a < b",
		" <remind in=\"5m\">お茶</remind><clipboard/><runtime>"} {
		x.Push(c)
	}
	x.Flush()
	if visible.String() != "調べますね。それとa < b <runtime>" {
		t.Errorf("visible = %q", visible.String())
	}
	want := []string{"face:happy::", "search:天気::", "run:whoami::admin", "remind:お茶:5m:", "clipboard:::"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %q", got)
	}
}

func TestReminderTime(t *testing.T) {
	now := time.Date(2026, 10, 6, 14, 30, 0, 0, time.Local)
	cases := []struct {
		attrs map[string]string
		want  time.Time
		ok    bool
	}{
		{map[string]string{"in": "300"}, now.Add(5 * time.Minute), true},
		{map[string]string{"in": "5m"}, now.Add(5 * time.Minute), true},
		{map[string]string{"in": "2時間"}, now.Add(2 * time.Hour), true},
		{map[string]string{"at": "15:00"}, time.Date(2026, 10, 6, 15, 0, 0, 0, time.Local), true},
		{map[string]string{"at": "09:00"}, time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local), true}, // 過ぎていれば明日
		{map[string]string{"at": "2026-10-08 10:30"}, time.Date(2026, 10, 8, 10, 30, 0, 0, time.Local), true},
		{map[string]string{"in": "soon"}, time.Time{}, false},
	}
	for _, c := range cases {
		got, ok := reminderTime(c.attrs, now)
		if ok != c.ok || (ok && !got.Equal(c.want)) {
			t.Errorf("%v: got %v %v, want %v %v", c.attrs, got, ok, c.want, c.ok)
		}
	}
}

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.0.1", "1.0.0", true}, {"1.10.0", "1.9.0", true}, {"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.0-beta", true}, {"0.9.0", "1.0.0", false}} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestHistoryMessages(t *testing.T) {
	e := func(role, text string) historyEntry { return historyEntry{Role: role, Text: text} }
	got := historyMessages([]historyEntry{e("assistant", "orphan"), e("user", "a"), e("assistant", "b"),
		e("user", "c"), e("user", "d"), e("assistant", "e"), e("user", "last")})
	want := []Message{{"role": "user", "content": "a"}, {"role": "assistant", "content": "b"},
		{"role": "user", "content": "c\nd"}, {"role": "assistant", "content": "e"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
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
