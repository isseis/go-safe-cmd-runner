//go:build test
// +build test

package audit

import "log/slog"

// NewAuditLoggerWithCustom creates a new audit logger instance using a custom logger
// This is useful for testing or when a specific logger configuration is needed
func NewAuditLoggerWithCustom(l *slog.Logger) *Logger {
	return &Logger{logger: l}
}
