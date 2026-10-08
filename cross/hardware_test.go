package main

import "testing"

func TestRecommendLocalModel(t *testing.T) {
	for _, c := range []struct {
		spec pcSpec
		want string
	}{
		{pcSpec{}, defaultLocalModelID}, // 分からなければ標準
		{pcSpec{RAM: 32 * gb, VRAM: 16 * gb}, "qwen3.5-9b"},
		{pcSpec{RAM: 16 * gb, VRAM: 8 * gb}, "qwen3.5-9b"},
		{pcSpec{RAM: 8 * gb, VRAM: 8 * gb}, "qwen3.5-4b"}, // GPU は大きいがメモリが少ない
		{pcSpec{RAM: 16 * gb}, "qwen3.5-4b"},              // GPU なしでもメモリが多ければ
		{pcSpec{RAM: 8 * gb, VRAM: 4 * gb}, "qwen3.5-4b"},
		{pcSpec{RAM: 8 * gb}, "qwen3.5-2b"},
		{pcSpec{RAM: 4 * gb}, "qwen3.5-0.8b"},
		{pcSpec{RAM: 16 * gb, Unified: true}, "qwen3.5-4b"}, // Mac (Apple シリコン) 16GB
		{pcSpec{RAM: 24 * gb, Unified: true}, "qwen3.5-9b"},
	} {
		if got := recommendLocalModel(c.spec).ID; got != c.want {
			t.Errorf("%+v: %s, want %s", c.spec, got, c.want)
		}
	}
}
