package main

// バックグラウンドで呼ばれたときに、画面の端から顔がぴょこっと出てくる小さな窓。
// 出てくる場所は peek_position で選ぶ (右下・左下・上など。custom はドラッグで決めた場所)。
// 窓はその場所に置いたまま、中身を画面側 (peek モードの app.js) で動かす。

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const peekW, peekH = 260, 220

type Peek struct {
	app    *application.App
	win    *application.WebviewWindow
	popped bool
	moving bool                          // /peek move で場所を決めている途中
	where  func() (pos string, x, y int) // 出てくる場所の設定 (Lumi が渡す)
}

func newPeek(app *application.App) *Peek {
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "peek",
		Title:            T("app.title"),
		Width:            peekW,
		Height:           peekH,
		URL:              "/#peek",
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		DisableResize:    true,
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

// 設定の場所に置き、上下どちらから出てくるかを返す ("top" / "bottom")
func (p *Peek) place() string {
	pos, cx, cy := "bottom-right", 0, 0
	if p.where != nil {
		pos, cx, cy = p.where()
	}
	if pos == "custom" {
		// ドラッグで決めた場所。今つないでいる画面の中にあるときだけ使う
		for _, s := range p.app.Screen.GetAll() {
			wa := s.WorkArea
			if mx, my := cx+peekW/2, cy+peekH/2; mx >= wa.X && mx < wa.X+wa.Width && my >= wa.Y && my < wa.Y+wa.Height {
				p.win.SetPosition(cx, cy)
				if my < wa.Y+wa.Height/2 {
					return "top"
				}
				return "bottom"
			}
		}
		pos = "bottom-right"
	}
	s := p.app.Screen.GetPrimary()
	if s == nil {
		return "bottom"
	}
	x, y, from := peekSpot(pos, s.WorkArea)
	p.win.SetPosition(x, y)
	return from
}

// 選んだ場所の左上の座標と、出てくる向き
func peekSpot(pos string, wa application.Rect) (int, int, string) {
	const margin = 12
	x := wa.X + wa.Width - peekW - margin // 右
	if strings.HasSuffix(pos, "-left") {
		x = wa.X + margin
	} else if strings.HasSuffix(pos, "-center") {
		x = wa.X + (wa.Width-peekW)/2
	}
	if strings.HasPrefix(pos, "top") {
		return x, wa.Y, "top"
	}
	return x, wa.Y + wa.Height - peekH, "bottom"
}

func (p *Peek) pop(text string) {
	from := p.place()
	p.popped = true
	p.win.Show()
	p.win.EmitEvent("peek", map[string]any{"mode": "pop", "text": text, "from": from})
}

// /peek move: 窓を出したままにして、ドラッグで場所を決めてもらう (ダブルクリックで決定)
func (p *Peek) startMove(hint string) {
	from := p.place()
	p.popped = true
	p.moving = true
	p.win.Show()
	p.win.EmitEvent("peek", map[string]any{"mode": "move", "text": hint, "from": from})
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
