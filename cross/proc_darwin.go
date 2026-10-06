package main

import "os/exec"

// macOS には親の終了で子を止める仕組みがないので、終了時に Stop で止める
func hideWindow(cmd *exec.Cmd)     {}
func killWithParent(cmd *exec.Cmd) {}
func afterStart(cmd *exec.Cmd)     {}

func windowsName() string { return "Windows" }
