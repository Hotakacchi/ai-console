//go:build !windows

package main

// Mac・Linux は全画面のアプリを見分けない (いつも false)
func fullscreenApp() bool { return false }
