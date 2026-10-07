package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 同じフォルダか (C:\Users\RUNNER~1 のような短い名前でも)
func sameDir(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

func runLines(t *testing.T, command, dir string) ([]string, string) {
	t.Helper()
	var lines []string
	newDir, err := runShell(command, dir, make(chan struct{}), func(s string) { lines = append(lines, s) })
	if err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	return lines, newDir
}

func TestRunShell(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)

	// 日本語の出力と、ふつうのコマンド (cmd.exe) の出力
	// (古いコマンドの日本語は OS の言語しだいなので、ここでは英数字で確かめる)
	cmd := "echo こんにちは; echo bye"
	if runtime.GOOS == "windows" {
		cmd = "Write-Output 'こんにちは'; cmd /c echo bye"
	}
	lines, newDir := runLines(t, cmd, dir)
	got := strings.Join(lines, "|")
	if !strings.Contains(got, "こんにちは") || !strings.Contains(got, "bye") {
		t.Errorf("output %q", got)
	}
	if !sameDir(newDir, dir) {
		t.Errorf("dir %q, want %q", newDir, dir)
	}

	// cd した場所が返る
	_, newDir = runLines(t, "cd sub", dir)
	if !sameDir(newDir, filepath.Join(dir, "sub")) {
		t.Errorf("after cd: %q", newDir)
	}

	// 引用符や記号を含むコマンドも、そのまま届く
	quoted := `echo "a  b"; echo 'x"y'; echo "$((1+2))"`
	wantQ := []string{"a  b", `x"y`, "3"}
	if runtime.GOOS == "windows" {
		quoted = `Write-Output "a  b"; Write-Output 'x"y'; Write-Output "$(1+2)"; Write-Output "100%"`
		wantQ = []string{"a  b", `x"y`, "3", "100%"}
	}
	lines, _ = runLines(t, quoted, dir)
	for _, w := range wantQ {
		found := false
		for _, l := range lines {
			if l == w {
				found = true
			}
		}
		if !found {
			t.Errorf("%q not in %q", w, lines)
		}
	}

	// 失敗するコマンドでも出力 (エラー) は受け取れる
	lines, _ = runLines(t, "nosuchcommand_lumi", dir)
	if len(lines) == 0 {
		t.Error("no error output")
	}
}

// Ctrl+C で、コマンドから起動されたプロセスごと止まる
func TestRunShellCancel(t *testing.T) {
	cmd := "sleep 30"
	if runtime.GOOS == "windows" {
		cmd = "ping -n 30 127.0.0.1"
	}
	cancel := make(chan struct{})
	first := make(chan struct{}, 1)
	go func() {
		<-first
		close(cancel)
	}()
	start := time.Now()
	if runtime.GOOS != "windows" {
		close(first) // sleep は何も出さないので、すぐ止める
	}
	runShell(cmd, t.TempDir(), cancel, func(string) {
		select {
		case first <- struct{}{}:
		default:
		}
	})
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("took %v to stop", d)
	}
	if runtime.GOOS == "windows" {
		out, _ := exec.Command("tasklist", "/FI", "IMAGENAME eq PING.EXE").Output()
		if strings.Contains(strings.ToLower(string(out)), "ping.exe") {
			t.Error("ping is still running")
		}
	}
}
