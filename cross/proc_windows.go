package main

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 子プロセスのコンソール窓を出さない
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func killWithParent(cmd *exec.Cmd) {}

// ルミ自体が管理者として (昇格して) 動いているか
func isElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

var job windows.Handle

// Job オブジェクトに入れておき、ルミが落ちても llama-server が残らないようにする
func afterStart(cmd *exec.Cmd) {
	if job == 0 {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
		job = h
	}
	if p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); err == nil {
		windows.AssignProcessToJobObject(job, p)
		windows.CloseHandle(p)
	}
}
