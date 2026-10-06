package main

// より正確な音声認識: Whisper (transformers.js + ONNX Runtime の WebAssembly) を画面側で動かす。
// 呼びかけ (「ルミ」) と確認の言葉は今までどおり Vosk が聞き、話しかけた内容だけを Whisper で書き起こす。
// ここではランタイムとモデルをダウンロードして /voice/whisper/ で画面に渡す。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type whisperFile struct {
	group string // runtime か、モデル名 (whisper-base / whisper-small)
	path  string // 置き場所 (models/onnx-community/<モデル>/ からの相対、runtime は whisper/ 直下)
	asset asset
}

// URL・大きさ・SHA-256 は固定 (npm と Hugging Face の特定の版)
var whisperFiles = []whisperFile{
	{"runtime", "transformers.min.js", asset{"https://cdn.jsdelivr.net/npm/@huggingface/transformers@4.3.0/dist/transformers.min.js", 581935, "1475fd440e9932ab206682ee42cb18f6097403e9ee77ea62084c592d0f83597d"}},
	{"runtime", "ort-wasm-simd-threaded.asyncify.mjs", asset{"https://cdn.jsdelivr.net/npm/onnxruntime-web@1.31.0-dev.20260914-8d85527a0/dist/ort-wasm-simd-threaded.asyncify.mjs", 53057, "0966b6105cd936744498aa60df7a22cbd47af3374dbc64a9ab561c08a71e3611"}},
	{"runtime", "ort-wasm-simd-threaded.asyncify.wasm", asset{"https://cdn.jsdelivr.net/npm/onnxruntime-web@1.31.0-dev.20260914-8d85527a0/dist/ort-wasm-simd-threaded.asyncify.wasm", 26861777, "49871f5a4409519797e127440868a6d1923339d9185907f301a5b2a1d90af082"}},
	{"whisper-base", "added_tokens.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/added_tokens.json", 34604, "9715fd2243b6f06a5858b5e32950d2853f73dd5bc201aafcf76f5082a2d8acd1"}},
	{"whisper-base", "config.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/config.json", 2243, "f4d0608f7d918166da7edb3e188de5ef1bfe70d9802e785d271fd88111e9cf4b"}},
	{"whisper-base", "generation_config.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/generation_config.json", 3832, "61070cf8de25b1e9256e8e102ded49d8d24a8369ed36ef84fdf21549e68125a0"}},
	{"whisper-base", "normalizer.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/normalizer.json", 52666, "bf1c507dc8724ca9cf9903640dacfb69dae2f00edee4f21ceba106a7392f26dd"}},
	{"whisper-base", "onnx/decoder_model_merged_quantized.onnx", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/onnx/decoder_model_merged_quantized.onnx", 53693315, "fa3ef9902734ce5ae6f9ef2bdb2ba9a6c4b5785b09f4f420ce036573dc9d090b"}},
	{"whisper-base", "onnx/encoder_model_quantized.onnx", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/onnx/encoder_model_quantized.onnx", 23201314, "5862993336bf33acd23736071aae2b32261d3b1b2f37780194460d4ef974dd46"}},
	{"whisper-base", "preprocessor_config.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/preprocessor_config.json", 339, "a6a76d28c93edb273669eb9e0b0636a2bddbb1272c3261e47b7ca6dfdbac1b8d"}},
	{"whisper-base", "special_tokens_map.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/special_tokens_map.json", 2194, "e67ae3a0aaa99abcd9f187138e12db1f65c16a14761c50ef10eef2c174a7a691"}},
	{"whisper-base", "tokenizer.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/tokenizer.json", 2480466, "27fc476bfe7f17299480be2273fc0608e4d5a99aba2ab5dec5374b4482d1a566"}},
	{"whisper-base", "tokenizer_config.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/tokenizer_config.json", 282682, "2e036e4dbacfdeb7242c7d4ec4149f4a16e86026048f94d1637e3a8ee9c6a573"}},
	{"whisper-base", "vocab.json", asset{"https://huggingface.co/onnx-community/whisper-base/resolve/1846881b6b3a3024392c1eea3ad983695bc23925/vocab.json", 1036584, "50d6a919f0a0601d56a04eb583c780d18553aa388254ba3158eb6a00f13e2c1a"}},
	{"whisper-small", "added_tokens.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/added_tokens.json", 34604, "9715fd2243b6f06a5858b5e32950d2853f73dd5bc201aafcf76f5082a2d8acd1"}},
	{"whisper-small", "config.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/config.json", 2227, "457854d452f17661e197d74aee12b8e74fb75ba30ebfaa7426d0d61ea1e08a18"}},
	{"whisper-small", "generation_config.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/generation_config.json", 3893, "f538b28220c6a6d6f1af1458d4141cacb4ef4963df3de98a19490440c412ddf0"}},
	{"whisper-small", "normalizer.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/normalizer.json", 52666, "bf1c507dc8724ca9cf9903640dacfb69dae2f00edee4f21ceba106a7392f26dd"}},
	{"whisper-small", "onnx/decoder_model_merged_quantized.onnx", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/onnx/decoder_model_merged_quantized.onnx", 156750845, "ec07c3cbb64172c39791e26ee870a65ac22b458c36722bfe2776b3dbf741e0c9"}},
	{"whisper-small", "onnx/encoder_model_quantized.onnx", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/onnx/encoder_model_quantized.onnx", 92326160, "a43a83f3c5361cd591cfa7c36f14b43cf7cb22f47a415cc14a8d557be800fa92"}},
	{"whisper-small", "preprocessor_config.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/preprocessor_config.json", 339, "a6a76d28c93edb273669eb9e0b0636a2bddbb1272c3261e47b7ca6dfdbac1b8d"}},
	{"whisper-small", "special_tokens_map.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/special_tokens_map.json", 2194, "e67ae3a0aaa99abcd9f187138e12db1f65c16a14761c50ef10eef2c174a7a691"}},
	{"whisper-small", "tokenizer.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/tokenizer.json", 2480466, "27fc476bfe7f17299480be2273fc0608e4d5a99aba2ab5dec5374b4482d1a566"}},
	{"whisper-small", "tokenizer_config.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/tokenizer_config.json", 282683, "2a4c4281cf9f51ac6ccc406fdc711a087afe6530f671fa7b80953edc498275ce"}},
	{"whisper-small", "vocab.json", asset{"https://huggingface.co/onnx-community/whisper-small/resolve/36050c46d777d46dc4b5f43f6d90574fc38f8732/vocab.json", 1036584, "50d6a919f0a0601d56a04eb583c780d18553aa388254ba3158eb6a00f13e2c1a"}},
}

func whisperDir(base string) string { return filepath.Join(voiceDir(base), "whisper") }

func whisperLocal(base string, f whisperFile) string {
	if f.group == "runtime" {
		return filepath.Join(whisperDir(base), f.path)
	}
	return filepath.Join(whisperDir(base), "models", "onnx-community", f.group, filepath.FromSlash(f.path))
}

func whisperModel(s *Settings) string {
	if s.Get("whisper_model", "base") == "small" {
		return "whisper-small"
	}
	return "whisper-base"
}

func whisperNeeded(model string) []whisperFile {
	var list []whisperFile
	for _, f := range whisperFiles {
		if f.group == "runtime" || f.group == model {
			list = append(list, f)
		}
	}
	return list
}

func whisperInstalled(base, model string) bool {
	for _, f := range whisperNeeded(model) {
		if st, err := os.Stat(whisperLocal(base, f)); err != nil || st.Size() != f.asset.Size {
			return false
		}
	}
	return true
}

func whisperSize(model string) int64 {
	var n int64
	for _, f := range whisperNeeded(model) {
		n += f.asset.Size
	}
	return n
}

func installWhisper(base, model string, progress func(string, float64), cancelled func() bool) error {
	files := whisperNeeded(model)
	if len(files) <= 3 {
		return errors.New("unknown whisper model: " + model)
	}
	total := float64(whisperSize(model))
	var done int64
	for _, f := range files {
		dest := whisperLocal(base, f)
		if st, err := os.Stat(dest); err == nil && st.Size() == f.asset.Size {
			done += f.asset.Size
			continue
		}
		os.MkdirAll(filepath.Dir(dest), 0o755)
		before := done
		name := filepath.Base(f.path)
		err := download(f.asset, dest, func(n int64) {
			progress(fmt.Sprintf("%s (%s)", model, name), float64(before+n)/total)
		}, cancelled, nil)
		if err != nil {
			return err
		}
		done += f.asset.Size
	}
	return nil
}

// 画面に渡す: 使うモデルの名前 (モデルの置き場所は /voice/whisper/models/)
func whisperClientModel(model string) string {
	return "onnx-community/" + strings.TrimPrefix(model, "onnx-community/")
}

// /install-whisper: Whisper のランタイムと今のモデルを入れて、話しかけた内容を Whisper で書き起こすようにする
func (l *Lumi) installWhisperCmd() {
	model := whisperModel(l.s)
	l.setBusy(true)
	cancel := make(chan struct{})
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	l.write(T("whisper.installStart", model, float64(whisperSize(model))/1e6)+"\n", "dim")
	go func() {
		defer l.setBusy(false)
		lastPct := -1
		defer l.setActivity("", 0)
		err := installWhisper(dataDir(), model, func(step string, ratio float64) {
			l.setActivity("download", ratio) // 口がプログレスバーになる
			if pct := int(ratio * 100); pct/10 != lastPct/10 {
				l.write(fmt.Sprintf("  [%3d%%] %s\n", pct, step), "dim")
				lastPct = pct
			}
		}, func() bool {
			select {
			case <-cancel:
				return true
			default:
				return false
			}
		})
		switch {
		case err == errCancelled:
			l.write("^C\n"+T("local.installCancelled", "/install-whisper")+"\n\n", "dim")
		case err != nil:
			l.errorText(T("download.failed", err.Error()))
		default:
			if l.s.Err == nil && l.s.Get("stt", "vosk") != "whisper" {
				l.s.Set("stt", "whisper")
			}
			l.whisperErrorShown = false
			l.info(T("whisper.installed"))
			if l.micOn && l.listening {
				l.applyVoice(false)
			}
		}
	}()
}
