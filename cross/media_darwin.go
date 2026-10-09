package main

import (
	"errors"
	"os/exec"
	"strings"
)

// Mac: 音量は osascript で。再生の操作は、動いているミュージック・Spotify に頼む
func sendMedia(a string) error {
	switch a {
	case "volup", "voldown":
		sign := "+"
		if a == "voldown" {
			sign = "-"
		}
		return exec.Command("osascript", "-e", "set volume output volume ((output volume of (get volume settings)) "+sign+" 10)").Run()
	case "mute":
		return exec.Command("osascript", "-e", "set volume output muted (not (output muted of (get volume settings)))").Run()
	}
	verb := map[string]string{"playpause": "playpause", "next": "next track", "prev": "previous track"}[a]
	done := false
	for _, app := range []string{"Spotify", "Music"} {
		running, _ := exec.Command("osascript", "-e", `application "`+app+`" is running`).Output()
		if strings.TrimSpace(string(running)) != "true" {
			continue
		}
		if exec.Command("osascript", "-e", `tell application "`+app+`" to `+verb).Run() == nil {
			done = true
		}
	}
	if !done {
		return errors.New("Spotify / Music")
	}
	return nil
}
