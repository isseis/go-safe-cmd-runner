package redaction

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSensitivePatterns_CombinedPatterns(t *testing.T) {
	patterns := DefaultSensitivePatterns()

	// Test that combined patterns are created
	assert.NotNil(t, patterns.combinedCredentialPattern, "Combined credential pattern should be created")
	assert.NotNil(t, patterns.combinedEnvVarPattern, "Combined env var pattern should be created")

	t.Run("IsSensitiveKey with combined pattern", func(t *testing.T) {
		// Test cases that should match
		testCases := []struct {
			key      string
			expected bool
		}{
			{"password", true},
			{"secret_key", true},
			{"api_token", true},
			{"AWS_ACCESS_KEY_ID", true},
			{"safe_field", false},
			{"username", false},
		}

		for _, tc := range testCases {
			result := patterns.IsSensitiveKey(tc.key)
			assert.Equal(t, tc.expected, result, "Key: %s", tc.key)
		}
	})

	t.Run("IsSensitiveValue with combined pattern", func(t *testing.T) {
		testCases := []struct {
			value    string
			expected bool
		}{
			{"my_password_123", true},
			{"bearer_token", true},
			{"safe_value", false},
			{"normal_text", false},
		}

		for _, tc := range testCases {
			result := patterns.IsSensitiveValue(tc.value)
			assert.Equal(t, tc.expected, result, "Value: %s", tc.value)
		}
	})

	t.Run("IsSensitiveEnvVar with combined pattern", func(t *testing.T) {
		testCases := []struct {
			envVar   string
			expected bool
		}{
			{"MY_PASSWORD", true},
			{"API_SECRET", true},
			{"DATABASE_TOKEN", true},
			{"PATH", false}, // Allowed env var
			{"HOME", false}, // Allowed env var
			{"NORMAL_VAR", false},
		}

		for _, tc := range testCases {
			result := patterns.IsSensitiveEnvVar(tc.envVar)
			assert.Equal(t, tc.expected, result, "EnvVar: %s", tc.envVar)
		}
	})
}

func TestNewSensitivePatterns(t *testing.T) {
	credentialPatterns := []string{
		`(?i)password`,
		`(?i)token`,
		`(?i)secret`,
	}

	envVarPatterns := []string{
		`(?i).*PASSWORD.*`,
		`(?i).*SECRET.*`,
	}

	allowedEnvVars := make(map[string]bool)

	patterns, err := NewSensitivePatterns(credentialPatterns, envVarPatterns, allowedEnvVars)
	require.NoError(t, err, "NewSensitivePatterns should succeed")

	// Test that patterns work correctly through the public API
	assert.True(t, patterns.IsSensitiveKey("password"))
	assert.True(t, patterns.IsSensitiveKey("token"))
	assert.True(t, patterns.IsSensitiveKey("secret"))
	assert.False(t, patterns.IsSensitiveKey("safe"))

	assert.True(t, patterns.IsSensitiveEnvVar("MY_PASSWORD_VAR"))
	assert.True(t, patterns.IsSensitiveEnvVar("SECRET_KEY"))
	assert.False(t, patterns.IsSensitiveEnvVar("NORMAL_VAR"))
}

func TestNewSensitivePatterns_ErrorHandling(t *testing.T) {
	// Test with invalid regex patterns
	invalidCredentialPatterns := []string{
		`(?i)password`,
		`[invalid`, // Invalid regex
	}

	envVarPatterns := []string{
		`(?i).*PASSWORD.*`,
	}

	allowedEnvVars := make(map[string]bool)

	patterns, err := NewSensitivePatterns(invalidCredentialPatterns, envVarPatterns, allowedEnvVars)
	assert.Error(t, err, "NewSensitivePatterns should fail with invalid regex")
	assert.Nil(t, patterns, "patterns should be nil on error")
	assert.Contains(t, err.Error(), "failed to build combined patterns")
}

func TestNewSensitivePatterns_EmptyPatterns(t *testing.T) {
	// Test with empty patterns (should succeed and create never-matching patterns)
	allowedEnvVars := make(map[string]bool)

	patterns, err := NewSensitivePatterns([]string{}, []string{}, allowedEnvVars)
	require.NoError(t, err, "NewSensitivePatterns should succeed with empty patterns")
	require.NotNil(t, patterns, "patterns should not be nil")

	// Should never match anything with empty patterns
	assert.False(t, patterns.IsSensitiveKey("password"))
	assert.False(t, patterns.IsSensitiveValue("secret_token"))
	assert.False(t, patterns.IsSensitiveEnvVar("MY_PASSWORD"))
}

// Benchmark to verify performance improvement
func BenchmarkIsSensitiveKey_IndividualPatterns(b *testing.B) {
	// Simulate original approach by manually checking each pattern
	credentialPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|token|secret|key|api_key)`),
		regexp.MustCompile(`(?i)aws_access_key_id`),
		regexp.MustCompile(`(?i)aws_secret_access_key`),
		regexp.MustCompile(`(?i)bearer`),
	}

	testKey := "api_secret_key"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate original loop-based approach
		found := false
		for _, pattern := range credentialPatterns {
			if pattern.MatchString(testKey) {
				found = true
				break
			}
		}
		_ = found
	}
}

func BenchmarkIsSensitiveKey_Combined(b *testing.B) {
	patterns := DefaultSensitivePatterns()
	testKey := "api_secret_key"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		patterns.IsSensitiveKey(testKey)
	}
}

// More realistic benchmark with various key types
func BenchmarkIsSensitiveKey_Mixed_IndividualPatterns(b *testing.B) {
	credentialPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|token|secret|key|api_key)`),
		regexp.MustCompile(`(?i)aws_access_key_id`),
		regexp.MustCompile(`(?i)aws_secret_access_key`),
		regexp.MustCompile(`(?i)bearer`),
		regexp.MustCompile(`(?i)authorization`),
	}

	testKeys := []string{
		"password",          // Should match first pattern
		"safe_field",        // Should match none (worst case)
		"aws_access_key_id", // Should match later pattern
		"username",          // Should match none
		"bearer_token",      // Should match later pattern
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := testKeys[i%len(testKeys)]
		// Simulate original loop-based approach
		found := false
		for _, pattern := range credentialPatterns {
			if pattern.MatchString(key) {
				found = true
				break
			}
		}
		_ = found
	}
}

func BenchmarkIsSensitiveKey_Mixed_Combined(b *testing.B) {
	patterns := DefaultSensitivePatterns()

	testKeys := []string{
		"password",          // Should match
		"safe_field",        // Should match none (worst case)
		"aws_access_key_id", // Should match
		"username",          // Should match none
		"bearer_token",      // Should match
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := testKeys[i%len(testKeys)]
		patterns.IsSensitiveKey(key)
	}
}
