package main

// 画面操作: AI がほかのアプリのボタンを押したり、文字を打ったりする (<ui>)。
//   <ui>
//   click 保存           画面に出ている文字の場所をクリック (OCR で探す)
//   click @640,360       画面の座標をクリック
//   doubleclick / rightclick  (click と同じ書き方)
//   type こんにちは       文字を打つ
//   key ctrl+s           キーを押す
//   scroll down 3        スクロール
//   wait 1               待つ (秒、5 まで)
//   </ui>
// 自動モードでも必ず、操作の一覧を見せて許可を取ってから行う (設定 ui_control で止められる)。
// 操作中にユーザーがマウスを動かしたら、そこで止める。終わったら、操作のあとの画面の文字を AI に返す。

import (
	"errors"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"
)

type uiAction struct {
	Kind string // click / doubleclick / rightclick / type / key / scroll / wait
	Arg  string
}

// AI が日本語や別の書き方で書いたときの言い換え
var uiVerbAliases = map[string]string{
	"クリック": "click", "ダブルクリック": "doubleclick", "右クリック": "rightclick", "入力": "type", "キー": "key",
	"スクロール": "scroll", "待つ": "wait", "double-click": "doubleclick", "right-click": "rightclick", "press": "key",
	"write": "type", "sleep": "wait", "dblclick": "doubleclick",
}

// 「@640,360」のような座標
func parsePoint(s string) (float64, float64, bool) {
	s, ok := strings.CutPrefix(strings.TrimSpace(s), "@")
	if !ok {
		return 0, 0, false
	}
	x, y, ok := strings.Cut(strings.ReplaceAll(s, " ", ""), ",")
	if !ok {
		return 0, 0, false
	}
	fx, err1 := strconv.ParseFloat(x, 64)
	fy, err2 := strconv.ParseFloat(y, 64)
	return fx, fy, err1 == nil && err2 == nil
}

// <ui> の中身を操作の並びにする (1 行に 1 つ)
func parseUIActions(body string) ([]uiAction, error) {
	var out []uiAction
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r", ""), "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-・*0123456789.)"))
		if line == "" {
			continue
		}
		verb, arg, _ := strings.Cut(strings.Replace(line, "　", " ", 1), " ")
		verb, arg = strings.ToLower(strings.TrimSpace(verb)), strings.TrimSpace(arg)
		if v, ok := uiVerbAliases[verb]; ok {
			verb = v
		}
		arg = strings.Trim(arg, "\"「」『』")
		switch verb {
		case "click", "doubleclick", "rightclick", "type", "key":
			if arg == "" {
				return nil, errors.New(T("ui.badLine", line))
			}
		case "scroll":
			if arg == "" {
				arg = "down"
			}
		case "wait":
			if arg == "" {
				arg = "1"
			}
		default:
			return nil, errors.New(T("ui.badLine", line))
		}
		out = append(out, uiAction{verb, arg})
	}
	if len(out) == 0 {
		return nil, errors.New(T("ui.empty"))
	}
	if len(out) > 30 {
		return nil, errors.New(T("ui.tooMany", 30))
	}
	return out, nil
}

// 確認に出す、操作の説明
func (a uiAction) describe() string {
	switch a.Kind {
	case "click", "doubleclick", "rightclick":
		where := T("ui.where.text", a.Arg)
		if x, y, ok := parsePoint(a.Arg); ok {
			where = T("ui.where.point", int(x), int(y))
		}
		return T("ui.do."+a.Kind, where)
	case "type":
		return T("ui.do.type", a.Arg)
	case "key":
		return T("ui.do.key", a.Arg)
	case "scroll":
		return T("ui.do.scroll", a.Arg)
	case "wait":
		return T("ui.do.wait", a.Arg)
	}
	return a.Kind + " " + a.Arg
}

// AI の <ui>: 許可を取ってから画面を操作し、結果とそのあとの画面の文字を返す
func (l *Lumi) uiTool(r toolRequest) string {
	if !l.s.On("ui_control", "on") {
		return T("ui.off") + "\n"
	}
	if !l.gui() {
		return T("ui.needWindow") + "\n"
	}
	actions, err := parseUIActions(r.Command)
	if err != nil {
		l.write(err.Error()+"\n", "yellow")
		return err.Error() + "\n"
	}
	if err := uiReady(); err != nil {
		l.errorText(err.Error())
		return err.Error() + "\n"
	}
	// 自動モードでも必ず聞く
	var list strings.Builder
	for i, a := range actions {
		fmt.Fprintf(&list, "  %d. %s\n", i+1, a.describe())
	}
	l.write(T("ui.plan")+"\n"+list.String(), "yellow")
	if a := strings.ToLower(l.ask(T("ui.ask"))); a != "y" && a != "a" {
		l.write(T("web.stopped")+"\n", "dim")
		return T("web.denied") + "\n"
	}
	l.taint() // 画面の文字も、外から来たもの

	// ルミの窓は隠して、操作する画面が見えるようにする
	visible := l.win.IsVisible()
	if visible {
		l.win.Hide()
		time.Sleep(400 * time.Millisecond)
	}
	done, failed := l.runUIActions(actions)
	time.Sleep(800 * time.Millisecond)
	after := ""
	if img, err := captureDisplay(); err == nil {
		after, _ = ocrImage(img, currentLang())
	}
	if visible {
		l.show()
	}

	var report strings.Builder
	report.WriteString(T("ui.result", done, len(actions)) + "\n")
	if failed != "" {
		report.WriteString(failed + "\n")
		l.write(failed+"\n", "yellow")
	}
	l.write(T("ui.result", done, len(actions))+"\n\n", "cyan")
	if after != "" {
		report.WriteString("[" + T("ui.after") + "]\n" + clipOCR(after, 3000) + "\n")
	}
	return report.String()
}

// 操作を順に行う。できた数と、止まったときの理由を返す
func (l *Lumi) runUIActions(actions []uiAction) (int, string) {
	var screen image.Rectangle
	lastX, lastY := uiCursor()
	for i, a := range actions {
		if l.cancelled() {
			return i, T("ui.cancelled")
		}
		// ユーザーがマウスを動かしていたら、そこで止める (操作を取り戻せるように)
		if x, y := uiCursor(); abs(x-lastX) > 30 || abs(y-lastY) > 30 {
			return i, T("ui.userMoved")
		}
		var err error
		switch a.Kind {
		case "click", "doubleclick", "rightclick":
			var x, y float64
			px, py, isPoint := parsePoint(a.Arg)
			img, cerr := captureDisplay()
			if cerr != nil {
				return i, T("screen.failed", cerr.Error())
			}
			screen = img.Bounds()
			if isPoint {
				x, y = px+float64(screen.Min.X), py+float64(screen.Min.Y)
			} else {
				lines, oerr := ocrRead(img, currentLang())
				if oerr != nil {
					return i, T("ocr.failed", oerr.Error())
				}
				var n int
				if x, y, n = findText(lines, a.Arg); n == 0 {
					return i, T("ui.notFound", a.Arg)
				}
			}
			sx, sy := uiScreenPoint(screen, x, y)
			if err = uiMoveTo(sx, sy); err == nil {
				time.Sleep(120 * time.Millisecond)
				button := "left"
				if a.Kind == "rightclick" {
					button = "right"
				}
				err = uiClick(button, a.Kind == "doubleclick")
			}
		case "type":
			err = uiType(a.Arg)
		case "key":
			err = uiKey(a.Arg)
		case "scroll":
			dir, num, _ := strings.Cut(a.Arg, " ")
			n, _ := strconv.Atoi(strings.TrimSpace(num))
			n = max(1, min(n, 20))
			if strings.HasPrefix(strings.ToLower(dir), "up") || dir == "上" {
				err = uiScroll(n)
			} else {
				err = uiScroll(-n)
			}
		case "wait":
			sec, _ := strconv.ParseFloat(a.Arg, 64)
			time.Sleep(time.Duration(max(0.1, min(sec, 5)) * float64(time.Second)))
		}
		if err != nil {
			return i, T("ui.failed", a.describe(), err.Error())
		}
		time.Sleep(300 * time.Millisecond) // 画面が変わるのを少し待つ
		lastX, lastY = uiCursor()
	}
	return len(actions), ""
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
