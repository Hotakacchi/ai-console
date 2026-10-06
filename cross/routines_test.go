package main

import "testing"

func TestParseRoutines(t *testing.T) {
	loadLocales()
	for _, lang := range []string{"ja", "en"} {
		setLanguage(lang)
		list := parseRoutines(T("routines.template"))
		if len(list) != 2 {
			t.Fatalf("%s: %d routines", lang, len(list))
		}
		kinds := ""
		for _, s := range list[0].Steps {
			kinds += s.Kind + " "
		}
		if kinds != "weather reminders ai " || list[0].Steps[2].Arg == "" {
			t.Errorf("%s: steps %q", lang, kinds)
		}
	}
	list := parseRoutines("[仕事モード @7:05]\n開く：https://example.com\nrun: echo hi\nSay: やあ\nこんにちは\n[x @25:99]\n")
	r := list[0]
	if r.Name != "仕事モード" || r.At != "07:05" || len(r.Steps) != 4 {
		t.Fatalf("%+v", r)
	}
	want := []routineStep{{"open", "https://example.com"}, {"run", "echo hi"}, {"say", "やあ"}, {"say", "こんにちは"}}
	for i, s := range r.Steps {
		if s != want[i] {
			t.Errorf("step %d: %+v", i, s)
		}
	}
	if list[1].At != "" {
		t.Errorf("bad time accepted: %q", list[1].At)
	}
	if routineKey("おはよう！") != routineKey("おはよう") || routineKey("Good Morning.") != routineKey("good morning") {
		t.Error("routineKey")
	}
}
