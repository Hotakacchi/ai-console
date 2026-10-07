package main

import (
	"fmt"
	"io"
	"os"
)

// Windows の Discord は名前付きパイプ \\.\pipe\discord-ipc-N で待っている
func dialDiscord(i int) (io.ReadWriteCloser, error) {
	return os.OpenFile(fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i), os.O_RDWR, 0)
}
