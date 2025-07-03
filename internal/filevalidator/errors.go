// Package filevalidator provides functionality for file validation and verification.
package filevalidator

import "errors"

var (
	// ErrMismatch indicates that the file content does not match the recorded hash during verification.
	ErrMismatch = errors.New("file content does not match the recorded hash")

	// ErrHashFileNotFound indicates that the hash file for verification was not found.
	ErrHashFileNotFound = errors.New("hash file not found")

	// ErrNilAlgorithm indicates that the algorithm is nil during Validator initialization.
	ErrNilAlgorithm = errors.New("algorithm cannot be nil")

	// ErrHashDirNotExist indicates that the hash directory does not exist.
	ErrHashDirNotExist = errors.New("hash directory does not exist")

	// ErrHashPathNotDir indicates that the hash path is not a directory.
	ErrHashPathNotDir = errors.New("hash path is not a directory")

	// ErrInvalidHashFileFormat indicates that the hash file has an invalid format.
	ErrInvalidHashFileFormat = errors.New("invalid hash file format")

	// ErrHashCollision indicates a hash collision was detected.
	ErrHashCollision = errors.New("hash collision detected")

	// ErrInvalidFilePathFormat indicates an invalid file path format was provided.
	ErrInvalidFilePathFormat = errors.New("invalid file path format")

	// ErrSuspiciousFilePath indicates a potentially malicious file path was detected.
	ErrSuspiciousFilePath = errors.New("suspicious file path detected")
)
