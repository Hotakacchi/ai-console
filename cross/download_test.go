package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// 途中で切れても、続きから取り直して最後まで落とせる
func TestDownloadRetry(t *testing.T) {
	loadLocales()
	body := []byte(strings.Repeat("lumi-download-test-", 50000)) // 約 1MB
	sum := sha256.Sum256(body)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			fmt.Sscanf(rg, "bytes=%d-", &start)
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)-start))
		if start > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
			w.WriteHeader(http.StatusPartialContent)
		}
		if n <= 2 {
			// 1・2 回目は途中で切る
			w.Write(body[start : start+100000])
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write(body[start:])
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	a := asset{srv.URL + "/file.bin", int64(len(body)), hex.EncodeToString(sum[:])}
	if err := download(a, dest, func(int64) {}, func() bool { return false }, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != string(body) {
		t.Error("content differs")
	}
	if calls.Load() != 3 {
		t.Errorf("calls %d", calls.Load())
	}
}

func TestShortNetError(t *testing.T) {
	long := "https://release-assets.githubusercontent.com/github-production-release-asset/1/abc?sig=xyz&jwt=eyJ"
	err := shortNetError(&url.Error{Op: "Get", URL: long, Err: errors.New("net/http: TLS handshake timeout")})
	if err.Error() != "release-assets.githubusercontent.com: net/http: TLS handshake timeout" {
		t.Errorf("%q", err)
	}
}
