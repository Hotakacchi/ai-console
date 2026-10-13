package main

import (
	"testing"
	"unsafe"
)

// SendInput に渡す形の大きさ (実際には送らない)
func TestWinInputSize(t *testing.T) {
	if s := unsafe.Sizeof(winInput{}); unsafe.Sizeof(uintptr(0)) == 8 && s != 40 {
		t.Errorf("INPUT is %d bytes, want 40", s)
	}
	if unsafe.Sizeof(mouseInput{}) > 32 || unsafe.Sizeof(keybdInput{}) > 32 {
		t.Error("union too small")
	}
}
