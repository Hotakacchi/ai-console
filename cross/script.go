package main

// 開発・テスト用: --script <ファイル> で起動すると、ファイルの各行を打ち込んだのと同じように順に送る。
// 前の返事が終わるのを待ってから次の行を送る。"#wait 秒" の行はその秒数だけ待つ。

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func (l *Lumi) runScript(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		l.errorText(err.Error())
		return
	}
	go func() {
		time.Sleep(time.Second)
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if rest, ok := strings.CutPrefix(line, "#wait"); ok {
				if n, err := strconv.ParseFloat(strings.TrimSpace(rest), 64); err == nil {
					time.Sleep(time.Duration(n * float64(time.Second)))
				}
				continue
			}
			for l.isBusy() {
				time.Sleep(200 * time.Millisecond)
			}
			l.emit("echo", line) // 画面に打ち込んだ行として出す
			l.submitTyped(line)
			time.Sleep(300 * time.Millisecond)
		}
	}()
}
