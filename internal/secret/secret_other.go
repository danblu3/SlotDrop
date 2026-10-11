//go:build !windows

// Package secret protects the saved RomM login. Only Windows is supported;
// elsewhere nothing is ever saved.
package secret

import "errors"

func Available() bool { return false }

func Protect([]byte) ([]byte, error) {
	return nil, errors.New("saving logins is only supported on Windows")
}

func Unprotect([]byte) ([]byte, error) {
	return nil, errors.New("saving logins is only supported on Windows")
}
