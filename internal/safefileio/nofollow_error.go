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
	return isNoFollowErrno(e.Err)
}

// isNoFollowErrno is isNoFollowError for a caller holding the bare errno, as a
// direct syscall wrapper does rather than os.OpenFile.
func isNoFollowErrno(err error) bool {
	return errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EMLINK)
}
