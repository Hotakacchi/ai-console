//go:build manual

package main

import (
	"encoding/base64"
	"os"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

// スクリーンショット用の見本の QR コード (本物の鍵ではない): go test -tags manual -run SampleQR
func TestSampleQR(t *testing.T) {
	png, err := qrcode.Encode("http://192.168.1.10:47800/?t=sample-key-for-screenshot", qrcode.Medium, 256)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(os.Getenv("QR_OUT"), []byte("data:image/png;base64,"+base64.StdEncoding.EncodeToString(png)), 0o644)
}
