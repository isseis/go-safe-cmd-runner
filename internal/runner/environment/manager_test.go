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

func TestValidateUserEnvNames(t *testing.T) {
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
			err := ValidateUserEnvNames(tt.envMap)

			if tt.wantErr {
				require.Error(t, err)
				// If tt.errType is a pointer to a struct error (e.g.
				// *runnertypes.ReservedEnvPrefixError), use errors.As to
				// check the error chain for that concrete type. For
				// sentinel errors, fall back to errors.Is.
				switch tt.errType.(type) {
				case *runnertypes.ReservedEnvPrefixError:
					var target *runnertypes.ReservedEnvPrefixError
					assert.True(t, errors.As(err, &target), "error type mismatch")
				default:
					assert.True(t, errors.Is(err, tt.errType), "error type mismatch")
				}

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

func TestValidateUserEnvNames_MultipleErrors(t *testing.T) {
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
			err := ValidateUserEnvNames(tt.envMap)

			require.Error(t, err)

			// Use helper function to extract all errors
			errs := extractReservedEnvPrefixErrors(err)

			// Use helper function to assert all expected errors are found
			assertAllErrorVarsFound(t, errs, tt.expectedErrorVars)
		})
	}
}

func TestAutoEnvProviderGenerate(t *testing.T) {
	// Fixed time for testing: 2025-10-05 14:30:22.123456789 UTC
	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	fixedClock := func() time.Time { return fixedTime }

	tests := []struct {
		name        string
		clock       Clock
		wantAutoEnv map[string]string
	}{
		{
			name:  "generate auto env with fixed clock",
			clock: fixedClock,
			wantAutoEnv: map[string]string{
				"__RUNNER_DATETIME": "20251005143022.123",
				// PID is dynamic, checked separately
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewAutoEnvProvider(tt.clock)
			result := provider.Generate()

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

			// Check that __runner_pid (lowercase) is also present
			pidLower, ok := result["__runner_pid"]
			assert.True(t, ok, "__runner_pid should be present")
			assert.Regexp(t, `^\d+$`, pidLower, "__runner_pid should be a number")

			// Check that the result contains only auto env (uppercase and lowercase DATETIME + PID)
			expectedCount := (len(tt.wantAutoEnv) + 1) * 2 // +1 for PID, *2 for uppercase and lowercase formats
			assert.Equal(t, expectedCount, len(result), "environment map size mismatch")
		})
	}
}

func TestAutoEnvProviderGenerateWithDefaultClock(t *testing.T) {
	provider := NewAutoEnvProvider(nil)

	result := provider.Generate()
	require.NotNil(t, result)

	// Check that auto env variables are present with valid formats (uppercase)
	datetime, ok := result["__RUNNER_DATETIME"]
	assert.True(t, ok, "__RUNNER_DATETIME should be present")
	assert.Regexp(t, `^\d{14}\.\d{3}$`, datetime, "__RUNNER_DATETIME should match format YYYYMMDDHHmmSS.mmm")

	pid, ok := result["__RUNNER_PID"]
	assert.True(t, ok, "__RUNNER_PID should be present")
	assert.Regexp(t, `^\d+$`, pid, "__RUNNER_PID should be a number")

	// Check that auto env variables are present with valid formats (lowercase)
	datetimeLower, ok := result["__runner_datetime"]
	assert.True(t, ok, "__runner_datetime should be present")
	assert.Regexp(t, `^\d{14}\.\d{3}$`, datetimeLower, "__runner_datetime should match format YYYYMMDDHHmmSS.mmm")

	pidLower, ok := result["__runner_pid"]
	assert.True(t, ok, "__runner_pid should be present")
	assert.Regexp(t, `^\d+$`, pidLower, "__runner_pid should be a number")

	// Check that only auto env variables are present (both uppercase and lowercase)
	assert.Equal(t, 4, len(result), "should contain __RUNNER_DATETIME, __RUNNER_PID, __runner_datetime, __runner_pid")
}

func TestAutoEnvProviderGenerateConsistency(t *testing.T) {
	// This test verifies that AutoEnvProvider generates consistent values

	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	fixedClock := func() time.Time { return fixedTime }

	provider := NewAutoEnvProvider(fixedClock)

	result := provider.Generate()

	// Auto env should always be present with correct values (uppercase)
	assert.Equal(t, "20251005143022.123", result["__RUNNER_DATETIME"])
	assert.Regexp(t, `^\d+$`, result["__RUNNER_PID"])

	// Auto env should always be present with correct values (lowercase)
	assert.Equal(t, "20251005143022.123", result["__runner_datetime"])
	assert.Regexp(t, `^\d+$`, result["__runner_pid"])

	// Only auto env variables should be present (both uppercase and lowercase)
	assert.Equal(t, 4, len(result), "should contain __RUNNER_DATETIME, __RUNNER_PID, __runner_datetime, __runner_pid")
}
