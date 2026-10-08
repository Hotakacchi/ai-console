package main

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Mac は ps の %CPU (少し前からの平均) の合計を、CPU の数で割る
type cpuSample struct{ percent float64 }

func cpuTimes() cpuSample {
	out, err := exec.Command("ps", "-A", "-o", "%cpu=").Output()
	if err != nil {
		return cpuSample{-1}
	}
	sum := 0.0
	for _, f := range strings.Fields(string(out)) {
		v, _ := strconv.ParseFloat(strings.ReplaceAll(f, ",", "."), 64)
		sum += v
	}
	return cpuSample{sum / float64(runtime.NumCPU())}
}

func cpuPercent(_, b cpuSample) (float64, bool) {
	if b.percent < 0 {
		return 0, false
	}
	return min(b.percent, 100), true
}

// kern.memorystatus_level は「空いている割合」(macOS がメモリの余裕として使う値)
func memoryPercent() (float64, bool) {
	out, err := exec.Command("sysctl", "-n", "kern.memorystatus_level").Output()
	if err != nil {
		return 0, false
	}
	free, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, false
	}
	return 100 - free, true
}

var batteryPct = regexp.MustCompile(`(\d+)%`)

func batteryLevel() (pct int, charging, ok bool) {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil || !strings.Contains(string(out), "InternalBattery") {
		return 0, false, false
	}
	m := batteryPct.FindStringSubmatch(string(out))
	if m == nil {
		return 0, false, false
	}
	pct, _ = strconv.Atoi(m[1])
	return pct, strings.Contains(string(out), "'AC Power'"), true
}
