package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 見つかったすべてのシェルで、コマンドの出力と cd が受け取れる
func TestAllShells(t *testing.T) {
	loadLocales()
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	shells := availableShells()
	if len(shells) == 0 {
		t.Fatal("no shell found")
	}
	for _, sh := range shells {
		t.Run(sh.Kind, func(t *testing.T) {
			if sh.Kind == "wsl" && os.Getenv("LUMI_TEST_WSL") == "" {
				t.Skip("WSL は起動に時間がかかるので LUMI_TEST_WSL=1 のときだけ")
			}
			var lines []string
			newDir, err := runShellWith(sh, "cd sub\necho hello-lumi\necho こんにちは", dir, make(chan struct{}), func(s string) { lines = append(lines, s) })
			if err != nil {
				t.Fatal(err)
			}
			// cmd の日本語は Windows の言語しだい (英語版では ? になる) なので、cmd 以外で確かめる
			if j := strings.Join(lines, "|"); !strings.Contains(j, "hello-lumi") || (sh.Kind != "cmd" && !strings.Contains(j, "こんにちは")) {
				t.Errorf("output %q", lines)
			}
			if !sameDir(newDir, filepath.Join(dir, "sub")) {
				t.Errorf("dir %q", newDir)
			}
			t.Logf("%s (%s): prompt %q", sh.Label, sh.Exe, sh.prompt(newDir))
		})
	}
}

func TestPickShell(t *testing.T) {
	loadLocales()
	auto, ok := pickShell("auto")
	if !ok || auto.Exe == "" {
		t.Fatalf("auto: %+v", auto)
	}
	if _, ok := pickShell("no-such-shell"); ok {
		t.Error("unknown shell accepted")
	}
}
