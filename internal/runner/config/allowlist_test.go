// Package config provides tests for allowlist enforcement functionality.
package config_test

import (
	"fmt"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllowlistViolation_Global tests allowlist enforcement at the global level
func TestAllowlistViolation_Global(t *testing.T) {
	tests := []struct {
		name        string
		fromEnv     []string
		allowlist   []string
		systemEnv   map[string]string
		expectError bool
		errorCheck  func(*testing.T, error) // Custom error validation function
	}{
		{
			name:        "Allowed variable reference - should succeed",
			fromEnv:     []string{"SAFE_VAR=HOME"},
			allowlist:   []string{"HOME", "USER"},
			systemEnv:   map[string]string{"HOME": "/home/user", "USER": "testuser"},
			expectError: false,
		},
		{
			name:        "Disallowed variable reference - should fail",
			fromEnv:     []string{"DANGER=SECRET_KEY"},
			allowlist:   []string{"HOME", "USER"},
			systemEnv:   map[string]string{"SECRET_KEY": "super-secret", "HOME": "/home/user"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
				var detailErr *config.ErrVariableNotInAllowlistDetail
				if assert.ErrorAs(t, err, &detailErr) {
					assert.Equal(t, "SECRET_KEY", detailErr.SystemVarName)
					assert.Equal(t, "DANGER", detailErr.InternalVarName)
					assert.Equal(t, "global", detailErr.Level)
				}
			},
		},
		{
			name:        "Empty allowlist blocks everything - should fail",
			fromEnv:     []string{"VAR=HOME"},
			allowlist:   []string{},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
		{
			name:        "Undefined system variable (allowed name) - should result in empty string",
			fromEnv:     []string{"VAR=NONEXISTENT"},
			allowlist:   []string{"NONEXISTENT"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expectError: false, // No error - just results in empty string
		},
		{
			name:        "Multiple references with one disallowed - should fail",
			fromEnv:     []string{"A=HOME", "B=SECRET"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user", "SECRET": "confidential"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
				var detailErr *config.ErrVariableNotInAllowlistDetail
				if assert.ErrorAs(t, err, &detailErr) {
					assert.Equal(t, "SECRET", detailErr.SystemVarName)
					assert.Equal(t, "B", detailErr.InternalVarName)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create a minimal GlobalConfig
			global := &runnertypes.GlobalConfig{
				FromEnv:      tt.fromEnv,
				EnvAllowlist: tt.allowlist,
			}

			// Create environment filter
			filter := environment.NewFilter(global.EnvAllowlist)

			// Perform global expansion which processes from_env
			err := config.ExpandGlobalConfig(global, filter)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				// Verify that the variable was successfully imported
				if len(tt.fromEnv) > 0 {
					assert.NotNil(t, global.ExpandedVars)
					// Parse the from_env to get the internal variable name
					// Format: "internal_name=SYSTEM_VAR"
					assert.NotEmpty(t, global.ExpandedVars)
				}
			}
		})
	}
}

// TestAllowlistViolation_Group tests allowlist enforcement at the group level with inheritance
func TestAllowlistViolation_Group(t *testing.T) {
	tests := []struct {
		name            string
		globalFromEnv   []string
		globalAllowlist []string
		groupFromEnv    []string // nil means inherit, empty means no system vars
		groupAllowlist  []string
		systemEnv       map[string]string
		expectError     bool
		errorCheck      func(*testing.T, error)
	}{
		{
			name:            "Inherit global allowlist - allowed",
			globalFromEnv:   []string{"home=HOME"},
			globalAllowlist: []string{"HOME", "USER"},
			groupFromEnv:    nil, // inherit
			groupAllowlist:  nil, // inherit
			systemEnv:       map[string]string{"HOME": "/home/user", "USER": "testuser"},
			expectError:     false,
		},
		{
			name:            "Inherit global allowlist - disallowed",
			globalFromEnv:   []string{"home=HOME"},
			globalAllowlist: []string{"HOME"},
			groupFromEnv:    []string{"sec=SECRET"}, // override - tries to access SECRET
			groupAllowlist:  nil,                    // inherit global allowlist (only HOME)
			systemEnv:       map[string]string{"HOME": "/home/user", "SECRET": "confidential"},
			expectError:     true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
				var detailErr *config.ErrVariableNotInAllowlistDetail
				if assert.ErrorAs(t, err, &detailErr) {
					assert.Equal(t, "SECRET", detailErr.SystemVarName)
					assert.Contains(t, detailErr.Level, "group")
				}
			},
		},
		{
			name:            "Override global allowlist - now allowed",
			globalFromEnv:   []string{"home=HOME"},
			globalAllowlist: []string{"HOME"},
			groupFromEnv:    []string{"sec=SECRET"},
			groupAllowlist:  []string{"SECRET"}, // override - now SECRET is allowed
			systemEnv:       map[string]string{"HOME": "/home/user", "SECRET": "confidential"},
			expectError:     false,
		},
		{
			name:            "Override global allowlist - now disallowed",
			globalFromEnv:   []string{"home=HOME"},
			globalAllowlist: []string{"HOME", "SECRET"},
			groupFromEnv:    []string{"home=HOME"}, // try to access HOME
			groupAllowlist:  []string{"SECRET"},    // override - only SECRET allowed now
			systemEnv:       map[string]string{"HOME": "/home/user", "SECRET": "confidential"},
			expectError:     true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
				var detailErr *config.ErrVariableNotInAllowlistDetail
				if assert.ErrorAs(t, err, &detailErr) {
					assert.Equal(t, "HOME", detailErr.SystemVarName)
				}
			},
		},
		{
			name:            "Empty group allowlist blocks everything",
			globalFromEnv:   []string{"home=HOME"},
			globalAllowlist: []string{"HOME"},
			groupFromEnv:    []string{"home=HOME"},
			groupAllowlist:  []string{}, // empty allowlist blocks everything
			systemEnv:       map[string]string{"HOME": "/home/user"},
			expectError:     true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create GlobalConfig
			global := &runnertypes.GlobalConfig{
				FromEnv:      tt.globalFromEnv,
				EnvAllowlist: tt.globalAllowlist,
			}

			// Create environment filter for global
			filter := environment.NewFilter(global.EnvAllowlist)

			// Expand global first
			err := config.ExpandGlobalConfig(global, filter)
			require.NoError(t, err)

			// Create CommandGroup
			group := &runnertypes.CommandGroup{
				Name:         "test_group",
				FromEnv:      tt.groupFromEnv,
				EnvAllowlist: tt.groupAllowlist,
			}

			// Expand group
			err = config.ExpandGroupConfig(group, global, filter)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, group.ExpandedVars)
			}
		})
	}
}

// TestAllowlistViolation_VerifyFiles tests allowlist enforcement in verify_files paths
func TestAllowlistViolation_VerifyFiles(t *testing.T) {
	tests := []struct {
		name        string
		level       string // "global" or "group"
		fromEnv     []string
		vars        []string
		verifyFiles []string
		allowlist   []string
		systemEnv   map[string]string
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name:        "Global verify_files with allowed variable",
			level:       "global",
			fromEnv:     []string{"home=HOME"},
			verifyFiles: []string{"/path/to/%{home}/file"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expectError: false,
		},
		{
			name:        "Global verify_files with disallowed variable",
			level:       "global",
			fromEnv:     []string{"secret=SECRET_KEY"},
			verifyFiles: []string{"/path/%{secret}/file"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"SECRET_KEY": "confidential", "HOME": "/home/user"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
		{
			name:        "Group verify_files with inherited allowlist",
			level:       "group",
			fromEnv:     []string{"user=USER"},
			verifyFiles: []string{"/path/%{user}/file"},
			allowlist:   []string{"USER"},
			systemEnv:   map[string]string{"USER": "testuser"},
			expectError: false,
		},
		{
			name:        "Group verify_files with overridden allowlist",
			level:       "group",
			fromEnv:     []string{"sec=SECRET"},
			verifyFiles: []string{"/path/%{sec}/file"},
			allowlist:   []string{"SECRET"},
			systemEnv:   map[string]string{"SECRET": "confidential", "HOME": "/home/user"},
			expectError: false,
		},
		{
			name:        "Multiple paths with one containing a disallowed variable",
			level:       "global",
			fromEnv:     []string{"home=HOME", "secret=SECRET"},
			verifyFiles: []string{"/a/%{home}/f", "/b/%{secret}/f"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user", "SECRET": "confidential"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
				var detailErr *config.ErrVariableNotInAllowlistDetail
				if assert.ErrorAs(t, err, &detailErr) {
					assert.Equal(t, "SECRET", detailErr.SystemVarName)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			if tt.level == "global" {
				// Test global level
				global := &runnertypes.GlobalConfig{
					FromEnv:      tt.fromEnv,
					Vars:         tt.vars,
					VerifyFiles:  tt.verifyFiles,
					EnvAllowlist: tt.allowlist,
				}

				filter := environment.NewFilter(global.EnvAllowlist)
				err := config.ExpandGlobalConfig(global, filter)

				if tt.expectError {
					require.Error(t, err)
					if tt.errorCheck != nil {
						tt.errorCheck(t, err)
					}
				} else {
					require.NoError(t, err)
					assert.NotNil(t, global.ExpandedVerifyFiles)
				}
			} else {
				// Test group level
				global := &runnertypes.GlobalConfig{
					EnvAllowlist: tt.allowlist,
				}

				filter := environment.NewFilter(global.EnvAllowlist)
				err := config.ExpandGlobalConfig(global, filter)
				require.NoError(t, err)

				group := &runnertypes.CommandGroup{
					Name:         "test_group",
					FromEnv:      tt.fromEnv,
					Vars:         tt.vars,
					VerifyFiles:  tt.verifyFiles,
					EnvAllowlist: tt.allowlist,
				}

				err = config.ExpandGroupConfig(group, global, filter)

				if tt.expectError {
					require.Error(t, err)
					if tt.errorCheck != nil {
						tt.errorCheck(t, err)
					}
				} else {
					require.NoError(t, err)
					assert.NotNil(t, group.ExpandedVerifyFiles)
				}
			}
		})
	}
}

// TestAllowlistViolation_ProcessEnv tests allowlist enforcement when env references variables
func TestAllowlistViolation_ProcessEnv(t *testing.T) {
	tests := []struct {
		name        string
		fromEnv     []string
		vars        []string
		env         []string
		allowlist   []string
		systemEnv   map[string]string
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name:        "env referencing allowed internal variable",
			vars:        []string{"myvar=value"},
			env:         []string{"MY_ENV=%{myvar}"},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: false,
		},
		{
			name:        "env referencing vars from allowed system env",
			fromEnv:     []string{"safe=HOME"},
			env:         []string{"MY_ENV=%{safe}"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expectError: false,
		},
		{
			name:        "env attempting to reference system env directly (should fail during expansion)",
			env:         []string{"MY_ENV=%{HOME}"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				// HOME is not in internal vars, so it fails with undefined variable error
				assert.ErrorIs(t, err, config.ErrUndefinedVariable)
			},
		},
		{
			name:        "Complex chain respecting allowlist",
			fromEnv:     []string{"a=HOME"},
			vars:        []string{"b=%{a}/data"},
			env:         []string{"MY_PATH=%{b}"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			global := &runnertypes.GlobalConfig{
				FromEnv:      tt.fromEnv,
				Vars:         tt.vars,
				Env:          tt.env,
				EnvAllowlist: tt.allowlist,
			}

			filter := environment.NewFilter(global.EnvAllowlist)
			err := config.ExpandGlobalConfig(global, filter)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, global.ExpandedEnv)
			}
		})
	}
}

// TestAllowlistViolation_EdgeCases tests edge cases and complex scenarios for allowlist
func TestAllowlistViolation_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		setupFn     func(*testing.T) (*runnertypes.GlobalConfig, *environment.Filter)
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name: "Case sensitivity",
			setupFn: func(t *testing.T) (*runnertypes.GlobalConfig, *environment.Filter) {
				t.Setenv("HOME", "/home/user")
				t.Setenv("home", "/home/user/lowercase") // Different variable
				global := &runnertypes.GlobalConfig{
					FromEnv:      []string{"myhome=home"}, // Try to access lowercase 'home'
					EnvAllowlist: []string{"HOME"},        // Only uppercase HOME allowed
				}
				filter := environment.NewFilter(global.EnvAllowlist)
				return global, filter
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrVariableNotInAllowlist)
			},
		},
		{
			name: "Allowlist containing special characters",
			setupFn: func(t *testing.T) (*runnertypes.GlobalConfig, *environment.Filter) {
				t.Setenv("MY_VAR_123", "value")
				global := &runnertypes.GlobalConfig{
					FromEnv:      []string{"myvar=MY_VAR_123"},
					EnvAllowlist: []string{"MY_VAR_123"},
				}
				filter := environment.NewFilter(global.EnvAllowlist)
				return global, filter
			},
			expectError: false,
		},
		{
			name: "Long allowlist",
			setupFn: func(t *testing.T) (*runnertypes.GlobalConfig, *environment.Filter) {
				// Set up many env vars
				for i := 0; i < 100; i++ {
					t.Setenv(fmt.Sprintf("VAR_%d", i), fmt.Sprintf("value_%d", i))
				}

				// Create large allowlist
				allowlist := make([]string, 100)
				for i := 0; i < 100; i++ {
					allowlist[i] = fmt.Sprintf("VAR_%d", i)
				}

				global := &runnertypes.GlobalConfig{
					FromEnv:      []string{"var50=VAR_50"},
					EnvAllowlist: allowlist,
				}
				filter := environment.NewFilter(global.EnvAllowlist)
				return global, filter
			},
			expectError: false,
		},
		{
			name: "Allowlist modification - restricted in group",
			setupFn: func(t *testing.T) (*runnertypes.GlobalConfig, *environment.Filter) {
				t.Setenv("HOME", "/home/user")
				t.Setenv("SECRET", "confidential")
				global := &runnertypes.GlobalConfig{
					FromEnv:      []string{"home=HOME", "sec=SECRET"},
					EnvAllowlist: []string{"HOME", "SECRET"}, // Both allowed at global
				}
				filter := environment.NewFilter(global.EnvAllowlist)
				return global, filter
			},
			expectError: false,
		},
		{
			name: "Multiple references to same system variable",
			setupFn: func(t *testing.T) (*runnertypes.GlobalConfig, *environment.Filter) {
				t.Setenv("HOME", "/home/user")
				global := &runnertypes.GlobalConfig{
					FromEnv:      []string{"home1=HOME", "home2=HOME", "home3=HOME"},
					EnvAllowlist: []string{"HOME"},
				}
				filter := environment.NewFilter(global.EnvAllowlist)
				return global, filter
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global, filter := tt.setupFn(t)
			err := config.ExpandGlobalConfig(global, filter)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
