package main

// ルミの中心: 入力を受け取り、コマンドを処理し、AI の返事を 1 文ずつ画面に喋らせる。
// 画面 (assets/app.js) とはイベントでやり取りする。
//   画面 → Go: submit(文字) / answer(確認への答え) / interrupt / ready / spoken(id)
//   Go → 画面: write / clear / busy / thinking / face / flash / ask / speak / commands / voices

import (
	"encoding/base64"
	"fmt"
	"math"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ビルド時に -ldflags "-X main.version=..." で上書きされる
var version = "1.3.0"

type Lumi struct {
	app   *application.App
	win   *application.WebviewWindow
	peek  *Peek
	mu    sync.Mutex
	s     *Settings
	ai    Provider
	muted bool

	installLocalOnStart bool // --install-local: 起動したらローカルAIをダウンロードする
	elevated            bool   // ルミ自体が管理者 (root) として動いている
	scriptPath          string // --script: テスト用に入力を流し込むファイル

	micOn     bool // 音声入力をこのセッションで使うか (--no-mic や /mic で切り替え)
	listening bool // 画面側で聞き取りが動いているか

	busy    bool
	cancel  chan struct{}
	answers chan string
	spoken  chan int
	speakID int

	onLanguageChanged func() // トレイのメニューなど、言語で変わるものを作り直す

	startOnce     sync.Once // リマインダーの見張りは 1 回だけ始める
	creditedStyle int       // VOICEVOX のクレジットを出した声
	ttsErrorShown bool
	whisperErrorShown bool
	replyText strings.Builder // 今の返事で喋った文 (履歴に残す)
	pending   []attachment    // 次の発言に付けるファイル (ドラッグ＆ドロップ・/attach)
	shellOn   bool            // シェルモード (打った行をそのままコマンドとして実行)
	shellDir  string          // コマンドを実行する場所 (cd で変わる)
}

func newLumi(app *application.App, muted, noMic bool) *Lumi {
	l := &Lumi{app: app, muted: muted, micOn: !noMic, elevated: isElevated(), creditedStyle: -1, answers: make(chan string, 1), spoken: make(chan int, 8)}
	memories.load()
	reminders.load()
	l.loadSettings()
	return l
}

// ---- 画面への出力 ----

func (l *Lumi) emit(name string, data any) {
	if l.win != nil {
		l.win.EmitEvent(name, data)
	}
	if phone.running() {
		phone.publish(name, data) // スマホでも見られるように
	}
}

func (l *Lumi) write(text, color string) { l.emit("write", map[string]any{"text": text, "color": color}) }
func (l *Lumi) info(text string)         { l.write(text+"\n\n", "dim") }
func (l *Lumi) errorText(text string)    { l.write(text+"\n\n", "red") }
func (l *Lumi) face(expr string)         { l.emit("face", expr) }

func (l *Lumi) isBusy() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.busy
}

func (l *Lumi) setBusy(b bool) {
	l.mu.Lock()
	l.busy = b
	l.mu.Unlock()
	l.emit("busy", b)
}

func (l *Lumi) updateTitle() {
	if l.win == nil {
		return
	}
	voice := "voice"
	if l.muted {
		voice = "mute"
	}
	if l.listening {
		voice += " | mic"
	}
	if l.elevated {
		voice += " | admin"
	}
	l.win.SetTitle(T("app.title") + "  —  " + l.ai.Label() + " | " + voice)
}

// ---- 設定 ----

func (l *Lumi) loadSettings() {
	l.s = LoadSettings()
	before := currentLang()
	setLanguage(l.s.Get("language", "auto"))
	l.ai = newProvider(l.s)
	l.updateTitle()
	if currentLang() != before {
		l.emit("i18n", clientMessages())
		if l.onLanguageChanged != nil {
			l.onLanguageChanged()
		}
	}
}

func (l *Lumi) wakeWord() string { return l.s.WakeWord() }

func (l *Lumi) voiceKey() string {
	return currentLang() + "|" + l.s.WakeWord() + "|" + l.s.Get("voice_input", "on") + "|" + l.s.Get("stt", "vosk") + "|" + l.s.Get("whisper_model", "base")
}

func (l *Lumi) reload(announce bool) {
	before := l.voiceKey()
	l.loadSettings()
	if l.voiceKey() != before {
		defer l.applyVoice(true)   // 言語・呼びかけ・オンオフが変わったときだけ聞き取りをやり直す
	}
	if l.s.Err != nil {
		l.errorText(T("settings.readError", l.s.Err.Error()))
	} else if announce {
		l.info(T("settings.reloaded", l.ai.Label()))
	}
	if !strings.HasPrefix(l.ai.Label(), "local:") {
		localServer.Stop()
	}
	l.sendVoiceSettings()
	l.sendAppearance()
	l.applyStartup()
	l.applyHotkey()
	l.applyPhone(false)
	l.restoreHistory(false) // AI を作り直したので、前の会話をもう一度渡す
	l.warmupLocal()
}

// 声の選び方と速さを画面に伝える (読み上げは画面側の音声合成で行う)
func (l *Lumi) sendVoiceSettings() {
	l.emit("voiceSettings", map[string]any{
		"voice": l.s.Get("voice", ""),
		"rate":  l.s.GetInt("voice_rate", 1),
		"muted": l.muted,
	})
}

// ---- 起動時 ----

// 画面の準備ができたら呼ばれる
func (l *Lumi) ready() {
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.name
	}
	l.emit("commands", names)
	l.emit("i18n", clientMessages())
	l.emit("admin", l.elevated)
	l.sendVoiceSettings()
	l.sendAppearance()
	l.write("Lumi Assistant [Version "+version+"]\n", "fg")
	l.write(T("welcome.hint")+"\n\n", "dim")
	if l.s.Err != nil {
		l.errorText(T("settings.readError", l.s.Err.Error()))
	}
	l.restoreHistory(true)
	l.applyHotkey()
	l.applyPhone(false)
	l.checkUpdate()
	l.startOnce.Do(func() {
		go l.runReminders()
		if l.scriptPath != "" {
			l.runScript(l.scriptPath)
		}
	})
	l.emit("flash", map[string]any{"expr": "happy", "seconds": 2.5})
	// インストーラーで「ローカルAIも入れる」を選んだときは、初回にダウンロードする
	if l.installLocalOnStart && !localInstalled(dataDir()) {
		l.installLocal()
	} else {
		l.warmupLocal()
	}
	l.applyVoice(true)
}

// ローカルAIなら、最初の返事を待たせないように先にモデルを読み込んでおく
func (l *Lumi) warmupLocal() {
	if !strings.HasPrefix(l.ai.Label(), "local:") {
		return
	}
	dir := dataDir()
	if !localInstalled(dir) {
		l.write(T("local.notInstalledHint", float64(localTotalSize())/1e9)+"\n\n", "yellow")
		return
	}
	l.write(T("local.starting")+"\n", "dim")
	go func() {
		p := l.ai
		if op, ok := p.(*thinkFiltered); ok && op.before != nil {
			if err := op.before(); err != nil {
				l.errorText(err.Error())
				return
			}
		}
		l.info(T("local.ready"))
	}()
}

// ---- 入力 ----

func (l *Lumi) submit(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if !strings.HasPrefix(text, "/") {
		// ルーティンの名前 (「おはよう」など) なら、それを動かす
		if r, ok := l.findRoutine(text); ok {
			if l.begin() {
				go l.runRoutine(r)
			}
			return
		}
		l.startReply(text)
		return
	}
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])
	arg := func(i int) string {
		if len(parts) > i {
			return parts[i]
		}
		return ""
	}
	switch cmd {
	case "/exit", "/quit":
		l.quit()
	case "/cls", "/clear":
		l.ai.Clear()
		l.takePending()
		clearHistory()
		l.emit("clear", nil)
	case "/memory":
		l.memoryCommand(parts[1:])
	case "/reminders":
		l.remindersCommand(parts[1:])
	case "/history":
		l.historyCommand(parts[1:])
	case "/update":
		l.updateCommand()
	case "/mute":
		l.muted = !l.muted
		l.updateTitle()
		l.sendVoiceSettings()
		if l.muted {
			l.info(T("mute.on"))
		} else {
			l.info(T("mute.off"))
		}
	case "/mic":
		l.toggleMic()
	case "/install-voice":
		l.installVoice()
	case "/voicetest":
		// 開発用 (一覧には出さない): データフォルダの voice/<ファイル> をマイクの代わりに聞かせる
		l.emit("voiceTest", map[string]any{"lang": currentLang(), "model": "/voice/" + currentLang() + "/model.tar.gz",
			"wake": l.s.WakeWord(), "file": "/voice/" + arg(1)})
	case "/reload":
		l.reload(true)
	case "/install-local":
		l.installLocal()
	case "/config":
		openFile(l.s.Path)
		l.info(T("config.opened"))
	case "/settings":
		l.showSettings()
	case "/set":
		// 値には空白を含められる (例: /set system_prompt あなたは…)
		value := ""
		if len(parts) > 2 {
			value = strings.TrimSpace(strings.SplitN(text, parts[1], 2)[1])
		}
		l.setCommand(arg(1), value, len(parts) > 2)
	case "/voices":
		if l.s.Get("tts", "system") == "voicevox" {
			l.listVoicevox()
		} else {
			l.emit("listVoices", nil)
		}
	case "/install-voicevox":
		l.installVoicevoxCmd()
	case "/install-whisper":
		l.installWhisperCmd()
	case "/install-vision":
		l.installVisionCmd()
	case "/commands":
		l.commandsFile()
	case "/words":
		l.wordsFile()
	case "/shell":
		l.toggleShell()
	case "/phone":
		l.phoneCommand(strings.TrimSpace(strings.TrimPrefix(text, parts[0])))
	case "/routines", "/routine":
		l.routinesCommand(strings.TrimSpace(strings.TrimPrefix(text, parts[0])))
	case "/attach":
		l.attachCommand(strings.TrimSpace(strings.TrimPrefix(text, parts[0])))
	case "/detach":
		l.detach()
	case "/screen":
		// 画面を撮って、続けて書いた質問 (なければ「この画面について教えて」) と一緒に見せる
		q := strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
		if q == "" {
			q = T("screen.defaultQuestion")
		}
		if l.begin() {
			go l.respond(q, true)
		}
	case "/peek":
		l.demoPeek()
	case "/help":
		var b strings.Builder
		for _, c := range commands {
			args := ""
			if c.args != "" {
				args = " " + T(c.args)
			}
			b.WriteString("  " + padRight(c.name+args, 24) + c.help() + "\n")
		}
		b.WriteString("\n  " + T("help.wake", l.wakeWord()) + "\n  " + T("help.tab") + "\n")
		b.WriteString("  " + T("help.shell") + "\n")
		b.WriteString("  " + T("help.keys"))
		l.info(b.String())
	default:
		l.errorText(T("cmd.unknown", cmd))
	}
}

// / コマンド (説明は locales の cmd.<名前>)
type command struct{ name, args string }

func (c command) help() string { return T("cmd." + strings.TrimPrefix(c.name, "/")) }

var commands = []command{
	{"/help", ""}, {"/settings", ""}, {"/set", "cmd.set.args"}, {"/voices", ""}, {"/config", ""},
	{"/reload", ""}, {"/mute", ""}, {"/mic", ""}, {"/install-local", ""}, {"/install-voice", ""}, {"/install-voicevox", ""},
	{"/install-whisper", ""}, {"/install-vision", ""},
	{"/attach", "cmd.attach.args"}, {"/detach", ""}, {"/screen", "cmd.screen.args"}, {"/commands", ""}, {"/words", ""}, {"/shell", ""}, {"/routines", "cmd.routines.args"}, {"/phone", "cmd.phone.args"},
	{"/memory", ""}, {"/reminders", ""}, {"/history", ""}, {"/update", ""},
	{"/peek", ""}, {"/cls", ""}, {"/exit", ""},
}

// 設定項目 (説明は locales の set.<項目>)。rule: 選べる値 (カンマ区切り) か #int:min:max / #num:min:max
type settingKey struct{ key, rule string }

func (k settingKey) help() string { return T("set." + k.key) }

var settingKeys = []settingKey{
	{"language", "#lang"},
	{"provider", "local,offline,anthropic,openai,command"},
	{"model", ""}, {"endpoint", ""}, {"api_key_env", ""}, {"command", ""},
	{"max_tokens", "#int:0:1000000"},
	{"effort", ",low,medium,high,xhigh,max"},
	{"local_gpu", "auto,off"},
	{"pc_control", "on,off"},
	{"web_search", "on,ask,off"},
	{"search_url", ""},
	{"voice_input", "on,off"},
	{"wake_word", ""},
	{"voice_debug", "off,on"},
	{"stt", "vosk,whisper"},
	{"whisper_model", "base,small"},
	{"system_prompt", ""},
	{"voice", ""},
	{"voice_rate", "#int:-10:10"},
	{"tts", "system,voicevox"},
	{"voicevox_voice", "#int:-1:10000"},
	{"background", "on,off"},
	{"startup", "on,off"},
	{"hotkey", ""},
	{"clipboard", "ask,on,off"},
	{"screen", "ask,on,off"},
	{"keep_history", "on,off"},
	{"update_check", "on,off"},
	{"weather_location", ""},
	{"phone", "off,on"},
	{"phone_port", "#int:1024:65535"},
	{"face_color", ""},
	{"face_size", "#int:20:100"},
	{"font_size", "#int:10:28"},
	{"font", ""},
}

// 言語は入っている翻訳から選ぶ
func (k settingKey) choices() string {
	if k.rule == "#lang" {
		return "auto," + strings.Join(languages(), ",")
	}
	return k.rule
}

func (l *Lumi) showSettings() {
	var b strings.Builder
	for _, k := range settingKeys {
		v := l.s.Get(k.key, "")
		if r := []rune(v); len(r) > 40 {
			v = string(r[:40]) + "…"
		}
		if v == "" {
			v = T("set.default")
		}
		b.WriteString("  " + padRight(k.key, 16) + padRight(v, 28) + k.help() + "\n")
	}
	b.WriteString("\n  " + T("set.howto"))
	l.info(b.String())
}

func (l *Lumi) setCommand(key, value string, hasValue bool) {
	if key == "" {
		l.showSettings()
		return
	}
	var def *settingKey
	for i := range settingKeys {
		if settingKeys[i].key == strings.ToLower(key) {
			def = &settingKeys[i]
		}
	}
	if def == nil {
		l.errorText(T("set.noKey", key))
		return
	}
	if !hasValue {
		v := l.s.Get(def.key, "")
		if v == "" {
			v = T("set.default")
		}
		msg := def.key + " = " + v + "    " + def.help()
		if rule := def.choices(); rule != "" && !strings.HasPrefix(rule, "#") {
			msg += "\n  " + T("set.choices", strings.ReplaceAll(strings.Trim(rule, ","), ",", " / "))
		}
		l.info(msg)
		return
	}
	if l.s.Err != nil {
		l.errorText(T("settings.broken"))
		return
	}
	if value == `""` {
		value = ""
	}
	var stored any = value
	rule := def.choices()
	if strings.HasPrefix(rule, "#") {
		r := strings.Split(rule, ":")
		lo, _ := strconv.ParseFloat(r[1], 64)
		hi, _ := strconv.ParseFloat(r[2], 64)
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || n < lo || n > hi || (r[0] == "#int" && n != math.Floor(n)) {
			kind := T("set.num")
			if r[0] == "#int" {
				kind = T("set.int")
			}
			l.errorText(T("set.range", def.key, r[1], r[2], kind))
			return
		}
		if r[0] == "#int" {
			stored = int(n)
		} else {
			stored = n
		}
	} else if rule != "" {
		value = strings.ToLower(value)
		ok := false
		for _, v := range strings.Split(rule, ",") {
			ok = ok || v == value
		}
		if !ok {
			l.errorText(T("set.allowed", def.key, strings.ReplaceAll(strings.Trim(rule, ","), ",", " / ")))
			return
		}
		stored = value
	}
	if err := l.s.Set(def.key, stored); err != nil {
		l.errorText(T("set.saveFailed", err.Error()))
		return
	}
	shown := value
	if shown == "" {
		shown = T("set.default")
	}
	l.info(T("set.done", def.key, shown))
	l.reload(false)
}

// 設定ファイルを OS の標準のアプリで開く
func openFile(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad.exe", path)
	case "darwin":
		cmd = exec.Command("open", "-t", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	cmd.Start()
}

// ---- ローカルAIのダウンロード ----

func (l *Lumi) installLocal() {
	l.setBusy(true)
	cancel := make(chan struct{})
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	l.write(T("local.installStart")+"\n", "dim")
	go func() {
		defer l.setBusy(false)
		lastStep, lastPct := "", -1
		err := installLocal(dataDir(), func(step string, ratio float64) {
			pct := int(ratio * 100)
			if step == lastStep && pct/5 == lastPct/5 {
				return
			}
			l.write(fmt.Sprintf("  [%3d%%] %s\n", pct, step), "dim")
			lastStep, lastPct = step, pct
		}, func() bool {
			select {
			case <-cancel:
				return true
			default:
				return false
			}
		})
		switch {
		case err == errCancelled:
			l.write("^C\n"+T("local.installCancelled", "/install-local")+"\n\n", "dim")
		case err != nil:
			l.errorText(T("download.failed", err.Error()))
		default:
			if l.s.Get("provider", "offline") == "offline" {
				l.s.Set("provider", "local")
			}
			l.loadSettings()
			l.write(T("local.installed", l.ai.Label())+"\n", "dim")
			l.warmupLocal()
		}
	}()
}

// ---- 返事 ----

func (l *Lumi) interrupt() {
	l.mu.Lock()
	busy, c := l.busy, l.cancel
	l.mu.Unlock()
	if !busy || c == nil {
		return
	}
	select {
	case <-c:
	default:
		close(c)
	}
	l.ai.Abort()
	l.emit("stopSpeaking", nil)
}

func (l *Lumi) cancelled() bool {
	l.mu.Lock()
	c := l.cancel
	l.mu.Unlock()
	if c == nil {
		return false
	}
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func (l *Lumi) startReply(text string) {
	if l.begin() {
		go l.respond(text, false)
	}
}

// 忙しくなければ忙しい状態にして、新しい中断用の印を作る (できたら true)
func (l *Lumi) begin() bool {
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		return false
	}
	l.cancel = make(chan struct{})
	l.mu.Unlock()
	l.setBusy(true)
	return true
}

const maxToolRounds = 5

// screenFirst: /screen のとき、最初に画面を撮って一緒に見せる
func (l *Lumi) respond(userText string, screenFirst bool) {
	attached := l.takePending()
	keep := l.s.On("keep_history", "on")
	if keep {
		saved := userText
		for _, a := range attached {
			saved += "  [" + a.Name + "]"
		}
		appendHistory("user", saved)
	}
	l.replyText.Reset()
	defer func() {
		if keep {
			appendHistory("assistant", l.replyText.String())
		}
	}()
	// 今の日時は発言の先頭に付ける (指示文に入れると毎分変わり、ローカルAIが前の会話を読み直すことになる)
	message := userText
	if _, offline := l.ai.(offlineProvider); !offline {
		message = "[" + time.Now().Format("2006-01-02 15:04 (Mon)") + "] " + userText
	}
	turn := withAttachments(message, attached)
	if screenFirst {
		img, _ := l.screenTool() // 撮れなかった理由は screenTool が画面に出している
		if img == nil {
			l.mu.Lock()
			l.pending = append(attached, l.pending...) // 付けていたファイルは次の発言に残す
			l.mu.Unlock()
			l.write("\n", "dim")
			l.setBusy(false)
			return
		}
		turn.Images = append(turn.Images, img)
	}
	declined := false // 一度断られたら、この返事の間はもうコマンドを聞かない
	for round := 0; round < maxToolRounds && turn.Text != "" && !l.cancelled(); round++ {
		requests := l.replyOnce(turn)
		turn = Turn{}
		if declined {
			kept := requests[:0]
			for _, r := range requests {
				if r.Kind != "run" {
					kept = append(kept, r)
				}
			}
			requests = kept
		}
		if len(requests) == 0 || l.cancelled() {
			break
		}
		// 1 つずつ処理して (コマンドは必ず確認してから実行)、結果をまとめて AI に返す
		var report strings.Builder
		var images [][]byte
		report.WriteString(T("tool.results") + "\n")
		for _, r := range requests {
			if l.cancelled() {
				break
			}
			if r.Kind == "clipboard" {
				report.WriteString(l.clipboardTool())
				continue
			}
			if r.Kind == "screen" {
				report.WriteString("\n[" + T("screen.label") + "]\n")
				if img, why := l.screenTool(); img != nil {
					images = append(images, img)
					report.WriteString(T("screen.attached") + "\n")
				} else {
					report.WriteString(why + "\n")
				}
				continue
			}
			if r.Kind == "search" || r.Kind == "fetch" {
				report.WriteString(l.webTool(r))
				continue
			}
			if !l.s.PcControl() {
				report.WriteString("\n$ " + r.Command + "\n" + T("tool.pcOff") + "\n")
				continue
			}
			admin := ""
			if r.Admin {
				admin = "  " + T("tool.adminTag")
			}
			report.WriteString("\n$ " + r.Command + admin + "\n")
			answer := l.confirm(r)
			if answer != "y" && answer != "a" {
				l.write(T("tool.notRun")+"\n", "dim")
				report.WriteString(T("tool.declined") + "\n")
				declined = true
				continue
			}
			l.emit("thinking", true)
			l.face("think")
			// 管理者として動き始めたら、終わるまで顔の色を変える
			l.emit("running", map[string]any{"on": true, "cmd": r.Command})
			result, ok := runCommand(r.Command, r.Admin || answer == "a", l.cancelChan(), func() { l.emit("admin", true) })
			l.emit("running", map[string]any{"on": false})
			l.emit("admin", l.elevated)
			l.showOutput(result)
			mood := "sad"
			if ok && !strings.Contains(result, "Exception") {
				mood = "happy"
			}
			l.emit("flash", map[string]any{"expr": mood, "seconds": 2})
			report.WriteString(l.fitOutput(result) + "\n")
		}
		if !l.cancelled() {
			turn = Turn{Text: report.String(), Images: images}
		}
	}
	l.face("normal")
	l.emit("thinking", false)
	if l.cancelled() {
		l.write("^C\n\n", "dim")
	} else {
		l.write("\n", "dim")
	}
	l.setBusy(false)
}

// ローカルAIはコンテキストが小さいので、コマンドの出力を短くしてから渡す
const localOutputLimit = 4000

func (l *Lumi) fitOutput(out string) string {
	if strings.ToLower(l.s.Get("provider", "offline")) != "local" {
		return out
	}
	if r := []rune(out); len(r) > localOutputLimit {
		return string(r[:localOutputLimit]) + "\n" + T("run.truncated")
	}
	return out
}

// 日本語は 2 文字分の幅として、見た目の幅で右を埋める
func padRight(s string, width int) string {
	w := 0
	for _, r := range s {
		if r >= 0x1100 && (r <= 0x115F || (r >= 0x2E80 && r <= 0xA4CF) || (r >= 0xAC00 && r <= 0xD7A3) ||
			(r >= 0xF900 && r <= 0xFAFF) || (r >= 0xFE30 && r <= 0xFE4F) || (r >= 0xFF00 && r <= 0xFF60) ||
			(r >= 0xFFE0 && r <= 0xFFE6) || r >= 0x1F300) {
			w += 2
		} else {
			w++
		}
	}
	if w >= width {
		return s + " "
	}
	return s + strings.Repeat(" ", width-w)
}

func (l *Lumi) cancelChan() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cancel
}

// AI に 1 回話しかけて返事を 1 文ずつ喋らせ、返事に含まれていた道具の呼び出しを返す
func (l *Lumi) replyOnce(turn Turn) []toolRequest {
	sentences := make(chan string, 64)
	done := make(chan struct{})
	go func() {
		for s := range sentences {
			if l.cancelled() {
				continue
			}
			if expr, ok := strings.CutPrefix(s, faceMarker); ok {
				l.showFeeling(expr)
				continue
			}
			l.speak(s)
		}
		close(done)
	}()

	l.emit("thinking", true)
	l.face("think")
	var sp sentenceSplitter
	var requests []toolRequest
	tools := toolExtractor{
		OnText: func(t string) {
			l.replyText.WriteString(t)
			for _, s := range sp.Push(t) {
				sentences <- s
			}
		},
		OnTag: func(r toolRequest) {
			switch r.Kind {
			case "face":
				// 表情は、それより前の文を喋り終えたところで変える
				for _, s := range sp.Push("\n") {
					sentences <- s
				}
				sentences <- faceMarker + r.Command
			case "remember", "forget":
				l.memoryTag(r)
			case "remind":
				l.remindTag(r)
			default:
				requests = append(requests, r)
			}
		},
	}
	l.ai.Reply(turn, tools.Push)
	tools.Flush()
	if rest := sp.Flush(); strings.TrimSpace(rest) != "" {
		sentences <- rest
	}
	close(sentences)
	<-done
	l.write("\n", "fg")
	return requests
}

// 読み上げの列にはさむ「ここで表情を変える」印
const faceMarker = "\x00face:"

// AI が <face> で指定した気持ちを顔に出す
func (l *Lumi) showFeeling(expr string) {
	expr = strings.ToLower(strings.TrimSpace(expr))
	switch expr {
	case "happy", "sad", "think":
	case "surprised":
		expr = "listen" // 目を大きく見開く
	default:
		return
	}
	l.emit("flash", map[string]any{"expr": expr, "seconds": 4})
}

// 1 文を画面に喋らせ (読み上げと文字の表示は画面側)、終わるまで待つ
func (l *Lumi) speak(sentence string) {
	text := cleanForSpeech(sentence)
	if text == "" {
		return
	}
	l.mu.Lock()
	l.speakID++
	id := l.speakID
	l.mu.Unlock()
	// VOICEVOX が使えればその声で (口の動きの時刻表つき)、だめなら画面側の音声合成で
	if wav, keys, ok := l.voicevoxSpeech(text); ok {
		l.emit("thinking", false)
		l.emit("speakAudio", map[string]any{"id": id, "text": text, "wav": base64.StdEncoding.EncodeToString(wav), "keys": keys})
	} else {
		l.emit("thinking", false)
		l.emit("speak", map[string]any{"id": id, "text": text})
	}
	timeout := time.After(time.Duration(len([]rune(text)))*400*time.Millisecond + 10*time.Second)
	for {
		select {
		case got := <-l.spoken:
			if got >= id {
				return
			}
		case <-l.cancelChan():
			return
		case <-timeout:
			return
		}
	}
}

// tts が voicevox なら、その声で文を WAV にする (ミュート中・失敗したときは ok=false)
func (l *Lumi) voicevoxSpeech(text string) ([]byte, []mouthKey, bool) {
	if l.muted || l.s.Get("tts", "system") != "voicevox" {
		return nil, nil, false
	}
	if err := voicevox.Start(dataDir()); err != nil {
		l.reportTTSError(err)
		return nil, nil, false
	}
	style := voicevox.pickStyle(l.s.GetInt("voicevox_voice", -1))
	wav, keys, err := voicevox.Synthesize(text, style, l.s.GetInt("voice_rate", 1))
	if err != nil {
		l.reportTTSError(err)
		return nil, nil, false
	}
	// 利用規約により、使っている声のクレジットを出す (声が変わったときに 1 回)
	if style != l.creditedStyle {
		name, styleName := voicevox.styleName(style)
		l.write("  "+T("voicevox.credit", name, styleName)+"\n", "dim")
		l.creditedStyle = style
	}
	return wav, keys, true
}

func (l *Lumi) reportTTSError(err error) {
	if !l.ttsErrorShown {
		l.ttsErrorShown = true
		l.errorText(T("voicevox.error", err.Error()))
	}
}

// コマンドを見せて、実行してよいか答えてもらう (y / a / それ以外は実行しない)
func (l *Lumi) confirm(r toolRequest) string {
	l.face("normal")
	need := ""
	if r.Admin {
		need = T("confirm.needAdmin")
	}
	l.write("\n"+T("confirm.title", need)+"\n", "yellow")
	for _, line := range strings.Split(r.Command, "\n") {
		l.write("  "+strings.TrimRight(line, "\r")+"\n", "white")
	}
	question := T("confirm.question")
	if r.Admin {
		question = T("confirm.questionAdmin")
	}
	answer := strings.ToLower(l.ask(question))
	if r.Admin && answer == "a" {
		answer = "y"
	}
	return answer
}

// 質問を出して、Enter で答えてもらうまで待つ
func (l *Lumi) ask(question string) string {
	for len(l.answers) > 0 {
		<-l.answers
	}
	l.emit("ask", question)
	select {
	case a := <-l.answers:
		return strings.TrimSpace(a)
	case <-l.cancelChan():
		return ""
	}
}

func (l *Lumi) showOutput(result string) {
	lines := strings.Split(result, "\n")
	const max = 15
	for i, line := range lines {
		if i >= max {
			l.write("  "+T("output.omitted", len(lines)-max)+"\n", "dim")
			break
		}
		l.write("  "+line+"\n", "dim")
	}
	l.write("\n", "fg")
}

// Web 検索 / ページの読み込みをして、AI に返す結果の文章を作る
func (l *Lumi) webTool(r toolRequest) string {
	search := r.Kind == "search"
	label := T("web.page")
	if search {
		label = T("web.search")
	}
	head := "\n[" + label + "] " + r.Command + "\n"
	mode := strings.ToLower(l.s.Get("web_search", "on"))
	if mode == "off" {
		return head + T("web.off") + "\n"
	}
	if search {
		l.write(T("web.searching", r.Command)+"\n", "cyan")
	} else {
		l.write(T("web.reading", r.Command)+"\n", "cyan")
	}
	if mode == "ask" {
		q := T("web.askRead")
		if search {
			q = T("web.askSearch")
		}
		if a := strings.ToLower(l.ask(q)); a != "y" && a != "a" {
			l.write(T("web.stopped")+"\n", "dim")
			return head + T("web.denied") + "\n"
		}
	}
	l.emit("thinking", true)
	l.face("think")
	var result string
	var err error
	if search {
		result, err = webSearch(r.Command, l.s.Get("search_url", ""))
	} else {
		result, err = fetchPage(r.Command)
	}
	if err != nil {
		result = T("web.failed", err.Error())
	}
	// 画面には要点だけ (検索ならタイトル、ページなら先頭数行) を出す
	lines := strings.Split(result, "\n")
	var shown []string
	for _, x := range lines {
		if search && len(x) > 2 && x[0] >= '1' && x[0] <= '9' && x[1] == '.' {
			shown = append(shown, x)
		}
	}
	if !search {
		shown = lines[:min(3, len(lines))]
	}
	if len(shown) == 0 {
		shown = lines[:1]
	}
	for _, x := range shown {
		if r := []rune(x); len(r) > 90 {
			x = string(r[:90]) + "…"
		}
		l.write("  "+x+"\n", "dim")
	}
	l.write("\n", "fg")
	return head + result + "\n"
}

// ---- 終了 ----

func (l *Lumi) quit() {
	voicevox.Stop()
	phone.stop()
	l.interrupt()
	localServer.Stop()
	l.app.Quit()
}

// 使える声の一覧 (画面側から名前が届く) を並べて表示する
func (l *Lumi) showVoices(names []string, current string) {
	if len(names) == 0 {
		l.info(T("voices.none"))
		return
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		mark := ""
		if n == current {
			mark = "  " + T("voices.current")
		}
		b.WriteString("  " + n + mark + "\n")
	}
	b.WriteString("\n  " + T("voices.howto"))
	l.info(b.String())
}
