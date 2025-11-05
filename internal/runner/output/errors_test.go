package output

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Define static test errors to satisfy linter requirements
var (
	errPathTraversalDetected = errors.New("path traversal detected")
	errPermissionDenied      = errors.New("permission denied")
	errDiskFull              = errors.New("disk full")
	errSizeLimitExceeded     = errors.New("size limit exceeded: 10MB")
	errFailedToRemoveTemp    = errors.New("failed to remove temp file")
	errTestCause             = errors.New("test cause")
)

// Test for CaptureError type
func TestCaptureError(t *testing.T) {
	tests := []struct {
		name     string
		err      CaptureError
		wantType ErrorType
		wantMsg  string
		testFunc func(t *testing.T, err CaptureError)
	}{
		{
			name: "path validation error",
			err: CaptureError{
				Type:  ErrorTypePathValidation,
				Path:  "../../../etc/passwd",
				Phase: PhasePreparation,
				Cause: errPathTraversalDetected,
			},
			wantType: ErrorTypePathValidation,
			wantMsg:  "output capture error during preparation phase: path validation for '../../../etc/passwd': path traversal detected",
			testFunc: func(t *testing.T, err CaptureError) {
				if err.Type != ErrorTypePathValidation {
					t.Errorf("Expected ErrorTypePathValidation, got %v", err.Type)
				}
				if err.Phase != PhasePreparation {
					t.Errorf("Expected PhasePreparation, got %v", err.Phase)
				}
			},
		},
		{
			name: "permission error",
			err: CaptureError{
				Type:  ErrorTypePermission,
				Path:  "/root/protected.txt",
				Phase: PhasePreparation,
				Cause: errPermissionDenied,
			},
			wantType: ErrorTypePermission,
			wantMsg:  "output capture error during preparation phase: permission denied for '/root/protected.txt': permission denied",
			testFunc: func(t *testing.T, err CaptureError) {
				if err.Type != ErrorTypePermission {
					t.Errorf("Expected ErrorTypePermission, got %v", err.Type)
				}
			},
		},
		{
			name: "filesystem error during execution",
			err: CaptureError{
				Type:  ErrorTypeFileSystem,
				Path:  "/tmp/output.txt",
				Phase: PhaseExecution,
				Cause: errDiskFull,
			},
			wantType: ErrorTypeFileSystem,
			wantMsg:  "output capture error during execution phase: filesystem error for '/tmp/output.txt': disk full",
			testFunc: func(t *testing.T, err CaptureError) {
				if err.Type != ErrorTypeFileSystem {
					t.Errorf("Expected ErrorTypeFileSystem, got %v", err.Type)
				}
				if err.Phase != PhaseExecution {
					t.Errorf("Expected PhaseExecution, got %v", err.Phase)
				}
			},
		},
		{
			name: "size limit exceeded",
			err: CaptureError{
				Type:  ErrorTypeSizeLimit,
				Path:  "/tmp/large-output.txt",
				Phase: PhaseExecution,
				Cause: errSizeLimitExceeded,
			},
			wantType: ErrorTypeSizeLimit,
			wantMsg:  "output capture error during execution phase: size limit exceeded for '/tmp/large-output.txt': size limit exceeded: 10MB",
			testFunc: func(t *testing.T, err CaptureError) {
				if err.Type != ErrorTypeSizeLimit {
					t.Errorf("Expected ErrorTypeSizeLimit, got %v", err.Type)
				}
			},
		},
		{
			name: "cleanup error",
			err: CaptureError{
				Type:  ErrorTypeCleanup,
				Path:  "/tmp/temp-file.tmp",
				Phase: PhaseCleanup,
				Cause: errFailedToRemoveTemp,
			},
			wantType: ErrorTypeCleanup,
			wantMsg:  "output capture error during cleanup phase: cleanup failed for '/tmp/temp-file.tmp': failed to remove temp file",
			testFunc: func(t *testing.T, err CaptureError) {
				if err.Type != ErrorTypeCleanup {
					t.Errorf("Expected ErrorTypeCleanup, got %v", err.Type)
				}
				if err.Phase != PhaseCleanup {
					t.Errorf("Expected PhaseCleanup, got %v", err.Phase)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Error() method
			assert.Equal(t, tt.wantMsg, tt.err.Error(), "Error message mismatch")

			// Test Unwrap() method
			assert.ErrorIs(t, tt.err, tt.err.Cause, "Expected error to wrap the original cause")

			// Run custom test function
			if tt.testFunc != nil {
				tt.testFunc(t, tt.err)
			}
		})
	}
}

// Test error type constants
func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name      string
		errorType ErrorType
		expected  string
	}{
		{"PathValidation", ErrorTypePathValidation, "path validation"},
		{"Permission", ErrorTypePermission, "permission denied"},
		{"FileSystem", ErrorTypeFileSystem, "filesystem error"},
		{"SizeLimit", ErrorTypeSizeLimit, "size limit exceeded"},
		{"Cleanup", ErrorTypeCleanup, "cleanup failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errorType.String() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, tt.errorType.String())
			}
		})
	}
}

// Test execution phase constants
func TestExecutionPhases(t *testing.T) {
	tests := []struct {
		name     string
		phase    ExecutionPhase
		expected string
	}{
		{"Preparation", PhasePreparation, "preparation phase"},
		{"Execution", PhaseExecution, "execution phase"},
		{"Finalization", PhaseFinalization, "finalization phase"},
		{"Cleanup", PhaseCleanup, "cleanup phase"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.phase.String() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, tt.phase.String())
			}
		})
	}
}

// Test if errors satisfy the error interface
func TestCaptureErrorInterface(t *testing.T) {
	err := CaptureError{
		Type:  ErrorTypePathValidation,
		Path:  "/test/path",
		Phase: PhasePreparation,
		Cause: errTestCause,
	}

	// Test that it implements error interface
	var _ error = err

	// Test that it implements fmt.Wrapper interface
	assert.ErrorIs(t, err, err.Cause, "Expected Unwrap() to return the original cause")
}
