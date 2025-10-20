//go:build test && skip_integration_tests
// +build test,skip_integration_tests

package executor_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// TestBuildProcessEnvironment_Basic tests basic environment variable merging
func TestBuildProcessEnvironment_Basic(t *testing.T) {
	// Set up test environment variables
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SECRET", "should_not_appear")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "PATH"},
		ExpandedEnv: map[string]string{
			"GLOBAL_VAR": "global_value",
		},
	}

	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedEnv: map[string]string{
			"GROUP_VAR": "group_value",
		},
	}

	cmd := &runnertypes.Command{
		Name: "test_cmd",
		ExpandedEnv: map[string]string{
			"CMD_VAR": "cmd_value",
		},
	}

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Verify system env vars (filtered by allowlist)
	assert.Equal(t, "/home/test", result["HOME"])
	assert.Equal(t, "/usr/bin:/bin", result["PATH"])
	assert.NotContains(t, result, "SECRET") // Should be filtered out

	// Verify merged env vars
	assert.Equal(t, "global_value", result["GLOBAL_VAR"])
	assert.Equal(t, "group_value", result["GROUP_VAR"])
	assert.Equal(t, "cmd_value", result["CMD_VAR"])
}

// TestBuildProcessEnvironment_Priority tests the priority order of environment variables
func TestBuildProcessEnvironment_Priority(t *testing.T) {
	t.Setenv("COMMON", "from_system")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"COMMON"},
		ExpandedEnv: map[string]string{
			"COMMON": "from_global",
		},
	}

	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedEnv: map[string]string{
			"COMMON": "from_group", // Should override global
		},
	}

	cmd := &runnertypes.Command{
		Name: "test_cmd",
		ExpandedEnv: map[string]string{
			"COMMON": "from_command", // Should override group
		},
	}

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Command env should have the highest priority
	assert.Equal(t, "from_command", result["COMMON"])
}

// TestBuildProcessEnvironment_AllowlistFiltering tests that only allowlisted vars are included
func TestBuildProcessEnvironment_AllowlistFiltering(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("USER", "testuser")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("SECRET", "secret")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "USER"},
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:        "test_group",
		ExpandedEnv: map[string]string{},
	}

	cmd := &runnertypes.Command{
		Name:        "test_cmd",
		ExpandedEnv: map[string]string{},
	}

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Only allowlisted variables should be included
	assert.Equal(t, "/home/test", result["HOME"])
	assert.Equal(t, "testuser", result["USER"])
	assert.NotContains(t, result, "PATH")
	assert.NotContains(t, result, "SECRET")
}

// TestBuildProcessEnvironment_EmptyEnv tests with empty environment configurations
func TestBuildProcessEnvironment_EmptyEnv(t *testing.T) {
	t.Setenv("HOME", "/home/test")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		ExpandedEnv:  map[string]string{}, // Empty
	}

	group := &runnertypes.CommandGroup{
		Name:        "test_group",
		ExpandedEnv: map[string]string{}, // Empty
	}

	cmd := &runnertypes.Command{
		Name:        "test_cmd",
		ExpandedEnv: map[string]string{}, // Empty
	}

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Only system env should be included
	assert.Equal(t, "/home/test", result["HOME"])
	assert.Len(t, result, 1)
}

// TestBuildProcessEnvironment_GroupAllowlistOverride tests group-level allowlist override
func TestBuildProcessEnvironment_GroupAllowlistOverride(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("USER", "testuser")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "PATH"},
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		EnvAllowlist: []string{"USER"}, // Override global allowlist
		ExpandedEnv:  map[string]string{},
	}

	cmd := &runnertypes.Command{
		Name:        "test_cmd",
		ExpandedEnv: map[string]string{},
	}

	result := executor.BuildProcessEnvironment(global, group, cmd)

	// Only USER should be included (group allowlist takes precedence)
	assert.Equal(t, "testuser", result["USER"])
	assert.NotContains(t, result, "HOME")
	assert.NotContains(t, result, "PATH")
}

// TestBuildProcessEnvironment_NilGroup tests with nil group
func TestBuildProcessEnvironment_NilGroup(t *testing.T) {
	t.Setenv("HOME", "/home/test")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		ExpandedEnv: map[string]string{
			"GLOBAL_VAR": "global_value",
		},
	}

	cmd := &runnertypes.Command{
		Name: "test_cmd",
		ExpandedEnv: map[string]string{
			"CMD_VAR": "cmd_value",
		},
	}

	result := executor.BuildProcessEnvironment(global, nil, cmd)

	// Should work without group
	assert.Equal(t, "/home/test", result["HOME"])
	assert.Equal(t, "global_value", result["GLOBAL_VAR"])
	assert.Equal(t, "cmd_value", result["CMD_VAR"])
}

// TestBuildProcessEnvironment_SystemVarNotInAllowlist tests system var not in allowlist
func TestBuildProcessEnvironment_SystemVarNotInAllowlist(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("PATH", "/usr/bin")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{}, // Empty allowlist
		ExpandedEnv: map[string]string{
			"CUSTOM": "custom_value",
		},
	}

	cmd := &runnertypes.Command{
		Name:        "test_cmd",
		ExpandedEnv: map[string]string{},
	}

	result := executor.BuildProcessEnvironment(global, nil, cmd)

	// No system vars should be included (empty allowlist)
	assert.NotContains(t, result, "HOME")
	assert.NotContains(t, result, "PATH")

	// Only explicitly defined env vars should be included
	assert.Equal(t, "custom_value", result["CUSTOM"])
}
