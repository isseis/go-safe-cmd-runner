package runnertypes

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PrivilegeEscalationInfo contains information about privilege escalation for error reporting
type PrivilegeEscalationInfo struct {
	IsPrivilegeEscalation bool     `json:"is_privilege_escalation"`
	EscalationType        string   `json:"escalation_type"`
	RequiredPrivileges    []string `json:"required_privileges"`
	DetectedPattern       string   `json:"detected_pattern"`
}

// SecurityViolationError represents a security violation during command execution
type SecurityViolationError struct {
	Command         string                   `json:"command"`
	DetectedRisk    string                   `json:"detected_risk"`
	DetectedPattern string                   `json:"detected_pattern"`
	RequiredSetting string                   `json:"required_setting"`
	CommandPath     string                   `json:"command_path"`
	RunID           string                   `json:"run_id"`
	PrivilegeInfo   *PrivilegeEscalationInfo `json:"privilege_info,omitempty"`
}

// Error implements the error interface
func (e *SecurityViolationError) Error() string {
	if e.PrivilegeInfo != nil && e.PrivilegeInfo.IsPrivilegeEscalation {
		// Build privilege escalation details
		privilegeDetails := fmt.Sprintf("privilege escalation detected (type: %s, requires: %v)",
			e.PrivilegeInfo.EscalationType, e.PrivilegeInfo.RequiredPrivileges)

		// Add pattern information, avoiding redundancy if patterns are the same
		if e.PrivilegeInfo.DetectedPattern != "" && e.PrivilegeInfo.DetectedPattern != e.DetectedPattern {
			privilegeDetails += fmt.Sprintf(", escalation pattern: %s", e.PrivilegeInfo.DetectedPattern)
		}

		return fmt.Sprintf("security violation: command '%s' has risk level '%s' which exceeds allowed limit (%s). "+
			"%s. Risk pattern: %s. Path: %s",
			e.Command, e.DetectedRisk, e.RequiredSetting, privilegeDetails,
			e.DetectedPattern, e.CommandPath)
	}

	return fmt.Sprintf("security violation: command '%s' has risk level '%s' which exceeds allowed limit (%s). "+
		"Risk pattern: %s. Path: %s",
		e.Command, e.DetectedRisk, e.RequiredSetting, e.DetectedPattern, e.CommandPath)
}

// Is implements the error comparison interface
func (e *SecurityViolationError) Is(target error) bool {
	var sve *SecurityViolationError
	return errors.As(target, &sve)
}

// Unwrap returns the underlying error (if any)
func (e *SecurityViolationError) Unwrap() error {
	return nil
}

// MarshalJSON implements json.Marshaler interface
func (e *SecurityViolationError) MarshalJSON() ([]byte, error) {
	type Alias SecurityViolationError
	return json.Marshal((*Alias)(e))
}

// NewSecurityViolationError creates a new SecurityViolationError
func NewSecurityViolationError(
	command, detectedRisk, detectedPattern, requiredSetting, commandPath, runID string,
	privilegeInfo *PrivilegeEscalationInfo,
) *SecurityViolationError {
	return &SecurityViolationError{
		Command:         command,
		DetectedRisk:    detectedRisk,
		DetectedPattern: detectedPattern,
		RequiredSetting: requiredSetting,
		CommandPath:     commandPath,
		RunID:           runID,
		PrivilegeInfo:   privilegeInfo,
	}
}

// IsSecurityViolationError checks if an error is a SecurityViolationError
func IsSecurityViolationError(err error) bool {
	var sve *SecurityViolationError
	return errors.As(err, &sve)
}

// AsSecurityViolationError attempts to convert an error to SecurityViolationError
func AsSecurityViolationError(err error) (*SecurityViolationError, bool) {
	var sve *SecurityViolationError
	if errors.As(err, &sve) {
		return sve, true
	}
	return nil, false
}

// ReservedEnvPrefixError represents an error when user tries to use reserved env prefix
type ReservedEnvPrefixError struct {
	VarName string
	Prefix  string
}

// NewReservedEnvPrefixError creates a new ReservedEnvPrefixError
func NewReservedEnvPrefixError(varName, prefix string) *ReservedEnvPrefixError {
	return &ReservedEnvPrefixError{
		VarName: varName,
		Prefix:  prefix,
	}
}

// Error implements the error interface
func (e *ReservedEnvPrefixError) Error() string {
	return fmt.Sprintf(
		"environment variable %q uses reserved prefix %q; "+
			"this prefix is reserved for automatically generated variables",
		e.VarName,
		e.Prefix,
	)
}

// Is implements the error comparison interface
func (e *ReservedEnvPrefixError) Is(target error) bool {
	var rpe *ReservedEnvPrefixError
	return errors.As(target, &rpe)
}

// Unwrap returns the underlying error (if any)
func (e *ReservedEnvPrefixError) Unwrap() error {
	return nil
}
