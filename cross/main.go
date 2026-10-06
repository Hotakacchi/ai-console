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

	// 窓やトレイの文言を作る前に言語を決める
	loadLocales()
	setLanguage(LoadSettings().Get("language", "auto"))

	app := application.New(application.Options{
		Name:        "Lumi",
		Description: T("app.description"),
		Icon:        icon,
		Assets:      application.AssetOptions{Handler: withVoiceFiles(application.BundledAssetFileServer(assets))},
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
	lumi = newLumi(app, slices.Contains(args, "--mute"), slices.Contains(args, "--no-mic"))
	lumi.installLocalOnStart = slices.Contains(args, "--install-local")
	if i := slices.Index(args, "--script"); i >= 0 && i+1 < len(args) {
		lumi.scriptPath = args[i+1]
	}

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            T("app.title"),
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
				log.Println("Lumi keeps running in the tray")
			}
			return
		}
		lumi.quit()
	})

	// トレイのアイコンとメニュー
	tray := app.SystemTray.New()
	tray.SetIcon(icon)
	buildTray := func() {
		tray.SetTooltip(T("app.title"))
		menu := application.NewMenu()
		menu.Add(T("tray.show")).OnClick(func(*application.Context) { lumi.show() })
		menu.Add(T("tray.mic")).OnClick(func(*application.Context) {
			// 返事やダウンロードの最中は、画面からの入力と同じく受け付けない
			if !lumi.isBusy() {
				lumi.submit("/mic")
			}
		})
		menu.Add(T("tray.peekDemo")).OnClick(func(*application.Context) { lumi.demoPeek() })
		menu.AddSeparator()
		menu.Add(T("tray.quit")).OnClick(func(*application.Context) { lumi.quit() })
		tray.SetMenu(menu)
	}
	buildTray()
	lumi.onLanguageChanged = buildTray
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
	app.Event.On("voiceState", func(e *application.CustomEvent) {
		m, _ := e.Data.(map[string]any)
		lumi.voiceState(asString(m["state"]), asString(m["message"]))
	})
	app.Event.On("voiceWoke", func(*application.CustomEvent) { lumi.voiceWoke() })
	app.Event.On("voiceDebug", func(e *application.CustomEvent) {
		// 聞き取った文をそのまま出す (呼びかけがうまく反応しないときの調整用)
		if lumi.s.Get("voice_debug", "off") == "on" {
			lumi.write("  [voice] "+asString(e.Data)+"\n", "dim")
		}
	})
	app.Event.On("voiceHeard", func(e *application.CustomEvent) { lumi.voiceHeard(asString(e.Data)) })
	app.Event.On("voiceTimeout", func(*application.CustomEvent) { lumi.voiceTimeout() })
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
		l.peek.pop(T("peek.hey"))
		time.Sleep(2800 * time.Millisecond)
		l.peek.retract(false, T("peek.ok"))
		time.Sleep(300 * time.Millisecond)
		l.show()
	}()
}
