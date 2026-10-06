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

	// 確認への答えが届く
	http.Post(base+"/api/answer?t="+token, "application/json", strings.NewReader(`{"answer":"y"}`))
	select {
	case a := <-l.answers:
		if a != "y" {
			t.Errorf("answer %q", a)
		}
	case <-time.After(2 * time.Second):
		t.Error("no answer")
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
