package environment

import (
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/variable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoEnvProviderGenerate(t *testing.T) {
	// Fixed time for testing: 2025-10-05 14:30:22.123456789 UTC
	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	fixedClock := func() time.Time { return fixedTime }

	tests := []struct {
		name        string
		clock       variable.Clock
		wantAutoEnv map[string]string
	}{
		{
			name:  "generate auto env with fixed clock",
			clock: fixedClock,
			wantAutoEnv: map[string]string{
				"__runner_datetime": "20251005143022.123",
				// PID is dynamic, checked separately
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := variable.NewAutoVarProviderWithClock(tt.clock)
			result := provider.Generate()

			require.NotNil(t, result)

			// Check that all auto internal variables are present
			for key, expectedValue := range tt.wantAutoEnv {
				actualValue, ok := result[key]
				assert.True(t, ok, "auto internal variable %q should be present", key)
				assert.Equal(t, expectedValue, actualValue, "auto internal variable %q value mismatch", key)
			}

			// Check that __runner_pid is present and is a valid number
			pid, ok := result["__runner_pid"]
			assert.True(t, ok, "__runner_pid should be present")
			assert.Regexp(t, `^\d+$`, pid, "__runner_pid should be a number")

			// Check that the result contains only auto internal variables (lowercase datetime + pid)
			expectedCount := len(tt.wantAutoEnv) + 1 // +1 for PID
			assert.Equal(t, expectedCount, len(result), "variable map size mismatch")
		})
	}
}

func TestAutoEnvProviderGenerateWithDefaultClock(t *testing.T) {
	provider := variable.NewAutoVarProvider()

	result := provider.Generate()
	require.NotNil(t, result)

	// Check that auto internal variables are present with valid formats (lowercase)
	datetime, ok := result["__runner_datetime"]
	assert.True(t, ok, "__runner_datetime should be present")
	assert.Regexp(t, `^\d{14}\.\d{3}$`, datetime, "__runner_datetime should match format YYYYMMDDHHmmSS.mmm")

	pid, ok := result["__runner_pid"]
	assert.True(t, ok, "__runner_pid should be present")
	assert.Regexp(t, `^\d+$`, pid, "__runner_pid should be a number")

	// Check that only auto internal variables are present (lowercase only)
	assert.Equal(t, 2, len(result), "should contain __runner_datetime, __runner_pid")
}

func TestAutoEnvProviderGenerateConsistency(t *testing.T) {
	// This test verifies that AutoEnvProvider generates consistent values

	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	fixedClock := func() time.Time { return fixedTime }

	provider := variable.NewAutoVarProviderWithClock(fixedClock)

	result := provider.Generate()

	// Auto internal variables should always be present with correct values (lowercase)
	assert.Equal(t, "20251005143022.123", result["__runner_datetime"])
	assert.Regexp(t, `^\d+$`, result["__runner_pid"])

	// Only auto internal variables should be present (lowercase only)
	assert.Equal(t, 2, len(result), "should contain __runner_datetime, __runner_pid")
}
