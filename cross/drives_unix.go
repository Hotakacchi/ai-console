//go:build !windows

package main

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// つながっているドライブ (マウントされた場所)
func listDrives() []string {
	var dirs []string
	me := os.Getenv("USER")
	if runtime.GOOS == "darwin" {
		dirs = []string{"/Volumes"}
	} else {
		if u, err := user.Current(); err == nil {
			me = u.Username
		}
		dirs = []string{"/media/" + me, "/run/media/" + me, "/media"}
	}
	var out []string
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := filepath.Join(d, e.Name())
			if d == "/media" && e.Name() == me {
				continue // /media/<ユーザー名> は入れ物 (中を上で見ている)
			}
			// Mac の起動ディスク (/Volumes/Macintosh HD → /) は数えない
			if target, err := filepath.EvalSymlinks(p); err == nil && target == "/" {
				continue
			}
			out = append(out, p)
		}
	}
	return out
}

func driveName(path string) string { return filepath.Base(path) }
