package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandCommandEnv_AllowlistInheritance tests that Command.Env allowlist inheritance works correctly
func TestExpandCommandEnv_AllowlistInheritance(t *testing.T) {
	// Set up system environment variables for testing
	t.Setenv("SYSTEM_VAR", "system_value")
	t.Setenv("SYS_VAR2", "sys_var2_value")
	t.Setenv("SYS_VAR3", "sys_var3_value")

	filter := environment.NewFilter([]string{"PATH", "HOME", "USER", "GLOBAL_VAR", "GROUP_VAR", "SYSTEM_VAR", "SYS_VAR1", "SYS_VAR2", "SYS_VAR3"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name            string
		cmd             runnertypes.Command
		group           runnertypes.CommandGroup
		globalAllowlist []string
		expectedVars    map[string]string
		expectError     bool
		description     string
	}{
		{
			name: "group allowlist affects system env references only",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"GLOBAL_VAR=global_value", "GROUP_VAR=${SYSTEM_VAR}/suffix"},
			},
			group: runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"SYSTEM_VAR"}, // Only SYSTEM_VAR is allowed for system env references
			},
			globalAllowlist: []string{"OTHER_VAR"}, // Different global allowlist
			expectedVars: map[string]string{
				"GLOBAL_VAR": "global_value",        // Command.Env variables are always allowed
				"GROUP_VAR":  "system_value/suffix", // Uses group allowlist for SYSTEM_VAR reference
			},
			expectError: false,
			description: "Group allowlist affects system env references, not Command.Env variables themselves",
		},
		{
			name: "nil group allowlist inherits from global allowlist",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"GLOBAL_VAR=global_value", "GROUP_VAR=group_value"},
			},
			group: runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: nil, // No group-specific allowlist
			},
			globalAllowlist: []string{"GLOBAL_VAR", "GROUP_VAR"}, // Global allowlist allows both
			expectedVars: map[string]string{
				"GLOBAL_VAR": "global_value",
				"GROUP_VAR":  "group_value",
			},
			expectError: false,
			description: "When group allowlist is nil, should inherit from global allowlist",
		},
		{
			name: "empty group allowlist blocks system env references only",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"LOCAL_VAR=local_value", "REF_VAR=${SYSTEM_VAR}/suffix"},
			},
			group: runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{}, // Empty allowlist (different from nil)
			},
			globalAllowlist: []string{"SYSTEM_VAR"}, // Global allowlist allows SYSTEM_VAR
			expectedVars: map[string]string{
				"LOCAL_VAR": "local_value", // Command.Env variables are always allowed
				// REF_VAR should fail expansion due to empty group allowlist blocking SYSTEM_VAR
			},
			expectError: true, // Should fail when expanding REF_VAR due to system env restriction
			description: "Empty group allowlist should block system env references, not Command.Env variables",
		},
		{
			name: "group allowlist restricts system env references",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"VAR1=value1", "VAR2=${SYS_VAR2}/suffix", "VAR3=${SYS_VAR3}/suffix"},
			},
			group: runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"SYS_VAR3"}, // Only SYS_VAR3 allowed for system env references
			},
			globalAllowlist: []string{"SYS_VAR1", "SYS_VAR2", "SYS_VAR3"}, // Global allows all
			expectedVars: map[string]string{
				"VAR1": "value1",                // Command.Env variable always allowed
				"VAR3": "sys_var3_value/suffix", // SYS_VAR3 allowed by group allowlist
				// VAR2 should fail due to SYS_VAR2 not in group allowlist
			},
			expectError: true, // Should fail when expanding VAR2 due to SYS_VAR2 restriction
			description: "Group allowlist subset should restrict system env references further than global",
		},
		{
			name: "group allowlist can be more permissive for system env references",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"VAR1=value1", "VAR2=${SYSTEM_VAR}/suffix"},
			},
			group: runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"SYSTEM_VAR"}, // Group allows SYSTEM_VAR
			},
			globalAllowlist: []string{"OTHER_VAR"}, // Global doesn't allow SYSTEM_VAR
			expectedVars: map[string]string{
				"VAR1": "value1",              // Command.Env variable always allowed
				"VAR2": "system_value/suffix", // SYSTEM_VAR allowed by group allowlist
			},
			expectError: false,
			description: "Group allowlist can be more permissive than global allowlist for system env references",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd     // Create a copy to avoid modifying test data
			group := tt.group // Create a copy to avoid modifying test data
			err := ExpandCommandEnv(&cmd, expander, nil, nil, tt.globalAllowlist, nil, group.EnvAllowlist, group.Name)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				return
			}

			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectedVars, cmd.ExpandedEnv, tt.description)
		})
	}
}
