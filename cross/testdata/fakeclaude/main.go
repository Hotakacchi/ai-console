// テスト用の偽の claude: stdin の発言を読み、stream-json で「受け取りました: <文>」と返す。
// 引数は FAKE_CLAUDE_ARGS のファイルに書く。FAKE_CLAUDE_MODE=login ならログインしていないときの出力、slow なら待ち続ける
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if f := os.Getenv("FAKE_CLAUDE_ARGS"); f != "" {
		b, _ := json.Marshal(os.Args[1:])
		os.WriteFile(f, b, 0o644)
	}
	var in struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	json.Unmarshal([]byte(line), &in)
	text, images := "", 0
	for _, c := range in.Message.Content {
		if c.Type == "text" {
			text = c.Text
		} else if c.Type == "image" {
			images++
		}
	}
	fmt.Println(`{"type":"system","subtype":"init","tools":[]}`)
	switch os.Getenv("FAKE_CLAUDE_MODE") {
	case "login":
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Not logged in · Please run /login"}]},"error":"authentication_failed"}`)
		fmt.Println(`{"type":"result","is_error":true,"result":"Not logged in · Please run /login"}`)
		return
	case "slow":
		time.Sleep(30 * time.Second)
	}
	last := text[strings.LastIndex(text, "\n")+1:]
	for _, piece := range []string{"受け取りました: ", last, fmt.Sprintf(" (画像 %d)", images)} {
		b, _ := json.Marshal(map[string]any{"type": "stream_event", "event": map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": piece}}})
		fmt.Println(string(b))
	}
	fmt.Println(`{"type":"result","is_error":false,"result":"ok"}`)
}
