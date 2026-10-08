package main

// より自然な声: VOICEVOX エンジン (https://voicevox.hiroshiba.jp/) を使う読み上げ。
//  - 配布されている .vvpp (中身は zip) から、必要なファイルだけを範囲指定でダウンロードする
//    (エンジンと最初の音声モデルで約 330MB。全部だと約 1.9GB)
//  - エンジンは llama-server と同じようにバックグラウンドで動かす
//  - 音の長さ (モーラごとの母音) がわかるので、口の動きを声にぴったり合わせられる
// VOICEVOX の利用規約により、声を使うときは「VOICEVOX:キャラクター名」と表示する。

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const voicevoxVersion = "0.25.2"

// OS と CPU ごとの配布ファイル (CPU 版)
var voicevoxAssets = map[string]string{
	"windows/amd64": "voicevox_engine-windows-cpu-" + voicevoxVersion + ".vvpp",
	"darwin/arm64":  "voicevox_engine-macos-arm64-" + voicevoxVersion + ".vvpp",
	"darwin/amd64":  "voicevox_engine-macos-x64-" + voicevoxVersion + ".vvpp",
	"linux/amd64":   "voicevox_engine-linux-cpu-x64-" + voicevoxVersion + ".vvpp",
	"linux/arm64":   "voicevox_engine-linux-cpu-arm64-" + voicevoxVersion + ".vvpp",
}

// 入れる音声モデル (0.vvm に四国めたん・ずんだもん・春日部つむぎ などが入っている)
var voicevoxModels = []string{"model/0.vvm"}

func voicevoxDir(base string) string { return filepath.Join(base, "voicevox") }

func voicevoxExe(base string) string {
	name := "run"
	if runtime.GOOS == "windows" {
		name = "run.exe"
	}
	return filepath.Join(voicevoxDir(base), name)
}

func voicevoxInstalled(base string) bool {
	_, err := os.Stat(filepath.Join(voicevoxDir(base), ".complete"))
	return err == nil
}

// ---- ダウンロード (zip の中の必要なファイルだけ) ----

// HTTP の範囲指定で読む io.ReaderAt。1MB ずつ読み、少しだけ覚えておく
type httpRangeReader struct {
	url     string
	size    int64
	client  *http.Client
	mu      sync.Mutex
	blocks  map[int64][]byte
	order   []int64
	fetched func(n int64)
	stop    func() bool
}

const rangeBlock = 1 << 20

func newHTTPRangeReader(u string) (*httpRangeReader, error) {
	r := &httpRangeReader{url: u, client: &http.Client{Timeout: 2 * time.Minute}, blocks: map[int64][]byte{}}
	req, _ := http.NewRequest("HEAD", u, nil)
	req.Header.Set("User-Agent", "Lumi")
	res, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	if res.StatusCode != 200 || res.ContentLength <= 0 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	r.size = res.ContentLength
	return r, nil
}

func (r *httpRangeReader) block(i int64) ([]byte, error) {
	r.mu.Lock()
	if b, ok := r.blocks[i]; ok {
		r.mu.Unlock()
		return b, nil
	}
	r.mu.Unlock()
	if r.stop != nil && r.stop() {
		return nil, errCancelled
	}
	start := i * rangeBlock
	end := min(start+rangeBlock, r.size) - 1
	var data []byte
	var err error
	for try := 0; try < 3; try++ { // 一時的な失敗は少し待ってやり直す
		req, _ := http.NewRequest("GET", r.url, nil) // 署名つきの転送先は期限があるので、毎回元の URL から
		req.Header.Set("User-Agent", "Lumi")
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		var res *http.Response
		if res, err = r.client.Do(req); err == nil {
			if res.StatusCode == http.StatusPartialContent {
				data, err = io.ReadAll(res.Body)
			} else {
				err = fmt.Errorf("HTTP %d", res.StatusCode)
			}
			res.Body.Close()
		}
		if err == nil && int64(len(data)) == end-start+1 {
			break
		}
		if err == nil {
			err = errors.New("short read")
		}
		time.Sleep(time.Duration(try+1) * time.Second)
	}
	if err != nil {
		return nil, err
	}
	if r.fetched != nil {
		r.fetched(int64(len(data)))
	}
	r.mu.Lock()
	r.blocks[i] = data
	r.order = append(r.order, i)
	if len(r.order) > 16 {
		delete(r.blocks, r.order[0])
		r.order = r.order[1:]
	}
	r.mu.Unlock()
	return data, nil
}

func (r *httpRangeReader) ReadAt(p []byte, off int64) (int, error) {
	n := 0
	for n < len(p) {
		if off >= r.size {
			return n, io.EOF
		}
		b, err := r.block(off / rangeBlock)
		if err != nil {
			return n, err
		}
		c := copy(p[n:], b[off%rangeBlock:])
		n += c
		off += int64(c)
	}
	return n, nil
}

// エンジンと音声モデルを入れる。progress(説明, 0〜1)
func installVoicevox(base string, progress func(string, float64), cancelled func() bool) error {
	asset, ok := voicevoxAssets[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return errors.New(T("voicevox.noBuild"))
	}
	r, err := newHTTPRangeReader("https://github.com/VOICEVOX/voicevox_engine/releases/download/" + voicevoxVersion + "/" + asset)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(r, r.size)
	if err != nil {
		return err
	}
	want := func(name string) bool {
		if !strings.HasPrefix(name, "model/") {
			return true
		}
		for _, m := range voicevoxModels {
			if name == m {
				return true
			}
		}
		return false
	}
	var total, done int64
	for _, f := range zr.File {
		if want(f.Name) {
			total += int64(f.CompressedSize64)
		}
	}
	r.fetched = func(n int64) {
		done += n
		progress(T("voicevox.downloading"), min(1, float64(done)/float64(total)))
	}
	r.stop = cancelled

	dir := voicevoxDir(base)
	os.MkdirAll(dir, 0o755)
	for _, f := range zr.File {
		if !want(f.Name) || f.FileInfo().IsDir() {
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(p, filepath.Clean(dir)+string(os.PathSeparator)) {
			continue
		}
		// 前回の途中まで入っていたら、大きさが合うものは飛ばす
		if st, err := os.Stat(p); err == nil && st.Size() == int64(f.UncompressedSize64) {
			continue
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Mode()&0o111 != 0 || filepath.Base(p) == "run" {
			mode = 0o755
		}
		err = writeFile(p+".part", rc, mode) // 読み終わりに zip の CRC32 で中身が確かめられる
		rc.Close()
		if err != nil {
			os.Remove(p + ".part")
			return err
		}
		if err := os.Rename(p+".part", p); err != nil {
			return err
		}
	}
	if runtime.GOOS != "windows" {
		os.Chmod(voicevoxExe(base), 0o755)
	}
	return os.WriteFile(filepath.Join(dir, ".complete"), []byte(voicevoxVersion), 0o644)
}

// ---- エンジンを動かす ----

type voicevoxServer struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	endpoint string
	ready    chan struct{}
	exited   chan struct{}
	err      error
	speakers []vvSpeaker
}

type vvSpeaker struct {
	Name   string `json:"name"`
	Styles []struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	} `json:"styles"`
}

var voicevox = &voicevoxServer{}

// 起動して使えるようになるまで待つ (起動済みならすぐ返る)
func (v *voicevoxServer) Start(base string) error {
	v.mu.Lock()
	if v.cmd != nil && !isClosed(v.exited) {
		ready := v.ready
		v.mu.Unlock()
		<-ready
		return v.err
	}
	if !voicevoxInstalled(base) {
		v.mu.Unlock()
		return errors.New(T("voicevox.notInstalled"))
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		v.mu.Unlock()
		return err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cmd := exec.Command(voicevoxExe(base), "--host", "127.0.0.1", "--port", fmt.Sprint(port))
	cmd.Dir = voicevoxDir(base)
	log, _ := os.Create(filepath.Join(voicevoxDir(base), "engine.log"))
	if log != nil {
		cmd.Stdout, cmd.Stderr = log, log
	}
	hideWindow(cmd)
	killWithParent(cmd)
	if err := cmd.Start(); err != nil {
		if log != nil {
			log.Close()
		}
		v.mu.Unlock()
		return err
	}
	afterStart(cmd)
	exited := make(chan struct{})
	v.cmd, v.endpoint, v.ready, v.exited, v.err = cmd, fmt.Sprintf("http://127.0.0.1:%d", port), make(chan struct{}), exited, nil
	ready := v.ready
	go func() {
		cmd.Wait()
		if log != nil {
			log.Close()
		}
		close(exited)
	}()
	go func() {
		defer close(ready)
		deadline := time.Now().Add(3 * time.Minute)
		client := &http.Client{Timeout: 2 * time.Second}
		for {
			select {
			case <-exited:
				v.err = errors.New(T("voicevox.startFailed"))
				return
			default:
			}
			if res, err := client.Get(v.endpoint + "/speakers"); err == nil {
				var sp []vvSpeaker
				ok := res.StatusCode == 200 && json.NewDecoder(res.Body).Decode(&sp) == nil
				res.Body.Close()
				if ok {
					v.mu.Lock()
					v.speakers = sp
					v.mu.Unlock()
					return
				}
			}
			if time.Now().After(deadline) {
				cmd.Process.Kill()
				v.err = errors.New(T("voicevox.startFailed"))
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
	v.mu.Unlock()
	<-ready
	return v.err
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// エンジンが動いているか
func (v *voicevoxServer) Running() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cmd != nil && !isClosed(v.exited)
}

func (v *voicevoxServer) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cmd != nil && v.cmd.Process != nil {
		v.cmd.Process.Kill()
	}
	v.cmd = nil
}

// 声の id からキャラクター名とスタイル名を引く
func (v *voicevoxServer) styleName(id int) (string, string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, s := range v.speakers {
		for _, st := range s.Styles {
			if st.ID == id {
				return s.Name, st.Name
			}
		}
	}
	return "", ""
}

// 声の id が無効なら、入っている声から既定を選ぶ (春日部つむぎ → 最初の声)
func (v *voicevoxServer) pickStyle(id int) int {
	if name, _ := v.styleName(id); name != "" {
		return id
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	first := -1
	for _, s := range v.speakers {
		for _, st := range s.Styles {
			if first < 0 {
				first = st.ID
			}
			if s.Name == "春日部つむぎ" {
				return st.ID
			}
		}
	}
	return first
}

// 口の動きの 1 点 (秒、開き具合 0〜1)
type mouthKey struct {
	T    float64 `json:"t"`
	Open float64 `json:"open"`
}

var vowelOpen = map[string]float64{"a": 1, "o": 0.8, "e": 0.6, "u": 0.5, "i": 0.4, "N": 0.2, "A": 0.6, "I": 0.3, "U": 0.3, "E": 0.4, "O": 0.5}

// 文を WAV にし、口の動きの時刻表と一緒に返す。rate は -10〜10
func (v *voicevoxServer) Synthesize(text string, style, rate int) ([]byte, []mouthKey, error) {
	v.mu.Lock()
	endpoint := v.endpoint
	v.mu.Unlock()
	client := &http.Client{Timeout: time.Minute}
	q := url.Values{"text": {text}, "speaker": {fmt.Sprint(style)}}
	res, err := client.Post(endpoint+"/audio_query?"+q.Encode(), "application/json", nil)
	if err != nil {
		return nil, nil, err
	}
	var query map[string]any
	err = json.NewDecoder(res.Body).Decode(&query)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 {
		return nil, nil, fmt.Errorf("audio_query: HTTP %d", res.StatusCode)
	}
	speed := 1 + float64(rate)*0.05
	query["speedScale"] = speed

	// モーラごとの母音から口の動きを作る (長さは speedScale で割る)
	var keys []mouthKey
	t := num(query["prePhonemeLength"]) / speed
	addMora := func(m map[string]any) {
		t += num(m["consonant_length"]) / speed
		keys = append(keys, mouthKey{t, vowelOpen[str(m, "vowel")]})
		t += num(m["vowel_length"]) / speed
	}
	if phrases, ok := query["accent_phrases"].([]any); ok {
		for _, p := range phrases {
			ph, _ := p.(map[string]any)
			moras, _ := ph["moras"].([]any)
			for _, m := range moras {
				if mm, ok := m.(map[string]any); ok {
					addMora(mm)
				}
			}
			if pm, ok := ph["pause_mora"].(map[string]any); ok {
				keys = append(keys, mouthKey{t, 0})
				t += num(pm["vowel_length"]) / speed
			}
		}
	}
	keys = append(keys, mouthKey{t, 0})

	body, _ := json.Marshal(query)
	res, err = client.Post(endpoint+"/synthesis?speaker="+fmt.Sprint(style), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, nil, fmt.Errorf("synthesis: HTTP %d", res.StatusCode)
	}
	wav, err := io.ReadAll(res.Body)
	return wav, keys, err
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}
