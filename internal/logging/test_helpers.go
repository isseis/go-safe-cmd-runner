//go:build test

package logging

import (
	"log/slog"
)

// NewSecurityLoggerWithLogger creates a new security logger with a custom logger
func NewSecurityLoggerWithLogger(logger *slog.Logger) *SecurityLogger {
	return &SecurityLogger{
		logger: logger,
	}
}
