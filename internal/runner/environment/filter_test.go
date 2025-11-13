package environment

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFilter(t *testing.T) {
	config := &runnertypes.ConfigSpec{}
	filter := NewFilter(config.Global.EnvAllowed)

	require.NotNil(t, filter, "NewFilter returned nil")
}

func TestNewFilter_WithAllowlist(t *testing.T) {
	allowlist := []string{"PATH", "HOME", "USER"}
	filter := NewFilter(allowlist)

	require.NotNil(t, filter)
	require.NotNil(t, filter.globalAllowlist)
	assert.Len(t, filter.globalAllowlist, 3)
}

func TestParseSystemEnvironment(t *testing.T) {
	// Save original environment
	origEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range origEnv {
			key, value, ok := parseEnvString(env)
			if ok {
				os.Setenv(key, value)
			}
		}
	}()

	// Set up test environment
	os.Clearenv()
	os.Setenv("TEST_VAR1", "value1")
	os.Setenv("TEST_VAR2", "value2")
	os.Setenv("EMPTY_VAR", "")

	filter := NewFilter([]string{})
	result := filter.ParseSystemEnvironment()

	assert.Contains(t, result, "TEST_VAR1")
	assert.Equal(t, "value1", result["TEST_VAR1"])
	assert.Contains(t, result, "TEST_VAR2")
	assert.Equal(t, "value2", result["TEST_VAR2"])
	assert.Contains(t, result, "EMPTY_VAR")
	assert.Equal(t, "", result["EMPTY_VAR"])
}

func TestParseSystemEnvironment_EmptyEnvironment(t *testing.T) {
	// Save original environment
	origEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range origEnv {
			key, value, ok := parseEnvString(env)
			if ok {
				os.Setenv(key, value)
			}
		}
	}()

	// Clear environment
	os.Clearenv()

	filter := NewFilter([]string{})
	result := filter.ParseSystemEnvironment()

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestFilterSystemEnvironment(t *testing.T) {
	// Save original environment
	origEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range origEnv {
			key, value, ok := parseEnvString(env)
			if ok {
				os.Setenv(key, value)
			}
		}
	}()

	// Set up test environment
	os.Clearenv()
	os.Setenv("ALLOWED_VAR", "value1")
	os.Setenv("ANOTHER_VAR", "value2")

	filter := NewFilter([]string{"ALLOWED_VAR", "ANOTHER_VAR"})
	result, err := filter.FilterSystemEnvironment()

	require.NoError(t, err)
	assert.Contains(t, result, "ALLOWED_VAR")
	assert.Contains(t, result, "ANOTHER_VAR")
}

func TestFilterGlobalVariables_SourceSystem(t *testing.T) {
	filter := NewFilter([]string{"PATH", "HOME"})

	envVars := map[string]string{
		"PATH": "/usr/bin",
		"HOME": "/home/user",
		"USER": "testuser",
	}

	result, err := filter.FilterGlobalVariables(envVars, SourceSystem)

	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "/usr/bin", result["PATH"])
	assert.Equal(t, "/home/user", result["HOME"])
	assert.Equal(t, "testuser", result["USER"])
}

func TestFilterGlobalVariables_SourceEnvFile(t *testing.T) {
	filter := NewFilter([]string{"API_KEY", "DATABASE_URL"})

	envVars := map[string]string{
		"API_KEY":      "secret123",
		"DATABASE_URL": "postgres://localhost",
		"DEBUG":        "true",
	}

	result, err := filter.FilterGlobalVariables(envVars, SourceEnvFile)

	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "secret123", result["API_KEY"])
	assert.Equal(t, "postgres://localhost", result["DATABASE_URL"])
	assert.Equal(t, "true", result["DEBUG"])
}

func TestFilterGlobalVariables_EmptyVariableName(t *testing.T) {
	filter := NewFilter([]string{})

	envVars := map[string]string{
		"":      "invalid",
		"VALID": "value",
	}

	result, err := filter.FilterGlobalVariables(envVars, SourceSystem)

	require.NoError(t, err)
	// Empty name should be skipped
	assert.NotContains(t, result, "")
	assert.Contains(t, result, "VALID")
	assert.Len(t, result, 1)
}

func TestFilterGlobalVariables_EmptyValue(t *testing.T) {
	filter := NewFilter([]string{"EMPTY_VAR"})

	envVars := map[string]string{
		"EMPTY_VAR": "",
	}

	result, err := filter.FilterGlobalVariables(envVars, SourceSystem)

	require.NoError(t, err)
	assert.Contains(t, result, "EMPTY_VAR")
	assert.Equal(t, "", result["EMPTY_VAR"])
}

func TestFilterGlobalVariables_SpecialCharactersInValue(t *testing.T) {
	filter := NewFilter([]string{})

	envVars := map[string]string{
		"VAR_WITH_NEWLINE": "line1\nline2",
		"VAR_WITH_TAB":     "value\twith\ttabs",
		"VAR_WITH_QUOTE":   "value\"with'quotes",
	}

	result, err := filter.FilterGlobalVariables(envVars, SourceSystem)

	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "line1\nline2", result["VAR_WITH_NEWLINE"])
	assert.Equal(t, "value\twith\ttabs", result["VAR_WITH_TAB"])
	assert.Equal(t, "value\"with'quotes", result["VAR_WITH_QUOTE"])
}

func TestFilterGlobalVariables_EmptyMap(t *testing.T) {
	filter := NewFilter([]string{})

	envVars := map[string]string{}

	result, err := filter.FilterGlobalVariables(envVars, SourceSystem)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// parseEnvString is a helper function to parse environment strings for test cleanup
func parseEnvString(env string) (string, string, bool) {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return env[:i], env[i+1:], true
		}
	}
	return "", "", false
}
