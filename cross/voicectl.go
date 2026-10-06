package main

// 音声入力の制御 (Go 側)。聞き取りそのものは画面側 (assets/voice.js) が行い、
//   画面 → Go: voiceState / voiceWoke / voiceHeard(文字) / voiceTimeout
//   Go → 画面: voiceStart / voiceStop

import (
	"fmt"
	"strings"
)

// 起動時や設定を読み直したときに、設定どおり音声入力を始める (止める)
func (l *Lumi) applyVoice(announce bool) {
	if !l.micOn || !l.s.On("voice_input", "on") {
		l.emit("voiceStop", nil)
		return
	}
	lang := currentLang()
	if _, ok := voiceModels[lang]; !ok {
		if announce {
			l.info(T("voice.error", T("voice.noModel")))
		}
		l.emit("voiceStop", nil)
		return
	}
	if !voiceInstalled(dataDir(), lang) {
		if announce {
			l.write(T("voice.installHint", float64(voiceDownloadSize(lang))/1e6)+"\n\n", "yellow")
		}
		l.emit("voiceStop", nil)
		return
	}
	start := map[string]any{
		"lang":  lang,
		"model": "/voice/" + lang + "/model.tar.gz",
		"wake":  l.s.WakeWord(),
	}
	if l.s.Get("stt", "vosk") == "whisper" {
		model := whisperModel(l.s)
		if whisperInstalled(dataDir(), model) {
			start["whisper"] = whisperClientModel(model)
		} else if announce {
			l.write(T("whisper.installHint", float64(whisperSize(model))/1e6)+"\n\n", "yellow")
		}
	}
	l.emit("voiceStart", start)
}

// /mic: その場で音声入力をオン/オフする (入っていなければダウンロードしてから始める)
func (l *Lumi) toggleMic() {
	if l.micOn && l.listening {
		l.micOn = false
		l.emit("voiceStop", nil)
		l.info(T("voice.off"))
		return
	}
	l.micOn = true
	if !l.s.On("voice_input", "on") {
		l.s.Set("voice_input", "on")
	}
	if !voiceInstalled(dataDir(), currentLang()) {
		l.installVoice()
		return
	}
	l.applyVoice(true)
}

// /install-voice: 今の言語の認識モデルをダウンロードして、音声入力を始める
func (l *Lumi) installVoice() {
	lang := currentLang()
	if _, ok := voiceModels[lang]; !ok {
		l.errorText(T("voice.error", T("voice.noModel")))
		return
	}
	l.setBusy(true)
	cancel := make(chan struct{})
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	l.write(T("voice.installStart", float64(voiceDownloadSize(lang))/1e6)+"\n", "dim")
	go func() {
		lastPct := -1
		err := installVoice(dataDir(), lang, func(step string, ratio float64) {
			if pct := int(ratio * 100); pct/10 != lastPct/10 {
				l.write(fmt.Sprintf("  [%3d%%] %s\n", pct, step), "dim")
				lastPct = pct
			}
		}, func() bool {
			select {
			case <-cancel:
				return true
			default:
				return false
			}
		})
		l.setBusy(false)
		switch {
		case err == errCancelled:
			l.write("^C\n"+T("local.installCancelled", "/install-voice")+"\n\n", "dim")
		case err != nil:
			l.errorText(T("download.failed", err.Error()))
		default:
			l.info(T("voice.installed"))
			if l.micOn {
				l.applyVoice(true)
			}
		}
	}()
}

// 画面から: 音声入力の状態 (ready / error / stopped)
func (l *Lumi) voiceState(state, message string) {
	switch state {
	case "ready":
		l.listening = true
		l.info(T("voice.on", l.s.WakeWord()))
	case "error":
		l.listening = false
		l.errorText(T("voice.error", message))
	case "stopped":
		l.listening = false
	}
	l.updateTitle()
}

// 呼びかけを聞き取った。ウィンドウが隠れていれば右下から顔を出す
func (l *Lumi) voiceWoke() {
	if l.s.Get("voice_debug", "off") == "on" {
		l.write(fmt.Sprintf("  [voice] woke (window visible: %v)\n", l.win.IsVisible()), "dim")
	}
	if !l.win.IsVisible() {
		l.peek.pop(T("peek.hey"))
	}
}

// 話しかけられた内容。ウィンドウを出して、打ち込まれたのと同じように扱う
func (l *Lumi) voiceHeard(text string) {
	if l.peek.popped {
		l.peek.retract(false, T("peek.ok"))
	}
	if !l.win.IsVisible() {
		l.show()
	}
	l.submit(voiceCommand(text))
}

func (l *Lumi) voiceTimeout() {
	if l.peek.popped {
		l.peek.retract(true, T("peek.bye"))
	}
}

// 声で言われたアプリのコマンド (「ミュート」など) を / コマンドに置き換える
func voiceCommand(text string) string {
	t := strings.ToLower(strings.Trim(strings.TrimSpace(text), "。！？!?. "))
	t = strings.NewReplacer(" ", "", "　", "").Replace(t)
	for _, cmd := range []string{"mute", "cls", "mic", "exit"} {
		for _, w := range TList("voice.cmd." + cmd) {
			if t == strings.NewReplacer(" ", "", "　", "").Replace(strings.ToLower(w)) {
				return "/" + cmd
			}
		}
	}
	return text
}
