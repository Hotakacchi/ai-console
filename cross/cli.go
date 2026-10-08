package main

// ターミナル版のルミ (lumi --cli / Windows は lumi-cli.exe)。
// 窓を開かずに、Windows ターミナルなどのタブの中で話す。中身は窓のルミと同じで、
// 画面に送っていたイベント (write / speak / ask / busy …) を、ここで文字にして端末に出す。
// 顔は、プロンプトの前の顔文字で表す。声の読み上げ・聞き取り・小窓・スマホ・Discord は窓のルミだけ。

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type cliUI struct {
	l     *Lumi
	out   io.Writer
	color bool

	mu         sync.Mutex
	expr       string    // 今の表情
	flash      string    // しばらくだけの表情
	flashUntil time.Time //
	shell      string    // シェルモードのプロンプト ("" なら普段)
	asking     bool      // 質問 (実行してよい？) の答えを待っている
	atPrompt   bool      // プロンプトを出して入力を待っている
	thinking   bool      // 「考え中」の行を出している (次に何か書くときに消す)
	lineStart  bool      // 今、行の先頭にいる
	lastCtrlC  time.Time
}

// ターミナル版で動かすか: --cli か、lumi-cli という名前で起動されたとき
func wantCLI(args []string) bool {
	for _, a := range args {
		if a == "--cli" {
			return true
		}
	}
	name := strings.ToLower(filepath.Base(os.Args[0]))
	return strings.HasPrefix(name, "lumi-cli")
}

func runCLI() {
	color := setupTerminal()
	l := newLumi(nil, true, true) // 読み上げも聞き取りもしない
	c := &cliUI{l: l, out: os.Stdout, color: color, expr: "normal", lineStart: true}
	l.cli = c
	l.write("Lumi Assistant [Version "+versionLabel()+"]\n", "fg")
	l.write(T("cli.hint")+"\n\n", "dim")
	if l.s.Err != nil {
		l.errorText(T("settings.readError", l.s.Err.Error()))
	}
	l.restoreHistory(true)
	l.startOnce.Do(func() {
		go l.runReminders()
		go l.watchDrives()
		go l.unloadIdleLocal()
	})
	l.flashFace("happy", 2.5)
	l.warmupLocal()
	c.loop()
}

func (l *Lumi) flashFace(expr string, seconds float64) {
	l.emit("flash", map[string]any{"expr": expr, "seconds": seconds})
}

// 表情ごとの顔文字
var kaomoji = map[string]string{
	"normal":    "(・‿・)",
	"happy":     "(＾▽＾)",
	"think":     "(・_・?)",
	"sad":       "(´・ω・`)",
	"listen":    "(・o・)",
	"sleep":     "(－_－)zz",
	"surprised": "(ﾟoﾟ)!",
	"glitch":    "(×_×)",
	"bell":      "(・o・)!",
}

func (c *cliUI) faceNow() string {
	expr := c.expr
	if time.Now().Before(c.flashUntil) {
		expr = c.flash
	}
	if k, ok := kaomoji[expr]; ok {
		return k
	}
	return kaomoji["normal"]
}

// 色の名前 (画面の CSS と同じ) から端末の色
var ansiColors = map[string]string{
	"fg": "", "white": "97", "dim": "90", "red": "91", "yellow": "93", "cyan": "96", "green": "92", "bright": "96",
}

func (c *cliUI) paint(text, color string) string {
	code := ansiColors[color]
	if !c.color || code == "" || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// 文字を出す (呼ぶ前に c.mu を取っておく)。プロンプトや「考え中」が出ていれば、先に消す
func (c *cliUI) print(text, color string) {
	if text == "" {
		return
	}
	c.clearLine()
	if c.atPrompt {
		// 入力待ちの間に届いた知らせ (リマインダー・ドライブなど): プロンプトを消して出し、あとで出し直す
		c.atPrompt = false
		defer c.showPrompt()
	}
	fmt.Fprint(c.out, c.paint(text, color))
	c.lineStart = strings.HasSuffix(text, "\n")
}

func (c *cliUI) clearLine() {
	if c.thinking || c.atPrompt {
		if c.color {
			fmt.Fprint(c.out, "\r\x1b[K")
		} else if !c.lineStart {
			fmt.Fprintln(c.out)
		}
		c.thinking = false
		c.lineStart = true
	}
}

func (c *cliUI) showPrompt() {
	if c.thinking {
		c.clearLine()
	}
	if !c.lineStart {
		fmt.Fprintln(c.out)
	}
	p := c.shell
	if p == "" {
		p = basePrompt()
	}
	fmt.Fprint(c.out, c.paint(c.faceNow(), "cyan")+" "+p)
	c.atPrompt = true
	c.lineStart = false
}

// ルミの中から画面へのイベント
func (c *cliUI) event(name string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, _ := data.(map[string]any)
	switch name {
	case "write":
		text, _ := m["text"].(string)
		color, _ := m["color"].(string)
		c.print(text, color)
	case "speak", "speakAudio":
		// 読み上げはしないので、文を出してすぐ「喋り終えた」ことにする
		text, _ := m["text"].(string)
		c.print(text, "fg")
		if id, ok := m["id"].(int); ok {
			select {
			case c.l.spoken <- id:
			default:
			}
		}
	case "face":
		c.expr, _ = data.(string)
	case "flash":
		expr, _ := m["expr"].(string)
		c.flash, c.flashUntil = expr, time.Now().Add(seconds(m["seconds"]))
	case "fx":
		fx, _ := m["name"].(string)
		if _, ok := kaomoji[fx]; ok {
			c.flash, c.flashUntil = fx, time.Now().Add(seconds(m["seconds"]))
		}
		if fx == "surprised" && c.atPrompt {
			c.clearLine()
			c.atPrompt = false
			c.showPrompt() // 顔文字を驚いた顔にして出し直す
		}
	case "thinking":
		if on, _ := data.(bool); on && c.lineStart && !c.thinking && !c.atPrompt {
			fmt.Fprint(c.out, c.paint(kaomoji["think"]+" …", "dim"))
			c.thinking = true
			c.lineStart = false
		}
	case "ask":
		q, _ := data.(string)
		c.clearLine()
		if !c.lineStart {
			fmt.Fprintln(c.out)
		}
		fmt.Fprint(c.out, c.paint(q+" ", "yellow"))
		c.asking = true
		c.lineStart = false
	case "busy":
		if b, _ := data.(bool); !b {
			c.asking = false
			c.showPrompt()
		}
	case "prompt":
		c.shell, _ = data.(string)
	case "clear":
		if c.color {
			fmt.Fprint(c.out, "\x1b[2J\x1b[H")
		}
		c.lineStart = true
	}
}

// 入力を 1 行ずつ読んで、窓のルミで打ったのと同じように渡す
func (c *cliUI) loop() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		for range sig {
			c.mu.Lock()
			c.lastCtrlC = time.Now()
			busy := c.l.isBusy()
			if !busy {
				c.clearLine()
				c.atPrompt = false
				fmt.Fprint(c.out, c.paint("^C  "+T("cli.howToExit"), "dim")+"\n")
				c.lineStart = true
				c.showPrompt()
			}
			c.mu.Unlock()
			if busy {
				c.l.interrupt()
			}
		}
	}()

	c.mu.Lock()
	c.showPrompt()
	c.mu.Unlock()
	in := bufio.NewReader(os.Stdin)
	for {
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			// Ctrl+C で読み込みが切れたときは続ける。本当に入力が終わったら (Ctrl+Z / Ctrl+D・パイプの終わり) 終わる
			time.Sleep(150 * time.Millisecond)
			c.mu.Lock()
			recent := time.Since(c.lastCtrlC) < time.Second
			c.mu.Unlock()
			if recent {
				continue
			}
			// 返事の途中なら終わるのを待ってから
			for c.l.isBusy() {
				time.Sleep(200 * time.Millisecond)
			}
			c.l.quit()
			return
		}
		line = strings.TrimRight(line, "\r\n")
		// 返事の途中に打った行は、返事が終わるのを待ってから渡す (先打ち)。
		// その間に「実行してよい？」と聞かれたら、その答えにする
		asking := false
		for {
			c.mu.Lock()
			asking = c.asking
			if asking {
				c.asking = false
			}
			c.atPrompt = false
			c.mu.Unlock()
			if asking || !c.l.isBusy() {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		c.mu.Lock()
		c.lineStart = true
		c.mu.Unlock()
		if asking {
			select {
			case c.l.answers <- line:
			default:
			}
			continue
		}
		t := strings.TrimSpace(line)
		if !c.l.shellOn && (t == "exit" || t == "quit") {
			c.l.quit()
			return
		}
		c.l.submitTyped(line)
		c.mu.Lock()
		if !c.l.isBusy() && !c.atPrompt {
			c.showPrompt()
		}
		c.mu.Unlock()
	}
}

// 終わる (Lumi.quit から)
func (c *cliUI) exit() {
	c.mu.Lock()
	c.clearLine()
	if !c.lineStart {
		fmt.Fprintln(c.out)
	}
	c.mu.Unlock()
	os.Exit(0)
}

// イベントの秒数 (Go からは int のことも float64 のこともある)
func seconds(v any) time.Duration {
	switch n := v.(type) {
	case float64:
		return time.Duration(n * float64(time.Second))
	case int:
		return time.Duration(n) * time.Second
	}
	return 0
}
