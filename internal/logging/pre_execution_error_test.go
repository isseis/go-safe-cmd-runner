package logging

import (
	"errors"
	"fmt"
	"testing"
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
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
	if !err.Is(target) {
		t.Error("Is() should return true for PreExecutionError type")
	}

	// Should return false for other error types
	if err.Is(errOtherError) {
		t.Error("Is() should return false for other error types")
	}
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
	if !errors.As(originalErr, &target) {
		t.Fatal("errors.As() should return true for direct PreExecutionError")
	}

	// Verify the extracted error has the same content
	if target.Type != originalErr.Type {
		t.Errorf("Type = %q, want %q", target.Type, originalErr.Type)
	}
	if target.Message != originalErr.Message {
		t.Errorf("Message = %q, want %q", target.Message, originalErr.Message)
	}
	if target.Component != originalErr.Component {
		t.Errorf("Component = %q, want %q", target.Component, originalErr.Component)
	}
	if target.RunID != originalErr.RunID {
		t.Errorf("RunID = %q, want %q", target.RunID, originalErr.RunID)
	}
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
	if !errors.As(wrappedErr, &target) {
		t.Fatal("errors.As() should return true for wrapped PreExecutionError")
	}

	// Verify the extracted error has the same content as the original
	if target.Type != originalErr.Type {
		t.Errorf("Type = %q, want %q", target.Type, originalErr.Type)
	}
	if target.Message != originalErr.Message {
		t.Errorf("Message = %q, want %q", target.Message, originalErr.Message)
	}
	if target.Component != originalErr.Component {
		t.Errorf("Component = %q, want %q", target.Component, originalErr.Component)
	}
	if target.RunID != originalErr.RunID {
		t.Errorf("RunID = %q, want %q", target.RunID, originalErr.RunID)
	}
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
	if !errors.As(level3, &target) {
		t.Fatal("errors.As() should return true for multiply wrapped PreExecutionError")
	}

	// Verify the extracted error is the original one
	if target.Type != originalErr.Type {
		t.Errorf("Type = %q, want %q", target.Type, originalErr.Type)
	}
	if target.Message != originalErr.Message {
		t.Errorf("Message = %q, want %q", target.Message, originalErr.Message)
	}
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
			if errors.As(tt.err, &target) {
				t.Errorf("errors.As() should return false for %s", tt.name)
			}
			if target != nil {
				t.Errorf("target should remain nil for %s, got %v", tt.name, target)
			}
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
	if errors.As(err, &wrongTarget) {
		t.Error("errors.As() should return false for wrong target type")
	}
	if wrongTarget != nil {
		t.Error("wrong target should remain nil")
	}
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
	if !errors.As(err, &preExecErr) {
		t.Fatal("Should be able to extract PreExecutionError from wrapped error")
	}

	// Verify all fields are correctly extracted
	if preExecErr.Type != ErrorTypeConfigParsing {
		t.Errorf("Type = %q, want %q", preExecErr.Type, ErrorTypeConfigParsing)
	}
	if preExecErr.Message != "integration test error" {
		t.Errorf("Message = %q, want %q", preExecErr.Message, "integration test error")
	}
	if preExecErr.Component != "integration" {
		t.Errorf("Component = %q, want %q", preExecErr.Component, "integration")
	}
	if preExecErr.RunID != "integration-test-id" {
		t.Errorf("RunID = %q, want %q", preExecErr.RunID, "integration-test-id")
	}
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

			if found != tt.shouldFind {
				t.Errorf("errors.As() = %v, want %v. %s", found, tt.shouldFind, tt.description)
			}

			if tt.shouldFind && preExecErr == nil {
				t.Error("preExecErr should not be nil when found")
			}

			if !tt.shouldFind && preExecErr != nil {
				t.Error("preExecErr should be nil when not found")
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
