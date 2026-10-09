package main

import (
	"os/exec"
)

// Linux: 再生は playerctl (MPRIS)、音量は pactl (PulseAudio / PipeWire)
func sendMedia(a string) error {
	var cmd *exec.Cmd
	switch a {
	case "playpause":
		cmd = exec.Command("playerctl", "play-pause")
	case "next":
		cmd = exec.Command("playerctl", "next")
	case "prev":
		cmd = exec.Command("playerctl", "previous")
	case "volup":
		cmd = exec.Command("pactl", "set-sink-volume", "@DEFAULT_SINK@", "+10%")
	case "voldown":
		cmd = exec.Command("pactl", "set-sink-volume", "@DEFAULT_SINK@", "-10%")
	default:
		cmd = exec.Command("pactl", "set-sink-mute", "@DEFAULT_SINK@", "toggle")
	}
	return cmd.Run()
}
