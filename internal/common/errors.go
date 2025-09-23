// Package common provides shared utilities and error definitions used across multiple packages.
package common

import "errors"

// ErrInvalidFileOperation indicates that an invalid file operation type was specified.
var ErrInvalidFileOperation = errors.New("invalid file operation")
