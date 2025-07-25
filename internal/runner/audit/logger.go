// Package audit provides structured audit logging for privileged command execution.
package audit

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Logger provides structured audit logging functionality
type Logger struct {
	logger *slog.Logger
}

// NewAuditLogger creates a new audit logger instance
func NewAuditLogger(logger *slog.Logger) *Logger {
	return &Logger{logger: logger}
}

// PrivilegeMetrics contains metrics about privilege usage during execution
type PrivilegeMetrics struct {
	ElevationCount int
	TotalDuration  time.Duration
}

// ExecutionResult represents the result of command execution for audit logging
type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// LogPrivilegedExecution logs the execution of a privileged command with full audit trail
func (a *Logger) LogPrivilegedExecution(
	_ context.Context,
	cmd runnertypes.Command,
	result *ExecutionResult,
	duration time.Duration,
	privilegeMetrics PrivilegeMetrics,
) {
	baseAttrs := []slog.Attr{
		slog.String("audit_type", "privileged_execution"),
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String("command_name", cmd.Name),
		slog.String("command_path", cmd.Cmd),
		slog.String("command_args", strings.Join(cmd.Args, " ")),
		slog.Int("exit_code", result.ExitCode),
		slog.Int64("execution_duration_ms", duration.Milliseconds()),
		slog.Int("user_id", os.Getuid()),
		slog.Int("effective_user_id", os.Geteuid()),
		slog.Int("process_id", os.Getpid()),
		slog.Int("elevation_count", privilegeMetrics.ElevationCount),
		slog.Int64("total_privilege_duration_ms", privilegeMetrics.TotalDuration.Milliseconds()),
	}

	// Add working directory if specified
	if cmd.Dir != "" {
		baseAttrs = append(baseAttrs, slog.String("working_directory", cmd.Dir))
	}

	if result.ExitCode == 0 {
		a.logger.LogAttrs(context.Background(), slog.LevelInfo, "Privileged command executed successfully", baseAttrs...)
	} else {
		// Create new slice to avoid modifying baseAttrs
		const additionalErrorAttrs = 2 // stdout and stderr
		errorAttrs := make([]slog.Attr, len(baseAttrs), len(baseAttrs)+additionalErrorAttrs)
		copy(errorAttrs, baseAttrs)
		errorAttrs = append(errorAttrs,
			slog.String("stdout", result.Stdout),
			slog.String("stderr", result.Stderr))
		a.logger.LogAttrs(context.Background(), slog.LevelError, "Privileged command failed", errorAttrs...)
	}
}

// LogPrivilegeEscalation logs privilege escalation events
func (a *Logger) LogPrivilegeEscalation(
	_ context.Context,
	operation string,
	commandName string,
	originalUID int,
	targetUID int,
	success bool,
	duration time.Duration,
) {
	attrs := []slog.Attr{
		slog.String("audit_type", "privilege_escalation"),
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String("operation", operation),
		slog.String("command_name", commandName),
		slog.Int("original_uid", originalUID),
		slog.Int("target_uid", targetUID),
		slog.Bool("success", success),
		slog.Int64("duration_ms", duration.Milliseconds()),
		slog.Int("process_id", os.Getpid()),
	}

	if success {
		a.logger.LogAttrs(context.Background(), slog.LevelInfo, "Privilege escalation successful", attrs...)
	} else {
		a.logger.LogAttrs(context.Background(), slog.LevelWarn, "Privilege escalation failed", attrs...)
	}
}

// LogSecurityEvent logs security-related events and potential threats
func (a *Logger) LogSecurityEvent(
	_ context.Context,
	eventType string,
	severity string,
	message string,
	details map[string]interface{},
) {
	attrs := []slog.Attr{
		slog.String("audit_type", "security_event"),
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String("event_type", eventType),
		slog.String("severity", severity),
		slog.String("message", message),
		slog.Int("user_id", os.Getuid()),
		slog.Int("effective_user_id", os.Geteuid()),
		slog.Int("process_id", os.Getpid()),
	}

	// Add custom details
	for key, value := range details {
		attrs = append(attrs, slog.Any(key, value))
	}

	switch severity {
	case "critical", "high":
		a.logger.LogAttrs(context.Background(), slog.LevelError, "Security event", attrs...)
	case "medium":
		a.logger.LogAttrs(context.Background(), slog.LevelWarn, "Security event", attrs...)
	default:
		a.logger.LogAttrs(context.Background(), slog.LevelInfo, "Security event", attrs...)
	}
}
