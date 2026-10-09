package main

// ルミ自身のことを聞かれたら、Web 検索ではなくルミの説明書 (GitHub のリポジトリの README) を見る。
//   AI の <lumidoc>知りたいこと</lumidoc>、または「ルミ」「Lumi」を含む <search> をここで受ける。
// README を見出し・表の行・箇条書きに分けて、質問と言葉がよく重なるところだけを AI に渡す
// (ローカルAIは一度に読める量が少ないので、全部は渡さない)。ネットにつながらなければ、コマンドの一覧で答える。

import (
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	lumiRepoURL   = "https://github.com/Hotakacchi/ai-console"
	lumiReadmeURL = "https://raw.githubusercontent.com/Hotakacchi/ai-console/main/README.md"
)

var lumiReadmeCache struct {
	sync.Mutex
	text string
	at   time.Time
}

// README (1 時間は覚えておく)
func lumiReadme() (string, error) {
	c := &lumiReadmeCache
	c.Lock()
	defer c.Unlock()
	if c.text != "" && time.Since(c.at) < time.Hour {
		return c.text, nil
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Get(lumiReadmeURL)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", &httpStatusError{res.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	c.text, c.at = string(b), time.Now()
	return c.text, nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "HTTP " + http.StatusText(e.code) }

// ルミについての質問か (小さいモデルが <search> で調べようとしたとき用)
func aboutLumi(q string) bool {
	q = strings.ToLower(q)
	for _, w := range []string{"ルミ", "るみ", "lumi", "ai-console"} {
		if strings.Contains(q, w) {
			return true
		}
	}
	return false
}

// README を、見出しつきの小さな項目 (段落・表の行・箇条書き) に分ける。今の言語の部分だけ
func docItems(md string) []string {
	ja, en, _ := strings.Cut(md, "\n## English")
	part := ja
	if currentLang() == "en" && en != "" {
		part = en
	}
	var items []string
	heading := ""
	var para []string
	flush := func() {
		if t := strings.TrimSpace(strings.Join(para, " ")); t != "" {
			items = append(items, strings.TrimSpace(heading+" "+t))
		}
		para = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(part, "\r", ""), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "#"):
			flush()
			heading = "【" + strings.TrimSpace(strings.TrimLeft(t, "#")) + "】"
		case t == "" || strings.HasPrefix(t, "<") || strings.HasPrefix(t, "|---") || strings.HasPrefix(t, "| ---"):
			flush() // 空行・画像などの HTML・表の区切り
		case strings.HasPrefix(t, "|") || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			flush() // 表の行や箇条書きは 1 つずつ
			para = []string{strings.Trim(t, "|-* ")}
			flush()
		default:
			para = append(para, t)
		}
	}
	flush()
	return items
}

// 2 文字ずつのかたまり (日本語は単語に区切れないので、これで重なりを数える)
func bigrams(s string) map[string]bool {
	s = strings.ToLower(s)
	var r []rune
	for _, c := range s {
		if c > ' ' && !strings.ContainsRune("、。,.!?！？「」『』()（）[]【】`*|:：/-_", c) {
			r = append(r, c)
		}
	}
	m := map[string]bool{}
	for i := 0; i+1 < len(r); i++ {
		m[string(r[i:i+2])] = true
	}
	return m
}

// 質問に関係するところを選ぶ (README の順に並べ、limit 文字まで)
func pickDoc(items []string, query string, limit int) []string {
	q := bigrams(query)
	for _, w := range []string{"ルミ", "るみ", "lu", "um", "mi", "教え", "えて", "とは", "って", "何が", "でき", "きる"} {
		delete(q, w) // どこにでもある言葉は数えない
	}
	type scored struct {
		i     int
		score float64
	}
	// あちこちの項目に出てくるかたまり (「イン」など) ほど軽く、珍しいものほど重く数える
	grams := make([]map[string]bool, len(items))
	df := map[string]int{}
	for i, it := range items {
		grams[i] = bigrams(it)
		for g := range grams[i] {
			df[g]++
		}
	}
	var list []scored
	for i := range items {
		score := 0.0
		for g := range q {
			if grams[i][g] {
				score += math.Log(float64(len(items)+1) / float64(df[g]))
			}
		}
		if score > 0 {
			list = append(list, scored{i, score / (1 + float64(len(grams[i]))/150)}) // 長い項目は少し割り引く
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a].score > list[b].score })
	var chosen []int
	total := 0
	for _, s := range list {
		// いちばん合う項目の 4 割に満たないものは、関係が薄いので渡さない
		if s.score < list[0].score*0.4 || total+len([]rune(items[s.i])) > limit {
			continue
		}
		chosen = append(chosen, s.i)
		total += len([]rune(items[s.i]))
	}
	sort.Ints(chosen)
	out := make([]string, len(chosen))
	for k, i := range chosen {
		out[k] = items[i]
	}
	return out
}

// ルミの機能や使い方についての質問か (「ルミ」と「機能・使い方・できること」などが両方ある)
func askingAboutLumi(q string) bool {
	if !aboutLumi(q) {
		return false
	}
	low := strings.ToLower(q)
	for _, w := range []string{"機能", "使い方", "できる", "新し", "コマンド", "設定", "やり方", "方法", "どうやって", "feature", "how", "can you", "command", "setting", "new"} {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// 小さいモデルは道具を使わずに答えがちなので、ルミについての質問には、先に説明書の関係するところを添える
func (l *Lumi) docContext(question string) string {
	if !askingAboutLumi(question) {
		return ""
	}
	limit := 4000
	if l.usingLocal() {
		limit = 2000
	}
	// 版 (v1.6.0 など) を聞かれたらその版の、「新機能」「最新」ならいちばん新しい版のリリースノートを添える
	if notes, tags := releaseNotesFor(question); notes != "" {
		if r := []rune(notes); len(r) > limit {
			notes = string(r[:limit]) + "…"
		}
		l.write(T("doc.readingNotes", tags)+"\n", "cyan")
		return "\n\n" + T("doc.notesSource", tags) + "\n" + notes + "\n" + T("doc.answerHint")
	}
	md, err := lumiReadme()
	if err != nil {
		return ""
	}
	picked := pickDoc(docItems(md), question, limit)
	if len(picked) == 0 {
		return ""
	}
	l.write(T("doc.reading")+"\n", "cyan")
	return "\n\n" + T("doc.source", lumiRepoURL) + "\n" + strings.Join(picked, "\n") + "\n" + T("doc.answerHint")
}

func asksWhatsNew(q string) bool {
	low := strings.ToLower(q)
	for _, w := range []string{"新機能", "新しい", "最新", "アップデート", "変わった", "増えた", "what's new", "whats new", "new feature", "latest", "update"} {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

var releaseNotesCache struct {
	sync.Mutex
	body map[string]string // タグ ("" は最新) → リリースノート
	tag  map[string]string // "" → 最新のタグ
	at   map[string]time.Time
}

var versionRe = regexp.MustCompile(`(?i)v?(\d+)\.(\d+)(?:\.(\d+))?`)

// リリースノート (1 時間は覚えておく)。tag が "" なら最新
func releaseBody(tag string) (string, string, error) {
	c := &releaseNotesCache
	c.Lock()
	defer c.Unlock()
	if c.body == nil {
		c.body, c.tag, c.at = map[string]string{}, map[string]string{}, map[string]time.Time{}
	}
	if b, ok := c.body[tag]; ok && time.Since(c.at[tag]) < time.Hour {
		return b, c.tag[tag], nil
	}
	var r releaseInfo
	var err error
	if tag == "" {
		r, err = fetchLatestRelease()
	} else {
		r, err = fetchReleaseByTag(tag)
	}
	if err != nil {
		return "", "", err
	}
	c.body[tag], c.tag[tag], c.at[tag] = r.Body, r.Tag, time.Now()
	return r.Body, r.Tag, nil
}

// 質問に合うリリースノート (今の言語の部分) と、その版の名前 ("" なら合うものなし)。
// 版が書いてあればその版。なければ「新機能」などのとき最新の版で、修正だけの版 (1.6.1 など) なら元の版 (1.6.0) も
func releaseNotesFor(question string) (string, string) {
	var tags []string
	for _, m := range versionRe.FindAllStringSubmatch(question, 3) {
		patch := m[3]
		if patch == "" {
			patch = "0"
		}
		tags = append(tags, "v"+m[1]+"."+m[2]+"."+patch)
	}
	if len(tags) == 0 {
		if !asksWhatsNew(question) {
			return "", ""
		}
		_, latest, err := releaseBody("")
		if err != nil {
			return "", ""
		}
		tags = []string{latest}
		if m := versionRe.FindStringSubmatch(latest); m != nil && m[3] != "" && m[3] != "0" {
			tags = append(tags, "v"+m[1]+"."+m[2]+".0")
		}
	}
	var parts, names []string
	for _, tag := range tags {
		body, name, err := releaseBody(tag)
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		parts = append(parts, "### "+name+"\n"+notesForLang(body))
		names = append(names, name)
	}
	return strings.Join(parts, "\n\n"), strings.Join(names, ", ")
}

// リリースノートは日本語のあとに英語 (## New など) が続き、最後にインストールの表がある。今の言語の部分だけ
func notesForLang(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r", ""), "\n")
	english := -1
	end := len(lines)
	for i, line := range lines {
		if strings.HasPrefix(line, "## インストール") {
			end = i
			break
		}
		if english < 0 && strings.HasPrefix(line, "## ") && isASCII(line) {
			english = i
		}
	}
	if english < 0 {
		english = end
	}
	part := lines[:english]
	if currentLang() == "en" && english < end {
		part = lines[english:end]
	}
	return strings.TrimSpace(strings.Join(part, "\n"))
}

// AI の <lumidoc> (と、ルミについての <search>): 説明書から関係するところを返す
func (l *Lumi) lumiDocTool(query string) string {
	l.write(T("doc.reading")+"\n", "cyan")
	head := "\n[" + T("doc.label") + "] " + query + "\n"
	limit := 6000
	if l.usingLocal() {
		limit = 2500
	}
	md, err := lumiReadme()
	if err != nil {
		// ネットにつながらない: アプリに入っているコマンドの一覧で答える
		var b strings.Builder
		for _, c := range commands {
			b.WriteString(c.name + ": " + c.help() + "\n")
		}
		return head + T("doc.offline", err.Error()) + "\n" + b.String()
	}
	picked := pickDoc(docItems(md), query, limit)
	if len(picked) == 0 {
		return head + T("doc.none", lumiRepoURL) + "\n"
	}
	for _, p := range picked {
		if r := []rune(p); len(r) > 70 {
			p = string(r[:70]) + "…"
		}
		l.write("  "+p+"\n", "dim")
	}
	l.write("\n", "fg")
	return head + T("doc.source", lumiRepoURL) + "\n" + strings.Join(picked, "\n") + "\n"
}

// text が、前に言ったどれかとほとんど同じか (2 文字ずつのかたまりが 45% 以上重なる。言い回しを変えた繰り返しで 5 割、ふつうの続きの返事は 2 割ほど)
func repeatsEarlier(earlier []string, text string) bool {
	b := bigrams(text)
	if len(b) < 10 {
		return false
	}
	for _, e := range earlier {
		a := bigrams(e)
		if len(a) < 10 {
			continue
		}
		both := 0
		for g := range b {
			if a[g] {
				both++
			}
		}
		if float64(both)/float64(min(len(a), len(b))) >= 0.45 {
			return true
		}
	}
	return false
}
