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
	// 説明文がそろっていて、画像の部品のファイル名がかぶらない
	loadLocales()
	setLanguage("ja")
	visions := map[string]string{}
	for _, m := range localModels {
		if key := "local.note." + m.Note; T(key) == key {
			t.Errorf("%s: no %s", m.ID, key)
		}
		if m.Vision != nil {
			if other, dup := visions[m.visionFile()]; dup {
				t.Errorf("%s and %s share %s", m.ID, other, m.visionFile())
			}
			visions[m.visionFile()] = m.ID
		}
	}
	// おすすめ (インストーラーで入れるもの) は Qwen3.5 からだけ
	for _, spec := range []pcSpec{{RAM: 2 * gb}, {RAM: 8 * gb}, {RAM: 16 * gb}, {RAM: 64 * gb, VRAM: 24 * gb}, {RAM: 64 * gb, Unified: true}} {
		if m := recommendLocalModel(spec); m.Family != "qwen" {
			t.Errorf("%+v recommends %s", spec, m.ID)
		}
	}

	s := &Settings{vals: map[string]any{"model": "my-own.gguf"}}
	if _, ok := currentLocalModel(s); ok {
		t.Error("an unlisted file should not match")
	}
}
