//go:build windows

package sdcard

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Space returns the card's total and free bytes.
func Space(root string) (total, free uint64, err error) {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, 0, err
	}
	var avail, tot, totFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &tot, &totFree); err != nil {
		return 0, 0, err
	}
	return tot, avail, nil
}

// Drive is a drive letter that might be the SD card.
type Drive struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Removable bool   `json:"removable"`
	IsSlot    bool   `json:"isSlot"`
	System    bool   `json:"system"` // the drive Windows runs from
}

func volumeRoot(path string) string {
	v := filepath.VolumeName(path)
	if v == "" {
		return ""
	}
	return v + `\`
}

// IsRemovable reports whether path is on a removable drive, like an SD card.
func IsRemovable(path string) bool {
	root := volumeRoot(path)
	rp, err := windows.UTF16PtrFromString(root)
	if root == "" || err != nil {
		return false
	}
	return windows.GetDriveType(rp) == windows.DRIVE_REMOVABLE
}

// IsSystemDrive reports whether path is on the drive Windows runs from.
func IsSystemDrive(path string) bool {
	sys := os.Getenv("SystemDrive")
	if sys == "" {
		sys = "C:"
	}
	return strings.EqualFold(filepath.VolumeName(path), sys)
}

// Drives lists drive letters, Slot cards first.
func Drives() []Drive {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var slot, other []Drive
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		rp, _ := windows.UTF16PtrFromString(root)
		kind := windows.GetDriveType(rp)
		if kind != windows.DRIVE_REMOVABLE && kind != windows.DRIVE_FIXED {
			continue
		}
		name := make([]uint16, windows.MAX_PATH+1)
		// Errors here just mean "no disk in the reader": skip quietly.
		if windows.GetVolumeInformation(rp, &name[0], uint32(len(name)), nil, nil, nil, nil, 0) != nil {
			continue
		}
		d := Drive{Path: root, Name: windows.UTF16ToString(name), Removable: kind == windows.DRIVE_REMOVABLE, IsSlot: IsSlotCard(root), System: IsSystemDrive(root)}
		if d.IsSlot {
			slot = append(slot, d)
		} else if d.Removable {
			other = append(other, d)
		}
	}
	return append(slot, other...)
}
