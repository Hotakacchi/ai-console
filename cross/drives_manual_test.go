//go:build manual

package main

import (
	"testing"
	"time"
)

// つながっているドライブと名前: go test -tags manual -run TestDrives -v
func TestDrives(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	for _, d := range listDrives() {
		t.Log(T("drive.added", driveName(d), d))
	}
}

// 電源の状態: go test -tags manual -run TestPower -v
func TestPower(t *testing.T) {
	on, ok := onACPower()
	t.Logf("AC power: %v (has battery: %v)", on, ok)
}

// この PC の性能とおすすめのモデル: go test -tags manual -run TestThisPC -v
func TestThisPC(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	s := thisPC()
	t.Logf("%+v → %s (%s)", s, recommendLocalModel(s).Name, s.describe())
}

// 見守りで測る値: go test -tags manual -run TestWatchValues -v
func TestWatchValues(t *testing.T) {
	a := cpuTimes()
	time.Sleep(2 * time.Second)
	cpu, ok := cpuPercent(a, cpuTimes())
	t.Logf("cpu %.1f%% (%v)", cpu, ok)
	mem, ok := memoryPercent()
	t.Logf("memory %.1f%% (%v)", mem, ok)
	free, total, ok := diskSpace()
	t.Logf("disk %s free %.1fGB / %.1fGB (%v)", systemDrive(), float64(free)/gb, float64(total)/gb, ok)
	pct, charging, ok := batteryLevel()
	t.Logf("battery %d%% charging=%v (%v)", pct, charging, ok)
	start := time.Now()
	name, size := topMemoryProcess()
	t.Logf("top memory: %s %.2fGB (%s)", name, float64(size)/gb, time.Since(start).Round(time.Millisecond))
}
