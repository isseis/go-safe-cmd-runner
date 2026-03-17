//go:build !netbsd

package safefileio

import (
	"errors"
	"os"
	"syscall"
)

// isNoFollowError checks if the error indicates we tried to open a symlink
func isNoFollowError(err error) bool {
	e, ok := errors.AsType[*os.PathError](err)
	if !ok {
		return false
	}
	return errors.Is(e.Err, syscall.ELOOP) || errors.Is(e.Err, syscall.EMLINK)
}
