package main

// ルミ自身のことを聞かれたら、Web 検索ではなくルミの説明書 (GitHub のリポジトリの README) を見る。
//   AI の <lumidoc>知りたいこと</lumidoc>、または「ルミ」「Lumi」を含む <search> をここで受ける。
// README を見出し・表の行・箇条書きに分けて、質問と言葉がよく重なるところだけを AI に渡す
// (ローカルAIは一度に読める量が少ないので、全部は渡さない)。ネットにつながらなければ、コマンドの一覧で答える。

import (
	"io"
	"math"
	"net/http"
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
