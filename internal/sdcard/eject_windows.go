//go:build windows

package sdcard

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	fsctlLockVolume          = 0x00090018
	fsctlDismountVolume      = 0x00090020
	ioctlStorageMediaRemoval = 0x002D4804
	ioctlStorageEjectMedia   = 0x002D4808
)

// CanEject reports whether root is a whole drive (like H:\) that can be ejected.
func CanEject(root string) bool {
	return len(filepath.VolumeName(root)) == 2 && strings.TrimRight(root[2:], `\`) == ""
}

// Eject flushes and dismounts the card, then asks the reader to eject it.
// Once the volume is dismounted it's safe to pull out, even if the reader
// can't physically eject.
func Eject(root string) error {
	if !CanEject(root) {
		return errors.New("only a whole drive can be ejected")
	}
	path, _ := windows.UTF16PtrFromString(`\\.\` + filepath.VolumeName(root))
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return errors.New("Windows wouldn't let the app eject the card. Use Safely Remove in the taskbar instead.")
	}
	defer windows.CloseHandle(h)
	var n uint32
	locked := false
	for i := 0; i < 12; i++ { // Explorer or antivirus may let go after a moment
		if windows.DeviceIoControl(h, fsctlLockVolume, nil, 0, nil, 0, &n, nil) == nil {
			locked = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !locked {
		return errors.New("Something is still using the card. Close any windows or programs showing it and try again.")
	}
	if err := windows.DeviceIoControl(h, fsctlDismountVolume, nil, 0, nil, 0, &n, nil); err != nil {
		return errors.New("The card couldn't be dismounted. Close anything using it and try again.")
	}
	prevent := byte(0)
	_ = windows.DeviceIoControl(h, ioctlStorageMediaRemoval, &prevent, 1, nil, 0, &n, nil)
	// Some built-in readers can't eject; the card is already safe to remove.
	_ = windows.DeviceIoControl(h, ioctlStorageEjectMedia, nil, 0, nil, 0, &n, nil)
	return nil
}
