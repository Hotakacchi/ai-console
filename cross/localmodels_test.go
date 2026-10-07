package main

import (
	"path/filepath"
	"testing"
)

func TestLocalModels(t *testing.T) {
	ids := map[string]bool{}
	for _, m := range localModels {
		if ids[m.ID] || len(m.Asset.SHA256) != 64 || m.Asset.Size <= 0 || m.File == "" {
			t.Errorf("bad model %+v", m)
		}
		ids[m.ID] = true
		if m.Vision != nil && len(m.Vision.SHA256) != 64 {
			t.Errorf("bad vision %s", m.ID)
		}
	}
	// 標準モデルの画像の部品は、前からのファイル名のまま (入れ直さなくていいように)
	if f := defaultLocalModel().visionFile(); f != "mmproj-Qwen3.5-4B-F16.gguf" {
		t.Errorf("vision file %q", f)
	}
	for _, v := range []string{"qwen3.5-9b", "QWEN3.5-9B", "Qwen3.5-9B-Q4_K_M.gguf"} {
		if m, ok := findLocalModel(v); !ok || m.ID != "qwen3.5-9b" {
			t.Errorf("find %q", v)
		}
	}
	if p := localModelPath("/data", "phi-4-mini"); filepath.Base(p) != "Phi-4-mini-instruct-Q4_K_M.gguf" {
		t.Errorf("path %q", p)
	}
	if p := localModelPath("/data", ""); filepath.Base(p) != "Qwen3.5-4B-Q4_K_M.gguf" {
		t.Errorf("default path %q", p)
	}
	s := &Settings{vals: map[string]any{"model": "my-own.gguf"}}
	if _, ok := currentLocalModel(s); ok {
		t.Error("an unlisted file should not match")
	}
}
