// Package risk provides command risk evaluation functionality for the safe command runner.
// It analyzes commands and determines their security risk level based on various patterns and behaviors.
package risk

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Evaluator interface defines methods for evaluating command risk levels
type Evaluator interface {
	EvaluateRisk(cmd *runnertypes.RuntimeCommand) (runnertypes.RiskLevel, error)
}

// StandardEvaluator implements risk evaluation using predefined patterns
type StandardEvaluator struct {
	networkAnalyzer *security.NetworkAnalyzer
}

// NewStandardEvaluator creates a new standard risk evaluator.
func NewStandardEvaluator() Evaluator {
	return &StandardEvaluator{networkAnalyzer: security.NewNetworkAnalyzer()}
}

// EvaluateRisk analyzes a command and returns its risk level
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (runnertypes.RiskLevel, error) {
	// Check for privilege escalation commands (critical risk - should be blocked)
	isPrivEsc, err := security.IsPrivilegeEscalationCommand(cmd.ExpandedCmd)
	if err != nil {
		return runnertypes.RiskLevelUnknown, err
	}
	if isPrivEsc {
		return runnertypes.RiskLevelCritical, nil
	}

	// Check for destructive file operations
	if security.IsDestructiveFileOperation(cmd.ExpandedCmd, cmd.ExpandedArgs) {
		return runnertypes.RiskLevelHigh, nil
	}

	// Check for network operations
	isNetwork, isHighRisk := e.networkAnalyzer.IsNetworkOperation(cmd.ExpandedCmd, cmd.ExpandedArgs)
	if isHighRisk {
		return runnertypes.RiskLevelHigh, nil
	}
	if isNetwork {
		return runnertypes.RiskLevelMedium, nil
	}

	// Check for system modification commands
	if security.IsSystemModification(cmd.ExpandedCmd, cmd.ExpandedArgs) {
		return runnertypes.RiskLevelMedium, nil
	}

	// Default to low risk for safe commands
	return runnertypes.RiskLevelLow, nil
}
