package environment

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_AutoEnvProviderAndExpander_AutoDateTime tests integration between
// AutoEnvProvider and VariableExpander for __RUNNER_DATETIME expansion
func TestIntegration_AutoEnvProviderAndExpander_AutoDateTime(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create AutoEnvProvider with fixed clock
	provider := NewAutoEnvProvider(clock)

	// Generate auto environment variables
	env := provider.Generate()

	// Verify auto-generated variables are present
	assert.Equal(t, "20251005143022.123", env["__RUNNER_DATETIME"])
	assert.NotEmpty(t, env["__RUNNER_PID"])

	// Create VariableExpander with empty filter (allow all)
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string containing __RUNNER_DATETIME
	input := "backup-${__RUNNER_DATETIME}.tar.gz"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion
	expected := "backup-20251005143022.123.tar.gz"
	assert.Equal(t, expected, expanded)
}

// TestIntegration_AutoEnvProviderAndExpander_AutoPID tests integration between
// AutoEnvProvider and VariableExpander for __RUNNER_PID expansion
func TestIntegration_AutoEnvProviderAndExpander_AutoPID(t *testing.T) {
	// Create AutoEnvProvider with default clock
	provider := NewAutoEnvProvider(nil)

	// Generate auto environment variables
	env := provider.Generate()

	// Verify __RUNNER_PID is present and equals current PID
	expectedPID := strconv.Itoa(os.Getpid())
	assert.Equal(t, expectedPID, env["__RUNNER_PID"])

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string containing __RUNNER_PID
	input := "process-${__RUNNER_PID}.log"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion
	expected := fmt.Sprintf("process-%s.log", expectedPID)
	assert.Equal(t, expected, expanded)
}

// TestIntegration_ManagerAndExpander_MultipleAutoVars tests integration with
// multiple auto-generated variables in a single string
func TestIntegration_ManagerAndExpander_MultipleAutoVars(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create EnvironmentManager with fixed clock
	provider := NewAutoEnvProvider(clock)

	// Build environment with auto-generated variables
	autoEnv := provider.Generate()

	// Add user-defined variables (simulating what happens in ExpandCommand)
	env := make(map[string]string)
	maps.Copy(env, autoEnv)
	env["HOST"] = "server01"

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string with multiple auto variables
	input := "${HOST}-${__RUNNER_DATETIME}-${__RUNNER_PID}.log"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion pattern
	expectedPID := strconv.Itoa(os.Getpid())
	expected := fmt.Sprintf("server01-20251231235959.999-%s.log", expectedPID)
	assert.Equal(t, expected, expanded)
}

// TestIntegration_ManagerAndExpander_ExpandStrings tests integration with
// multiple strings (like command args)
func TestIntegration_ManagerAndExpander_ExpandStrings(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 1000000, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create EnvironmentManager with fixed clock
	provider := NewAutoEnvProvider(clock)

	// Build environment with auto-generated variables
	autoEnv := provider.Generate()

	// Add user-defined variables (simulating what happens in ExpandCommand)
	env := make(map[string]string)
	maps.Copy(env, autoEnv)
	env["DATA_DIR"] = "/data"

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand multiple strings (simulating command args)
	inputs := []string{
		"tar",
		"czf",
		"${DATA_DIR}/backup-${__RUNNER_DATETIME}.tar.gz",
		"${DATA_DIR}/files",
	}
	expanded, err := expander.ExpandStrings(inputs, env, []string{}, "")
	require.NoError(t, err)

	// Verify expansion
	expected := []string{
		"tar",
		"czf",
		"/data/backup-20250101000000.001.tar.gz",
		"/data/files",
	}
	assert.Equal(t, expected, expanded)
}

// TestIntegration_ManagerAndExpander_RealTimeClock tests integration with
// real-time clock (not fixed) to ensure datetime format is correct
func TestIntegration_ManagerAndExpander_RealTimeClock(t *testing.T) {
	// Create EnvironmentManager with nil clock (uses time.Now)
	provider := NewAutoEnvProvider(nil)

	// Build environment with auto-generated variables
	env := provider.Generate()

	// Verify __RUNNER_DATETIME matches expected format (YYYYMMDDHHmmSS.mmm)
	datetime := env["__RUNNER_DATETIME"]
	assert.NotEmpty(t, datetime)

	// Verify format using regex
	datetimePattern := regexp.MustCompile(`^\d{14}\.\d{3}$`)
	assert.Truef(t, datetimePattern.MatchString(datetime),
		"__RUNNER_DATETIME format should be YYYYMMDDHHmmSS.mmm, got: %s", datetime)

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string with real datetime
	input := "log-${__RUNNER_DATETIME}.txt"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion pattern
	expectedPattern := regexp.MustCompile(`^log-\d{14}\.\d{3}\.txt$`)
	assert.Truef(t, expectedPattern.MatchString(expanded),
		"Expanded string should match pattern, got: %s", expanded)
}

// TestIntegration_ManagerAndExpander_NoUserEnv tests integration with
// only auto-generated variables (no user-defined variables)
func TestIntegration_ManagerAndExpander_NoUserEnv(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 500000000, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create EnvironmentManager with fixed clock
	provider := NewAutoEnvProvider(clock)

	// Build environment with auto-generated variables only
	env := provider.Generate()

	// Verify only auto-generated variables are present
	assert.Len(t, env, 2) // Only __RUNNER_DATETIME and __RUNNER_PID
	assert.Equal(t, "20250615120000.500", env["__RUNNER_DATETIME"])
	assert.NotEmpty(t, env["__RUNNER_PID"])

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string with only auto variables
	input := "${__RUNNER_DATETIME}"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion
	assert.Equal(t, "20250615120000.500", expanded)
}

// TestIntegration_ManagerAndExpander_MixedWithSystemEnv tests that
// auto-generated variables work alongside system environment variables
func TestIntegration_ManagerAndExpander_MixedWithSystemEnv(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 3, 20, 8, 30, 45, 100000000, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create EnvironmentManager with fixed clock
	provider := NewAutoEnvProvider(clock)

	// Build environment with auto-generated variables
	autoEnv := provider.Generate()

	// Add user-defined variables (simulating what happens in ExpandCommand)
	env := make(map[string]string)
	maps.Copy(env, autoEnv)
	env["PATH"] = "/usr/bin:/bin"
	env["HOME"] = "/home/user"
	env["CUSTOM"] = "value"
	env["LOG_PATH"] = "/var/log/${__RUNNER_DATETIME}"

	// Verify both auto and user variables are present
	assert.Equal(t, "20250320083045.100", env["__RUNNER_DATETIME"])
	assert.NotEmpty(t, env["__RUNNER_PID"])
	assert.Equal(t, "/usr/bin:/bin", env["PATH"])
	assert.Equal(t, "/home/user", env["HOME"])
	assert.Equal(t, "value", env["CUSTOM"])

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand LOG_PATH which references __RUNNER_DATETIME
	logPath := env["LOG_PATH"]
	expanded, err := expander.ExpandString(logPath, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion
	assert.Equal(t, "/var/log/20250320083045.100", expanded)
}
