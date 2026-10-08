//go:build manual

package main

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"time"
)

// whisper.cpp を一時フォルダに入れて書き起こす: WHISPER_WAV=音声.wav go test -tags manual -run WhisperCpp -v -timeout 20m
// (WAV は 16kHz・16bit・モノラル)
func TestWhisperCpp(t *testing.T) {
	loadLocales()
	base := tempDataDir(t)
	model := "whisper-base"
	if m := os.Getenv("WHISPER_MODEL"); m != "" {
		model = m
	}
	start := time.Now()
	if err := installWhisperCpp(base, model, func(string, float64) {}, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	if !whisperCppInstalled(base, model) {
		t.Fatal("not installed")
	}
	t.Logf("installed in %s", time.Since(start).Round(time.Second))

	wav, err := os.ReadFile(os.Getenv("WHISPER_WAV"))
	if err != nil {
		t.Skip("WHISPER_WAV is not set")
	}
	// data の部分 (16bit の音) を取り出す
	pcm := []byte{}
	for off := 12; off+8 <= len(wav); {
		size := int(binary.LittleEndian.Uint32(wav[off+4:]))
		if string(wav[off:off+4]) == "data" {
			pcm = wav[off+8 : off+8+size]
			break
		}
		off += 8 + size
	}
	for _, prompt := range []string{"", "会議 リマインダー ルミ"} {
		start = time.Now()
		text, err := transcribeCpp(base, model, pcm, "ja", prompt)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s (prompt %q): %q", time.Since(start).Round(time.Millisecond), prompt, text)
		if !strings.Contains(text, "天気") {
			t.Error("unexpected text")
		}
	}
}
