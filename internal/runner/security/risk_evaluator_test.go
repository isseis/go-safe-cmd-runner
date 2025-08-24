package security

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestNewDefaultRiskEvaluator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)

	assert.NotNil(t, evaluator)
	assert.NotNil(t, evaluator.logger)
}

func TestEvaluateCommandExecution_AllowedRisk(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)
	ctx := context.Background()

	testCases := []struct {
		name            string
		riskLevel       RiskLevel
		detectedPattern string
		reason          string
		privilegeResult *PrivilegeEscalationResult
		command         *runnertypes.Command
		expectError     bool
	}{
		{
			name:            "no risk command",
			riskLevel:       RiskLevelNone,
			detectedPattern: "",
			reason:          "",
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: false,
				RiskLevel:             RiskLevelNone,
				CommandPath:           "/bin/ls",
			},
			command: &runnertypes.Command{
				Name: "list-files",
				Cmd:  "ls",
			},
			expectError: false,
		},
		{
			name:            "low risk command",
			riskLevel:       RiskLevelLow,
			detectedPattern: "ls",
			reason:          "directory listing",
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: false,
				RiskLevel:             RiskLevelNone,
				CommandPath:           "/bin/ls",
			},
			command: &runnertypes.Command{
				Name: "list-files",
				Cmd:  "ls",
			},
			expectError: false,
		},
		{
			name:            "medium risk command",
			riskLevel:       RiskLevelMedium,
			detectedPattern: "systemctl",
			reason:          "system service control",
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSystemd,
				RiskLevel:             RiskLevelMedium,
				RequiredPrivileges:    []string{"systemd"},
				CommandPath:           "/bin/systemctl",
				DetectedPattern:       "systemctl",
			},
			command: &runnertypes.Command{
				Name: "restart-service",
				Cmd:  "systemctl",
			},
			expectError: false, // Medium risk is allowed (max is High)
		},
		{
			name:            "high risk command",
			riskLevel:       RiskLevelHigh,
			detectedPattern: "sudo",
			reason:          "privilege escalation",
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSudo,
				RiskLevel:             RiskLevelHigh,
				RequiredPrivileges:    []string{"root"},
				CommandPath:           "/usr/bin/sudo",
				DetectedPattern:       "sudo",
			},
			command: &runnertypes.Command{
				Name: "sudo-command",
				Cmd:  "sudo",
			},
			expectError: false, // High risk is allowed when max is High
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := evaluator.EvaluateCommandExecution(
				ctx, tc.riskLevel, tc.detectedPattern, tc.reason,
				tc.privilegeResult, tc.command,
			)

			if tc.expectError {
				assert.Error(t, err)
				assert.True(t, runnertypes.IsSecurityViolationError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvaluateCommandExecution_ExceededRisk(t *testing.T) {
	// Note: In Phase 1, we use RiskLevelHigh as default max, so no risk should exceed
	// This test case will be more relevant when max_risk_level configuration is added
	// For now, we'll test with a mock scenario where we temporarily modify the logic

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)
	ctx := context.Background()

	// Create a scenario where risk would exceed if max was lower
	// This is preparation for future implementation
	privilegeResult := &PrivilegeEscalationResult{
		IsPrivilegeEscalation: true,
		EscalationType:        PrivilegeEscalationTypeSudo,
		RiskLevel:             RiskLevelHigh,
		RequiredPrivileges:    []string{"root"},
		CommandPath:           "/usr/bin/sudo",
		DetectedPattern:       "sudo",
		Reason:                "Command requires root privileges",
	}

	command := &runnertypes.Command{
		Name: "dangerous-sudo",
		Cmd:  "sudo",
	}

	err := evaluator.EvaluateCommandExecution(
		ctx, RiskLevelHigh, "sudo", "privilege escalation",
		privilegeResult, command,
	)

	// Currently should not error since max is High
	assert.NoError(t, err)
}

func TestEvaluateCommandExecution_PrivilegedBypass(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)
	ctx := context.Background()

	privilegeResult := &PrivilegeEscalationResult{
		IsPrivilegeEscalation: true,
		EscalationType:        PrivilegeEscalationTypeSudo,
		RiskLevel:             RiskLevelHigh,
		RequiredPrivileges:    []string{"root"},
		CommandPath:           "/usr/bin/sudo",
		DetectedPattern:       "sudo",
		Reason:                "Command requires root privileges",
	}

	// Command with privilege escalation - should pass with current risk level limit
	command := &runnertypes.Command{
		Name: "privilege-sudo",
		Cmd:  "sudo",
	}

	err := evaluator.EvaluateCommandExecution(
		ctx, RiskLevelHigh, "sudo", "privilege escalation",
		privilegeResult, command,
	)

	// Should pass because RiskLevelHigh is within the default max allowed level
	assert.NoError(t, err)
}

func TestEvaluateCommandExecution_PrivilegeEscalationHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)
	ctx := context.Background()

	testCases := []struct {
		name              string
		basicRisk         RiskLevel
		privilegeResult   *PrivilegeEscalationResult
		expectedEffective RiskLevel
	}{
		{
			name:      "no privilege escalation",
			basicRisk: RiskLevelMedium,
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: false,
				RiskLevel:             RiskLevelNone,
				CommandPath:           "/bin/ls",
			},
			expectedEffective: RiskLevelMedium,
		},
		{
			name:      "privilege escalation higher than basic",
			basicRisk: RiskLevelLow,
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSudo,
				RiskLevel:             RiskLevelHigh,
				RequiredPrivileges:    []string{"root"},
				CommandPath:           "/usr/bin/sudo",
				DetectedPattern:       "sudo",
			},
			expectedEffective: RiskLevelHigh,
		},
		{
			name:      "basic risk higher than privilege escalation",
			basicRisk: RiskLevelHigh,
			privilegeResult: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSystemd,
				RiskLevel:             RiskLevelMedium,
				RequiredPrivileges:    []string{"systemd"},
				CommandPath:           "/bin/systemctl",
				DetectedPattern:       "systemctl",
			},
			expectedEffective: RiskLevelHigh,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			command := &runnertypes.Command{
				Name: "test-command",
				Cmd:  "test",
			}

			effective := evaluator.determineEffectiveRiskLevel(tc.basicRisk, tc.privilegeResult)
			assert.Equal(t, tc.expectedEffective, effective)

			// Test full evaluation
			err := evaluator.EvaluateCommandExecution(
				ctx, tc.basicRisk, "test", "test reason",
				tc.privilegeResult, command,
			)

			// Should not error since all test cases have effective risk <= High
			assert.NoError(t, err)
		})
	}
}

func TestDetermineEffectiveRiskLevel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)

	testCases := []struct {
		name          string
		basicRisk     RiskLevel
		privilegeRisk RiskLevel
		isEscalation  bool
		expectedRisk  RiskLevel
	}{
		{
			name:          "no escalation",
			basicRisk:     RiskLevelMedium,
			privilegeRisk: RiskLevelNone,
			isEscalation:  false,
			expectedRisk:  RiskLevelMedium,
		},
		{
			name:          "escalation higher",
			basicRisk:     RiskLevelLow,
			privilegeRisk: RiskLevelHigh,
			isEscalation:  true,
			expectedRisk:  RiskLevelHigh,
		},
		{
			name:          "basic higher",
			basicRisk:     RiskLevelHigh,
			privilegeRisk: RiskLevelMedium,
			isEscalation:  true,
			expectedRisk:  RiskLevelHigh,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			privilegeResult := &PrivilegeEscalationResult{
				IsPrivilegeEscalation: tc.isEscalation,
				RiskLevel:             tc.privilegeRisk,
			}

			result := evaluator.determineEffectiveRiskLevel(tc.basicRisk, privilegeResult)
			assert.Equal(t, tc.expectedRisk, result)
		})
	}
}

func TestMaxRiskLevel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)

	testCases := []struct {
		name     string
		riskA    RiskLevel
		riskB    RiskLevel
		expected RiskLevel
	}{
		{"none vs low", RiskLevelNone, RiskLevelLow, RiskLevelLow},
		{"low vs none", RiskLevelLow, RiskLevelNone, RiskLevelLow},
		{"medium vs low", RiskLevelMedium, RiskLevelLow, RiskLevelMedium},
		{"high vs medium", RiskLevelHigh, RiskLevelMedium, RiskLevelHigh},
		{"equal levels", RiskLevelMedium, RiskLevelMedium, RiskLevelMedium},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluator.maxRiskLevel(tc.riskA, tc.riskB)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsRiskLevelExceeded(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	evaluator := NewDefaultRiskEvaluator(logger)

	testCases := []struct {
		name          string
		effectiveRisk RiskLevel
		maxRisk       RiskLevel
		expected      bool
	}{
		{"none within low", RiskLevelNone, RiskLevelLow, false},
		{"low within low", RiskLevelLow, RiskLevelLow, false},
		{"medium exceeds low", RiskLevelMedium, RiskLevelLow, true},
		{"high exceeds medium", RiskLevelHigh, RiskLevelMedium, true},
		{"medium within high", RiskLevelMedium, RiskLevelHigh, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluator.isRiskLevelExceeded(tc.effectiveRisk, tc.maxRisk)
			assert.Equal(t, tc.expected, result)
		})
	}
}
