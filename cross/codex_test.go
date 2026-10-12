package main

import (
	"slices"
	"strings"
	"testing"
)

func TestReadCodexStream(t *testing.T) {
	loadLocales()
	in := `{"type":"thread.started","thread_id":"x"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"考え中"}}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"調べますね。"}}
{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"<run>Get-PSDrive C</run>"}}
{"type":"turn.completed","usage":{"input_tokens":1}}
`
	var got strings.Builder
	reply, failure := readCodexStream(strings.NewReader(in), func(s string) { got.WriteString(s) })
	if reply != "調べますね。\n\n<run>Get-PSDrive C</run>" || got.String() != reply || failure != "" {
		t.Errorf("reply %q got %q failure %q", reply, got.String(), failure)
	}
	_, failure = readCodexStream(strings.NewReader(`{"type":"turn.failed","error":{"message":"You've hit your usage limit."}}`+"\n"), func(string) {})
	if !strings.Contains(codexFailure(failure), "usage limit") {
		t.Errorf("limit: %q", codexFailure(failure))
	}
	if codexFailure("Not logged in. Run codex login") != T("codex.login") {
		t.Error("login")
	}
}

func TestCodexArgs(t *testing.T) {
	args := codexArgs(`C:\tmp\work`, `C:\tmp\instructions.md`, "gpt-5", []string{`C:\tmp\image1.jpg`})
	for _, want := range []string{"exec", "--json", "--ephemeral", "--ignore-user-config", "features.computer_use=false", "--image", "-m"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}
	if i := slices.Index(args, "--sandbox"); i < 0 || args[i+1] != "read-only" {
		t.Errorf("sandbox: %q", args)
	}
	if !slices.Contains(args, `model_instructions_file="C:/tmp/instructions.md"`) {
		t.Errorf("instructions: %q", args)
	}
	if args[len(args)-1] != "-" {
		t.Errorf("発言は stdin から: %q", args)
	}
}
