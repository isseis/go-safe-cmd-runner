//go:build skip_risk_tests

package risk_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestStandardEvaluator_EvaluateRisk(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name     string
		cmd      *runnertypes.Command
		expected runnertypes.RiskLevel
	}{
		{
			name: "privilege escalation command - sudo",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "sudo",
				Args: []string{"ls", "/root"},
			},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name: "privilege escalation command - su",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "su",
				Args: []string{"root"},
			},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name: "privilege escalation command - doas",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "doas",
				Args: []string{"ls", "/root"},
			},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name: "destructive file operation - rm",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "rm",
				Args: []string{"-rf", "/tmp/files"},
			},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name: "destructive file operation - find with delete",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "find",
				Args: []string{"/tmp", "-name", "*.tmp", "-delete"},
			},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name: "network operation - wget",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "wget",
				Args: []string{"https://example.com/file.txt"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "network operation - curl",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "curl",
				Args: []string{"-O", "https://example.com/file.txt"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "system modification - systemctl",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "systemctl",
				Args: []string{"restart", "nginx"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "package installation - apt install",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "apt",
				Args: []string{"install", "vim"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "safe package query - apt list",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "apt",
				Args: []string{"list", "--installed"},
			},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name: "safe command - echo",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "echo",
				Args: []string{"Hello, World!"},
			},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name: "safe command - ls",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "ls",
				Args: []string{"-la", "/home"},
			},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name: "safe command - cat",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "cat",
				Args: []string{"/etc/passwd"},
			},
			expected: runnertypes.RiskLevelLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnertypes.PrepareCommand(tt.cmd)
			result, err := evaluator.EvaluateRisk(tt.cmd)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestStandardEvaluator_RiskLevelHierarchy tests that risk levels are properly prioritized
func TestStandardEvaluator_RiskLevelHierarchy(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name        string
		cmd         *runnertypes.Command
		expected    runnertypes.RiskLevel
		description string
	}{
		{
			name: "critical risk overrides all",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "sudo",
				Args: []string{"rm", "-rf", "/"},
			},
			expected:    runnertypes.RiskLevelCritical,
			description: "Privilege escalation should be classified as critical even with destructive operations",
		},
		{
			name: "high risk destructive operations",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "rm",
				Args: []string{"-rf", "/important/data"},
			},
			expected:    runnertypes.RiskLevelHigh,
			description: "Destructive file operations should be high risk",
		},
		{
			name: "medium risk network operations",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "wget",
				Args: []string{"https://suspicious.example.com/script.sh"},
			},
			expected:    runnertypes.RiskLevelMedium,
			description: "Network operations should be medium risk",
		},
		{
			name: "medium risk system modifications",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "systemctl",
				Args: []string{"stop", "important-service"},
			},
			expected:    runnertypes.RiskLevelMedium,
			description: "System modifications should be medium risk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnertypes.PrepareCommand(tt.cmd)
			result, err := evaluator.EvaluateRisk(tt.cmd)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Test: %s\nExpected: %v, Got: %v\nDescription: %s",
					tt.name, tt.expected, result, tt.description)
			}
		})
	}
}

// TestStandardEvaluator_ErrorHandling tests error handling in risk evaluation
func TestStandardEvaluator_ErrorHandling(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name         string
		cmd          *runnertypes.Command
		expectError  bool
		expectedRisk runnertypes.RiskLevel
	}{
		{
			name: "normal command should not error",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "echo",
				Args: []string{"hello"},
			},
			expectError:  false,
			expectedRisk: runnertypes.RiskLevelLow,
		},
		{
			name: "empty command name",
			cmd: &runnertypes.CommandSpec{
				Cmd:  "",
				Args: []string{"test"},
			},
			expectError:  false, // IsPrivilegeEscalationCommand handles empty command gracefully
			expectedRisk: runnertypes.RiskLevelLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnertypes.PrepareCommand(tt.cmd)
			result, err := evaluator.EvaluateRisk(tt.cmd)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectError && result != tt.expectedRisk {
				t.Errorf("expected risk %v, got %v", tt.expectedRisk, result)
			}
		})
	}
}
