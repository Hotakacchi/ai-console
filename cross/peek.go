package main

// バックグラウンドで呼ばれたときに、画面右下から顔がぴょこっと出てくる小さな窓。
// 窓は右下に置いたまま、中身を画面側 (peek モードの app.js) で動かす。

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

const peekW, peekH = 260, 220

type Peek struct {
	app    *application.App
	win    *application.WebviewWindow
	popped bool
}

func newPeek(app *application.App) *Peek {
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           "peek",
		Title:          "ルミ",
		Width:          peekW,
		Height:         peekH,
		URL:            "/#peek",
		Frameless:      true,
		AlwaysOnTop:    true,
		Hidden:         true,
		DisableResize:  true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
			// Windows 11 が枠なしの窓に付ける縁取りと影を付けない
			DisableFramelessWindowDecorations: true,
		},
	})
	return &Peek{app: app, win: win}
}

// 画面の右下 (作業領域の内側) に置く
func (p *Peek) place() {
	if s := p.app.Screen.GetPrimary(); s != nil {
		wa := s.WorkArea
		p.win.SetPosition(wa.X+wa.Width-peekW-12, wa.Y+wa.Height-peekH)
	}
}

func (p *Peek) pop(text string) {
	p.place()
	p.popped = true
	p.win.Show()
	p.win.EmitEvent("peek", map[string]any{"mode": "pop", "text": text})
}

// 引っ込む。sleepy なら眠そうな顔でゆっくり沈む。終わったら画面側から peekDone が来る
func (p *Peek) retract(sleepy bool, text string) {
	if !p.popped {
		return
	}
	p.popped = false
	mode := "retract"
	if sleepy {
		mode = "sleepy"
	}
	p.win.EmitEvent("peek", map[string]any{"mode": mode, "text": text})
}

func (p *Peek) hide() {
	if !p.popped {
		p.win.Hide()
	}
}
