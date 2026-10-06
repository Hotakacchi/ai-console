package main

// ルミ (クロスプラットフォーム版) の入口。Wails v3 で Windows / macOS / Linux の画面を作る。

import (
	"embed"
	_ "embed"
	"log"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed assets
var assets embed.FS

//go:embed assets/icon.png
var icon []byte

func main() {
	args := os.Args[1:]
	var lumi *Lumi

	app := application.New(application.Options{
		Name:        "Lumi",
		Description: "顔のあるコンソール風アシスタント",
		Icon:        icon,
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		Mac: application.MacOptions{
			// ウィンドウを閉じてもトレイ (メニューバー) で動き続ける
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// すでに起動していたら、そちらのウィンドウを出して終わる
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.github.hotakacchi.lumi",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if lumi != nil {
					lumi.show()
				}
			},
		},
	})
	lumi = newLumi(app, slices.Contains(args, "--mute"))

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "ルミ",
		Width:            1000,
		Height:           640,
		MinWidth:         480,
		MinHeight:        300,
		URL:              "/",
		Hidden:           slices.Contains(args, "--background"),
		BackgroundColour: application.NewRGB(12, 12, 12),
		Windows:          application.WindowsWindow{Theme: application.Dark},
	})
	lumi.win = win
	lumi.updateTitle()
	lumi.peek = newPeek(app)

	// × で閉じたときは、設定が on ならトレイに隠れて動き続ける
	toldAboutTray := false
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if lumi.s.On("background", "on") {
			e.Cancel()
			win.Hide()
			if !toldAboutTray {
				toldAboutTray = true
				log.Println("ルミはトレイで動いています")
			}
			return
		}
		lumi.quit()
	})

	// トレイのアイコンとメニュー
	tray := app.SystemTray.New()
	tray.SetIcon(icon)
	tray.SetTooltip("ルミ")
	menu := application.NewMenu()
	menu.Add("表示").OnClick(func(*application.Context) { lumi.show() })
	menu.Add("呼ばれたときの動きを試す").OnClick(func(*application.Context) { lumi.demoPeek() })
	menu.AddSeparator()
	menu.Add("終了").OnClick(func(*application.Context) { lumi.quit() })
	tray.SetMenu(menu)
	if runtime.GOOS != "darwin" {
		tray.OnClick(func() { lumi.show() })
	}

	// 画面からのイベント
	app.Event.On("ready", func(*application.CustomEvent) { lumi.ready() })
	app.Event.On("submit", func(e *application.CustomEvent) { lumi.submit(asString(e.Data)) })
	app.Event.On("answer", func(e *application.CustomEvent) {
		select {
		case lumi.answers <- asString(e.Data):
		default:
		}
	})
	app.Event.On("interrupt", func(*application.CustomEvent) { lumi.interrupt() })
	app.Event.On("spoken", func(e *application.CustomEvent) {
		if f, ok := e.Data.(float64); ok {
			select {
			case lumi.spoken <- int(f):
			default:
			}
		}
	})
	app.Event.On("voices", func(e *application.CustomEvent) {
		m, _ := e.Data.(map[string]any)
		var names []string
		if list, ok := m["names"].([]any); ok {
			for _, n := range list {
				names = append(names, asString(n))
			}
		}
		lumi.showVoices(names, asString(m["current"]))
	})
	app.Event.On("peekDone", func(*application.CustomEvent) { lumi.peek.hide() })
	app.Event.On("peekClicked", func(*application.CustomEvent) {
		lumi.peek.retract(false, "")
		lumi.show()
	})

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		lumi.applyStartup()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	localServer.Stop()
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// トレイからウィンドウを出して前に持ってくる
func (l *Lumi) show() {
	l.win.Show()
	l.win.Restore()
	l.win.Focus()
}

// startup の設定に合わせて、ログイン時の自動起動を登録・解除する
func (l *Lumi) applyStartup() {
	if l.s.Get("startup", "off") == "on" {
		l.app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: "Lumi", Arguments: []string{"--background"}})
	} else if on, _ := l.app.Autostart.IsEnabled(); on {
		l.app.Autostart.Disable()
	}
}

// 呼ばれたときのアニメーションを試す (ウィンドウを隠して、右下から顔を出す)
func (l *Lumi) demoPeek() {
	l.win.Hide()
	go func() {
		time.Sleep(700 * time.Millisecond)
		l.peek.pop("なあに？")
		time.Sleep(2800 * time.Millisecond)
		l.peek.retract(false, "はーい！")
		time.Sleep(300 * time.Millisecond)
		l.show()
	}()
}
