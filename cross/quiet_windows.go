package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSHQueryUserNotificationState = windows.NewLazySystemDLL("shell32.dll").NewProc("SHQueryUserNotificationState")
	user32Quiet                      = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow          = user32Quiet.NewProc("GetForegroundWindow")
	procGetWindowRect                = user32Quiet.NewProc("GetWindowRect")
	procGetClassNameW                = user32Quiet.NewProc("GetClassNameW")
	procMonitorFromWindow            = user32Quiet.NewProc("MonitorFromWindow")
	procGetMonitorInfoW              = user32Quiet.NewProc("GetMonitorInfoW")
)

type monitorInfo struct {
	Size    uint32
	Monitor windows.Rect
	Work    windows.Rect
	Flags   uint32
}

// 全画面のアプリ (ゲーム・動画・プレゼン) が前にあるか:
// Direct3D の全画面・プレゼンテーション モードか、手前の窓がモニターとぴったり同じ大きさのとき。
// (Windows の「全画面のアプリがある (QUNS_BUSY)」は、別のモニターの窓などでも立つことがあるので使わない)
func fullscreenApp() bool {
	if procSHQueryUserNotificationState.Find() == nil {
		var state int32
		if r, _, _ := procSHQueryUserNotificationState.Call(uintptr(unsafe.Pointer(&state))); r == 0 && (state == 3 || state == 4) {
			return true
		}
	}
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	// デスクトップやタスクバーは全画面のアプリではない
	cls := make([]uint16, 64)
	procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), uintptr(len(cls)))
	switch windows.UTF16ToString(cls) {
	case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return false
	}
	// ルミ自身の窓は数えない
	var pid uint32
	windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid)
	if int(pid) == os.Getpid() {
		return false
	}
	var r windows.Rect
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}
	mon, _, _ := procMonitorFromWindow.Call(hwnd, 2) // MONITOR_DEFAULTTONEAREST
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return false
	}
	// ぴったり同じ大きさのときだけ (ふつうに最大化した窓は、枠の分だけモニターからはみ出すので数えない)
	return r == mi.Monitor
}
