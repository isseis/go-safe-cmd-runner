// Package audit provides structured audit logging for privileged command execution.
package audit

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Logger provides structured audit logging functionality
type Logger struct {
	logger *slog.Logger
}

// NewAuditLogger creates a new audit logger instance using the global logger
// This integrates with the new logging framework for unified logging
func NewAuditLogger() *Logger {
	return &Logger{logger: slog.Default()}
}

// NewAuditLoggerWithCustom creates a new audit logger instance with a custom logger
// This method is preserved for backwards compatibility and testing
func NewAuditLoggerWithCustom(logger *slog.Logger) *Logger {
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

// LogUserGroupExecution logs the execution of a command with user/group privilege changes
func (l *Logger) LogUserGroupExecution(
	ctx context.Context,
	cmd *runnertypes.RuntimeCommand,
	result *ExecutionResult,
	duration time.Duration,
	privilegeMetrics PrivilegeMetrics,
) {
	baseAttrs := []slog.Attr{
		slog.String("audit_type", "user_group_execution"),
		slog.Bool("audit", true), // Mark as audit event for new logging framework
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String("command_name", cmd.Name()),
		slog.String("command_path", cmd.Cmd()),
		slog.String("command_args", strings.Join(cmd.Args(), " ")),
		slog.String("expanded_command_path", cmd.ExpandedCmd),
		slog.String("expanded_command_args", strings.Join(cmd.ExpandedArgs, " ")),
		slog.Int("exit_code", result.ExitCode),
		slog.Int64("execution_duration_ms", duration.Milliseconds()),
		slog.Int("user_id", os.Getuid()),
		slog.Int("effective_user_id", os.Geteuid()),
		slog.Int("process_id", os.Getpid()),
		slog.Int("elevation_count", privilegeMetrics.ElevationCount),
		slog.Int64("total_privilege_duration_ms", privilegeMetrics.TotalDuration.Milliseconds()),
	}

	// Add user/group information
	if cmd.RunAsUser() != "" {
		baseAttrs = append(baseAttrs, slog.String("run_as_user", cmd.RunAsUser()))
	}
	if cmd.RunAsGroup() != "" {
		baseAttrs = append(baseAttrs, slog.String("run_as_group", cmd.RunAsGroup()))
	}

	// Add working directory if specified
	if cmd.EffectiveWorkDir != "" {
		baseAttrs = append(baseAttrs, slog.String("working_directory", cmd.EffectiveWorkDir))
	}

	if result.ExitCode == 0 {
		l.logger.LogAttrs(ctx, slog.LevelInfo, "User/group command executed successfully", baseAttrs...)
	} else {
		// Create new slice to avoid modifying baseAttrs
		additionalAttrs := []slog.Attr{
			slog.String("stdout", result.Stdout),
			slog.String("stderr", result.Stderr),
			slog.Bool("slack_notify", true), // Notify Slack for failed user/group commands
			slog.String("message_type", "user_group_command_failure"),
		}
		errorAttrs := make([]slog.Attr, len(baseAttrs), len(baseAttrs)+len(additionalAttrs))
		copy(errorAttrs, baseAttrs)
		errorAttrs = append(errorAttrs, additionalAttrs...)
		l.logger.LogAttrs(ctx, slog.LevelError, "User/group command failed", errorAttrs...)
	}
}

// LogPrivilegeEscalation logs privilege escalation events
func (l *Logger) LogPrivilegeEscalation(
	ctx context.Context,
	operation string,
	commandName string,
	originalUID int,
	targetUID int,
	success bool,
	duration time.Duration,
) {
	attrs := []slog.Attr{
		slog.String("audit_type", "privilege_escalation"),
		slog.Bool("audit", true), // Mark as audit event
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String(common.PrivilegeEscalationFailureAttrs.Operation, operation),
		slog.String(common.PrivilegeEscalationFailureAttrs.CommandName, commandName),
		slog.Int(common.PrivilegeEscalationFailureAttrs.OriginalUID, originalUID),
		slog.Int(common.PrivilegeEscalationFailureAttrs.TargetUID, targetUID),
		slog.Bool("success", success),
		slog.Int64("duration_ms", duration.Milliseconds()),
		slog.Int("process_id", os.Getpid()),
	}

	if success {
		l.logger.LogAttrs(ctx, slog.LevelInfo, "Privilege escalation successful", attrs...)
	} else {
		// Failed privilege escalation should be notified via Slack
		// Create new slice to avoid modifying the original attrs slice
		failureAttrs := slices.Clone(attrs)
		failureAttrs = append(failureAttrs,
			slog.Bool("slack_notify", true),
			slog.String("message_type", "privilege_escalation_failure"),
		)
		l.logger.LogAttrs(ctx, slog.LevelWarn, "Privilege escalation failed", failureAttrs...)
	}
}

// LogSecurityEvent logs security-related events and potential threats
func (l *Logger) LogSecurityEvent(
	ctx context.Context,
	eventType string,
	severity string,
	message string,
	details map[string]any,
) {
	attrs := []slog.Attr{
		slog.String("audit_type", "security_event"),
		slog.Bool("audit", true), // Mark as audit event
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String(common.SecurityAlertAttrs.EventType, eventType),
		slog.String(common.SecurityAlertAttrs.Severity, severity),
		slog.String(common.SecurityAlertAttrs.Message, message),
		slog.Int("user_id", os.Getuid()),
		slog.Int("effective_user_id", os.Geteuid()),
		slog.Int("process_id", os.Getpid()),
	}

	// Add custom details
	for key, value := range details {
		attrs = append(attrs, slog.Any(key, value))
	}

	// Add Slack notification for critical and high severity events
	shouldNotifySlack := severity == common.SeverityCritical || severity == common.SeverityHigh
	if shouldNotifySlack {
		attrs = append(attrs,
			slog.Bool("slack_notify", true),
			slog.String("message_type", "security_alert"),
		)
	}

	switch severity {
	case "critical", "high":
		l.logger.LogAttrs(ctx, slog.LevelError, "Security event", attrs...)
	case "medium":
		l.logger.LogAttrs(ctx, slog.LevelWarn, "Security event", attrs...)
	default:
		l.logger.LogAttrs(ctx, slog.LevelInfo, "Security event", attrs...)
	}
}

// LogRiskProfile logs command risk profile information with detailed risk factors
func (l *Logger) LogRiskProfile(
	ctx context.Context,
	commandName string,
	baseRiskLevel runnertypes.RiskLevel,
	riskReasons []string,
	networkType string,
) {
	attrs := []slog.Attr{
		slog.String("audit_type", "command_risk_profile"),
		slog.Bool("audit", true), // Mark as audit event
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String("command_name", commandName),
		slog.String("risk_level", baseRiskLevel.String()),
		slog.String("network_type", networkType),
		slog.Int("user_id", os.Getuid()),
		slog.Int("effective_user_id", os.Geteuid()),
		slog.Int("process_id", os.Getpid()),
	}

	// Add risk reasons as an array
	if len(riskReasons) > 0 {
		attrs = append(attrs, slog.Any("risk_factors", riskReasons))
	}

	// Determine log level based on risk level
	var logLevel slog.Level
	switch baseRiskLevel {
	case runnertypes.RiskLevelCritical:
		logLevel = slog.LevelError
	case runnertypes.RiskLevelHigh:
		logLevel = slog.LevelWarn
	case runnertypes.RiskLevelMedium:
		logLevel = slog.LevelInfo
	default:
		logLevel = slog.LevelDebug
	}

	l.logger.LogAttrs(ctx, logLevel, "Command risk profile", attrs...)
}
