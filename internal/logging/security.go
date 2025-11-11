// Package logging provides security-relevant logging functionality
package logging

import (
	"log/slog"
	"time"
)

// SecurityLogger logs security-relevant timeout events
type SecurityLogger struct {
	logger *slog.Logger
}

// NewSecurityLogger creates a new security logger
func NewSecurityLogger() *SecurityLogger {
	return &SecurityLogger{
		logger: slog.Default(),
	}
}

// NewSecurityLoggerWithLogger creates a new security logger with a custom logger
func NewSecurityLoggerWithLogger(logger *slog.Logger) *SecurityLogger {
	return &SecurityLogger{
		logger: logger,
	}
}

// LogUnlimitedExecution logs when a command starts execution with unlimited timeout
func (s *SecurityLogger) LogUnlimitedExecution(cmdName string, user string) {
	s.logger.Warn("Command starting with unlimited timeout",
		"command", cmdName,
		"user", user,
		"timeout", "unlimited",
		"security_event", "unlimited_execution_start")
}

// LogLongRunningProcess logs when a process has been running for an extended period
func (s *SecurityLogger) LogLongRunningProcess(cmdName string, duration time.Duration, pid int) {
	s.logger.Warn("Long-running process detected",
		"command", cmdName,
		"pid", pid,
		"duration_minutes", int(duration.Minutes()),
		"security_event", "long_running_process")
}

// LogTimeoutExceeded logs when a command exceeds its timeout
func (s *SecurityLogger) LogTimeoutExceeded(cmdName string, timeoutSeconds int32, pid int) {
	s.logger.Error("Command exceeded timeout",
		"command", cmdName,
		"pid", pid,
		"timeout_seconds", timeoutSeconds,
		"security_event", "timeout_exceeded")
}

// LogTimeoutConfiguration logs the effective timeout configuration for a command
func (s *SecurityLogger) LogTimeoutConfiguration(cmdName string, timeoutSeconds int32, source string) {
	if timeoutSeconds == 0 {
		s.logger.Info("Command configured with unlimited timeout",
			"command", cmdName,
			"timeout", "unlimited",
			"source", source)
	} else {
		s.logger.Debug("Command timeout configured",
			"command", cmdName,
			"timeout_seconds", timeoutSeconds,
			"source", source)
	}
}
