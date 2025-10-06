package environment

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to extract all ReservedEnvPrefixError from a joined error
func extractReservedEnvPrefixErrors(err error) []*runnertypes.ReservedEnvPrefixError {
	var result []*runnertypes.ReservedEnvPrefixError

	type unwrapper interface {
		Unwrap() []error
	}

	var collect func(error)
	collect = func(e error) {
		if e == nil {
			return
		}

		// Check if this error is a ReservedEnvPrefixError
		var rpe *runnertypes.ReservedEnvPrefixError
		if errors.As(e, &rpe) {
			result = append(result, rpe)
		}

		// Check if this error wraps multiple errors (from errors.Join)
		if u, ok := e.(unwrapper); ok {
			for _, unwrappedErr := range u.Unwrap() {
				collect(unwrappedErr)
			}
		}
	}

	collect(err)
	return result
}

// Helper function to assert that all expected error variables are found
func assertAllErrorVarsFound(t *testing.T, errs []*runnertypes.ReservedEnvPrefixError, expectedVars []string) {
	t.Helper()

	foundVars := make(map[string]bool)
	for _, rpe := range errs {
		assert.Equal(t, AutoEnvPrefix, rpe.Prefix)
		foundVars[rpe.VarName] = true
	}

	assert.Equal(t, len(expectedVars), len(foundVars),
		"Expected %d errors but found %d", len(expectedVars), len(foundVars))

	for _, expectedVar := range expectedVars {
		assert.True(t, foundVars[expectedVar],
			"Expected error for variable %q but it was not found", expectedVar)
	}
}

func TestManagerValidateUserEnvNames(t *testing.T) {
	tests := []struct {
		name         string
		envMap       map[string]string
		wantErr      bool
		errType      error
		invalidNames []string // Expected invalid names in error
	}{
		{
			name: "valid environment variable names",
			envMap: map[string]string{
				"PATH":     "/usr/bin",
				"HOME":     "/home/user",
				"CUSTOM":   "value",
				"GO_PATH":  "/go",
				"__CUSTOM": "value", // Not using the reserved prefix
			},
			wantErr: false,
		},
		{
			name:    "empty environment names",
			envMap:  map[string]string{},
			wantErr: false,
		},
		{
			name: "reserved prefix DATETIME",
			envMap: map[string]string{
				"PATH":              "/usr/bin",
				"__RUNNER_DATETIME": "value",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_DATETIME"},
		},
		{
			name: "reserved prefix PID",
			envMap: map[string]string{
				"PATH":         "/usr/bin",
				"__RUNNER_PID": "value",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_PID"},
		},
		{
			name: "reserved prefix custom variable",
			envMap: map[string]string{
				"PATH":            "/usr/bin",
				"__RUNNER_CUSTOM": "value",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_CUSTOM"},
		},
		{
			name: "multiple reserved prefix violations",
			envMap: map[string]string{
				"__RUNNER_VAR1": "value1",
				"__RUNNER_VAR2": "value2",
			},
			wantErr:      true,
			errType:      &runnertypes.ReservedEnvPrefixError{},
			invalidNames: []string{"__RUNNER_VAR1", "__RUNNER_VAR2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(nil)
			err := manager.ValidateUserEnvNames(tt.envMap)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errType), "error type mismatch")

				// Extract and validate all errors
				errs := extractReservedEnvPrefixErrors(err)
				require.NotEmpty(t, errs, "should have at least one ReservedEnvPrefixError")

				// Verify at least one error matches the expected invalid names
				foundVars := make([]string, 0, len(errs))
				for _, rpe := range errs {
					assert.Equal(t, AutoEnvPrefix, rpe.Prefix)
					foundVars = append(foundVars, rpe.VarName)
				}

				// Check that all found vars are in the expected list
				for _, foundVar := range foundVars {
					assert.True(t, slices.Contains(tt.invalidNames, foundVar),
						"found unexpected error variable %q, expected one of: %v", foundVar, tt.invalidNames)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManagerValidateUserEnvNames_MultipleErrors(t *testing.T) {
	tests := []struct {
		name              string
		envMap            map[string]string
		expectedErrorVars []string // All expected variable names in errors
	}{
		{
			name: "two reserved prefix violations",
			envMap: map[string]string{
				"PATH":          "/usr/bin",
				"__RUNNER_VAR1": "value1",
				"__RUNNER_VAR2": "value2",
			},
			expectedErrorVars: []string{"__RUNNER_VAR1", "__RUNNER_VAR2"},
		},
		{
			name: "three reserved prefix violations",
			envMap: map[string]string{
				"__RUNNER_VAR1": "value1",
				"__RUNNER_VAR2": "value2",
				"__RUNNER_VAR3": "value3",
				"VALID_VAR":     "valid",
			},
			expectedErrorVars: []string{"__RUNNER_VAR1", "__RUNNER_VAR2", "__RUNNER_VAR3"},
		},
		{
			name: "all reserved prefix violations",
			envMap: map[string]string{
				"__RUNNER_DATETIME": "value1",
				"__RUNNER_PID":      "value2",
				"__RUNNER_CUSTOM":   "value3",
			},
			expectedErrorVars: []string{"__RUNNER_DATETIME", "__RUNNER_PID", "__RUNNER_CUSTOM"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(nil)
			err := manager.ValidateUserEnvNames(tt.envMap)

			require.Error(t, err)

			// Use helper function to extract all errors
			errs := extractReservedEnvPrefixErrors(err)

			// Use helper function to assert all expected errors are found
			assertAllErrorVarsFound(t, errs, tt.expectedErrorVars)
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
