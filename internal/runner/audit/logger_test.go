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
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	auditLogger := audit.NewAuditLogger(logger)
	assert.NotNil(t, auditLogger)
}

func TestLogger_LogPrivilegedExecution(t *testing.T) {
	tests := []struct {
		name         string
		cmd          runnertypes.Command
		result       *audit.ExecutionResult
		expectLogMsg string
	}{
		{
			name: "successful privileged execution",
			cmd: runnertypes.Command{
				Name:       "test_cmd",
				Cmd:        "/bin/echo",
				Args:       []string{"hello"},
				Privileged: true,
			},
			result: &audit.ExecutionResult{
				Stdout:   "hello\n",
				Stderr:   "",
				ExitCode: 0,
			},
			expectLogMsg: "Privileged command executed successfully",
		},
		{
			name: "failed privileged execution",
			cmd: runnertypes.Command{
				Name:       "test_fail",
				Cmd:        "/bin/false",
				Args:       []string{},
				Privileged: true,
			},
			result: &audit.ExecutionResult{
				Stdout:   "",
				Stderr:   "command failed",
				ExitCode: 1,
			},
			expectLogMsg: "Privileged command failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			auditLogger := audit.NewAuditLogger(logger)

			ctx := context.Background()
			duration := 100 * time.Millisecond
			metrics := audit.PrivilegeMetrics{
				ElevationCount: 2,
				TotalDuration:  50 * time.Millisecond,
			}

			auditLogger.LogPrivilegedExecution(ctx, tt.cmd, tt.result, duration, metrics)

			logOutput := buf.String()
			assert.Contains(t, logOutput, tt.expectLogMsg)
			assert.Contains(t, logOutput, "audit_type")
			assert.Contains(t, logOutput, "privileged_execution")
			assert.Contains(t, logOutput, tt.cmd.Name)
			assert.Contains(t, logOutput, tt.cmd.Cmd)
		})
	}
}

func TestLogger_LogPrivilegeEscalation(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	auditLogger := audit.NewAuditLogger(logger)

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
			auditLogger := audit.NewAuditLogger(logger)

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
