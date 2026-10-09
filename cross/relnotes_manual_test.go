//go:build manual

package main

import "testing"

// GitHub から版ごとのリリースノートを取る: go test -tags manual -run ReleaseNotesFor -v
func TestReleaseNotesFor(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	for _, q := range []string{"ルミの v1.6.0 の新機能は？", "ルミの新機能は？", "ルミの使い方"} {
		notes, tags := releaseNotesFor(q)
		short := []rune(notes)
		if len(short) > 120 {
			short = short[:120]
		}
		t.Logf("%s → [%s] %d chars: %q", q, tags, len([]rune(notes)), string(short))
	}
}
