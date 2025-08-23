package security

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateCommandExecution_AllowedRisk(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	evaluator := NewDefaultRiskEvaluator(logger)

	tests := []struct {
		name            string
		riskLevel       runnertypes.RiskLevel
		detectedPattern string
		reason          string
		privilegeResult *PrivilegeEscalationResult
		command         *runnertypes.Command
		expectError     bool
	}{
		{
			name:            "low risk command",
			riskLevel:       runnertypes.RiskLevelLow,
			detectedPattern: "basic_command",
			reason:          "Standard system command",
			privilegeResult: nil,
			command: &runnertypes.Command{
				Name:       "ls",
				Privileged: false,
			},
			expectError: false,
		},
		{
			name:            "medium risk command",
			riskLevel:       runnertypes.RiskLevelMedium,
			detectedPattern: "system_command",
			reason:          "System management command",
			privilegeResult: nil,
			command: &runnertypes.Command{
				Name:       "systemctl",
				Privileged: false,
			},
			expectError: false,
		},
		{
			name:            "no risk command",
			riskLevel:       runnertypes.RiskLevelNone,
			detectedPattern: "",
			reason:          "",
			privilegeResult: nil,
			command: &runnertypes.Command{
				Name:       "echo",
				Privileged: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evaluator.EvaluateCommandExecution(
				context.Background(),
				tt.riskLevel,
				tt.detectedPattern,
				tt.reason,
				tt.privilegeResult,
				tt.command,
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvaluateCommandExecution_ExceededRisk(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	evaluator := NewDefaultRiskEvaluator(logger)

	tests := []struct {
		name            string
		riskLevel       runnertypes.RiskLevel
		detectedPattern string
		reason          string
		privilegeResult *PrivilegeEscalationResult
		command         *runnertypes.Command
		expectError     bool
	}{
		{
			name:            "high risk command without privilege",
			riskLevel:       runnertypes.RiskLevelHigh,
			detectedPattern: "dangerous_command",
			reason:          "Potentially dangerous operation",
			privilegeResult: nil,
			command: &runnertypes.Command{
				Name:       "rm",
				Privileged: false,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evaluator.EvaluateCommandExecution(
				context.Background(),
				tt.riskLevel,
				tt.detectedPattern,
				tt.reason,
				tt.privilegeResult,
				tt.command,
			)

			if tt.expectError {
				assert.Error(t, err)
				// Check if it's a SecurityViolationError
				assert.True(t, runnertypes.IsSecurityViolationError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvaluateCommandExecution_PrivilegedBypass(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	evaluator := NewDefaultRiskEvaluator(logger)

	tests := []struct {
		name            string
		riskLevel       runnertypes.RiskLevel
		detectedPattern string
		reason          string
		privilegeResult *PrivilegeEscalationResult
		command         *runnertypes.Command
		expectError     bool
	}{
		{
			name:            "privileged command with privilege escalation",
			riskLevel:       runnertypes.RiskLevelHigh,
			detectedPattern: "sudo_command",
			reason:          "Privilege escalation detected",
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSudo,
				RiskLevel:             runnertypes.RiskLevelHigh,
				RequiredPrivileges:    []string{"root"},
				CommandPath:           "/usr/bin/sudo",
				DetectedPattern:       "sudo_command",
				Reason:                "Command uses sudo for privilege escalation",
			},
			command: &runnertypes.Command{
				Name:       "sudo",
				Privileged: true,
			},
			expectError: false,
		},
		{
			name:            "high risk privileged command",
			riskLevel:       runnertypes.RiskLevelHigh,
			detectedPattern: "dangerous_command",
			reason:          "Potentially dangerous operation",
			privilegeResult: nil,
			command: &runnertypes.Command{
				Name:       "dd",
				Privileged: true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evaluator.EvaluateCommandExecution(
				context.Background(),
				tt.riskLevel,
				tt.detectedPattern,
				tt.reason,
				tt.privilegeResult,
				tt.command,
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvaluateCommandExecution_PrivilegeEscalationHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	evaluator := NewDefaultRiskEvaluator(logger)

	tests := []struct {
		name            string
		riskLevel       runnertypes.RiskLevel
		detectedPattern string
		reason          string
		privilegeResult *PrivilegeEscalationResult
		command         *runnertypes.Command
		expectError     bool
		errorMessage    string
	}{
		{
			name:            "privilege escalation without privileged flag",
			riskLevel:       runnertypes.RiskLevelMedium,
			detectedPattern: "sudo_command",
			reason:          "Privilege escalation detected",
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSudo,
				RiskLevel:             runnertypes.RiskLevelHigh,
				RequiredPrivileges:    []string{"root"},
				CommandPath:           "/usr/bin/sudo",
				DetectedPattern:       "sudo_command",
				Reason:                "Command uses sudo for privilege escalation",
			},
			command: &runnertypes.Command{
				Name:       "sudo",
				Privileged: false,
			},
			expectError:  true,
			errorMessage: "privilege escalation (sudo)",
		},
		{
			name:            "systemctl without privileged flag",
			riskLevel:       runnertypes.RiskLevelMedium,
			detectedPattern: "systemctl_command",
			reason:          "System service management",
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSystemd,
				RiskLevel:             runnertypes.RiskLevelMedium,
				RequiredPrivileges:    []string{"systemd"},
				CommandPath:           "/bin/systemctl",
				DetectedPattern:       "systemctl_command",
				Reason:                "Command manages system services",
			},
			command: &runnertypes.Command{
				Name:       "systemctl",
				Privileged: false,
			},
			expectError:  true,
			errorMessage: "privilege escalation (systemd)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evaluator.EvaluateCommandExecution(
				context.Background(),
				tt.riskLevel,
				tt.detectedPattern,
				tt.reason,
				tt.privilegeResult,
				tt.command,
			)

			if tt.expectError {
				require.Error(t, err)

				// Check SecurityViolationError details
				secErr, isSecErr := runnertypes.GetSecurityViolationError(err)
				require.True(t, isSecErr)
				assert.Equal(t, tt.command.Name, secErr.Command)

				// For privilege escalation cases, check that we have privilege info
				if tt.privilegeResult != nil && tt.privilegeResult.IsPrivilegeEscalation {
					assert.NotNil(t, secErr.PrivilegeInfo)
					assert.Equal(t, string(tt.privilegeResult.EscalationType), secErr.PrivilegeInfo.EscalationType)
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRiskEvaluator_HelperMethods(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	evaluator := NewDefaultRiskEvaluator(logger)

	t.Run("parseMaxRiskLevel", func(t *testing.T) {
		tests := []struct {
			name     string
			command  *runnertypes.Command
			expected runnertypes.RiskLevel
		}{
			{
				name: "privileged command",
				command: &runnertypes.Command{
					Name:       "sudo",
					Privileged: true,
				},
				expected: runnertypes.RiskLevelHigh,
			},
			{
				name: "non-privileged command",
				command: &runnertypes.Command{
					Name:       "ls",
					Privileged: false,
				},
				expected: runnertypes.RiskLevelMedium,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := evaluator.parseMaxRiskLevel(tt.command)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("exceedsRiskLevel", func(t *testing.T) {
		tests := []struct {
			name          string
			detectedLevel runnertypes.RiskLevel
			maxLevel      runnertypes.RiskLevel
			expected      bool
		}{
			{"none vs medium", runnertypes.RiskLevelNone, runnertypes.RiskLevelMedium, false},
			{"low vs medium", runnertypes.RiskLevelLow, runnertypes.RiskLevelMedium, false},
			{"medium vs medium", runnertypes.RiskLevelMedium, runnertypes.RiskLevelMedium, false},
			{"high vs medium", runnertypes.RiskLevelHigh, runnertypes.RiskLevelMedium, true},
			{"high vs high", runnertypes.RiskLevelHigh, runnertypes.RiskLevelHigh, false},
			{"medium vs low", runnertypes.RiskLevelMedium, runnertypes.RiskLevelLow, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := evaluator.exceedsRiskLevel(tt.detectedLevel, tt.maxLevel)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("generateRequiredSetting", func(t *testing.T) {
		tests := []struct {
			name            string
			riskLevel       runnertypes.RiskLevel
			maxRiskLevel    runnertypes.RiskLevel
			privilegeResult *PrivilegeEscalationResult
			expected        string
		}{
			{
				name:         "high risk exceeds medium",
				riskLevel:    runnertypes.RiskLevelHigh,
				maxRiskLevel: runnertypes.RiskLevelMedium,
				expected:     "max_risk_level = \"high\"",
			},
			{
				name:         "privilege escalation",
				riskLevel:    runnertypes.RiskLevelMedium,
				maxRiskLevel: runnertypes.RiskLevelMedium,
				privilegeResult: &PrivilegeEscalationResult{
					IsPrivilegeEscalation: true,
				},
				expected: "privileged = true",
			},
			{
				name:         "both issues",
				riskLevel:    runnertypes.RiskLevelHigh,
				maxRiskLevel: runnertypes.RiskLevelMedium,
				privilegeResult: &PrivilegeEscalationResult{
					IsPrivilegeEscalation: true,
				},
				expected: "max_risk_level = \"high\" or privileged = true",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := evaluator.generateRequiredSetting(tt.riskLevel, tt.maxRiskLevel, tt.privilegeResult)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}
