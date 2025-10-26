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
	if err := os.WriteFile(configPath, []byte(configTOML), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Create hash directory
	hashDir := filepath.Join(tempDir, "hashes")
	if err := os.MkdirAll(hashDir, 0o700); err != nil {
		t.Fatalf("Failed to create hash directory: %v", err)
	}

	// Load and prepare config
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	if err != nil {
		t.Fatalf("Failed to create verification manager: %v", err)
	}

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-env-priority")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	return cfg
}

// envPriorityTestHelper is a helper function to reduce boilerplate in environment priority tests.
// It sets up the test environment, loads config, and verifies expected variables.
func envPriorityTestHelper(t *testing.T, systemEnv map[string]string, configTOML string, expectVars map[string]string) {
	t.Helper()

	cfg := configSetupHelper(t, systemEnv, configTOML)

	// Extract the first command
	if len(cfg.Groups) == 0 || len(cfg.Groups[0].Commands) == 0 {
		t.Fatal("No command found in config")
	}
	cmdSpec := &cfg.Groups[0].Commands[0]

	// Expand configuration to runtime types
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	if err != nil {
		t.Fatalf("Failed to expand global config: %v", err)
	}
	runtimeGroup, err := config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
	if err != nil {
		t.Fatalf("Failed to expand group config: %v", err)
	}
	runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout())
	if err != nil {
		t.Fatalf("Failed to expand command config: %v", err)
	}

	// Call production code to build final environment
	// This tests the actual implementation in executor.BuildProcessEnvironment
	finalEnv := executor.BuildProcessEnvironment(runtimeGlobal, runtimeGroup, runtimeCmd)

	// Verify expected variables
	for k, expectedVal := range expectVars {
		envVar, ok := finalEnv[k]
		if !ok {
			t.Errorf("Variable %s not found in final environment", k)
			continue
		}
		if envVar.Value != expectedVal {
			t.Errorf("Variable %s: expected %q, got %q", k, expectedVal, envVar.Value)
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
	if len(cfg.Groups) == 0 || len(cfg.Groups[0].Commands) == 0 {
		t.Fatal("No command found in config")
	}
	cmdSpec := &cfg.Groups[0].Commands[0]
	groupSpec := &cfg.Groups[0]

	// Expand configuration to runtime types to access ExpandedVars
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	if err != nil {
		t.Fatalf("Failed to expand global config: %v", err)
	}
	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
	if err != nil {
		t.Fatalf("Failed to expand group config: %v", err)
	}
	runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout())
	if err != nil {
		t.Fatalf("Failed to expand command config: %v", err)
	}

	// Verify vars expansion at each level
	if runtimeGlobal.ExpandedVars["base"] != "/home/test/app" {
		t.Errorf("Global vars: expected base=/home/test/app, got %q", runtimeGlobal.ExpandedVars["base"])
	}

	if runtimeGroup.ExpandedVars["rel_path"] != "data" {
		t.Errorf("Group vars: expected rel_path=data, got %q", runtimeGroup.ExpandedVars["rel_path"])
	}

	if runtimeGroup.ExpandedVars["data_dir"] != "/home/test/app/data" {
		t.Errorf("Group vars: expected data_dir=/home/test/app/data, got %q", runtimeGroup.ExpandedVars["data_dir"])
	}

	if runtimeCmd.ExpandedVars["filename"] != "output.txt" {
		t.Errorf("Command vars: expected filename=output.txt, got %q", runtimeCmd.ExpandedVars["filename"])
	}

	if runtimeCmd.ExpandedVars["output"] != "/home/test/app/data/output.txt" {
		t.Errorf("Command vars: expected output=/home/test/app/data/output.txt, got %q", runtimeCmd.ExpandedVars["output"])
	}

	// Verify env expansion
	if runtimeGlobal.ExpandedEnv["APP_BASE"] != "/home/test/app" {
		t.Errorf("Global env: expected APP_BASE=/home/test/app, got %q", runtimeGlobal.ExpandedEnv["APP_BASE"])
	}

	if runtimeGroup.ExpandedEnv["DATA_DIR"] != "/home/test/app/data" {
		t.Errorf("Group env: expected DATA_DIR=/home/test/app/data, got %q", runtimeGroup.ExpandedEnv["DATA_DIR"])
	}

	if runtimeCmd.ExpandedEnv["OUTPUT"] != "/home/test/app/data/output.txt" {
		t.Errorf("Command env: expected OUTPUT=/home/test/app/data/output.txt, got %q", runtimeCmd.ExpandedEnv["OUTPUT"])
	}

	// Verify command args expansion
	if len(runtimeCmd.ExpandedArgs) != 1 {
		t.Fatalf("Expected 1 arg, got %d", len(runtimeCmd.ExpandedArgs))
	}
	if runtimeCmd.ExpandedArgs[0] != "/home/test/app/data" {
		t.Errorf("Command args: expected /home/test/app/data, got %q", runtimeCmd.ExpandedArgs[0])
	}
}
