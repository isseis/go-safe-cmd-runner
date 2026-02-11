// Package common provides shared utilities and error definitions used across multiple packages.
//
//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import "errors"

// ErrInvalidFileOperation indicates that an invalid file operation type was specified.
var ErrInvalidFileOperation = errors.New("invalid file operation")
