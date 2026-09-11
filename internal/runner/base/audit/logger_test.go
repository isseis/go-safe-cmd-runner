package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuditLogger(t *testing.T) {
	auditLogger := audit.NewAuditLogger(nil)
	assert.NotNil(t, auditLogger)
}

func TestNewAuditLoggerWithCustom(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	auditLogger := audit.NewAuditLoggerWithCustom(logger)
	assert.NotNil(t, auditLogger)
}

func TestLogger_LogUserGroupExecution(t *testing.T) {
	tests := []struct {
		name        string
		cmd         *runnertypes.RuntimeCommand
		result      *audit.ExecutionResult
		duration    time.Duration
		metrics     audit.PrivilegeMetrics
		wantLevel   slog.Level
		wantMessage string
	}{
		{
			name: "successful user/group command",
			cmd: executortestutil.CreateRuntimeCommand("/bin/echo", []string{"test"},
				executortestutil.WithName("test_user_group_cmd"),
				executortestutil.WithRunAsUser("testuser"),
				executortestutil.WithRunAsGroup("testgroup")),
			result: &audit.ExecutionResult{
				Stdout:   "test output",
				Stderr:   "",
				ExitCode: 0,
			},
			duration: 100 * time.Millisecond,
			metrics: audit.PrivilegeMetrics{
				ElevationCount: 1,
				TotalDuration:  50 * time.Millisecond,
			},
			wantLevel:   slog.LevelInfo,
			wantMessage: "User/group command executed successfully",
		},
		{
			name: "failed user/group command",
			cmd: executortestutil.CreateRuntimeCommand("/bin/false", []string{},
				executortestutil.WithName("test_failed_user_group_cmd"),
				executortestutil.WithRunAsUser("testuser"),
				executortestutil.WithRunAsGroup("testgroup")),
			result: &audit.ExecutionResult{
				Stdout:   "",
				Stderr:   "command failed",
				ExitCode: 1,
			},
			duration: 200 * time.Millisecond,
			metrics: audit.PrivilegeMetrics{
				ElevationCount: 1,
				TotalDuration:  75 * time.Millisecond,
			},
			// A non-zero exit code is logged at ERROR under a different message.
			wantLevel:   slog.LevelError,
			wantMessage: "User/group command failed",
		},
		{
			name: "user only command",
			cmd: executortestutil.CreateRuntimeCommand("/bin/id", []string{},
				executortestutil.WithName("test_user_only_cmd"),
				executortestutil.WithRunAsUser("testuser")),
			result: &audit.ExecutionResult{
				Stdout:   "uid=1001(testuser)",
				Stderr:   "",
				ExitCode: 0,
			},
			duration: 50 * time.Millisecond,
			metrics: audit.PrivilegeMetrics{
				ElevationCount: 1,
				TotalDuration:  25 * time.Millisecond,
			},
			wantLevel:   slog.LevelInfo,
			wantMessage: "User/group command executed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, rec := tu.NewRecordingLogger()
			auditLogger := audit.NewAuditLoggerWithCustom(logger)

			ctx := context.Background()
			auditLogger.LogUserGroupExecution(ctx, tt.cmd, tt.result, tt.duration, tt.metrics)

			record := rec.RequireRecord(t, tt.wantLevel, tt.wantMessage)
			record.AssertAttrs(t, map[string]any{
				"audit_type":            "user_group_execution",
				"command_name":          tt.cmd.Name(),
				"expanded_command_path": tt.cmd.ExpandedCmd,
			})
			// The identity attributes are written only when the command declares
			// one, so their absence is as much a part of the contract as their value.
			if tt.cmd.RunAsUser() != "" {
				record.AssertAttrs(t, map[string]any{"run_as_user": tt.cmd.RunAsUser()})
			} else {
				assert.NotContains(t, record.Attrs, "run_as_user")
			}
			if tt.cmd.RunAsGroup() != "" {
				record.AssertAttrs(t, map[string]any{"run_as_group": tt.cmd.RunAsGroup()})
			} else {
				assert.NotContains(t, record.Attrs, "run_as_group")
			}

			// Only a failed command is a notification candidate, so only the
			// failure path carries the command scope context.
			if tt.result.ExitCode != 0 {
				record.AssertNotificationContext(t, common.CommandScope(tt.cmd.GroupName(), tt.cmd.Name()))
			} else {
				_, hasContext := record.NotificationContext()
				assert.False(t, hasContext, "successful commands must not carry a notification context")
			}
		})
	}
}

// TestLogger_LogUserGroupExecution_ByOperationAttrs pins the per-operation
// breakdown's key format and its microsecond unit (see PrivilegeMetrics.ByOperation).
func TestLogger_LogUserGroupExecution_ByOperationAttrs(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	auditLogger := audit.NewAuditLoggerWithCustom(logger)

	cmd := executortestutil.CreateRuntimeCommand("/bin/echo", []string{"test"},
		executortestutil.WithName("test_cmd"),
		executortestutil.WithRunAsUser("testuser"))
	result := &audit.ExecutionResult{ExitCode: 0}
	metrics := audit.PrivilegeMetrics{
		ElevationCount: 2,
		TotalDuration:  49 * time.Microsecond,
		ByOperation: map[runnertypes.Operation]time.Duration{
			runnertypes.OperationUserGroupExecution: 42 * time.Microsecond,
			runnertypes.OperationKillAfterCancel:    7 * time.Microsecond,
		},
	}

	auditLogger.LogUserGroupExecution(context.Background(), cmd, result, 0, metrics)

	rec.RequireRecord(t, slog.LevelInfo, "User/group command executed successfully").
		AssertAttrs(t, map[string]any{
			"privilege_duration_user_group_execution_us": int64(42),
			"privilege_duration_kill_after_cancel_us":    int64(7),
		})
}

// logUserGroupExecutionEntry runs LogUserGroupExecution against a fresh JSON
// logger (no RedactingHandler) and returns the parsed log entry.
func logUserGroupExecutionEntry(
	t *testing.T,
	cmd *runnertypes.RuntimeCommand,
	result *audit.ExecutionResult,
) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	audit.NewAuditLoggerWithCustom(logger).LogUserGroupExecution(
		context.Background(), cmd, result, 0, audit.PrivilegeMetrics{},
	)

	var logEntry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &logEntry), "failed to parse JSON log output")
	return logEntry
}

// TestLogUserGroupExecution_OutputMasking verifies stdout/stderr recorded on a
// failed command are boundary-redacted before being logged.
func TestLogUserGroupExecution_OutputMasking(t *testing.T) {
	t.Run("sensitive stderr masked", func(t *testing.T) {
		cmd := executortestutil.CreateRuntimeCommand("/bin/false", []string{})
		result := &audit.ExecutionResult{
			Stdout:   "leaked token: apikey=letmein123",
			Stderr:   "connection failed: password=hunter2",
			ExitCode: 1,
		}
		// positive control: the secrets are actually present in the raw values.
		require.Contains(t, result.Stdout, "letmein123")
		require.Contains(t, result.Stderr, "hunter2")

		entry := logUserGroupExecutionEntry(t, cmd, result)

		stdout, ok := entry["stdout"].(string)
		require.True(t, ok)
		assert.NotContains(t, stdout, "letmein123", "secret must be masked")
		assert.Contains(t, stdout, "[REDACTED]")

		stderr, ok := entry["stderr"].(string)
		require.True(t, ok)
		assert.NotContains(t, stderr, "hunter2", "secret must be masked")
		assert.Contains(t, stderr, "[REDACTED]")
	})

	t.Run("NoSensitiveContent", func(t *testing.T) {
		cmd := executortestutil.CreateRuntimeCommand("/bin/false", []string{})
		result := &audit.ExecutionResult{
			Stdout:   "normal output",
			Stderr:   "normal error",
			ExitCode: 1,
		}
		entry := logUserGroupExecutionEntry(t, cmd, result)
		assert.Equal(t, "normal output", entry["stdout"])
		assert.Equal(t, "normal error", entry["stderr"])
	})
}

// TestLogUserGroupExecution_ArgMasking verifies command_args/expanded_command_args
// are boundary-redacted.
func TestLogUserGroupExecution_ArgMasking(t *testing.T) {
	t.Run("sensitive args masked", func(t *testing.T) {
		cmd := executortestutil.CreateRuntimeCommand("/bin/echo", []string{"--password=supersecretvalue"})
		result := &audit.ExecutionResult{ExitCode: 0}
		// positive control
		require.Contains(t, strings.Join(cmd.Args(), " "), "supersecretvalue")

		entry := logUserGroupExecutionEntry(t, cmd, result)

		args, ok := entry["command_args"].(string)
		require.True(t, ok)
		assert.NotContains(t, args, "supersecretvalue", "secret must be masked")
		assert.Contains(t, args, "[REDACTED]")

		expandedArgs, ok := entry["expanded_command_args"].(string)
		require.True(t, ok)
		assert.NotContains(t, expandedArgs, "supersecretvalue", "secret must be masked")
		assert.Contains(t, expandedArgs, "[REDACTED]")
	})

	t.Run("NoSensitiveContent", func(t *testing.T) {
		cmd := executortestutil.CreateRuntimeCommand("/bin/echo", []string{"--verbose", "value"})
		result := &audit.ExecutionResult{ExitCode: 0}
		entry := logUserGroupExecutionEntry(t, cmd, result)
		assert.Equal(t, "--verbose value", entry["command_args"])
	})
}

// TestNewAuditLogger_UsesInjectedRedactionConfig verifies that NewAuditLogger
// applies the *redaction.Config it is given (e.g. one built with
// WithWebhookHost) rather than always falling back to DefaultConfig(), which
// has no webhook host and so cannot mask a Slack-compatible webhook URL.
func TestNewAuditLogger_UsesInjectedRedactionConfig(t *testing.T) {
	const webhookURL = "https://mattermost.example.com/hooks/abcdefghijklmnopqrstuvwxyz"

	redactionConfig, err := redaction.NewConfig(redaction.WithWebhookHost("mattermost.example.com"))
	require.NoError(t, err)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	cmd := executortestutil.CreateRuntimeCommand("/bin/false", []string{})
	result := &audit.ExecutionResult{Stderr: webhookURL, ExitCode: 1}

	audit.NewAuditLoggerWithCustomRedaction(logger, redactionConfig).LogUserGroupExecution(
		context.Background(), cmd, result, 0, audit.PrivilegeMetrics{},
	)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	stderr, ok := entry["stderr"].(string)
	require.True(t, ok)
	assert.NotContains(t, stderr, "abcdefghijklmnopqrstuvwxyz", "webhook path must not reach the log output")
	assert.Contains(t, stderr, "https://mattermost.example.com/[REDACTED]")
}

// logRiskProfileEntry runs LogRiskProfile against a fresh DEBUG-level JSON logger
// and returns the parsed log entry. DEBUG is used so even low-risk allow entries
// (which log at Debug) are captured.
func logRiskProfileEntry(t *testing.T, entry risktypes.RiskAuditEntry) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	audit.NewAuditLoggerWithCustom(logger).LogRiskProfile(context.Background(), entry)

	var logEntry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &logEntry), "failed to parse JSON log output")
	return logEntry
}

// TestLogRiskProfile_LogLevelByRisk verifies the log level corresponds to
// the effective risk level for allow decisions (no deny floor applies).
func TestLogRiskProfile_LogLevelByRisk(t *testing.T) {
	tests := []struct {
		name          string
		level         runnertypes.RiskLevel
		expectedLevel string
	}{
		{"critical maps to error", runnertypes.RiskLevelCritical, "ERROR"},
		{"high maps to warn", runnertypes.RiskLevelHigh, "WARN"},
		{"medium maps to info", runnertypes.RiskLevelMedium, "INFO"},
		{"low maps to debug", runnertypes.RiskLevelLow, "DEBUG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
				CommandName: "cmd",
				Mode:        risktypes.ModeNormal,
				Decision:    risktypes.DecisionAllow,
				Assessment:  risktypes.RiskAssessment{Level: tt.level},
			})
			assert.Equal(t, "command_risk_profile", entry["audit_type"])
			assert.Equal(t, tt.expectedLevel, entry["level"])
			assert.Equal(t, "allow", entry["decision"])
		})
	}
}

// TestLogRiskProfile_DenySeverityFloor verifies every deny is logged at
// Warn or above, independent of the risk-level mapping, so a Medium command
// denied under a Low ceiling does not sink to Info.
func TestLogRiskProfile_DenySeverityFloor(t *testing.T) {
	tests := []struct {
		name          string
		level         runnertypes.RiskLevel
		expectedLevel string
	}{
		{"low deny floored to warn", runnertypes.RiskLevelLow, "WARN"},
		{"medium deny floored to warn", runnertypes.RiskLevelMedium, "WARN"},
		{"high deny stays warn", runnertypes.RiskLevelHigh, "WARN"},
		{"critical deny stays error", runnertypes.RiskLevelCritical, "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
				CommandName: "cmd",
				Mode:        risktypes.ModeNormal,
				Decision:    risktypes.DecisionDeny,
				Assessment:  risktypes.RiskAssessment{Level: tt.level},
			})
			assert.Equal(t, tt.expectedLevel, entry["level"])
			assert.Equal(t, "deny", entry["decision"])
		})
	}
}

// TestLogRiskProfile_ReasonCodesAndFactors verifies the entry carries both
// machine-readable reason codes and human-readable risk factors.
func TestLogRiskProfile_ReasonCodesAndFactors(t *testing.T) {
	entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
		CommandName: "claude",
		Mode:        risktypes.ModeNormal,
		Decision:    risktypes.DecisionAllow,
		Assessment: risktypes.RiskAssessment{
			Level:       runnertypes.RiskLevelHigh,
			ReasonCodes: []risktypes.ReasonCode{risktypes.ReasonProfileDataExfil, risktypes.ReasonProfileNetwork},
			Reasons:     []string{"May send sensitive data to external service"},
		},
	})

	codes, ok := entry["reason_codes"].([]any)
	require.True(t, ok, "reason_codes should be an array")
	assert.ElementsMatch(t, []any{"profile_data_exfil", "profile_network"}, codes)

	factors, ok := entry["risk_factors"].([]any)
	require.True(t, ok, "risk_factors should be an array")
	assert.Equal(t, []any{"May send sensitive data to external service"}, factors)
}

// TestLogRiskProfile_NoProfileReasonCode verifies a command with no
// profile (e.g. binary-analysis-derived risk) still emits a reason code.
func TestLogRiskProfile_NoProfileReasonCode(t *testing.T) {
	entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
		CommandName: "unknown-tool",
		Mode:        risktypes.ModeNormal,
		Decision:    risktypes.DecisionAllow,
		Assessment: risktypes.RiskAssessment{
			Level:       runnertypes.RiskLevelMedium,
			ReasonCodes: []risktypes.ReasonCode{risktypes.ReasonBinaryAnalysisNetwork},
			// No human-readable Reasons (no profile).
		},
	})

	codes, ok := entry["reason_codes"].([]any)
	require.True(t, ok, "reason_codes should be present even without a profile")
	assert.Contains(t, codes, "binary_analysis_network")
	assert.NotContains(t, entry, "risk_factors")
}

// TestLogRiskProfile_UncertainReason verifies an uncertain abort records
// which uncertain case caused it via the blocking reason and reason codes.
func TestLogRiskProfile_UncertainReason(t *testing.T) {
	entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
		CommandName: "mystery",
		Mode:        risktypes.ModeNormal,
		Decision:    risktypes.DecisionDeny,
		Assessment: risktypes.RiskAssessment{
			Level:          runnertypes.RiskLevelLow,
			Blocking:       true,
			BlockingReason: risktypes.ReasonUncertainMissingRecord,
			ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonUncertainMissingRecord},
		},
	})

	assert.Equal(t, "uncertain_missing_record", entry["blocking_reason"])
	codes, ok := entry["reason_codes"].([]any)
	require.True(t, ok)
	assert.Contains(t, codes, "uncertain_missing_record")
	assert.Equal(t, "deny", entry["decision"])
}

// TestLogRiskProfile_CorrelationFieldsAndAbsence verifies correlation
// fields carry real values when present and an explicit absence marker (never a
// sentinel inside a value field) when absent, and that a deny entry is still
// emitted.
func TestLogRiskProfile_CorrelationFieldsAndAbsence(t *testing.T) {
	t.Run("all present", func(t *testing.T) {
		entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
			CommandName:    "rm",
			Mode:           risktypes.ModeNormal,
			ResolvedPath:   new("/usr/bin/rm"),
			ContentHash:    new("sha256:abc"),
			RecordID:       new("schema-v1"),
			MaxAllowedRisk: runnertypes.RiskLevelLow,
			Decision:       risktypes.DecisionDeny,
			Assessment: risktypes.RiskAssessment{
				Level:          runnertypes.RiskLevelHigh,
				BlockingReason: risktypes.ReasonDestructiveFileOperation,
				ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonDestructiveFileOperation},
			},
		})
		assert.Equal(t, "/usr/bin/rm", entry["resolved_path"])
		assert.Equal(t, "sha256:abc", entry["content_hash"])
		assert.Equal(t, "schema-v1", entry["record_id"])
		assert.Equal(t, "low", entry["max_allowed_risk"])
		assert.Equal(t, "deny", entry["decision"])
	})

	t.Run("absent rendered as marker, deny still emitted", func(t *testing.T) {
		entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
			CommandName:    "missing",
			Mode:           risktypes.ModeNormal,
			MaxAllowedRisk: runnertypes.RiskLevelLow,
			Decision:       risktypes.DecisionDeny,
			ErrorClass:     risktypes.ErrorClassPathResolution,
			Assessment: risktypes.RiskAssessment{
				Level:          runnertypes.RiskLevelLow,
				Blocking:       true,
				BlockingReason: risktypes.ReasonSymlinkResolutionFailed,
			},
		})
		// Absence is explicit via the boundary marker; the DTO held nil, never a sentinel.
		assert.Equal(t, "n/a", entry["resolved_path"])
		assert.Equal(t, "n/a", entry["content_hash"])
		assert.Equal(t, "n/a", entry["record_id"])
		assert.Equal(t, "deny", entry["decision"])
		assert.Equal(t, "path_resolution", entry["error_class"])
	})
}

// TestLogRiskProfile_ArgMasking verifies secrets passed as command
// arguments are masked using the redaction mechanism before being logged.
func TestLogRiskProfile_ArgMasking(t *testing.T) {
	entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
		CommandName: "deploy",
		Mode:        risktypes.ModeNormal,
		Decision:    risktypes.DecisionAllow,
		Args:        []string{"--user=admin", "--password=supersecretvalue"},
		Assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	})

	args, ok := entry["command_args"].([]any)
	require.True(t, ok, "command_args should be an array")
	var sb strings.Builder
	for _, a := range args {
		sb.WriteString(a.(string) + " ")
	}
	joined := sb.String()
	assert.NotContains(t, joined, "supersecretvalue", "secret must be masked")
	assert.Contains(t, joined, "[REDACTED]")
	assert.Contains(t, joined, "admin", "non-sensitive arg preserved")
}

// TestLogRiskProfile_OperandZones verifies the per-operand trust-zone records are
// emitted with each subfield carrying the carrier value, that an unresolved
// operand keeps its cause while leaving resolved empty, that an empty carrier
// omits the key, and that the key is still emitted on a deny path.
func TestLogRiskProfile_OperandZones(t *testing.T) {
	zones := []risktypes.OperandZone{
		{
			Index:           0,
			Raw:             "/usr/bin/ls",
			Resolved:        "/usr/bin/ls",
			Zone:            risktypes.ZoneTrustCritical,
			Role:            risktypes.OperandRoleWrite,
			MatchedCritical: "/usr/bin",
			Trusted:         false,
		},
		{
			Index:    1,
			Raw:      "src",
			Resolved: "/work/src",
			Zone:     risktypes.ZoneSafeZone,
			Role:     risktypes.OperandRoleRead,
			Trusted:  true,
		},
		{
			Index:         2,
			Raw:           "/work/ghost",
			Resolved:      "", // unresolved: no resolved path
			Zone:          risktypes.ZoneUnresolved,
			Role:          risktypes.OperandRoleWrite,
			UnresolvedErr: "lstat /work/ghost: no such file or directory",
		},
	}

	t.Run("present with subfields, including unresolved", func(t *testing.T) {
		entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
			CommandName: "cp",
			Mode:        risktypes.ModeNormal,
			Decision:    risktypes.DecisionAllow,
			Assessment: risktypes.RiskAssessment{
				Level:        runnertypes.RiskLevelHigh,
				OperandZones: zones,
			},
		})

		got, ok := entry["operand_zones"].([]any)
		require.True(t, ok, "operand_zones should be an array")
		require.Len(t, got, 3)

		write := got[0].(map[string]any)
		assert.Equal(t, float64(0), write["index"])
		assert.Equal(t, "/usr/bin/ls", write["raw"])
		assert.Equal(t, "/usr/bin/ls", write["resolved"])
		assert.Equal(t, "trust-critical", write["zone"])
		assert.Equal(t, "write", write["role"])
		assert.Equal(t, "/usr/bin", write["matched_critical"])
		assert.Equal(t, false, write["trusted"])

		read := got[1].(map[string]any)
		assert.Equal(t, "read", read["role"])
		assert.Equal(t, "safe-zone", read["zone"])
		assert.Equal(t, true, read["trusted"])

		// An applied-but-unresolvable operand: resolved is empty (distinguishing
		// it from non-application) and the cause is preserved.
		unresolved := got[2].(map[string]any)
		assert.Equal(t, "unresolved", unresolved["zone"])
		assert.Empty(t, unresolved["resolved"], "unresolved operand has no resolved path")
		assert.Equal(t, "lstat /work/ghost: no such file or directory", unresolved["unresolved_err"])
	})

	t.Run("empty carrier omits the key", func(t *testing.T) {
		entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
			CommandName: "systemctl",
			Mode:        risktypes.ModeNormal,
			Decision:    risktypes.DecisionAllow,
			Assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelHigh},
		})
		assert.NotContains(t, entry, "operand_zones", "empty carrier must not write the key")
	})

	t.Run("emitted on a deny path", func(t *testing.T) {
		entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
			CommandName: "cp",
			Mode:        risktypes.ModeNormal,
			Decision:    risktypes.DecisionDeny,
			Assessment: risktypes.RiskAssessment{
				Level:        runnertypes.RiskLevelHigh,
				OperandZones: zones,
			},
		})
		got, ok := entry["operand_zones"].([]any)
		require.True(t, ok, "operand_zones should be present on a deny with a carrier")
		assert.Len(t, got, 3)
	})
}

// TestLogRiskProfile_OperandZoneMasking verifies secrets embedded in the operand
// Raw/Resolved/UnresolvedErr strings are masked through the same redaction as
// command_args, while non-sensitive path components are preserved (the mask is
// surgical, not a whole-field drop). This boundary redaction is the only control
// for these strings, so the negation is load-bearing (S-1).
func TestLogRiskProfile_OperandZoneMasking(t *testing.T) {
	entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
		CommandName: "curl",
		Mode:        risktypes.ModeNormal,
		Decision:    risktypes.DecisionAllow,
		Assessment: risktypes.RiskAssessment{
			Level: runnertypes.RiskLevelMedium,
			OperandZones: []risktypes.OperandZone{
				{
					Index:         0,
					Raw:           "/data/dump?token=supersecretvalue",
					Resolved:      "/data/dump?token=supersecretvalue",
					Zone:          risktypes.ZoneUnresolved,
					Role:          risktypes.OperandRoleWrite,
					UnresolvedErr: "open failed for password=hunter2 backend",
				},
			},
		},
	})

	got, ok := entry["operand_zones"].([]any)
	require.True(t, ok, "operand_zones should be an array")
	require.Len(t, got, 1)
	oz := got[0].(map[string]any)

	raw := oz["raw"].(string)
	resolved := oz["resolved"].(string)
	unresolvedErr := oz["unresolved_err"].(string)

	// (i) The secret values must not appear, replaced by the redaction placeholder.
	assert.NotContains(t, raw, "supersecretvalue", "raw secret must be masked")
	assert.NotContains(t, resolved, "supersecretvalue", "resolved secret must be masked")
	assert.NotContains(t, unresolvedErr, "hunter2", "unresolved_err secret must be masked")
	assert.Contains(t, raw, "[REDACTED]")
	assert.Contains(t, unresolvedErr, "[REDACTED]")

	// (ii) Non-sensitive path components are preserved (surgical mask).
	assert.Contains(t, raw, "/data/dump", "non-sensitive path component preserved")
}

// TestLogRiskProfile_Chain verifies an indirect-execution chain records
// every executed/loaded artifact so the chain is correlatable from one entry.
func TestLogRiskProfile_Chain(t *testing.T) {
	entry := logRiskProfileEntry(t, risktypes.RiskAuditEntry{
		CommandName: "env",
		Mode:        risktypes.ModeNormal,
		Decision:    risktypes.DecisionAllow,
		Assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelMedium},
		Chain: []risktypes.ExecutedArtifact{
			{Path: "/usr/bin/env", Role: risktypes.RoleWrapper, Disposition: risktypes.DispVerified, ContentHash: new("sha256:env")},
			{Path: "/usr/bin/curl", Role: risktypes.RoleInner, Disposition: risktypes.DispVerified, ContentHash: new("sha256:curl")},
		},
	})

	chain, ok := entry["chain"].([]any)
	require.True(t, ok, "chain should be an array")
	require.Len(t, chain, 2)

	first := chain[0].(map[string]any)
	assert.Equal(t, "/usr/bin/env", first["path"])
	assert.Equal(t, "wrapper", first["role"])
	assert.Equal(t, "verified", first["disposition"])
	assert.Equal(t, "sha256:env", first["content_hash"])

	second := chain[1].(map[string]any)
	assert.Equal(t, "/usr/bin/curl", second["path"])
	assert.Equal(t, "inner", second["role"])
}
