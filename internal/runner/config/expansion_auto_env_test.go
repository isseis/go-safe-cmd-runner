package config

import (
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/require"
)

// TestExpandGlobalEnv_AutomaticEnvironmentVariables tests that ExpandGlobalEnv correctly handles automatic environment variables
func TestExpandGlobalEnv_AutomaticEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name      string
		globalEnv []string
		autoEnv   map[string]string
		expected  map[string]string
	}{
		{
			name:      "reference_automatic_variables_in_global_env",
			globalEnv: []string{"TIMESTAMP=${__RUNNER_DATETIME}", "PROCESS_ID=${__RUNNER_PID}"},
			autoEnv: map[string]string{
				"__RUNNER_DATETIME": "20241015103025.123",
				"__RUNNER_PID":      "12345",
			},
			expected: map[string]string{
				"TIMESTAMP":  "20241015103025.123",
				"PROCESS_ID": "12345",
			},
		},
		{
			name:      "mix_automatic_and_regular_variables",
			globalEnv: []string{"OUTPUT_FILE=output-${__RUNNER_DATETIME}.log", "DEBUG_MODE=true"},
			autoEnv: map[string]string{
				"__RUNNER_DATETIME": "20241015103025.456",
			},
			expected: map[string]string{
				"OUTPUT_FILE": "output-20241015103025.456.log",
				"DEBUG_MODE":  "true",
			},
		},
		{
			name:      "automatic_variables_with_complex_references",
			globalEnv: []string{"BACKUP_DIR=/backups/${__RUNNER_PID}", "BACKUP_FILE=${BACKUP_DIR}/backup-${__RUNNER_DATETIME}.tar.gz"},
			autoEnv: map[string]string{
				"__RUNNER_DATETIME": "20241015103025.789",
				"__RUNNER_PID":      "67890",
			},
			expected: map[string]string{
				"BACKUP_DIR":  "/backups/67890",
				"BACKUP_FILE": "/backups/67890/backup-20241015103025.789.tar.gz",
			},
		},
		{
			name:      "empty_automatic_environment",
			globalEnv: []string{"SIMPLE_VAR=value"},
			autoEnv:   nil,
			expected: map[string]string{
				"SIMPLE_VAR": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: []string{}, // Empty allowlist for simplicity
			}

			filter := environment.NewFilter([]string{})
			expander := environment.NewVariableExpander(filter)

			err := ExpandGlobalEnv(cfg, expander, tt.autoEnv)
			require.NoError(t, err)

			require.Equal(t, tt.expected, cfg.ExpandedEnv)
		})
	}
}

// TestExpandGroupEnv_AutomaticEnvironmentVariables tests that ExpandGroupEnv correctly handles automatic environment variables
func TestExpandGroupEnv_AutomaticEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name      string
		groupEnv  []string
		globalEnv map[string]string
		autoEnv   map[string]string
		expected  map[string]string
	}{
		{
			name:     "reference_automatic_variables_in_group_env",
			groupEnv: []string{"LOG_FILE=log-${__RUNNER_PID}.txt", "TIMESTAMP_VAR=${__RUNNER_DATETIME}"},
			autoEnv: map[string]string{
				"__RUNNER_DATETIME": "20241015103025.123",
				"__RUNNER_PID":      "12345",
			},
			expected: map[string]string{
				"LOG_FILE":      "log-12345.txt",
				"TIMESTAMP_VAR": "20241015103025.123",
			},
		},
		{
			name:     "priority_automatic_over_global",
			groupEnv: []string{"OUTPUT_FILE=${GLOBAL_PREFIX}-${__RUNNER_PID}.out"},
			globalEnv: map[string]string{
				"GLOBAL_PREFIX": "app",
			},
			autoEnv: map[string]string{
				"__RUNNER_PID": "67890",
			},
			expected: map[string]string{
				"OUTPUT_FILE": "app-67890.out",
			},
		},
		{
			name:     "complex_references_with_automatic_variables",
			groupEnv: []string{"WORK_DIR=/tmp/${__RUNNER_PID}", "CONFIG_FILE=${WORK_DIR}/config-${__RUNNER_DATETIME}.json"},
			autoEnv: map[string]string{
				"__RUNNER_DATETIME": "20241015103025.456",
				"__RUNNER_PID":      "54321",
			},
			expected: map[string]string{
				"WORK_DIR":    "/tmp/54321",
				"CONFIG_FILE": "/tmp/54321/config-20241015103025.456.json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &runnertypes.CommandGroup{
				Name:         "test_group",
				Env:          tt.groupEnv,
				EnvAllowlist: []string{}, // Empty allowlist for simplicity
			}

			filter := environment.NewFilter([]string{})
			expander := environment.NewVariableExpander(filter)

			err := ExpandGroupEnv(group, expander, tt.autoEnv, tt.globalEnv, []string{})
			require.NoError(t, err)

			require.Equal(t, tt.expected, group.ExpandedEnv)
		})
	}
}

// TestConfigLoader_AutomaticEnvironmentVariables_Integration tests the full integration of automatic environment variables
func TestConfigLoader_AutomaticEnvironmentVariables_Integration(t *testing.T) {
	tomlContent := `
[global]
env = ["TIMESTAMP=${__RUNNER_DATETIME}", "PROCESS_ID=${__RUNNER_PID}"]
env_allowlist = ["HOME"]

[[groups]]
name = "backup"
env = ["BACKUP_FILE=backup-${TIMESTAMP}.tar.gz", "LOG_FILE=log-${PROCESS_ID}.txt"]

[[groups.commands]]
name = "create_backup"
cmd = "echo"
args = ["Creating backup: ${BACKUP_FILE}, Log: ${LOG_FILE}"]
`

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(tomlContent))
	require.NoError(t, err)

	// Verify global environment variables contain automatic variables
	require.Contains(t, cfg.Global.ExpandedEnv, "TIMESTAMP")
	require.Contains(t, cfg.Global.ExpandedEnv, "PROCESS_ID")

	// Verify automatic variables have correct format
	timestamp := cfg.Global.ExpandedEnv["TIMESTAMP"]
	require.Len(t, timestamp, 18) // YYYYMMDDHHmmSS.msec format (e.g., "20251010161041.386")
	require.True(t, strings.HasPrefix(timestamp, "202"))

	processID := cfg.Global.ExpandedEnv["PROCESS_ID"]
	require.NotEmpty(t, processID)

	// Verify group environment variables reference automatic variables correctly
	require.Contains(t, cfg.Groups[0].ExpandedEnv, "BACKUP_FILE")
	require.Contains(t, cfg.Groups[0].ExpandedEnv, "LOG_FILE")

	backupFile := cfg.Groups[0].ExpandedEnv["BACKUP_FILE"]
	require.True(t, strings.HasPrefix(backupFile, "backup-"))
	require.True(t, strings.HasSuffix(backupFile, ".tar.gz"))
	require.Contains(t, backupFile, timestamp)

	logFile := cfg.Groups[0].ExpandedEnv["LOG_FILE"]
	require.True(t, strings.HasPrefix(logFile, "log-"))
	require.True(t, strings.HasSuffix(logFile, ".txt"))
	require.Contains(t, logFile, processID)
}
