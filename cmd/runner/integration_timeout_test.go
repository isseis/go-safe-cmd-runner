package main

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timeoutTestHelper is a helper function for timeout resolution integration tests.
// It loads config from TOML, creates a RuntimeCommand, and returns the effective timeout value.
func timeoutTestHelper(t *testing.T, configTOML string) int {
	t.Helper()

	// Load config using the helper from integration_envpriority_test.go
	cfg := configSetupHelper(t, nil, configTOML)

	// Extract the first command
	require.NotEmpty(t, cfg.Groups, "Groups should not be empty")
	require.NotEmpty(t, cfg.Groups[0].Commands, "Commands in first group should not be empty")
	cmdSpec := &cfg.Groups[0].Commands[0]

	// Create RuntimeCommand with timeout resolution
	// Note: Group-level timeout is not yet implemented (future enhancement)
	groupName := ""
	if len(cfg.Groups) > 0 {
		groupName = cfg.Groups[0].Name
	}
	finalRuntimeCmd, err := runnertypes.NewRuntimeCommand(cmdSpec, common.NewFromIntPtr(cfg.Global.Timeout), groupName)
	require.NoError(t, err, "Failed to create RuntimeCommand")

	return finalRuntimeCmd.EffectiveTimeout
}

// TestRunner_TimeoutResolution_Hierarchy tests the timeout resolution hierarchy.
// Note: Group-level timeout is not yet implemented (future enhancement).
// Current hierarchy: command timeout > global timeout > default timeout (60s)
func TestRunner_TimeoutResolution_Hierarchy(t *testing.T) {
	tests := []struct {
		name            string
		configTOML      string
		expectedTimeout int
	}{
		{
			name: "default_timeout_when_nothing_set",
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectedTimeout: 60, // DefaultTimeout from internal/common/types.go
		},
		{
			name: "global_timeout_only",
			configTOML: `
[global]
timeout = 120
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectedTimeout: 120,
		},
		{
			name: "command_timeout_overrides_global",
			configTOML: `
[global]
timeout = 120
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
timeout = 240
`,
			expectedTimeout: 240,
		},
		{
			name: "command_timeout_overrides_global_when_explicitly_set",
			configTOML: `
[global]
timeout = 120
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
timeout = 300
`,
			expectedTimeout: 300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualTimeout := timeoutTestHelper(t, tt.configTOML)
			assert.Equal(t, tt.expectedTimeout, actualTimeout,
				"Timeout should be correctly resolved according to hierarchy")
		})
	}
}

// TestRunner_TimeoutResolution_UnlimitedExecution tests unlimited timeout (timeout = 0).
// Note: Group-level timeout is not yet implemented (future enhancement).
func TestRunner_TimeoutResolution_UnlimitedExecution(t *testing.T) {
	tests := []struct {
		name            string
		configTOML      string
		expectedTimeout int
	}{
		{
			name: "global_unlimited_timeout",
			configTOML: `
[global]
timeout = 0
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectedTimeout: 0,
		},
		{
			name: "command_unlimited_timeout_overrides_global",
			configTOML: `
[global]
timeout = 120
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
timeout = 0
`,
			expectedTimeout: 0,
		},
		{
			name: "unlimited_at_command_overrides_limited_at_global",
			configTOML: `
[global]
timeout = 180
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
timeout = 0
`,
			expectedTimeout: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualTimeout := timeoutTestHelper(t, tt.configTOML)
			assert.Equal(t, tt.expectedTimeout, actualTimeout,
				"Timeout = 0 should mean unlimited execution")
		})
	}
}

// TestRunner_TimeoutResolution_EdgeCases tests edge cases in timeout configuration.
// Note: Group-level timeout is not yet implemented (future enhancement).
func TestRunner_TimeoutResolution_EdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		configTOML      string
		expectedTimeout int
	}{
		{
			name: "large_timeout_value",
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
timeout = 86400
`,
			expectedTimeout: 86400, // 24 hours (MaxTimeout)
		},
		{
			name: "very_small_timeout",
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
timeout = 1
`,
			expectedTimeout: 1,
		},
		{
			name: "global_unlimited_inherited_by_command",
			configTOML: `
[global]
timeout = 0
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectedTimeout: 0, // Command inherits global unlimited timeout
		},
		{
			name: "command_timeout_overrides_global",
			configTOML: `
[global]
timeout = 30
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
timeout = 90
`,
			expectedTimeout: 90, // Command has highest priority
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualTimeout := timeoutTestHelper(t, tt.configTOML)
			assert.Equal(t, tt.expectedTimeout, actualTimeout,
				"Edge case timeout should be correctly resolved")
		})
	}
}

// TestRunner_TimeoutResolution_MultipleCommands tests that each command independently resolves its timeout.
// Note: Group-level timeout is not yet implemented (future enhancement).
func TestRunner_TimeoutResolution_MultipleCommands(t *testing.T) {
	configTOML := `
[global]
timeout = 60
[[groups]]
name = "test_group"
[[groups.commands]]
name = "cmd1"
cmd = "echo"
args = ["cmd1"]
[[groups.commands]]
name = "cmd2"
cmd = "echo"
args = ["cmd2"]
timeout = 180
[[groups.commands]]
name = "cmd3"
cmd = "echo"
args = ["cmd3"]
timeout = 0
`

	cfg := configSetupHelper(t, nil, configTOML)

	// Verify that we have 3 commands
	require.NotEmpty(t, cfg.Groups, "Groups should not be empty")
	require.NotEmpty(t, cfg.Groups[0].Commands, "Commands in first group should not be empty")
	require.GreaterOrEqual(t, len(cfg.Groups[0].Commands), 3, "Expected at least 3 commands in config")
	groupSpec := &cfg.Groups[0]

	// Test each command independently
	testCases := []struct {
		cmdIndex        int
		expectedTimeout int
		description     string
	}{
		{0, 60, "cmd1 should inherit global timeout"},
		{1, 180, "cmd2 should use its own timeout"},
		{2, 0, "cmd3 should use unlimited timeout"},
	}

	for _, tc := range testCases {
		cmdSpec := &groupSpec.Commands[tc.cmdIndex]

		finalRuntimeCmd, err := runnertypes.NewRuntimeCommand(cmdSpec, common.NewFromIntPtr(cfg.Global.Timeout), groupSpec.Name)
		require.NoError(t, err, "Failed to create RuntimeCommand for %s", cmdSpec.Name)

		assert.Equal(t, tc.expectedTimeout, finalRuntimeCmd.EffectiveTimeout, tc.description)
	}
}
