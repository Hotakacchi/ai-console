package main

// 自作コマンド (プラグイン): データフォルダの plugins に置いたスクリプトが、ファイル名で /名前 のコマンドになる。
//   例: plugins/backup.ps1 → /backup (続けて書いたものは引数として渡す)
// 使えるもの: .ps1 (PowerShell) / .bat .cmd / .exe (Windows)、.sh と実行できるファイル (Mac・Linux)、.py (Python)、.js (Node.js)。
// ファイルの最初のコメント行が説明になり、/help・/plugins に出る。AI も <plugin name="名前">引数</plugin> で使える (実行前に確認する)。
// 実行する場所は、シェルモードと同じ今いるフォルダ。

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
)

type plugin struct {
	Name string // コマンドの名前 (/ なし、小文字)
	Path string
	Desc string // 最初のコメント行
}

func pluginsDir() string { return filepath.Join(dataDir(), "plugins") }

// plugins フォルダのプラグイン (名前順)。組み込みのコマンドと同じ名前のものは使わない
func listPlugins() []plugin {
	entries, err := os.ReadDir(pluginsDir())
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, c := range commands {
		seen[strings.TrimPrefix(c.name, "/")] = true
	}
	var out []plugin
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		path := filepath.Join(pluginsDir(), name)
		if _, ok := pluginArgv(path); !ok {
			continue
		}
		id := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		if id == "" || strings.ContainsAny(id, " \t") || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, plugin{Name: id, Path: path, Desc: pluginDesc(path)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func findPlugin(name string) (plugin, bool) {
	name = strings.ToLower(strings.TrimPrefix(name, "/"))
	for _, p := range listPlugins() {
		if p.Name == name {
			return p, true
		}
	}
	return plugin{}, false
}

// スクリプトを動かすためのコマンド (拡張子で決める)
func pluginArgv(path string) ([]string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	win := runtime.GOOS == "windows"
	switch ext {
	case ".ps1":
		exe := "pwsh"
		if _, err := exec.LookPath(exe); err != nil {
			if !win {
				return nil, false
			}
			exe = "powershell"
		}
		// Windows の標準 (Restricted) ではスクリプトが動かないので、自分で書いたもの (署名なし) は動く RemoteSigned にする。
		// 出力を UTF-8 にする入口 (ps1Runner) を通す (そのままだと、コンソールの文字コードで日本語が化ける)
		return []string{exe, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "RemoteSigned", "-File", ps1Runner(), path}, true
	case ".bat", ".cmd":
		return []string{"cmd.exe", "/d", "/c", path}, win
	case ".exe":
		return []string{path}, win
	case ".py":
		for _, exe := range []string{"python3", "python", "py"} {
			if _, err := exec.LookPath(exe); err == nil {
				return []string{exe, path}, true
			}
		}
		return []string{"python", path}, true // 入っていなければ、動かしたときにそう出る
	case ".js", ".mjs":
		return []string{"node", path}, true
	case ".sh":
		return []string{"sh", path}, !win
	case "":
		// Mac・Linux: 実行できるファイル (#! で始まるスクリプトなど)
		if st, err := os.Stat(path); err == nil && !win && st.Mode()&0o111 != 0 {
			return []string{path}, true
		}
	}
	return nil, false
}

// .ps1 のプラグインを動かす入口 (データフォルダの plugin-run.ps1)。出力を UTF-8 にしてから、渡されたスクリプトを引数つきで動かす
func ps1Runner() string {
	path := filepath.Join(dataDir(), "plugin-run.ps1")
	body := "# Lumi: runs a plugin (.ps1) with UTF-8 output\r\n" +
		"[Console]::OutputEncoding = [Text.Encoding]::UTF8\r\n" +
		"$OutputEncoding = [Text.Encoding]::UTF8\r\n" +
		"$__script = $args[0]\r\n" +
		"$__rest = @($args | Select-Object -Skip 1)\r\n" +
		"& $__script @__rest\r\n"
	if b, err := os.ReadFile(path); err != nil || string(b) != body {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(body), 0o644)
	}
	return path
}

// 最初のコメント行 (#!… の行と空行は飛ばす)。コメントで始まらなければ説明なし
func pluginDesc(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; i < 20 && sc.Scan(); i++ {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#!") || strings.EqualFold(line, "@echo off") {
			continue
		}
		for _, mark := range []string{"#", "//", "::", "REM ", "rem ", "Rem "} {
			if rest, ok := strings.CutPrefix(line, mark); ok {
				return strings.TrimSpace(rest)
			}
		}
		return ""
	}
	return ""
}

// 引数を空白で分ける ("…" でくくったところは 1 つ)
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	quoted, has := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted, has = !quoted, true
		case (r == ' ' || r == '\t') && !quoted:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if has {
		out = append(out, cur.String())
	}
	return out
}

func (l *Lumi) pluginCmd(p plugin, args string) *exec.Cmd {
	argv, _ := pluginArgv(p.Path)
	argv = append(slices.Clone(argv), splitArgs(args)...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = l.currentDir()
	cmd.Env = append(os.Environ(), "LUMI_DATA="+dataDir(), "LUMI_LANG="+currentLang(),
		"PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "WSL_UTF8=1")
	return cmd
}

// /名前 引数: プラグインを動かして、出力をその場で出す (自分で打ったコマンドなので確認はしない)
func (l *Lumi) runPluginCommand(p plugin, args string) {
	if !l.begin() {
		return
	}
	go func() {
		label := strings.TrimSpace("/" + p.Name + " " + args)
		l.emit("running", map[string]any{"on": true, "cmd": label})
		lines := 0
		err := streamCmd(l.pluginCmd(p, args), l.cancelChan(), func(line string) {
			if lines++; lines <= maxShellLines {
				l.write(line+"\n", "fg")
			} else if lines == maxShellLines+1 {
				l.write(T("shell.tooMany", maxShellLines)+"\n", "dim")
			}
		})
		l.emit("running", map[string]any{"on": false})
		switch {
		case l.cancelled():
			l.write("^C\n", "dim")
		case err != nil:
			l.errorText(T("plugin.failed", p.Name, err.Error()))
		}
		l.write("\n", "fg")
		l.setBusy(false)
	}()
}

// AI の <plugin name="…">: 確認してから動かし、出力を返す
func (l *Lumi) pluginTool(r toolRequest) string {
	name := strings.ToLower(strings.TrimSpace(r.Attrs["name"]))
	args := strings.TrimSpace(r.Command)
	label := strings.TrimSpace("/" + name + " " + args)
	head := "\n[" + T("plugin.label") + "] " + label + "\n"
	p, ok := findPlugin(name)
	if !ok {
		return head + T("plugin.notFound", name) + "\n"
	}
	if !l.s.PcControl() {
		return head + T("tool.pcOff") + "\n"
	}
	if !l.autoRun() {
		if a := l.confirm(toolRequest{Kind: "run", Command: label}); a != "y" && a != "a" {
			l.write(T("tool.notRun")+"\n", "dim")
			return head + T("tool.declined") + "\n"
		}
	} else {
		l.write("\n"+T("auto.running")+"\n  "+label+"\n", "cyan")
	}
	l.emit("running", map[string]any{"on": true, "cmd": label})
	var out strings.Builder
	err := streamCmd(l.pluginCmd(p, args), l.cancelChan(), func(line string) { out.WriteString(line + "\n") })
	l.emit("running", map[string]any{"on": false})
	result := strings.TrimRight(out.String(), "\n")
	if err != nil {
		result += "\n" + T("plugin.failed", p.Name, err.Error())
	}
	l.showOutput(result)
	return head + l.fitOutput(result) + "\n"
}

// /plugins: 一覧を出す。open でフォルダを開く。フォルダがなければ見本を 1 つ入れて作る
func (l *Lumi) pluginsCommand(arg string) {
	dir := pluginsDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			l.errorText(err.Error())
			return
		}
		writeSamplePlugin(dir)
	}
	if strings.EqualFold(strings.TrimSpace(arg), "open") {
		openFolder(dir)
	}
	list := listPlugins()
	var b strings.Builder
	if len(list) == 0 {
		b.WriteString(T("plugin.none") + "\n")
	}
	for _, p := range list {
		b.WriteString("  " + padRight("/"+p.Name, 24) + p.Desc + "\n")
	}
	b.WriteString("\n  " + T("plugin.howto", dir))
	l.info(b.String())
	l.sendCommands() // 新しく置いたものも Tab で補えるように
}

// 見本: /hello [名前]
func writeSamplePlugin(dir string) {
	desc, hello := T("plugin.sampleDesc"), T("plugin.sampleHello")
	if runtime.GOOS == "windows" {
		// Windows PowerShell 5.1 でも日本語を読めるよう、BOM 付きの UTF-8 で書く
		body := "\ufeff# " + desc + "\r\n" +
			"$who = if ($args) { $args -join ' ' } else { 'Lumi' }\r\n" +
			"\"" + strings.ReplaceAll(hello, "%s", "$who") + "\"\r\n"
		os.WriteFile(filepath.Join(dir, "hello.ps1"), []byte(body), 0o644)
		return
	}
	body := "#!/bin/sh\n# " + desc + "\n" +
		"who=\"${*:-Lumi}\"\n" +
		"echo \"" + strings.ReplaceAll(hello, "%s", "$who") + "\"\n"
	os.WriteFile(filepath.Join(dir, "hello.sh"), []byte(body), 0o755)
}

// フォルダをファイルの一覧の画面 (エクスプローラー / Finder) で開く
func openFolder(dir string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	cmd.Start()
}

// 入力の Tab で補うコマンドの名前 (組み込み + プラグイン)
func (l *Lumi) sendCommands() {
	names := make([]string, 0, len(commands))
	for _, c := range commands {
		names = append(names, c.name)
	}
	for _, p := range listPlugins() {
		names = append(names, "/"+p.Name)
	}
	l.emit("commands", names)
}

// AI に渡す、プラグインの一覧 (なければ "")
func pluginsPrompt() string {
	list := listPlugins()
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(T("prompt.plugins"))
	for _, p := range list {
		b.WriteString("\n- " + p.Name)
		if p.Desc != "" {
			b.WriteString(": " + p.Desc)
		}
	}
	return b.String()
}
