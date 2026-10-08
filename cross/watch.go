package main

// PC の見守り: CPU・メモリが張り付いたり、ディスクの空きやバッテリーが少なくなったら、困った顔で知らせる。
//   CPU・メモリ … 10 秒ごとに測り、1 分間ずっと 90% を超えていたら (メモリはいちばん使っているアプリの名前も)
//   ディスク     … システムのドライブ (Windows は C:、Mac・Linux は /) の空きが 10GB を切ったら
//   バッテリー   … 充電していないときに 15% を切ったら (充電すれば、次にまた知らせる)
// 同じ知らせはしばらく繰り返さない。声は出さない (窓が隠れていれば小窓で顔を出す)。/set pc_watch off で止める。

import (
	"fmt"
	"time"
)

const (
	watchEvery    = 10 * time.Second
	watchSamples  = 6  // 1 分ぶん
	watchBusyPct  = 90 // CPU・メモリがこれを超えたら
	watchBattery  = 15 // バッテリーがこれを切ったら
	watchDiskFree = 10 * gb
)

var watchQuiet = map[string]time.Duration{"cpu": 30 * time.Minute, "memory": 30 * time.Minute, "disk": 6 * time.Hour}

func (l *Lumi) watchPC() {
	var cpu, mem []float64
	last := map[string]time.Time{}
	batteryTold := false
	tick := 0
	prevCPU := cpuTimes()
	for range time.Tick(watchEvery) {
		tick++
		if !l.s.On("pc_watch", "on") {
			cpu, mem = nil, nil
			continue
		}
		push := func(list []float64, v float64) []float64 {
			list = append(list, v)
			if len(list) > watchSamples {
				list = list[1:]
			}
			return list
		}
		now := cpuTimes()
		if p, ok := cpuPercent(prevCPU, now); ok {
			cpu = push(cpu, p)
		}
		prevCPU = now
		if p, ok := memoryPercent(); ok {
			mem = push(mem, p)
		}
		tell := func(kind, text string) {
			if time.Since(last[kind]) < watchQuiet[kind] {
				return
			}
			last[kind] = time.Now()
			go l.reportPC(text, "sad")
		}
		if allOver(cpu, watchBusyPct) {
			tell("cpu", T("watch.cpu", avg(cpu)))
			cpu = nil
		}
		if allOver(mem, watchBusyPct) {
			text := T("watch.memory", avg(mem))
			if name, size := topMemoryProcess(); name != "" {
				text += " " + T("watch.memoryTop", name, fmt.Sprintf("%.1fGB", float64(size)/gb))
			}
			tell("memory", text)
			mem = nil
		}
		// ディスクとバッテリーは 1 分ごとで十分
		if tick%6 == 1 {
			if free, total, ok := diskSpace(); ok && total > 0 && free < watchDiskFree {
				tell("disk", T("watch.disk", systemDrive(), fmt.Sprintf("%.1fGB", float64(free)/gb)))
			}
			if pct, charging, ok := batteryLevel(); ok {
				switch {
				case charging || pct > watchBattery:
					batteryTold = false
				case !batteryTold:
					batteryTold = true
					go l.reportPC(T("watch.battery", pct), "sad")
				}
			}
		}
	}
}

func allOver(list []float64, limit float64) bool {
	if len(list) < watchSamples {
		return false
	}
	for _, v := range list {
		if v < limit {
			return false
		}
	}
	return true
}

func avg(list []float64) float64 {
	sum := 0.0
	for _, v := range list {
		sum += v
	}
	return sum / float64(len(list))
}

// 顔 (expr) とメッセージで知らせる。返事の途中なら終わるのを待つ。窓が隠れていれば小窓で顔を出す
func (l *Lumi) reportPC(text, expr string) {
	for !l.begin() {
		time.Sleep(500 * time.Millisecond)
	}
	defer l.setBusy(false)
	popped := false
	if l.gui() && !l.win.IsVisible() {
		l.peek.pop(text)
		popped = true
	}
	l.emit("flash", map[string]any{"expr": expr, "seconds": 4})
	l.write(text+"\n\n", "yellow")
	if popped {
		time.Sleep(4 * time.Second)
		l.peek.retract(false, "")
	}
}

// いちばん大きいもの
func topOf(sum map[string]uint64) (string, uint64) {
	name, size := "", uint64(0)
	for n, s := range sum {
		if s > size {
			name, size = n, s
		}
	}
	return name, size
}
