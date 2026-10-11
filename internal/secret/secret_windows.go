//go:build windows

// Package secret protects the saved RomM login with Windows' Data Protection
// API (DPAPI). Only your Windows account on this PC can unlock it; copying the
// settings file to another PC or user gets nothing usable.
package secret

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var entropy = []byte("slot-label-maker/romm-login/v1")

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func take(out *windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	b := make([]byte, out.Size)
	copy(b, unsafe.Slice(out.Data, out.Size))
	return b
}

// Available reports whether logins can be saved securely here.
func Available() bool { return true }

// Protect encrypts data for the current Windows user.
func Protect(data []byte) ([]byte, error) {
	var out windows.DataBlob
	err := windows.CryptProtectData(blob(data), nil, blob(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	return take(&out), nil
}

// Unprotect decrypts what Protect made.
func Unprotect(data []byte) ([]byte, error) {
	var out windows.DataBlob
	err := windows.CryptUnprotectData(blob(data), nil, blob(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	return take(&out), nil
}
