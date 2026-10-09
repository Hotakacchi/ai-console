package main

import "golang.org/x/sys/windows"

var procGetClipboardSequenceNumber = windows.NewLazySystemDLL("user32.dll").NewProc("GetClipboardSequenceNumber")

// クリップボードが変わるたびに増える番号 (クリップボードを開かずに分かる)。0 は分からない
func clipboardSeq() uint32 {
	r, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(r)
}
