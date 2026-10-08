//go:build manual

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// 見張りの知らせが出るか: go test -tags manual -run ProcWatch -v -timeout 5m
// (CPU を使い続ける小さなプロセスを窓なしで動かし、1 分ほどで「使い続けている」、止めたら「終わりました」)
func TestProcWatch(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	tempDataDir(t)
	var out bytes.Buffer
	l := &Lumi{s: &Settings{vals: map[string]any{}}, answers: make(chan string, 1), spoken: make(chan int, 8)}
	l.cli = &cliUI{l: l, out: &out, lineStart: true}

	busy := exec.Command("powershell", "-NoProfile", "-Command", "while ($true) { }")
	hideWindow(busy)
	if err := busy.Start(); err != nil {
		t.Fatal(err)
	}
	defer busy.Process.Kill()
	time.Sleep(time.Second)

	t.Log(l.addWatch(fmt.Sprint(busy.Process.Pid)))
	t.Log(l.addWatch("no-such-app-xyz"))
	go l.runProcWatch()

	waitFor := func(what string, limit time.Duration) {
		start := time.Now()
		for time.Since(start) < limit {
			if strings.Contains(out.String(), what) {
				t.Logf("%q after %s", what, time.Since(start).Round(time.Second))
				return
			}
			time.Sleep(time.Second)
		}
		t.Errorf("no %q in:\n%s", what, out.String())
	}
	waitFor("CPU を使い続けています", 100*time.Second)
	busy.Process.Kill()
	waitFor("終わりました", 20*time.Second)
	t.Log("\n" + out.String())
}
