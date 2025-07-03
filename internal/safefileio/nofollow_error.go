//go:build !netbsd

package safefileio

import (
	"errors"
	"os"
	"syscall"
)

// isNoFollowError checks if the error indicates we tried to open a symlink
func isNoFollowError(err error) bool {
	var e *os.PathError
	if !errors.As(err, &e) {
		return false
	}
	return errors.Is(e.Err, syscall.ELOOP) || errors.Is(e.Err, syscall.EMLINK)
}
