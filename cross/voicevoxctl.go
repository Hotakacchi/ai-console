package main

// VOICEVOX の操作 (Go 側): /install-voicevox と /voices

import (
	"fmt"
	"strings"
)

// /install-voicevox: エンジンと音声モデルを入れて、読み上げを VOICEVOX に切り替える
func (l *Lumi) installVoicevoxCmd() {
	l.setBusy(true)
	cancel := make(chan struct{})
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	l.write(T("voicevox.installStart")+"\n", "dim")
	go func() {
		defer l.setBusy(false)
		lastPct := -1
		defer l.setActivity("", 0)
		err := installVoicevox(dataDir(), func(step string, ratio float64) {
			l.setActivity("download", ratio) // 口がプログレスバーになる
			if pct := int(ratio * 100); pct/5 != lastPct/5 {
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
		switch {
		case err == errCancelled:
			l.write("^C\n"+T("local.installCancelled", "/install-voicevox")+"\n\n", "dim")
		case err != nil:
			l.errorText(T("download.failed", err.Error()))
		default:
			if l.s.Err == nil {
				l.s.Set("tts", "voicevox")
			}
			l.ttsErrorShown = false
			l.write(T("voicevox.installed")+"\n", "dim")
			if err := voicevox.Start(dataDir()); err != nil {
				l.errorText(T("voicevox.error", err.Error()))
				return
			}
			l.info(T("voicevox.ready"))
		}
	}()
}

// /voices (VOICEVOX のとき): キャラクターとスタイルの一覧
func (l *Lumi) listVoicevox() {
	if err := voicevox.Start(dataDir()); err != nil {
		l.errorText(T("voicevox.error", err.Error()))
		return
	}
	current := voicevox.pickStyle(l.s.GetInt("voicevox_voice", -1))
	voicevox.mu.Lock()
	speakers := voicevox.speakers
	voicevox.mu.Unlock()
	var b strings.Builder
	for _, s := range speakers {
		for _, st := range s.Styles {
			mark := ""
			if st.ID == current {
				mark = "  " + T("voices.current")
			}
			fmt.Fprintf(&b, "  %4d  %s (%s)%s\n", st.ID, s.Name, st.Name, mark)
		}
	}
	b.WriteString("\n  " + T("voicevox.howto"))
	l.info(b.String())
}
