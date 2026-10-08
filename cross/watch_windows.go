package main

import (
	"encoding/csv"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")

type cpuSample struct{ idle, total uint64 }

func cpuTimes() cpuSample {
	var idle, kernel, user windows.Filetime
	r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return cpuSample{}
	}
	ft := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	// kernel には idle も含まれる
	return cpuSample{idle: ft(idle), total: ft(kernel) + ft(user)}
}

func cpuPercent(a, b cpuSample) (float64, bool) {
	if a.total == 0 || b.total <= a.total {
		return 0, false
	}
	total, idle := b.total-a.total, b.idle-a.idle
	return 100 * float64(total-idle) / float64(total), true
}

func memoryPercent() (float64, bool) {
	m := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r == 0 || m.TotalPhys == 0 {
		return 0, false
	}
	return 100 * float64(m.TotalPhys-m.AvailPhys) / float64(m.TotalPhys), true
}

func systemDrive() string {
	if d := os.Getenv("SystemDrive"); d != "" {
		return d
	}
	return "C:"
}

func diskSpace() (free, total uint64, ok bool) {
	p, _ := windows.UTF16PtrFromString(systemDrive() + `\`)
	var avail, tot, totalFree uint64
	if windows.GetDiskFreeSpaceEx(p, &avail, &tot, &totalFree) != nil {
		return 0, 0, false
	}
	return avail, tot, true
}

func batteryLevel() (pct int, charging, ok bool) {
	var s systemPowerStatus
	if r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s))); r == 0 {
		return 0, false, false
	}
	if s.BatteryFlag == 128 || s.BatteryFlag == 255 || s.BatteryLifePercent > 100 {
		return 0, false, false
	}
	return int(s.BatteryLifePercent), s.ACLineStatus == 1, true
}

// いちばんメモリを使っているアプリ (同じ名前のプロセスは合計。Chrome などはたくさんに分かれているので)
func topMemoryProcess() (string, uint64) {
	cmd := exec.Command("tasklist", "/fo", "csv", "/nh")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", 0
	}
	rows, err := csv.NewReader(strings.NewReader(decodeOutput(out))).ReadAll()
	if err != nil {
		return "", 0
	}
	sum := map[string]uint64{}
	for _, r := range rows {
		if len(r) < 5 {
			continue
		}
		// "123,456 K" のような書き方 (区切りは言語で違う)
		digits := strings.Map(func(c rune) rune {
			if c >= '0' && c <= '9' {
				return c
			}
			return -1
		}, r[4])
		kb, err := strconv.ParseUint(digits, 10, 64)
		if err != nil {
			continue
		}
		sum[strings.TrimSuffix(r[0], ".exe")] += kb * 1024
	}
	return topOf(sum)
}
