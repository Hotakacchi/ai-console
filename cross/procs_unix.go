//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// 動いているプロセス (ps で、メモリと CPU の時間もまとめて取れる)
func listProcs(want func(p procInfo) bool) []procInfo {
	out, err := exec.Command("ps", "-A", "-o", "pid=,rss=,time=,comm=").Output()
	if err != nil {
		return nil
	}
	var list []procInfo
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		rss, _ := strconv.ParseUint(f[1], 10, 64)
		p := procInfo{PID: pid, Name: filepath.Base(strings.Join(f[3:], " ")), Mem: rss * 1024, CPU: psTime(f[2])}
		list = append(list, p)
	}
	return list
}

// ps の TIME ([[日-]時:]分:秒) を秒に
func psTime(s string) float64 {
	days := 0.0
	if d, rest, ok := strings.Cut(s, "-"); ok {
		v, _ := strconv.ParseFloat(d, 64)
		days, s = v, rest
	}
	total := 0.0
	for _, part := range strings.Split(s, ":") {
		v, _ := strconv.ParseFloat(part, 64)
		total = total*60 + v
	}
	return days*86400 + total
}
