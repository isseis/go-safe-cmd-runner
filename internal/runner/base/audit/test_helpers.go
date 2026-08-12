//go:build test

package audit

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
)

// NewAuditLoggerWithCustom creates a new audit logger instance using a custom logger
// This is useful for testing or when a specific logger configuration is needed
func NewAuditLoggerWithCustom(l *slog.Logger) *Logger {
	return &Logger{logger: l, redactor: redaction.DefaultConfig()}
}

// NewAuditLoggerWithCustomRedaction creates a new audit logger instance using a
// custom logger and a custom redaction Config, so a test can verify that a
// Logger honors an injected Config (e.g. one built with WithWebhookHost)
// rather than always falling back to DefaultConfig().
func NewAuditLoggerWithCustomRedaction(l *slog.Logger, redactionConfig *redaction.Config) *Logger {
	return &Logger{logger: l, redactor: redactionConfig}
}
