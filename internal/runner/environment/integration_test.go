package environment_test

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/variable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_AutoEnvProviderAndExpander_AutoDateTime tests integration between
// AutoEnvProvider and VariableExpander for __runner_datetime expansion
func TestIntegration_AutoEnvProviderAndExpander_AutoDateTime(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC)
	clock := func() time.Time { return fixedTime }

	// Create AutoEnvProvider with fixed clock
	provider := variable.NewAutoVarProviderWithClock(clock)

	// Generate auto internal variables
	env := provider.Generate()

	// Verify auto-generated variables are present
	assert.Equal(t, "20251005143022.123", env["__runner_datetime"])
	assert.NotEmpty(t, env["__runner_pid"])

	// Create VariableExpander with empty filter (allow all)
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Expand string containing __runner_datetime
	input := "backup-${__runner_datetime}.tar.gz"
	expanded, err := expander.ExpandString(input, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion
	expected := "backup-20251005143022.123.tar.gz"
	assert.Equal(t, expected, expanded)
}

// TestIntegration_AutoEnvProviderAndExpander_AutoPID tests integration between
// AutoEnvProvider and VariableExpander for __runner_pid expansion
func TestIntegration_AutoEnvProviderAndExpander_AutoPID(t *testing.T) {
	// Create AutoEnvProvider with default clock
	provider := variable.NewAutoVarProvider()

	// Generate auto internal variables
	env := provider.Generate()

	// Verify __runner_pid is present and equals current PID
	expectedPID := strconv.Itoa(os.Getpid())
	assert.Equal(t, expectedPID, env["__runner_pid"])

	// Create VariableExpander
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Expand string containing __runner_pid
	input := "process-${__runner_pid}.log"
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
	provider := variable.NewAutoVarProviderWithClock(clock)

	// Build environment with auto-generated variables
	autoEnv := provider.Generate()

	// Add user-defined variables (simulating what happens in ExpandCommand)
	env := make(map[string]string)
	maps.Copy(env, autoEnv)
	env["host"] = "server01"

	// Create VariableExpander
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Expand string with multiple auto variables
	input := "${host}-${__runner_datetime}-${__runner_pid}.log"
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
	provider := variable.NewAutoVarProviderWithClock(clock)

	// Build environment with auto-generated variables
	autoEnv := provider.Generate()

	// Add user-defined variables (simulating what happens in ExpandCommand)
	env := make(map[string]string)
	maps.Copy(env, autoEnv)
	env["data_dir"] = "/data"

	// Create VariableExpander
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Expand multiple strings (simulating command args)
	inputs := []string{
		"tar",
		"czf",
		"${data_dir}/backup-${__runner_datetime}.tar.gz",
		"${data_dir}/files",
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
	provider := variable.NewAutoVarProvider()

	// Build environment with auto-generated variables
	env := provider.Generate()

	// Verify __runner_datetime matches expected format (YYYYMMDDHHmmSS.mmm)
	datetime := env["__runner_datetime"]
	assert.NotEmpty(t, datetime)

	// Verify format using regex
	datetimePattern := regexp.MustCompile(`^\d{14}\.\d{3}$`)
	assert.Truef(t, datetimePattern.MatchString(datetime),
		"__runner_datetime format should be YYYYMMDDHHmmSS.mmm, got: %s", datetime)

	// Create VariableExpander
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Expand string with real datetime
	input := "log-${__runner_datetime}.txt"
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
	provider := variable.NewAutoVarProviderWithClock(clock)

	// Build environment with auto-generated variables only
	env := provider.Generate()

	// Verify only auto-generated variables are present (lowercase format only)
	assert.Len(t, env, 2) // __runner_datetime, __runner_pid
	assert.Equal(t, "20250615120000.500", env["__runner_datetime"])
	assert.NotEmpty(t, env["__runner_pid"])

	// Create VariableExpander
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Expand string with only auto variables
	input := "${__runner_datetime}"
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
	provider := variable.NewAutoVarProviderWithClock(clock)

	// Build environment with auto-generated variables
	autoEnv := provider.Generate()

	// Add user-defined variables (simulating what happens in ExpandCommand)
	env := make(map[string]string)
	maps.Copy(env, autoEnv)
	env["path"] = "/usr/bin:/bin"
	env["home"] = "/home/user"
	env["custom"] = "value"
	env["log_path"] = "/var/log/${__runner_datetime}"

	// Verify both auto and user variables are present
	assert.Equal(t, "20250320083045.100", env["__runner_datetime"])
	assert.NotEmpty(t, env["__runner_pid"])
	assert.Equal(t, "/usr/bin:/bin", env["path"])
	assert.Equal(t, "/home/user", env["home"])
	assert.Equal(t, "value", env["custom"])

	// Create VariableExpander
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Expand log_path which references __runner_datetime
	logPath := env["log_path"]
	expanded, err := expander.ExpandString(logPath, env, []string{}, "", make(map[string]struct{}))
	require.NoError(t, err)

	// Verify expansion
	assert.Equal(t, "/var/log/20250320083045.100", expanded)
}
