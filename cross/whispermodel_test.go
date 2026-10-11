package main

import (
	"runtime"
	"testing"
)

func TestDedicatedGPU(t *testing.T) {
	for _, c := range []struct {
		spec pcSpec
		want bool
	}{
		{pcSpec{RAM: 32 * gb, VRAM: 16 * gb, GPU: "NVIDIA GeForce RTX 5070 Ti"}, true},
		{pcSpec{RAM: 32 * gb, VRAM: 8 * gb, GPU: "AMD Radeon RX 7600"}, true},
		{pcSpec{RAM: 16 * gb, VRAM: 2 * gb, GPU: "NVIDIA GeForce MX250"}, false}, // メモリが少ない
		{pcSpec{RAM: 32 * gb, VRAM: 4 * gb, GPU: "AMD Radeon(TM) Graphics"}, false},
		{pcSpec{RAM: 16 * gb, VRAM: 4 * gb, GPU: "Intel(R) Arc(TM) Graphics"}, false},
		{pcSpec{RAM: 32 * gb, GPU: "Apple M3", Unified: true}, false},
		{pcSpec{}, false},
	} {
		if got := c.spec.dedicatedGPU(); got != c.want {
			t.Errorf("%+v: got %v", c.spec, got)
		}
	}
}

func TestRecommendWhisper(t *testing.T) {
	gpu := pcSpec{RAM: 32 * gb, VRAM: 16 * gb, GPU: "NVIDIA GeForce RTX 5070 Ti"}
	small := pcSpec{RAM: 16 * gb}
	tiny := pcSpec{RAM: 4 * gb}
	want := map[string][3]string{
		"windows": {"whisper-turbo", "whisper-small", "whisper-base"},
		"linux":   {"whisper-small", "whisper-small", "whisper-base"},
		"darwin":  {"whisper-base", "whisper-base", "whisper-base"},
	}[runtime.GOOS]
	if runtime.GOOS == "windows" && runtime.GOARCH != "amd64" {
		want = [3]string{"whisper-base", "whisper-base", "whisper-base"}
	}
	got := [3]string{recommendWhisper(gpu), recommendWhisper(small), recommendWhisper(tiny)}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	for _, m := range got {
		if _, ok := whisperCppModels[m]; !ok {
			t.Errorf("no model %s", m)
		}
	}
}
