package errors

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLogCriticalToStderr_Output(t *testing.T) {
	// Capture stderr output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	testErr := errors.New("test critical error")
	testComponent := "test-component"
	testMessage := "critical test message"

	LogCriticalToStderr(testComponent, testMessage, testErr)

	// Close writer and restore stderr
	w.Close()
	os.Stderr = oldStderr

	// Read captured output
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected elements
	if !strings.Contains(output, "CRITICAL:") {
		t.Error("Output does not contain 'CRITICAL:'")
	}

	if !strings.Contains(output, testMessage) {
		t.Errorf("Output does not contain test message: %q", testMessage)
	}

	if !strings.Contains(output, testComponent) {
		t.Errorf("Output does not contain component: %q", testComponent)
	}

	if !strings.Contains(output, testErr.Error()) {
		t.Errorf("Output does not contain error: %v", testErr)
	}

	// Verify timestamp format (ISO 8601 with timezone)
	if !strings.Contains(output, "[20") {
		t.Error("Output does not contain timestamp starting with '[20'")
	}
}

func TestLogClassifiedError_AllSeverities(t *testing.T) {
	tests := []struct {
		name         string
		severity     ErrorSeverity
		shouldStderr bool
	}{
		{
			name:         "critical severity",
			severity:     ErrorSeverityCritical,
			shouldStderr: true,
		},
		{
			name:         "warning severity",
			severity:     ErrorSeverityWarning,
			shouldStderr: false,
		},
		{
			name:         "info severity",
			severity:     ErrorSeverityInfo,
			shouldStderr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stderr to check if critical errors are logged there
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			classifiedErr := ClassifyVerificationError(
				ErrorTypeConfigVerification,
				tt.severity,
				"test message",
				"/test/path",
				errors.New("test error"),
			)

			LogClassifiedError(classifiedErr)

			// Close writer and restore stderr
			w.Close()
			os.Stderr = oldStderr

			// Read captured output
			buf := new(bytes.Buffer)
			buf.ReadFrom(r)
			output := buf.String()

			if tt.shouldStderr {
				if output == "" {
					t.Error("Critical error should write to stderr, but got empty output")
				}
				if !strings.Contains(output, "CRITICAL:") {
					t.Error("Critical error output should contain 'CRITICAL:'")
				}
			} else if strings.Contains(output, "CRITICAL:") {
				// Warning and Info should not write to stderr (they use slog)
				t.Errorf("%s should not write 'CRITICAL:' to stderr", tt.name)
			}
		})
	}
}

func TestLogClassifiedError_WithStructuredFields(t *testing.T) {
	testErr := errors.New("structured test error")
	testMessage := "test structured message"
	testFilePath := "/test/structured/path"

	classifiedErr := ClassifyVerificationError(
		ErrorTypeConfigVerification,
		ErrorSeverityCritical,
		testMessage,
		testFilePath,
		testErr,
	)

	// Verify structured fields are properly set
	if classifiedErr.Message != testMessage {
		t.Errorf("Message = %q, want %q", classifiedErr.Message, testMessage)
	}

	if classifiedErr.FilePath != testFilePath {
		t.Errorf("FilePath = %q, want %q", classifiedErr.FilePath, testFilePath)
	}

	if classifiedErr.Cause != testErr {
		t.Errorf("Cause = %v, want %v", classifiedErr.Cause, testErr)
	}

	if classifiedErr.Component != "verification" {
		t.Errorf("Component = %q, want %q", classifiedErr.Component, "verification")
	}

	if classifiedErr.Type != ErrorTypeConfigVerification {
		t.Errorf("Type = %v, want %v", classifiedErr.Type, ErrorTypeConfigVerification)
	}

	if classifiedErr.Severity != ErrorSeverityCritical {
		t.Errorf("Severity = %v, want %v", classifiedErr.Severity, ErrorSeverityCritical)
	}

	// Verify timestamp is reasonable
	now := time.Now()
	if classifiedErr.Timestamp.After(now) {
		t.Errorf("Timestamp %v is after current time %v", classifiedErr.Timestamp, now)
	}

	// Timestamp should be very recent (within last second)
	diff := now.Sub(classifiedErr.Timestamp)
	if diff > time.Second {
		t.Errorf("Timestamp is too old: %v ago", diff)
	}
}
