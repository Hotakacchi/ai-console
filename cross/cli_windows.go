package main

import "golang.org/x/sys/windows"

// 端末で色 (ESC[..m) を使えるようにする。出力が端末でなければ (ファイルやパイプ) 色は使わない
func setupTerminal() bool {
	h := windows.Handle(windows.Stdout)
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
