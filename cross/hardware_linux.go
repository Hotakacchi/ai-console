package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func detectSpec() pcSpec {
	var s pcSpec
	if f, err := os.Open("/proc/meminfo"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
				kb, _ := strconv.ParseUint(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "kB")), 10, 64)
				s.RAM = kb * 1024
			}
		}
		f.Close()
	}
	// NVIDIA は nvidia-smi で
	if out, err := exec.Command("nvidia-smi", "--query-gpu=memory.total,name", "--format=csv,noheader,nounits").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			mib, name, _ := strings.Cut(line, ",")
			if v, err := strconv.ParseUint(strings.TrimSpace(mib), 10, 64); err == nil && v*1024*1024 > s.VRAM {
				s.VRAM, s.GPU = v*1024*1024, strings.TrimSpace(name)
			}
		}
	}
	// AMD は /sys から
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/mem_info_vram_total")
	for _, c := range cards {
		b, _ := os.ReadFile(c)
		if v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64); err == nil && v > s.VRAM {
			s.VRAM, s.GPU = v, "AMD GPU"
		}
	}
	if s.VRAM < 1*gb {
		s.VRAM, s.GPU = 0, ""
	}
	return s
}
