//go:build manual

package main

import "testing"

// 今の指示文を見る: go test -tags manual -run ShowPrompt -v
func TestShowPrompt(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	s := &Settings{vals: map[string]any{"provider": "local"}}
	p := s.SystemPrompt()
	t.Logf("%d chars\n%s", len([]rune(p)), p)
}
