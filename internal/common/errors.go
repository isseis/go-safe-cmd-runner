// Package common provides shared utilities and error definitions used across multiple packages.
package common

import "errors"

// ErrInvalidFileOperation indicates that an invalid file operation type was specified.
var ErrInvalidFileOperation = errors.New("invalid file operation")

// ErrInvalidNotificationContext indicates that a notification context
// attribute does not follow the fixed record encoding defined by
// NotificationContext.LogValue.
var ErrInvalidNotificationContext = errors.New("invalid notification context")
