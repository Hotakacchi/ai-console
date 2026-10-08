//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func systemDrive() string { return "/" }

func diskSpace() (free, total uint64, ok bool) {
	var st syscall.Statfs_t
	if syscall.Statfs("/", &st) != nil {
		return 0, 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), uint64(st.Blocks) * uint64(st.Bsize), true
}

// いちばんメモリを使っているアプリ (ps の RSS。同じ名前は合計)
func topMemoryProcess() (string, uint64) {
	out, err := exec.Command("ps", "-A", "-o", "rss=,comm=").Output()
	if err != nil {
		return "", 0
	}
	sum := map[string]uint64{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		kb, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		sum[filepath.Base(strings.Join(f[1:], " "))] += kb * 1024
	}
	return topOf(sum)
}
