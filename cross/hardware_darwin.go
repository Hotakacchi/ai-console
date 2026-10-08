package main

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func detectSpec() pcSpec {
	var s pcSpec
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		s.RAM, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	}
	// Apple シリコンはメモリを GPU と分け合う。Intel の Mac は GPU を数えない (llama.cpp の Mac 版は CPU で動かす)
	s.Unified = runtime.GOARCH == "arm64"
	return s
}
