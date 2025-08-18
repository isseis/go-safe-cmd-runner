package logging

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Static errors for testing to satisfy err113 linter
var (
	errStandardError = errors.New("standard error")
	errInnerError    = errors.New("inner error")
	errOtherError    = errors.New("other error")
)

func TestPreExecutionError_ErrorMessage(t *testing.T) {
	err := &PreExecutionError{
		Type:      ErrorTypeConfigParsing,
		Message:   "test message",
		Component: "test component",
		RunID:     "test-run-id",
	}

	expected := "config_parsing_failed: test message (component: test component, run_id: test-run-id)"
	assert.Equal(t, expected, err.Error())
}

func TestPreExecutionError_Is(t *testing.T) {
	err := &PreExecutionError{
		Type:      ErrorTypeConfigParsing,
		Message:   "test message",
		Component: "test component",
		RunID:     "test-run-id",
	}

	// Should return true for PreExecutionError type
	var target *PreExecutionError
	assert.True(t, err.Is(target), "Is() should return true for PreExecutionError type")

	// Should return false for other error types
	assert.False(t, err.Is(errOtherError), "Is() should return false for other error types")
}

func TestPreExecutionError_As_Success(t *testing.T) {
	originalErr := &PreExecutionError{
		Type:      ErrorTypeConfigParsing,
		Message:   "test message",
		Component: "test component",
		RunID:     "test-run-id",
	}

	// Test direct error
	var target *PreExecutionError
	require.True(t, errors.As(originalErr, &target), "errors.As() should return true for direct PreExecutionError")

	// Verify the extracted error has the same content
	assert.Equal(t, originalErr.Type, target.Type)
	assert.Equal(t, originalErr.Message, target.Message)
	assert.Equal(t, originalErr.Component, target.Component)
	assert.Equal(t, originalErr.RunID, target.RunID)
}

func TestPreExecutionError_As_WrappedError(t *testing.T) {
	originalErr := &PreExecutionError{
		Type:      ErrorTypeLogFileOpen,
		Message:   "cannot open log file",
		Component: "logging",
		RunID:     "wrapped-test-id",
	}

	// Wrap the error using fmt.Errorf
	wrappedErr := fmt.Errorf("failed to initialize: %w", originalErr)

	// Test wrapped error extraction
	var target *PreExecutionError
	require.True(t, errors.As(wrappedErr, &target), "errors.As() should return true for wrapped PreExecutionError")

	// Verify the extracted error has the same content as the original
	assert.Equal(t, originalErr.Type, target.Type)
	assert.Equal(t, originalErr.Message, target.Message)
	assert.Equal(t, originalErr.Component, target.Component)
	assert.Equal(t, originalErr.RunID, target.RunID)
}

func TestPreExecutionError_As_MultipleWrapping(t *testing.T) {
	originalErr := &PreExecutionError{
		Type:      ErrorTypePrivilegeDrop,
		Message:   "failed to drop privileges",
		Component: "security",
		RunID:     "multi-wrap-test",
	}

	// Multiple levels of wrapping
	level1 := fmt.Errorf("level 1: %w", originalErr)
	level2 := fmt.Errorf("level 2: %w", level1)
	level3 := fmt.Errorf("level 3: %w", level2)

	// Test extraction through multiple wrap levels
	var target *PreExecutionError
	require.True(t, errors.As(level3, &target), "errors.As() should return true for multiply wrapped PreExecutionError")

	// Verify the extracted error is the original one
	assert.Equal(t, originalErr.Type, target.Type)
	assert.Equal(t, originalErr.Message, target.Message)
}

func TestPreExecutionError_As_False_Cases(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "standard error",
			err:  errStandardError,
		},
		{
			name: "wrapped standard error",
			err:  fmt.Errorf("wrapped: %w", errInnerError),
		},
		{
			name: "nil error",
			err:  nil,
		},
		{
			name: "different custom error",
			err:  &customError{message: "custom error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target *PreExecutionError
			assert.False(t, errors.As(tt.err, &target), "errors.As() should return false for %s", tt.name)
			assert.Nil(t, target, "target should remain nil for %s", tt.name)
		})
	}
}

func TestPreExecutionError_As_WrongTargetType(t *testing.T) {
	err := &PreExecutionError{
		Type:      ErrorTypeFileAccess,
		Message:   "file access error",
		Component: "file",
		RunID:     "wrong-type-test",
	}

	// Test with wrong target type (different error type)
	var wrongTarget *customError
	assert.False(t, errors.As(err, &wrongTarget), "errors.As() should return false for wrong target type")
	assert.Nil(t, wrongTarget, "wrong target should remain nil")
}

func TestPreExecutionError_As_Integration(t *testing.T) {
	// Simulate real-world usage similar to main.go
	createError := func() error {
		return &PreExecutionError{
			Type:      ErrorTypeConfigParsing,
			Message:   "integration test error",
			Component: "integration",
			RunID:     "integration-test-id",
		}
	}

	wrappedError := func() error {
		return fmt.Errorf("wrapper context: %w", createError())
	}

	// Test the pattern used in main.go
	err := wrappedError()
	var preExecErr *PreExecutionError
	require.True(t, errors.As(err, &preExecErr), "Should be able to extract PreExecutionError from wrapped error")

	// Verify all fields are correctly extracted
	assert.Equal(t, ErrorTypeConfigParsing, preExecErr.Type)
	assert.Equal(t, "integration test error", preExecErr.Message)
	assert.Equal(t, "integration", preExecErr.Component)
	assert.Equal(t, "integration-test-id", preExecErr.RunID)
}

func TestPreExecutionError_As_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		errorChain  func() error
		shouldFind  bool
		description string
	}{
		{
			name: "deeply nested error chain",
			errorChain: func() error {
				base := &PreExecutionError{Type: ErrorTypeLogFileOpen, Message: "deep", Component: "test", RunID: "deep-test"}
				level1 := fmt.Errorf("level1: %w", base)
				level2 := fmt.Errorf("level2: %w", level1)
				level3 := fmt.Errorf("level3: %w", level2)
				level4 := fmt.Errorf("level4: %w", level3)
				return level4
			},
			shouldFind:  true,
			description: "Should find PreExecutionError in deeply nested chain",
		},
		{
			name: "mixed error types in chain",
			errorChain: func() error {
				preExecErr := &PreExecutionError{Type: ErrorTypePrivilegeDrop, Message: "mixed", Component: "test", RunID: "mixed-test"}
				mixedLevel2 := fmt.Errorf("mixed2: %w", preExecErr)
				return fmt.Errorf("final: %w", mixedLevel2)
			},
			shouldFind:  true,
			description: "Should find PreExecutionError even with mixed error types",
		},
		{
			name: "only custom errors in chain",
			errorChain: func() error {
				customErr1 := &customError{message: "custom1"}
				level2 := fmt.Errorf("level2: %w", customErr1)
				return fmt.Errorf("final: %w", level2)
			},
			shouldFind:  false,
			description: "Should not find PreExecutionError in chain with only custom errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.errorChain()
			var preExecErr *PreExecutionError
			found := errors.As(err, &preExecErr)

			assert.Equal(t, tt.shouldFind, found, "errors.As() result: %s", tt.description)

			if tt.shouldFind {
				assert.NotNil(t, preExecErr, "preExecErr should not be nil when found")
			} else {
				assert.Nil(t, preExecErr, "preExecErr should be nil when not found")
			}
		})
	}
}

// customError is a helper type for testing false cases
type customError struct {
	message string
}

func (e *customError) Error() string {
	return e.message
}
