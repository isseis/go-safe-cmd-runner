package verification

import (
	"errors"
	"testing"
	"time"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ErrHashMismatch    = errors.New("hash mismatch")
	ErrConfigInvalid   = errors.New("config invalid")
	ErrValuesDontMatch = errors.New("values don't match")
	ErrOriginalError   = errors.New("original error")
	ErrDifferentError  = errors.New("different error")
)

func TestError_Error(t *testing.T) {
	testCases := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name: "error with path",
			err: &Error{
				Op:   "VerifyHash",
				Path: "/path/to/config.toml",
				Err:  ErrHashMismatch,
			},
			expected: "/path/to/config.toml: hash mismatch",
		},
		{
			name: "error without path",
			err: &Error{
				Op:  "ValidateConfig",
				Err: ErrConfigInvalid,
			},
			expected: "ValidateConfig failed: config invalid",
		},
		{
			name: "error with all fields",
			err: &Error{
				Op:   "CompareHash",
				Path: "/etc/config.toml",
				Err:  ErrValuesDontMatch,
			},
			expected: "/etc/config.toml: values don't match",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.err.Error())
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	verificationErr := &Error{
		Op:  "TestOperation",
		Err: ErrOriginalError,
	}

	assert.Equal(t, ErrOriginalError, verificationErr.Unwrap())
}

func TestError_Is(t *testing.T) {
	verificationErr := &Error{
		Op:  "TestOperation",
		Err: ErrOriginalError,
	}

	assert.True(t, verificationErr.Is(ErrOriginalError))
	assert.ErrorIs(t, verificationErr, ErrOriginalError)
}

func TestStaticErrors(t *testing.T) {
	// Test that static errors are properly defined
	assert.NotNil(t, ErrVerificationDisabled)
	assert.NotNil(t, ErrHashDirectoryEmpty)
	assert.NotNil(t, ErrHashDirectoryInvalid)
	assert.NotNil(t, ErrConfigNil)
	assert.NotNil(t, ErrSecurityValidatorNotInitialized)

	// Test error messages
	assert.Equal(t, "verification is disabled", ErrVerificationDisabled.Error())
	assert.Equal(t, "hash directory cannot be empty", ErrHashDirectoryEmpty.Error())
	assert.Equal(t, "hash directory is invalid", ErrHashDirectoryInvalid.Error())
	assert.Equal(t, "config cannot be nil", ErrConfigNil.Error())
	assert.Equal(t, "security validator not initialized", ErrSecurityValidatorNotInitialized.Error())
}

// Test SecurityViolationError
func TestSecurityViolationError(t *testing.T) {
	err := &SecurityViolationError{
		Op:      "TestOperation",
		Context: "test context",
	}

	assert.Contains(t, err.Error(), "security violation in TestOperation: test context")
	assert.Contains(t, err.Error(), "at")
}

// Test HashDirectorySecurityError
func TestHashDirectorySecurityError(t *testing.T) {
	err := NewHashDirectorySecurityError(
		"/custom/hash/dir",
		"/usr/local/etc/go-safe-cmd-runner/hashes",
		"production environment requires default directory",
	)

	assert.Equal(t, "/custom/hash/dir", err.RequestedDir)
	assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", err.DefaultDir)
	assert.Equal(t, "production environment requires default directory", err.Reason)

	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "hash directory security violation")
	assert.Contains(t, errorMsg, "/custom/hash/dir")
	assert.Contains(t, errorMsg, "/usr/local/etc/go-safe-cmd-runner/hashes")
	assert.Contains(t, errorMsg, "production environment requires default directory")
}

// Test security constraint validation
func TestValidateProductionConstraints(t *testing.T) {
	t.Run("accepts default hash directory", func(t *testing.T) {
		err := validateProductionConstraints("/usr/local/etc/go-safe-cmd-runner/hashes")
		assert.NoError(t, err)
	})

	t.Run("rejects custom hash directory", func(t *testing.T) {
		err := validateProductionConstraints("/custom/hash/dir")
		require.Error(t, err)

		var hashDirErr *HashDirectorySecurityError
		assert.ErrorAs(t, err, &hashDirErr)
		assert.Equal(t, "/custom/hash/dir", hashDirErr.RequestedDir)
		assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", hashDirErr.DefaultDir)
		assert.Equal(t, "production environment requires default hash directory", hashDirErr.Reason)
	})
}

// Test security constraint validation in manager creation
func TestSecurityConstraintsInManager(t *testing.T) {
	t.Run("production mode with strict security enforces constraints", func(t *testing.T) {
		_, err := newManagerInternal("/custom/dir",
			withFSInternal(commontesting.NewMockFileSystem()),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeProduction),
			withSecurityLevel(SecurityLevelStrict),
		)

		require.Error(t, err)
		var hashDirErr *HashDirectorySecurityError
		assert.ErrorAs(t, err, &hashDirErr)
	})

	t.Run("testing mode with relaxed security allows custom directories", func(t *testing.T) {
		mockFS := commontesting.NewMockFileSystem()
		mockFS.AddDir("/custom", 0o755)
		mockFS.AddDir("/custom/dir", 0o755)

		manager, err := newManagerInternal("/custom/dir",
			withFSInternal(mockFS),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeTesting),
			withSecurityLevel(SecurityLevelRelaxed),
		)

		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.Equal(t, "/custom/dir", manager.hashDir)
	})
}

// TestSecurityViolationErrorAdvanced tests additional SecurityViolationError functionality
func TestSecurityViolationErrorAdvanced(t *testing.T) {
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

// TestHashDirectorySecurityErrorAdvanced tests additional HashDirectorySecurityError functionality
func TestHashDirectorySecurityErrorAdvanced(t *testing.T) {
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

// Additional error constants for complete error testing

// TestErrorStructure tests the general verification Error type structure
func TestErrorStructure(t *testing.T) {
	baseErr := ErrOriginalError

	t.Run("error_with_path", func(t *testing.T) {
		err := &Error{
			Op:   "VerifyHash",
			Path: "/path/to/config.toml",
			Err:  baseErr,
		}

		expectedMessage := "/path/to/config.toml: original error"
		assert.Equal(t, expectedMessage, err.Error())
	})

	t.Run("error_without_path", func(t *testing.T) {
		err := &Error{
			Op:  "ValidateDirectory",
			Err: baseErr,
		}

		expectedMessage := "ValidateDirectory failed: original error"
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

		// Test errors.Is with wrapper
		assert.ErrorIs(t, err, baseErr)
	})

	t.Run("error_interface_compliance", func(t *testing.T) {
		err := &Error{Op: "Test", Err: baseErr}

		// Should implement error interface
		var _ error = err
		assert.NotEmpty(t, err.Error())
	})
}

// TestVerificationErrorStructure tests the VerificationError type
func TestVerificationErrorStructure(t *testing.T) {
	baseErr := ErrConfigInvalid

	t.Run("error_with_group", func(t *testing.T) {
		err := &VerificationError{
			Op:            "group",
			Group:         "test-group",
			Details:       []string{"file1.txt", "file2.txt"},
			TotalFiles:    10,
			VerifiedFiles: 8,
			FailedFiles:   2,
			SkippedFiles:  0,
			Err:           baseErr,
		}

		expectedMessage := "group verification failed for group test-group: 2 of 10 files failed: [file1.txt file2.txt]"
		assert.Equal(t, expectedMessage, err.Error())
		assert.Equal(t, "test-group", err.Group)
		assert.Equal(t, []string{"file1.txt", "file2.txt"}, err.Details)
		assert.Equal(t, 10, err.TotalFiles)
		assert.Equal(t, 8, err.VerifiedFiles)
		assert.Equal(t, 2, err.FailedFiles)
		assert.Equal(t, 0, err.SkippedFiles)
	})

	t.Run("error_without_group", func(t *testing.T) {
		err := &VerificationError{
			Op:            "global",
			Details:       []string{"global_file.txt"},
			TotalFiles:    5,
			VerifiedFiles: 4,
			FailedFiles:   1,
			SkippedFiles:  0,
			Err:           baseErr,
		}

		expectedMessage := "global verification failed: 1 of 5 files failed: [global_file.txt]"
		assert.Equal(t, expectedMessage, err.Error())
		assert.Empty(t, err.Group)
		assert.Equal(t, 5, err.TotalFiles)
		assert.Equal(t, 4, err.VerifiedFiles)
		assert.Equal(t, 1, err.FailedFiles)
		assert.Equal(t, 0, err.SkippedFiles)
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

		// Test errors.Is with wrapper
		assert.ErrorIs(t, err, baseErr)
	})

	t.Run("error_interface_compliance", func(t *testing.T) {
		err := &VerificationError{Op: "test", Err: baseErr}

		// Should implement error interface
		var _ error = err
		assert.NotEmpty(t, err.Error())
	})

	t.Run("error_without_details_includes_underlying_error", func(t *testing.T) {
		err := &VerificationError{
			Op:  "global",
			Err: baseErr,
		}

		// Should include underlying error when Details is empty
		expectedMessage := "global verification failed: config invalid"
		assert.Equal(t, expectedMessage, err.Error())
	})

	t.Run("error_without_details_or_underlying_error", func(t *testing.T) {
		err := &VerificationError{
			Op: "global",
		}

		// Should return base message only
		expectedMessage := "global verification failed"
		assert.Equal(t, expectedMessage, err.Error())
	})
}

// TestPredefinedErrorsComplete tests all predefined error variables
func TestPredefinedErrorsComplete(t *testing.T) {
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
			assert.ErrorIs(t, tc.err, tc.err)

			// Verify it implements error interface
			_ = tc.err
		})
	}
}

// TestResultStructure tests the Result struct
func TestResultStructure(t *testing.T) {
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

// TestFileDetailStructure tests the FileDetail struct
func TestFileDetailStructure(t *testing.T) {
	t.Run("file_detail_creation", func(t *testing.T) {
		testErr := ErrHashMismatch
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

// TestErrorWrappingBehavior tests error wrapping and unwrapping behavior
func TestErrorWrappingBehavior(t *testing.T) {
	t.Run("nested_error_wrapping", func(t *testing.T) {
		// Create a chain of errors
		rootErr := ErrValuesDontMatch

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
		assert.ErrorIs(t, verificationErr, rootErr)
		assert.ErrorIs(t, verificationErr, verifyErr)

		// Test direct unwrapping
		assert.Equal(t, verifyErr, verificationErr.Unwrap())
		assert.Equal(t, rootErr, verifyErr.Unwrap())
	})
}
