package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSecurityLogger(t *testing.T) {
	logger := NewSecurityLogger()
	require.NotNil(t, logger, "NewSecurityLogger returned nil")
	assert.NotNil(t, logger.logger, "logger not initialized")
}

func TestNewSecurityLoggerWithLogger(t *testing.T) {
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))

	logger := NewSecurityLoggerWithLogger(customLogger)
	require.NotNil(t, logger, "NewSecurityLoggerWithLogger returned nil")
	assert.Equal(t, customLogger, logger.logger, "custom logger not set correctly")
}

func TestSecurityLogger_LogMethods(t *testing.T) {
	tests := []struct {
		name           string
		logFunc        func(*SecurityLogger)
		expectedLevel  string
		expectedFields map[string]any
		logLevel       slog.Level
	}{
		{
			name: "LogUnlimitedExecution",
			logFunc: func(sl *SecurityLogger) {
				sl.LogUnlimitedExecution("test-command", "testuser")
			},
			expectedLevel: "WARN",
			expectedFields: map[string]any{
				"command":        "test-command",
				"user":           "testuser",
				"timeout":        "unlimited",
				"security_event": "unlimited_execution_start",
			},
			logLevel: slog.LevelWarn,
		},
		{
			name: "LogLongRunningProcess",
			logFunc: func(sl *SecurityLogger) {
				sl.LogLongRunningProcess("long-command", 15*time.Minute, 12345)
			},
			expectedLevel: "WARN",
			expectedFields: map[string]any{
				"command":          "long-command",
				"pid":              float64(12345), // JSON numbers are float64
				"duration_minutes": float64(15),
				"security_event":   "long_running_process",
			},
			logLevel: slog.LevelWarn,
		},
		{
			name: "LogTimeoutExceeded",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutExceeded("timeout-command", 300, 67890)
			},
			expectedLevel: "ERROR",
			expectedFields: map[string]any{
				"command":         "timeout-command",
				"pid":             float64(67890),
				"timeout_seconds": float64(300),
				"security_event":  "timeout_exceeded",
			},
			logLevel: slog.LevelError,
		},
		{
			name: "LogTimeoutConfiguration_Unlimited",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutConfiguration("unlimited-command", 0, "command-level")
			},
			expectedLevel: "INFO",
			expectedFields: map[string]any{
				"command": "unlimited-command",
				"timeout": "unlimited",
				"source":  "command-level",
			},
			logLevel: slog.LevelInfo,
		},
		{
			name: "LogTimeoutConfiguration_Limited",
			logFunc: func(sl *SecurityLogger) {
				sl.LogTimeoutConfiguration("limited-command", 120, "global")
			},
			expectedLevel: "DEBUG",
			expectedFields: map[string]any{
				"command":         "limited-command",
				"timeout_seconds": float64(120),
				"source":          "global",
			},
			logLevel: slog.LevelDebug,
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
		})
	}
}
