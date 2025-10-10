package config

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/require"
)

// TestExpandGlobalEnv_SelfReferenceToSystemEnv tests self-reference to system environment variables
func TestExpandGlobalEnv_SelfReferenceToSystemEnv(t *testing.T) {
	// Set up test environment variable
	testVar := "TEST_GLOBAL_PATH"
	testValue := "/original/path"
	originalValue := os.Getenv(testVar)
	os.Setenv(testVar, testValue)
	defer func() {
		if originalValue == "" {
			os.Unsetenv(testVar)
		} else {
			os.Setenv(testVar, originalValue)
		}
	}()

	tests := []struct {
		name      string
		globalEnv []string
		allowlist []string
		expected  map[string]string
	}{
		{
			name:      "self_reference_system_path",
			globalEnv: []string{"TEST_GLOBAL_PATH=/custom:${TEST_GLOBAL_PATH}"},
			allowlist: []string{"TEST_GLOBAL_PATH"},
			expected: map[string]string{
				"TEST_GLOBAL_PATH": "/custom:/original/path",
			},
		},
		{
			name:      "multiple_self_references",
			globalEnv: []string{"TEST_GLOBAL_PATH=/first:${TEST_GLOBAL_PATH}:/last", "CUSTOM_VAR=prefix-${TEST_GLOBAL_PATH}"},
			allowlist: []string{"TEST_GLOBAL_PATH"},
			expected: map[string]string{
				"TEST_GLOBAL_PATH": "/first:/original/path:/last",
				"CUSTOM_VAR":       "prefix-/first:/original/path:/last",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			filter := environment.NewFilter(tt.allowlist)
			expander := environment.NewVariableExpander(filter)
			autoEnv := map[string]string{} // Empty auto env for this test

			err := ExpandGlobalEnv(cfg, expander, autoEnv)
			require.NoError(t, err)

			require.Equal(t, tt.expected, cfg.ExpandedEnv)
		})
	}
}

// TestExpandGroupEnv_SelfReferenceToSystemEnv tests self-reference to system environment variables
func TestExpandGroupEnv_SelfReferenceToSystemEnv(t *testing.T) {
	// Set up test environment variable
	testVar := "TEST_GROUP_PATH"
	testValue := "/group/original"
	originalValue := os.Getenv(testVar)
	os.Setenv(testVar, testValue)
	defer func() {
		if originalValue == "" {
			os.Unsetenv(testVar)
		} else {
			os.Setenv(testVar, originalValue)
		}
	}()

	tests := []struct {
		name      string
		groupEnv  []string
		globalEnv map[string]string
		allowlist []string
		expected  map[string]string
	}{
		{
			name:      "self_reference_system_path",
			groupEnv:  []string{"TEST_GROUP_PATH=/group/custom:${TEST_GROUP_PATH}"},
			globalEnv: map[string]string{},
			allowlist: []string{"TEST_GROUP_PATH"},
			expected: map[string]string{
				"TEST_GROUP_PATH": "/group/custom:/group/original",
			},
		},
		{
			name:      "self_reference_with_global_precedence",
			groupEnv:  []string{"TEST_GROUP_PATH=/group:${TEST_GROUP_PATH}"},
			globalEnv: map[string]string{"TEST_GROUP_PATH": "/global/override"},
			allowlist: []string{"TEST_GROUP_PATH"},
			expected: map[string]string{
				"TEST_GROUP_PATH": "/group:/global/override", // Global env takes precedence over system env
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &runnertypes.CommandGroup{
				Name:         "test_group",
				Env:          tt.groupEnv,
				EnvAllowlist: tt.allowlist,
			}

			filter := environment.NewFilter(tt.allowlist)
			expander := environment.NewVariableExpander(filter)
			autoEnv := map[string]string{} // Empty auto env for this test

			err := ExpandGroupEnv(group, tt.globalEnv, tt.allowlist, expander, autoEnv)
			require.NoError(t, err)

			require.Equal(t, tt.expected, group.ExpandedEnv)
		})
	}
}

// TestSelfReferenceIntegration tests the full integration of self-reference across all levels
func TestSelfReferenceIntegration(t *testing.T) {
	// Set up test environment variable
	testVar := "INTEGRATION_PATH"
	testValue := "/system/path"
	originalValue := os.Getenv(testVar)
	os.Setenv(testVar, testValue)
	defer func() {
		if originalValue == "" {
			os.Unsetenv(testVar)
		} else {
			os.Setenv(testVar, originalValue)
		}
	}()

	tomlContent := []byte(`[global]
env = ["INTEGRATION_PATH=/global:${INTEGRATION_PATH}"]
env_allowlist = ["INTEGRATION_PATH"]

[[groups]]
name = "test_group"
env = ["INTEGRATION_PATH=/group:${INTEGRATION_PATH}:/group_end"]

[[groups.commands]]
name = "test_command"
cmd = "echo"
args = ["${INTEGRATION_PATH}"]
`)

	loader := NewLoader()
	cfg, err := loader.LoadConfig(tomlContent)
	require.NoError(t, err)

	// Check global environment expansion
	require.Contains(t, cfg.Global.ExpandedEnv, "INTEGRATION_PATH")
	require.Equal(t, "/global:/system/path", cfg.Global.ExpandedEnv["INTEGRATION_PATH"])

	// Check group environment expansion
	require.Contains(t, cfg.Groups[0].ExpandedEnv, "INTEGRATION_PATH")
	require.Equal(t, "/group:/global:/system/path:/group_end", cfg.Groups[0].ExpandedEnv["INTEGRATION_PATH"])

	// Note: Command expansion (ExpandedCmd, ExpandedArgs) happens in bootstrap, not in config loader
	// So we only verify that the command structure is correct and environment variables are properly expanded
	cmd := &cfg.Groups[0].Commands[0]
	require.Equal(t, "echo", cmd.Cmd)
	require.Equal(t, []string{"${INTEGRATION_PATH}"}, cmd.Args)

	// The important part is that group environment variables are correctly expanded for command use
	require.Equal(t, "/group:/global:/system/path:/group_end", cfg.Groups[0].ExpandedEnv["INTEGRATION_PATH"])
}

// TestSelfReferenceWithAutomaticVariables tests self-reference in combination with automatic variables
func TestSelfReferenceWithAutomaticVariables(t *testing.T) {
	// Set up test environment variable
	testVar := "TEST_AUTO_PATH"
	testValue := "/auto/system"
	originalValue := os.Getenv(testVar)
	os.Setenv(testVar, testValue)
	defer func() {
		if originalValue == "" {
			os.Unsetenv(testVar)
		} else {
			os.Setenv(testVar, originalValue)
		}
	}()

	cfg := &runnertypes.GlobalConfig{
		Env:          []string{"TEST_AUTO_PATH=/auto:${TEST_AUTO_PATH}:${__RUNNER_PID}"},
		EnvAllowlist: []string{"TEST_AUTO_PATH"},
	}

	filter := environment.NewFilter(cfg.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)
	autoEnv := map[string]string{
		"__RUNNER_PID": "12345",
	}

	err := ExpandGlobalEnv(cfg, expander, autoEnv)
	require.NoError(t, err)

	require.Contains(t, cfg.ExpandedEnv, "TEST_AUTO_PATH")
	require.Equal(t, "/auto:/auto/system:12345", cfg.ExpandedEnv["TEST_AUTO_PATH"])
}
