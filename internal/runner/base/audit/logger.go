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
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// auditValueAbsent is the fixed marker rendered for a correlation field whose
// value could not be obtained (a nil pointer in RiskAuditEntry). The marker is
// applied only at this log-output boundary; the entry itself represents absence
// with a nil pointer and never stores a sentinel string in a value field.
// Keeping the key present on every entry lets incident searches treat the field
// uniformly instead of distinguishing "absent" from "omitted".
const auditValueAbsent = "n/a"

// argRedactor masks secrets in command arguments before they reach the log,
// staying consistent with the global redaction mechanism. The production
// logger also runs behind a RedactingHandler; applying redaction here keeps the
// masking guarantee even when LogRiskProfile writes to a logger without that
// handler.
var argRedactor = redaction.DefaultConfig()

// Logger provides structured audit logging functionality
type Logger struct {
	logger *slog.Logger
}

// NewAuditLogger creates a new audit logger instance using the global logger
// This integrates with the new logging framework for unified logging
func NewAuditLogger() *Logger {
	return &Logger{logger: slog.Default()}
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
		slog.String("command_args", argRedactor.RedactText(strings.Join(cmd.Args(), " "))),
		slog.String("expanded_command_path", cmd.ExpandedCmd),
		slog.String("expanded_command_args", argRedactor.RedactText(strings.Join(cmd.ExpandedArgs, " "))),
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
			slog.String("stdout", argRedactor.RedactText(result.Stdout)),
			slog.String("stderr", argRedactor.RedactText(result.Stderr)),
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
		slog.String(common.PrivilegeEscalationFailureAttrs.Operation, argRedactor.RedactText(operation)),
		slog.String(common.PrivilegeEscalationFailureAttrs.CommandName, argRedactor.RedactText(commandName)),
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
		failureAttrs = append(
			failureAttrs,
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
		slog.String(common.SecurityAlertAttrs.Message, argRedactor.RedactText(message)),
		slog.Int("user_id", os.Getuid()),
		slog.Int("effective_user_id", os.Geteuid()),
		slog.Int("process_id", os.Getpid()),
	}

	// Add custom details under a "detail_" namespace so caller-supplied keys
	// cannot collide with (and overwrite) the schema attributes above. String
	// values are boundary-redacted here; composite values (map/struct/slice)
	// are left to the RedactingHandler's recursive redaction.
	for key, value := range details {
		prefixedKey := "detail_" + key
		switch v := value.(type) {
		case string:
			attrs = append(attrs, slog.String(prefixedKey, argRedactor.RedactText(v)))
		case int:
			attrs = append(attrs, slog.Int64(prefixedKey, int64(v)))
		case int64:
			attrs = append(attrs, slog.Int64(prefixedKey, v))
		case float64:
			attrs = append(attrs, slog.Float64(prefixedKey, v))
		case bool:
			attrs = append(attrs, slog.Bool(prefixedKey, v))
		default:
			attrs = append(attrs, slog.Any(prefixedKey, v))
		}
	}

	// Add Slack notification for critical and high severity events
	shouldNotifySlack := severity == common.SeverityCritical || severity == common.SeverityHigh
	if shouldNotifySlack {
		attrs = append(
			attrs,
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

// LogRiskProfile emits the command_risk_profile audit entry for a single risk
// decision. It takes a RiskAuditEntry parameter object (defined in risktypes so
// audit does not import risk) carrying the correlation fields required for incident correlation:
// resolved_path, content_hash, the analysis record identifier, max_allowed_risk,
// decision, reason_codes, and the human-readable risk_factors. The entry
// is always written -- including for denies on the error-return path -- so audit
// is never skipped by an early return.
func (l *Logger) LogRiskProfile(ctx context.Context, entry risktypes.RiskAuditEntry) {
	assessment := entry.Assessment

	attrs := []slog.Attr{
		slog.String("audit_type", "command_risk_profile"),
		slog.Bool("audit", true), // Mark as audit event
		slog.Int64("timestamp", time.Now().Unix()),
		slog.String("command_name", entry.CommandName),
		slog.String("mode", string(entry.Mode)),
		slog.String("decision", string(entry.Decision)),
		slog.String("risk_level", assessment.Level.String()),
		slog.String("max_allowed_risk", entry.MaxAllowedRisk.String()),
		// Correlation fields. Absent (nil) values render as the boundary marker so
		// the key is present on every entry without storing a sentinel in the DTO.
		slog.String("resolved_path", optStr(entry.ResolvedPath)),
		slog.String("content_hash", optStr(entry.ContentHash)),
		slog.String("record_id", optStr(entry.RecordID)),
		slog.String("network_type", assessment.NetworkType),
		slog.Int("user_id", os.Getuid()),
		slog.Int("effective_user_id", os.Geteuid()),
		slog.Int("process_id", os.Getpid()),
	}

	// Machine-readable reason codes: present even for commands without a
	// profile (e.g. binary-analysis-derived risk).
	if len(assessment.ReasonCodes) > 0 {
		codes := make([]string, len(assessment.ReasonCodes))
		for i, c := range assessment.ReasonCodes {
			codes[i] = string(c)
		}
		attrs = append(attrs, slog.Any("reason_codes", codes))
	}

	// Human-readable risk factors.
	if len(assessment.Reasons) > 0 {
		attrs = append(attrs, slog.Any("risk_factors", assessment.Reasons))
	}

	// Deny reasoning. BlockingReason is set for every deny (Blocking or Critical);
	// ErrorClass distinguishes a failure-induced deny.
	if assessment.BlockingReason != "" {
		attrs = append(attrs, slog.String("blocking_reason", string(assessment.BlockingReason)))
	}
	if entry.ErrorClass != "" {
		attrs = append(attrs, slog.String("error_class", string(entry.ErrorClass)))
	}
	if entry.VerificationUnavailable {
		attrs = append(attrs, slog.Bool("verification_unavailable", true))
	}

	// Masked command arguments. Apply redaction here so secrets are not
	// leaked even when the destination logger has no RedactingHandler.
	if len(entry.Args) > 0 {
		masked := make([]string, len(entry.Args))
		for i, a := range entry.Args {
			masked[i] = argRedactor.RedactText(a)
		}
		attrs = append(attrs, slog.Any("command_args", masked))
	}

	// Indirect-execution chain: each executed/loaded artifact's identity so
	// the whole chain is correlatable from a single entry.
	if len(entry.Chain) > 0 {
		chain := make([]map[string]string, len(entry.Chain))
		for i := range entry.Chain {
			a := &entry.Chain[i]
			chain[i] = map[string]string{
				"path":         a.Path,
				"role":         string(a.Role),
				"disposition":  string(a.Disposition),
				"content_hash": optStr(a.ContentHash),
			}
		}
		attrs = append(attrs, slog.Any("chain", chain))
	}

	// Per-operand trust-zone records. Present only when axis 2 applied (a file
	// operation): an empty/nil carrier means axis 2 did not apply, so the key is
	// omitted (same len()>0 guard as reason_codes/chain; never write []/null).
	// Raw/Resolved/UnresolvedErr may carry secrets, so each is routed through the
	// same boundary redaction as command_args. The RedactingHandler now also
	// recurses into map/slice elements passed via slog.Any, so this boundary
	// redaction is a second, redundant layer (defense-in-depth) rather than the
	// only control. Zone/Role/MatchedCritical/Index/Trusted are enum, fixed-path,
	// or non-string values and are not masked.
	// Elements are map[string]any (not map[string]string like chain) to preserve
	// the int Index and bool Trusted as typed JSON values.
	if len(assessment.OperandZones) > 0 {
		zones := make([]map[string]any, len(assessment.OperandZones))
		for i := range assessment.OperandZones {
			oz := &assessment.OperandZones[i]
			zones[i] = map[string]any{
				"index":            oz.Index,
				"raw":              argRedactor.RedactText(oz.Raw),
				"resolved":         argRedactor.RedactText(oz.Resolved),
				"zone":             string(oz.Zone),
				"role":             string(oz.Role),
				"matched_critical": oz.MatchedCritical,
				"trusted":          oz.Trusted,
				"unresolved_err":   argRedactor.RedactText(oz.UnresolvedErr),
			}
		}
		attrs = append(attrs, slog.Any("operand_zones", zones))
	}

	l.logger.LogAttrs(ctx, riskLogLevel(assessment.Level, entry.Decision), "Command risk profile", attrs...)
}

// optStr renders an optional correlation value: its real value when present, or
// the absence marker when nil. The DTO never holds a sentinel string; the marker
// exists only at this output boundary.
func optStr(s *string) string {
	if s == nil {
		return auditValueAbsent
	}
	return *s
}

// riskLogLevel maps the effective risk to a log level (Critical->Error,
// High->Warn, Medium->Info, else->Debug) and then applies the deny severity
// floor: any deny is at least Warn, so a Medium command denied under a
// Low ceiling is still found by a Warn/Error deny search instead of sinking to
// Info.
func riskLogLevel(level runnertypes.RiskLevel, decision risktypes.Decision) slog.Level {
	var logLevel slog.Level
	switch level {
	case runnertypes.RiskLevelCritical:
		logLevel = slog.LevelError
	case runnertypes.RiskLevelHigh:
		logLevel = slog.LevelWarn
	case runnertypes.RiskLevelMedium:
		logLevel = slog.LevelInfo
	default:
		logLevel = slog.LevelDebug
	}
	if decision == risktypes.DecisionDeny {
		logLevel = max(logLevel, slog.LevelWarn)
	}
	return logLevel
}
