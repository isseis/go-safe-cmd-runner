package verification

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
			expected: "verification error in VerifyHash for /path/to/config.toml: hash mismatch",
		},
		{
			name: "error without path",
			err: &Error{
				Op:  "ValidateConfig",
				Err: ErrConfigInvalid,
			},
			expected: "verification error in ValidateConfig: config invalid",
		},
		{
			name: "error with all fields",
			err: &Error{
				Op:   "CompareHash",
				Path: "/etc/config.toml",
				Err:  ErrValuesDontMatch,
			},
			expected: "verification error in CompareHash for /etc/config.toml: values don't match",
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
	assert.False(t, verificationErr.Is(ErrDifferentError))
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

// Test ProductionAPIViolationError
func TestProductionAPIViolationError(t *testing.T) {
	err := NewProductionAPIViolationError("NewManagerForTest", "/path/to/test.go", 42)

	assert.Equal(t, "NewManagerForTest", err.APIName)
	assert.Equal(t, "/path/to/test.go", err.CallerFile)
	assert.Equal(t, 42, err.CallerLine)

	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "production API violation")
	assert.Contains(t, errorMsg, "testing API NewManagerForTest")
	assert.Contains(t, errorMsg, "/path/to/test.go:42")
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
		assert.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, "/custom/hash/dir", hashDirErr.RequestedDir)
		assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", hashDirErr.DefaultDir)
		assert.Equal(t, "production environment requires default hash directory", hashDirErr.Reason)
	})
}

// Test security constraint validation in manager creation
func TestSecurityConstraintsInManager(t *testing.T) {
	t.Run("production mode with strict security enforces constraints", func(t *testing.T) {
		_, err := newManagerInternal("/custom/dir",
			withFSInternal(common.NewMockFileSystem()),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeProduction),
			withSecurityLevel(SecurityLevelStrict),
		)

		require.Error(t, err)
		var hashDirErr *HashDirectorySecurityError
		assert.True(t, errors.As(err, &hashDirErr))
	})

	t.Run("testing mode with relaxed security allows custom directories", func(t *testing.T) {
		mockFS := common.NewMockFileSystem()
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
