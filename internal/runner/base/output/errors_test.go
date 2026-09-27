package output

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Define static test errors to satisfy linter requirements
var (
	errPathTraversalDetected = errors.New("path traversal detected")
	errPermissionDenied      = errors.New("permission denied")
	errDiskFull              = errors.New("disk full")
	errFailedToRemoveTemp    = errors.New("failed to remove temp file")
	errTestCause             = errors.New("test cause")
)

// newCaptureErrorForTest builds a CaptureError of a kind that has no production
// constructor, so these tests can pin that those kinds' messages did not
// change. It lives in this test file: production can only build the
// size-limit and filesystem kinds.
func newCaptureErrorForTest(typ ErrorType, path string, phase ExecutionPhase, cause error) *CaptureError {
	return &CaptureError{typ: typ, path: path, phase: phase, cause: cause}
}

// Test for CaptureError type
func TestCaptureError(t *testing.T) {
	tests := []struct {
		name     string
		err      *CaptureError
		wantType ErrorType
		wantMsg  string
		testFunc func(t *testing.T, err *CaptureError)
	}{
		{
			name:     "path validation error",
			err:      newCaptureErrorForTest(ErrorTypePathValidation, "../../../etc/passwd", PhasePreparation, errPathTraversalDetected),
			wantType: ErrorTypePathValidation,
			wantMsg:  "output capture error during preparation phase: path validation for '../../../etc/passwd': path traversal detected",
			testFunc: func(t *testing.T, err *CaptureError) {
				assert.Equal(t, PhasePreparation, err.phase)
			},
		},
		{
			name:     "permission error",
			err:      newCaptureErrorForTest(ErrorTypePermission, "/root/protected.txt", PhasePreparation, errPermissionDenied),
			wantType: ErrorTypePermission,
			wantMsg:  "output capture error during preparation phase: permission denied for '/root/protected.txt': permission denied",
		},
		{
			name:     "filesystem error during execution",
			err:      newFileSystemError("/tmp/output.txt", errDiskFull),
			wantType: ErrorTypeFileSystem,
			wantMsg:  "output capture error during execution phase: filesystem error for '/tmp/output.txt': disk full",
			testFunc: func(t *testing.T, err *CaptureError) {
				assert.Equal(t, PhaseExecution, err.phase)
			},
		},
		{
			name:     "size limit exceeded",
			err:      newSizeLimitError("/tmp/large-output.txt", 10*1024*1024),
			wantType: ErrorTypeSizeLimit,
			wantMsg:  "output capture error during execution phase: output size limit exceeded for '/tmp/large-output.txt' (limit: 10485760 bytes)",
			testFunc: func(t *testing.T, err *CaptureError) {
				assert.Equal(t, int64(10*1024*1024), err.limit)
				assert.Equal(t, 1, strings.Count(err.Error(), "size limit exceeded"),
					"the size-limit message must state the fact exactly once")
				assert.ErrorIs(t, err, ErrOutputSizeExceeded)
			},
		},
		{
			name:     "cleanup error",
			err:      newCaptureErrorForTest(ErrorTypeCleanup, "/tmp/temp-file.tmp", PhaseCleanup, errFailedToRemoveTemp),
			wantType: ErrorTypeCleanup,
			wantMsg:  "output capture error during cleanup phase: cleanup failed for '/tmp/temp-file.tmp': failed to remove temp file",
			testFunc: func(t *testing.T, err *CaptureError) {
				assert.Equal(t, PhaseCleanup, err.phase)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Error() method
			assert.Equal(t, tt.wantMsg, tt.err.Error(), "Error message mismatch")
			assert.Equal(t, tt.wantType, tt.err.typ, "Error type mismatch")

			// Test Unwrap() method
			assert.ErrorIs(t, tt.err, tt.err.cause, "Expected error to wrap the original cause")

			// Run custom test function
			if tt.testFunc != nil {
				tt.testFunc(t, tt.err)
			}
		})
	}
}

// TestNewSizeLimitErrorPanicsOnNonPositiveLimit pins that the only constructor
// of a size-limit error rejects a non-positive limit, so a limit in the
// message always bounded the write.
func TestNewSizeLimitErrorPanicsOnNonPositiveLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
	}{
		{name: "zero limit", limit: 0},
		{name: "negative limit", limit: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.PanicsWithValue(t,
				fmt.Sprintf("newSizeLimitError: limit must be positive, got %d", tt.limit),
				func() { _ = newSizeLimitError("/tmp/out", tt.limit) })
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
			assert.Equal(t, tt.expected, tt.errorType.String())
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
			assert.Equal(t, tt.expected, tt.phase.String())
		})
	}
}

// Test if errors satisfy the error interface
func TestCaptureErrorInterface(t *testing.T) {
	err := newCaptureErrorForTest(ErrorTypePathValidation, "/test/path", PhasePreparation, errTestCause)

	// Test that it implements error interface
	var _ error = err

	// Test that it implements fmt.Wrapper interface
	assert.ErrorIs(t, err, err.cause, "Expected Unwrap() to return the original cause")
}
