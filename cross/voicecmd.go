package main

// 声でアプリのコマンドを使う:
//   「(ルミ、) コマンド ミュート」「スラッシュ 自動モード オン」「コマンド 画面 このエラーは何」のように、
//   合図の言葉 (voice.cmdPrefix) のあとにコマンドの呼び名 (voice.name.<コマンド>) と、続けて引数を言う。
//   「ミュート」「画面を消して」などの決まった言い方 (voice.cmd.<コマンド>) だけでも使える (前から)。
// 近くの人の声やテレビの音で勝手に動かないよう、ダウンロード・アップデート・自動モード・スマホ・終了などは確認してから。

import (
	"strings"
)

// 声で呼べるコマンド (呼び名は locales の voice.name.<名前>)
var voiceCommands = []string{
	"help", "settings", "voices", "config", "reload", "mute", "mic",
	"install-local", "install-voice", "install-voicevox", "install-whisper", "install-vision",
	"detach", "screen", "commands", "words", "shell", "auto", "routines", "plugins",
	"phone", "discord", "watch", "notes", "media", "find", "clips", "memory", "reminders", "history", "update", "peek", "cls", "exit",
}

// 声で言われたときは確認してから実行するもの
var voiceConfirm = map[string]bool{
	"exit": true, "update": true, "auto": true, "phone": true, "discord": true, "reload": true,
	"install-local": true, "install-voice": true, "install-voicevox": true, "install-whisper": true, "install-vision": true,
}

func voiceNorm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer(" ", "", "　", "", "、", "", "。", "", ",", "", ".", "", "！", "", "？", "", "!", "", "?", "").Replace(s)
}

// 声で言われたことがアプリのコマンドなら、/ コマンドにして返す (confirm: 実行前に確認が要る)。
// コマンドでなければ ok=false
func parseVoiceCommand(text string) (cmd string, confirm, ok bool) {
	t := voiceNorm(text)
	// 決まった言い方だけのもの (「ミュート」など、前から)
	for _, c := range []string{"mute", "cls", "mic", "exit"} {
		for _, w := range TList("voice.cmd." + c) {
			if t == voiceNorm(w) {
				return "/" + c, false, true
			}
		}
	}
	// 「コマンド …」「スラッシュ …」
	rest, found := "", false
	for _, p := range TList("voice.cmdPrefix") {
		if r, ok := strings.CutPrefix(t, voiceNorm(p)); ok {
			rest, found = r, true
			break
		}
	}
	if !found || rest == "" {
		return "", false, false
	}
	// いちばん長く一致する呼び名を探す (「画面」より「画面クリア」を先に)
	best, bestLen := "", 0
	for _, c := range voiceCommands {
		names := append(TList("voice.name."+c), c)
		for _, n := range names {
			if n = voiceNorm(n); n != "" && strings.HasPrefix(rest, n) && len(n) > bestLen {
				best, bestLen = c, len(n)
			}
		}
	}
	// 自作コマンド (プラグイン) は名前そのままで
	plugin := false
	for _, p := range listPlugins() {
		if n := voiceNorm(p.Name); strings.HasPrefix(rest, n) && len(n) > bestLen {
			best, bestLen, plugin = p.Name, len(n), true
		}
	}
	if best == "" {
		return "", false, false
	}
	arg := voiceArg(rest[bestLen:], text)
	cmd = strings.TrimSpace("/" + best + " " + arg)
	return cmd, plugin || voiceConfirm[best], true
}

// 引数: 「オン」「オフ」「一覧」などの決まった言葉ならその英語に、ほかは言ったまま (/screen の質問など)
func voiceArg(rest, original string) string {
	if rest == "" {
		return ""
	}
	for _, a := range []string{"on", "off", "list", "open", "move", "reset"} {
		for _, w := range append(TList("voice.arg."+a), a) {
			if rest == voiceNorm(w) {
				return a
			}
		}
	}
	// 元の文から、引数にあたる部分を (間の空白や句読点を残して) 取り出す:
	// 空白・句読点でない文字だけを数えて、後ろから rest の文字数ぶんのところから
	r := []rune(original)
	var kept []int
	for i, c := range r {
		if voiceNorm(string(c)) != "" {
			kept = append(kept, i)
		}
	}
	n := len([]rune(rest))
	if n > len(kept) {
		return rest
	}
	return strings.Trim(string(r[kept[len(kept)-n]:]), " 　、。,.!?！？")
}

// 声で聞き取った文を、コマンドならコマンドとして (要るなら確認してから)、そうでなければ話しかけとして渡す
func (l *Lumi) submitVoice(text string) {
	cmd, confirm, ok := parseVoiceCommand(text)
	if !ok {
		l.submit(text)
		return
	}
	if !confirm {
		l.submit(cmd)
		return
	}
	go func() {
		if !l.begin() {
			return
		}
		answer := strings.ToLower(l.ask(T("voice.cmdConfirm", cmd)))
		l.setBusy(false)
		if answer == "y" || answer == "a" {
			l.submit(cmd)
		} else {
			l.info(T("tool.notRun"))
		}
	}()
}

// 前からの呼び方 (テスト用): 決まった言い方・コマンドなら / コマンドを、そうでなければそのまま返す
func voiceCommand(text string) string {
	if cmd, _, ok := parseVoiceCommand(text); ok {
		return cmd
	}
	return text
}
