package main

// settings.json の読み書き。項目の並びを保ったまま 1 項目ずつ書き換えられる。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// AI への指示文 (キャラクター・PC 操作・Web 検索) は locales の prompt.* にある

type Settings struct {
	Path string
	Err  error
	keys []string
	vals map[string]any
}

// settings.json のひな形 (Windows 版と同じ項目)
func settingsTemplate(provider string) string {
	return `{
  "language": "auto",
  "provider": "` + provider + `",
  "model": "",
  "endpoint": "",
  "api_key_env": "",
  "command": "",
  "max_tokens": 0,
  "effort": "",
  "local_gpu": "auto",
  "idle_unload": 1,
  "pc_watch": "on",
  "pc_control": "on",
  "auto_run": "off",
  "shell": "auto",
  "web_search": "on",
  "clipboard": "ask",
  "screen": "ask",
  "hotkey": "CmdOrCtrl+Alt+L",
  "update_check": "on",
  "weather_location": "",
  "phone": "off",
  "discord": "off",
  "keep_history": "on",
  "search_url": "",
  "voice_input": "on",
  "wake_word": "",
  "wake_mode": "hey",
  "wake_confidence": 0.6,
  "stt": "vosk",
  "whisper_model": "base",
  "system_prompt": "",
  "voice": "",
  "voice_rate": 1,
  "tts": "system",
  "voicevox_voice": -1,
  "background": "on",
  "startup": "off"
}
`
}

// データの置き場所。exe と同じフォルダに settings.json があればそこを使う (持ち運び用)
func dataDir() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, "settings.json")); err == nil {
			return dir
		}
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "Lumi")
		}
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Lumi")
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "lumi")
	}
	return filepath.Join(home, ".local", "share", "lumi")
}

func LoadSettings() *Settings {
	dir := dataDir()
	s := &Settings{Path: filepath.Join(dir, "settings.json"), vals: map[string]any{}}
	if _, err := os.Stat(s.Path); os.IsNotExist(err) {
		provider := "offline"
		if localInstalled(dir, "") {
			provider = "local"
		}
		os.MkdirAll(dir, 0o755)
		if err := os.WriteFile(s.Path, []byte(settingsTemplate(provider)), 0o644); err != nil {
			s.Err = err
			return s
		}
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		s.Err = err
		return s
	}
	s.Err = s.parse(data)
	return s
}

// 並びを保つため、トークンを順に読む
func (s *Settings) parse(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	dec.UseNumber()
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("%s", T("settings.notJSON"))
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := t.(string)
		var v any
		if err := dec.Decode(&v); err != nil {
			return err
		}
		if _, ok := s.vals[key]; !ok {
			s.keys = append(s.keys, key)
		}
		s.vals[key] = v
	}
	return nil
}

func (s *Settings) Get(key, fallback string) string {
	v, ok := s.vals[key]
	if !ok || v == nil {
		return fallback
	}
	str := strings.TrimSpace(fmt.Sprint(v))
	if str == "" {
		return fallback
	}
	return str
}

func (s *Settings) GetInt(key string, fallback int) int {
	n, err := strconv.Atoi(s.Get(key, ""))
	if err != nil || n == 0 {
		return fallback
	}
	return n
}

func (s *Settings) GetFloat(key string, fallback float64) float64 {
	f, err := strconv.ParseFloat(s.Get(key, ""), 64)
	if err != nil {
		return fallback
	}
	return f
}

func (s *Settings) On(key, fallback string) bool {
	return strings.ToLower(s.Get(key, fallback)) != "off"
}

// 1 項目を書き換えて保存する (項目の並びはそのまま)
func (s *Settings) Set(key string, value any) error {
	// 読めなかった (壊れた) ファイルを、読めた分だけで上書きしてしまわないようにする
	if s.Err != nil {
		return s.Err
	}
	if _, ok := s.vals[key]; !ok {
		s.keys = append(s.keys, key)
	}
	s.vals[key] = value
	var b strings.Builder
	b.WriteString("{\n")
	for i, k := range s.keys {
		kj, _ := json.Marshal(k)
		vj, _ := json.Marshal(s.vals[k])
		b.WriteString("  " + string(kj) + ": " + string(vj))
		if i < len(s.keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return os.WriteFile(s.Path, []byte(b.String()), 0o644)
}

func (s *Settings) PcControl() bool { return s.On("pc_control", "on") }
func (s *Settings) WebSearch() bool { return s.On("web_search", "on") }

func (s *Settings) SystemPrompt() string {
	p := T("prompt.system")
	if custom := s.Get("system_prompt", ""); custom != "" {
		// 自分で書いたキャラクター設定でも、話す言語は今の言語に合わせる
		p = custom + "\n" + T("prompt.language")
	}
	// 自分のこと (バージョン・OS・使っている AI・できること) を聞かれても答えられるように
	p += "\n\n" + T("prompt.self", versionLabel(), osName(), aiLabel(s))
	if s.PcControl() {
		// 小さなモデルでも迷わないよう、この OS で使える具体例を添える
		disk, folder := pcExamples()
		p += "\n\n" + T("prompt.pc", osName(), shellName())
		p += "\n" + T("prompt.pcExample", T("prompt.pcAskDisk"), disk)
		p += "\n" + T("prompt.pcExample", T("prompt.pcAskFolder"), folder)
		p += "\n" + T("prompt.pcPlaces", homeDir(), strings.Join(drives(), " "))
		p += "\n" + T("prompt.pcBook") + "\n" + T("pcbook."+runtime.GOOS)
		if mine := userCommands(); mine != "" {
			p += "\n" + T("prompt.pcMine") + "\n" + mine
		}
		if plugins := pluginsPrompt(); plugins != "" {
			p += "\n\n" + plugins
		}
		if s.Get("auto_run", "off") == "on" {
			p += "\n" + T("prompt.auto")
		}
	}
	if s.WebSearch() {
		p += "\n\n" + T("prompt.web")
	}
	if s.Get("clipboard", "ask") != "off" {
		p += "\n\n" + T("prompt.clipboard")
	}
	if s.Get("screen", "ask") != "off" {
		p += "\n\n" + T("prompt.screen")
	}
	// 表情・リマインダー (今の時刻つき)・記憶
	p += "\n\n" + T("prompt.face")
	p += "\n\n" + T("prompt.remind")
	p += "\n\n" + T("prompt.watch")
	p += "\n\n" + T("prompt.memory") + "\n" + memories.prompt()
	return p
}

// 呼びかけの言葉 (空なら言語ごとの既定)
func (s *Settings) WakeWord() string { return s.Get("wake_word", T("voice.wakeWord")) }

// API キーは設定ファイルに書かず、api_key_env に書いた環境変数から読む
func (s *Settings) APIKey(defaultEnv string) string {
	env := s.Get("api_key_env", defaultEnv)
	if env == "" {
		return ""
	}
	return os.Getenv(env)
}

func osName() string {
	switch runtime.GOOS {
	case "windows":
		return windowsName()
	case "darwin":
		return "Mac"
	}
	return "Linux"
}

// 「ディスクの空き容量」「ダウンロードフォルダを開く」の、この OS でのコマンド
func pcExamples() (disk, folder string) {
	switch runtime.GOOS {
	case "windows":
		return "Get-PSDrive C", `Start-Process "$env:USERPROFILE\Downloads"`
	case "darwin":
		return "df -h /", "open ~/Downloads"
	}
	return "df -h /", "xdg-open ~/Downloads"
}

func shellName() string {
	switch runtime.GOOS {
	case "windows":
		return "PowerShell"
	case "darwin":
		return "zsh"
	}
	return "bash"
}

// 今つないでいる AI の名前 (指示文で自分のことを話すため)
func aiLabel(s *Settings) string {
	model := s.Get("model", "")
	switch strings.ToLower(s.Get("provider", "offline")) {
	case "local":
		if m, ok := currentLocalModel(s); ok {
			return T("self.local", m.Name)
		}
		return T("self.local", strings.TrimSuffix(filepath.Base(model), filepath.Ext(model)))
	case "anthropic":
		if model == "" {
			model = "claude-opus-5-5"
		}
		return "Claude (" + model + ")"
	case "openai":
		return T("self.openai", model)
	case "command":
		return T("self.command")
	}
	return T("self.offline")
}
