package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type cpuSample struct{ idle, total uint64 }

// /proc/stat の 1 行目 (cpu user nice system idle iowait irq softirq steal …)
func cpuTimes() cpuSample {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return cpuSample{}
	}
	fields := strings.Fields(sc.Text())
	var s cpuSample
	for i, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		s.total += n
		if i == 3 || i == 4 { // idle と iowait
			s.idle += n
		}
	}
	return s
}

func cpuPercent(a, b cpuSample) (float64, bool) {
	if a.total == 0 || b.total <= a.total {
		return 0, false
	}
	total, idle := b.total-a.total, b.idle-a.idle
	return 100 * float64(total-idle) / float64(total), true
}

func memoryPercent() (float64, bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	defer f.Close()
	var total, avail uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), ":")
		n, _ := strconv.ParseUint(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "kB")), 10, 64)
		switch k {
		case "MemTotal":
			total = n
		case "MemAvailable":
			avail = n
		}
	}
	if total == 0 {
		return 0, false
	}
	return 100 * float64(total-avail) / float64(total), true
}

func batteryLevel() (pct int, charging, ok bool) {
	bats, _ := filepath.Glob("/sys/class/power_supply/BAT*")
	if len(bats) == 0 {
		return 0, false, false
	}
	b, err := os.ReadFile(filepath.Join(bats[0], "capacity"))
	if err != nil {
		return 0, false, false
	}
	pct, err = strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false, false
	}
	on, _ := onACPower()
	return pct, on, true
}
