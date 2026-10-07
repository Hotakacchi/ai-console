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
