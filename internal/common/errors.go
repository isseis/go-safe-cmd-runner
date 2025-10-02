// Package common provides shared utilities and error definitions used across multiple packages.
//
//nolint:revive // common is an appropriate name for shared utilities package
package common

import "errors"

// ErrInvalidFileOperation indicates that an invalid file operation type was specified.
var ErrInvalidFileOperation = errors.New("invalid file operation")
