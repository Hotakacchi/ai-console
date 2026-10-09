package main

// 長い作業を任せるモード: /task <目標> で、AI が手順を立てて (<plan>)、一歩ずつ進め (<step>番号</step>)、
// 終わったらまとめる (<done>まとめ</done>)。手順はチェックリストで画面に出す。
// 往復はふだんの 5 回より多い 25 回まで。道具を使わずに止まっても、手順が残っていれば「続けて」と促す。
// Ctrl+C でいつでも止められ、コマンドの確認はふだんどおり (自動モードなら確認なし)。

import (
	"strconv"
	"strings"
	"sync"
)

const taskMaxRounds = 25

type taskState struct {
	mu    sync.Mutex
	goal  string
	steps []string
	done  map[int]bool
	fin   bool
}

func (t *taskState) remaining() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for i := range t.steps {
		if !t.done[i+1] {
			n++
		}
	}
	return n
}

func (t *taskState) finished() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fin
}

// /task <目標>
func (l *Lumi) taskCommand(goal string) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		l.info(T("task.how"))
		return
	}
	if !l.begin() {
		return
	}
	l.mu.Lock()
	l.task = &taskState{goal: goal, done: map[int]bool{}}
	l.mu.Unlock()
	l.write(T("task.start", goal)+"\n", "cyan")
	go func() {
		defer func() {
			l.mu.Lock()
			l.task = nil
			l.mu.Unlock()
		}()
		l.respond(T("task.instruction", goal), false)
	}()
}

func (l *Lumi) currentTask() *taskState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.task
}

// <plan>: 手順を覚えて、チェックリストで出す
func (l *Lumi) planTag(r toolRequest) {
	t := l.currentTask()
	if t == nil {
		return
	}
	var steps []string
	for _, line := range strings.Split(r.Command, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "0123456789.)-・ "))
		if line != "" {
			steps = append(steps, line)
		}
	}
	t.mu.Lock()
	t.steps = steps
	t.mu.Unlock()
	l.write("\n"+T("task.plan")+"\n", "cyan")
	for i, s := range steps {
		l.write("  ☐ "+strconv.Itoa(i+1)+". "+s+"\n", "white")
	}
	l.write("\n", "fg")
}

// <step>番号</step>: その手順を終わったことにする
func (l *Lumi) stepTag(r toolRequest) {
	t := l.currentTask()
	if t == nil {
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(r.Command))
	t.mu.Lock()
	if err != nil || n < 1 || n > len(t.steps) {
		t.mu.Unlock()
		return
	}
	t.done[n] = true
	name := t.steps[n-1]
	total, left := len(t.steps), 0
	for i := range t.steps {
		if !t.done[i+1] {
			left++
		}
	}
	t.mu.Unlock()
	l.write("  ☑ "+strconv.Itoa(n)+". "+name+"  ("+T("task.progress", total-left, total)+")\n", "green")
}

// <done>まとめ</done>
func (l *Lumi) doneTag(r toolRequest) {
	t := l.currentTask()
	if t == nil {
		return
	}
	t.mu.Lock()
	t.fin = true
	t.mu.Unlock()
	l.write("\n✔ "+T("task.finished")+"\n", "green")
	if s := strings.TrimSpace(r.Command); s != "" {
		l.write(s+"\n", "fg")
	}
	l.emit("flash", map[string]any{"expr": "happy", "seconds": 3})
}

// 返事の繰り返しの上限 (作業モードなら多め)
func (l *Lumi) maxRounds() int {
	if l.currentTask() != nil {
		return taskMaxRounds
	}
	return maxToolRounds
}

// 道具を使わずに止まったとき、作業が残っていれば続きを促す文を返す ("" なら終わり)
func (l *Lumi) taskNudge() string {
	t := l.currentTask()
	if t == nil || t.finished() {
		return ""
	}
	if t.remaining() == 0 && len(t.steps) > 0 {
		return T("task.wrapUp")
	}
	return T("task.continue")
}
