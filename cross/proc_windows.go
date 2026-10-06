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

// "Windows 11" / "Windows 10" など (11 もバージョンは 10.0 のままなので、ビルド番号 22000 以上で見分ける)
func windowsName() string {
	v := windows.RtlGetVersion()
	switch {
	case v.MajorVersion == 10 && v.BuildNumber >= 22000:
		return "Windows 11"
	case v.MajorVersion == 10:
		return "Windows 10"
	}
	return "Windows"
}

// この PC のドライブ (C:\ D:\ …)
func drives() []string {
	var list []string
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return []string{`C:\`}
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		// CD などの入れ替えられるドライブやネットワークドライブも含める (中身がないものは除く)
		if t := windows.GetDriveType(windows.StringToUTF16Ptr(root)); t == windows.DRIVE_NO_ROOT_DIR || t == windows.DRIVE_UNKNOWN {
			continue
		}
		list = append(list, root)
	}
	return list
}
