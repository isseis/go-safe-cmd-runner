// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
package safefileio

import (
	"errors"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

var (
	// ErrInvalidFilePath indicates that the specified file path is invalid.
	ErrInvalidFilePath = errors.New("invalid file path")

	// ErrIsSymlink indicates that the specified path is a symbolic link, which is not allowed.
	ErrIsSymlink = errors.New("path is a symbolic link")

	// ErrFileTooLarge indicates that the file is too large.
	ErrFileTooLarge = errors.New("file too large")

	// ErrFileExists indicates that the file already exists.
	ErrFileExists = errors.New("file exists")

	// ErrInvalidFilePermissions indicates that the file has inappropriate permissions.
	ErrInvalidFilePermissions = errors.New("invalid file permissions")

	// ErrInvalidFileOperation indicates that an invalid file operation type was specified.
	// This is an alias to the common error definition to maintain API compatibility.
	ErrInvalidFileOperation = common.ErrInvalidFileOperation

	// ErrTempLinkNameExhausted indicates that a unique temporary hard-link name
	// could not be allocated after repeated EEXIST collisions.
	ErrTempLinkNameExhausted = errors.New("failed to allocate a unique temporary link name")

	// ErrUnsupportedFileHandle indicates that the provided File implementation
	// does not support the operation being requested (e.g. it is not backed by
	// an *os.File and therefore cannot be used for fd-anchored operations).
	// This is distinct from ErrInvalidFilePath, which signals a problem with
	// the path itself rather than with the type of the file handle.
	ErrUnsupportedFileHandle = errors.New("unsupported file handle type")

	// ErrSourceIdentityMismatch indicates that the directory entry at a source
	// path no longer refers to the inode that was previously verified (e.g. an
	// fd was opened and validated, then the path was replaced before a later
	// operation). This is distinct from ErrInvalidFilePath, which signals a
	// problem with the path's syntax or structure rather than a runtime
	// TOCTOU-detected identity change.
	ErrSourceIdentityMismatch = errors.New("source path no longer refers to the verified inode")
)
