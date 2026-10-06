package main

// ルミの中心: 入力を受け取り、コマンドを処理し、AI の返事を 1 文ずつ画面に喋らせる。
// 画面 (assets/app.js) とはイベントでやり取りする。
//   画面 → Go: submit(文字) / answer(確認への答え) / interrupt / ready / spoken(id)
//   Go → 画面: write / clear / busy / thinking / face / flash / ask / speak / commands / voices

import (
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

const version = "0.3.0-beta"

type Lumi struct {
	app   *application.App
	win   *application.WebviewWindow
	peek  *Peek
	mu    sync.Mutex
	s     *Settings
	ai    Provider
	muted bool

	busy    bool
	cancel  chan struct{}
	answers chan string
	spoken  chan int
	speakID int
}

func newLumi(app *application.App, muted bool) *Lumi {
	l := &Lumi{app: app, muted: muted, answers: make(chan string, 1), spoken: make(chan int, 8)}
	l.loadSettings()
	return l
}

// ---- 画面への出力 ----

func (l *Lumi) emit(name string, data any) {
	if l.win != nil {
		l.win.EmitEvent(name, data)
	}
}

func (l *Lumi) write(text, color string) { l.emit("write", map[string]any{"text": text, "color": color}) }
func (l *Lumi) info(text string)         { l.write(text+"\n\n", "dim") }
func (l *Lumi) errorText(text string)    { l.write(text+"\n\n", "red") }
func (l *Lumi) face(expr string)         { l.emit("face", expr) }

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
	l.win.SetTitle("ルミ  —  " + l.ai.Label() + " | " + voice)
}

// ---- 設定 ----

func (l *Lumi) loadSettings() {
	l.s = LoadSettings()
	l.ai = newProvider(l.s)
	l.updateTitle()
}

func (l *Lumi) reload(announce bool) {
	l.loadSettings()
	if l.s.Err != nil {
		l.errorText("settings.json を読めませんでした: " + l.s.Err.Error())
	} else if announce {
		l.info("設定を読み直しました。（" + l.ai.Label() + "）")
	}
	if !strings.HasPrefix(l.ai.Label(), "local:") {
		localServer.Stop()
	}
	l.sendVoiceSettings()
	l.applyStartup()
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
	l.sendVoiceSettings()
	l.write("Lumi Assistant [Version "+version+"]\n", "fg")
	l.write("話しかけると声で返事をします。/help でコマンド一覧。\n\n", "dim")
	if l.s.Err != nil {
		l.errorText("settings.json を読めませんでした: " + l.s.Err.Error())
	}
	l.emit("flash", map[string]any{"expr": "happy", "seconds": 2.5})
	l.warmupLocal()
}

// ローカルAIなら、最初の返事を待たせないように先にモデルを読み込んでおく
func (l *Lumi) warmupLocal() {
	if !strings.HasPrefix(l.ai.Label(), "local:") {
		return
	}
	dir := dataDir()
	if !localInstalled(dir) {
		l.write(fmt.Sprintf("ローカルAIがまだ入っていません。/install-local と入力するとダウンロードします（約%.1fGB）。\n\n",
			float64(localTotalSize())/1e9), "yellow")
		return
	}
	l.write("ローカルAIを起動しています…\n", "dim")
	go func() {
		p := l.ai
		if op, ok := p.(*thinkFiltered); ok && op.before != nil {
			if err := op.before(); err != nil {
				l.errorText(err.Error())
				return
			}
		}
		l.info("ローカルAIの準備ができました。")
	}()
}

// ---- 入力 ----

func (l *Lumi) submit(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if !strings.HasPrefix(text, "/") {
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
		l.emit("clear", nil)
	case "/mute":
		l.muted = !l.muted
		l.updateTitle()
		l.sendVoiceSettings()
		if l.muted {
			l.info("読み上げをオフにしました。")
		} else {
			l.info("読み上げをオンにしました。")
		}
	case "/mic":
		l.info("音声入力はこのバージョンではまだ使えません (準備中)。")
	case "/reload":
		l.reload(true)
	case "/install-local":
		l.installLocal()
	case "/config":
		openFile(l.s.Path)
		l.info("settings.json を開きました。保存したら /reload で反映されます。")
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
		l.emit("listVoices", nil)
	case "/peek":
		l.demoPeek()
	case "/help":
		var b strings.Builder
		for _, c := range commands {
			b.WriteString("  " + padRight(c.name+" "+c.args, 24) + c.help + "\n")
		}
		b.WriteString("\n  Tab でコマンドを補完できます。\n")
		b.WriteString("  Ctrl+C / Esc  返事を止める     ↑↓  入力履歴")
		l.info(b.String())
	default:
		l.errorText("知らないコマンドです: " + cmd + "（/help で一覧）")
	}
}

type command struct{ name, args, help string }

var commands = []command{
	{"/help", "", "コマンド一覧"},
	{"/settings", "", "今の設定を一覧する"},
	{"/set", "<項目> <値>", "設定を変える (例: /set voice_rate 3)"},
	{"/voices", "", "使える声の一覧"},
	{"/config", "", "設定ファイル (settings.json) を開く"},
	{"/reload", "", "設定ファイルを読み直す"},
	{"/mute", "", "読み上げのオン/オフ"},
	{"/mic", "", "音声入力のオン/オフ"},
	{"/install-local", "", "ローカルAIをダウンロードする"},
	{"/peek", "", "バックグラウンドで呼ばれたときの動きを試す"},
	{"/cls", "", "画面と会話をリセット"},
	{"/exit", "", "終了"},
}

type settingKey struct{ key, help, rule string } // rule: 選べる値 (カンマ区切り) か #int:min:max / #num:min:max

var settingKeys = []settingKey{
	{"provider", "使うAI", "local,offline,anthropic,openai,command"},
	{"model", "モデル名 (local では .gguf のファイル名)", ""},
	{"endpoint", "API の URL", ""},
	{"api_key_env", "API キーが入っている環境変数の名前", ""},
	{"command", "command で実行するコマンド", ""},
	{"max_tokens", "返答の最大トークン数 (0 で既定)", "#int:0:1000000"},
	{"effort", "Anthropic の effort", ",low,medium,high,xhigh,max"},
	{"local_gpu", "ローカルAIで GPU を使うか", "auto,off"},
	{"pc_control", "PC の操作 (毎回確認あり)", "on,off"},
	{"web_search", "Web 検索 (ask は毎回確認)", "on,ask,off"},
	{"search_url", "SearXNG の URL (空なら DuckDuckGo)", ""},
	{"system_prompt", "キャラクター設定 (空なら既定)", ""},
	{"voice", "声の名前 (/voices で一覧)", ""},
	{"voice_rate", "読み上げの速さ (-10〜10)", "#int:-10:10"},
	{"background", "閉じてもトレイで動き続ける", "on,off"},
	{"startup", "ログイン時にトレイで起動する", "on,off"},
}

func (l *Lumi) showSettings() {
	var b strings.Builder
	for _, k := range settingKeys {
		v := l.s.Get(k.key, "")
		if r := []rune(v); len(r) > 40 {
			v = string(r[:40]) + "…"
		}
		if v == "" {
			v = "(既定)"
		}
		b.WriteString("  " + padRight(k.key, 16) + padRight(v, 28) + k.help + "\n")
	}
	b.WriteString("\n  変えるには /set <項目> <値>。空に戻すには /set <項目> \"\"")
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
		l.errorText("そんな設定項目はありません: " + key + "（/settings で一覧）")
		return
	}
	if !hasValue {
		v := l.s.Get(def.key, "")
		if v == "" {
			v = "(既定)"
		}
		msg := def.key + " = " + v + "    " + def.help
		if def.rule != "" && !strings.HasPrefix(def.rule, "#") {
			msg += "\n  選べる値: " + strings.ReplaceAll(strings.Trim(def.rule, ","), ",", " / ")
		}
		l.info(msg)
		return
	}
	if l.s.Err != nil {
		l.errorText("settings.json が壊れているので変更できません。/config で直してください。")
		return
	}
	if value == `""` {
		value = ""
	}
	var stored any = value
	if strings.HasPrefix(def.rule, "#") {
		r := strings.Split(def.rule, ":")
		lo, _ := strconv.ParseFloat(r[1], 64)
		hi, _ := strconv.ParseFloat(r[2], 64)
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || n < lo || n > hi || (r[0] == "#int" && n != math.Floor(n)) {
			kind := "数"
			if r[0] == "#int" {
				kind = "整数"
			}
			l.errorText(fmt.Sprintf("%s は %s〜%s の%sで指定してください。", def.key, r[1], r[2], kind))
			return
		}
		if r[0] == "#int" {
			stored = int(n)
		} else {
			stored = n
		}
	} else if def.rule != "" {
		value = strings.ToLower(value)
		ok := false
		for _, v := range strings.Split(def.rule, ",") {
			ok = ok || v == value
		}
		if !ok {
			l.errorText(def.key + " に使える値: " + strings.ReplaceAll(strings.Trim(def.rule, ","), ",", " / "))
			return
		}
		stored = value
	}
	if err := l.s.Set(def.key, stored); err != nil {
		l.errorText("保存できませんでした: " + err.Error())
		return
	}
	shown := value
	if shown == "" {
		shown = "(既定)"
	}
	l.info(def.key + " を " + shown + " にしました。")
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
	l.write("ローカルAIをダウンロードします。Ctrl+C で中断できます（次回は続きから再開します）。\n", "dim")
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
			l.write("^C\nダウンロードを中断しました。もう一度 /install-local で続きから再開します。\n\n", "dim")
		case err != nil:
			l.errorText("ダウンロードに失敗しました: " + err.Error())
		default:
			if l.s.Get("provider", "offline") == "offline" {
				l.s.Set("provider", "local")
			}
			l.loadSettings()
			l.write("ローカルAIを入れました。（"+l.ai.Label()+"）\n", "dim")
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
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		return
	}
	l.cancel = make(chan struct{})
	l.mu.Unlock()
	l.setBusy(true)
	go l.respond(text)
}

const maxToolRounds = 5

func (l *Lumi) respond(userText string) {
	message := userText
	declined := false // 一度断られたら、この返事の間はもうコマンドを聞かない
	for round := 0; round < maxToolRounds && message != "" && !l.cancelled(); round++ {
		requests := l.replyOnce(message)
		message = ""
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
		report.WriteString("[ツールの結果]\n")
		for _, r := range requests {
			if l.cancelled() {
				break
			}
			if r.Kind == "search" || r.Kind == "fetch" {
				report.WriteString(l.webTool(r))
				continue
			}
			if !l.s.PcControl() {
				report.WriteString("\n$ " + r.Command + "\n(PC の操作はオフになっています)\n")
				continue
			}
			admin := ""
			if r.Admin {
				admin = "  (管理者)"
			}
			report.WriteString("\n$ " + r.Command + admin + "\n")
			answer := l.confirm(r)
			if answer != "y" && answer != "a" {
				l.write("実行しませんでした。\n", "dim")
				report.WriteString("(ユーザーが実行を許可しませんでした。別のコマンドは提案せず、言葉だけで答えてください)\n")
				declined = true
				continue
			}
			l.emit("thinking", true)
			l.face("think")
			result := runCommand(r.Command, r.Admin || answer == "a", l.cancelChan())
			l.showOutput(result)
			mood := "sad"
			if strings.HasPrefix(result, "終了コード 0") && !strings.Contains(result, "Exception") {
				mood = "happy"
			}
			l.emit("flash", map[string]any{"expr": mood, "seconds": 2})
			report.WriteString(result + "\n")
		}
		if !l.cancelled() {
			message = report.String()
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
func (l *Lumi) replyOnce(message string) []toolRequest {
	sentences := make(chan string, 64)
	done := make(chan struct{})
	go func() {
		for s := range sentences {
			if !l.cancelled() {
				l.speak(s)
			}
		}
		close(done)
	}()

	l.emit("thinking", true)
	l.face("think")
	var sp sentenceSplitter
	var tools toolExtractor
	l.ai.Reply(message, func(chunk string) {
		for _, s := range sp.Push(tools.Push(chunk)) {
			sentences <- s
		}
	})
	for _, s := range sp.Push(tools.Flush()) {
		sentences <- s
	}
	if rest := sp.Flush(); strings.TrimSpace(rest) != "" {
		sentences <- rest
	}
	close(sentences)
	<-done
	l.write("\n", "fg")
	return tools.Requests
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
	l.emit("thinking", false)
	l.emit("speak", map[string]any{"id": id, "text": text})
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

// コマンドを見せて、実行してよいか答えてもらう (y / a / それ以外は実行しない)
func (l *Lumi) confirm(r toolRequest) string {
	l.face("normal")
	need := ""
	if r.Admin {
		need = "（管理者権限が必要です）"
	}
	l.write("\nルミがコマンドを実行しようとしています"+need+":\n", "yellow")
	for _, line := range strings.Split(r.Command, "\n") {
		l.write("  "+strings.TrimRight(line, "\r")+"\n", "white")
	}
	question := "実行しますか？ [y=実行 / a=管理者として実行 / N=やめる] "
	if r.Admin {
		question = "管理者として実行しますか？ [y/N] "
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
			l.write(fmt.Sprintf("  …(%d 行省略)\n", len(lines)-max), "dim")
			break
		}
		l.write("  "+line+"\n", "dim")
	}
	l.write("\n", "fg")
}

// Web 検索 / ページの読み込みをして、AI に返す結果の文章を作る
func (l *Lumi) webTool(r toolRequest) string {
	search := r.Kind == "search"
	label := "ページ"
	if search {
		label = "Web検索"
	}
	head := "\n[" + label + "] " + r.Command + "\n"
	mode := strings.ToLower(l.s.Get("web_search", "on"))
	if mode == "off" {
		return head + "(Web 検索はオフになっています)\n"
	}
	if search {
		l.write("検索: "+r.Command+"\n", "cyan")
	} else {
		l.write("ページを読む: "+r.Command+"\n", "cyan")
	}
	if mode == "ask" {
		q := "読みますか？ [y/N] "
		if search {
			q = "検索しますか？ [y/N] "
		}
		if a := strings.ToLower(l.ask(q)); a != "y" && a != "a" {
			l.write("やめました。\n", "dim")
			return head + "(ユーザーが許可しませんでした)\n"
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
		result = "(読み込めませんでした: " + err.Error() + ")"
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
	l.interrupt()
	localServer.Stop()
	l.app.Quit()
}

// 使える声の一覧 (画面側から名前が届く) を並べて表示する
func (l *Lumi) showVoices(names []string, current string) {
	if len(names) == 0 {
		l.info("この環境では画面側の音声合成が使えません。")
		return
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		mark := ""
		if n == current {
			mark = "  ← 使用中"
		}
		b.WriteString("  " + n + mark + "\n")
	}
	b.WriteString("\n  変えるには /set voice <名前>")
	l.info(b.String())
}
