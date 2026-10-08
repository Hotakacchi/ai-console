package main

// Whisper を whisper.cpp で動かす (Windows x64 と Linux)。画面の中 (WebAssembly・1 スレッド) より数倍速い。
// 書き起こすたびに whisper-cli を起動するので、使っていない間はメモリを使わない。
// Mac は whisper.cpp の配布物がないので、今までどおり画面の中で動かす (whisper.go)。
//   画面 → Go: whisperNative {id, pcm (16kHz・16bit の base64), lang, prompt}
//   Go → 画面: whisperResult {id, text | error}

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const whisperCppBase = "https://github.com/ggml-org/whisper.cpp/releases/download/b5454/"

var whisperCppAssets = map[string]asset{
	"windows/amd64": {whisperCppBase + "whisper-bin-x64.zip", 8928640, "6ba69e3482d7826214f90a6a9c84ca07782aec1e1d0c6a7c30c994fd5d816ccb"},
	"linux/amd64":   {whisperCppBase + "whisper-bin-ubuntu-x64.tar.gz", 10364195, "a72becf15d7917f990f6313867a52638b82b7f9ef237fb0c980dac56a135781c"},
	"linux/arm64":   {whisperCppBase + "whisper-bin-ubuntu-arm64.tar.gz", 4608377, "6b95ebfc60447df48e70ef00a73bdc3f41679ed2d01ef465827206a2ff325149"},
}

// モデル (ggml 形式を 5bit に圧縮したもの。精度はほぼそのままで小さく速い)
const whisperGGML = "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/"

var whisperCppModels = map[string]asset{
	"whisper-base":  {whisperGGML + "ggml-base-q5_1.bin", 59707625, "422f1ae452ade6f30a004d7e5c6a43195e4433bc370bf23fac9cc591f01a8898"},
	"whisper-small": {whisperGGML + "ggml-small-q5_1.bin", 190085487, "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb"},
}

// この PC で whisper.cpp を使えるか
func whisperCppSupported() bool {
	_, ok := whisperCppAssets[runtime.GOOS+"/"+runtime.GOARCH]
	return ok
}

func whisperCppDir(base string) string { return filepath.Join(voiceDir(base), "whisper.cpp") }

func whisperCppModelPath(base, model string) string {
	return filepath.Join(whisperCppDir(base), "models", filepath.Base(whisperCppModels[model].URL))
}

func whisperCppExe(base string) string {
	name := "whisper-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	found := ""
	filepath.WalkDir(filepath.Join(whisperCppDir(base), "bin"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func whisperCppInstalled(base, model string) bool {
	st, err := os.Stat(whisperCppModelPath(base, model))
	return whisperCppExe(base) != "" && err == nil && st.Size() == whisperCppModels[model].Size
}

func whisperCppSize(base, model string) int64 {
	n := whisperCppModels[model].Size
	if whisperCppExe(base) == "" {
		n += whisperCppAssets[runtime.GOOS+"/"+runtime.GOARCH].Size
	}
	return n
}

func installWhisperCpp(base, model string, progress func(string, float64), cancelled func() bool) error {
	bin, ok := whisperCppAssets[runtime.GOOS+"/"+runtime.GOARCH]
	m, okm := whisperCppModels[model]
	if !ok || !okm {
		return errors.New("whisper.cpp: " + runtime.GOOS + "/" + runtime.GOARCH + " " + model)
	}
	total := float64(whisperCppSize(base, model))
	var done int64
	if whisperCppExe(base) == "" {
		archive := filepath.Join(whisperCppDir(base), filepath.Base(bin.URL))
		os.MkdirAll(filepath.Dir(archive), 0o755)
		if err := download(bin, archive, func(n int64) { progress("whisper.cpp", float64(n)/total) }, cancelled, nil); err != nil {
			return err
		}
		os.RemoveAll(filepath.Join(whisperCppDir(base), "bin"))
		if err := extract(archive, filepath.Join(whisperCppDir(base), "bin")); err != nil {
			return err
		}
		os.Remove(archive)
		done = bin.Size
	}
	dest := whisperCppModelPath(base, model)
	if st, err := os.Stat(dest); err != nil || st.Size() != m.Size {
		os.MkdirAll(filepath.Dir(dest), 0o755)
		before := done
		if err := download(m, dest, func(n int64) { progress(model, float64(before+n)/total) }, cancelled, nil); err != nil {
			return err
		}
	}
	return nil
}

// 16kHz・16bit・モノラルの音を WAV にする
func pcmToWAV(pcm []byte) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(pcm)))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))     // PCM
	w(uint16(1))     // モノラル
	w(uint32(16000)) // 16kHz
	w(uint32(16000 * 2))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}

// 書き起こしに使うスレッドの数 (CPU の半分、2〜8)
func whisperThreads() int {
	return max(2, min(8, runtime.NumCPU()/2))
}

// pcm (16kHz・16bit) を書き起こす。prompt は単語帳 (前に話した文として見せ、その言葉を書き起こしやすくする)
func transcribeCpp(base, model string, pcm []byte, lang, prompt string) (string, error) {
	exe := whisperCppExe(base)
	if exe == "" {
		return "", errors.New("whisper.cpp is not installed")
	}
	f, err := os.CreateTemp("", "lumi-voice-*.wav")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(pcmToWAV(pcm))
	f.Close()
	if err != nil {
		return "", err
	}
	if lang != "ja" {
		lang = "en"
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	args := []string{"-m", whisperCppModelPath(base, model), "-l", lang, "-t", strconv.Itoa(whisperThreads()), "-nt", "-np", "-f", f.Name()}
	if p := strings.TrimSpace(prompt); p != "" {
		if r := []rune(p); len(r) > 400 {
			p = string(r[len(r)-400:]) // 長すぎると逆効果なので後ろの方だけ
		}
		args = append(args, "--prompt", p)
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = filepath.Dir(exe)
	if runtime.GOOS == "linux" {
		cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+filepath.Dir(exe))
	}
	hideWindow(cmd)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return "", errors.New(strings.TrimSpace(err.Error() + " " + msg))
	}
	var lines []string
	for _, line := range strings.Split(decodeOutput(out.Bytes()), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " "), nil
}

// 画面から届いた音を書き起こして返す
func (l *Lumi) nativeWhisper(data any) {
	m, _ := data.(map[string]any)
	id := m["id"]
	reply := func(text string, err error) {
		r := map[string]any{"id": id, "text": text}
		if err != nil {
			r["error"] = err.Error()
		}
		l.emit("whisperResult", r)
	}
	pcm, err := base64.StdEncoding.DecodeString(asString(m["pcm"]))
	if err != nil {
		reply("", err)
		return
	}
	text, err := transcribeCpp(dataDir(), whisperModel(l.s), pcm, asString(m["lang"]), asString(m["prompt"]))
	reply(text, err)
}
