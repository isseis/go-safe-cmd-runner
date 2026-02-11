package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuditLogger(t *testing.T) {
	auditLogger := audit.NewAuditLogger()
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
		name     string
		cmd      *runnertypes.RuntimeCommand
		result   *audit.ExecutionResult
		duration time.Duration
		metrics  audit.PrivilegeMetrics
	}{
		{
			name: "successful user/group command",
			cmd: executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
				executortesting.WithName("test_user_group_cmd"),
				executortesting.WithRunAsUser("testuser"),
				executortesting.WithRunAsGroup("testgroup")),
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
		},
		{
			name: "failed user/group command",
			cmd: executortesting.CreateRuntimeCommand("/bin/false", []string{},
				executortesting.WithName("test_failed_user_group_cmd"),
				executortesting.WithRunAsUser("testuser"),
				executortesting.WithRunAsGroup("testgroup")),
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
		},
		{
			name: "user only command",
			cmd: executortesting.CreateRuntimeCommand("/bin/id", []string{},
				executortesting.WithName("test_user_only_cmd"),
				executortesting.WithRunAsUser("testuser")),
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			auditLogger := audit.NewAuditLoggerWithCustom(logger)

			ctx := context.Background()
			auditLogger.LogUserGroupExecution(ctx, tt.cmd, tt.result, tt.duration, tt.metrics)

			logOutput := buf.String()
			assert.Contains(t, logOutput, "user_group_execution")
			assert.Contains(t, logOutput, tt.cmd.Name())
			assert.Contains(t, logOutput, tt.cmd.ExpandedCmd)
			if tt.cmd.RunAsUser() != "" {
				assert.Contains(t, logOutput, tt.cmd.RunAsUser())
			}
			if tt.cmd.RunAsGroup() != "" {
				assert.Contains(t, logOutput, tt.cmd.RunAsGroup())
			}
		})
	}
}

func TestLogger_LogPrivilegeEscalation(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	auditLogger := audit.NewAuditLoggerWithCustom(logger)

	ctx := context.Background()
	operation := "command_execution"
	commandName := "test_command"
	originalUID := 1000
	targetUID := 0
	success := true
	duration := 10 * time.Millisecond

	auditLogger.LogPrivilegeEscalation(ctx, operation, commandName, originalUID, targetUID, success, duration)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "Privilege escalation successful")
	assert.Contains(t, logOutput, "audit_type")
	assert.Contains(t, logOutput, "privilege_escalation")
	assert.Contains(t, logOutput, operation)
	assert.Contains(t, logOutput, commandName)
}

func TestLogger_LogSecurityEvent(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		severity  string
		message   string
		details   map[string]any
		expectLog string
	}{
		{
			name:      "critical security event",
			eventType: "privilege_violation",
			severity:  "critical",
			message:   "Unauthorized privilege escalation attempt",
			details: map[string]any{
				"source_uid": 1000,
				"target_uid": 0,
				"command":    "/bin/su",
			},
			expectLog: "Security event",
		},
		{
			name:      "info security event",
			eventType: "audit_log",
			severity:  "info",
			message:   "Regular security audit",
			details:   map[string]any{},
			expectLog: "Security event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			auditLogger := audit.NewAuditLoggerWithCustom(logger)

			ctx := context.Background()
			auditLogger.LogSecurityEvent(ctx, tt.eventType, tt.severity, tt.message, tt.details)

			logOutput := buf.String()
			assert.Contains(t, logOutput, tt.expectLog)
			assert.Contains(t, logOutput, "audit_type")
			assert.Contains(t, logOutput, "security_event")
			assert.Contains(t, logOutput, tt.eventType)
			assert.Contains(t, logOutput, tt.severity)
			assert.Contains(t, logOutput, tt.message)
		})
	}
}

func TestLogger_LogRiskProfile(t *testing.T) {
	tests := []struct {
		name              string
		commandName       string
		baseRiskLevel     runnertypes.RiskLevel
		riskReasons       []string
		networkType       string
		expectedAuditType string
		expectedRiskLevel string
		expectedLogLevel  string
	}{
		{
			name:              "single risk factor",
			commandName:       "curl",
			baseRiskLevel:     runnertypes.RiskLevelMedium,
			riskReasons:       []string{"Always performs network operations"},
			networkType:       "Always",
			expectedAuditType: "command_risk_profile",
			expectedRiskLevel: "medium",
			expectedLogLevel:  "INFO",
		},
		{
			name:          "multiple risk factors",
			commandName:   "claude",
			baseRiskLevel: runnertypes.RiskLevelHigh,
			riskReasons: []string{
				"Always communicates with external AI API",
				"May send sensitive data to external service",
			},
			networkType:       "Always",
			expectedAuditType: "command_risk_profile",
			expectedRiskLevel: "high",
			expectedLogLevel:  "WARN",
		},
		{
			name:              "privilege escalation",
			commandName:       "sudo",
			baseRiskLevel:     runnertypes.RiskLevelCritical,
			riskReasons:       []string{"Allows execution with elevated privileges, can compromise entire system"},
			networkType:       "None",
			expectedAuditType: "command_risk_profile",
			expectedRiskLevel: "critical",
			expectedLogLevel:  "ERROR",
		},
		{
			name:              "unknown risk level",
			commandName:       "ls",
			baseRiskLevel:     runnertypes.RiskLevelUnknown,
			riskReasons:       []string{},
			networkType:       "None",
			expectedAuditType: "command_risk_profile",
			expectedRiskLevel: "unknown",
			expectedLogLevel:  "DEBUG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			// Use DEBUG level to capture all log levels including DEBUG
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			auditLogger := audit.NewAuditLoggerWithCustom(logger)

			ctx := context.Background()
			auditLogger.LogRiskProfile(ctx, tt.commandName, tt.baseRiskLevel, tt.riskReasons, tt.networkType)

			// Parse JSON log output
			var logEntry map[string]any
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			require.NoError(t, err, "Failed to parse JSON log output")

			// Validate structured fields
			assert.Equal(t, tt.expectedAuditType, logEntry["audit_type"])
			assert.Equal(t, true, logEntry["audit"])
			assert.Equal(t, tt.commandName, logEntry["command_name"])
			assert.Equal(t, tt.expectedRiskLevel, logEntry["risk_level"])
			assert.Equal(t, tt.networkType, logEntry["network_type"])
			assert.Equal(t, tt.expectedLogLevel, logEntry["level"])

			// Validate risk_factors array if present
			if len(tt.riskReasons) > 0 {
				require.Contains(t, logEntry, "risk_factors")
				riskFactors, ok := logEntry["risk_factors"].([]any)
				require.True(t, ok, "risk_factors should be an array")
				require.Equal(t, len(tt.riskReasons), len(riskFactors))
				for i, expectedReason := range tt.riskReasons {
					assert.Equal(t, expectedReason, riskFactors[i])
				}
			} else {
				assert.NotContains(t, logEntry, "risk_factors")
			}

			// Validate that standard fields are present
			assert.Contains(t, logEntry, "timestamp")
			assert.Contains(t, logEntry, "user_id")
			assert.Contains(t, logEntry, "effective_user_id")
			assert.Contains(t, logEntry, "process_id")
		})
	}
}
