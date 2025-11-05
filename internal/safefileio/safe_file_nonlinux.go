//go:build !linux

// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// This file contains non-Linux platform implementation that uses the portable
// two-phase verification method (safeOpenFileFallback).
package safefileio

import (
	"os"
)

// isOpenat2Available always returns false on non-Linux platforms
func isOpenat2Available() bool {
	return false
}

// safeOpenFileInternal implements file opening for non-Linux platforms.
// It uses the portable safeOpenFileFallback method which performs two-phase
// verification to detect symlink attacks and TOCTOU race conditions.
func (fs *osFS) safeOpenFileInternal(absPath string, flag int, perm os.FileMode) (*os.File, error) {
	return safeOpenFileFallback(absPath, flag, perm)
}
