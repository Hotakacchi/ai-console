package main

import "testing"

func TestStopOnlyText(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	defer setLanguage("auto")
	for text, want := range map[string]bool{
		"ルミ、止めて": true, "止めて": true, "ねえルミ、ストップ": true, "キャンセル": true, "もういいよ。": true,
		"音楽を止めて": false, "PCを止めて": false, "シャットダウンして": false, "止めてから再生して": false, "": false,
	} {
		if got := stopOnlyText(text); got != want {
			t.Errorf("%q: got %v", text, got)
		}
	}
}
