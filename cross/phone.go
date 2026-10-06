package main

// スマホから話しかける: 同じ Wi-Fi (家の中) のスマホのブラウザから、ルミと話せるようにする。
//   /phone        … 受け付けを始めて、開く URL と QR コードを出す (phone を on にする)
//   /phone off    … やめる
//   /phone reset  … 鍵を作り直す (前の URL では入れなくなる)
// 家の中 (と Tailscale などの VPN) からだけ、URL に入った鍵があるときだけ受け付ける。
// スマホからの発言は、話しかけたのと同じように AI に渡す (「!」のコマンドや / コマンドは受け付けない)。

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const defaultPhonePort = 47800

type phoneServer struct {
	mu      sync.Mutex
	srv     *http.Server
	port    int
	clients map[chan []byte]bool
}

var phone = &phoneServer{clients: map[chan []byte]bool{}}

func phoneTokenPath() string { return filepath.Join(dataDir(), "phone_token") }

// 鍵 (URL に入れる)。初めてなら作って保存する
func phoneToken(reset bool) string {
	if !reset {
		if b, err := os.ReadFile(phoneTokenPath()); err == nil && len(strings.TrimSpace(string(b))) >= 32 {
			return strings.TrimSpace(string(b))
		}
	}
	b := make([]byte, 16)
	rand.Read(b)
	t := hex.EncodeToString(b)
	os.MkdirAll(dataDir(), 0o755)
	os.WriteFile(phoneTokenPath(), []byte(t), 0o600)
	return t
}

// 起動時: 設定が on なら受け付けを始める
func (l *Lumi) applyPhone(announce bool) {
	if l.s.Get("phone", "off") != "on" {
		phone.stop()
		return
	}
	port := l.s.GetInt("phone_port", defaultPhonePort)
	if err := phone.start(l, port); err != nil {
		l.errorText(T("phone.failed", err.Error()))
		return
	}
	if announce {
		l.showPhoneURL()
	}
}

// /phone [off|reset]
func (l *Lumi) phoneCommand(arg string) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "off":
		phone.stop()
		if l.s.Err == nil {
			l.s.Set("phone", "off")
		}
		l.info(T("phone.off"))
		return
	case "reset":
		phoneToken(true)
		l.info(T("phone.reset"))
	}
	if l.s.Err == nil && l.s.Get("phone", "off") != "on" {
		l.s.Set("phone", "on")
	}
	l.applyPhone(true)
}

// 開く URL と QR コード
func (l *Lumi) showPhoneURL() {
	ips := lanAddresses()
	if len(ips) == 0 {
		l.errorText(T("phone.noNetwork"))
		return
	}
	token := phoneToken(false)
	var urls []string
	for _, ip := range ips {
		urls = append(urls, fmt.Sprintf("http://%s/?t=%s", net.JoinHostPort(ip, fmt.Sprint(phone.port)), token))
	}
	if png, err := qrcode.Encode(urls[0], qrcode.Medium, 256); err == nil {
		l.emit("image", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png))
	}
	var b strings.Builder
	b.WriteString(T("phone.on") + "\n")
	for _, u := range urls {
		b.WriteString("  " + u + "\n")
	}
	b.WriteString("\n" + T("phone.howto"))
	l.info(b.String())
}

// スマホから届く、家の中の IPv4 アドレス (仮想の LAN は後ろに)
func lanAddresses() []string {
	type cand struct {
		ip    string
		score int
	}
	var list []cand
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		virtual := strings.Contains(name, "vethernet") || strings.Contains(name, "wsl") || strings.Contains(name, "hyper-v") ||
			strings.Contains(name, "virtualbox") || strings.Contains(name, "vmware") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-")
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip := ipn.IP.To4()
			score := 0
			switch {
			case ip[0] == 192 && ip[1] == 168:
				score = 3
			case ip[0] == 10:
				score = 2
			case ip.IsPrivate():
				score = 1
			case sharedNet.Contains(ip): // Tailscale など
				score = 0
			default:
				continue
			}
			if virtual {
				score -= 10
			}
			list = append(list, cand{ip.String(), score})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })
	var out []string
	for _, c := range list {
		if c.score > -10 { // 仮想の LAN (WSL など) はスマホから届かないので出さない
			out = append(out, c.ip)
		}
	}
	return out
}

// ---- サーバー ----

func (p *phoneServer) start(l *Lumi, port int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.srv != nil && p.port == port {
		return nil
	}
	if p.srv != nil {
		p.srv.Close()
		p.srv = nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	static, _ := fs.Sub(assets, "assets")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, _ := fs.ReadFile(static, "phone.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	mux.HandleFunc("/face.js", func(w http.ResponseWriter, r *http.Request) {
		data, _ := fs.ReadFile(static, "face.js")
		w.Header().Set("Content-Type", "text/javascript")
		w.Write(data)
	})
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		data, _ := fs.ReadFile(static, "icon.png")
		w.Header().Set("Content-Type", "image/png")
		w.Write(data)
	})
	mux.HandleFunc("/api/events", p.events)
	mux.HandleFunc("/api/say", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Text string }
		if r.Method != "POST" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		text := strings.TrimSpace(body.Text)
		// コマンドの直接実行やアプリの設定は、PC の前でだけ
		if text == "" || strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!") {
			http.Error(w, T("phone.onlyChat"), 403)
			return
		}
		if l.isBusy() {
			http.Error(w, T("phone.busy"), 409)
			return
		}
		l.emit("echo", text+"  📱")
		l.submit(text)
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/answer", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Answer string }
		if r.Method != "POST" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		a := strings.ToLower(strings.TrimSpace(body.Answer))
		if a != "y" && a != "a" {
			a = "n"
		}
		select {
		case l.answers <- a:
			l.emit("askAnswered", a)
		default:
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			l.interrupt()
		}
		w.WriteHeader(204)
	})
	p.srv = &http.Server{Handler: phoneGuard(mux), ReadHeaderTimeout: 10 * time.Second}
	p.port = port
	go p.srv.Serve(ln)
	return nil
}

func (p *phoneServer) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.srv != nil {
		p.srv.Close()
		p.srv = nil
	}
	for c := range p.clients {
		close(c)
		delete(p.clients, c)
	}
}

func (p *phoneServer) running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.srv != nil
}

// 家の中 (と VPN) から、鍵つきのときだけ通す
func phoneGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if ip == nil || !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || sharedNet.Contains(ip)) {
			http.Error(w, "forbidden", 403)
			return
		}
		token := r.URL.Query().Get("t")
		if token == "" {
			token = r.Header.Get("X-Lumi-Token")
		}
		want := phoneToken(false)
		if r.URL.Path != "/face.js" && r.URL.Path != "/icon.png" &&
			subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
			http.Error(w, T("phone.badToken"), 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// 画面への出来事を、つながっているスマホに流す (Server-Sent Events)
func (p *phoneServer) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no stream", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	c := make(chan []byte, 256)
	p.mu.Lock()
	p.clients[c] = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		if p.clients[c] {
			delete(p.clients, c)
			close(c)
		}
		p.mu.Unlock()
	}()
	fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "hello", "lang": currentLang(), "msgs": phoneMessages()}))
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case msg, ok := <-c:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// 画面に送ったイベントのうち、スマホでも見せるもの
func (p *phoneServer) publish(name string, data any) {
	var ev map[string]any
	switch name {
	case "write", "echo", "busy", "ask", "askAnswered", "clear", "thinking", "running", "shellMode":
		ev = map[string]any{"type": name, "data": data}
	case "speak", "speakAudio":
		// 声の WAV は送らず、文だけ (スマホ側で読み上げる)
		if m, ok := data.(map[string]any); ok {
			ev = map[string]any{"type": "speak", "data": m["text"]}
		}
	case "face", "flash":
		ev = map[string]any{"type": name, "data": data}
	default:
		return
	}
	msg := mustJSON(ev)
	p.mu.Lock()
	defer p.mu.Unlock()
	for c := range p.clients {
		select {
		case c <- msg:
		default: // 読むのが遅いスマホは飛ばす
		}
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// スマホの画面の文言
func phoneMessages() map[string]string {
	m := map[string]string{}
	for _, k := range []string{"phone.page.placeholder", "phone.page.send", "phone.page.stop", "phone.page.sound", "phone.page.yes", "phone.page.admin", "phone.page.no", "phone.page.hint", "phone.page.disconnected", "phone.busy", "phone.onlyChat"} {
		m[k] = T(k)
	}
	return m
}
