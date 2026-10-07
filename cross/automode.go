package main

// 自動モード: AI が提案したコマンドを、確認せずにそのまま実行する (Claude Code の自動モードのように)。
//   /auto [on|off]  か  Shift+Tab で切り替え (設定 auto_run。最初はオフ)
// 管理者権限が必要なコマンドと、取り消せない・危ない操作 (一括削除・フォーマット・シャットダウン・
// ダウンロードしたものの実行など) は、自動モードでもこれまでどおり確認する。

import (
	"regexp"
	"strings"
)

func (l *Lumi) autoRun() bool { return l.s.Get("auto_run", "off") == "on" }

// /auto [on|off] (何も付けなければ切り替え)
func (l *Lumi) autoCommand(arg string) {
	on := !l.autoRun()
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on":
		on = true
	case "off":
		on = false
	}
	if l.s.Err == nil {
		v := "off"
		if on {
			v = "on"
		}
		l.s.Set("auto_run", v)
	}
	if on {
		l.info(T("auto.on"))
	} else {
		l.info(T("auto.off"))
	}
	l.updateTitle()
	l.emit("autoMode", on)
}

// 自動モードでも確認する、危ない・取り消せない操作 (大文字・小文字は区別しない)
var riskyPatterns = []*regexp.Regexp{
	// 消す
	regexp.MustCompile(`(?i)\bremove-item\b.*-r(ecurse)?\b`),
	regexp.MustCompile(`(?i)\b(rm|ri|del|erase|rd|rmdir)\b.*(\s-[a-z]*r[a-z]*\b|/s\b|-recurse\b)`),
	regexp.MustCompile(`(?i)\b(remove-item|rm|del)\b.*[\\/:]\*`), // ワイルドカードでまとめて
	regexp.MustCompile(`(?i)\bclear-recyclebin\b`),
	regexp.MustCompile(`(?i)\bcipher\b.*/w`),
	// ディスク
	regexp.MustCompile(`(?i)\b(format|format-volume|diskpart|clear-disk|initialize-disk|remove-partition|mkfs(\.\w+)?|fdisk|parted|wipefs)\b`),
	regexp.MustCompile(`(?i)\bdd\b.*\bof=`),
	// 電源
	regexp.MustCompile(`(?i)\b(shutdown|stop-computer|restart-computer|reboot|poweroff|halt)\b`),
	// システムの設定
	regexp.MustCompile(`(?i)\breg(\.exe)?\s+(delete|add|import)\b`),
	regexp.MustCompile(`(?i)\b(remove-itemproperty|set-itemproperty|new-itemproperty)\b.*\bhk(lm|cu|cr|u)\b`),
	regexp.MustCompile(`(?i)\b(set-executionpolicy|bcdedit|vssadmin|takeown|icacls|cacls|netsh\s+advfirewall|set-mppreference|add-mppreference)\b`),
	regexp.MustCompile(`(?i)\b(net\s+user|net\s+localgroup|new-localuser|remove-localuser|add-localgroupmember)\b`),
	regexp.MustCompile(`(?i)\b(uninstall-package|remove-appxpackage|winget\s+uninstall|msiexec\s+/x)\b`),
	regexp.MustCompile(`(?i)\b(systemctl|launchctl)\s+(stop|disable|mask|unload|remove|bootout)\b`),
	regexp.MustCompile(`(?i)\b(chmod|chown)\s+-[a-z]*r`),
	regexp.MustCompile(`(?i)\bsudo\b|\bsu\s|\bpkexec\b|\brunas\b|-verb\s+runas`),
	// ダウンロードしたものをそのまま実行する
	regexp.MustCompile(`(?i)\b(iex|invoke-expression)\b`),
	regexp.MustCompile(`(?i)\b(curl|wget|iwr|irm|invoke-webrequest|invoke-restmethod)\b.*\|\s*(sh|bash|zsh|iex|powershell|pwsh|python)\b`),
	regexp.MustCompile(`(?i)-encodedcommand\b|\s-enc\s`),
	// 動いているものを強制的に止める (保存していない作業が消える)
	regexp.MustCompile(`(?i)\b(stop-process|kill|taskkill|pkill|killall)\b`),
	// フォークボム
	regexp.MustCompile(`:\(\)\s*\{`),
}

// 自動モードでも確認したほうがよいコマンドか
func riskyCommand(cmd string) bool {
	for _, re := range riskyPatterns {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}
