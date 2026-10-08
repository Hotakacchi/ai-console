//go:build manual

package main

import "testing"

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
