package main

// ルミ (クロスプラットフォーム版) の入口。Wails v3 で Windows / macOS / Linux の画面を作る。

import (
	"embed"
	"encoding/json"
	_ "embed"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"
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

	// ターミナル版 (窓を開かずに、端末の中で話す)
	if wantCLI(args) {
		runCLI()
		return
	}

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
		EnableFileDrop:   true, // ファイルを落として読ませる
	})
	lumi.win = win
	cleanupOldExe()
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		go lumi.attachFiles(e.Context().DroppedFiles())
	})
	lumi.updateTitle()
	lumi.peek = newPeek(app)
	lumi.peek.where = func() (string, int, int) {
		return lumi.s.Get("peek_position", "bottom-right"), lumi.s.GetInt("peek_x", 0), lumi.s.GetInt("peek_y", 0)
	}

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
	app.Event.On("submit", func(e *application.CustomEvent) { lumi.submitTyped(asString(e.Data)) })
	app.Event.On("answer", func(e *application.CustomEvent) {
		select {
		case lumi.answers <- asString(e.Data):
		default:
		}
	})
	app.Event.On("interrupt", func(*application.CustomEvent) { lumi.interrupt() })
	// 話しかけた内容の音を whisper.cpp で書き起こす (Windows・Linux)
	app.Event.On("whisperNative", func(e *application.CustomEvent) { go lumi.nativeWhisper(e.Data) })
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
	// タブ (シェルのタブを + で開いて × で閉じる)
	tabArg := func(e *application.CustomEvent) (int, map[string]any) {
		m, _ := e.Data.(map[string]any)
		id, _ := m["tab"].(float64)
		return int(id), m
	}
	app.Event.On("tabOpen", func(e *application.CustomEvent) {
		id, m := tabArg(e)
		lumi.tabOpen(id, asString(m["shell"]))
	})
	app.Event.On("tabSubmit", func(e *application.CustomEvent) {
		id, m := tabArg(e)
		lumi.tabSubmit(id, asString(m["text"]))
	})
	app.Event.On("tabInterrupt", func(e *application.CustomEvent) { id, _ := tabArg(e); lumi.tabInterrupt(id) })
	app.Event.On("tabClose", func(e *application.CustomEvent) { id, _ := tabArg(e); lumi.tabClose(id) })
	// --script の "#tab dump <ファイル>" (テスト用): 画面のタブの様子をファイルに書く
	app.Event.On("testDump", func(e *application.CustomEvent) {
		if lumi.dumpPath != "" {
			b, _ := json.MarshalIndent(e.Data, "", "  ")
			os.WriteFile(lumi.dumpPath, b, 0o644)
		}
	})
	app.Event.On("voiceState", func(e *application.CustomEvent) {
		m, _ := e.Data.(map[string]any)
		lumi.voiceState(asString(m["state"]), asString(m["message"]))
	})
	app.Event.On("voiceWoke", func(*application.CustomEvent) { lumi.voiceWoke() })
	app.Event.On("voiceWhisperError", func(e *application.CustomEvent) {
		// Whisper が使えなかった (Vosk の結果で続ける)。何度も出さないように最初の 1 回だけ
		if !lumi.whisperErrorShown {
			lumi.whisperErrorShown = true
			lumi.errorText(T("whisper.error", asString(e.Data)))
		}
	})
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
	// /peek move でドラッグして、ダブルクリックで決めた場所を覚える
	// Shift+Tab: 自動モードの切り替え
	app.Event.On("toggleAuto", func(*application.CustomEvent) { lumi.autoCommand("") })
	app.Event.On("peekMoved", func(*application.CustomEvent) { lumi.savePeekPlace() })

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		lumi.applyStartup()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	localServer.Stop()
	voicevox.Stop()
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// トレイからウィンドウを出して前に持ってくる
func (l *Lumi) show() {
	if !l.gui() {
		return
	}
	l.win.Show()
	l.win.Restore()
	l.win.Focus()
}

// startup の設定に合わせて、ログイン時の自動起動を登録・解除する
func (l *Lumi) applyStartup() {
	if !l.gui() {
		return
	}
	if l.s.Get("startup", "off") == "on" {
		l.app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: "Lumi", Arguments: []string{"--background"}})
	} else if on, _ := l.app.Autostart.IsEnabled(); on {
		l.app.Autostart.Disable()
	}
}

// 呼ばれたときのアニメーションを試す (ウィンドウを隠して、右下から顔を出す)
// ドラッグで決めた小窓の場所を保存する
func (l *Lumi) savePeekPlace() {
	if !l.peek.moving {
		return
	}
	l.peek.moving = false
	x, y := l.peek.win.Position()
	if l.s.Err == nil {
		l.s.Set("peek_position", "custom")
		l.s.Set("peek_x", x)
		l.s.Set("peek_y", y)
	}
	l.peek.retract(false, T("peek.ok"))
	l.info(T("peek.saved"))
}

// /peek [move]
func (l *Lumi) peekCommand(arg string) {
	if !l.gui() {
		l.info(T("cli.noWindow"))
		return
	}
	if strings.EqualFold(strings.TrimSpace(arg), "move") {
		l.peek.startMove(T("peek.moveHint"))
		l.info(T("peek.moving"))
		return
	}
	l.demoPeek()
}

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
