//go:build linux

// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// This file contains Linux-specific implementation using openat2 system call
// for atomic symlink-safe file operations.
package safefileio

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// openat2 constants for RESOLVE flags
const (
	// ResolveNoSymlinks disallows resolution of symbolic links
	ResolveNoSymlinks = 0x04
	// AtFdcwd represents the current working directory
	AtFdcwd = -0x64
	// SysOpenat2 is the system call number for openat2 on Linux
	SysOpenat2 = 437
)

// openHow struct for openat2 system call
type openHow struct {
	flags   uint64
	mode    uint64
	resolve uint64
}

const testFilePerm = 0o600 // Read/write for owner only

// isOpenat2Available checks if openat2 system call is available and working
func isOpenat2Available() bool {
	// Create a temporary directory for testing
	testDir, err := os.MkdirTemp("", "openat2test")
	if err != nil {
		return false
	}
	defer func() {
		if err := os.RemoveAll(testDir); err != nil {
			slog.Warn("failed to remove test directory", slog.Any("error", err), slog.String("path", testDir))
		}
	}()

	testFile := filepath.Join(testDir, "testfile")
	how := openHow{
		flags:   uint64(os.O_CREATE | os.O_RDWR | os.O_EXCL),
		mode:    testFilePerm, // #nosec G302 - file permissions are appropriate for test file
		resolve: ResolveNoSymlinks,
	}

	// Test openat2 with actual file operations
	fd, err := openat2(AtFdcwd, testFile, &how)
	// Clean up the test file
	if fd >= 0 {
		_ = syscall.Close(fd)
	}

	return err == nil
}

// openat2 wraps the openat2 system call
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
	pathBytes, err := syscall.BytePtrFromString(pathname)
	if err != nil {
		return -1, err
	}

	fd, _, errno := syscall.Syscall6(
		SysOpenat2,
		uintptr(dirfd),
		// #nosec G103 - uintptr conversion is required for syscall interface
		uintptr(unsafe.Pointer(pathBytes)),
		// #nosec G103 - uintptr conversion is required for syscall interface
		uintptr(unsafe.Pointer(how)),
		unsafe.Sizeof(*how),
		0, 0,
	)

	if errno != 0 {
		return -1, errno
	}

	return int(fd), nil
}

// safeOpenFileInternal implements Linux-specific file opening with openat2 support.
// It attempts to use the openat2 system call with RESOLVE_NO_SYMLINKS for atomic
// symlink-safe operations. When openat2 is unavailable or disabled, it falls back
// to safeOpenFileFallback which performs two-phase verification.
func (fs *osFS) safeOpenFileInternal(absPath string, flag int, perm os.FileMode) (*os.File, error) {
	if !fs.openat2Available {
		// Fall back to the portable method when openat2 is not available
		return safeOpenFileFallback(absPath, flag, perm)
	}

	// Use openat2 with RESOLVE_NO_SYMLINKS for atomic operation
	how := openHow{
		// #nosec G115 - flag conversion is intentional and safe within valid flag range
		flags:   uint64(flag),
		mode:    uint64(perm),
		resolve: ResolveNoSymlinks,
	}

	fd, err := openat2(AtFdcwd, absPath, &how)
	if err != nil {
		// Check for specific errors
		if errno, ok := err.(syscall.Errno); ok {
			switch errno {
			case syscall.ELOOP:
				return nil, ErrIsSymlink
			case syscall.EEXIST:
				return nil, ErrFileExists
			case syscall.ENOENT:
				return nil, os.ErrNotExist // Return standard not exist error
			}
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return os.NewFile(uintptr(fd), absPath), nil
}
