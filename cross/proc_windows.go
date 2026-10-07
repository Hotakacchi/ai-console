package main

import (
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 子プロセスのコンソール窓を出さない
func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}

// コマンドラインをそのまま渡す (cmd.exe の引用符の決まりは、ふつうの書き方と違うので)
func setCmdLine(cmd *exec.Cmd, line string) { cmd.SysProcAttr.CmdLine = line }

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

func newGroup(cmd *exec.Cmd) {}

// cmd と、そこから起動されたプロセス (ping など) をまとめて止める
func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	hideWindow(kill)
	if kill.Run() != nil {
		cmd.Process.Kill()
	}
}

// ルミが終わっても動き続けるように (更新のときに使う)
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
}

// 少し待ってから exe を起動する (ルミが終わってから。cmd.exe の timeout と start を使う)
func relaunchLater(exe string) error {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
		CmdLine:       `cmd.exe /d /c "timeout /t 2 /nobreak >nul & start "" "` + exe + `""`,
	}
	return cmd.Start()
}
