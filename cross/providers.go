package main

// AI プロバイダー (返事の取得先)。新しいものを足すときは Provider を実装して newProvider に登録する。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Provider interface {
	Label() string
	// 返事を断片ごとに onText に渡す (呼び出し側のゴルーチンで実行される)
	Reply(t Turn, onText func(string))
	// 返事の取得を途中で打ち切る
	Abort()
	// 会話履歴を消す
	Clear()
	// 前の会話 (user / assistant の文) を履歴として渡す
	Seed(msgs []Message)
}

type Message = map[string]any

// 1 回の発言: 文字と、AI に見せる画像 (JPEG)
type Turn struct {
	Text   string
	Images [][]byte
}

// 見せた画像は、返事が終わったら履歴から外して印だけ残す (毎回送り直すと重いので)
func imagePlaceholder(n int) string { return "\n" + T("image.shown", n) }

// 履歴のおおよその大きさ (文字数)。足りなくなりそうなら古い発言から捨てる
func messageSize(m Message) int {
	if parts, ok := m["content"].([]any); ok {
		n := 0
		for _, p := range parts {
			pm, _ := p.(map[string]any)
			if t := str(pm, "type"); t == "image" || t == "image_url" {
				n += 1500 // 画像 1 枚はおよそこのくらい (縮めてから渡すので)
				continue
			}
			b, _ := json.Marshal(p)
			n += len([]rune(string(b)))
		}
		return n
	}
	b, _ := json.Marshal(m["content"])
	return len([]rune(string(b)))
}

func trimHistory(history []Message, budget int) []Message {
	if budget <= 0 {
		return history
	}
	total := 0
	for _, m := range history {
		total += messageSize(m)
	}
	// 最後 (今の発言) は残す。user から始まるように assistant だけ残ったら一緒に捨てる
	for len(history) > 1 && (total > budget || history[0]["role"] != "user") {
		total -= messageSize(history[0])
		history = history[1:]
	}
	return history
}

func newProvider(s *Settings) Provider {
	switch strings.ToLower(s.Get("provider", "offline")) {
	case "anthropic":
		return newAnthropic(s)
	case "openai":
		return newOpenAI(s)
	case "local":
		return newLocal(s)
	case "command":
		return &commandProvider{s: s}
	}
	return offlineProvider{}
}

// ---------------- HTTP + Server-Sent Events の土台 ----------------

type httpBase struct {
	s       *Settings
	history []Message
	mu      sync.Mutex
	cancel  context.CancelFunc
	aborted bool
	budget  int                    // 履歴の上限 (文字数のめやす、0 なら制限なし)
	content func(t Turn) any       // 発言を API の形にする (画像の渡し方が API ごとに違う)
}

func (h *httpBase) Clear()              { h.history = nil }
func (h *httpBase) Seed(msgs []Message) { h.history = append([]Message(nil), msgs...) }

func (h *httpBase) Abort() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.aborted = true
	if h.cancel != nil {
		h.cancel()
	}
}

// send が返した assistant の content を履歴に足す (nil なら今回の発言ごと取り消す)
func (h *httpBase) reply(t Turn, onText func(string), send func(ctx context.Context) (any, error)) {
	var content any = t.Text
	if len(t.Images) > 0 && h.content != nil {
		content = h.content(t)
	}
	h.history = trimHistory(append(h.history, Message{"role": "user", "content": content}), h.budget)
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.aborted = false
	h.cancel = cancel
	h.mu.Unlock()
	defer cancel()

	content, err := send(ctx)
	if err != nil || content == nil {
		h.history = h.history[:len(h.history)-1]
		if err != nil && !h.aborted {
			onText(T("ai.connectFailed", err.Error()))
		}
		return
	}
	if len(t.Images) > 0 {
		h.history[len(h.history)-1]["content"] = t.Text + imagePlaceholder(len(t.Images))
	}
	h.history = append(h.history, Message{"role": "assistant", "content": content})
}

// POST して、SSE の data 行ごとに onEvent を呼ぶ
func postStream(ctx context.Context, url string, headers map[string]string, body any, onEvent func(map[string]any) error) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(b, &e)
		return fmt.Errorf("HTTP %d %s", res.StatusCode, e.Error.Message)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[5:])
		if payload == "[DONE]" {
			break
		}
		var ev map[string]any
		if json.Unmarshal([]byte(payload), &ev) == nil {
			if err := onEvent(ev); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

func obj(m map[string]any, k string) map[string]any {
	v, _ := m[k].(map[string]any)
	return v
}

// ---------------- Anthropic (Claude) ----------------

type anthropicProvider struct {
	httpBase
	model, key, endpoint string
}

func newAnthropic(s *Settings) *anthropicProvider {
	return &anthropicProvider{
		httpBase: httpBase{s: s, content: func(t Turn) any {
			var blocks []any
			for _, img := range t.Images {
				blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{
					"type": "base64", "media_type": "image/jpeg", "data": base64.StdEncoding.EncodeToString(img)}})
			}
			return append(blocks, map[string]any{"type": "text", "text": t.Text})
		}},
		model:    s.Get("model", "claude-opus-5-5"),
		key:      s.APIKey("ANTHROPIC_API_KEY"),
		endpoint: s.Get("endpoint", "https://api.anthropic.com/v1/messages"),
	}
}

func (p *anthropicProvider) Label() string { return "anthropic: " + p.model }

func (p *anthropicProvider) Reply(t Turn, onText func(string)) {
	p.reply(t, onText, func(ctx context.Context) (any, error) {
		if p.key == "" {
			return nil, errors.New(T("ai.noKey"))
		}
		body := map[string]any{
			"model":      p.model,
			"max_tokens": p.s.GetInt("max_tokens", 64000),
			"system":     p.s.SystemPrompt(),
			"messages":   p.history,
			"stream":     true,
		}
		effort := p.s.Get("effort", "")
		if effort == "" && p.model == "claude-opus-5-5" {
			effort = "low"
		}
		if effort != "" {
			body["output_config"] = map[string]any{"effort": effort}
		}
		headers := map[string]string{"x-api-key": p.key, "anthropic-version": "2023-06-01"}
		// 安全フィルターで断られたとき、サーバー側で別モデルに切り替えて答えさせる (対応モデルのみ)
		switch p.model {
		case "claude-opus-5-5", "claude-opus-5", "claude-fable-5-1", "claude-sonnet-5-5":
			headers["anthropic-beta"] = "server-side-fallback-2026-07-01"
			body["fallbacks"] = "default"
		}

		// thinking ブロックも含めて content を組み立て直し、そのまま履歴に返す
		var blocks []map[string]any
		stop := ""
		err := postStream(ctx, p.endpoint, headers, body, func(ev map[string]any) error {
			switch str(ev, "type") {
			case "content_block_start":
				b := obj(ev, "content_block")
				if b == nil {
					b = map[string]any{} // 想定外の形でも後の書き込みで落ちないように
				}
				blocks = append(blocks, b)
			case "content_block_delta":
				if len(blocks) == 0 {
					return nil
				}
				b, d := blocks[len(blocks)-1], obj(ev, "delta")
				switch str(d, "type") {
				case "text_delta":
					b["text"] = str(b, "text") + str(d, "text")
					onText(str(d, "text"))
				case "thinking_delta":
					b["thinking"] = str(b, "thinking") + str(d, "thinking")
				case "signature_delta":
					b["signature"] = str(d, "signature")
				}
			case "message_delta":
				stop = str(obj(ev, "delta"), "stop_reason")
			case "error":
				return errors.New(str(obj(ev, "error"), "message"))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if stop == "refusal" {
			onText(T("ai.refusal"))
			return nil, nil
		}
		return blocks, nil
	})
}

// ---------------- OpenAI 互換 (OpenAI / Ollama / LM Studio / OpenRouter など) ----------------

type openAIProvider struct {
	httpBase
	model, key, endpoint string
	addOptions           func(map[string]any)
	before               func() error // 送る直前の準備 (ローカルAIの起動など)
	label                string
}

func newOpenAI(s *Settings) *openAIProvider {
	p := &openAIProvider{
		httpBase: httpBase{s: s, content: func(t Turn) any {
			var parts []any
			for _, img := range t.Images {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{
					"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(img)}})
			}
			return append(parts, map[string]any{"type": "text", "text": t.Text})
		}},
		model:    s.Get("model", ""),
		key:      s.APIKey(""),
		endpoint: s.Get("endpoint", "https://api.openai.com/v1/chat/completions"),
	}
	p.label = "openai: " + p.model
	return p
}

func (p *openAIProvider) Label() string { return p.label }

func (p *openAIProvider) Reply(t Turn, onText func(string)) {
	p.reply(t, onText, func(ctx context.Context) (any, error) {
		if p.before != nil {
			if err := p.before(); err != nil {
				return nil, err
			}
		}
		messages := append([]Message{{"role": "system", "content": p.s.SystemPrompt()}}, p.history...)
		body := map[string]any{"model": p.model, "messages": messages, "stream": true}
		if n := p.s.GetInt("max_tokens", 0); n > 0 {
			body["max_tokens"] = n
		}
		if p.addOptions != nil {
			p.addOptions(body)
		}
		headers := map[string]string{}
		if p.key != "" {
			headers["Authorization"] = "Bearer " + p.key
		}
		var out strings.Builder
		err := postStream(ctx, p.endpoint, headers, body, func(ev map[string]any) error {
			choices, _ := ev["choices"].([]any)
			if len(choices) == 0 {
				return nil
			}
			c, _ := choices[0].(map[string]any)
			piece := str(obj(c, "delta"), "content")
			if piece != "" {
				out.WriteString(piece)
				onText(piece)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return out.String(), nil
	})
}

// ---------------- ローカル (llama.cpp + 標準モデル) ----------------

func newLocal(s *Settings) Provider {
	dir := dataDir()
	modelPath := localModelPath(dir, s.Get("model", ""))
	gpu := strings.ToLower(s.Get("local_gpu", "auto")) != "off"
	p := newOpenAI(s)
	p.model = strings.TrimSuffix(filepath.Base(modelPath), filepath.Ext(modelPath))
	p.label = "local: " + p.model
	ctx := localContext(s)
	// 指示文と返事の分を残して、履歴はコンテキストに収まる分だけにする (日本語はおよそ 1 文字 1 トークン)
	p.budget = ctx - 3500
	p.before = func() error {
		if err := localServer.Start(dir, modelPath, visionPath(dir, modelPath), gpu, ctx); err != nil {
			return err
		}
		p.endpoint = localServer.Endpoint() + "/v1/chat/completions"
		p.key = localServer.APIKey()
		return nil
	}
	p.addOptions = func(body map[string]any) {
		// Qwen3.5 は既定で考えてから答えるので、会話用に考える過程を切る。サンプリングはモデル推奨値
		body["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
		body["temperature"] = 0.7
		body["top_p"] = 0.8
		body["top_k"] = 20
		body["presence_penalty"] = 1.5
	}
	return &thinkFiltered{p}
}

// <think>...</think> が混ざっても読み上げないようにする
type thinkFiltered struct{ *openAIProvider }

var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

func (t *thinkFiltered) Reply(turn Turn, onText func(string)) {
	var pending strings.Builder
	inside := false
	t.openAIProvider.Reply(turn, func(chunk string) {
		pending.WriteString(chunk)
		s := pending.String()
		var out strings.Builder
		for {
			tag := "<think>"
			if inside {
				tag = "</think>"
			}
			if i := strings.Index(s, tag); i >= 0 {
				if !inside {
					out.WriteString(s[:i])
				}
				s = s[i+len(tag):]
				inside = !inside
				continue
			}
			keep := 0
			for k := min(len(tag)-1, len(s)); k > 0; k-- {
				if strings.HasPrefix(tag, s[len(s)-k:]) {
					keep = k
					break
				}
			}
			if !inside {
				out.WriteString(s[:len(s)-keep])
			}
			s = s[len(s)-keep:]
			break
		}
		pending.Reset()
		pending.WriteString(s)
		if out.Len() > 0 {
			onText(out.String())
		}
	})
	if !inside && pending.Len() > 0 {
		onText(pending.String())
	}
	// 履歴に残す返事からも考える過程を除く
	if n := len(t.history); n > 0 {
		if c, ok := t.history[n-1]["content"].(string); ok {
			t.history[n-1]["content"] = strings.TrimSpace(thinkRe.ReplaceAllString(c, ""))
		}
	}
}

// ---------------- 外部コマンド (自作スクリプトなど) ----------------
// 発言ごとにコマンドを起動し、stdin に {"system": ..., "messages": [...]} の JSON を渡す。stdout が返事になる。

type commandProvider struct {
	s       *Settings
	history []Message
	mu      sync.Mutex
	cmd     *exec.Cmd
	aborted bool
}

func (c *commandProvider) Label() string { return "command: " + c.s.Get("command", "") }
func (c *commandProvider) Clear()              { c.history = nil }
func (c *commandProvider) Seed(msgs []Message) { c.history = append([]Message(nil), msgs...) }

func (c *commandProvider) Abort() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aborted = true
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
}

func (c *commandProvider) Reply(t Turn, onText func(string)) {
	command := c.s.Get("command", "")
	if command == "" {
		onText(T("command.empty"))
		return
	}
	// 画像は "images" に base64 の JPEG で渡す (次からは印だけにする)
	msg := Message{"role": "user", "content": t.Text}
	if len(t.Images) > 0 {
		var imgs []string
		for _, img := range t.Images {
			imgs = append(imgs, base64.StdEncoding.EncodeToString(img))
		}
		msg["images"] = imgs
	}
	c.history = append(c.history, msg)
	defer func() {
		if len(t.Images) > 0 {
			for i := range c.history {
				if _, ok := c.history[i]["images"]; ok {
					delete(c.history[i], "images")
					c.history[i]["content"] = t.Text + imagePlaceholder(len(t.Images))
				}
			}
		}
	}()
	cmd := shellCommand(command)
	cmd.Dir = dataDir()
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	payload, _ := json.Marshal(map[string]any{"system": c.s.SystemPrompt(), "messages": c.history})
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, _ := cmd.StdoutPipe()
	hideWindow(cmd)
	c.mu.Lock()
	c.aborted = false
	c.cmd = cmd
	c.mu.Unlock()

	var reply strings.Builder
	if err := cmd.Start(); err != nil {
		onText(T("command.failed", err.Error()))
		c.history = c.history[:len(c.history)-1]
		return
	}
	buf := make([]byte, 1024)
	for {
		n, err := out.Read(buf)
		if n > 0 {
			piece := strings.ReplaceAll(string(buf[:n]), "\r", "")
			reply.WriteString(piece)
			onText(piece)
		}
		if err != nil {
			break
		}
	}
	err := cmd.Wait()
	if err != nil && !c.aborted && strings.TrimSpace(reply.String()) == "" {
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		onText(T("command.errorExit", strings.TrimSpace(lines[len(lines)-1])))
	}
	if r := strings.TrimSpace(reply.String()); r != "" && !c.aborted {
		c.history = append(c.history, Message{"role": "assistant", "content": r})
	} else {
		c.history = c.history[:len(c.history)-1]
	}
}

// OS ごとのシェルでコマンドを動かす
func shellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/d", "/s", "/c", command)
	}
	return exec.Command("/bin/sh", "-c", command)
}

// ---------------- オフライン (AI なし) ----------------

type offlineProvider struct{}

func (offlineProvider) Label() string { return "offline" }
func (offlineProvider) Abort()        {}
func (offlineProvider) Clear()        {}
func (offlineProvider) Seed([]Message) {}

func (offlineProvider) Reply(t Turn, onText func(string)) {
	text := t.Text
	now := time.Now()
	lower := strings.ToLower(text)
	// 言語ごとの言葉の一覧 (offline.words.*) のどれかを含むか。英単語は単語の区切りで比べる
	has := func(key string) bool {
		for _, w := range TList(key) {
			w = strings.ToLower(w)
			if isASCII(w) {
				if containsWord(lower, w) {
					return true
				}
			} else if strings.Contains(lower, w) {
				return true
			}
		}
		return false
	}
	pick := func(key string, i int) string {
		if list := TList(key); i < len(list) {
			return list[i]
		}
		return ""
	}
	switch {
	case has("offline.words.time"):
		onText(T("offline.time", now.Hour(), now.Minute()))
	case has("offline.words.date"):
		onText(T("offline.date", int(now.Month()), now.Day(), pick("offline.weekdays", int(now.Weekday())), pick("offline.months", int(now.Month())-1)))
	case has("offline.words.hello"):
		onText(T("offline.hello"))
	case has("offline.words.name"):
		onText(T("offline.name"))
	case has("offline.words.thanks"):
		onText(T("offline.thanks"))
	default:
		onText(T("offline.default"))
	}
}

// s の中に w が、前後が英数字でない形で入っているか ("hi" は "this" に一致しない)
func containsWord(s, w string) bool {
	isWordByte := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
	}
	for start := 0; ; {
		i := strings.Index(s[start:], w)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(w)
		if (i == 0 || !isWordByte(s[i-1])) && (end == len(s) || !isWordByte(s[end])) {
			return true
		}
		start = i + 1
	}
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// ---------------- 文の区切り ----------------

type sentenceSplitter struct{ buf strings.Builder }

const sentenceEnds = "。！？!?\n"
const sentenceClosers = "」』）)\"'"

// 断片を足し、区切れた文を返す (文末の直後の閉じかっこは同じ文に含める)
func (sp *sentenceSplitter) Push(chunk string) []string {
	sp.buf.WriteString(chunk)
	var out []string
	for {
		s := []rune(sp.buf.String())
		i := -1
		for k, r := range s {
			if strings.ContainsRune(sentenceEnds, r) {
				i = k
				break
			}
		}
		if i < 0 {
			if len(s) > 60 {
				if c := strings.LastIndex(string(s), "、"); c >= 0 {
					head := string(s)[:c+len("、")]
					out = append(out, head)
					rest := string(s)[len(head):]
					sp.buf.Reset()
					sp.buf.WriteString(rest)
				}
			}
			return out
		}
		for i+1 < len(s) && strings.ContainsRune(sentenceEnds+sentenceClosers, s[i+1]) {
			i++
		}
		if i == len(s)-1 && s[i] != '\n' {
			return out // 閉じかっこが次の断片で来るかもしれないので待つ (最後は Flush で出る)
		}
		out = append(out, string(s[:i+1]))
		sp.buf.Reset()
		sp.buf.WriteString(string(s[i+1:]))
	}
}

func (sp *sentenceSplitter) Flush() string {
	s := sp.buf.String()
	sp.buf.Reset()
	return s
}

var urlRe = regexp.MustCompile(`https?://\S+`)
var markRe = regexp.MustCompile("[*#`_>|\\[\\]]")

// 読み上げに向かない URL や記号を取る
func cleanForSpeech(s string) string {
	return strings.TrimSpace(markRe.ReplaceAllString(urlRe.ReplaceAllString(s, ""), ""))
}
