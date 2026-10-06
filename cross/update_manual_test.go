//go:build manual

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 最新のリリースから、この OS のインストーラーをダウンロードして SHA-256 を確かめる (入れはしない):
// go test -tags manual -run UpdateDownload -v
func TestUpdateDownload(t *testing.T) {
	loadLocales()
	r, err := fetchLatestRelease()
	if err != nil {
		t.Fatal(err)
	}
	kind := kindWinSetup
	if runtime.GOOS != "windows" {
		kind = kindTar
	}
	name := updateAssetName(kind, r.version(), runtime.GOARCH)
	var a asset
	for _, x := range r.Assets {
		if x.Name == name {
			a = asset{x.URL, x.Size, strings.TrimPrefix(x.Digest, "sha256:")}
		}
	}
	if a.SHA256 == "" {
		t.Fatalf("%s: no asset or digest", name)
	}
	dest := filepath.Join(t.TempDir(), name)
	if err := download(a, dest, func(int64) {}, func() bool { return false }, nil); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(dest)
	t.Logf("%s %s: %d bytes, sha256 ok", r.Tag, name, st.Size())

	// 中身を変えたら、確かめで弾かれる
	a.SHA256 = strings.Repeat("0", 64)
	if err := download(a, dest+"2", func(int64) {}, func() bool { return false }, nil); err == nil {
		t.Error("a wrong digest was accepted")
	}
}
