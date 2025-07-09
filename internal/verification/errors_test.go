package verification

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
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
				Op:       "CompareHash",
				Path:     "/etc/config.toml",
				Expected: "abc123",
				Actual:   "def456",
				Err:      ErrValuesDontMatch,
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
	assert.True(t, errors.Is(verificationErr, ErrOriginalError))
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
