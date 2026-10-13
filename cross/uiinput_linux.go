package main

// 画面操作の入力 (Linux): xdotool (X11)。Wayland では使えないことがある

import (
	"errors"
	"image"
	"os/exec"
	"strconv"
	"strings"
)

func xdotool(args ...string) error {
	out, err := exec.Command("xdotool", args...).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(err.Error() + " " + string(out)))
	}
	return nil
}

func uiScreenPoint(img image.Rectangle, x, y float64) (int, int) { return int(x + 0.5), int(y + 0.5) }

func uiMoveTo(x, y int) error { return xdotool("mousemove", strconv.Itoa(x), strconv.Itoa(y)) }

func uiCursor() (int, int) {
	out, err := exec.Command("xdotool", "getmouselocation", "--shell").Output()
	if err != nil {
		return 0, 0
	}
	var x, y int
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "X="); ok {
			x, _ = strconv.Atoi(v)
		} else if v, ok := strings.CutPrefix(line, "Y="); ok {
			y, _ = strconv.Atoi(v)
		}
	}
	return x, y
}

func uiClick(button string, double bool) error {
	b := "1"
	if button == "right" {
		b = "3"
	}
	if double {
		return xdotool("click", "--repeat", "2", b)
	}
	return xdotool("click", b)
}

func uiScroll(notches int) error {
	b, n := "4", notches
	if notches < 0 {
		b, n = "5", -notches
	}
	return xdotool("click", "--repeat", strconv.Itoa(max(1, n)), b)
}

func uiType(text string) error { return xdotool("type", "--delay", "12", "--", text) }

var xKeys = map[string]string{
	"ctrl": "ctrl", "control": "ctrl", "shift": "shift", "alt": "alt", "win": "super", "cmd": "ctrl", "command": "ctrl",
	"enter": "Return", "return": "Return", "tab": "Tab", "esc": "Escape", "escape": "Escape", "space": "space",
	"backspace": "BackSpace", "delete": "Delete", "del": "Delete", "home": "Home", "end": "End",
	"pageup": "Prior", "pagedown": "Next", "up": "Up", "down": "Down", "left": "Left", "right": "Right",
}

func uiKey(combo string) error {
	var keys []string
	for _, k := range strings.Split(strings.ToLower(strings.ReplaceAll(combo, " ", "")), "+") {
		if v, ok := xKeys[k]; ok {
			keys = append(keys, v)
		} else if len(k) == 1 {
			keys = append(keys, k)
		} else if n, err := strconv.Atoi(strings.TrimPrefix(k, "f")); err == nil && k[0] == 'f' && n >= 1 && n <= 12 {
			keys = append(keys, "F"+strconv.Itoa(n))
		} else {
			return errors.New(T("ui.unknownKey", k))
		}
	}
	return xdotool("key", strings.Join(keys, "+"))
}

func uiReady() error {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return errors.New(T("ui.needXdotool"))
	}
	return nil
}
