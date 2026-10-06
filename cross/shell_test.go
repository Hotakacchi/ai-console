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

	// 日本語の出力 (PowerShell の文字と、古いコマンドの Shift_JIS の出力)
	cmd := "echo こんにちは"
	if runtime.GOOS == "windows" {
		cmd = "Write-Output 'こんにちは'; cmd /c echo さようなら"
	}
	lines, newDir := runLines(t, cmd, dir)
	got := strings.Join(lines, "|")
	if !strings.Contains(got, "こんにちは") || (runtime.GOOS == "windows" && !strings.Contains(got, "さようなら")) {
		t.Errorf("output %q", got)
	}
	if !strings.EqualFold(filepath.Clean(newDir), filepath.Clean(dir)) {
		t.Errorf("dir %q, want %q", newDir, dir)
	}

	// cd した場所が返る
	_, newDir = runLines(t, "cd sub", dir)
	if !strings.EqualFold(newDir, filepath.Join(dir, "sub")) {
		t.Errorf("after cd: %q", newDir)
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
