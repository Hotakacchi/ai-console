package main

// ローカルAIのモデルの一覧。/install-local list で一覧、/install-local <名前> でそのモデルを入れて切り替える。
// URL・大きさ・SHA-256 は Hugging Face の特定のリビジョンに固定している。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type localModel struct {
	ID     string // /install-local や /set model で使う名前
	Name   string // 画面や AI への指示に出す名前
	File   string // models フォルダの中のファイル名
	Family string // qwen / phi (返事のさせ方がモデルによって違う)
	Note   string // 説明 (locales の local.note.<Note>)
	Asset  asset
	Vision *asset // 画像を読む部品 (mmproj)。なければ画像は読めない
}

func hfURL(repo, rev, file string) string {
	return "https://huggingface.co/" + repo + "/resolve/" + rev + "/" + file
}

const defaultLocalModelID = "qwen3.5-4b"

var localModels = []localModel{
	{"qwen3.5-0.8b", "Qwen3.5 0.8B", "Qwen3.5-0.8B-Q4_K_M.gguf", "qwen", "tiny",
		asset{hfURL("unsloth/Qwen3.5-0.8B-GGUF", "6ab461498e2023f6e3c1baea90a8f0fe38ab64d0", "Qwen3.5-0.8B-Q4_K_M.gguf"), 532517120, "bd258782e35f7f458f8aced1adc053e6e92e89bc735ba3be89d38a06121dc517"},
		&asset{hfURL("unsloth/Qwen3.5-0.8B-GGUF", "6ab461498e2023f6e3c1baea90a8f0fe38ab64d0", "mmproj-F16.gguf"), 204987232, "56e4c6cfe73b0c82e3e82bc518d7591997e61d81f723fc41a586f4fa69ea2453"}},
	{"qwen3.5-2b", "Qwen3.5 2B", "Qwen3.5-2B-Q4_K_M.gguf", "qwen", "light",
		asset{hfURL("unsloth/Qwen3.5-2B-GGUF", "f6d5376be1edb4d416d56da11e5397a961aca8ae", "Qwen3.5-2B-Q4_K_M.gguf"), 1280835840, "aaf42c8b7c3cab2bf3d69c355048d4a0ee9973d48f16c731c0520ee914699223"},
		&asset{hfURL("unsloth/Qwen3.5-2B-GGUF", "f6d5376be1edb4d416d56da11e5397a961aca8ae", "mmproj-F16.gguf"), 668227264, "7035e9cb8d7c6a9681d07eef9a364783e86ea4cd73faab2eabb4f43a101830c7"}},
	{"qwen3.5-4b", "Qwen3.5 4B", "Qwen3.5-4B-Q4_K_M.gguf", "qwen", "standard",
		asset{hfURL("unsloth/Qwen3.5-4B-GGUF", "e87f176479d0855a907a41277aca2f8ee7a09523", "Qwen3.5-4B-Q4_K_M.gguf"), 2740937888, "00fe7986ff5f6b463e62455821146049db6f9313603938a70800d1fb69ef11a4"},
		&asset{hfURL("unsloth/Qwen3.5-4B-GGUF", "e87f176479d0855a907a41277aca2f8ee7a09523", "mmproj-F16.gguf"), 672423616, "cd88edcf8d031894960bb0c9c5b9b7e1fea6ebee02b9f7ce925a00d12891f864"}},
	{"qwen3.5-9b", "Qwen3.5 9B", "Qwen3.5-9B-Q4_K_M.gguf", "qwen", "smart",
		asset{hfURL("unsloth/Qwen3.5-9B-GGUF", "3885219b6810b007914f3a7950a8d1b469d598a5", "Qwen3.5-9B-Q4_K_M.gguf"), 5680522464, "03b74727a860a56338e042c4420bb3f04b2fec5734175f4cb9fa853daf52b7e8"},
		&asset{hfURL("unsloth/Qwen3.5-9B-GGUF", "3885219b6810b007914f3a7950a8d1b469d598a5", "mmproj-F16.gguf"), 918166080, "f70dc3509053962b0d0d3ee8a7eacebf5d60aa560cad78254ae8698516ae029f"}},
	{"phi-4-mini", "Phi-4 mini", "Phi-4-mini-instruct-Q4_K_M.gguf", "phi", "english",
		asset{hfURL("unsloth/Phi-4-mini-instruct-GGUF", "78eb92a46fc37e6b524df991ed9aca9bc6aa7b80", "Phi-4-mini-instruct-Q4_K_M.gguf"), 2491874272, "88c00229914083cd112853aab84ed51b87bdf6b9ce42f532d8c85c7c63b1730a"},
		nil},

	// ほかの会社・研究所のモデル (選んで入れるもの。インストーラーやおすすめでは入れない)
	{"gemma-3-1b", "Gemma 3 1B", "gemma-3-1b-it-Q4_K_M.gguf", "gemma", "gemma1b",
		asset{hfURL("unsloth/gemma-3-1b-it-GGUF", "f0b45be0aac41bd6a100a4b5734cad5f67255bfb", "gemma-3-1b-it-Q4_K_M.gguf"), 806058272, "8270790f3ab69fdfe860b7b64008d9a19986d8df7e407bb018184caa08798ebd"},
		nil},
	{"gemma-3-4b", "Gemma 3 4B", "gemma-3-4b-it-Q4_K_M.gguf", "gemma", "gemma4b",
		asset{hfURL("unsloth/gemma-3-4b-it-GGUF", "5a3566e716d80f709ed7b79817eaf7733d2a1fce", "gemma-3-4b-it-Q4_K_M.gguf"), 2489894016, "04a43a22e8d2003deda5acc262f68ec1005fa76c735a9962a8c77042a74a7d19"},
		&asset{hfURL("unsloth/gemma-3-4b-it-GGUF", "5a3566e716d80f709ed7b79817eaf7733d2a1fce", "mmproj-F16.gguf"), 851251328, "731199e016ec5f227b8293fef839899472e0ee4c51adf5f9e5cb66f6558fa142"}},
	{"gemma-3-12b", "Gemma 3 12B", "gemma-3-12b-it-Q4_K_M.gguf", "gemma", "gemma12b",
		asset{hfURL("unsloth/gemma-3-12b-it-GGUF", "d15e4c7dc21dc55d56bf8549db57a71ad8a2a35d", "gemma-3-12b-it-Q4_K_M.gguf"), 7300778336, "15b8fd9d8672cd4240c178c217ca781409291f34e353d2e913b29c7602ceb3ff"},
		&asset{hfURL("unsloth/gemma-3-12b-it-GGUF", "d15e4c7dc21dc55d56bf8549db57a71ad8a2a35d", "mmproj-F16.gguf"), 854200448, "5de4ccfc379faa4cbf608dd365c028aa41f9774cdb1191d148d88910d1014f71"}},
	{"gpt-oss-20b", "gpt-oss 20B", "gpt-oss-20b-MXFP4.gguf", "gpt-oss", "gptoss",
		asset{hfURL("ggml-org/gpt-oss-20b-GGUF", "ef9b12f2ff56c69cf32153a02784e7a3c88bf524", "gpt-oss-20b-MXFP4.gguf"), 12109566624, "27cd6c432c7672cb812a92f611cf3ba7bbc35928262bb1e1253ff4ee6ae35901"},
		nil},
	{"llama-3.2-3b", "Llama 3.2 3B", "Llama-3.2-3B-Instruct-Q4_K_M.gguf", "llama", "llama3b",
		asset{hfURL("unsloth/Llama-3.2-3B-Instruct-GGUF", "e7d0997e49c9cb00d88b4c1a6a16aa894b0bbc31", "Llama-3.2-3B-Instruct-Q4_K_M.gguf"), 2019377600, "6c99cc00ae910f6a532a80022cb4bc1939094527a089c29294b841c0bd87f74d"},
		nil},
	{"llama-3.1-8b", "Llama 3.1 8B", "Llama-3.1-8B-Instruct-Q4_K_M.gguf", "llama", "llama8b",
		asset{hfURL("unsloth/Llama-3.1-8B-Instruct-GGUF", "600b0020115fd6b17f0752848fe7b5a1be686bcd", "Llama-3.1-8B-Instruct-Q4_K_M.gguf"), 4920739200, "b3bdbf23b47d7e6bb791c99b206deb169cd5a96362a9e3399028df2faacdc506"},
		nil},
	{"mistral-small-24b", "Mistral Small 3.2 24B", "Mistral-Small-3.2-24B-Instruct-2506-Q4_K_M.gguf", "mistral", "mistral",
		asset{hfURL("unsloth/Mistral-Small-3.2-24B-Instruct-2506-GGUF", "b750ec2299225e492f1bd27cab88a0a595fa848f", "Mistral-Small-3.2-24B-Instruct-2506-Q4_K_M.gguf"), 14333922848, "a3cc56310807ed0d145eaf9f018ccda9ae7ad8edb41ec870aa2454b0d4700b3c"},
		&asset{hfURL("unsloth/Mistral-Small-3.2-24B-Instruct-2506-GGUF", "b750ec2299225e492f1bd27cab88a0a595fa848f", "mmproj-F16.gguf"), 878053568, "d6af684ae9136398eaa0b59ea9e0b0b850bb6ac5084f1e8c5cb8f85251825eaf"}},
	{"deepseek-r1-8b", "DeepSeek-R1 8B", "DeepSeek-R1-0528-Qwen3-8B-Q4_K_M.gguf", "deepseek", "deepseek",
		asset{hfURL("unsloth/DeepSeek-R1-0528-Qwen3-8B-GGUF", "eb48357c179d34dbf515983f798dfb8752a0f261", "DeepSeek-R1-0528-Qwen3-8B-Q4_K_M.gguf"), 5027785216, "a86349a4180c4e6bb43f874c29c404fa2be3f90b15509bd6d86f697dba724ec1"},
		nil},
	{"swallow-8b", "Llama 3.1 Swallow 8B", "Llama-3.1-Swallow-8B-Instruct-v0.5-Q4_K_M.gguf", "llama", "swallow",
		asset{hfURL("mmnga/Llama-3.1-Swallow-8B-Instruct-v0.5-gguf", "dc00f584312c641eb7af415f7f44bcc4487350cb", "Llama-3.1-Swallow-8B-Instruct-v0.5-Q4_K_M.gguf"), 4920736064, "6da177cee6797ad8f67cdaf6fac5d52818cc0582c7b0779c5c50ef419ca0b088"},
		nil},
	{"sarashina-3b", "Sarashina2.2 3B", "sarashina2.2-3b-instruct-v0.1-Q4_K_M.gguf", "sarashina", "sarashina",
		asset{hfURL("mmnga/sarashina2.2-3b-instruct-v0.1-gguf", "31d771319b04032f33e0d9d860f3984ea4812154", "sarashina2.2-3b-instruct-v0.1-Q4_K_M.gguf"), 2066390112, "d96f4d98eb528df26e8bc09ab81a1d165be4fce67616739e65980bed9038f0f2"},
		nil},
	{"llm-jp-1.8b", "LLM-jp-3.1 1.8B", "llm-jp-3.1-1.8b-instruct4-Q4_K_M.gguf", "llmjp", "llmjp",
		asset{hfURL("mmnga/llm-jp-3.1-1.8b-instruct4-gguf", "14ddbab20c68d8befdced79bd9daa3d7a1a29376", "llm-jp-3.1-1.8b-instruct4-Q4_K_M.gguf"), 1164239136, "e442e3985ba1afd10700e04df2887f81e4c6075f144400d927e7f7d992501632"},
		nil},
}

func defaultLocalModel() localModel {
	m, _ := findLocalModel(defaultLocalModelID)
	return m
}

// 名前 (qwen3.5-9b) かファイル名 (Qwen3.5-9B-Q4_K_M.gguf) で探す
func findLocalModel(v string) (localModel, bool) {
	v = strings.TrimSpace(v)
	for _, m := range localModels {
		if strings.EqualFold(m.ID, v) || strings.EqualFold(m.File, filepath.Base(v)) {
			return m, true
		}
	}
	return localModel{}, false
}

// 設定 model のモデル (空なら標準。一覧にない自分の GGUF なら ok=false)
func currentLocalModel(s *Settings) (localModel, bool) {
	v := s.Get("model", "")
	if v == "" {
		return defaultLocalModel(), true
	}
	return findLocalModel(v)
}

// 画像を読む部品のファイル名 (標準モデルは前からの名前のまま)
func (m localModel) visionFile() string {
	return "mmproj-" + strings.TrimSuffix(m.File, "-Q4_K_M.gguf") + "-F16.gguf"
}

func (m localModel) installed(base string) bool {
	st, err := os.Stat(filepath.Join(modelsDir(base), m.File))
	return err == nil && st.Size() == m.Asset.Size
}

// /install-local [list|名前]
func (l *Lumi) installLocalCommand(arg string) {
	arg = strings.ToLower(strings.TrimSpace(arg))
	switch arg {
	case "list":
		l.listLocalModels()
		return
	case "":
		m, ok := currentLocalModel(l.s)
		if !ok {
			m = defaultLocalModel()
		}
		// まだ何も選んでいなくて入ってもいなければ、この PC に合ったものを入れる
		if l.s.Get("model", "") == "" && !localInstalled(dataDir(), "") {
			spec := thisPC()
			m = recommendLocalModel(spec)
			l.write(T("local.recommendFor", spec.describe(), m.Name)+"\n", "cyan")
		}
		l.installLocalModel(m)
		return
	}
	m, ok := findLocalModel(arg)
	if !ok {
		l.errorText(T("local.noSuchModel", arg))
		l.listLocalModels()
		return
	}
	l.installLocalModel(m)
}

func (l *Lumi) listLocalModels() {
	cur, _ := currentLocalModel(l.s)
	base := dataDir()
	spec := thisPC()
	rec := recommendLocalModel(spec)
	var b strings.Builder
	b.WriteString("  " + T("local.specLine", spec.describe()) + "\n\n")
	for i, m := range localModels {
		if i > 0 && m.Family != "qwen" && localModels[i-1].Family == "qwen" {
			b.WriteString("\n  " + T("local.others") + "\n")
		}
		mark := "  "
		if m.ID == cur.ID && strings.EqualFold(l.s.Get("provider", ""), "local") {
			mark = "* "
		}
		state := ""
		if m.ID == rec.ID {
			state = T("local.recommended") + " "
		}
		if m.installed(base) {
			state += T("local.modelInstalled")
		}
		vision := ""
		if m.Vision != nil {
			vision = " 🖼"
		}
		fmt.Fprintf(&b, "  %s%s %6.1fGB%s  %s  %s\n", mark, padRight(m.ID, 18), float64(m.Asset.Size)/1e9, vision, T("local.note."+m.Note), state)
	}
	b.WriteString("\n  " + T("local.modelsHow"))
	l.info(b.String())
}
