package main

// Web 検索とページの読み込み。どの AI プロバイダーでも使えるよう、アプリ側で行う。
//  - 検索: DuckDuckGo (API キー不要) か、自分で立てた SearXNG
//  - 読み込み: 公開サイトの HTML から本文の文字だけを取り出す (PC 内・家庭内ネットワークは読まない)

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

const (
	maxSearchResults = 5
	maxPageChars     = 3000
	browserUA        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
)

type hit struct{ Title, URL, Snippet string }

// searxng が空なら DuckDuckGo を使う
func webSearch(query, searxng string) (string, error) {
	var hits []hit
	var err error
	if searxng == "" {
		hits, err = duckDuckGo(query)
	} else {
		hits, err = searXNG(query, searxng)
	}
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "(検索結果がありませんでした)", nil
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n", i+1, h.Title, h.URL, h.Snippet)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

var (
	ddgLink    = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	ddgSnippet = regexp.MustCompile(`class="result__snippet"[^>]*>([\s\S]*?)</a>`)
)

func duckDuckGo(query string) ([]hit, error) {
	body, err := fetchRaw("https://html.duckduckgo.com/html/", url.Values{"q": {query}, "kl": {"jp-jp"}}, false)
	if err != nil {
		return nil, err
	}
	links := ddgLink.FindAllStringSubmatch(body, -1)
	snippets := ddgSnippet.FindAllStringSubmatch(body, -1)
	var hits []hit
	for i, l := range links {
		if len(hits) >= maxSearchResults {
			break
		}
		u := html.UnescapeString(l[1])
		if strings.Contains(u, "duckduckgo.com/y.js") { // 広告
			continue
		}
		// DuckDuckGo の転送リンク (/l/?uddg=本当のURL) をほどく
		if p, err := url.Parse(u); err == nil && p.Query().Get("uddg") != "" {
			u = p.Query().Get("uddg")
		}
		if strings.HasPrefix(u, "//") {
			u = "https:" + u
		}
		h := hit{Title: plainText(l[2]), URL: u}
		if i < len(snippets) {
			h.Snippet = plainText(snippets[i][1])
		}
		hits = append(hits, h)
	}
	return hits, nil
}

func searXNG(query, base string) ([]hit, error) {
	body, err := fetchRaw(strings.TrimRight(base, "/")+"/search?format=json&language=ja&q="+url.QueryEscape(query), nil, false)
	if err != nil {
		return nil, err
	}
	var r struct {
		Results []struct{ Title, URL, Content string } `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return nil, err
	}
	var hits []hit
	for _, x := range r.Results {
		if len(hits) >= maxSearchResults {
			break
		}
		hits = append(hits, hit{x.Title, x.URL, x.Content})
	}
	return hits, nil
}

var (
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	dropRe    = regexp.MustCompile(`(?is)<(script|style|noscript|svg|nav|header|footer)\b.*?</(script|style|noscript|svg|nav|header|footer)>`)
	breakRe   = regexp.MustCompile(`(?i)<(br|p|div|li|h[1-6]|tr)\b[^>]*>`)
	tagRe     = regexp.MustCompile(`<[^>]+>`)
	spaceRe   = regexp.MustCompile(`[ \t\r\f\v\x{00a0}]+`)
	blankLnRe = regexp.MustCompile(`\n\s*\n+`)
)

// ページを読み、本文の文字だけを返す
func fetchPage(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "(http か https の URL だけ読めます)", nil
	}
	if isPrivateHost(u.Hostname()) {
		return "(PC 内や家庭内ネットワークのページは読めません)", nil
	}
	body, err := fetchRaw(u.String(), nil, true)
	if err != nil {
		return "", err
	}
	title := ""
	if m := titleRe.FindStringSubmatch(body); m != nil {
		title = plainText(m[1])
	}
	text := dropRe.ReplaceAllString(body, " ")
	text = breakRe.ReplaceAllString(text, "\n")
	text = blankLnRe.ReplaceAllString(plainText(text), "\n")
	if r := []rune(text); len(r) > maxPageChars {
		text = string(r[:maxPageChars]) + "…(以下省略)"
	}
	if title != "" {
		text = "タイトル: " + title + "\n" + text
	}
	return text, nil
}

// localhost・プライベート IP などには行かない (名前解決した先も確かめる)
func isPrivateHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return true
		}
	}
	return false
}

// GET (form が nil) か POST。checkHosts なら転送先が PC 内・家庭内でないか 1 回ずつ確かめる
func fetchRaw(target string, form url.Values, checkHosts bool) (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("転送が多すぎます")
			}
			if checkHosts && ((req.URL.Scheme != "http" && req.URL.Scheme != "https") || isPrivateHost(req.URL.Hostname())) {
				return errors.New("PC 内や家庭内ネットワークへの転送なので読みません")
			}
			return nil
		},
	}
	var req *http.Request
	if form != nil {
		req, _ = http.NewRequest("POST", target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, _ = http.NewRequest("GET", target, nil)
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ja,en;q=0.8")
	res, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) && ue.Err != nil {
			return "", ue.Err
		}
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	// 文字コードはヘッダーか <meta> から判断して UTF-8 にそろえる (Shift_JIS のページなど)
	r, err := charset.NewReader(io.LimitReader(res.Body, 2<<20), res.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(r)
	return string(b), err
}

// タグを取り、文字参照を戻し、空白をまとめる
func plainText(s string) string {
	s = html.UnescapeString(tagRe.ReplaceAllString(s, ""))
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}
