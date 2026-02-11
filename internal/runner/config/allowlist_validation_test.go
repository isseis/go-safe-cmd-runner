package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllowlist_ViolationAtGlobalLevel tests allowlist violations at the global level
func TestAllowlist_ViolationAtGlobalLevel(t *testing.T) {
	tests := []struct {
		name        string
		spec        *runnertypes.GlobalSpec
		wantErr     bool
		description string
	}{
		{
			name: "from_env with empty allowlist blocks all",
			spec: &runnertypes.GlobalSpec{
				EnvImport:  []string{"MY_VAR=HOME"},
				EnvAllowed: []string{}, // Empty allowlist
			},
			wantErr:     true,
			description: "Empty allowlist should block all system variables",
		},
		{
			name: "from_env with system variable not in allowlist",
			spec: &runnertypes.GlobalSpec{
				EnvImport:  []string{"MY_VAR=HOME"},
				EnvAllowed: []string{"PATH", "USER"}, // HOME not in list
			},
			wantErr:     true,
			description: "System variable not in allowlist should be rejected",
		},
		{
			name: "from_env with system variable in allowlist",
			spec: &runnertypes.GlobalSpec{
				EnvImport:  []string{"MY_VAR=HOME"},
				EnvAllowed: []string{"HOME", "PATH"},
			},
			wantErr:     false,
			description: "System variable in allowlist should be accepted",
		},
		{
			name: "multiple from_env with mixed allowlist status",
			spec: &runnertypes.GlobalSpec{
				EnvImport:  []string{"VAR1=HOME", "VAR2=NOTALLOWED"},
				EnvAllowed: []string{"HOME"},
			},
			wantErr:     true,
			description: "First violation should be reported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			if tt.wantErr {
				require.Error(t, err, tt.description)
				require.ErrorIs(t, err, config.ErrVariableNotInAllowlist, "error should be ErrVariableNotInAllowlist")
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}

// TestAllowlist_ViolationAtGroupLevel tests allowlist violations at the group level
func TestAllowlist_ViolationAtGroupLevel(t *testing.T) {
	tests := []struct {
		name        string
		globalSpec  *runnertypes.GlobalSpec
		groupSpec   *runnertypes.GroupSpec
		wantErr     bool
		description string
	}{
		{
			name: "group from_env with empty global allowlist",
			globalSpec: &runnertypes.GlobalSpec{
				EnvAllowed: []string{}, // Empty allowlist
			},
			groupSpec: &runnertypes.GroupSpec{
				Name:      "test-group",
				EnvImport: []string{"group_var=HOME"},
			},
			wantErr:     true,
			description: "Group-level from_env should respect global allowlist",
		},
		{
			name: "group from_env with system variable not in allowlist",
			globalSpec: &runnertypes.GlobalSpec{
				EnvAllowed: []string{"PATH"},
			},
			groupSpec: &runnertypes.GroupSpec{
				Name:      "test-group",
				EnvImport: []string{"group_var=HOME"},
			},
			wantErr:     true,
			description: "Group-level from_env should check global allowlist",
		},
		{
			name: "group from_env with system variable in allowlist",
			globalSpec: &runnertypes.GlobalSpec{
				EnvAllowed: []string{"HOME", "PATH"},
			},
			groupSpec: &runnertypes.GroupSpec{
				Name:      "test-group",
				EnvImport: []string{"group_var=HOME"},
			},
			wantErr:     false,
			description: "Group-level from_env should succeed with allowed variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First expand global
			globalRuntime, err := config.ExpandGlobal(tt.globalSpec)
			require.NoError(t, err)

			// Then expand group
			_, err = config.ExpandGroup(tt.groupSpec, globalRuntime)
			if tt.wantErr {
				require.Error(t, err, tt.description)
				require.ErrorIs(t, err, config.ErrVariableNotInAllowlist, "error should be ErrVariableNotInAllowlist")
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}

// TestAllowlist_ViolationAtCommandLevel tests allowlist violations at the command level
func TestAllowlist_ViolationAtCommandLevel(t *testing.T) {
	tests := []struct {
		name        string
		globalSpec  *runnertypes.GlobalSpec
		groupSpec   *runnertypes.GroupSpec
		cmdSpec     *runnertypes.CommandSpec
		wantErr     bool
		description string
	}{
		{
			name: "command from_env with empty global allowlist",
			globalSpec: &runnertypes.GlobalSpec{
				EnvAllowed: []string{}, // Empty allowlist
			},
			groupSpec: &runnertypes.GroupSpec{
				Name: "test-group",
			},
			cmdSpec: &runnertypes.CommandSpec{
				Name:      "test-cmd",
				Cmd:       "echo",
				EnvImport: []string{"cmd_var=HOME"},
			},
			wantErr:     true,
			description: "Command-level from_env should respect global allowlist",
		},
		{
			name: "command from_env with system variable not in allowlist",
			globalSpec: &runnertypes.GlobalSpec{
				EnvAllowed: []string{"PATH"},
			},
			groupSpec: &runnertypes.GroupSpec{
				Name: "test-group",
			},
			cmdSpec: &runnertypes.CommandSpec{
				Name:      "test-cmd",
				Cmd:       "echo",
				EnvImport: []string{"cmd_var=HOME"},
			},
			wantErr:     true,
			description: "Command-level from_env should check global allowlist",
		},
		{
			name: "command from_env with system variable in allowlist",
			globalSpec: &runnertypes.GlobalSpec{
				EnvAllowed: []string{"HOME", "PATH"},
			},
			groupSpec: &runnertypes.GroupSpec{
				Name: "test-group",
			},
			cmdSpec: &runnertypes.CommandSpec{
				Name:      "test-cmd",
				Cmd:       "echo",
				EnvImport: []string{"cmd_var=HOME"},
			},
			wantErr:     false,
			description: "Command-level from_env should succeed with allowed variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First expand global
			globalRuntime, err := config.ExpandGlobal(tt.globalSpec)
			require.NoError(t, err)

			// Then expand group
			groupRuntime, err := config.ExpandGroup(tt.groupSpec, globalRuntime)
			require.NoError(t, err)

			// Finally expand command
			_, err = config.ExpandCommand(tt.cmdSpec, nil, groupRuntime, globalRuntime, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
			if tt.wantErr {
				require.Error(t, err, tt.description)
				require.ErrorIs(t, err, config.ErrVariableNotInAllowlist, "error should be ErrVariableNotInAllowlist")
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}

// TestAllowlist_DetailedErrorMessages tests that errors contain detailed information
func TestAllowlist_DetailedErrorMessages(t *testing.T) {
	tests := []struct {
		name            string
		spec            *runnertypes.GlobalSpec
		wantSystemVar   string
		wantInternalVar string
		description     string
	}{
		{
			name: "error includes variable name",
			spec: &runnertypes.GlobalSpec{
				EnvImport:  []string{"MY_VAR=SECRET_VAR"},
				EnvAllowed: []string{"HOME"},
			},
			wantSystemVar:   "SECRET_VAR",
			wantInternalVar: "MY_VAR",
			description:     "Error should include the rejected system variable name",
		},
		{
			name: "error for empty allowlist",
			spec: &runnertypes.GlobalSpec{
				EnvImport:  []string{"VAR=PATH"},
				EnvAllowed: []string{},
			},
			wantSystemVar:   "PATH",
			wantInternalVar: "VAR",
			description:     "Error should include the variable even with empty allowlist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, tt.description)
			require.ErrorIs(t, err, config.ErrVariableNotInAllowlist, "error should be ErrVariableNotInAllowlist")

			// Check detailed error information
			var detailErr *config.ErrVariableNotInAllowlistDetail
			if assert.ErrorAs(t, err, &detailErr) {
				assert.Equal(t, tt.wantSystemVar, detailErr.SystemVarName, "system variable name should match")
				assert.Equal(t, tt.wantInternalVar, detailErr.InternalVarName, "internal variable name should match")
			}
		})
	}
}

// TestAllowlist_EmptyAllowlistBlocksAll tests that an empty allowlist blocks all system variables
func TestAllowlist_EmptyAllowlistBlocksAll(t *testing.T) {
	commonSystemVars := []string{"HOME", "PATH", "USER", "SHELL", "PWD", "TMPDIR"}

	for _, sysVar := range commonSystemVars {
		t.Run("blocks_"+sysVar, func(t *testing.T) {
			spec := &runnertypes.GlobalSpec{
				EnvImport:  []string{"MY_VAR=" + sysVar},
				EnvAllowed: []string{}, // Empty allowlist
			}

			_, err := config.ExpandGlobal(spec)
			require.Error(t, err, "Empty allowlist should block "+sysVar)
			require.ErrorIs(t, err, config.ErrVariableNotInAllowlist, "error should be ErrVariableNotInAllowlist")

			// Verify the error contains the variable name
			var detailErr *config.ErrVariableNotInAllowlistDetail
			if assert.ErrorAs(t, err, &detailErr) {
				assert.Equal(t, sysVar, detailErr.SystemVarName, "system variable name should match")
			}
		})
	}
}

// TestAllowlist_InheritanceAcrossLevels tests that allowlist is properly inherited
func TestAllowlist_InheritanceAcrossLevels(t *testing.T) {
	t.Run("group inherits global allowlist", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			EnvAllowed: []string{"HOME", "PATH"},
		}
		groupSpec := &runnertypes.GroupSpec{
			Name:      "test-group",
			EnvImport: []string{"var=HOME"}, // Should be allowed
		}

		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		_, err = config.ExpandGroup(groupSpec, globalRuntime)
		require.NoError(t, err, "Group should inherit global allowlist")
	})

	t.Run("command inherits global allowlist", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			EnvAllowed: []string{"HOME", "PATH"},
		}
		groupSpec := &runnertypes.GroupSpec{
			Name: "test-group",
		}
		cmdSpec := &runnertypes.CommandSpec{
			Name:      "test-cmd",
			Cmd:       "echo",
			EnvImport: []string{"var=PATH"}, // Should be allowed
		}

		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupRuntime, err := config.ExpandGroup(groupSpec, globalRuntime)
		require.NoError(t, err)
		_, err = config.ExpandCommand(cmdSpec, nil, groupRuntime, globalRuntime, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
		require.NoError(t, err, "Command should inherit global allowlist")
	})
}
