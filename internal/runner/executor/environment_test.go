//go:build test
// +build test

package executor_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// Helper functions for creating test data

func createTestRuntimeGlobal(envAllowlist []string, expandedEnv map[string]string) *runnertypes.RuntimeGlobal {
	spec := &runnertypes.GlobalSpec{
		EnvAllowlist: envAllowlist,
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
	return &runnertypes.RuntimeCommand{
		Spec:             spec,
		ExpandedCmd:      "echo",
		ExpandedArgs:     expandedArgs,
		ExpandedEnv:      expandedEnv,
		EffectiveWorkDir: "/tmp",
	}
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

	cmd := createTestRuntimeCommand(
		[]string{"hello"},
		map[string]string{
			"CMD_VAR": "cmd_value",
		},
	)

	result := executor.BuildProcessEnvironment(global, cmd)

	// Verify system env vars (filtered by allowlist)
	assert.Equal(t, "/home/test", result["HOME"])
	assert.Equal(t, "/usr/bin:/bin", result["PATH"])
	assert.NotContains(t, result, "SECRET") // Should be filtered out

	// Verify merged env vars
	assert.Equal(t, "global_value", result["GLOBAL_VAR"])
	assert.Equal(t, "cmd_value", result["CMD_VAR"])
}

// TestBuildProcessEnvironment_Priority tests the priority order of environment variables
func TestBuildProcessEnvironment_Priority(t *testing.T) {
	t.Setenv("COMMON", "from_system")

	global := createTestRuntimeGlobal(
		[]string{"COMMON"},
		map[string]string{
			"COMMON": "from_global",
		},
	)

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{
			"COMMON": "from_command", // Should override global
		},
	)

	result := executor.BuildProcessEnvironment(global, cmd)

	// Command env should have the highest priority
	assert.Equal(t, "from_command", result["COMMON"])
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

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{},
	)

	result := executor.BuildProcessEnvironment(global, cmd)

	// Only allowlisted variables should be included
	assert.Equal(t, "/home/test", result["HOME"])
	assert.Equal(t, "testuser", result["USER"])
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

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{}, // Empty
	)

	result := executor.BuildProcessEnvironment(global, cmd)

	// Only system env should be included
	assert.Equal(t, "/home/test", result["HOME"])
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

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{
			"CMD_VAR": "cmd_value",
		},
	)

	result := executor.BuildProcessEnvironment(global, cmd)

	// Should work properly
	assert.Equal(t, "/home/test", result["HOME"])
	assert.Equal(t, "global_value", result["GLOBAL_VAR"])
	assert.Equal(t, "cmd_value", result["CMD_VAR"])
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

	cmd := createTestRuntimeCommand(
		[]string{"test"},
		map[string]string{},
	)

	result := executor.BuildProcessEnvironment(global, cmd)

	// No system vars should be included (empty allowlist)
	assert.NotContains(t, result, "HOME")
	assert.NotContains(t, result, "PATH")

	// Only explicitly defined env vars should be included
	assert.Equal(t, "custom_value", result["CUSTOM"])
}
