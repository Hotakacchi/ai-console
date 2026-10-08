package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const powerPoll = 2 * time.Second

var procGetSystemPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

// SYSTEM_POWER_STATUS
type systemPowerStatus struct {
	ACLineStatus        byte // 0: バッテリー、1: 電源、255: 不明
	BatteryFlag         byte // 128: バッテリーがない
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// 電源につながっているか。バッテリーのない PC や分からないときは ok=false
func onACPower() (bool, bool) {
	var s systemPowerStatus
	if r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s))); r == 0 {
		return false, false
	}
	if s.BatteryFlag == 128 || s.BatteryFlag == 255 || s.ACLineStatus > 1 {
		return false, false
	}
	return s.ACLineStatus == 1, true
}
