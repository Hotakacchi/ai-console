//go:build manual

package main

import "testing"

// 天気を実際に取ってくる: go test -tags manual -run Weather -v
func TestWeather(t *testing.T) {
	loadLocales()
	for _, lang := range []string{"ja", "en"} {
		setLanguage(lang)
		for _, loc := range []string{"", "Tokyo"} {
			l := &Lumi{s: &Settings{vals: map[string]any{"weather_location": loc}}}
			w, err := l.weather()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s %q: %s", lang, loc, w)
		}
	}
}
