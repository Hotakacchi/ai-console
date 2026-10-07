package main

// USB メモリなどのドライブがつながったら、驚いた顔をして知らせる。
// 数秒ごとにドライブの一覧を見比べるだけ (Windows はドライブ文字、Mac は /Volumes、Linux は /media など)。
// 一覧は軽く取れるものだけにして、名前 (ボリュームラベル) は増えたドライブのときだけ調べる
// (つながらないネットワークドライブなどは、名前を調べると待たされるため)。

import (
	"sort"
	"time"
)

const drivePoll = 2 * time.Second

func (l *Lumi) watchDrives() {
	known := driveSet()
	for range time.Tick(drivePoll) {
		now := driveSet()
		var added []string
		for id := range now {
			if !known[id] {
				added = append(added, id)
			}
		}
		sort.Strings(added)
		for _, id := range added {
			l.fx("surprised", 2.5)
			l.info(T("drive.added", driveName(id), id))
		}
		known = now
	}
}

func driveSet() map[string]bool {
	m := map[string]bool{}
	for _, id := range listDrives() {
		m[id] = true
	}
	return m
}
