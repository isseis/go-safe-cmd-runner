//go:build test

package risk

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStandardEvaluator(t *testing.T) {
	evaluator := NewStandardEvaluator()
	require.NotNil(t, evaluator)
	assert.IsType(t, &StandardEvaluator{}, evaluator)
}

func TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{
			name:     "sudo command",
			cmd:      "sudo",
			args:     []string{"ls", "/root"},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name:     "su command",
			cmd:      "su",
			args:     []string{"root"},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name:     "doas command",
			cmd:      "doas",
			args:     []string{"ls", "/root"},
			expected: runnertypes.RiskLevelCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCmd := &runnertypes.RuntimeCommand{
				ExpandedCmd:  tt.cmd,
				ExpandedArgs: tt.args,
			}
			result, err := evaluator.EvaluateRisk(runtimeCmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_DestructiveFileOperations(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{
			name:     "rm with recursive flag",
			cmd:      "rm",
			args:     []string{"-rf", "/tmp/files"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "find with delete",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.tmp", "-delete"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "dd command",
			cmd:      "dd",
			args:     []string{"if=/dev/zero", "of=/tmp/test"},
			expected: runnertypes.RiskLevelHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCmd := &runnertypes.RuntimeCommand{
				ExpandedCmd:  tt.cmd,
				ExpandedArgs: tt.args,
			}
			result, err := evaluator.EvaluateRisk(runtimeCmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_NetworkOperations(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{
			name:     "wget download",
			cmd:      "wget",
			args:     []string{"https://example.com/file.txt"},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name:     "curl download",
			cmd:      "curl",
			args:     []string{"-O", "https://example.com/file.txt"},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name:     "nc command",
			cmd:      "nc",
			args:     []string{"-l", "8080"},
			expected: runnertypes.RiskLevelMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCmd := &runnertypes.RuntimeCommand{
				ExpandedCmd:  tt.cmd,
				ExpandedArgs: tt.args,
			}
			result, err := evaluator.EvaluateRisk(runtimeCmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_SystemModifications(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{
			name:     "systemctl restart",
			cmd:      "systemctl",
			args:     []string{"restart", "nginx"},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name:     "apt install",
			cmd:      "apt",
			args:     []string{"install", "vim"},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name:     "yum install",
			cmd:      "yum",
			args:     []string{"install", "vim"},
			expected: runnertypes.RiskLevelMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCmd := &runnertypes.RuntimeCommand{
				ExpandedCmd:  tt.cmd,
				ExpandedArgs: tt.args,
			}
			result, err := evaluator.EvaluateRisk(runtimeCmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_SafeCommands(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{
			name:     "echo command",
			cmd:      "echo",
			args:     []string{"Hello, World!"},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name:     "ls command",
			cmd:      "ls",
			args:     []string{"-la", "/home"},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name:     "cat command",
			cmd:      "cat",
			args:     []string{"/etc/passwd"},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name:     "apt list (query)",
			cmd:      "apt",
			args:     []string{"list", "--installed"},
			expected: runnertypes.RiskLevelLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCmd := &runnertypes.RuntimeCommand{
				ExpandedCmd:  tt.cmd,
				ExpandedArgs: tt.args,
			}
			result, err := evaluator.EvaluateRisk(runtimeCmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_EmptyCommand(t *testing.T) {
	evaluator := NewStandardEvaluator()

	runtimeCmd := &runnertypes.RuntimeCommand{
		ExpandedCmd:  "",
		ExpandedArgs: []string{"test"},
	}
	result, err := evaluator.EvaluateRisk(runtimeCmd)
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelLow, result)
}

func TestStandardEvaluator_EvaluateRisk_RiskLevelHierarchy(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name        string
		cmd         string
		args        []string
		expected    runnertypes.RiskLevel
		description string
	}{
		{
			name:        "critical risk overrides all",
			cmd:         "sudo",
			args:        []string{"rm", "-rf", "/"},
			expected:    runnertypes.RiskLevelCritical,
			description: "Privilege escalation should be classified as critical even with destructive operations",
		},
		{
			name:        "high risk destructive operations",
			cmd:         "rm",
			args:        []string{"-rf", "/important/data"},
			expected:    runnertypes.RiskLevelHigh,
			description: "Destructive file operations should be high risk",
		},
		{
			name:        "medium risk network operations",
			cmd:         "wget",
			args:        []string{"https://suspicious.example.com/script.sh"},
			expected:    runnertypes.RiskLevelMedium,
			description: "Network operations should be medium risk",
		},
		{
			name:        "medium risk system modifications",
			cmd:         "systemctl",
			args:        []string{"stop", "important-service"},
			expected:    runnertypes.RiskLevelMedium,
			description: "System modifications should be medium risk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCmd := &runnertypes.RuntimeCommand{
				ExpandedCmd:  tt.cmd,
				ExpandedArgs: tt.args,
			}
			result, err := evaluator.EvaluateRisk(runtimeCmd)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result, "Test: %s\nDescription: %s", tt.name, tt.description)
		})
	}
}
