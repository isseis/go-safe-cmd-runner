package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/require"
)

// TestAllowlistViolation_Global tests allowlist violations at Global.Env level
func TestAllowlistViolation_Global(t *testing.T) {
	filter := environment.NewFilter(nil)
	expander := environment.NewVariableExpander(filter)

	// Set up test environment variables
	t.Setenv("HOME", "/home/test")
	t.Setenv("USER", "testuser")
	t.Setenv("FORBIDDEN_VAR", "forbidden_value")
	t.Setenv("SECRET_KEY", "secret_value")

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expectError bool
	}{
		{
			name:        "allowed variable",
			globalEnv:   []string{"TEST_VAR=${HOME}"},
			allowlist:   []string{"HOME", "USER"},
			expectError: false,
		},
		{
			name:        "forbidden variable",
			globalEnv:   []string{"TEST_VAR=${FORBIDDEN_VAR}"},
			allowlist:   []string{"HOME", "USER"},
			expectError: true,
		},
		{
			name:        "empty allowlist blocks all",
			globalEnv:   []string{"TEST_VAR=${HOME}"},
			allowlist:   []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAllowlistViolation_Group tests allowlist violations at Group.Env level
func TestAllowlistViolation_Group(t *testing.T) {
	filter := environment.NewFilter(nil)
	expander := environment.NewVariableExpander(filter)

	// Set up test environment variables
	t.Setenv("HOME", "/home/test")
	t.Setenv("USER", "testuser")
	t.Setenv("FORBIDDEN_VAR", "forbidden_value")

	tests := []struct {
		name            string
		groupEnv        []string
		groupAllowlist  []string
		globalAllowlist []string
		expectError     bool
	}{
		{
			name:            "inherit allowlist - allowed",
			groupEnv:        []string{"TEST_VAR=${HOME}"},
			groupAllowlist:  nil, // inherit from global
			globalAllowlist: []string{"HOME", "USER"},
			expectError:     false,
		},
		{
			name:            "inherit allowlist - forbidden",
			groupEnv:        []string{"TEST_VAR=${FORBIDDEN_VAR}"},
			groupAllowlist:  nil, // inherit from global
			globalAllowlist: []string{"HOME", "USER"},
			expectError:     true,
		},
		{
			name:            "override allowlist - allowed",
			groupEnv:        []string{"TEST_VAR=${FORBIDDEN_VAR}"},
			groupAllowlist:  []string{"FORBIDDEN_VAR"}, // explicit override
			globalAllowlist: []string{"HOME", "USER"},
			expectError:     false,
		},
		{
			name:            "override allowlist - forbidden",
			groupEnv:        []string{"TEST_VAR=${HOME}"},
			groupAllowlist:  []string{"FORBIDDEN_VAR"}, // explicit override
			globalAllowlist: []string{"HOME", "USER"},
			expectError:     true,
		},
		{
			name:            "empty allowlist rejects all",
			groupEnv:        []string{"TEST_VAR=${HOME}"},
			groupAllowlist:  []string{}, // reject all
			globalAllowlist: []string{"HOME", "USER"},
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &runnertypes.CommandGroup{
				Name:         "test_group",
				Env:          tt.groupEnv,
				EnvAllowlist: tt.groupAllowlist,
			}

			globalEnv := make(map[string]string)

			err := config.ExpandGroupEnv(group, expander, nil, globalEnv, tt.globalAllowlist)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAllowlistViolation_Command tests allowlist violations at Command.Env level
func TestAllowlistViolation_Command(t *testing.T) {
	filter := environment.NewFilter(nil)
	expander := environment.NewVariableExpander(filter)

	// Set up test environment variables
	t.Setenv("HOME", "/home/test")
	t.Setenv("USER", "testuser")
	t.Setenv("FORBIDDEN_VAR", "forbidden_value")

	tests := []struct {
		name            string
		commandEnv      []string
		groupAllowlist  []string
		globalAllowlist []string
		expectError     bool
	}{
		{
			name:            "allowed with group allowlist",
			commandEnv:      []string{"TEST_VAR=${HOME}"},
			groupAllowlist:  []string{"HOME", "USER"},
			globalAllowlist: []string{"HOME"},
			expectError:     false,
		},
		{
			name:            "forbidden with group allowlist",
			commandEnv:      []string{"TEST_VAR=${FORBIDDEN_VAR}"},
			groupAllowlist:  []string{"HOME", "USER"},
			globalAllowlist: []string{"HOME"},
			expectError:     true,
		},
		{
			name:            "inherit from global allowlist",
			commandEnv:      []string{"TEST_VAR=${HOME}"},
			groupAllowlist:  nil, // inherit from global
			globalAllowlist: []string{"HOME", "USER"},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &runnertypes.Command{
				Name: "test_cmd",
				Env:  tt.commandEnv,
			}

			globalEnv := make(map[string]string)
			groupEnv := make(map[string]string)

			err := config.ExpandCommandEnv(
				cmd,
				expander,
				nil, // autoEnv
				globalEnv,
				tt.globalAllowlist,
				groupEnv,
				tt.groupAllowlist,
				"test_group",
			)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAllowlistViolation_VerifyFiles tests allowlist violations in VerifyFiles expansion
func TestAllowlistViolation_VerifyFiles(t *testing.T) {
	filter := environment.NewFilter(nil)
	expander := environment.NewVariableExpander(filter)

	// Set up test environment variables
	t.Setenv("HOME", "/home/test")
	t.Setenv("USER", "testuser")
	t.Setenv("FORBIDDEN_VAR", "/forbidden/path")

	t.Run("global verify_files with forbidden var", func(t *testing.T) {
		cfg := &runnertypes.GlobalConfig{
			VerifyFiles:  []string{"${FORBIDDEN_VAR}/verify.sh"},
			EnvAllowlist: []string{"HOME", "USER"},
		}

		err := config.ExpandGlobalVerifyFiles(cfg, filter, expander)
		require.Error(t, err)
	})

	t.Run("global verify_files with allowed var", func(t *testing.T) {
		cfg := &runnertypes.GlobalConfig{
			VerifyFiles:  []string{"${HOME}/verify.sh"},
			EnvAllowlist: []string{"HOME", "USER"},
		}

		err := config.ExpandGlobalVerifyFiles(cfg, filter, expander)
		require.NoError(t, err)
	})

	t.Run("group verify_files with empty allowlist", func(t *testing.T) {
		group := &runnertypes.CommandGroup{
			Name:         "test_group",
			VerifyFiles:  []string{"${HOME}/verify.sh"},
			EnvAllowlist: []string{}, // reject all
		}

		globalConfig := &runnertypes.GlobalConfig{
			EnvAllowlist: []string{"HOME", "USER"},
		}

		err := config.ExpandGroupVerifyFiles(group, globalConfig, filter, expander)
		require.Error(t, err)
	})
}

// TestAllowlistViolation_EmptyAllowlist tests that empty allowlist rejects all system variables
func TestAllowlistViolation_EmptyAllowlist(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	// Set up test environment variable
	t.Setenv("HOME", "/home/test")

	t.Run("global env with empty allowlist", func(t *testing.T) {
		cfg := &runnertypes.GlobalConfig{
			Env:          []string{"TEST_VAR=${HOME}"},
			EnvAllowlist: []string{}, // empty allowlist rejects all
		}

		err := config.ExpandGlobalEnv(cfg, expander, nil)
		require.Error(t, err)
	})

	t.Run("group env with empty allowlist override", func(t *testing.T) {
		group := &runnertypes.CommandGroup{
			Name:         "test_group",
			Env:          []string{"TEST_VAR=${HOME}"},
			EnvAllowlist: []string{}, // explicit empty allowlist
		}

		globalEnv := make(map[string]string)
		globalAllowlist := []string{"HOME", "USER"} // global allows HOME

		err := config.ExpandGroupEnv(group, expander, nil, globalEnv, globalAllowlist)
		require.Error(t, err)
	})
}
