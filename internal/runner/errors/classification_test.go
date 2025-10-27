package errors

import (
	"errors"
	"testing"
	"time"
)

func TestClassifyVerificationError_AllFields(t *testing.T) {
	testErr := errors.New("test error")
	testFilePath := "/path/to/test/file"
	testMessage := "verification failed"

	beforeTime := time.Now()
	result := ClassifyVerificationError(
		ErrorTypeConfigVerification,
		ErrorSeverityCritical,
		testMessage,
		testFilePath,
		testErr,
	)
	afterTime := time.Now()

	// Verify error type
	if result.Type != ErrorTypeConfigVerification {
		t.Errorf("Type = %v, want %v", result.Type, ErrorTypeConfigVerification)
	}

	// Verify severity
	if result.Severity != ErrorSeverityCritical {
		t.Errorf("Severity = %v, want %v", result.Severity, ErrorSeverityCritical)
	}

	// Verify message
	if result.Message != testMessage {
		t.Errorf("Message = %q, want %q", result.Message, testMessage)
	}

	// Verify file path
	if result.FilePath != testFilePath {
		t.Errorf("FilePath = %q, want %q", result.FilePath, testFilePath)
	}

	// Verify component is always "verification"
	if result.Component != "verification" {
		t.Errorf("Component = %q, want %q", result.Component, "verification")
	}

	// Verify cause error
	if result.Cause != testErr {
		t.Errorf("Cause = %v, want %v", result.Cause, testErr)
	}

	// Verify timestamp is reasonable (between before and after)
	if result.Timestamp.Before(beforeTime) || result.Timestamp.After(afterTime) {
		t.Errorf("Timestamp = %v, want between %v and %v", result.Timestamp, beforeTime, afterTime)
	}
}

func TestClassifyVerificationError_WithCause(t *testing.T) {
	originalErr := errors.New("original error")
	wrappedErr := ClassifyVerificationError(
		ErrorTypeEnvironmentVerification,
		ErrorSeverityWarning,
		"wrapped error",
		"/test/path",
		originalErr,
	)

	// Verify errors.Is works correctly
	if !errors.Is(wrappedErr.Cause, originalErr) {
		t.Errorf("errors.Is(wrappedErr.Cause, originalErr) = false, want true")
	}

	// Verify the error chain is maintained
	if wrappedErr.Cause == nil {
		t.Error("Cause is nil, want non-nil")
	}

	if wrappedErr.Cause.Error() != originalErr.Error() {
		t.Errorf("Cause.Error() = %q, want %q", wrappedErr.Cause.Error(), originalErr.Error())
	}
}

func TestClassifyVerificationError_DifferentTypes(t *testing.T) {
	tests := []struct {
		name      string
		errorType ErrorType
		severity  ErrorSeverity
	}{
		{
			name:      "config verification critical",
			errorType: ErrorTypeConfigVerification,
			severity:  ErrorSeverityCritical,
		},
		{
			name:      "environment verification warning",
			errorType: ErrorTypeEnvironmentVerification,
			severity:  ErrorSeverityWarning,
		},
		{
			name:      "hash directory validation info",
			errorType: ErrorTypeHashDirectoryValidation,
			severity:  ErrorSeverityInfo,
		},
		{
			name:      "global verification critical",
			errorType: ErrorTypeGlobalVerification,
			severity:  ErrorSeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyVerificationError(
				tt.errorType,
				tt.severity,
				"test message",
				"/test/path",
				nil,
			)

			if result.Type != tt.errorType {
				t.Errorf("Type = %v, want %v", result.Type, tt.errorType)
			}

			if result.Severity != tt.severity {
				t.Errorf("Severity = %v, want %v", result.Severity, tt.severity)
			}
		})
	}
}

func TestClassifyVerificationError_NilCause(t *testing.T) {
	// Test that nil cause is handled correctly
	result := ClassifyVerificationError(
		ErrorTypeConfigVerification,
		ErrorSeverityCritical,
		"error without cause",
		"/test/path",
		nil,
	)

	if result.Cause != nil {
		t.Errorf("Cause = %v, want nil", result.Cause)
	}

	// Other fields should still be set correctly
	if result.Message != "error without cause" {
		t.Errorf("Message = %q, want %q", result.Message, "error without cause")
	}
}
