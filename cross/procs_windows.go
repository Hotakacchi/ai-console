package main

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procK32GetProcessMemoryInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

// PROCESS_MEMORY_COUNTERS
type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// 動いているプロセス。want が true を返すものだけ、メモリと CPU の時間も調べる (全部開くと重いので)
func listProcs(want func(p procInfo) bool) []procInfo {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []procInfo
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		p := procInfo{PID: int(e.ProcessID), Name: strings.TrimSuffix(windows.UTF16ToString(e.ExeFile[:]), ".exe")}
		if want(p) {
			p.Mem, p.CPU = procStats(e.ProcessID)
		}
		out = append(out, p)
	}
	return out
}

// メモリ (ワーキングセット) と、これまでに使った CPU の時間 (秒)
func procStats(pid uint32) (uint64, float64) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, 0
	}
	defer windows.CloseHandle(h)
	var mem uint64
	pmc := processMemoryCounters{Cb: uint32(unsafe.Sizeof(processMemoryCounters{}))}
	if r, _, _ := procK32GetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.Cb)); r != 0 {
		mem = uint64(pmc.WorkingSetSize)
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return mem, 0
	}
	ft := func(f windows.Filetime) float64 { return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) / 1e7 }
	return mem, ft(kernel) + ft(user)
}
