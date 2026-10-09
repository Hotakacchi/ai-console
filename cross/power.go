package main

// 充電を始めたら (電源につないだら)、顔に充電のアニメーションを出す。
// 数秒ごとに電源の状態を見て、「バッテリー → 電源」に変わったときだけ。バッテリーのない PC では何もしない。

import "time"

func (l *Lumi) watchPower() {
	prev, ok := onACPower()
	for range time.Tick(powerPoll) {
		now, okNow := onACPower()
		if ok && okNow && now && !prev {
			l.fx("charge", 3)
			l.fireTrigger("charge", nil) // 自動化 (@充電)
		}
		prev, ok = now, okNow
	}
}
