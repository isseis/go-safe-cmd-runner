// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
package safefileio

import "errors"

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
	ErrInvalidFileOperation = errors.New("invalid file operation")
)
