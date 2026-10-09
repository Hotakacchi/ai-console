package main

// 音楽の操作: 「次の曲」「一時停止」「音量上げて」などで、キーボードのメディアキーと同じ操作をする。
//   AI の <media>playpause|next|prev|volup|voldown|mute</media>、または /media <操作>
// Windows はメディアキーを押したことにする (Spotify・YouTube など、ほとんどのアプリに効く)。
// Mac は音量は osascript で、再生の操作はミュージック・Spotify に。Linux は playerctl と pactl で。

import "strings"

var mediaActions = []string{"playpause", "next", "prev", "volup", "voldown", "mute"}

func normalizeMedia(a string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	switch a {
	case "play", "pause", "toggle":
		return "playpause"
	case "previous", "back":
		return "prev"
	case "up", "volume+", "louder":
		return "volup"
	case "down", "volume-", "quieter":
		return "voldown"
	}
	return a
}

func validMedia(a string) bool {
	for _, x := range mediaActions {
		if a == x {
			return true
		}
	}
	return false
}

// AI の <media> と /media
func (l *Lumi) mediaAction(a string) string {
	a = normalizeMedia(a)
	if !validMedia(a) {
		return T("media.unknown", a, strings.Join(mediaActions, " / "))
	}
	if err := sendMedia(a); err != nil {
		return T("media.failed", err.Error())
	}
	return T("media.done", T("media."+a))
}

func (l *Lumi) mediaTool(r toolRequest) string {
	msg := l.mediaAction(r.Command)
	l.write("  🎵 "+msg+"\n", "cyan")
	return "\n[" + T("media.label") + "] " + r.Command + "\n" + msg + "\n"
}

func (l *Lumi) mediaCommand(arg string) {
	if strings.TrimSpace(arg) == "" {
		l.info(T("media.how", strings.Join(mediaActions, " / ")))
		return
	}
	l.info(l.mediaAction(arg))
}
