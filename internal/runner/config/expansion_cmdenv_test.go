package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandCommandEnv_WithGlobalEnv tests that Command.Env can reference Global.ExpandedEnv variables
func TestExpandCommandEnv_WithGlobalEnv(t *testing.T) {
	filter := environment.NewFilter([]string{"PATH", "HOME"})
	expander := environment.NewVariableExpander(filter)

	globalEnv := map[string]string{
		"BASE_DIR":    "/opt/app",
		"CONFIG_FILE": "/etc/app.conf",
	}

	tests := []struct {
		name         string
		cmd          runnertypes.Command
		groupName    string
		allowlist    []string
		expectedVars map[string]string
	}{
		{
			name: "reference global env variable",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"WORK_DIR=${BASE_DIR}/work"},
			},
			groupName: "test_group",
			allowlist: []string{"PATH"},
			expectedVars: map[string]string{
				"WORK_DIR": "/opt/app/work",
			},
		},
		{
			name: "reference multiple global env variables",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"DATA_DIR=${BASE_DIR}/data", "CONFIG=${CONFIG_FILE}"},
			},
			groupName: "test_group",
			allowlist: []string{},
			expectedVars: map[string]string{
				"DATA_DIR": "/opt/app/data",
				"CONFIG":   "/etc/app.conf",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd // Create a copy to avoid modifying test data
			err := ExpandCommandEnv(&cmd, expander, nil, globalEnv, tt.allowlist, nil, nil, tt.groupName)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedVars, cmd.ExpandedEnv)
		})
	}
}

// TestExpandCommandEnv_WithGroupEnv tests that Command.Env can reference Group.ExpandedEnv variables
func TestExpandCommandEnv_WithGroupEnv(t *testing.T) {
	filter := environment.NewFilter([]string{"PATH", "HOME"})
	expander := environment.NewVariableExpander(filter)

	groupEnv := map[string]string{
		"DEPLOY_DIR": "/opt/deploy",
		"LOG_LEVEL":  "debug",
	}

	tests := []struct {
		name         string
		cmd          runnertypes.Command
		groupName    string
		allowlist    []string
		expectedVars map[string]string
	}{
		{
			name: "reference group env variable",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"BACKUP_DIR=${DEPLOY_DIR}/backup"},
			},
			groupName: "test_group",
			allowlist: []string{},
			expectedVars: map[string]string{
				"BACKUP_DIR": "/opt/deploy/backup",
			},
		},
		{
			name: "reference multiple group env variables",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"APP_DIR=${DEPLOY_DIR}/app", "VERBOSITY=${LOG_LEVEL}"},
			},
			groupName: "test_group",
			allowlist: []string{},
			expectedVars: map[string]string{
				"APP_DIR":   "/opt/deploy/app",
				"VERBOSITY": "debug",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd // Create a copy to avoid modifying test data
			err := ExpandCommandEnv(&cmd, expander, nil, nil, tt.allowlist, groupEnv, nil, tt.groupName)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedVars, cmd.ExpandedEnv)
		})
	}
}

// TestExpandCommandEnv_WithBothGlobalAndGroupEnv tests that Command.Env can reference both Global and Group env
func TestExpandCommandEnv_WithBothGlobalAndGroupEnv(t *testing.T) {
	filter := environment.NewFilter([]string{"PATH", "HOME"})
	expander := environment.NewVariableExpander(filter)

	globalEnv := map[string]string{
		"BASE_DIR": "/opt/app",
	}

	groupEnv := map[string]string{
		"DEPLOY_DIR": "/opt/app/deploy", // This was already expanded by ExpandGroupEnv
	}

	tests := []struct {
		name         string
		cmd          runnertypes.Command
		groupName    string
		allowlist    []string
		expectedVars map[string]string
	}{
		{
			name: "reference both global and group env",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"BACKUP_DIR=${DEPLOY_DIR}/backup", "LOG_DIR=${BASE_DIR}/logs"},
			},
			groupName: "test_group",
			allowlist: []string{},
			expectedVars: map[string]string{
				"BACKUP_DIR": "/opt/app/deploy/backup",
				"LOG_DIR":    "/opt/app/logs",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd // Create a copy to avoid modifying test data
			err := ExpandCommandEnv(&cmd, expander, nil, globalEnv, tt.allowlist, groupEnv, nil, tt.groupName)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedVars, cmd.ExpandedEnv)
		})
	}
}

// TestExpandCommandEnv_VariableReferencePriority tests the priority order for variable references
func TestExpandCommandEnv_VariableReferencePriority(t *testing.T) {
	filter := environment.NewFilter([]string{"PATH", "SHARED_VAR"})
	expander := environment.NewVariableExpander(filter)

	// Set up test environment with same variable name at different levels
	t.Setenv("SHARED_VAR", "system_value")

	globalEnv := map[string]string{
		"SHARED_VAR": "global_value",
	}

	groupEnv := map[string]string{
		"SHARED_VAR": "group_value",
	}

	tests := []struct {
		name         string
		cmd          runnertypes.Command
		globalEnv    map[string]string
		groupEnv     map[string]string
		allowlist    []string
		expectedVars map[string]string
		description  string
	}{
		{
			name: "group env takes precedence over global env",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"RESULT=${SHARED_VAR}"},
			},
			globalEnv: globalEnv,
			groupEnv:  groupEnv,
			allowlist: []string{"SHARED_VAR"},
			expectedVars: map[string]string{
				"RESULT": "group_value", // Group env wins
			},
			description: "When a variable exists in both global and group env, group env takes precedence",
		},
		{
			name: "global env takes precedence over system env",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"RESULT=${SHARED_VAR}"},
			},
			globalEnv: globalEnv,
			groupEnv:  nil,
			allowlist: []string{"SHARED_VAR"},
			expectedVars: map[string]string{
				"RESULT": "global_value", // Global env wins over system
			},
			description: "When a variable exists in both global env and system env, global env takes precedence",
		},
		{
			name: "system env used when not in global or group env",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"RESULT=${SHARED_VAR}"},
			},
			globalEnv: nil,
			groupEnv:  nil,
			allowlist: []string{"SHARED_VAR"},
			expectedVars: map[string]string{
				"RESULT": "system_value", // System env is used
			},
			description: "When a variable is not in global or group env, system env is used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd // Create a copy to avoid modifying test data
			err := ExpandCommandEnv(&cmd, expander, nil, tt.globalEnv, tt.allowlist, tt.groupEnv, nil, "test_group")
			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectedVars, cmd.ExpandedEnv, tt.description)
		})
	}
}

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
