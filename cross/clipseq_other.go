//go:build !windows

package main

// Windows 以外は番号がないので、毎回読む (0 は分からない)
func clipboardSeq() uint32 { return 0 }
