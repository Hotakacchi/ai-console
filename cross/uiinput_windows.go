package main

// 画面操作の入力 (Windows): SetCursorPos と SendInput

import (
	"errors"
	"image"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32UI         = windows.NewLazySystemDLL("user32.dll")
	procSetCursorPos = user32UI.NewProc("SetCursorPos")
	procGetCursorPos = user32UI.NewProc("GetCursorPos")
	procSendInput    = user32UI.NewProc("SendInput")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseLeftDown  = 0x0002
	mouseLeftUp    = 0x0004
	mouseRightDown = 0x0008
	mouseRightUp   = 0x0010
	mouseWheel     = 0x0800

	keyUp      = 0x0002
	keyUnicode = 0x0004
)

// INPUT (64 ビットでは 40 バイト: 種類 4 + 詰め物 4 + 中身 32)
type winInput struct {
	typ  uint32
	_    uint32
	data [32]byte
}

type mouseInput struct {
	dx, dy    int32
	mouseData uint32
	flags     uint32
	time      uint32
	extra     uintptr
}

type keybdInput struct {
	vk, scan uint16
	flags    uint32
	time     uint32
	extra    uintptr
}

func sendInputs(in []winInput) error {
	if len(in) == 0 {
		return nil
	}
	n, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(n) != len(in) {
		return errors.New("SendInput: " + err.Error())
	}
	return nil
}

func mouseEvent(flags, data uint32) winInput {
	in := winInput{typ: inputMouse}
	*(*mouseInput)(unsafe.Pointer(&in.data[0])) = mouseInput{mouseData: data, flags: flags}
	return in
}

func keyEvent(vk, scan uint16, flags uint32) winInput {
	in := winInput{typ: inputKeyboard}
	*(*keybdInput)(unsafe.Pointer(&in.data[0])) = keybdInput{vk: vk, scan: scan, flags: flags}
	return in
}

// 撮った画面の座標を、マウスの座標にする (Windows はそのまま)
func uiScreenPoint(img image.Rectangle, x, y float64) (int, int) { return int(x + 0.5), int(y + 0.5) }

func uiMoveTo(x, y int) error {
	if r, _, err := procSetCursorPos.Call(uintptr(x), uintptr(y)); r == 0 {
		return errors.New("SetCursorPos: " + err.Error())
	}
	return nil
}

func uiCursor() (int, int) {
	var p struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y)
}

// button: left / right。double: ダブルクリック
func uiClick(button string, double bool) error {
	down, up := uint32(mouseLeftDown), uint32(mouseLeftUp)
	if button == "right" {
		down, up = mouseRightDown, mouseRightUp
	}
	if err := sendInputs([]winInput{mouseEvent(down, 0), mouseEvent(up, 0)}); err != nil {
		return err
	}
	if double {
		time.Sleep(60 * time.Millisecond)
		return sendInputs([]winInput{mouseEvent(down, 0), mouseEvent(up, 0)})
	}
	return nil
}

// notches: 上が正 (1 目盛り = 120)
func uiScroll(notches int) error {
	return sendInputs([]winInput{mouseEvent(mouseWheel, uint32(int32(notches*120)))})
}

// 文字をそのまま打つ (日本語も。IME を通さない)
func uiType(text string) error {
	var in []winInput
	for _, u := range windows.StringToUTF16(text) {
		if u == 0 {
			continue
		}
		if u == '\n' {
			in = append(in, keyEvent(0x0D, 0, 0), keyEvent(0x0D, 0, keyUp))
			continue
		}
		in = append(in, keyEvent(0, u, keyUnicode), keyEvent(0, u, keyUnicode|keyUp))
	}
	return sendInputs(in)
}

var winKeys = map[string]uint16{
	"ctrl": 0x11, "control": 0x11, "shift": 0x10, "alt": 0x12, "win": 0x5B, "cmd": 0x11, "command": 0x11,
	"enter": 0x0D, "return": 0x0D, "tab": 0x09, "esc": 0x1B, "escape": 0x1B, "space": 0x20, "backspace": 0x08,
	"delete": 0x2E, "del": 0x2E, "insert": 0x2D, "home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
}

// 「ctrl+s」「alt+f4」「enter」のようなキーを押す
func uiKey(combo string) error {
	var vks []uint16
	for _, k := range strings.Split(strings.ToLower(strings.ReplaceAll(combo, " ", "")), "+") {
		vk, ok := winKeys[k]
		if !ok && len(k) == 1 && (k[0] >= 'a' && k[0] <= 'z' || k[0] >= '0' && k[0] <= '9') {
			vk, ok = uint16(strings.ToUpper(k)[0]), true
		}
		if !ok {
			return errors.New(T("ui.unknownKey", k))
		}
		vks = append(vks, vk)
	}
	var in []winInput
	for _, vk := range vks {
		in = append(in, keyEvent(vk, 0, 0))
	}
	for i := len(vks) - 1; i >= 0; i-- {
		in = append(in, keyEvent(vks[i], 0, keyUp))
	}
	return sendInputs(in)
}

// 画面操作ができるか (Windows はいつもできる)
func uiReady() error { return nil }
