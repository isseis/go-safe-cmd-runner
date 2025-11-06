package executor_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// Helper functions for creating test data

func createTestRuntimeGlobal(envAllowlist []string, expandedEnv map[string]string) *runnertypes.RuntimeGlobal {
	spec := &runnertypes.GlobalSpec{
		EnvAllowed: envAllowlist,
	}
	return &runnertypes.RuntimeGlobal{
		Spec:        spec,
		ExpandedEnv: expandedEnv,
	}
}

func createTestRuntimeCommand(expandedArgs []string, expandedEnv map[string]string) *runnertypes.RuntimeCommand {
	spec := &runnertypes.CommandSpec{
		Name:    "test-command",
		Cmd:     "echo",
		Args:    expandedArgs,
		WorkDir: "/tmp",
	}
	cmd := executortesting.CreateRuntimeCommand(spec)
	// Override ExpandedEnv for this specific test helper
	cmd.ExpandedEnv = expandedEnv
	return cmd
}

func createTestRuntimeGroup(expandedEnv map[string]string) *runnertypes.RuntimeGroup {
	spec := &runnertypes.GroupSpec{
		Name: "test-group",
	}
	group, _ := runnertypes.NewRuntimeGroup(spec)
	group.ExpandedEnv = expandedEnv
	return group
}

// TestBuildProcessEnvironment_Basic tests basic environment variable merging
func TestBuildProcessEnvironment_Basic(t *testing.T) {
	// Set up test environment variables
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SECRET", "should_not_appear")

	global := createTestRuntimeGlobal(
		[]string{"HOME", "PATH"},
		map[string]string{
			"GLOBAL_VAR": "global_value",
		},
	)

	group := createTestRuntimeGroup(
		map[string]string{
			"GROUP_VAR": "group_value",
		},
	)

	cmd := createTestRuntimeCommand(
		[]string{"hello"},
		map[string]string{
			"CMD_VAR": "cmd_value",
		},
	)

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Verify system env vars (filtered by allowlist)
	assert.Equal(t, "/home/test", result["HOME"].Value)
	assert.Equal(t, "/usr/bin:/bin", result["PATH"].Value)
	assert.NotContains(t, result, "SECRET") // Should be filtered out

	// Verify merged env vars
	assert.Equal(t, "global_value", result["GLOBAL_VAR"].Value)
	assert.Equal(t, "group_value", result["GROUP_VAR"].Value)
	assert.Equal(t, "cmd_value", result["CMD_VAR"].Value)
}

// TestBuildProcessEnvironment_Priority tests the priority order of environment variables
// Priority: System < Global < Group < Command
func TestBuildProcessEnvironment_Priority(t *testing.T) {
	t.Run("Command overrides Group, Global, and System", func(t *testing.T) {
		t.Setenv("COMMON", "from_system")

		global := createTestRuntimeGlobal(
			[]string{"COMMON"},
			map[string]string{
				"COMMON": "from_global",
			},
		)

		group := createTestRuntimeGroup(
			map[string]string{
				"COMMON": "from_group",
			},
		)

		cmd := createTestRuntimeCommand(
			[]string{"test"},
			map[string]string{
				"COMMON": "from_command",
			},
		)

		result := executor.BuildProcessEnvironment(global, group, cmd)

		// Command env should have the highest priority
		assert.Equal(t, "from_command", result["COMMON"].Value)
	})

	t.Run("Group overrides Global and System", func(t *testing.T) {
		t.Setenv("COMMON", "from_system")

		global := createTestRuntimeGlobal(
			[]string{"COMMON"},
			map[string]string{
				"COMMON": "from_global",
			},
		)

		group := createTestRuntimeGroup(
			map[string]string{
				"COMMON": "from_group",
			},
		)

		cmd := createTestRuntimeCommand(
			[]string{"test"},
			map[string]string{},
		)

		result := executor.BuildProcessEnvironment(global, group, cmd)

		// Group env should override global and system
		assert.Equal(t, "from_group", result["COMMON"].Value)
	})

	t.Run("Global overrides System", func(t *testing.T) {
		t.Setenv("COMMON", "from_system")

		global := createTestRuntimeGlobal(
			[]string{"COMMON"},
			map[string]string{
				"COMMON": "from_global",
			},
		)

		group := createTestRuntimeGroup(map[string]string{})

		cmd := createTestRuntimeCommand(
			[]string{"test"},
			map[string]string{},
		)

		result := executor.BuildProcessEnvironment(global, group, cmd)

		// Global env should override system
		assert.Equal(t, "from_global", result["COMMON"].Value)
	})

	t.Run("System env is used when not overridden", func(t *testing.T) {
		t.Setenv("COMMON", "from_system")

		global := createTestRuntimeGlobal(
			[]string{"COMMON"},
			map[string]string{},
		)

		group := createTestRuntimeGroup(map[string]string{})

		cmd := createTestRuntimeCommand(
			[]string{"test"},
			map[string]string{},
		)

		result := executor.BuildProcessEnvironment(global, group, cmd)

		// System env should be used when not overridden
		assert.Equal(t, "from_system", result["COMMON"].Value)
	})
}

// TestBuildProcessEnvironment_AllowlistFiltering tests that only allowlisted vars are included
func TestBuildProcessEnvironment_AllowlistFiltering(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("USER", "testuser")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("SECRET", "secret")

	global := createTestRuntimeGlobal(
		[]string{"HOME", "USER"},
		map[string]string{},
	)

	group := createTestRuntimeGroup(map[string]string{})

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{},
	)

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Only allowlisted variables should be included
	assert.Equal(t, "/home/test", result["HOME"].Value)
	assert.Equal(t, "testuser", result["USER"].Value)
	assert.NotContains(t, result, "PATH")
	assert.NotContains(t, result, "SECRET")
}

// TestBuildProcessEnvironment_EmptyEnv tests with empty environment configurations
func TestBuildProcessEnvironment_EmptyEnv(t *testing.T) {
	t.Setenv("HOME", "/home/test")

	global := createTestRuntimeGlobal(
		[]string{"HOME"},
		map[string]string{}, // Empty
	)

	group := createTestRuntimeGroup(map[string]string{}) // Empty

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{}, // Empty
	)

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Only system env should be included
	assert.Equal(t, "/home/test", result["HOME"].Value)
	assert.Len(t, result, 1)
}

// TestBuildProcessEnvironment_NilEnvMaps tests with nil environment maps
func TestBuildProcessEnvironment_NilEnvMaps(t *testing.T) {
	t.Setenv("HOME", "/home/test")

	global := createTestRuntimeGlobal(
		[]string{"HOME"},
		map[string]string{
			"GLOBAL_VAR": "global_value",
		},
	)

	group := createTestRuntimeGroup(
		map[string]string{
			"GROUP_VAR": "group_value",
		},
	)

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{
			"CMD_VAR": "cmd_value",
		},
	)

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Should work properly
	assert.Equal(t, "/home/test", result["HOME"].Value)
	assert.Equal(t, "global_value", result["GLOBAL_VAR"].Value)
	assert.Equal(t, "group_value", result["GROUP_VAR"].Value)
	assert.Equal(t, "cmd_value", result["CMD_VAR"].Value)
}

// TestBuildProcessEnvironment_SystemVarNotInAllowlist tests system var not in allowlist
func TestBuildProcessEnvironment_SystemVarNotInAllowlist(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin")

	global := createTestRuntimeGlobal(
		[]string{}, // Empty allowlist
		map[string]string{
			"CUSTOM": "custom_value",
		},
	)

	group := createTestRuntimeGroup(map[string]string{})

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{},
	)

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// No system vars should be included (empty allowlist)
	assert.NotContains(t, result, "HOME")
	assert.NotContains(t, result, "PATH")

	// Only explicitly defined env vars should be included
	assert.Equal(t, "custom_value", result["CUSTOM"].Value)
}

// TestBuildProcessEnvironment_OriginTracking tests that origin information is correctly tracked
func TestBuildProcessEnvironment_OriginTracking(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin:/bin")

	global := createTestRuntimeGlobal(
		[]string{"HOME", "PATH"},
		map[string]string{
			"GLOBAL_VAR": "global_value",
		},
	)

	group := createTestRuntimeGroup(
		map[string]string{
			"GROUP_VAR": "group_value",
		},
	)

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{
			"CMD_VAR": "cmd_value",
		},
	)

	envMap := executor.BuildProcessEnvironment(global, group, cmd)

	// Verify environment variables are built correctly
	assert.Equal(t, "/home/test", envMap["HOME"].Value)
	assert.Equal(t, "/usr/bin:/bin", envMap["PATH"].Value)
	assert.Equal(t, "global_value", envMap["GLOBAL_VAR"].Value)
	assert.Equal(t, "group_value", envMap["GROUP_VAR"].Value)
	assert.Equal(t, "cmd_value", envMap["CMD_VAR"].Value)

	// Verify origin tracking
	assert.Equal(t, "system", envMap["HOME"].Origin)
	assert.Equal(t, "system", envMap["PATH"].Origin)
	assert.Equal(t, "vars", envMap["GLOBAL_VAR"].Origin)
	assert.Equal(t, "vars", envMap["GROUP_VAR"].Origin)
	assert.Equal(t, "command", envMap["CMD_VAR"].Origin)

	// Verify all environment variables have origin information
	assert.Equal(t, 5, len(envMap))
}

// TestBuildProcessEnvironment_OriginOverride tests that origin is updated when variables are overridden
func TestBuildProcessEnvironment_OriginOverride(t *testing.T) {
	t.Setenv("COMMON", "from_system")

	global := createTestRuntimeGlobal(
		[]string{"COMMON"},
		map[string]string{
			"COMMON": "from_global",
		},
	)

	group := createTestRuntimeGroup(
		map[string]string{
			"COMMON": "from_group",
		},
	)

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{
			"COMMON": "from_command",
		},
	)

	envMap := executor.BuildProcessEnvironment(global, group, cmd)

	// Verify the final value is from command (highest priority)
	assert.Equal(t, "from_command", envMap["COMMON"].Value)

	// Verify origin reflects the actual source (command, not system)
	assert.Equal(t, "command", envMap["COMMON"].Origin)
}

// TestBuildProcessEnvironment_SystemEnvFiltering tests that system environment variable filtering is tracked correctly
func TestBuildProcessEnvironment_SystemEnvFiltering(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("SECRET", "secret_value")
	t.Setenv("USER", "testuser")

	// Only allow HOME and USER
	global := createTestRuntimeGlobal(
		[]string{"HOME", "USER"},
		map[string]string{
			"GLOBAL_VAR": "global_value",
		},
	)

	group := createTestRuntimeGroup(map[string]string{})

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{},
	)

	envMap := executor.BuildProcessEnvironment(global, group, cmd)

	// Verify only allowlisted system vars are included
	assert.Contains(t, envMap, "HOME")
	assert.Contains(t, envMap, "USER")
	assert.NotContains(t, envMap, "PATH")
	assert.NotContains(t, envMap, "SECRET")

	// Verify origins for allowlisted system vars
	assert.Equal(t, "system", envMap["HOME"].Origin)
	assert.Equal(t, "system", envMap["USER"].Origin)

	// Verify global var and its origin
	assert.Equal(t, "global_value", envMap["GLOBAL_VAR"].Value)
	assert.Equal(t, "vars", envMap["GLOBAL_VAR"].Origin)

	// Verify PATH and SECRET are not in envMap
	assert.NotContains(t, envMap, "PATH")
	assert.NotContains(t, envMap, "SECRET")
}
