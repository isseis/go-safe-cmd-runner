//go:build test

package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandGlobal_EnvImportVarsConflict tests conflict detection at global level
func TestExpandGlobal_EnvImportVarsConflict(t *testing.T) {
	spec := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"PATH"},
		EnvImport:  []string{"My_path=PATH"},
		Vars: map[string]any{
			"My_path": "/custom/path", // Conflicts with env_import
		},
	}

	_, err := config.ExpandGlobal(spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrEnvImportVarsConflict)
}

// TestExpandGroup_EnvImportVarsConflict tests conflict detection at group level
func TestExpandGroup_EnvImportVarsConflict(t *testing.T) {
	// Setup global
	globalSpec := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"PATH"},
	}
	globalRuntime, err := config.ExpandGlobal(globalSpec)
	require.NoError(t, err)

	// Group defines both env_import and vars with same variable name
	groupSpec := &runnertypes.GroupSpec{
		Name:       "test_group",
		EnvAllowed: []string{"USER"},
		EnvImport:  []string{"my_var=USER"},
		Vars: map[string]any{
			"my_var": "override_value", // Conflicts with group env_import
		},
		Commands: []runnertypes.CommandSpec{},
	}

	_, err = config.ExpandGroup(groupSpec, globalRuntime)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrEnvImportVarsConflict)
	assert.Contains(t, err.Error(), "my_var")
}

// TestExpandGroup_SameLevelConflict tests conflict at same group level
func TestExpandGroup_SameLevelConflict(t *testing.T) {
	globalSpec := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"PATH"},
	}
	globalRuntime, err := config.ExpandGlobal(globalSpec)
	require.NoError(t, err)

	// Group defines both env_import and vars with same name
	groupSpec := &runnertypes.GroupSpec{
		Name:       "test_group",
		EnvAllowed: []string{"USER"},
		EnvImport:  []string{"my_user=USER"},
		Vars: map[string]any{
			"my_user": "custom_user", // Conflicts with group-level env_import
		},
		Commands: []runnertypes.CommandSpec{},
	}

	_, err = config.ExpandGroup(groupSpec, globalRuntime)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrEnvImportVarsConflict)
	assert.Contains(t, err.Error(), "my_user")
}

// TestExpandGlobal_NoConflict tests that different variable names don't conflict
func TestExpandGlobal_NoConflict(t *testing.T) {
	spec := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"PATH"},
		EnvImport:  []string{"SysPath=PATH"},
		Vars: map[string]any{
			"CustomPath": "/custom/path", // Different name - no conflict
		},
	}

	runtime, err := config.ExpandGlobal(spec)
	require.NoError(t, err)
	assert.NotNil(t, runtime)

	// Verify both variables exist
	assert.Contains(t, runtime.ExpandedVars, "SysPath")
	assert.Contains(t, runtime.ExpandedVars, "CustomPath")
	assert.Equal(t, "/custom/path", runtime.ExpandedVars["CustomPath"])
}

// TestExpandGroup_InheritedEnvImportVarsTracking tests that env_import tracking is inherited
func TestExpandGroup_InheritedEnvImportVarsTracking(t *testing.T) {
	// Global defines env_import
	globalSpec := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"PATH", "USER"},
		EnvImport:  []string{"GlobalPath=PATH"},
	}
	globalRuntime, err := config.ExpandGlobal(globalSpec)
	require.NoError(t, err)

	// Group adds its own env_import
	groupSpec := &runnertypes.GroupSpec{
		Name:       "test_group",
		EnvAllowed: []string{"HOME"},
		EnvImport:  []string{"group_home=HOME"},
		Vars: map[string]any{
			"group_var": "value",
		},
		Commands: []runnertypes.CommandSpec{},
	}

	groupRuntime, err := config.ExpandGroup(groupSpec, globalRuntime)
	require.NoError(t, err)

	// Verify EnvImportVars contains both global and group level variables
	assert.Contains(t, groupRuntime.EnvImportVars, "GlobalPath")
	assert.Contains(t, groupRuntime.EnvImportVars, "group_home")
	assert.NotContains(t, groupRuntime.EnvImportVars, "group_var") // vars should not be in EnvImportVars
}
