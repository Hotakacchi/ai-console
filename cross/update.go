package main

// 自動アップデート: 新しい版が出ていたら /update でダウンロードして入れ替え、起動し直す。
// どの形で入れたかに合わせて、同じ形の配布ファイルを使う。
//   Windows: インストーラー (自分だけ / すべてのユーザー) か、持ち運び用の exe
//   macOS:   dmg の中の Lumi.app で置き換える
//   Linux:   .deb (pkexec で入れる) か、tar.gz の中の lumi で置き換える
// ダウンロードしたファイルは GitHub が出している SHA-256 で確かめる。

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const latestReleaseAPI = "https://api.github.com/repos/Hotakacchi/ai-console/releases/latest"

type releaseInfo struct {
	Tag    string `json:"tag_name"`
	URL    string `json:"html_url"`
	Body   string `json:"body"` // リリースノート (ルミの新機能を聞かれたときに使う)
	Assets []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"` // "sha256:..."
	} `json:"assets"`
}

func (r releaseInfo) version() string { return strings.TrimPrefix(r.Tag, "v") }

func fetchLatestRelease() (releaseInfo, error) {
	var r releaseInfo
	req, _ := http.NewRequest("GET", latestReleaseAPI, nil)
	req.Header.Set("User-Agent", "Lumi/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return r, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return r, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil || r.Tag == "" {
		return r, errors.New("no release")
	}
	return r, nil
}

// どの形で入れたか
type installKind string

const (
	kindWinSetup    installKind = "win-setup"
	kindWinPortable installKind = "win-portable"
	kindMacApp      installKind = "mac-app"
	kindDeb         installKind = "deb"
	kindTar         installKind = "tar"
	kindDev         installKind = "dev" // go run などの開発中 (更新しない)
)

func currentInstall() (installKind, string) {
	exe, err := os.Executable()
	if err != nil {
		return kindDev, ""
	}
	exe, _ = filepath.EvalSymlinks(exe)
	if strings.Contains(exe, "go-build") || version == "dev" {
		return kindDev, exe
	}
	switch runtime.GOOS {
	case "windows":
		// exe の隣に settings.json があれば持ち運び用
		if dataDir() == filepath.Dir(exe) {
			return kindWinPortable, exe
		}
		return kindWinSetup, exe
	case "darwin":
		if app := filepath.Dir(filepath.Dir(filepath.Dir(exe))); strings.HasSuffix(app, ".app") {
			return kindMacApp, exe
		}
		return kindDev, exe
	}
	if exe == "/usr/bin/lumi" {
		return kindDeb, exe
	}
	return kindTar, exe
}

// 入れた形に合う配布ファイルの名前
func updateAssetName(kind installKind, ver, arch string) string {
	switch kind {
	case kindWinSetup:
		return "Lumi-Windows-Setup-" + ver + ".exe"
	case kindWinPortable:
		return "lumi-windows-" + arch + ".exe"
	case kindMacApp:
		return "lumi-mac-" + ver + ".dmg"
	case kindDeb:
		return "lumi_" + ver + "_" + arch + ".deb"
	case kindTar:
		return "lumi-linux-" + arch + ".tar.gz"
	}
	return ""
}

// 起動時: 新しい版があれば知らせる (update_check が on のとき)
func (l *Lumi) checkUpdate() {
	if !l.s.On("update_check", "on") {
		return
	}
	go func() {
		r, err := fetchLatestRelease()
		if err != nil || !newerVersion(r.version(), version) {
			return
		}
		l.write(T("update.available", r.Tag)+"\n\n", "yellow")
	}()
}

// /update: 新しい版があれば、確認してから入れ替える
func (l *Lumi) updateCommand() {
	if !l.gui() {
		l.info(T("cli.updateInWindow")) // 入れ替えるのは窓のルミの exe なので、そちらで
		return
	}
	if !l.begin() {
		return
	}
	go func() {
		done := false
		defer func() {
			if !done {
				l.setBusy(false)
			}
		}()
		r, err := fetchLatestRelease()
		if err != nil {
			l.errorText(T("update.checkFailed", err.Error()))
			return
		}
		if !newerVersion(r.version(), version) {
			l.info(T("update.latest", versionLabel()))
			return
		}
		kind, exe := currentInstall()
		name := updateAssetName(kind, r.version(), runtime.GOARCH)
		var a asset
		for _, x := range r.Assets {
			if x.Name == name {
				a = asset{x.URL, x.Size, strings.TrimPrefix(x.Digest, "sha256:")}
			}
		}
		if kind == kindDev || a.URL == "" || a.SHA256 == "" {
			l.info(T("update.manual", r.Tag, r.URL))
			openURL(r.URL)
			return
		}
		if a := strings.ToLower(l.ask(T("update.ask", version, r.version()))); a != "y" {
			l.write(T("web.stopped")+"\n\n", "dim")
			return
		}
		dir, err := os.MkdirTemp("", "lumi_update_")
		if err != nil {
			l.errorText(T("update.failed", err.Error()))
			return
		}
		file := filepath.Join(dir, name)
		last := -1
		cancel := l.cancelChan()
		err = download(a, file, func(n int64) {
			l.setActivity("download", float64(n)/float64(max(1, a.Size)))
			if pct := int(n * 100 / max(1, a.Size)); pct/20 != last/20 {
				l.write(fmt.Sprintf("  [%3d%%] %s\n", pct, T("update.downloading", r.Tag)), "dim")
				last = pct
			}
		}, func() bool {
			select {
			case <-cancel:
				return true
			default:
				return false
			}
		}, nil)
		l.setActivity("", 0)
		if err != nil {
			os.RemoveAll(dir)
			if err == errCancelled {
				l.write("^C\n\n", "dim")
			} else {
				l.errorText(T("update.failed", err.Error()) + "\n" + T("update.retry", r.URL))
			}
			return
		}
		l.info(T("update.installing", r.Tag))
		if err := applyUpdate(kind, exe, file); err != nil {
			l.errorText(T("update.failed", err.Error()))
			return
		}
		// 新しい版は、ルミが終わってから入れ替えて起動される
		done = true
		time.Sleep(time.Second)
		l.quit()
	}()
}

// 入れ替えて、ルミが終わったあとに新しい版を起動する仕組みを動かす
func applyUpdate(kind installKind, exe, file string) error {
	switch kind {
	case kindWinSetup:
		mode := "/CURRENTUSER"
		if pf := os.Getenv("ProgramFiles"); pf != "" && strings.HasPrefix(strings.ToLower(exe), strings.ToLower(pf)) {
			mode = "/ALLUSERS" // すべてのユーザー用に入っていれば、同じ形で (OS の確認が出る)
		}
		// /RELAUNCH: 入れ終わったらルミを起動し直す (インストーラーの [Run] で)。
		// ルミが動いたままでも、インストーラーが閉じるのを待ってから入れ替える
		return startDetached(file, "/SILENT", "/SUPPRESSMSGBOXES", "/NORESTART", mode, "/RELAUNCH")
	case kindWinPortable:
		// 動いている exe は消せないが名前は変えられるので、古いほうを .old にして新しいものを置く
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return err
		}
		if err := copyFile(file, exe); err != nil {
			os.Rename(old, exe)
			return err
		}
		return relaunchLater(exe)
	case kindMacApp:
		app := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
		if !writable(filepath.Dir(app)) {
			return errors.New(T("update.notWritable", filepath.Dir(app)))
		}
		script := `sleep 2; m=$(mktemp -d); hdiutil attach -nobrowse -quiet -mountpoint "$m" ` + shQuote(file) +
			` && rm -rf ` + shQuote(app+".old") + ` && mv ` + shQuote(app) + " " + shQuote(app+".old") +
			` && ditto "$m/Lumi.app" ` + shQuote(app) + ` && rm -rf ` + shQuote(app+".old") +
			`; hdiutil detach -quiet "$m"; open ` + shQuote(app)
		return startDetached("/bin/sh", "-c", script)
	case kindDeb:
		// 認証画面を出して入れる (動いている間に入れ替えても、Linux では問題ない)
		cmd := exec.Command("pkexec", "apt-get", "install", "-y", "--allow-downgrades", file)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, lastLine(string(out)))
		}
		return relaunchLater(exe)
	case kindTar:
		if err := extractLumiBinary(file, exe); err != nil {
			return err
		}
		return relaunchLater(exe)
	}
	return errors.New("unsupported")
}

// tar.gz の中の lumi-linux/lumi を、動いている exe と入れ替える (新しいファイルを書いてから名前を変える)
func extractLumiBinary(archive, exe string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return errors.New("lumi not found in the archive")
		}
		if filepath.Base(h.Name) != "lumi" || h.Typeflag != tar.TypeReg {
			continue
		}
		tmp := exe + ".new"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		out.Close()
		if err != nil {
			os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, exe)
	}
}

// 前の更新で残った古い exe を片付ける (Windows の持ち運び用)
func cleanupOldExe() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".lumi_w_")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// ルミが終わっても動き続けるように起動する
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	detach(cmd)
	return cmd.Start()
}
