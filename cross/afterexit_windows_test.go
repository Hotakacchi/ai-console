package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// 更新のインストーラーは、ルミが終わってから動く (窓は出さない。ping が終わってから cmd がファイルを書くのを確かめる)
func TestAfterExit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "it's a dir")
	os.MkdirAll(dir, 0o755)
	out := filepath.Join(dir, "ok.txt")
	ping := exec.Command("ping", "-n", "4", "127.0.0.1")
	hideWindow(ping)
	if err := ping.Start(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	ps := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command",
		afterExitScript(ping.Process.Pid, "cmd.exe", []string{"/c", "echo ok>\"" + out + "\""}))
	hideWindow(ps)
	if b, err := ps.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	ping.Wait()
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(out); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal("not written:", err)
	}
	t.Logf("written after %v: %q", time.Since(start), b)
	if time.Since(start) < 2500*time.Millisecond {
		t.Fatal("did not wait for the process")
	}
}
