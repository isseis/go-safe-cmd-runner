package logging

import (
	"bytes"
	"log/slog"
	"testing"
	"time"
)

func TestNewSecurityLogger(t *testing.T) {
	logger := NewSecurityLogger()
	if logger == nil {
		t.Fatal("NewSecurityLogger returned nil")
	}
	if logger.logger == nil {
		t.Error("logger not initialized")
	}
}

func TestNewSecurityLoggerWithLogger(t *testing.T) {
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))

	logger := NewSecurityLoggerWithLogger(customLogger)
	if logger == nil {
		t.Fatal("NewSecurityLoggerWithLogger returned nil")
	}
	if logger.logger != customLogger {
		t.Error("custom logger not set correctly")
	}
}

func TestSecurityLogger_LogUnlimitedExecution(t *testing.T) {
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))
	logger := NewSecurityLoggerWithLogger(customLogger)

	logger.LogUnlimitedExecution("test-command", "testuser")

	output := buf.String()
	if output == "" {
		t.Error("LogUnlimitedExecution produced no output")
	}

	// Check that the output contains expected fields
	expectedFields := []string{"test-command", "testuser", "unlimited"}
	for _, field := range expectedFields {
		if !containsString(output, field) {
			t.Errorf("LogUnlimitedExecution output missing expected field: %s", field)
		}
	}
}

func TestSecurityLogger_LogLongRunningProcess(t *testing.T) {
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))
	logger := NewSecurityLoggerWithLogger(customLogger)

	duration := 15 * time.Minute
	logger.LogLongRunningProcess("long-command", duration, 12345)

	output := buf.String()
	if output == "" {
		t.Error("LogLongRunningProcess produced no output")
	}

	// Check that the output contains expected fields
	expectedFields := []string{"long-command", "12345", "15"}
	for _, field := range expectedFields {
		if !containsString(output, field) {
			t.Errorf("LogLongRunningProcess output missing expected field: %s", field)
		}
	}
}

func TestSecurityLogger_LogTimeoutExceeded(t *testing.T) {
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))
	logger := NewSecurityLoggerWithLogger(customLogger)

	logger.LogTimeoutExceeded("timeout-command", 300, 67890)

	output := buf.String()
	if output == "" {
		t.Error("LogTimeoutExceeded produced no output")
	}

	// Check that the output contains expected fields
	expectedFields := []string{"timeout-command", "67890", "300"}
	for _, field := range expectedFields {
		if !containsString(output, field) {
			t.Errorf("LogTimeoutExceeded output missing expected field: %s", field)
		}
	}
}

func TestSecurityLogger_LogTimeoutConfiguration_Unlimited(t *testing.T) {
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))
	logger := NewSecurityLoggerWithLogger(customLogger)

	logger.LogTimeoutConfiguration("unlimited-command", 0, "command-level")

	output := buf.String()
	if output == "" {
		t.Error("LogTimeoutConfiguration produced no output")
	}

	// Check that the output contains expected fields
	expectedFields := []string{"unlimited-command", "unlimited", "command-level"}
	for _, field := range expectedFields {
		if !containsString(output, field) {
			t.Errorf("LogTimeoutConfiguration output missing expected field: %s", field)
		}
	}
}

func TestSecurityLogger_LogTimeoutConfiguration_Limited(t *testing.T) {
	var buf bytes.Buffer
	// Use a handler with DEBUG level enabled
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	customLogger := slog.New(slog.NewTextHandler(&buf, opts))
	logger := NewSecurityLoggerWithLogger(customLogger)

	logger.LogTimeoutConfiguration("limited-command", 120, "global")

	output := buf.String()
	if output == "" {
		t.Error("LogTimeoutConfiguration produced no output")
	}

	// Check that the output contains expected fields
	expectedFields := []string{"limited-command", "120", "global"}
	for _, field := range expectedFields {
		if !containsString(output, field) {
			t.Errorf("LogTimeoutConfiguration output missing expected field: %s", field)
		}
	}
}

// Helper function to check if a string contains a substring
func containsString(str, substr string) bool {
	return bytes.Contains([]byte(str), []byte(substr))
}
