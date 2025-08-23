package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

var errSecurityViolation = errors.New("security violation")

// RiskEvaluator interface defines methods for evaluating command execution risks
type RiskEvaluator interface {
	// EvaluateCommandExecution evaluates whether a command should be allowed to execute
	// based on its risk level and privilege escalation analysis
	EvaluateCommandExecution(
		ctx context.Context,
		riskLevel runnertypes.RiskLevel,
		detectedPattern string,
		reason string,
		privilegeResult *PrivilegeEscalationResult,
		command *runnertypes.Command,
	) error
}

// DefaultRiskEvaluator implements RiskEvaluator interface
type DefaultRiskEvaluator struct {
	logger *slog.Logger
}

// NewDefaultRiskEvaluator creates a new instance of DefaultRiskEvaluator
func NewDefaultRiskEvaluator(logger *slog.Logger) *DefaultRiskEvaluator {
	return &DefaultRiskEvaluator{
		logger: logger,
	}
}

// EvaluateCommandExecution evaluates command execution based on risk levels and configuration
func (re *DefaultRiskEvaluator) EvaluateCommandExecution(
	_ context.Context,
	riskLevel runnertypes.RiskLevel,
	detectedPattern string,
	reason string,
	privilegeResult *PrivilegeEscalationResult,
	command *runnertypes.Command,
) error {
	// Get the command's maximum allowed risk level
	maxRiskLevel := re.parseMaxRiskLevel(command)

	// Evaluate privilege escalation risk first
	if privilegeResult != nil && privilegeResult.IsPrivilegeEscalation && !command.Privileged {
		re.logSecurityViolation(command.Name, privilegeResult.RiskLevel, privilegeResult.DetectedPattern, privilegeResult.Reason, privilegeResult)

		return re.createSecurityViolationError(
			command.Name,
			privilegeResult.RiskLevel,
			privilegeResult.DetectedPattern,
			maxRiskLevel,
			privilegeResult,
			fmt.Sprintf("Command '%s' requires privilege escalation but 'privileged=true' is not set", command.Name))
	}

	// If command is marked as privileged, allow privilege escalation
	if command.Privileged && privilegeResult != nil && privilegeResult.IsPrivilegeEscalation {
		re.logger.Info("Command execution allowed due to privileged flag",
			"command", command.Name,
			"privilege_type", privilegeResult.EscalationType,
			"risk_level", riskLevel)
		return nil
	}

	// Evaluate basic risk level
	if re.exceedsRiskLevel(riskLevel, maxRiskLevel) {
		re.logSecurityViolation(command.Name, riskLevel, detectedPattern, reason, privilegeResult)

		return re.createSecurityViolationError(
			command.Name,
			riskLevel,
			detectedPattern,
			maxRiskLevel,
			privilegeResult,
			fmt.Sprintf("Command '%s' has %s risk level which exceeds maximum allowed %s",
				command.Name, riskLevel, maxRiskLevel))
	}

	// Command is allowed
	re.logger.Debug("Command execution allowed",
		"command", command.Name,
		"risk_level", riskLevel,
		"max_risk_level", maxRiskLevel,
		"privileged", command.Privileged)

	return nil
}

// parseMaxRiskLevel extracts and validates the max_risk_level from command configuration
func (re *DefaultRiskEvaluator) parseMaxRiskLevel(command *runnertypes.Command) runnertypes.RiskLevel {
	// Use the Command's GetMaxRiskLevel method which handles defaults
	return command.GetMaxRiskLevel()
}

const (
	riskLevelNoneValue   = 0
	riskLevelLowValue    = 1
	riskLevelMediumValue = 2
	riskLevelHighValue   = 3
)

// exceedsRiskLevel checks if the detected risk level exceeds the maximum allowed level
func (re *DefaultRiskEvaluator) exceedsRiskLevel(detectedLevel, maxLevel runnertypes.RiskLevel) bool {
	riskLevels := map[runnertypes.RiskLevel]int{
		runnertypes.RiskLevelNone:   riskLevelNoneValue,
		runnertypes.RiskLevelLow:    riskLevelLowValue,
		runnertypes.RiskLevelMedium: riskLevelMediumValue,
		runnertypes.RiskLevelHigh:   riskLevelHighValue,
	}

	detectedValue, detectedExists := riskLevels[detectedLevel]
	maxValue, maxExists := riskLevels[maxLevel]

	// If either level is invalid, consider it a violation for safety
	if !detectedExists || !maxExists {
		re.logger.Warn("Invalid risk level encountered",
			"detected_level", detectedLevel,
			"max_level", maxLevel)
		return true
	}

	return detectedValue > maxValue
}

// logSecurityViolation logs detailed information about security violations
func (re *DefaultRiskEvaluator) logSecurityViolation(
	commandName string,
	riskLevel runnertypes.RiskLevel,
	detectedPattern string,
	reason string,
	privilegeResult *PrivilegeEscalationResult,
) {
	logAttrs := []slog.Attr{
		slog.String("event", "security_risk_violation"),
		slog.String("command", commandName),
		slog.String("risk_level", string(riskLevel)),
		slog.String("detected_pattern", detectedPattern),
		slog.String("reason", reason),
	}

	if privilegeResult != nil && privilegeResult.IsPrivilegeEscalation {
		logAttrs = append(logAttrs,
			slog.Bool("privilege_escalation", true),
			slog.String("escalation_type", string(privilegeResult.EscalationType)),
			slog.Any("required_privileges", privilegeResult.RequiredPrivileges),
			slog.String("command_path", privilegeResult.CommandPath),
		)
	}

	re.logger.LogAttrs(context.Background(), slog.LevelWarn, "Security violation detected", logAttrs...)
}

// createSecurityViolationError creates a structured security violation error
func (re *DefaultRiskEvaluator) createSecurityViolationError(
	commandName string,
	riskLevel runnertypes.RiskLevel,
	detectedPattern string,
	maxRiskLevel runnertypes.RiskLevel,
	privilegeResult *PrivilegeEscalationResult,
	message string,
) error {
	var privilegeInfo *runnertypes.PrivilegeEscalationInfo
	if privilegeResult != nil && privilegeResult.IsPrivilegeEscalation {
		privilegeInfo = &runnertypes.PrivilegeEscalationInfo{
			EscalationType:     string(privilegeResult.EscalationType),
			RequiredPrivileges: privilegeResult.RequiredPrivileges,
			DetectedPattern:    privilegeResult.DetectedPattern,
			CommandPath:        privilegeResult.CommandPath,
		}
	}

	requiredSetting := re.generateRequiredSetting(riskLevel, maxRiskLevel, privilegeResult)

	return runnertypes.NewSecurityViolationError(
		commandName,
		string(riskLevel),
		detectedPattern,
		requiredSetting,
		re.getCommandPath(privilegeResult),
		"", // runID - will be populated by the caller if needed
		privilegeInfo,
		fmt.Errorf("%w: %s", errSecurityViolation, message),
	)
}

// generateRequiredSetting generates a human-readable description of what setting would allow the command
func (re *DefaultRiskEvaluator) generateRequiredSetting(
	riskLevel runnertypes.RiskLevel,
	maxRiskLevel runnertypes.RiskLevel,
	privilegeResult *PrivilegeEscalationResult,
) string {
	var settings []string

	// If risk level is too high, suggest increasing max_risk_level
	if re.exceedsRiskLevel(riskLevel, maxRiskLevel) {
		settings = append(settings, fmt.Sprintf("max_risk_level = \"%s\"", riskLevel))
	}

	// If privilege escalation is detected, suggest privileged flag
	if privilegeResult != nil && privilegeResult.IsPrivilegeEscalation {
		settings = append(settings, "privileged = true")
	}

	if len(settings) == 0 {
		return "command should be allowed with current settings"
	}

	return strings.Join(settings, " or ")
}

// getCommandPath extracts the command path from privilege result or returns empty string
func (re *DefaultRiskEvaluator) getCommandPath(privilegeResult *PrivilegeEscalationResult) string {
	if privilegeResult != nil {
		return privilegeResult.CommandPath
	}
	return ""
}
