//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// macOS は言語環境の設定、Linux は環境変数 (LC_ALL / LC_MESSAGES / LANG) から
func osLanguage() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			for _, f := range strings.FieldsFunc(string(out), func(r rune) bool { return strings.ContainsRune("()\", \n", r) }) {
				if f != "" {
					return f
				}
			}
		}
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			return strings.SplitN(v, ".", 2)[0]
		}
	}
	return "en"
}
