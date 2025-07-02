//go:build netbsd

package filevalidator

import (
	"os"
	"syscall"
)

// isNoFollowError checks if the error indicates we tried to open a symlink
func isNoFollowError(err error) bool {
	e, ok := err.(*os.PathError)
	if !ok {
		return false
	}
	return e.Err == syscall.EFTYPE
}
