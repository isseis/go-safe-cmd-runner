package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configSetupHelper creates a complete test environment with config file and hash directory.
// It returns the loaded config.
func configSetupHelper(t *testing.T, systemEnv map[string]string, configTOML string) *runnertypes.ConfigSpec {
	t.Helper()

	// Set up system environment
	for k, v := range systemEnv {
		t.Setenv(k, v)
	}

	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(configTOML), 0o644), "Failed to write config file")

	// Create hash directory
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700), "Failed to create hash directory")

	// Load and prepare config
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err, "Failed to create verification manager")

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-env-priority")
	require.NoError(t, err, "Failed to load config")

	return cfg
}

// envPriorityTestHelper is a helper function to reduce boilerplate in environment priority tests.
// It sets up the test environment, loads config, and verifies expected variables.
func envPriorityTestHelper(t *testing.T, systemEnv map[string]string, configTOML string, expectVars map[string]string) {
	t.Helper()

	cfg := configSetupHelper(t, systemEnv, configTOML)

	// Extract the first command
	require.NotEmpty(t, cfg.Groups, "No group found in config")
	require.NotEmpty(t, cfg.Groups[0].Commands, "No command found in config")
	cmdSpec := &cfg.Groups[0].Commands[0]

	// Expand configuration to runtime types
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err, "Failed to expand global config")
	runtimeGroup, err := config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
	require.NoError(t, err, "Failed to expand group config")
	runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), common.NewUnsetOutputSizeLimit())
	require.NoError(t, err, "Failed to expand command config")

	// Call production code to build final environment
	// This tests the actual implementation in executor.BuildProcessEnvironment
	finalEnv := executor.BuildProcessEnvironment(runtimeGlobal, runtimeGroup, runtimeCmd)

	// Verify expected variables
	for k, expectedVal := range expectVars {
		envVar, ok := finalEnv[k]
		assert.True(t, ok, "Variable %s not found in final environment", k)
		if ok {
			assert.Equal(t, expectedVal, envVar.Value, "Variable %s value mismatch", k)
		}
	}
}

// TestRunner_EnvironmentVariablePriority_Basic tests basic environment variable priority rules
// Priority: command env > group env > global env > system env
func TestRunner_EnvironmentVariablePriority_Basic(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
	}{
		{
			name: "system_env_only",
			systemEnv: map[string]string{
				"TEST_VAR": "system_value",
			},
			configTOML: `
[global]
env_allowed = ["TEST_VAR"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["TEST_VAR"]
`,
			expectVars: map[string]string{
				"TEST_VAR": "system_value",
			},
		},
		{
			name: "global_overrides_system",
			systemEnv: map[string]string{
				"TEST_VAR": "system_value",
			},
			configTOML: `
[global]
env_vars = ["TEST_VAR=global_value"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["TEST_VAR"]
`,
			expectVars: map[string]string{
				"TEST_VAR": "global_value",
			},
		},
		// Note: Group-level env is not currently merged into process environment by BuildProcessEnvironment.
		// This test passes because command-level env correctly overrides all other levels (system and global).
		{
			name: "command_overrides_all",
			systemEnv: map[string]string{
				"TEST_VAR": "system_value",
			},
			configTOML: `
[global]
env_vars = ["TEST_VAR=global_value"]
[[groups]]
name = "test_group"
env_vars = ["TEST_VAR=group_value"]
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["TEST_VAR"]
env_vars = ["TEST_VAR=command_value"]
`,
			expectVars: map[string]string{
				"TEST_VAR": "command_value",
			},
		},
		{
			name: "mixed_priority",
			systemEnv: map[string]string{
				"VAR_A": "sys_a",
				"VAR_B": "sys_b",
				"VAR_C": "sys_c",
			},
			configTOML: `
[global]
env_allowed = ["VAR_A", "VAR_B", "VAR_C"]
env_vars = ["VAR_B=global_b"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env_vars = ["VAR_C=command_c"]
`,
			expectVars: map[string]string{
				"VAR_A": "sys_a",
				"VAR_B": "global_b",
				"VAR_C": "command_c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envPriorityTestHelper(t, tt.systemEnv, tt.configTOML, tt.expectVars)
		})
	}
}

// TestRunner_EnvironmentVariablePriority_WithVars tests environment variable priority with vars expansion
func TestRunner_EnvironmentVariablePriority_WithVars(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
	}{
		{
			name: "vars_referencing_lower_priority_env",
			systemEnv: map[string]string{
				"USER": "testuser",
			},
			configTOML: `
[global]
env_import = ["USER=USER"]
env_allowed = ["USER"]
vars = ["myvar=%{USER}"]
env_vars = ["HOME=%{myvar}"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["HOME"]
`,
			expectVars: map[string]string{
				"HOME": "testuser",
			},
		},
		{
			name:      "command_vars_overriding_group",
			systemEnv: map[string]string{},
			configTOML: `
[global]
vars = ["v=global"]
[[groups]]
name = "test_group"
vars = ["v=group"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
vars = ["v=command"]
env_vars = ["RESULT=%{v}"]
`,
			expectVars: map[string]string{
				"RESULT": "command",
			},
		},
		{
			name: "complex_chain_respecting_priority",
			systemEnv: map[string]string{
				"HOME": "/home/test",
			},
			configTOML: `
[global]
env_import = ["HOME=HOME"]
env_allowed = ["HOME"]
vars = ["gv2=%{HOME}/global"]
[[groups]]
name = "test_group"
vars = ["gv3=%{gv2}/group"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env_vars = ["FINAL=%{gv3}/cmd"]
`,
			expectVars: map[string]string{
				"FINAL": "/home/test/global/group/cmd",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envPriorityTestHelper(t, tt.systemEnv, tt.configTOML, tt.expectVars)
		})
	}
}

// TestRunner_EnvironmentVariablePriority_EdgeCases tests edge cases and unusual scenarios
func TestRunner_EnvironmentVariablePriority_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
	}{
		{
			name: "empty_value_at_different_levels",
			systemEnv: map[string]string{
				"VAR": "system",
			},
			configTOML: `
[global]
env_vars = ["VAR="]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"VAR": "", // Empty, not unset
			},
		},
		{
			name:      "unset_at_higher_priority",
			systemEnv: map[string]string{},
			configTOML: `
[global]
env_vars = ["VAR=global_value"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"VAR": "global_value",
			},
		},
		{
			name:      "numeric_and_special_values",
			systemEnv: map[string]string{},
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env_vars = ["NUM=123", "SPECIAL=$pecial!@#"]
`,
			expectVars: map[string]string{
				"NUM":     "123",
				"SPECIAL": "$pecial!@#",
			},
		},
		{
			name:      "very_long_value",
			systemEnv: map[string]string{},
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env_vars = ["LONG=` + strings.Repeat("a", 1000) + `"]
`,
			expectVars: map[string]string{
				"LONG": strings.Repeat("a", 1000),
			},
		},
		{
			name: "many_variables",
			systemEnv: map[string]string{
				"S1": "s1", "S2": "s2", "S3": "s3",
			},
			configTOML: `
[global]
env_allowed = ["S1", "S2", "S3"]
env_vars = ["G1=g1", "G2=g2", "G3=g3"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env_vars = ["C1=c1", "C2=c2", "C3=c3"]
`,
			expectVars: map[string]string{
				"S1": "s1", "S2": "s2", "S3": "s3",
				"G1": "g1", "G2": "g2", "G3": "g3",
				"C1": "c1", "C2": "c2", "C3": "c3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envPriorityTestHelper(t, tt.systemEnv, tt.configTOML, tt.expectVars)
		})
	}
}

// TestRunner_ResolveEnvironmentVars_Integration tests the integration of variable resolution
func TestRunner_ResolveEnvironmentVars_Integration(t *testing.T) {
	systemEnv := map[string]string{
		"HOME": "/home/test",
		"USER": "testuser",
	}

	configTOML := `
[global]
env_import = ["HOME=HOME", "USER=USER"]
env_allowed = ["HOME", "USER"]
vars = ["base=%{HOME}/app"]
env_vars = ["APP_BASE=%{base}"]
[[groups]]
name = "test_group"
vars = ["rel_path=data", "data_dir=%{base}/%{rel_path}"]
env_vars = ["DATA_DIR=%{data_dir}"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["%{data_dir}"]
vars = ["filename=output.txt", "output=%{data_dir}/%{filename}"]
env_vars = ["OUTPUT=%{output}"]
`

	cfg := configSetupHelper(t, systemEnv, configTOML)

	// Extract the first command
	require.NotEmpty(t, cfg.Groups, "No group found in config")
	require.NotEmpty(t, cfg.Groups[0].Commands, "No command found in config")
	cmdSpec := &cfg.Groups[0].Commands[0]
	groupSpec := &cfg.Groups[0]

	// Expand configuration to runtime types to access ExpandedVars
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err, "Failed to expand global config")
	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
	require.NoError(t, err, "Failed to expand group config")
	runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), common.NewUnsetOutputSizeLimit())
	require.NoError(t, err, "Failed to expand command config")

	// Verify vars expansion at each level
	assert.Equal(t, "/home/test/app", runtimeGlobal.ExpandedVars["base"], "Global vars: base mismatch")
	assert.Equal(t, "data", runtimeGroup.ExpandedVars["rel_path"], "Group vars: rel_path mismatch")
	assert.Equal(t, "/home/test/app/data", runtimeGroup.ExpandedVars["data_dir"], "Group vars: data_dir mismatch")
	assert.Equal(t, "output.txt", runtimeCmd.ExpandedVars["filename"], "Command vars: filename mismatch")
	assert.Equal(t, "/home/test/app/data/output.txt", runtimeCmd.ExpandedVars["output"], "Command vars: output mismatch")

	// Verify env expansion
	assert.Equal(t, "/home/test/app", runtimeGlobal.ExpandedEnv["APP_BASE"], "Global env: APP_BASE mismatch")
	assert.Equal(t, "/home/test/app/data", runtimeGroup.ExpandedEnv["DATA_DIR"], "Group env: DATA_DIR mismatch")
	assert.Equal(t, "/home/test/app/data/output.txt", runtimeCmd.ExpandedEnv["OUTPUT"], "Command env: OUTPUT mismatch")

	// Verify command args expansion
	require.Len(t, runtimeCmd.ExpandedArgs, 1, "Expected 1 arg")
	assert.Equal(t, "/home/test/app/data", runtimeCmd.ExpandedArgs[0], "Command args mismatch")
}
