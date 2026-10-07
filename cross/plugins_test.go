package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// テスト用のデータフォルダ (LOCALAPPDATA / HOME を一時フォルダに向ける)
func tempDataDir(t *testing.T) string {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	return dataDir()
}

func TestSplitArgs(t *testing.T) {
	got := splitArgs(`a  "b c" d"e f"  ""`)
	want := []string{"a", "b c", "de f", ""}
	if !slices.Equal(got, want) {
		t.Errorf("splitArgs = %q, want %q", got, want)
	}
}

func TestPluginTag(t *testing.T) {
	var got []toolRequest
	x := toolExtractor{OnText: func(string) {}, OnTag: func(r toolRequest) { got = append(got, r) }}
	for _, c := range []string{`使います。<plugin name="backup">D: `, `"my files"</plugin> と <plugin name="hello"/>。`} {
		x.Push(c)
	}
	x.Flush()
	if len(got) != 2 || got[0].Kind != "plugin" || got[0].Attrs["name"] != "backup" || got[0].Command != `D: "my files"` ||
		got[1].Attrs["name"] != "hello" || got[1].Command != "" {
		t.Errorf("tags = %+v", got)
	}
}

func TestPlugins(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	dir := tempDataDir(t)
	l := &Lumi{s: &Settings{vals: map[string]any{}}}
	l.shellDir = t.TempDir()
	// 最初の /plugins で、フォルダと見本 (/hello) ができる
	l.pluginsCommand("")
	list := listPlugins()
	if len(list) != 1 || list[0].Name != "hello" || list[0].Desc == "" {
		t.Fatalf("sample plugin: %+v (dir %s)", list, dir)
	}

	pdir := pluginsDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(pdir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("help.py", "# 組み込みの /help と同じ名前は使わない\n")
	write("notes.txt", "# 動かせないファイル\n")
	write("_hidden.py", "# _ で始まるものは使わない\n")
	write("Greet.py", "#!/usr/bin/env python3\n\n# あいさつ\nprint('hi')\n")
	names := []string{}
	for _, p := range listPlugins() {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"greet", "hello"}) {
		t.Errorf("plugins = %v", names)
	}
	if p, ok := findPlugin("/GREET"); !ok || p.Desc != "あいさつ" {
		t.Errorf("findPlugin = %+v %v", p, ok)
	}
	if s := pluginsPrompt(); !strings.Contains(s, "- greet: あいさつ") || !strings.Contains(s, "- hello: ") {
		t.Errorf("prompt: %s", s)
	}

	// 見本を引数つきで動かす
	if _, err := os.Stat(filepath.Join(pdir, map[bool]string{true: "hello.ps1", false: "hello.sh"}[runtime.GOOS == "windows"])); err != nil {
		t.Fatal(err)
	}
	hello, _ := findPlugin("hello")
	var out []string
	if err := streamCmd(l.pluginCmd(hello, `"ほたか くん"`), nil, func(s string) { out = append(out, s) }); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(out, "\n"); !strings.Contains(got, "こんにちは、ほたか くん さん！") {
		t.Errorf("hello output: %q", got)
	}
}
