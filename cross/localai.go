package main

// ローカルAI (llama.cpp の llama-server + GGUF モデル) のダウンロードと起動。
// 取得元はバージョンを固定し、SHA-256 で中身を確かめる。

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

type asset struct {
	URL    string
	Size   int64
	SHA256 string
}

const llamaBase = "https://github.com/ggml-org/llama.cpp/releases/download/b11433/"

// OS と CPU ごとの llama.cpp (GPU があれば使える Vulkan / Metal 版)
var llamaAssets = map[string]asset{
	"windows/amd64": {llamaBase + "llama-b11433-bin-win-vulkan-x64.zip", 33337778, "021001e2b7a60aab1d23b301edb2f076e8a5136d8d29686d0d95341342088714"},
	"windows/arm64": {llamaBase + "llama-b11433-bin-win-vulkan-arm64.zip", 25879207, "73ebd3d4d4f6dfa2ea3e46ac01038fca9916f98575abfba2ae7f6b0b9ba880df"},
	"darwin/arm64":  {llamaBase + "llama-b11433-bin-macos-arm64.tar.gz", 11971372, "5e7b2383009facb31404f308cdb9edd1fb15131330801408fb582bd05e188f97"},
	"darwin/amd64":  {llamaBase + "llama-b11433-bin-macos-x64.tar.gz", 11487490, "caaadcff99ce696bbb6a1ef6f7c0050250fe60d8448464ee7c90878cde3a5370"},
	"linux/amd64":   {llamaBase + "llama-b11433-bin-ubuntu-vulkan-x64.tar.gz", 31636263, "1243a90945de3644f86bc98b588c9f7664c8528cf925b184671df060e8334d1f"},
	"linux/arm64":   {llamaBase + "llama-b11433-bin-ubuntu-vulkan-arm64.tar.gz", 24845697, "ca6b5a7256b6517fa27a939eaf164784d4fe2475a9954ee5f268ba23e3a33727"},
}

const (
	modelName = "Qwen3.5-4B"
	modelFile = "Qwen3.5-4B-Q4_K_M.gguf"
)

var modelAsset = asset{
	"https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/resolve/e87f176479d0855a907a41277aca2f8ee7a09523/Qwen3.5-4B-Q4_K_M.gguf",
	2740937888,
	"00fe7986ff5f6b463e62455821146049db6f9313603938a70800d1fb69ef11a4",
}

func llamaAsset() (asset, bool) {
	a, ok := llamaAssets[runtime.GOOS+"/"+runtime.GOARCH]
	return a, ok
}

func localTotalSize() int64 {
	a, _ := llamaAsset()
	return a.Size + modelAsset.Size
}

func localDir(base string) string  { return filepath.Join(base, "local") }
func llamaDir(base string) string  { return filepath.Join(localDir(base), "llama") }
func modelsDir(base string) string { return filepath.Join(localDir(base), "models") }

// model が空なら標準モデル、ファイル名だけなら models フォルダ内として扱う
func localModelPath(base, model string) string {
	if model == "" {
		model = modelFile
	}
	if filepath.IsAbs(model) {
		return model
	}
	return filepath.Join(modelsDir(base), model)
}

func serverExe(base string) string {
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	found := ""
	filepath.WalkDir(llamaDir(base), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func localInstalled(base string) bool {
	_, err := os.Stat(localModelPath(base, ""))
	return serverExe(base) != "" && err == nil
}

var errCancelled = errors.New("cancelled")

// llama.cpp と標準モデルをダウンロードする。progress(説明, 0〜1)
func installLocal(base string, progress func(string, float64), cancelled func() bool) error {
	la, ok := llamaAsset()
	if !ok {
		return errors.New(T("local.noBuild", runtime.GOOS+"/"+runtime.GOARCH))
	}
	total := float64(la.Size + modelAsset.Size)
	os.MkdirAll(modelsDir(base), 0o755)

	if serverExe(base) == "" {
		archive := filepath.Join(localDir(base), filepath.Base(la.URL))
		err := download(la, archive, func(done int64) {
			progress(T("local.dlEngine"), float64(done)/total)
		}, cancelled, nil)
		if err != nil {
			return err
		}
		progress(T("local.extracting"), float64(la.Size)/total)
		os.RemoveAll(llamaDir(base))
		if err := extract(archive, llamaDir(base)); err != nil {
			return err
		}
		os.Remove(archive)
	}

	model := localModelPath(base, "")
	if _, err := os.Stat(model); err != nil {
		err := download(modelAsset, model, func(done int64) {
			progress(T("local.dlModel", modelName), float64(la.Size+done)/total)
		}, cancelled, func() { progress(T("local.verifying"), 1) })
		if err != nil {
			return err
		}
	}
	progress(T("local.done"), 1)
	return nil
}

// ダウンロード用の HTTP。つながるまで少し長めに待つ (GitHub のファイル置き場は時々遅い)
var downloadClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout:   30 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	IdleConnTimeout:       90 * time.Second,
	ForceAttemptHTTP2:     true,
}}

// 途中で止まっても .part から再開できるダウンロード。最後にサイズと SHA-256 を確かめる。
// つながらない・途中で切れたときは、少し待ってから続きを取り直す (4 回まで)
func download(a asset, dest string, progress func(int64), cancelled func() bool, verifying func()) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if err = downloadOnce(a, dest, progress, cancelled, verifying); err == nil || !retryable(err) {
			return err
		}
		for i := 0; i < (attempt+1)*3*10; i++ { // 3 秒、6 秒、9 秒待つ (中断はすぐ受け付ける)
			if cancelled() {
				return errCancelled
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return shortNetError(err)
}

// ネットワークの一時的な失敗か (やり直せば直りそうか)
func retryable(err error) bool {
	var ne net.Error
	var ue *url.Error
	return errors.As(err, &ne) || errors.As(err, &ue) || errors.Is(err, io.ErrUnexpectedEOF)
}

// 署名つきの長い URL をそのまま見せないように、「サーバー名: 理由」にする
func shortNetError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		host := ue.URL
		if u, perr := url.Parse(ue.URL); perr == nil {
			host = u.Host
		}
		return fmt.Errorf("%s: %v", host, ue.Err)
	}
	return err
}

func downloadOnce(a asset, dest string, progress func(int64), cancelled func() bool, verifying func()) error {
	part := dest + ".part"
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	if have > a.Size {
		os.Remove(part)
		have = 0
	}
	if have < a.Size {
		req, _ := http.NewRequest("GET", a.URL, nil)
		req.Header.Set("User-Agent", "Lumi")
		if have > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		}
		res, err := downloadClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			os.Remove(part) // 続きから取れないので、次は最初から
			return io.ErrUnexpectedEOF
		}
		if res.StatusCode >= 500 {
			return &url.Error{Op: "Get", URL: a.URL, Err: errors.New(T("download.http", res.StatusCode))} // サーバーの一時的な不調
		}
		if res.StatusCode >= 300 {
			return errors.New(T("download.http", res.StatusCode))
		}
		flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
		if have > 0 && res.StatusCode != http.StatusPartialContent {
			have = 0 // 再開できないサーバーなら最初から
		}
		if have == 0 {
			flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		}
		f, err := os.OpenFile(part, flags, 0o644)
		if err != nil {
			return err
		}
		buf := make([]byte, 1<<20)
		last := time.Time{}
		for {
			if cancelled() {
				f.Close()
				return errCancelled
			}
			n, rerr := res.Body.Read(buf)
			if n > 0 {
				if _, err := f.Write(buf[:n]); err != nil {
					f.Close()
					return err
				}
				have += int64(n)
				if time.Since(last) > 200*time.Millisecond {
					progress(have)
					last = time.Now()
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				f.Close()
				return rerr
			}
		}
		f.Close()
	}

	if verifying != nil {
		verifying()
	}
	sum, err := fileSHA256(part)
	if st, _ := os.Stat(part); err != nil || st.Size() != a.Size || sum != a.SHA256 {
		os.Remove(part)
		return errors.New(T("download.corrupt", filepath.Base(dest)))
	}
	os.Remove(dest)
	return os.Rename(part, dest)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// zip / tar.gz を展開する (展開先の外に出るパスは無視する)
func extract(archive, dest string) error {
	safe := func(name string) (string, bool) {
		p := filepath.Join(dest, name)
		return p, strings.HasPrefix(p, filepath.Clean(dest)+string(os.PathSeparator))
	}
	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			p, ok := safe(f.Name)
			if !ok || f.FileInfo().IsDir() {
				continue
			}
			os.MkdirAll(filepath.Dir(p), 0o755)
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = writeFile(p, rc, f.Mode())
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, ok := safe(h.Name)
		if !ok {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(p, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := writeFile(p, tr, fs.FileMode(h.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// ライブラリの別名 (libfoo.so -> libfoo.so.1) は展開先の中を指すものだけ作る
			if target, ok := safe(filepath.Join(filepath.Dir(h.Name), h.Linkname)); ok {
				os.MkdirAll(filepath.Dir(p), 0o755)
				os.Symlink(filepath.Base(target), p)
			}
		}
	}
}

func writeFile(path string, r io.Reader, mode fs.FileMode) error {
	if mode&0o111 == 0 {
		mode = 0o644
	} else {
		mode = 0o755
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	f.Close()
	return err
}

// ---------------- llama-server を 1 つだけ動かす ----------------

// 動いている llama-server 1 つ分
type serverProc struct {
	cmd      *exec.Cmd
	args     string
	endpoint string
	apiKey   string
	ready    chan struct{} // 使えるようになった (か、失敗した) ら閉じる
	exited   chan struct{} // プロセスが終わったら閉じる
	err      error
}

func (p *serverProc) wait() error { <-p.ready; return p.err }

func (p *serverProc) alive() bool {
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

// モデルの読み込みが終わると /health が 200 を返すので、それまで待つ
func (p *serverProc) watch() {
	defer close(p.ready)
	deadline := time.Now().Add(5 * time.Minute)
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case <-p.exited:
			p.err = errors.New(T("local.startFailed"))
			return
		default:
		}
		if res, err := client.Get(p.endpoint + "/health"); err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				return
			}
		}
		if time.Now().After(deadline) {
			p.cmd.Process.Kill()
			p.err = errors.New(T("local.timeout"))
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// 待っている間はロックを持たないので、読み込み中でも Stop (終了) がすぐ効く
type llamaServer struct {
	mu sync.Mutex
	p  *serverProc
}

var localServer = &llamaServer{}

func (s *llamaServer) Endpoint() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return ""
	}
	return s.p.endpoint
}

func (s *llamaServer) APIKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return ""
	}
	return s.p.apiKey
}

func (s *llamaServer) Start(base, modelPath, mmproj string, gpu bool, context int) error {
	p, err := s.spawn(base, modelPath, mmproj, gpu, context)
	if err != nil {
		return err
	}
	return p.wait()
}

// 同じ設定で動いていればそれを、なければ新しく起動したものを返す (起動を待たない)
func (s *llamaServer) spawn(base, modelPath, mmproj string, gpu bool, context int) (*serverProc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exe := serverExe(base)
	if _, err := os.Stat(modelPath); exe == "" || err != nil {
		return nil, errors.New(T("local.notInstalled"))
	}
	key := fmt.Sprintf("%s|%s|%v|%d", modelPath, mmproj, gpu, context)
	if s.p != nil && s.p.args == key && s.p.alive() {
		return s.p, nil
	}
	s.stopLocked()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	// 同じ PC のほかのアプリから勝手に使われないよう、起動ごとにランダムな鍵をかける
	b := make([]byte, 16)
	rand.Read(b)
	apiKey := hex.EncodeToString(b)
	ngl := "0"
	if gpu {
		ngl = "99"
	}
	// -np 1: 会話は 1 つだけなので、コンテキストを丸ごと 1 つの会話に使う
	args := []string{"-m", modelPath, "--host", "127.0.0.1", "--port", fmt.Sprint(port),
		"-c", fmt.Sprint(context), "-np", "1", "-ngl", ngl, "--api-key", apiKey}
	if mmproj != "" {
		args = append(args, "--mmproj", mmproj) // 画像を読むための部品
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	libEnv := map[string]string{"linux": "LD_LIBRARY_PATH", "darwin": "DYLD_LIBRARY_PATH"}[runtime.GOOS]
	cmd.Env = os.Environ()
	if libEnv != "" {
		cmd.Env = append(cmd.Env, libEnv+"="+filepath.Dir(exe))
	}
	logFile, _ := os.Create(filepath.Join(localDir(base), "server.log"))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	hideWindow(cmd)
	killWithParent(cmd)
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return nil, err
	}
	afterStart(cmd)
	p := &serverProc{
		cmd: cmd, args: key, apiKey: apiKey,
		endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		ready:    make(chan struct{}), exited: make(chan struct{}),
	}
	go func() {
		cmd.Wait()
		if logFile != nil {
			logFile.Close()
		}
		close(p.exited)
	}()
	go p.watch()
	s.p = p
	return p, nil
}

func (s *llamaServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *llamaServer) stopLocked() {
	if s.p != nil && s.p.cmd.Process != nil {
		s.p.cmd.Process.Kill()
	}
	s.p = nil
}
