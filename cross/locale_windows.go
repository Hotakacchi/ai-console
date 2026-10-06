package main

import (
	"syscall"
	"unsafe"
)

// Windows の表示言語 (例: ja-JP)
func osLanguage() string {
	buf := make([]uint16, 85)
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	if n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n > 0 {
		return syscall.UTF16ToString(buf)
	}
	return "en"
}
