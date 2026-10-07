package main

import (
	"strings"

	"golang.org/x/sys/windows"
)

// つながっているドライブ (E: など)。CD ドライブのように中身がないと数えられないものも含む
func listDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<i) != 0 {
			out = append(out, string(rune('A'+i))+":")
		}
	}
	return out
}

// ドライブの名前 (ボリュームラベル。なければ種類)
func driveName(letter string) string {
	root, _ := windows.UTF16PtrFromString(letter + `\`)
	buf := make([]uint16, windows.MAX_PATH+1)
	if windows.GetVolumeInformation(root, &buf[0], uint32(len(buf)), nil, nil, nil, nil, 0) == nil {
		if name := strings.TrimSpace(windows.UTF16ToString(buf)); name != "" {
			return name
		}
	}
	switch windows.GetDriveType(root) {
	case windows.DRIVE_REMOVABLE:
		return T("drive.removable")
	case windows.DRIVE_CDROM:
		return T("drive.cd")
	case windows.DRIVE_REMOTE:
		return T("drive.network")
	}
	return T("drive.disk")
}
