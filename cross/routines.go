package main

// ルーティン: 決まった言葉 (「おはよう」「仕事モード」など) で、いくつかのことをまとめて行う。
// データフォルダの routines.txt に書く (/routines で開く。なければ標準の内容を使う)。
//
//	[おはよう @07:30]          ← 名前。@時刻 を付けると毎日その時刻にも動く
//	天気                        ← 今日の天気 (weather_location の場所、空なら自動)
//	予定                        ← 今日のリマインダー
//	開く: https://…             ← URL・フォルダ・ファイルを開く
//	実行: コマンド              ← コマンドを実行する (自分で書いたものなので確認しない)
//	言う: こんにちは            ← そのまま喋る
//	AI: 一言あいさつして        ← ここまでに集めた天気・予定などを添えて AI に頼む (最後に 1 回)
//
// 名前と同じことを打つか話しかけると動く。英語の書き方 (weather / reminders / open: / run: / say: / ai:) も使える。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type routineStep struct {
	Kind string // weather / reminders / open / run / say / ai
	Arg  string
}

type routine struct {
	Name  string
	At    string // "07:30" (毎日その時刻に動く。空なら動かない)
	Steps []routineStep
}

func routinesPath() string { return filepath.Join(dataDir(), "routines.txt") }

var routineHeadRe = regexp.MustCompile(`^\[(.+?)(?:\s*@\s*(\d{1,2}:\d{2}))?\]$`)

// routines.txt の中身 (なければ標準のもの)
func loadRoutines() []routine {
	data, err := os.ReadFile(routinesPath())
	if err != nil {
		return parseRoutines(T("routines.template"))
	}
	return parseRoutines(string(data))
}

func parseRoutines(text string) []routine {
	var list []routine
	var cur *routine
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := routineHeadRe.FindStringSubmatch(line); m != nil {
			at := m[2]
			if at != "" {
				if t, err := time.Parse("15:04", at); err == nil {
					at = t.Format("15:04")
				} else {
					at = ""
				}
			}
			list = append(list, routine{Name: strings.TrimSpace(m[1]), At: at})
			cur = &list[len(list)-1]
			continue
		}
		if cur != nil {
			cur.Steps = append(cur.Steps, parseStep(line))
		}
	}
	return list
}

// 1 行を手順にする (日本語・英語の書き方)
func parseStep(line string) routineStep {
	low := strings.ToLower(line)
	switch low {
	case "天気", "weather":
		return routineStep{Kind: "weather"}
	case "予定", "reminders", "schedule":
		return routineStep{Kind: "reminders"}
	}
	for kind, heads := range map[string][]string{
		"open": {"開く", "open"},
		"run":  {"実行", "run"},
		"say":  {"言う", "say"},
		"ai":   {"ai"},
	} {
		for _, h := range heads {
			for _, sep := range []string{":", "："} {
				if strings.HasPrefix(low, h+sep) {
					return routineStep{Kind: kind, Arg: strings.TrimSpace(line[len(h)+len(sep):])}
				}
			}
		}
	}
	return routineStep{Kind: "say", Arg: line} // 書き方に当てはまらない行は、そのまま喋る
}

var routinePunct = regexp.MustCompile(`[\s、。,.!！?？~〜ー-]+`)

func routineKey(s string) string { return strings.ToLower(routinePunct.ReplaceAllString(s, "")) }

// 発言がルーティンの名前なら、そのルーティン
func (l *Lumi) findRoutine(text string) (routine, bool) {
	key := routineKey(text)
	if w := routineKey(l.s.WakeWord()); w != "" {
		key = strings.TrimPrefix(key, w) // 「ルミ、おはよう」も
	}
	if key == "" {
		return routine{}, false
	}
	for _, r := range loadRoutines() {
		if routineKey(r.Name) == key {
			return r, true
		}
	}
	return routine{}, false
}

// ルーティンを動かす (呼ぶ前に begin で忙しい状態にしておく。最後に忙しい状態を解く)
func (l *Lumi) runRoutine(r routine) {
	l.write("  "+T("routine.start", r.Name)+"\n", "cyan")
	l.emit("flash", map[string]any{"expr": "happy", "seconds": 2})
	var info []string // AI に渡す材料
	aiPrompt := ""
	for _, st := range r.Steps {
		if l.cancelled() {
			break
		}
		switch st.Kind {
		case "weather":
			w, kind, err := l.weather()
			if err != nil {
				w = T("routine.weatherFailed", err.Error())
			} else if kind != "" {
				l.fx(kind, 12) // 晴れならサングラス、雨なら傘
			}
			info = append(info, w)
		case "reminders":
			info = append(info, todaysReminders())
		case "open":
			openURL(st.Arg)
			l.write("  "+T("routine.opened", st.Arg)+"\n", "dim")
		case "run":
			l.write("  $ "+st.Arg+"\n", "dim")
			var out strings.Builder
			l.emit("running", map[string]any{"on": true, "cmd": st.Arg})
			runShell(st.Arg, homeDir(), l.cancelChan(), func(line string) {
				l.write(line+"\n", "fg")
				out.WriteString(line + "\n")
			})
			l.emit("running", map[string]any{"on": false})
			info = append(info, "$ "+st.Arg+"\n"+strings.TrimSpace(out.String()))
		case "say":
			l.write("\n", "fg")
			l.speak(st.Arg)
			l.write("\n", "fg")
		case "ai":
			aiPrompt = st.Arg
		}
	}
	// AI に頼むなら、集めた材料を付けて頼む (AI がなければ、材料をそのまま読み上げる)
	_, offline := l.ai.(offlineProvider)
	if aiPrompt != "" && !offline && !l.cancelled() {
		msg := aiPrompt
		if len(info) > 0 {
			msg += "\n\n" + T("routine.info") + "\n" + strings.Join(info, "\n")
		}
		l.respond(msg, false) // 最後に忙しい状態を解く
		return
	}
	for _, s := range info {
		if l.cancelled() {
			break
		}
		l.write("\n", "fg")
		for _, line := range strings.Split(s, "\n") {
			l.speak(line)
		}
		l.write("\n", "fg")
	}
	l.write("\n", "fg")
	l.setBusy(false)
}

// 今日のリマインダー
func todaysReminders() string {
	now := time.Now()
	var items []string
	for _, it := range reminders.list() {
		if it.At.Year() == now.Year() && it.At.YearDay() == now.YearDay() {
			items = append(items, T("routine.reminderItem", it.At.Format("15:04"), it.Text))
		}
	}
	if len(items) == 0 {
		return T("routine.noReminders")
	}
	return T("routine.reminders", strings.Join(items, "、"))
}

// ---- 天気 (wttr.in、API キー不要) ----

// 天気の文と、顔の演出の種類 (sun / rain / snow / cloud)
func (l *Lumi) weather() (string, string, error) {
	if !l.s.WebSearch() {
		return "", "", fmt.Errorf("%s", T("routine.webOff"))
	}
	loc := strings.TrimSpace(l.s.Get("weather_location", "")) // 空なら、つないでいる場所から自動で
	lang := currentLang()
	u := "https://wttr.in/" + url.PathEscape(loc) + "?format=j1&lang=" + lang
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "curl/8 (Lumi)")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var w struct {
		Current []map[string]any `json:"current_condition"`
		Area    []struct {
			Name []struct{ Value string } `json:"areaName"`
		} `json:"nearest_area"`
		Weather []struct {
			Max    string `json:"maxtempC"`
			Min    string `json:"mintempC"`
			Hourly []struct {
				Rain string `json:"chanceofrain"`
			} `json:"hourly"`
		} `json:"weather"`
	}
	if err := json.NewDecoder(res.Body).Decode(&w); err != nil || len(w.Current) == 0 || len(w.Weather) == 0 {
		return "", "", fmt.Errorf("%s", T("routine.weatherBad"))
	}
	c := w.Current[0]
	desc := ""
	if v, ok := c["lang_"+lang].([]any); ok && len(v) > 0 {
		desc = str(v[0].(map[string]any), "value")
	}
	if desc == "" {
		if v, ok := c["weatherDesc"].([]any); ok && len(v) > 0 {
			desc = str(v[0].(map[string]any), "value")
		}
	}
	if loc == "" && len(w.Area) > 0 && len(w.Area[0].Name) > 0 {
		loc = w.Area[0].Name[0].Value
	}
	rain := 0
	for _, h := range w.Weather[0].Hourly {
		var n int
		fmt.Sscan(h.Rain, &n)
		rain = max(rain, n)
	}
	return T("routine.weather", loc, strings.TrimSpace(desc), str(c, "temp_C"), w.Weather[0].Max, w.Weather[0].Min, rain), weatherKind(str(c, "weatherCode")), nil
}

// ---- 時刻で動くルーティン ----

var (
	routineRanMu sync.Mutex
	routineRan   = map[string]string{} // 名前 → 最後に動いた日
)

// 毎分、@時刻 のルーティンを確かめる (runReminders から呼ぶ)
func (l *Lumi) checkScheduledRoutines(now time.Time) {
	hm, day := now.Format("15:04"), now.Format("2006-01-02")
	for _, r := range loadRoutines() {
		if r.At != hm {
			continue
		}
		routineRanMu.Lock()
		done := routineRan[r.Name] == day
		routineRan[r.Name] = day
		routineRanMu.Unlock()
		if done {
			continue
		}
		go func(r routine) {
			for !l.begin() { // 返事の途中なら終わるまで待つ
				time.Sleep(500 * time.Millisecond)
			}
			if !l.win.IsVisible() {
				l.peek.pop(r.Name)
				defer func() {
					time.Sleep(2 * time.Second)
					l.peek.retract(false, "")
				}()
			}
			l.runRoutine(r)
		}(r)
	}
}

// /routines: 一覧と、routines.txt を開く。/routines <名前> でそのルーティンを動かす
func (l *Lumi) routinesCommand(arg string) {
	if arg = strings.TrimSpace(arg); arg != "" {
		r, ok := l.findRoutine(arg)
		if !ok {
			l.errorText(T("routine.noSuch", arg))
			return
		}
		if l.begin() {
			go l.runRoutine(r)
		}
		return
	}
	p := routinesPath()
	if _, err := os.Stat(p); err != nil {
		os.MkdirAll(dataDir(), 0o755)
		os.WriteFile(p, []byte(T("routines.template")), 0o644)
	}
	var b strings.Builder
	for _, r := range loadRoutines() {
		at := ""
		if r.At != "" {
			at = "  @" + r.At
		}
		fmt.Fprintf(&b, "  [%s]%s  %s\n", r.Name, at, T("routine.steps", len(r.Steps)))
	}
	b.WriteString("\n  " + T("routine.howto", p))
	openFile(p)
	l.info(b.String())
}

// wttr.in の天気の番号を、顔の演出の種類にする
func weatherKind(code string) string {
	switch code {
	case "113":
		return "sun"
	case "116", "119", "122", "143", "248", "260":
		return "cloud"
	case "179", "182", "185", "227", "230", "317", "320", "323", "326", "329", "332", "335", "338", "350", "362", "365", "368", "371", "374", "377", "392", "395":
		return "snow"
	case "":
		return ""
	}
	return "rain" // それ以外は雨・雷・霧雨など
}
