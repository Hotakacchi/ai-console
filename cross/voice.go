package main

// 音声入力の準備。認識は画面側で Vosk (WebAssembly) が行う。
// ここでは vosk.js と言語ごとの認識モデルをダウンロードし、/voice/ で画面に渡す。

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var voskJS = asset{
	"https://cdn.jsdelivr.net/npm/vosk-browser@0.0.8/dist/vosk.js",
	5804767,
	"29504515526e974f4cb053cf08811c4de5fb2a74007c0a5a957db50eaa8d5d0c",
}

// 言語ごとの小さい認識モデル (https://alphacephei.com/vosk/models)
var voiceModels = map[string]asset{
	"ja": {"https://alphacephei.com/vosk/models/vosk-model-small-ja-0.22.zip", 49704573,
		"efa092d280153a77615e9e0c7d7283e93e600de3d19d3bec686c57ef19d52eac"},
	"en": {"https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip", 41205931,
		"30f26242c4eb449f948e42cb302dd7a686cb29a3423a8367f99ff41780942498"},
}

func voiceDir(base string) string { return filepath.Join(base, "voice") }

func voiceModelPath(base, lang string) string {
	return filepath.Join(voiceDir(base), lang, "model.tar.gz")
}

func voiceInstalled(base, lang string) bool {
	_, e1 := os.Stat(filepath.Join(voiceDir(base), "vosk.js"))
	_, e2 := os.Stat(voiceModelPath(base, lang))
	return e1 == nil && e2 == nil
}

func voiceDownloadSize(lang string) int64 { return voskJS.Size + voiceModels[lang].Size }

// vosk.js と認識モデルをダウンロードし、モデルを Vosk が読める tar.gz にする
func installVoice(base, lang string, progress func(string, float64), cancelled func() bool) error {
	m, ok := voiceModels[lang]
	if !ok {
		return errors.New(T("voice.noModel"))
	}
	total := float64(voskJS.Size + m.Size)
	dir := voiceDir(base)
	os.MkdirAll(filepath.Join(dir, lang), 0o755)

	js := filepath.Join(dir, "vosk.js")
	if _, err := os.Stat(js); err != nil {
		if err := download(voskJS, js, func(done int64) { progress("vosk.js", float64(done)/total) }, cancelled, nil); err != nil {
			return err
		}
	}
	out := voiceModelPath(base, lang)
	if _, err := os.Stat(out); err == nil {
		return nil
	}
	zipPath := filepath.Join(dir, lang, "model.zip")
	err := download(m, zipPath, func(done int64) {
		progress(filepath.Base(m.URL), float64(voskJS.Size+done)/total)
	}, cancelled, nil)
	if err != nil {
		return err
	}
	if err := zipToTarGz(zipPath, out); err != nil {
		os.Remove(out)
		return err
	}
	os.Remove(zipPath)
	return nil
}

// zip の中身 (vosk-model-xxx/...) を model/... として tar.gz に入れ直す
func zipToTarGz(zipPath, outPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	f, err := os.Create(outPath + ".part")
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, zf := range zr.File {
		parts := strings.SplitN(strings.TrimPrefix(zf.Name, "/"), "/", 2)
		if len(parts) < 2 || parts[1] == "" || strings.Contains(parts[1], "..") {
			continue
		}
		name := path.Join("model", parts[1])
		if zf.FileInfo().IsDir() {
			tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755})
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(zf.UncompressedSize64)})
		_, err = io.Copy(tw, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	f.Close()
	return os.Rename(outPath+".part", outPath)
}

// /voice/ 以下はデータフォルダの voice から返す (それ以外は同梱の画面ファイル)
func withVoiceFiles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/voice/") {
			next.ServeHTTP(w, r)
			return
		}
		rel := path.Clean(strings.TrimPrefix(r.URL.Path, "/voice/"))
		if strings.HasPrefix(rel, "..") {
			http.NotFound(w, r)
			return
		}
		// OS の設定に左右されないように、モジュールと wasm の種類はここで決める
		switch path.Ext(rel) {
		case ".mjs", ".js":
			w.Header().Set("Content-Type", "text/javascript")
		case ".wasm":
			w.Header().Set("Content-Type", "application/wasm")
		}
		http.ServeFile(w, r, filepath.Join(voiceDir(dataDir()), filepath.FromSlash(rel)))
	})
}
