package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
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
			err := config.ExpandCommandEnv(&cmd, expander, nil, globalEnv, tt.allowlist, nil, nil, tt.groupName)
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
			err := config.ExpandCommandEnv(&cmd, expander, nil, nil, tt.allowlist, groupEnv, nil, tt.groupName)
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
			err := config.ExpandCommandEnv(&cmd, expander, nil, globalEnv, tt.allowlist, groupEnv, nil, tt.groupName)
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
			err := config.ExpandCommandEnv(&cmd, expander, nil, tt.globalEnv, tt.allowlist, tt.groupEnv, nil, "test_group")
			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectedVars, cmd.ExpandedEnv, tt.description)
		})
	}
}
