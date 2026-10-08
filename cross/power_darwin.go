package main

import (
	"os/exec"
	"strings"
	"time"
)

// pmset を動かすので、ほかより少しゆっくり見る
const powerPoll = 4 * time.Second

// 電源につながっているか (pmset -g batt の 1 行目: Now drawing from 'AC Power' / 'Battery Power')。
// バッテリーのない Mac (iMac など) は ok=false
func onACPower() (bool, bool) {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return false, false
	}
	s := string(out)
	if !strings.Contains(s, "InternalBattery") {
		return false, false
	}
	return strings.Contains(s, "'AC Power'"), true
}
