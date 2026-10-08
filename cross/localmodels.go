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
	for _, m := range localModels {
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
		fmt.Fprintf(&b, "  %s%s %6.1fGB%s  %s  %s\n", mark, padRight(m.ID, 14), float64(m.Asset.Size)/1e9, vision, T("local.note."+m.Note), state)
	}
	b.WriteString("\n  " + T("local.modelsHow"))
	l.info(b.String())
}
