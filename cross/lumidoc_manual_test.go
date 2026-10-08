//go:build manual

package main

import (
	"os"
	"strings"
	"testing"
)

// 説明書から選ばれるところを見る: LUMI_Q=質問 go test -tags manual -run LumiDocPick -v
func TestLumiDocPick(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	md, _ := os.ReadFile("../README.md")
	t.Log("\n" + strings.Join(pickDoc(docItems(string(md)), os.Getenv("LUMI_Q"), 2500), "\n"))
}
