package main

// 画面操作の入力 (Mac): CGEvent。「システム設定 > プライバシーとセキュリティ > アクセシビリティ」でルミを許可する必要がある

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>

static int lumiTrusted(void) {
	const void *keys[] = { kAXTrustedCheckOptionPrompt };
	const void *vals[] = { kCFBooleanTrue };
	CFDictionaryRef opts = CFDictionaryCreate(NULL, keys, vals, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int ok = AXIsProcessTrustedWithOptions(opts) ? 1 : 0;
	CFRelease(opts);
	return ok;
}

static double lumiPointsWide(void) { return CGDisplayBounds(CGMainDisplayID()).size.width; }

static void lumiCursor(double *x, double *y) {
	CGEventRef e = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	*x = p.x; *y = p.y;
}

static void lumiMove(double x, double y) {
	CGEventRef e = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, CGPointMake(x, y), kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void lumiClick(int right, int count) {
	double x, y;
	lumiCursor(&x, &y);
	CGEventType down = right ? kCGEventRightMouseDown : kCGEventLeftMouseDown;
	CGEventType up = right ? kCGEventRightMouseUp : kCGEventLeftMouseUp;
	CGMouseButton b = right ? kCGMouseButtonRight : kCGMouseButtonLeft;
	for (int i = 1; i <= count; i++) {
		CGEventRef d = CGEventCreateMouseEvent(NULL, down, CGPointMake(x, y), b);
		CGEventSetIntegerValueField(d, kCGMouseEventClickState, i);
		CGEventPost(kCGHIDEventTap, d);
		CFRelease(d);
		CGEventRef u = CGEventCreateMouseEvent(NULL, up, CGPointMake(x, y), b);
		CGEventSetIntegerValueField(u, kCGMouseEventClickState, i);
		CGEventPost(kCGHIDEventTap, u);
		CFRelease(u);
	}
}

static void lumiScroll(int lines) {
	CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 1, lines);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void lumiTypeChar(UniChar c) {
	CGEventRef d = CGEventCreateKeyboardEvent(NULL, 0, true);
	CGEventKeyboardSetUnicodeString(d, 1, &c);
	CGEventPost(kCGHIDEventTap, d);
	CFRelease(d);
	CGEventRef u = CGEventCreateKeyboardEvent(NULL, 0, false);
	CGEventKeyboardSetUnicodeString(u, 1, &c);
	CGEventPost(kCGHIDEventTap, u);
	CFRelease(u);
}

static void lumiKey(int code, unsigned long long flags) {
	CGEventRef d = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)code, true);
	CGEventSetFlags(d, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, d);
	CFRelease(d);
	CGEventRef u = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)code, false);
	CGEventSetFlags(u, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, u);
	CFRelease(u);
}
*/
import "C"

import (
	"errors"
	"image"
	"strings"
	"time"
	"unicode/utf16"
)

// 撮った画面 (ピクセル) の座標を、マウスの座標 (ポイント。Retina では半分) にする
func uiScreenPoint(img image.Rectangle, x, y float64) (int, int) {
	scale := 1.0
	if w := img.Dx(); w > 0 {
		scale = float64(C.lumiPointsWide()) / float64(w)
	}
	return int(x*scale + 0.5), int(y*scale + 0.5)
}

func uiMoveTo(x, y int) error {
	C.lumiMove(C.double(x), C.double(y))
	return nil
}

func uiCursor() (int, int) {
	var x, y C.double
	C.lumiCursor(&x, &y)
	return int(x), int(y)
}

func uiClick(button string, double bool) error {
	right, count := 0, 1
	if button == "right" {
		right = 1
	}
	if double {
		count = 2
	}
	C.lumiClick(C.int(right), C.int(count))
	return nil
}

func uiScroll(notches int) error {
	C.lumiScroll(C.int(notches))
	return nil
}

func uiType(text string) error {
	for _, u := range utf16.Encode([]rune(text)) {
		if u == '\n' {
			C.lumiKey(36, 0) // return
		} else {
			C.lumiTypeChar(C.UniChar(u))
		}
		time.Sleep(8 * time.Millisecond)
	}
	return nil
}

// Mac の仮想キーコード
var macKeys = map[string]int{
	"a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7, "c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15,
	"y": 16, "t": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "9": 25, "7": 26, "8": 28, "0": 29, "o": 31, "u": 32,
	"i": 34, "p": 35, "l": 37, "j": 38, "k": 40, "n": 45, "m": 46,
	"enter": 36, "return": 36, "tab": 48, "space": 49, "backspace": 51, "delete": 117, "del": 117, "esc": 53, "escape": 53,
	"home": 115, "end": 119, "pageup": 116, "pagedown": 121, "left": 123, "right": 124, "down": 125, "up": 126,
	"f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97, "f7": 98, "f8": 100, "f9": 101, "f10": 109, "f11": 103, "f12": 111,
}

var macFlags = map[string]uint64{
	"shift": 1 << 17, "ctrl": 1 << 18, "control": 1 << 18, "alt": 1 << 19, "option": 1 << 19, "cmd": 1 << 20, "command": 1 << 20, "win": 1 << 20,
}

func uiKey(combo string) error {
	var flags uint64
	code := -1
	for _, k := range strings.Split(strings.ToLower(strings.ReplaceAll(combo, " ", "")), "+") {
		if f, ok := macFlags[k]; ok {
			flags |= f
			continue
		}
		c, ok := macKeys[k]
		if !ok {
			return errors.New(T("ui.unknownKey", k))
		}
		code = c
	}
	if code < 0 {
		return errors.New(T("ui.unknownKey", combo))
	}
	C.lumiKey(C.int(code), C.ulonglong(flags))
	return nil
}

// アクセシビリティの許可があるか (なければ、許可を求める画面を出す)
func uiReady() error {
	if C.lumiTrusted() == 0 {
		return errors.New(T("ui.macPermission"))
	}
	return nil
}
