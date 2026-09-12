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

// typeName returns the registered type name of the token, or an empty string
// for the zero token. It is test-only so production code has no accessor that
// turns a token back into the type-name string.
func (n Notification) typeName() string {
	if n.definition == nil {
		return ""
	}
	return n.definition.messageType
}

// NotificationMessageType returns the registered type name of the token. Test
// code outside the package uses it to name the expected message_type without
// writing the type-name literal, so a test asserts the token a firing point
// actually chose rather than a string it copied.
func NotificationMessageType(notification Notification) string {
	return notification.typeName()
}
