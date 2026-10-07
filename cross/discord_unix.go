//go:build !windows

package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Mac・Linux の Discord は UNIX ソケット discord-ipc-N で待っている (Flatpak・Snap 版は別の場所)
func dialDiscord(i int) (io.ReadWriteCloser, error) {
	name := fmt.Sprintf("discord-ipc-%d", i)
	var dirs []string
	for _, env := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := os.Getenv(env); v != "" {
			dirs = append(dirs, v)
		}
	}
	dirs = append(dirs, "/tmp")
	for _, dir := range dirs {
		for _, sub := range []string{"", "app/com.discordapp.Discord", "snap.discord", ".flatpak/dev.vencord.Vesktop/xdg-run"} {
			if c, err := net.DialTimeout("unix", filepath.Join(dir, sub, name), time.Second); err == nil {
				return c, nil
			}
		}
	}
	return nil, errors.New("not found")
}
