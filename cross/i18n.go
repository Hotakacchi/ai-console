package main

// 多言語対応。文言は locales/<言語>.json にあり、T("キー", 引数...) で取り出す。
// データフォルダの locales/<言語>.json を置けば、言語の追加や文言の上書きができる。

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	i18nMu   sync.RWMutex
	messages = map[string]map[string]string{}
	lang     = "en"
)

// 同梱の翻訳と、データフォルダに置かれた翻訳を読み込む
func loadLocales() {
	i18nMu.Lock()
	defer i18nMu.Unlock()
	read := func(name string, data []byte) {
		code := strings.TrimSuffix(filepath.Base(name), ".json")
		m := map[string]string{}
		if json.Unmarshal(data, &m) != nil {
			return
		}
		if messages[code] == nil {
			messages[code] = map[string]string{}
		}
		for k, v := range m {
			messages[code][k] = v
		}
	}
	entries, _ := localeFS.ReadDir("locales")
	for _, e := range entries {
		data, _ := localeFS.ReadFile("locales/" + e.Name())
		read(e.Name(), data)
	}
	extra, _ := filepath.Glob(filepath.Join(dataDir(), "locales", "*.json"))
	for _, f := range extra {
		if data, err := os.ReadFile(f); err == nil {
			read(f, data)
		}
	}
}

// 使える言語のコード (ja, en, ...)
func languages() []string {
	i18nMu.RLock()
	defer i18nMu.RUnlock()
	var list []string
	for code := range messages {
		list = append(list, code)
	}
	sort.Strings(list)
	return list
}

// setting が auto や空なら OS の言語、それも無ければ英語にする
func setLanguage(setting string) {
	code := strings.ToLower(strings.TrimSpace(setting))
	if code == "" || code == "auto" {
		code = osLanguage()
	}
	code = strings.SplitN(strings.ReplaceAll(code, "_", "-"), "-", 2)[0]
	i18nMu.Lock()
	defer i18nMu.Unlock()
	if _, ok := messages[code]; ok {
		lang = code
	} else {
		lang = "en"
	}
}

func currentLang() string {
	i18nMu.RLock()
	defer i18nMu.RUnlock()
	return lang
}

// 今の言語の文言 (無ければ英語、それも無ければキーそのもの)
func T(key string, args ...any) string {
	i18nMu.RLock()
	s, ok := messages[lang][key]
	if !ok {
		s, ok = messages["en"][key]
	}
	i18nMu.RUnlock()
	if !ok {
		s = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// カンマ区切りの文言を一覧にする
func TList(key string) []string {
	var out []string
	for _, s := range strings.Split(T(key), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// 画面に渡すための、今の言語の文言をまとめたもの
func clientMessages() map[string]string {
	keys := []string{"voice.wakeWord", "voice.wakeAliases", "voice.wakePrefixes", "voice.yes", "voice.no", "voice.listening",
		"voice.letterY", "voice.letterA", "voice.letterN",
		"tab.lumi", "tab.new", "tab.choose", "tab.close",
		"voice.cmd.mute", "voice.cmd.cls", "voice.cmd.mic", "voice.cmd.exit", "voice.mark"}
	m := map[string]string{"lang": currentLang()}
	for _, k := range keys {
		m[k] = T(k)
	}
	return m
}
