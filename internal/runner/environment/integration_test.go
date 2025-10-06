package environment

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ManagerAndExpander_AutoDateTime tests integration between
// EnvironmentManager and VariableExpander for __RUNNER_DATETIME expansion
func TestIntegration_ManagerAndExpander_AutoDateTime(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create EnvironmentManager with fixed clock
	manager := NewManager(clock)

	// Build environment with auto-generated variables
	userEnv := map[string]string{
		"USER_VAR": "user_value",
	}
	env, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)

	// Verify auto-generated variables are present
	assert.Equal(t, "202510051430.123", env["__RUNNER_DATETIME"])
	assert.NotEmpty(t, env["__RUNNER_PID"])
	assert.Equal(t, "user_value", env["USER_VAR"])

	// Create VariableExpander with empty filter (allow all)
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string containing __RUNNER_DATETIME
	input := "backup-${__RUNNER_DATETIME}.tar.gz"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]bool))
	require.NoError(t, err)

	// Verify expansion
	expected := "backup-202510051430.123.tar.gz"
	assert.Equal(t, expected, expanded)
}

// TestIntegration_ManagerAndExpander_AutoPID tests integration between
// EnvironmentManager and VariableExpander for __RUNNER_PID expansion
func TestIntegration_ManagerAndExpander_AutoPID(t *testing.T) {
	// Create EnvironmentManager with default clock
	manager := NewManager(nil)

	// Build environment with auto-generated variables
	userEnv := map[string]string{}
	env, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)

	// Verify __RUNNER_PID is present and equals current PID
	expectedPID := strconv.Itoa(os.Getpid())
	assert.Equal(t, expectedPID, env["__RUNNER_PID"])

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string containing __RUNNER_PID
	input := "process-${__RUNNER_PID}.log"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]bool))
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
	manager := NewManager(clock)

	// Build environment
	userEnv := map[string]string{
		"HOST": "server01",
	}
	env, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string with multiple auto variables
	input := "${HOST}-${__RUNNER_DATETIME}-${__RUNNER_PID}.log"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]bool))
	require.NoError(t, err)

	// Verify expansion pattern
	expectedPID := strconv.Itoa(os.Getpid())
	expected := fmt.Sprintf("server01-202512312359.999-%s.log", expectedPID)
	assert.Equal(t, expected, expanded)
}

// TestIntegration_ManagerAndExpander_ExpandStrings tests integration with
// multiple strings (like command args)
func TestIntegration_ManagerAndExpander_ExpandStrings(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 1000000, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create EnvironmentManager with fixed clock
	manager := NewManager(clock)

	// Build environment
	userEnv := map[string]string{
		"DATA_DIR": "/data",
	}
	env, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)

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
		"/data/backup-202501010000.001.tar.gz",
		"/data/files",
	}
	assert.Equal(t, expected, expanded)
}

// TestIntegration_ManagerAndExpander_RealTimeClock tests integration with
// real-time clock (not fixed) to ensure datetime format is correct
func TestIntegration_ManagerAndExpander_RealTimeClock(t *testing.T) {
	// Create EnvironmentManager with nil clock (uses time.Now)
	manager := NewManager(nil)

	// Build environment
	userEnv := map[string]string{}
	env, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)

	// Verify __RUNNER_DATETIME matches expected format (YYYYMMDDHHMM.mmm)
	datetime := env["__RUNNER_DATETIME"]
	assert.NotEmpty(t, datetime)

	// Verify format using regex
	datetimePattern := regexp.MustCompile(`^\d{12}\.\d{3}$`)
	assert.Truef(t, datetimePattern.MatchString(datetime),
		"__RUNNER_DATETIME format should be YYYYMMDDHHMM.mmm, got: %s", datetime)

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string with real datetime
	input := "log-${__RUNNER_DATETIME}.txt"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]bool))
	require.NoError(t, err)

	// Verify expansion pattern
	expectedPattern := regexp.MustCompile(`^log-\d{12}\.\d{3}\.txt$`)
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
	manager := NewManager(clock)

	// Build environment with no user variables
	env, err := manager.BuildEnv(map[string]string{})
	require.NoError(t, err)

	// Verify only auto-generated variables are present
	assert.Len(t, env, 2) // Only __RUNNER_DATETIME and __RUNNER_PID
	assert.Equal(t, "202506151200.500", env["__RUNNER_DATETIME"])
	assert.NotEmpty(t, env["__RUNNER_PID"])

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand string with only auto variables
	input := "${__RUNNER_DATETIME}"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]bool))
	require.NoError(t, err)

	// Verify expansion
	assert.Equal(t, "202506151200.500", expanded)
}

// TestIntegration_ManagerAndExpander_MixedWithSystemEnv tests that
// auto-generated variables work alongside system environment variables
func TestIntegration_ManagerAndExpander_MixedWithSystemEnv(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 3, 20, 8, 30, 45, 100000000, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create EnvironmentManager with fixed clock
	manager := NewManager(clock)

	// Build environment with user variables (simulating system env)
	userEnv := map[string]string{
		"PATH":     "/usr/bin:/bin",
		"HOME":     "/home/user",
		"CUSTOM":   "value",
		"LOG_PATH": "/var/log/${__RUNNER_DATETIME}",
	}
	env, err := manager.BuildEnv(userEnv)
	require.NoError(t, err)

	// Verify both auto and user variables are present
	assert.Equal(t, "202503200830.100", env["__RUNNER_DATETIME"])
	assert.NotEmpty(t, env["__RUNNER_PID"])
	assert.Equal(t, "/usr/bin:/bin", env["PATH"])
	assert.Equal(t, "/home/user", env["HOME"])
	assert.Equal(t, "value", env["CUSTOM"])

	// Create VariableExpander
	filter := NewFilter([]string{})
	expander := NewVariableExpander(filter)

	// Expand LOG_PATH which references __RUNNER_DATETIME
	logPath := env["LOG_PATH"]
	expanded, err := expander.ExpandString(logPath, env, []string{}, "", make(map[string]bool))
	require.NoError(t, err)

	// Verify expansion
	assert.Equal(t, "/var/log/202503200830.100", expanded)
}
