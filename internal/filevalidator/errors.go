package filevalidator

import "errors"

var (
	// ErrMismatch indicates that the file content does not match the recorded hash during verification.
	ErrMismatch = errors.New("file content does not match the recorded hash")

	// ErrHashFileNotFound indicates that the hash file for verification was not found.
	ErrHashFileNotFound = errors.New("hash file not found")

	// ErrInvalidFilePath indicates that the specified file path is invalid.
	ErrInvalidFilePath = errors.New("invalid file path")

	// ErrIsSymlink indicates that the specified path is a symbolic link, which is not allowed.
	ErrIsSymlink = errors.New("path is a symbolic link")

	// ErrNilAlgorithm indicates that the algorithm is nil during Validator initialization.
	ErrNilAlgorithm = errors.New("algorithm cannot be nil")
)
