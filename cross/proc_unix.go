//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// 子プロセスをまとめて止められるよう、新しいプロセスグループで動かす
func newGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// cmd と、そこから起動されたプロセスをまとめて止める
func killTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// ルミが終わっても動き続けるように (更新のときに使う)
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
