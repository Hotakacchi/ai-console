package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestPhoneServer(t *testing.T) {
	loadLocales()
	l := &Lumi{answers: make(chan string, 1)}
	port := freePort(t)
	if err := phone.start(l, port); err != nil {
		t.Fatal(err)
	}
	defer phone.stop()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	token := phoneToken(false)

	// 鍵がなければ入れない。鍵があればページが出る
	if r, _ := http.Get(base + "/"); r.StatusCode != 403 {
		t.Errorf("no token: %d", r.StatusCode)
	}
	if r, _ := http.Get(base + "/?t=wrong"); r.StatusCode != 403 {
		t.Errorf("wrong token: %d", r.StatusCode)
	}
	r, err := http.Get(base + "/?t=" + token)
	if err != nil || r.StatusCode != 200 || !strings.Contains(r.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("page: %v %v", err, r)
	}

	// スマホからは、コマンドや / コマンドは受け付けない
	for _, text := range []string{"/exit", "!dir"} {
		r, _ := http.Post(base+"/api/say?t="+token, "application/json", strings.NewReader(`{"text":"`+text+`"}`))
		if r.StatusCode != 403 {
			t.Errorf("%s accepted: %d", text, r.StatusCode)
		}
	}

	// 確認への答え: 「はい」には PC の画面に出た番号が要る。「いいえ」は番号なしで届く
	answer := func(body string) int {
		r, err := http.Post(base+"/api/answer?t="+token, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return r.StatusCode
	}
	got := func() string {
		select {
		case a := <-l.answers:
			return a
		case <-time.After(500 * time.Millisecond):
			return ""
		}
	}
	l.askCode = "1234"
	if s := answer(`{"answer":"y"}`); s != 403 || got() != "" {
		t.Errorf("yes without the number: %d", s)
	}
	if s := answer(`{"answer":"y","code":"0000"}`); s != 403 || got() != "" {
		t.Errorf("yes with a wrong number: %d", s)
	}
	if s := answer(`{"answer":"y","code":"1234"}`); s != 204 || got() != "y" {
		t.Errorf("yes with the number: %d", s)
	}
	if s := answer(`{"answer":"n"}`); s != 204 || got() != "n" {
		t.Errorf("no: %d", s)
	}
	// 3 回間違えたら、正しい番号でももう答えられない
	l.askCode, l.askTries = "5678", 0
	for i := 0; i < 3; i++ {
		answer(`{"answer":"y","code":"0000"}`)
	}
	if s := answer(`{"answer":"y","code":"5678"}`); s != 403 || got() != "" {
		t.Errorf("after 3 wrong numbers: %d", s)
	}
	if c := newAskCode(); len(c) != 4 || strings.Trim(c, "0123456789") != "" {
		t.Errorf("code %q", c)
	}

	// 画面への出来事がスマホに流れる
	ev, err := http.Get(base + "/api/events?t=" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer ev.Body.Close()
	sc := bufio.NewScanner(ev.Body)
	sc.Scan() // hello
	if !strings.Contains(sc.Text(), `"hello"`) {
		t.Fatalf("first event %q", sc.Text())
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		phone.publish("speak", map[string]any{"id": 1, "text": "こんにちは", "wav": "xxx"})
	}()
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "data:") {
			if !strings.Contains(line, "こんにちは") || strings.Contains(line, "xxx") {
				t.Errorf("event %q", line)
			}
			break
		}
	}
}
