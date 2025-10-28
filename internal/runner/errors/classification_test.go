package errors

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, ErrorTypeConfigVerification, result.Type)

	// Verify severity
	assert.Equal(t, ErrorSeverityCritical, result.Severity)

	// Verify message
	assert.Equal(t, testMessage, result.Message)

	// Verify file path
	assert.Equal(t, testFilePath, result.FilePath)

	// Verify component is always "verification"
	assert.Equal(t, "verification", result.Component)

	// Verify cause error
	assert.Equal(t, testErr, result.Cause)

	// Verify timestamp is reasonable (between before and after)
	assert.True(t, !result.Timestamp.Before(beforeTime) && !result.Timestamp.After(afterTime))
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
	assert.True(t, errors.Is(wrappedErr.Cause, originalErr))

	// Verify the error chain is maintained
	assert.NotNil(t, wrappedErr.Cause)

	assert.Equal(t, originalErr.Error(), wrappedErr.Cause.Error())
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

			assert.Equal(t, tt.errorType, result.Type)
			assert.Equal(t, tt.severity, result.Severity)
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

	assert.Nil(t, result.Cause)

	// Other fields should still be set correctly
	assert.Equal(t, "error without cause", result.Message)
}
