package security

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

const (
	// Risk level ordering for comparison
	riskOrderNone   = 0
	riskOrderLow    = 1
	riskOrderMedium = 2
	riskOrderHigh   = 3
)

// PrivilegeEscalationInfo contains information about privilege escalation for error reporting
type PrivilegeEscalationInfo struct {
	IsPrivilegeEscalation bool
	EscalationType        PrivilegeEscalationType
	RequiredPrivileges    []string
	DetectedPattern       string
}

// RiskEvaluator interface defines methods for comprehensive risk evaluation
type RiskEvaluator interface {
	EvaluateCommandExecution(
		ctx context.Context,
		riskLevel RiskLevel,
		detectedPattern string,
		reason string,
		privilegeResult *PrivilegeEscalationResult,
		command *runnertypes.Command,
	) error
}

// DefaultRiskEvaluator is the default implementation of RiskEvaluator
type DefaultRiskEvaluator struct {
	logger *slog.Logger
}

// NewDefaultRiskEvaluator creates a new DefaultRiskEvaluator
func NewDefaultRiskEvaluator(logger *slog.Logger) *DefaultRiskEvaluator {
	return &DefaultRiskEvaluator{
		logger: logger,
	}
}

// EvaluateCommandExecution evaluates the risk of command execution considering both
// basic security analysis and privilege escalation analysis
func (re *DefaultRiskEvaluator) EvaluateCommandExecution(
	_ context.Context,
	riskLevel RiskLevel,
	detectedPattern string,
	reason string,
	privilegeResult *PrivilegeEscalationResult,
	command *runnertypes.Command,
) error {
	// No privileged command bypass - all commands are subject to risk evaluation

	// Determine the effective risk level
	effectiveRiskLevel := re.determineEffectiveRiskLevel(riskLevel, privilegeResult)

	// Get maximum allowed risk level for the command
	// In Phase 1, we don't have max_risk_level field yet, so use high as default
	maxAllowedRiskLevel := RiskLevelHigh

	// Compare risk levels
	if re.isRiskLevelExceeded(effectiveRiskLevel, maxAllowedRiskLevel) {
		// Create privilege escalation info for error reporting
		var privilegeInfo *PrivilegeEscalationInfo
		if privilegeResult.IsPrivilegeEscalation {
			privilegeInfo = &PrivilegeEscalationInfo{
				IsPrivilegeEscalation: privilegeResult.IsPrivilegeEscalation,
				EscalationType:        privilegeResult.EscalationType,
				RequiredPrivileges:    privilegeResult.RequiredPrivileges,
				DetectedPattern:       privilegeResult.DetectedPattern,
			}
		}

		// Log security violation
		re.logSecurityViolation(command, effectiveRiskLevel, maxAllowedRiskLevel, detectedPattern, reason, privilegeInfo)

		// Create PrivilegeEscalationInfo for runnertypes if needed
		var privilegeInfoForError *runnertypes.PrivilegeEscalationInfo
		if privilegeInfo != nil {
			privilegeInfoForError = &runnertypes.PrivilegeEscalationInfo{
				IsPrivilegeEscalation: privilegeInfo.IsPrivilegeEscalation,
				EscalationType:        string(privilegeInfo.EscalationType),
				RequiredPrivileges:    privilegeInfo.RequiredPrivileges,
				DetectedPattern:       privilegeInfo.DetectedPattern,
			}
		}

		// Return security violation error
		return runnertypes.NewSecurityViolationError(
			command.Cmd,
			effectiveRiskLevel.String(),
			detectedPattern,
			fmt.Sprintf("max_risk_level=%s", maxAllowedRiskLevel.String()),
			privilegeResult.CommandPath,
			"", // RunID will be set by caller if needed
			privilegeInfoForError,
		)
	}

	// Log successful risk evaluation
	re.logger.Debug("command risk evaluation passed",
		"command", command.Name,
		"cmd", command.Cmd,
		"effective_risk_level", effectiveRiskLevel,
		"max_allowed_risk_level", maxAllowedRiskLevel,
		"basic_risk_level", riskLevel,
		"privilege_escalation", privilegeResult.IsPrivilegeEscalation,
		"escalation_type", privilegeResult.EscalationType,
	)

	return nil
}

// determineEffectiveRiskLevel determines the effective risk level considering both
// basic security risk and privilege escalation risk
func (re *DefaultRiskEvaluator) determineEffectiveRiskLevel(
	basicRiskLevel RiskLevel,
	privilegeResult *PrivilegeEscalationResult,
) RiskLevel {
	// If no privilege escalation, use basic risk level
	if !privilegeResult.IsPrivilegeEscalation {
		return basicRiskLevel
	}

	// If privilege escalation is detected, take the higher risk level
	return re.maxRiskLevel(basicRiskLevel, privilegeResult.RiskLevel)
}

// maxRiskLevel returns the higher of two risk levels
func (re *DefaultRiskEvaluator) maxRiskLevel(a, b RiskLevel) RiskLevel {
	riskOrder := map[RiskLevel]int{
		RiskLevelNone:   riskOrderNone,
		RiskLevelLow:    riskOrderLow,
		RiskLevelMedium: riskOrderMedium,
		RiskLevelHigh:   riskOrderHigh,
	}

	if riskOrder[a] >= riskOrder[b] {
		return a
	}
	return b
}

// isRiskLevelExceeded checks if the effective risk level exceeds the maximum allowed
func (re *DefaultRiskEvaluator) isRiskLevelExceeded(effectiveRiskLevel, maxAllowedRiskLevel RiskLevel) bool {
	riskOrder := map[RiskLevel]int{
		RiskLevelNone:   riskOrderNone,
		RiskLevelLow:    riskOrderLow,
		RiskLevelMedium: riskOrderMedium,
		RiskLevelHigh:   riskOrderHigh,
	}

	return riskOrder[effectiveRiskLevel] > riskOrder[maxAllowedRiskLevel]
}

// logSecurityViolation logs detailed information about security violations
func (re *DefaultRiskEvaluator) logSecurityViolation(
	command *runnertypes.Command,
	effectiveRiskLevel RiskLevel,
	maxAllowedRiskLevel RiskLevel,
	detectedPattern string,
	reason string,
	privilegeInfo *PrivilegeEscalationInfo,
) {
	logFields := []any{
		"event", "security_risk_violation",
		"command", command.Name,
		"cmd", command.Cmd,
		"effective_risk_level", effectiveRiskLevel,
		"max_allowed_risk_level", maxAllowedRiskLevel,
		"detected_pattern", detectedPattern,
		"reason", reason,
	}

	if privilegeInfo != nil && privilegeInfo.IsPrivilegeEscalation {
		logFields = append(logFields,
			"privilege_escalation", true,
			"escalation_type", privilegeInfo.EscalationType,
			"required_privileges", privilegeInfo.RequiredPrivileges,
			"privilege_pattern", privilegeInfo.DetectedPattern,
		)
	}

	re.logger.Error("security risk violation detected", logFields...)
}
