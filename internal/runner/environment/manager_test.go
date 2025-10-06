package environment

import (
	"errors"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerValidateUserEnvNames(t *testing.T) {
	tests := []struct {
		name         string
		envNames     []string
		wantErr      bool
		errType      error
		invalidNames []string // Expected invalid names in error
	}{
		{
			name: "valid environment variable names",
			envNames: []string{
				"PATH",
				"HOME",
				"CUSTOM",
				"GO_PATH",
				"__CUSTOM", // Not using the reserved prefix
			},
			wantErr: false,
		},
		{
			name:     "empty environment names",
			envNames: []string{},
			wantErr:  false,
		},
		{
			name: "reserved prefix DATETIME",
			envNames: []string{
				"PATH",
				"__RUNNER_DATETIME",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_DATETIME"},
		},
		{
			name: "reserved prefix PID",
			envNames: []string{
				"PATH",
				"__RUNNER_PID",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_PID"},
		},
		{
			name: "reserved prefix custom variable",
			envNames: []string{
				"PATH",
				"__RUNNER_CUSTOM",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_CUSTOM"},
		},
		{
			name: "multiple reserved prefix violations",
			envNames: []string{
				"__RUNNER_VAR1",
				"__RUNNER_VAR2",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_VAR1", "__RUNNER_VAR2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(nil)
			err := manager.ValidateUserEnvNames(tt.envNames)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errType), "error type mismatch")

				// Check that the error contains the reserved prefix
				var rpe *runnertypes.ReservedEnvPrefixError
				if errors.As(err, &rpe) {
					assert.Equal(t, AutoEnvPrefix, rpe.Prefix)
					// Check that the error references one of the invalid names
					found := false
					for _, invalidName := range tt.invalidNames {
						if rpe.VarName == invalidName {
							found = true
							break
						}
					}
					assert.True(t, found, "error should reference one of the invalid variables: %v, got: %s", tt.invalidNames, rpe.VarName)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManagerBuildEnv(t *testing.T) {
	// Fixed time for testing: 2025-10-05 14:30:22.123456789 UTC
	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	fixedClock := func() time.Time { return fixedTime }

	tests := []struct {
		name        string
		userEnv     map[string]string
		clock       Clock
		wantAutoEnv map[string]string
		wantErr     bool
	}{
		{
			name: "merge auto and user env",
			userEnv: map[string]string{
				"PATH":   "/usr/bin",
				"HOME":   "/home/user",
				"CUSTOM": "value",
			},
			clock: fixedClock,
			wantAutoEnv: map[string]string{
				"__RUNNER_DATETIME": "202510051430.123",
				// PID is dynamic, checked separately
			},
			wantErr: false,
		},
		{
			name:    "empty user env",
			userEnv: map[string]string{},
			clock:   fixedClock,
			wantAutoEnv: map[string]string{
				"__RUNNER_DATETIME": "202510051430.123",
			},
			wantErr: false,
		},
		{
			name: "user env with many variables",
			userEnv: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
				"VAR3": "value3",
			},
			clock: fixedClock,
			wantAutoEnv: map[string]string{
				"__RUNNER_DATETIME": "202510051430.123",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(tt.clock)
			result, err := manager.BuildEnv(tt.userEnv)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			// Check that all auto env variables are present
			for key, expectedValue := range tt.wantAutoEnv {
				actualValue, ok := result[key]
				assert.True(t, ok, "auto env variable %q should be present", key)
				assert.Equal(t, expectedValue, actualValue, "auto env variable %q value mismatch", key)
			}

			// Check that __RUNNER_PID is present and is a valid number
			pid, ok := result["__RUNNER_PID"]
			assert.True(t, ok, "__RUNNER_PID should be present")
			assert.Regexp(t, `^\d+$`, pid, "__RUNNER_PID should be a number")

			// Check that all user env variables are present
			for key, expectedValue := range tt.userEnv {
				actualValue, ok := result[key]
				assert.True(t, ok, "user env variable %q should be present", key)
				assert.Equal(t, expectedValue, actualValue, "user env variable %q value mismatch", key)
			}

			// Check that the result contains both auto and user env
			expectedCount := len(tt.wantAutoEnv) + 1 + len(tt.userEnv) // +1 for PID
			assert.Equal(t, expectedCount, len(result), "environment map size mismatch")
		})
	}
}

func TestManagerBuildEnvWithDefaultClock(t *testing.T) {
	manager := NewManager(nil)
	userEnv := map[string]string{
		"PATH": "/usr/bin",
	}

	result, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check that auto env variables are present with valid formats
	datetime, ok := result["__RUNNER_DATETIME"]
	assert.True(t, ok, "__RUNNER_DATETIME should be present")
	assert.Regexp(t, `^\d{12}\.\d{3}$`, datetime, "__RUNNER_DATETIME should match format YYYYMMDDHHMM.mmm")

	pid, ok := result["__RUNNER_PID"]
	assert.True(t, ok, "__RUNNER_PID should be present")
	assert.Regexp(t, `^\d+$`, pid, "__RUNNER_PID should be a number")

	// Check user env
	path, ok := result["PATH"]
	assert.True(t, ok, "PATH should be present")
	assert.Equal(t, "/usr/bin", path)
}

func TestManagerBuildEnvNoConflict(t *testing.T) {
	// This test verifies that user env cannot override auto env
	// However, validation should catch this before BuildEnv is called
	// BuildEnv assumes userEnv has already been validated

	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	fixedClock := func() time.Time { return fixedTime }

	manager := NewManager(fixedClock)

	// User env with regular variables (no reserved prefix)
	userEnv := map[string]string{
		"PATH": "/usr/bin",
	}

	result, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)

	// Auto env should always be present with correct values
	assert.Equal(t, "202510051430.123", result["__RUNNER_DATETIME"])
	assert.Regexp(t, `^\d+$`, result["__RUNNER_PID"])

	// User env should also be present
	assert.Equal(t, "/usr/bin", result["PATH"])
}
