//go:build !windows

package main

import "os"

// 出力が端末なら色を使う (ファイルやパイプなら使わない)
func setupTerminal() bool {
	st, err := os.Stdout.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"
}
