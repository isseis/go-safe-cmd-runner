package errors_test

import (
	"bytes"
	goerrors "errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/errors"
	"github.com/stretchr/testify/assert"
)

func TestLogCriticalToStderr_Output(t *testing.T) {
	// Capture stderr output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	testErr := goerrors.New("test critical error")
	testComponent := "test-component"
	testMessage := "critical test message"

	errors.LogCriticalToStderr(testComponent, testMessage, testErr)

	// Close writer and restore stderr
	w.Close()
	os.Stderr = oldStderr

	// Read captured output
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected elements
	assert.Contains(t, output, "CRITICAL:", "Output should contain 'CRITICAL:'")
	assert.Contains(t, output, testMessage, "Output should contain test message")
	assert.Contains(t, output, testComponent, "Output should contain component")
	assert.Contains(t, output, testErr.Error(), "Output should contain error")

	// Verify timestamp format (ISO 8601 with timezone)
	assert.Contains(t, output, "[20", "Output should contain timestamp starting with '[20'")
}

func TestLogClassifiedError_AllSeverities(t *testing.T) {
	tests := []struct {
		name         string
		severity     errors.ErrorSeverity
		shouldStderr bool
	}{
		{
			name:         "critical severity",
			severity:     errors.ErrorSeverityCritical,
			shouldStderr: true,
		},
		{
			name:         "warning severity",
			severity:     errors.ErrorSeverityWarning,
			shouldStderr: false,
		},
		{
			name:         "info severity",
			severity:     errors.ErrorSeverityInfo,
			shouldStderr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stderr to check if critical errors are logged there
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			classifiedErr := errors.ClassifyVerificationError(
				errors.ErrorTypeConfigVerification,
				tt.severity,
				"test message",
				"/test/path",
				goerrors.New("test error"),
			)

			errors.LogClassifiedError(classifiedErr)

			// Close writer and restore stderr
			w.Close()
			os.Stderr = oldStderr

			// Read captured output
			buf := new(bytes.Buffer)
			buf.ReadFrom(r)
			output := buf.String()

			if tt.shouldStderr {
				assert.NotEmpty(t, output, "Critical error should write to stderr")
				assert.Contains(t, output, "CRITICAL:", "Critical error output should contain 'CRITICAL:'")
			} else if strings.Contains(output, "CRITICAL:") {
				assert.Fail(t, "should not write 'CRITICAL:' to stderr", tt.name)
			}
		})
	}
}

func TestLogClassifiedError_WithStructuredFields(t *testing.T) {
	testErr := goerrors.New("structured test error")
	testMessage := "test structured message"
	testFilePath := "/test/structured/path"

	classifiedErr := errors.ClassifyVerificationError(
		errors.ErrorTypeConfigVerification,
		errors.ErrorSeverityCritical,
		testMessage,
		testFilePath,
		testErr,
	)

	// Verify structured fields are properly set
	assert.Equal(t, testMessage, classifiedErr.Message)
	assert.Equal(t, testFilePath, classifiedErr.FilePath)
	assert.Equal(t, testErr, classifiedErr.Cause)
	assert.Equal(t, "verification", classifiedErr.Component)
	assert.Equal(t, errors.ErrorTypeConfigVerification, classifiedErr.Type)
	assert.Equal(t, errors.ErrorSeverityCritical, classifiedErr.Severity)

	// Verify timestamp is reasonable
	now := time.Now()
	assert.False(t, classifiedErr.Timestamp.After(now), "Timestamp should not be after current time")

	// Timestamp should be very recent (within last second)
	diff := now.Sub(classifiedErr.Timestamp)
	assert.LessOrEqual(t, diff, time.Second, "Timestamp should not be more than 1 second ago")
}
