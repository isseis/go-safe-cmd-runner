//go:build test

package verification

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecurityViolationErrorComplete tests the base SecurityViolationError type
func TestSecurityViolationErrorComplete(t *testing.T) {
	t.Run("error_message_format", func(t *testing.T) {
		testTime := time.Date(2025, 9, 15, 12, 0, 0, 0, time.UTC)
		err := &SecurityViolationError{
			Op:      "TestOperation",
			Context: "test security violation",
			Time:    testTime,
		}

		expectedMessage := "security violation in TestOperation: test security violation (at 2025-09-15T12:00:00Z)"
		assert.Equal(t, expectedMessage, err.Error())
	})
}

// TestProductionAPIViolationErrorComplete tests the production API violation error
func TestProductionAPIViolationErrorComplete(t *testing.T) {
	t.Run("creation_and_error_message", func(t *testing.T) {
		err := NewProductionAPIViolationError("NewManagerForTest", "/path/to/file.go", 42)

		// Verify fields are set correctly
		assert.Equal(t, "APIViolation", err.Op)
		assert.Equal(t, "NewManagerForTest", err.APIName)
		assert.Equal(t, "/path/to/file.go", err.CallerFile)
		assert.Equal(t, 42, err.CallerLine)
		assert.Contains(t, err.Context, "testing API NewManagerForTest called from production code")
		assert.False(t, err.Time.IsZero())

		// Verify error message format
		errorMsg := err.Error()
		assert.Contains(t, errorMsg, "production API violation")
		assert.Contains(t, errorMsg, "NewManagerForTest")
		assert.Contains(t, errorMsg, "/path/to/file.go:42")
	})

	t.Run("error_interface_compliance", func(t *testing.T) {
		err := NewProductionAPIViolationError("TestAPI", "test.go", 1)

		// Should implement error interface
		var _ error = err
		assert.NotEmpty(t, err.Error())
	})
}

// TestHashDirectorySecurityErrorComplete tests the hash directory security error
func TestHashDirectorySecurityErrorComplete(t *testing.T) {
	t.Run("creation_and_error_message", func(t *testing.T) {
		err := NewHashDirectorySecurityError(
			"/custom/hash/dir",
			"/usr/local/etc/go-safe-cmd-runner/hashes",
			"custom directories not allowed in production",
		)

		// Verify fields are set correctly
		assert.Equal(t, "HashDirectoryValidation", err.Op)
		assert.Equal(t, "/custom/hash/dir", err.RequestedDir)
		assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", err.DefaultDir)
		assert.Equal(t, "custom directories not allowed in production", err.Reason)
		assert.Contains(t, err.Context, "custom hash directory rejected")
		assert.False(t, err.Time.IsZero())

		// Verify error message format
		errorMsg := err.Error()
		assert.Contains(t, errorMsg, "hash directory security violation")
		assert.Contains(t, errorMsg, "/custom/hash/dir")
		assert.Contains(t, errorMsg, "/usr/local/etc/go-safe-cmd-runner/hashes")
		assert.Contains(t, errorMsg, "custom directories not allowed in production")
	})

	t.Run("error_interface_compliance", func(t *testing.T) {
		err := NewHashDirectorySecurityError("req", "def", "reason")

		// Should implement error interface
		var _ error = err
		assert.NotEmpty(t, err.Error())
	})
}

// Test error constants for consistent error testing
var (
	errTestUnderlying   = errors.New("underlying error")
	errTestDifferent    = errors.New("different error")
	errTestHashMismatch = errors.New("hash mismatch")
	errTestRootCause    = errors.New("root cause")
	errTestVerifyFailed = errors.New("verification failed")
)

// TestError tests the general verification Error type
func TestError(t *testing.T) {
	baseErr := errTestUnderlying

	t.Run("error_with_path", func(t *testing.T) {
		err := &Error{
			Op:   "VerifyHash",
			Path: "/path/to/config.toml",
			Err:  baseErr,
		}

		expectedMessage := "verification error in VerifyHash for /path/to/config.toml: underlying error"
		assert.Equal(t, expectedMessage, err.Error())
	})

	t.Run("error_without_path", func(t *testing.T) {
		err := &Error{
			Op:  "ValidateDirectory",
			Err: baseErr,
		}

		expectedMessage := "verification error in ValidateDirectory: underlying error"
		assert.Equal(t, expectedMessage, err.Error())
	})

	t.Run("unwrap_functionality", func(t *testing.T) {
		err := &Error{
			Op:   "TestOp",
			Path: "/test/path",
			Err:  baseErr,
		}

		// Test Unwrap
		assert.Equal(t, baseErr, err.Unwrap())

		// Test Is functionality
		assert.True(t, err.Is(baseErr))
		assert.False(t, err.Is(errTestDifferent))

		// Test errors.Is with wrapper
		assert.True(t, errors.Is(err, baseErr))
	})

	t.Run("error_interface_compliance", func(t *testing.T) {
		err := &Error{Op: "Test", Err: baseErr}

		// Should implement error interface
		var _ error = err
		assert.NotEmpty(t, err.Error())
	})
}

// TestVerificationError tests the VerificationError type
func TestVerificationError(t *testing.T) {
	baseErr := errTestVerifyFailed

	t.Run("error_with_group", func(t *testing.T) {
		err := &VerificationError{
			Op:      "group",
			Group:   "test-group",
			Details: []string{"file1.txt", "file2.txt"},
			Err:     baseErr,
		}

		expectedMessage := "verification error in group for group test-group: verification failed"
		assert.Equal(t, expectedMessage, err.Error())
		assert.Equal(t, "test-group", err.Group)
		assert.Equal(t, []string{"file1.txt", "file2.txt"}, err.Details)
	})

	t.Run("error_without_group", func(t *testing.T) {
		err := &VerificationError{
			Op:      "global",
			Details: []string{"global_file.txt"},
			Err:     baseErr,
		}

		expectedMessage := "verification error in global: verification failed"
		assert.Equal(t, expectedMessage, err.Error())
		assert.Empty(t, err.Group)
	})

	t.Run("unwrap_functionality", func(t *testing.T) {
		err := &VerificationError{
			Op:  "test",
			Err: baseErr,
		}

		// Test Unwrap
		assert.Equal(t, baseErr, err.Unwrap())

		// Test Is functionality
		assert.True(t, err.Is(baseErr))
		assert.False(t, err.Is(errTestDifferent))

		// Test errors.Is with wrapper
		assert.True(t, errors.Is(err, baseErr))
	})

	t.Run("error_interface_compliance", func(t *testing.T) {
		err := &VerificationError{Op: "test", Err: baseErr}

		// Should implement error interface
		var _ error = err
		assert.NotEmpty(t, err.Error())
	})
}

// TestPredefinedErrors tests all predefined error variables
func TestPredefinedErrors(t *testing.T) {
	predefinedErrors := []struct {
		name string
		err  error
	}{
		{"ErrVerificationDisabled", ErrVerificationDisabled},
		{"ErrHashDirectoryEmpty", ErrHashDirectoryEmpty},
		{"ErrHashDirectoryInvalid", ErrHashDirectoryInvalid},
		{"ErrConfigNil", ErrConfigNil},
		{"ErrSecurityValidatorNotInitialized", ErrSecurityValidatorNotInitialized},
		{"ErrGlobalVerificationFailed", ErrGlobalVerificationFailed},
		{"ErrGroupVerificationFailed", ErrGroupVerificationFailed},
		{"ErrPathResolverNotInitialized", ErrPathResolverNotInitialized},
		{"ErrCommandNotFound", ErrCommandNotFound},
	}

	for _, tc := range predefinedErrors {
		t.Run(tc.name, func(t *testing.T) {
			// Verify error is not nil and has a message
			require.NotNil(t, tc.err)
			assert.NotEmpty(t, tc.err.Error())

			// Verify error can be compared with errors.Is
			assert.True(t, errors.Is(tc.err, tc.err))

			// Verify it implements error interface
			_ = tc.err
		})
	}
}

// TestResult tests the Result struct
func TestResult(t *testing.T) {
	t.Run("result_struct_creation", func(t *testing.T) {
		result := Result{
			TotalFiles:    10,
			VerifiedFiles: 8,
			FailedFiles:   []string{"file1.txt", "file2.txt"},
			SkippedFiles:  []string{"file3.txt"},
			Duration:      time.Minute,
		}

		assert.Equal(t, 10, result.TotalFiles)
		assert.Equal(t, 8, result.VerifiedFiles)
		assert.Equal(t, []string{"file1.txt", "file2.txt"}, result.FailedFiles)
		assert.Equal(t, []string{"file3.txt"}, result.SkippedFiles)
		assert.Equal(t, time.Minute, result.Duration)
	})

	t.Run("empty_result", func(t *testing.T) {
		result := Result{}

		assert.Equal(t, 0, result.TotalFiles)
		assert.Equal(t, 0, result.VerifiedFiles)
		assert.Nil(t, result.FailedFiles)
		assert.Nil(t, result.SkippedFiles)
		assert.Equal(t, time.Duration(0), result.Duration)
	})
}

// TestFileDetail tests the FileDetail struct
func TestFileDetail(t *testing.T) {
	t.Run("file_detail_creation", func(t *testing.T) {
		testErr := errTestHashMismatch
		detail := FileDetail{
			Path:         "ls",
			ResolvedPath: "/usr/bin/ls",
			HashMatched:  false,
			Error:        testErr,
			Duration:     time.Millisecond * 100,
		}

		assert.Equal(t, "ls", detail.Path)
		assert.Equal(t, "/usr/bin/ls", detail.ResolvedPath)
		assert.False(t, detail.HashMatched)
		assert.Equal(t, testErr, detail.Error)
		assert.Equal(t, time.Millisecond*100, detail.Duration)
	})

	t.Run("successful_file_detail", func(t *testing.T) {
		detail := FileDetail{
			Path:         "cat",
			ResolvedPath: "/usr/bin/cat",
			HashMatched:  true,
			Error:        nil,
			Duration:     time.Millisecond * 50,
		}

		assert.Equal(t, "cat", detail.Path)
		assert.Equal(t, "/usr/bin/cat", detail.ResolvedPath)
		assert.True(t, detail.HashMatched)
		assert.Nil(t, detail.Error)
		assert.Equal(t, time.Millisecond*50, detail.Duration)
	})
}

// TestErrorWrapping tests error wrapping and unwrapping behavior
func TestErrorWrapping(t *testing.T) {
	t.Run("nested_error_wrapping", func(t *testing.T) {
		// Create a chain of errors
		rootErr := errTestRootCause

		verifyErr := &Error{
			Op:   "VerifyHash",
			Path: "/config.toml",
			Err:  rootErr,
		}

		verificationErr := &VerificationError{
			Op:      "global",
			Group:   "",
			Details: []string{"/config.toml"},
			Err:     verifyErr,
		}

		// Test error chain unwrapping
		assert.True(t, errors.Is(verificationErr, rootErr))
		assert.True(t, errors.Is(verificationErr, verifyErr))

		// Test direct unwrapping
		assert.Equal(t, verifyErr, verificationErr.Unwrap())
		assert.Equal(t, rootErr, verifyErr.Unwrap())
	})

	t.Run("security_error_isolation", func(t *testing.T) {
		// Security errors should not wrap other errors (they are root causes)
		secErr := NewProductionAPIViolationError("TestAPI", "file.go", 1)
		hashErr := NewHashDirectorySecurityError("/custom", "/default", "reason")

		// Security errors should not have Unwrap methods or should return nil
		// (they are designed to be root security violations)
		assert.IsType(t, &ProductionAPIViolationError{}, secErr)
		assert.IsType(t, &HashDirectorySecurityError{}, hashErr)
	})
}
