package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const powerPoll = 2 * time.Second

// 電源につながっているか (/sys/class/power_supply の Mains の online)。バッテリーのない PC は ok=false
func onACPower() (bool, bool) {
	dirs, _ := filepath.Glob("/sys/class/power_supply/*")
	battery, mains, online := false, false, false
	for _, d := range dirs {
		kind, _ := os.ReadFile(filepath.Join(d, "type"))
		switch strings.TrimSpace(string(kind)) {
		case "Battery":
			battery = true
		case "Mains":
			mains = true
			if v, _ := os.ReadFile(filepath.Join(d, "online")); strings.TrimSpace(string(v)) == "1" {
				online = true
			}
		}
	}
	if !battery || !mains {
		return false, false
	}
	return online, true
}
