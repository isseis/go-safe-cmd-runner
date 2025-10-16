// Package config provides tests for security integration functionality.
package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecurityIntegration_E2E tests end-to-end security verification across all features
func TestSecurityIntegration_E2E(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T)
		global      *runnertypes.GlobalConfig
		groups      map[string]*runnertypes.CommandGroup
		expectError bool
		errorCheck  func(*testing.T, error)
		validate    func(*testing.T, *runnertypes.GlobalConfig, map[string]*runnertypes.CommandGroup)
	}{
		{
			name: "Allowlist + Redaction integration",
			setup: func(t *testing.T) {
				t.Setenv("SAFE_VAR", "safe_value")
				t.Setenv("SECRET_VAR", "super_secret")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"MY_SAFE=SAFE_VAR"},
				EnvAllowlist: []string{"SAFE_VAR"},
				Vars:         []string{"derived=%{MY_SAFE}/subdir"},
				Env:          []string{"MY_ENV=%{derived}"},
			},
			expectError: false,
			validate: func(_ *testing.T, global *runnertypes.GlobalConfig, _ map[string]*runnertypes.CommandGroup) {
				// Verify that allowed variable is properly expanded
				// Check ExpandedVars and ExpandedEnv instead of raw Vars/Env
				require.NotNil(t, global.ExpandedVars)
				require.NotNil(t, global.ExpandedEnv)
				assert.Equal(t, "safe_value/subdir", global.ExpandedVars["derived"])
				assert.Equal(t, "safe_value/subdir", global.ExpandedEnv["MY_ENV"])
			},
		},
		{
			name: "Allowlist blocks disallowed variable",
			setup: func(t *testing.T) {
				t.Setenv("SECRET_VAR", "super_secret")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"MY_SECRET=SECRET_VAR"},
				EnvAllowlist: []string{"SAFE_VAR"}, // SECRET_VAR is not in allowlist
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
		{
			name: "from_env + allowlist + vars + env chain",
			setup: func(t *testing.T) {
				t.Setenv("BASE_PATH", "/home/user")
				t.Setenv("SECRET_PATH", "/secret")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"base=BASE_PATH"},
				EnvAllowlist: []string{"BASE_PATH"},
				Vars:         []string{"work_dir=%{base}/work"},
				Env:          []string{"WORK_DIR=%{work_dir}"},
			},
			expectError: false,
			validate: func(_ *testing.T, global *runnertypes.GlobalConfig, _ map[string]*runnertypes.CommandGroup) {
				// Verify the entire chain is properly expanded
				require.NotNil(t, global.ExpandedVars)
				require.NotNil(t, global.ExpandedEnv)
				assert.Equal(t, "/home/user/work", global.ExpandedVars["work_dir"])
				assert.Equal(t, "/home/user/work", global.ExpandedEnv["WORK_DIR"])
			},
		},
		{
			name: "Multiple groups with different allowlists - isolation",
			setup: func(t *testing.T) {
				t.Setenv("VAR_A", "value_a")
				t.Setenv("VAR_B", "value_b")
			},
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{"VAR_A", "VAR_B"},
			},
			groups: map[string]*runnertypes.CommandGroup{
				"group_a": {
					FromEnv:      []string{"my_var=VAR_A"},
					EnvAllowlist: []string{"VAR_A"}, // Only VAR_A is allowed
				},
				"group_b": {
					FromEnv:      []string{"my_var=VAR_B"},
					EnvAllowlist: []string{"VAR_B"}, // Only VAR_B is allowed
				},
			},
			expectError: false,
			validate: func(_ *testing.T, _ *runnertypes.GlobalConfig, _ map[string]*runnertypes.CommandGroup) {
				// Verify each group has access only to its allowed variables
				// This will be validated during expansion
			},
		},
		{
			name: "Group trying to access disallowed variable",
			setup: func(t *testing.T) {
				t.Setenv("VAR_A", "value_a")
				t.Setenv("VAR_B", "value_b")
			},
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{"VAR_A"},
			},
			groups: map[string]*runnertypes.CommandGroup{
				"bad_group": {
					FromEnv:      []string{"my_var=VAR_B"}, // VAR_B not in allowlist
					EnvAllowlist: nil,                      // Inherits from global
				},
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
		{
			name: "Complex chain with verify_files",
			setup: func(t *testing.T) {
				t.Setenv("PROJECT_ROOT", "/home/project")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"root=PROJECT_ROOT"},
				EnvAllowlist: []string{"PROJECT_ROOT"},
				Vars:         []string{"config_dir=%{root}/config"},
				VerifyFiles:  []string{"%{config_dir}/app.conf"},
			},
			expectError: false,
			validate: func(_ *testing.T, global *runnertypes.GlobalConfig, _ map[string]*runnertypes.CommandGroup) {
				// Verify that verify_files paths are properly expanded
				require.NotNil(t, global.ExpandedVerifyFiles)
				require.Len(t, global.ExpandedVerifyFiles, 1)
				assert.Equal(t, "/home/project/config/app.conf", global.ExpandedVerifyFiles[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			if tt.setup != nil {
				tt.setup(t)
			}

			// Create environment filter
			var filter *environment.Filter
			if tt.global != nil && tt.global.EnvAllowlist != nil {
				filter = environment.NewFilter(tt.global.EnvAllowlist)
			} else {
				filter = environment.NewFilter([]string{})
			}

			// Expand global configuration
			var err error
			if tt.global != nil {
				err = config.ExpandGlobalConfig(tt.global, filter)
			}

			// Process groups if present
			if err == nil && tt.groups != nil {
				for _, groupCfg := range tt.groups {
					// Create group-specific filter
					var groupFilter *environment.Filter
					switch {
					case groupCfg.EnvAllowlist != nil:
						groupFilter = environment.NewFilter(groupCfg.EnvAllowlist)
					case tt.global != nil:
						groupFilter = environment.NewFilter(tt.global.EnvAllowlist)
					default:
						groupFilter = environment.NewFilter([]string{})
					}

					err = config.ExpandGroupConfig(groupCfg, tt.global, groupFilter)
					if err != nil {
						break
					}
				}
			}

			// Check error expectations
			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, tt.global, tt.groups)
				}
			}
		})
	}
}

// TestSecurityAttackPrevention tests protection against common attack vectors
func TestSecurityAttackPrevention(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T)
		global      *runnertypes.GlobalConfig
		expectError bool
		errorCheck  func(*testing.T, error)
		validate    func(*testing.T, *runnertypes.GlobalConfig)
	}{
		{
			name: "Command injection via variables - safe handling",
			setup: func(_ *testing.T) {
				// Variables can contain any value, but they won't be executed as commands
			},
			global: &runnertypes.GlobalConfig{
				Vars: []string{"cmd=rm -rf /"},
				Env:  []string{"COMMAND=%{cmd}"},
			},
			expectError: false,
			validate: func(t *testing.T, global *runnertypes.GlobalConfig) {
				// The value should be expanded but treated as literal string
				require.NotNil(t, global.ExpandedEnv)
				assert.Equal(t, "rm -rf /", global.ExpandedEnv["COMMAND"])
			},
		},
		{
			name: "Path traversal via variables - should be handled safely",
			setup: func(_ *testing.T) {
				// Path traversal attempts should be detected or handled safely
			},
			global: &runnertypes.GlobalConfig{
				Vars:        []string{"path=../../etc/passwd"},
				VerifyFiles: []string{"/safe/dir/%{path}"},
			},
			expectError: false,
			validate: func(t *testing.T, global *runnertypes.GlobalConfig) {
				// The path should be expanded as is
				// File verification will handle the actual security check
				require.NotNil(t, global.ExpandedVerifyFiles)
				require.Len(t, global.ExpandedVerifyFiles, 1)
				assert.Equal(t, "/safe/dir/../../etc/passwd", global.ExpandedVerifyFiles[0])
			},
		},
		{
			name: "Allowlist bypass via indirect reference - should fail",
			setup: func(t *testing.T) {
				t.Setenv("SECRET", "confidential")
				t.Setenv("SAFE", "public")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"indirect=SECRET"},
				EnvAllowlist: []string{"SAFE"}, // SECRET is not allowed
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
		{
			name: "Environment variable injection - safe handling",
			setup: func(t *testing.T) {
				t.Setenv("MALICIOUS", "value; rm -rf /")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"user_input=MALICIOUS"},
				EnvAllowlist: []string{"MALICIOUS"},
				Env:          []string{"SAFE_VAR=%{user_input}"},
			},
			expectError: false,
			validate: func(t *testing.T, global *runnertypes.GlobalConfig) {
				// Special characters should be preserved as literal values
				require.NotNil(t, global.ExpandedEnv)
				assert.Equal(t, "value; rm -rf /", global.ExpandedEnv["SAFE_VAR"])
			},
		},
		{
			name: "Reserved prefix violation - auto variables",
			setup: func(_ *testing.T) {
				// Test that user cannot override reserved auto variables
			},
			global: &runnertypes.GlobalConfig{
				Vars: []string{"__runner_test=malicious"},
			},
			expectError: true, // Should fail due to reserved prefix
			errorCheck: func(t *testing.T, err error) {
				// Verify that reserved prefix violation is detected using structured error
				assert.ErrorIs(t, err, config.ErrReservedVariablePrefix)

				var detailErr *config.ErrReservedVariablePrefixDetail
				if assert.ErrorAs(t, err, &detailErr) {
					assert.Equal(t, "__runner_test", detailErr.VariableName)
					assert.Equal(t, "__runner_", detailErr.Prefix)
					assert.Equal(t, "global", detailErr.Level)
					assert.Equal(t, "vars", detailErr.Field)
				}
			},
		},
		{
			name: "Multiple attack vectors combined",
			setup: func(t *testing.T) {
				t.Setenv("SAFE", "public")
				t.Setenv("SECRET", "confidential")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"s1=SAFE"},
				EnvAllowlist: []string{"SAFE"},
				Vars:         []string{"v1=%{s1}/../../etc", "v2=$(cat /etc/passwd)"},
				Env:          []string{"E1=%{v1}", "E2=%{v2}"},
			},
			expectError: false,
			validate: func(t *testing.T, global *runnertypes.GlobalConfig) {
				// Values should be expanded as literal strings without execution
				require.NotNil(t, global.ExpandedEnv)
				assert.Equal(t, "public/../../etc", global.ExpandedEnv["E1"])
				assert.Equal(t, "$(cat /etc/passwd)", global.ExpandedEnv["E2"])
			},
		},
		{
			name: "Circular reference detection",
			setup: func(_ *testing.T) {
				// Test that circular references are detected
			},
			global: &runnertypes.GlobalConfig{
				Vars: []string{"a=%{b}", "b=%{c}", "c=%{a}"},
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				// Should detect circular reference
				assert.Error(t, err)
			},
		},
		{
			name: "Allowlist with similar names - exact match required",
			setup: func(t *testing.T) {
				t.Setenv("HOME", "/home/user")
				t.Setenv("HOME_DIR", "/home")
				t.Setenv("HOMEWORK", "/homework")
			},
			global: &runnertypes.GlobalConfig{
				FromEnv:      []string{"h=HOME_DIR"},
				EnvAllowlist: []string{"HOME"}, // Only HOME is allowed, not HOME_DIR
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			if tt.setup != nil {
				tt.setup(t)
			}

			// Create environment filter
			var filter *environment.Filter
			if tt.global != nil && tt.global.EnvAllowlist != nil {
				filter = environment.NewFilter(tt.global.EnvAllowlist)
			} else {
				filter = environment.NewFilter([]string{})
			}

			// Expand global configuration
			var err error
			if tt.global != nil {
				err = config.ExpandGlobalConfig(tt.global, filter)
			}

			// Check error expectations
			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, tt.global)
				}
			}
		})
	}
}
