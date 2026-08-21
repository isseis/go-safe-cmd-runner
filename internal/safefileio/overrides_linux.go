//go:build linux && !test

package safefileio

// openat2Syscall issues one openat2 system call.
//
// This file and test_helpers_overrides_linux.go define openat2Syscall
// exclusively by build tag. The production build deliberately has no function
// value to swap: this call is the symlink-safe open that every security check
// in this package is anchored to, so nothing outside a test build may redirect
// it.
func openat2Syscall(dirfd int, pathname string, how *openHow) (int, error) {
	return rawOpenat2(dirfd, pathname, how)
}
