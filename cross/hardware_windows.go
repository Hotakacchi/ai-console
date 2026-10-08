package main

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// MEMORYSTATUSEX
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// 画面アダプター (ディスプレイ) のドライバーの設定がある場所。HardwareInformation.qwMemorySize が GPU 専用のメモリ
const displayClass = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

func detectSpec() pcSpec {
	var s pcSpec
	m := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r != 0 {
		s.RAM = m.TotalPhys
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, displayClass, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return s
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	for _, n := range names {
		sub, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		mem, _, err := sub.GetIntegerValue("HardwareInformation.qwMemorySize")
		if err != nil {
			// 古いドライバーは 4 バイトの値 (4GB まで)
			if b, _, err2 := sub.GetBinaryValue("HardwareInformation.MemorySize"); err2 == nil && len(b) >= 4 {
				mem = uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
			} else if v, _, err2 := sub.GetIntegerValue("HardwareInformation.MemorySize"); err2 == nil {
				mem = v
			}
		}
		desc, _, _ := sub.GetStringValue("DriverDesc")
		sub.Close()
		if mem > s.VRAM {
			s.VRAM, s.GPU = mem, strings.TrimSpace(desc)
		}
	}
	// 内蔵 GPU (1GB 未満) は数えない
	if s.VRAM < 1*gb {
		s.VRAM, s.GPU = 0, ""
	}
	return s
}
