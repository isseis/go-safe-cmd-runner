package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/identifier"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSecurityLogger(t *testing.T) {
	logger := NewSecurityLogger()
	require.NotNil(t, logger, "NewSecurityLogger returned nil")
	assert.NotNil(t, logger.logger, "logger not initialized")
}

func TestSecurityLogger_LogMethods(t *testing.T) {
	tests := []struct {
		name           string
		logFunc        func(*SecurityLogger)
		expectedLevel  string
		expectedFields map[string]any
		logLevel       slog.Level
		// cmdName is the typed value the method must put on the command
		// attribute. Capturing it with a resolving handler cannot tell a
		// declared identifier from a plain string, so the raw value is
		// asserted separately below.
		cmdName identifier.Identifier
	}{
		{
			name: "LogUnlimitedExecution",
			logFunc: func(sl *SecurityLogger) {
				sl.LogUnlimitedExecution(identifier.NewIdentifier("test-command"), "testuser")
			},
			expectedLevel: "WARN",
			expectedFields: map[string]any{
				"command":        "test-command",
				"user":           "testuser",
				"timeout":        "unlimited",
				"security_event": "unlimited_execution_start",
			},
			logLevel: slog.LevelWarn,
			cmdName:  identifier.NewIdentifier("test-command"),
		},
		{
			name: "LogLongRunningProcess",
			logFunc: func(sl *SecurityLogger) {
				sl.LogLongRunningProcess(identifier.NewIdentifier("long-command"), 15*time.Minute, 12345)
			},
			expectedLevel: "WARN",
			expectedFields: map[string]any{
				"command":          "long-command",
				"pid":              float64(12345), // JSON numbers are float64
				"duration_minutes": float64(15),
				"security_event":   "long_running_process",
			},
			logLevel: slog.LevelWarn,
			cmdName:  identifier.NewIdentifier("long-command"),
		},
		{
			name: "LogTimeoutExceeded",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutExceeded(identifier.NewIdentifier("timeout-command"), 300, 67890)
			},
			expectedLevel: "ERROR",
			expectedFields: map[string]any{
				"command":         "timeout-command",
				"pid":             float64(67890),
				"timeout_seconds": float64(300),
				"security_event":  "timeout_exceeded",
			},
			logLevel: slog.LevelError,
			cmdName:  identifier.NewIdentifier("timeout-command"),
		},
		{
			name: "LogTimeoutConfiguration_Unlimited",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutConfiguration(identifier.NewIdentifier("unlimited-command"), 0, "command-level")
			},
			expectedLevel: "INFO",
			expectedFields: map[string]any{
				"command": "unlimited-command",
				"timeout": "unlimited",
				"source":  "command-level",
			},
			logLevel: slog.LevelInfo,
			cmdName:  identifier.NewIdentifier("unlimited-command"),
		},
		{
			name: "LogTimeoutConfiguration_Limited",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutConfiguration(identifier.NewIdentifier("limited-command"), 120, "global")
			},
			expectedLevel: "DEBUG",
			expectedFields: map[string]any{
				"command":         "limited-command",
				"timeout_seconds": float64(120),
				"source":          "global",
			},
			logLevel: slog.LevelDebug,
			cmdName:  identifier.NewIdentifier("limited-command"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := &slog.HandlerOptions{
				Level: slog.LevelDebug, // Enable all log levels for testing
			}
			customLogger := slog.New(slog.NewJSONHandler(&buf, opts))
			logger := NewSecurityLoggerWithLogger(customLogger)

			// Execute the log function
			tt.logFunc(logger)

			// Parse the JSON output
			var logEntry map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &logEntry), "Failed to parse JSON log output: %s", buf.String())

			// Verify log level
			level, ok := logEntry["level"].(string)
			assert.True(t, ok, "log level field is not a string")
			assert.Equal(t, tt.expectedLevel, level)

			// Verify expected fields
			for key, expectedValue := range tt.expectedFields {
				actualValue, ok := logEntry[key]
				assert.True(t, ok, "Missing expected field %q in log output", key)
				assert.Equal(t, expectedValue, actualValue, "Field %q mismatch", key)
			}

			// The handler above resolves the identifier to its name, which is
			// also what a plain string would render as. Capture the raw
			// attribute to pin the declared type, since that is what the
			// redaction exemption keys on.
			recorder := tu.NewLogRecorder(nil)
			tt.logFunc(NewSecurityLoggerWithLogger(slog.New(recorder)))
			records := recorder.RecordsAtLevel(tt.logLevel)
			require.Len(t, records, 1)
			records[0].AssertAttrs(t, map[string]any{"command": tt.cmdName})
		})
	}
}

// TestSecurityLogger_IdentifierSurvivesRedaction pins the end-to-end behavior
// of all four methods: the command attribute they emit is a declared
// identifier, so a name that would match the value-based redaction patterns
// appears verbatim instead of being masked.
func TestSecurityLogger_IdentifierSurvivesRedaction(t *testing.T) {
	const commandName = "rotate_api_key"

	tests := []struct {
		name    string
		logFunc func(*SecurityLogger)
		level   slog.Level
	}{
		{
			name: "LogUnlimitedExecution",
			logFunc: func(sl *SecurityLogger) {
				sl.LogUnlimitedExecution(identifier.NewIdentifier(commandName), "testuser")
			},
			level: slog.LevelWarn,
		},
		{
			name: "LogLongRunningProcess",
			logFunc: func(sl *SecurityLogger) {
				sl.LogLongRunningProcess(identifier.NewIdentifier(commandName), time.Minute, 12345)
			},
			level: slog.LevelWarn,
		},
		{
			name: "LogTimeoutExceeded",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutExceeded(identifier.NewIdentifier(commandName), 300, 67890)
			},
			level: slog.LevelError,
		},
		{
			name: "LogTimeoutConfiguration_Unlimited",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutConfiguration(identifier.NewIdentifier(commandName), 0, "command-level")
			},
			level: slog.LevelInfo,
		},
		{
			name: "LogTimeoutConfiguration_Limited",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutConfiguration(identifier.NewIdentifier(commandName), 120, "global")
			},
			level: slog.LevelDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tu.NewLogRecorder(nil)
			handler := redaction.NewRedactingHandler(recorder, nil, nil)

			tt.logFunc(NewSecurityLoggerWithLogger(slog.New(handler)))

			records := recorder.RecordsAtLevel(tt.level)
			require.Len(t, records, 1)
			records[0].AssertAttrs(t, map[string]any{"command": commandName})
		})
	}
}
