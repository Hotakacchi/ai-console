package main

// この PC に合ったローカルAIを選ぶ: メモリと GPU のメモリを調べて、重すぎないモデルをすすめる。
// インストーラーの「ローカルAIもダウンロードする」と、名前なしの /install-local は、おすすめのものを入れる。

import "fmt"

const gb = 1 << 30

type pcSpec struct {
	RAM     uint64 // メモリ (0 は分からない)
	VRAM    uint64 // いちばん大きい GPU 専用のメモリ (0 は GPU なし・分からない)
	GPU     string // その GPU の名前
	Unified bool   // メモリを CPU と GPU で分け合う (Apple シリコンの Mac)
}

// 機械を調べるのは 1 回だけ (変わらないので)
var cachedSpec *pcSpec

func thisPC() pcSpec {
	if cachedSpec == nil {
		s := detectSpec()
		cachedSpec = &s
	}
	return *cachedSpec
}

// AI に使える GPU のメモリ (Apple シリコンはメモリの 2/3 ほどを GPU に使える)
func (s pcSpec) gpuMemory() uint64 {
	if s.Unified {
		return s.RAM / 3 * 2
	}
	return s.VRAM
}

// おすすめのモデル (分からないときは標準)
func recommendLocalModel(s pcSpec) localModel {
	pick := func(id string) localModel { m, _ := findLocalModel(id); return m }
	g := s.gpuMemory()
	switch {
	case s.RAM == 0:
		return defaultLocalModel()
	// (メモリを分け合う Mac は、OS やほかのアプリの分も要るので 24GB から)
	case g >= 8*gb && s.RAM >= 16*gb && (!s.Unified || s.RAM >= 24*gb):
		return pick("qwen3.5-9b")
	case g >= 4*gb || s.RAM >= 16*gb:
		return pick("qwen3.5-4b")
	case s.RAM >= 8*gb:
		return pick("qwen3.5-2b")
	}
	return pick("qwen3.5-0.8b")
}

// 「メモリ 32GB、GPU NVIDIA GeForce RTX 5070 Ti (16GB)」のような説明
func (s pcSpec) describe() string {
	if s.RAM == 0 {
		return T("spec.unknown")
	}
	ram := fmt.Sprintf("%.0fGB", float64(s.RAM)/gb)
	switch {
	case s.Unified:
		return T("spec.unified", ram)
	case s.VRAM > 0:
		return T("spec.gpu", ram, s.GPU, fmt.Sprintf("%.0fGB", float64(s.VRAM)/gb))
	}
	return T("spec.noGPU", ram)
}
