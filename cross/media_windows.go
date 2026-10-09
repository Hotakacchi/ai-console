package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

var procKeybdEvent = windows.NewLazySystemDLL("user32.dll").NewProc("keybd_event")

// メディアキー (VK_MEDIA_* / VK_VOLUME_*) を押して離したことにする
var mediaKeys = map[string]uintptr{
	"playpause": 0xB3, "next": 0xB0, "prev": 0xB1,
	"volup": 0xAF, "voldown": 0xAE, "mute": 0xAD,
}

func sendMedia(a string) error {
	vk, ok := mediaKeys[a]
	if !ok {
		return errors.New(a)
	}
	const keyUp, extended = 0x0002, 0x0001
	times := 1
	if a == "volup" || a == "voldown" {
		times = 5 // 1 回だと 2% しか変わらないので、10% くらい
	}
	for i := 0; i < times; i++ {
		procKeybdEvent.Call(vk, 0, extended, 0)
		procKeybdEvent.Call(vk, 0, extended|keyUp, 0)
	}
	return nil
}
