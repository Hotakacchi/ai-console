package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateAssetName(t *testing.T) {
	for kind, want := range map[installKind]string{
		kindWinSetup:    "Lumi-Windows-Setup-1.4.0.exe",
		kindWinPortable: "lumi-windows-amd64.exe",
		kindMacApp:      "lumi-mac-1.4.0.dmg",
		kindDeb:         "lumi_1.4.0_amd64.deb",
		kindTar:         "lumi-linux-amd64.tar.gz",
		kindDev:         "",
	} {
		if got := updateAssetName(kind, "1.4.0", "amd64"); got != want {
			t.Errorf("%s: %q", kind, got)
		}
	}
	// テスト中 (go test) は開発中の扱いで、更新しない
	if kind, _ := currentInstall(); kind != kindDev {
		t.Errorf("kind %s", kind)
	}
}

// tar.gz の中の lumi で、動いている exe を置き換える
func TestExtractLumiBinary(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "lumi-linux-amd64.tar.gz")
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"lumi-linux/lumi.desktop": "x", "lumi-linux/lumi": "NEW BINARY"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	f.Close()
	exe := filepath.Join(dir, "lumi")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	if err := extractLumiBinary(archive, exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW BINARY" {
		t.Errorf("got %q", b)
	}
}
