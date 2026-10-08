//go:build manual

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// モデルの生の返事を見る (調べもの用): LUMI_TEMP=1 LUMI_MODEL=llm-jp-1.8b go test -tags manual -run LocalRaw -v -timeout 30m
func TestLocalRaw(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	if os.Getenv("LUMI_TEMP") != "" {
		tempDataDir(t)
	}
	m, ok := findLocalModel(os.Getenv("LUMI_MODEL"))
	if !ok {
		t.Fatal("no model")
	}
	base := dataDir()
	if err := installLocal(base, m, func(string, float64) {}, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	defer localServer.Stop()
	if err := localServer.Start(base, localModelPath(base, m.ID), "", true, 8192); err != nil {
		t.Fatal(err)
	}
	for _, sys := range []string{"", (&Settings{vals: map[string]any{"provider": "local", "model": m.ID}}).SystemPrompt()} {
		msgs := []map[string]string{}
		if sys != "" {
			msgs = append(msgs, map[string]string{"role": "system", "content": sys})
		}
		msgs = append(msgs, map[string]string{"role": "user", "content": "こんにちは！自己紹介して"})
		body, _ := json.Marshal(map[string]any{"messages": msgs, "max_tokens": 4000, "temperature": 0.7})
		req, _ := http.NewRequest("POST", localServer.Endpoint()+"/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+localServer.APIKey())
		req.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(r.Body)
		r.Body.Close()
		t.Logf("system prompt %d chars → %s", len(sys), out)
	}
	// ルミと同じ送り方 (ストリーミング) で、届いたものを数える
	if os.Getenv("LUMI_STREAM") != "" {
		s := &Settings{vals: map[string]any{"provider": "local", "model": m.ID}}
		p := newLocal(s).(*thinkFiltered)
		body := map[string]any{"model": p.model, "stream": true, "messages": []Message{{"role": "system", "content": s.SystemPrompt()}, {"role": "user", "content": "こんにちは！自己紹介して"}}}
		p.addOptions(body)
		kinds := map[string]int{}
		var content strings.Builder
		var last map[string]any
		err := postStream(context.Background(), localServer.Endpoint()+"/v1/chat/completions", map[string]string{"Authorization": "Bearer " + localServer.APIKey()}, body, func(ev map[string]any) error {
			last = ev
			choices, _ := ev["choices"].([]any)
			if len(choices) == 0 {
				kinds["no choices"]++
				return nil
			}
			d := obj(choices[0].(map[string]any), "delta")
			for k := range d {
				kinds[k]++
			}
			content.WriteString(str(d, "content"))
			return nil
		})
		t.Logf("stream err=%v kinds=%v content=%q last=%v", err, kinds, content.String(), last)
	}
}

// thinkFiltered を通す前の文字を見る: LUMI_MODEL=… go test -tags manual -run LocalBeforeFilter -v
func TestLocalBeforeFilter(t *testing.T) {
	loadLocales()
	setLanguage("ja")
	m, _ := findLocalModel(os.Getenv("LUMI_MODEL"))
	s := &Settings{vals: map[string]any{"provider": "local", "model": m.ID}}
	p := newLocal(s).(*thinkFiltered)
	defer localServer.Stop()
	var raw, filtered strings.Builder
	p.openAIProvider.Reply(Turn{Text: "こんにちは！自己紹介して"}, func(s string) { raw.WriteString(s) })
	p.Clear()
	p.Reply(Turn{Text: "こんにちは！自己紹介して"}, func(s string) { filtered.WriteString(s) })
	t.Logf("raw=%q\nfiltered=%q", raw.String(), filtered.String())
}
