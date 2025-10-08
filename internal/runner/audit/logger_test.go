package audit_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
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
		cmd      runnertypes.Command
		result   *audit.ExecutionResult
		duration time.Duration
		metrics  audit.PrivilegeMetrics
	}{
		{
			name: "successful user/group command",
			cmd: runnertypes.Command{
				Name:       "test_user_group_cmd",
				Cmd:        "/bin/echo",
				Args:       []string{"test"},
				RunAsUser:  "testuser",
				RunAsGroup: "testgroup",
			},
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
			cmd: runnertypes.Command{
				Name:       "test_failed_user_group_cmd",
				Cmd:        "/bin/false",
				Args:       []string{},
				RunAsUser:  "testuser",
				RunAsGroup: "testgroup",
			},
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
			cmd: runnertypes.Command{
				Name:      "test_user_only_cmd",
				Cmd:       "/bin/id",
				Args:      []string{},
				RunAsUser: "testuser",
			},
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
			assert.Contains(t, logOutput, tt.cmd.Name)
			assert.Contains(t, logOutput, tt.cmd.Cmd)
			if tt.cmd.RunAsUser != "" {
				assert.Contains(t, logOutput, tt.cmd.RunAsUser)
			}
			if tt.cmd.RunAsGroup != "" {
				assert.Contains(t, logOutput, tt.cmd.RunAsGroup)
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
		name           string
		commandName    string
		baseRiskLevel  runnertypes.RiskLevel
		riskReasons    []string
		networkType    string
		expectContains []string
	}{
		{
			name:          "single risk factor",
			commandName:   "curl",
			baseRiskLevel: runnertypes.RiskLevelMedium,
			riskReasons:   []string{"Always performs network operations"},
			networkType:   "Always",
			expectContains: []string{
				"command_risk_profile",
				"curl",
				"medium",
				"Always performs network operations",
				"Always",
			},
		},
		{
			name:          "multiple risk factors",
			commandName:   "claude",
			baseRiskLevel: runnertypes.RiskLevelHigh,
			riskReasons: []string{
				"Always communicates with external AI API",
				"May send sensitive data to external service",
			},
			networkType: "Always",
			expectContains: []string{
				"command_risk_profile",
				"claude",
				"high",
				"Always communicates with external AI API",
				"May send sensitive data to external service",
			},
		},
		{
			name:          "privilege escalation",
			commandName:   "sudo",
			baseRiskLevel: runnertypes.RiskLevelCritical,
			riskReasons:   []string{"Allows execution with elevated privileges, can compromise entire system"},
			networkType:   "None",
			expectContains: []string{
				"command_risk_profile",
				"sudo",
				"critical",
				"Allows execution with elevated privileges, can compromise entire system",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			auditLogger := audit.NewAuditLoggerWithCustom(logger)

			ctx := context.Background()
			auditLogger.LogRiskProfile(ctx, tt.commandName, tt.baseRiskLevel, tt.riskReasons, tt.networkType)

			logOutput := buf.String()
			for _, expected := range tt.expectContains {
				assert.Contains(t, logOutput, expected, "Expected log to contain: %s", expected)
			}
		})
	}
}
