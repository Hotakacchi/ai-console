package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTerminalTabs(t *testing.T) {
	loadLocales()
	l := &Lumi{s: &Settings{vals: map[string]any{}}}
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	l.shellDir = dir

	l.tabOpen(7, "")
	tab := getTab(7)
	if tab == nil || !sameDir(tab.dir, dir) {
		t.Fatalf("tab %+v", tab)
	}
	wait := func() {
		for i := 0; i < 200; i++ {
			termTabsMu.Lock()
			running := tab.running
			termTabsMu.Unlock()
			if !running {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("still running")
	}
	l.tabSubmit(7, "cd sub")
	wait()
	if !sameDir(tab.dir, filepath.Join(dir, "sub")) {
		t.Errorf("dir %q", tab.dir)
	}
	// 別のタブは、別の場所のまま
	l.tabOpen(8, "")
	if !sameDir(getTab(8).dir, dir) {
		t.Error("tabs share the directory")
	}
	l.tabSubmit(7, "exit")
	if getTab(7) != nil {
		t.Error("exit didn't close the tab")
	}
	l.tabClose(8)
	if getTab(8) != nil {
		t.Error("close didn't remove the tab")
	}
}
