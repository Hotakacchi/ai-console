package main

// /shell と !コマンド で使うシェル。設定 shell が auto なら、その PC に合わせて選ぶ:
//   Windows: PowerShell 7 (pwsh) があればそれ、なければ Windows PowerShell
//   Mac・Linux: ログインシェル ($SHELL。zsh・bash・fish など)
// /shell list で見つかったシェルの一覧、/shell <名前> で切り替え。
// (AI が提案するコマンドは、これまでどおり PowerShell / zsh / bash で動かす)

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

type shellSpec struct {
	Kind  string // powershell / pwsh / cmd / gitbash / wsl / bash / zsh / fish / sh
	Exe   string
	Label string
}

var shellKinds = []string{"powershell", "pwsh", "cmd", "gitbash", "wsl", "bash", "zsh", "fish", "sh"}

// この PC で使えるシェル (使える順)
func availableShells() []shellSpec {
	var list []shellSpec
	add := func(kind, exe, label string) {
		if exe != "" {
			list = append(list, shellSpec{kind, exe, label})
		}
	}
	if runtime.GOOS == "windows" {
		add("pwsh", lookPath("pwsh.exe"), "PowerShell 7")
		add("powershell", "powershell.exe", "Windows PowerShell")
		add("cmd", "cmd.exe", T("shell.cmdLabel"))
		add("gitbash", gitBashPath(), "Git Bash")
		if wsl := lookPath("wsl.exe"); wsl != "" {
			add("wsl", wsl, "WSL")
		}
		return list
	}
	if login := filepath.Base(os.Getenv("SHELL")); login != "" {
		for _, k := range []string{"zsh", "bash", "fish", "sh"} {
			if login == k {
				add(k, lookPath(os.Getenv("SHELL")), k)
			}
		}
	}
	for _, k := range []string{"zsh", "bash", "fish", "sh"} {
		found := false
		for _, s := range list {
			found = found || s.Kind == k
		}
		if !found {
			add(k, lookPath(k), k)
		}
	}
	return list
}

func lookPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// Git for Windows の bash (C:\Windows\System32\bash.exe は WSL のものなので使わない)
func gitBashPath() string {
	var dirs []string
	if git := lookPath("git.exe"); git != "" {
		dirs = append(dirs, filepath.Join(filepath.Dir(filepath.Dir(git)), "bin"))
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if v := os.Getenv(env); v != "" {
			sub := "Git"
			if env == "LOCALAPPDATA" {
				sub = filepath.Join("Programs", "Git")
			}
			dirs = append(dirs, filepath.Join(v, sub, "bin"))
		}
	}
	for _, d := range dirs {
		p := filepath.Join(d, "bash.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// 設定 (auto か名前) に合うシェル。見つからなければ auto と同じ
func pickShell(want string) (shellSpec, bool) {
	list := availableShells()
	if len(list) == 0 {
		return shellSpec{"sh", "/bin/sh", "sh"}, false
	}
	want = strings.ToLower(strings.TrimSpace(want))
	if want != "" && want != "auto" {
		for _, s := range list {
			if s.Kind == want {
				return s, true
			}
		}
		return list[0], false
	}
	return list[0], true
}

// command を dir で動かし、最後に今いる場所 (cd のあと) を目印つきで出させる
func (s shellSpec) command(command, dir string) *exec.Cmd {
	sq := func(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }
	marker := func(pwd string) string { return "\nprintf '\\n" + cwdMarker + "%s\\n' " + pwd + "\n" }
	switch s.Kind {
	case "powershell", "pwsh":
		// 出力は UTF-8 のバイトで書き出させる (コンソールの文字コードに左右されないように)。
		// *>&1 で Write-Host やエラーも含めて受け取る。進み具合の表示は読めない形 (CLIXML) になるので出さない
		script := "$ErrorActionPreference = 'Continue'; $ProgressPreference = 'SilentlyContinue'\n" +
			"$__o = [Console]::OpenStandardOutput(); $__e = New-Object Text.UTF8Encoding $false\n" +
			"function __w($s) { $b = $__e.GetBytes([string]$s + \"`n\"); $__o.Write($b, 0, $b.Length); $__o.Flush() }\n" +
			"Set-Location -LiteralPath '" + strings.ReplaceAll(dir, "'", "''") + "' -ErrorAction SilentlyContinue\n" +
			"try { & {\n" + command + "\n} *>&1 | Out-String -Stream -Width 200 | ForEach-Object { __w $_ } } catch { __w $_ }\n" +
			"__w ('" + cwdMarker + "' + (Get-Location).Path)\n"
		return exec.Command(s.Exe, "-NoProfile", "-NonInteractive", "-Command", script)
	case "cmd":
		// !CD! は実行したあとの場所 (/v:on で、行を読んだ時点ではなく実行した時点の値になる)。
		// cmd は 1 行しか受け取れないので、複数行は & でつなぐ
		one := strings.Join(strings.FieldsFunc(command, func(r rune) bool { return r == '\n' || r == '\r' }), " & ")
		line := `"chcp 65001 >nul 2>&1 & cd /d "` + dir + `" & ` + one + ` & echo. & echo ` + cwdMarker + `!CD!"`
		cmd := exec.Command(s.Exe)
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		setCmdLine(cmd, `cmd.exe /d /v:on /s /c `+line)
		return cmd
	case "gitbash":
		return exec.Command(s.Exe, "-c", "cd "+sq(dir)+" 2>/dev/null\n"+command+marker(`"$(cygpath -w "$PWD")"`))
	case "wsl":
		return exec.Command(s.Exe, "--cd", dir, "-e", "sh", "-c", command+marker(`"$(wslpath -w "$PWD" 2>/dev/null || echo "$PWD")"`))
	}
	// bash / zsh / fish / sh
	return exec.Command(s.Exe, "-c", "cd "+sq(dir)+" 2>/dev/null\n"+command+marker(`"$PWD"`))
}

// そのシェルらしいプロンプト
func (s shellSpec) prompt(dir string) string {
	switch s.Kind {
	case "powershell", "pwsh":
		return "PS " + dir + "> "
	case "cmd":
		return dir + ">"
	}
	if home := homeDir(); dir == home || strings.HasPrefix(dir, home+string(os.PathSeparator)) {
		dir = "~" + strings.TrimPrefix(dir, home)
	}
	mark := "$"
	switch s.Kind {
	case "zsh":
		mark = "%"
	case "fish":
		mark = ">"
	}
	return s.Label + " " + dir + " " + mark + " "
}

// /shell list
func (l *Lumi) listShells() {
	cur, _ := pickShell(l.s.Get("shell", "auto"))
	var b strings.Builder
	for _, s := range availableShells() {
		mark := "  "
		if s.Kind == cur.Kind {
			mark = "* "
		}
		b.WriteString("  " + mark + padRight(s.Kind, 12) + padRight(s.Label, 20) + s.Exe + "\n")
	}
	b.WriteString("\n  " + T("shell.listHow"))
	l.info(b.String())
}
