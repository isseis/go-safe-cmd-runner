// Package risk provides command risk evaluation functionality for the safe command runner.
// It analyzes commands and determines their security risk level based on various patterns and behaviors.
package risk

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Evaluator interface defines methods for evaluating command risk levels
type Evaluator interface {
	EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error)
}

// StandardEvaluator implements risk evaluation using predefined patterns
type StandardEvaluator struct{}

// NewStandardEvaluator creates a new standard risk evaluator
func NewStandardEvaluator() Evaluator {
	return &StandardEvaluator{}
}

// EvaluateRisk analyzes a command and returns its risk level
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error) {
	// Use ExpandedCmd if available, fallback to original Cmd
	cmdToEvaluate := cmd.ExpandedCmd
	if cmdToEvaluate == "" {
		cmdToEvaluate = cmd.Cmd
	}

	// Use ExpandedArgs if available, fallback to original Args
	argsToEvaluate := cmd.ExpandedArgs
	if len(argsToEvaluate) == 0 {
		argsToEvaluate = cmd.Args
	}

	// Check for privilege escalation commands (critical risk - should be blocked)
	isPrivEsc, err := security.IsPrivilegeEscalationCommand(cmdToEvaluate)
	if err != nil {
		return runnertypes.RiskLevelUnknown, err
	}
	if isPrivEsc {
		return runnertypes.RiskLevelCritical, nil
	}

	// Check for destructive file operations
	if security.IsDestructiveFileOperation(cmdToEvaluate, argsToEvaluate) {
		return runnertypes.RiskLevelHigh, nil
	}

	// Check for network operations
	isNetwork, isHighRisk := security.IsNetworkOperation(cmdToEvaluate, argsToEvaluate)
	if isHighRisk {
		return runnertypes.RiskLevelHigh, nil
	}
	if isNetwork {
		return runnertypes.RiskLevelMedium, nil
	}

	// Check for system modification commands
	if security.IsSystemModification(cmdToEvaluate, argsToEvaluate) {
		return runnertypes.RiskLevelMedium, nil
	}

	// Default to low risk for safe commands
	return runnertypes.RiskLevelLow, nil
}
