package main

import (
	"net"
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

	// 「コマンド …」で、どのコマンドも呼べる (引数も)。危ないものは確認つき
	setLanguage("ja")
	for _, c := range []struct {
		said, cmd string
		confirm   bool
	}{
		{"コマンド ミュート", "/mute", false},
		{"コマンド、設定。", "/settings", false},
		{"スラッシュ 自動モード オン", "/auto on", true},
		{"コマンド 自動モード オフ", "/auto off", true},
		{"コマンド ローカルAI 一覧", "/install-local list", true},
		{"コマンド 画面 このエラーは何？", "/screen このエラーは何", false},
		{"コマンド 画面クリア", "/cls", false}, // 「画面」より長い「画面クリア」が勝つ
		{"コマンド アップデート", "/update", true},
		{"コマンド 終了", "/exit", true},
		{"コマンド 一覧", "/help", false},
		{"command shell", "/shell", false},
	} {
		cmd, confirm, ok := parseVoiceCommand(c.said)
		if !ok || cmd != c.cmd || confirm != c.confirm {
			t.Errorf("%q → %q confirm=%v ok=%v, want %q confirm=%v", c.said, cmd, confirm, ok, c.cmd, c.confirm)
		}
	}
	// 合図の言葉がなければ、普段の話しかけ
	for _, said := range []string{"設定を変えたいんだけど", "コマンド", "コマンドって何？", "アップデートある？"} {
		if cmd, _, ok := parseVoiceCommand(said); ok {
			t.Errorf("%q became %q", said, cmd)
		}
	}
	setLanguage("en")
	if cmd, confirm, ok := parseVoiceCommand("Command auto mode on."); !ok || cmd != "/auto on" || !confirm {
		t.Errorf("en auto = %q %v %v", cmd, confirm, ok)
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

func TestWhisperFiles(t *testing.T) {
	for _, model := range []string{"whisper-base", "whisper-small"} {
		files := whisperNeeded(model)
		if len(files) != 14 {
			t.Errorf("%s: %d files", model, len(files))
		}
		seen := map[string]bool{}
		for _, f := range files {
			p := whisperLocal("/x", f)
			if seen[p] || len(f.asset.SHA256) != 64 || f.asset.Size <= 0 {
				t.Errorf("%s: bad entry %s", model, f.path)
			}
			seen[p] = true
		}
	}
	if err := installWhisper(t.TempDir(), "whisper-huge", func(string, float64) {}, func() bool { return true }); err == nil {
		t.Error("unknown model accepted")
	}
}

func TestBlockedIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "192.168.0.1": true, "169.254.1.1": true,
		"100.100.1.1": true, "::1": true, "::ffff:127.0.0.1": true, "fd00::1": true, "0.0.0.0": true,
		"8.8.8.8": false, "100.128.0.1": false, "2001:4860:4860::8888": false,
	} {
		if got := blockedIP(net.ParseIP(ip)); got != want {
			t.Errorf("%s: %v", ip, got)
		}
	}
}

// /set voice_input off などで、聞き取りのやり直し (止める) が必要と分かる
func TestVoiceSettingChange(t *testing.T) {
	loadLocales()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(`{"voice_input": "on"}`), 0o644)
	s := &Settings{Path: path, vals: map[string]any{}}
	s.parse([]byte(`{"voice_input": "on"}`))
	l := &Lumi{s: s}
	l.voiceApplied = l.voiceKey() // 聞き取りを始めた
	if l.voiceChanged() {
		t.Fatal("changed before anything changed")
	}
	// /set は先に設定を書き換えてから読み直す: それでも「変わった」と分かること
	s.Set("voice_input", "off")
	if !l.voiceChanged() {
		t.Error("voice_input off was not noticed")
	}
	l.voiceApplied = l.voiceKey()
	s.Set("stt", "whisper")
	if !l.voiceChanged() {
		t.Error("stt change was not noticed")
	}
}

// 設定画面から変えた値も /set と同じように確かめて保存する
func TestSettingsWindowSave(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	tempDataDir(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(`{"provider": "offline"}`), 0o644)
	s := &Settings{Path: path, vals: map[string]any{}}
	s.parse([]byte(`{"provider": "offline"}`))
	if def := findSettingKey("idle_unload"); def == nil {
		t.Fatal("no idle_unload")
	} else if _, _, msg := checkSetting(def, "abc"); msg == "" {
		t.Error("non-number accepted")
	} else if v, _, msg := checkSetting(def, "3"); msg != "" || v != 3 {
		t.Errorf("3 → %v %q", v, msg)
	}
	if _, _, msg := checkSetting(findSettingKey("wake_mode"), "HEY"); msg != "" {
		t.Errorf("choices are case-insensitive: %q", msg)
	}
	if _, _, msg := checkSetting(findSettingKey("wake_mode"), "loud"); msg == "" {
		t.Error("bad choice accepted")
	}
	// どのまとまりにも、存在しない項目が入っていない / 全部の項目がどこかにある
	inGroup := map[string]bool{}
	for _, g := range settingGroups {
		for _, k := range g.keys {
			if findSettingKey(k) == nil {
				t.Errorf("group %s has unknown key %s", g.id, k)
			}
			inGroup[k] = true
		}
	}
	for _, k := range settingKeys {
		if !inGroup[k.key] {
			t.Errorf("%s is not in any group of the settings window", k.key)
		}
	}
}

// ほぼ同時に 2 回呼ばれても、返事を始められるのは 1 つだけ
func TestBeginOnce(t *testing.T) {
	for i := 0; i < 50; i++ {
		l := &Lumi{s: &Settings{vals: map[string]any{}}}
		ok := make(chan bool, 8)
		for j := 0; j < 8; j++ {
			go func() { ok <- l.begin() }()
		}
		n := 0
		for j := 0; j < 8; j++ {
			if <-ok {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%d replies started at once", n)
		}
	}
}
