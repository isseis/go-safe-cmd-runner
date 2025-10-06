package runnertypes

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReservedEnvPrefixError(t *testing.T) {
	tests := []struct {
		name        string
		varName     string
		prefix      string
		expectedMsg string
	}{
		{
			name:        "simple reserved prefix error",
			varName:     "__RUNNER_CUSTOM",
			prefix:      "__RUNNER_",
			expectedMsg: `environment variable "__RUNNER_CUSTOM" uses reserved prefix "__RUNNER_"; this prefix is reserved for automatically generated variables`,
		},
		{
			name:        "datetime variable error",
			varName:     "__RUNNER_DATETIME",
			prefix:      "__RUNNER_",
			expectedMsg: `environment variable "__RUNNER_DATETIME" uses reserved prefix "__RUNNER_"; this prefix is reserved for automatically generated variables`,
		},
		{
			name:        "PID variable error",
			varName:     "__RUNNER_PID",
			prefix:      "__RUNNER_",
			expectedMsg: `environment variable "__RUNNER_PID" uses reserved prefix "__RUNNER_"; this prefix is reserved for automatically generated variables`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewReservedEnvPrefixError(tt.varName, tt.prefix)

			// Check error message
			assert.Equal(t, tt.expectedMsg, err.Error())

			// Check VarName and Prefix fields
			assert.Equal(t, tt.varName, err.VarName)
			assert.Equal(t, tt.prefix, err.Prefix)
		})
	}
}

func TestReservedEnvPrefixError_Is(t *testing.T) {
	err1 := NewReservedEnvPrefixError("__RUNNER_CUSTOM", "__RUNNER_")
	err2 := NewReservedEnvPrefixError("__RUNNER_OTHER", "__RUNNER_")
	otherErr := errors.New("different error")

	// Test with errors.Is for type checking
	var target *ReservedEnvPrefixError
	assert.True(t, errors.Is(err1, target))
	assert.True(t, errors.Is(err2, target))
	assert.False(t, errors.Is(otherErr, target))

	// Test with errors.As for extracting the error
	var gotErr *ReservedEnvPrefixError
	assert.True(t, errors.As(err1, &gotErr))
	assert.Equal(t, err1, gotErr)
	assert.False(t, errors.As(otherErr, &gotErr))
}

func TestReservedEnvPrefixError_Unwrap(t *testing.T) {
	err := NewReservedEnvPrefixError("__RUNNER_CUSTOM", "__RUNNER_")

	// Unwrap should return nil as there's no underlying error
	assert.Nil(t, err.Unwrap())
}
