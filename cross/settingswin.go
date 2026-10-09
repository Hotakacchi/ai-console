package main

// 設定画面: /settings window (またはトレイの「設定」) で開く小さな窓。
// 開いたときだけ窓を作り、閉じたら消す (使っていない間はメモリを使わない)。
// 値の確かめ方と保存は /set と同じ (checkSetting)。変えたらすぐ読み直して反映する。
//   画面 → Go: settingsGet / settingsSet {key, value}
//   Go → 画面: settingsData {groups, labels} / settingsSaved {key, ok, msg}

import (
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// 画面での並べ方 (まとまりごと)
var settingGroups = []struct {
	id   string
	keys []string
}{
	{"general", []string{"language", "background", "startup", "hotkey", "update_check", "keep_history", "long_memory", "idle_unload"}},
	{"ai", []string{"provider", "model", "endpoint", "api_key_env", "command", "max_tokens", "effort", "local_gpu", "system_prompt"}},
	{"pc", []string{"pc_control", "auto_run", "shell", "web_search", "search_url", "clipboard", "screen", "pc_watch"}},
	{"voice", []string{"voice_input", "wake_word", "wake_mode", "voice_debug", "stt", "whisper_model", "tts", "voice", "voice_rate", "voicevox_voice"}},
	{"look", []string{"face_color", "face_size", "font_size", "font", "peek_position"}},
	{"link", []string{"phone", "phone_port", "discord", "discord_app_id", "weather_location"}},
}

// /settings window
func (l *Lumi) openSettingsWindow() {
	if !l.gui() {
		l.info(T("settings.noWindow"))
		return
	}
	if l.settingsWin != nil {
		l.settingsWin.Show()
		l.settingsWin.Focus()
		return
	}
	w := l.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "settings",
		Title:            T("settings.title"),
		Width:            760,
		Height:           640,
		MinWidth:         480,
		MinHeight:        360,
		URL:              "/#settings",
		BackgroundColour: application.NewRGB(12, 12, 12),
		Windows:          application.WindowsWindow{Theme: application.Dark},
	})
	l.settingsWin = w
	// 閉じたら窓ごと消す (次に開くときに作り直す)
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { l.settingsWin = nil })
	w.Show()
}

// 画面に、今の設定と選べる値を送る
func (l *Lumi) sendSettingsData() {
	if l.settingsWin == nil {
		return
	}
	var groups []map[string]any
	for _, g := range settingGroups {
		var items []map[string]any
		for _, k := range g.keys {
			def := findSettingKey(k)
			if def == nil {
				continue
			}
			item := map[string]any{"key": def.key, "value": l.s.Get(def.key, ""), "help": def.help(), "kind": "text"}
			switch rule := def.choices(); {
			case strings.HasPrefix(rule, "#"):
				r := strings.Split(rule, ":")
				item["kind"] = "number"
				if len(r) == 3 {
					item["min"], _ = strconv.ParseFloat(r[1], 64)
					item["max"], _ = strconv.ParseFloat(r[2], 64)
					item["int"] = r[0] == "#int"
				}
			case rule != "":
				item["kind"] = "choice"
				item["choices"] = strings.Split(rule, ",")
			}
			items = append(items, item)
		}
		groups = append(groups, map[string]any{"id": g.id, "title": T("settings.group." + g.id), "items": items})
	}
	labels := map[string]string{}
	for _, k := range []string{"settings.title", "settings.default", "settings.hint", "settings.saved", "settings.close"} {
		labels[k] = T(k)
	}
	l.settingsWin.EmitEvent("settingsData", map[string]any{"groups": groups, "labels": labels})
}

// 画面で値が変わった: /set と同じように確かめて保存し、読み直して反映する
func (l *Lumi) saveSettingFromWindow(key, value string) {
	reply := func(ok bool, msg string) {
		if l.settingsWin != nil {
			l.settingsWin.EmitEvent("settingsSaved", map[string]any{"key": key, "ok": ok, "msg": msg})
		}
	}
	def := findSettingKey(key)
	if def == nil {
		reply(false, T("set.noKey", key))
		return
	}
	if l.s.Err != nil {
		reply(false, T("settings.broken"))
		return
	}
	stored, shown, msg := checkSetting(def, value)
	if msg != "" {
		reply(false, msg)
		return
	}
	if err := l.s.Set(def.key, stored); err != nil {
		reply(false, T("set.saveFailed", err.Error()))
		return
	}
	if shown == "" {
		shown = T("set.default")
	}
	l.reload(false)
	reply(true, T("set.done", def.key, shown))
	l.sendSettingsData() // 読み直した結果 (言語が変わったときの文言など) を出し直す
}
