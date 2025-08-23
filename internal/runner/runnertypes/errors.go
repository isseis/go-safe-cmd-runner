package runnertypes

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PrivilegeEscalationInfo contains detailed information about privilege escalation
type PrivilegeEscalationInfo struct {
	// EscalationType specifies the type of privilege escalation detected
	EscalationType string `json:"escalation_type"`

	// RequiredPrivileges lists the specific privileges required
	RequiredPrivileges []string `json:"required_privileges"`

	// DetectedPattern describes the pattern that triggered the detection
	DetectedPattern string `json:"detected_pattern"`

	// CommandPath is the resolved absolute path of the command
	CommandPath string `json:"command_path"`
}

// SecurityViolationError represents a security policy violation
type SecurityViolationError struct {
	// Command is the command that violated security policy
	Command string `json:"command"`

	// DetectedRisk describes the risk level that was detected
	DetectedRisk string `json:"detected_risk"`

	// DetectedPattern describes the pattern that triggered the violation
	DetectedPattern string `json:"detected_pattern"`

	// RequiredSetting describes what setting would allow this command
	RequiredSetting string `json:"required_setting"`

	// CommandPath is the resolved path of the command
	CommandPath string `json:"command_path"`

	// RunID is the unique identifier for this execution attempt
	RunID string `json:"run_id"`

	// PrivilegeInfo contains detailed information about privilege escalation if applicable
	PrivilegeInfo *PrivilegeEscalationInfo `json:"privilege_info,omitempty"`

	// underlying error, if any
	underlying error
}

// Error returns the error message for SecurityViolationError
func (e *SecurityViolationError) Error() string {
	if e.PrivilegeInfo != nil {
		return fmt.Sprintf("security violation: command '%s' detected as %s risk with privilege escalation (%s). Pattern: %s. Required setting: %s",
			e.Command, e.DetectedRisk, e.PrivilegeInfo.EscalationType, e.DetectedPattern, e.RequiredSetting)
	}

	return fmt.Sprintf("security violation: command '%s' detected as %s risk. Pattern: %s. Required setting: %s",
		e.Command, e.DetectedRisk, e.DetectedPattern, e.RequiredSetting)
}

// Is implements the error interface for error comparison
func (e *SecurityViolationError) Is(target error) bool {
	var secErr *SecurityViolationError
	return errors.As(target, &secErr)
}

// Unwrap returns the underlying error
func (e *SecurityViolationError) Unwrap() error {
	return e.underlying
}

// MarshalJSON implements json.Marshaler for SecurityViolationError
func (e *SecurityViolationError) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type            string                   `json:"type"`
		Command         string                   `json:"command"`
		DetectedRisk    string                   `json:"detected_risk"`
		DetectedPattern string                   `json:"detected_pattern"`
		RequiredSetting string                   `json:"required_setting"`
		CommandPath     string                   `json:"command_path"`
		RunID           string                   `json:"run_id"`
		PrivilegeInfo   *PrivilegeEscalationInfo `json:"privilege_info,omitempty"`
		Message         string                   `json:"message"`
	}{
		Type:            "SecurityViolationError",
		Command:         e.Command,
		DetectedRisk:    e.DetectedRisk,
		DetectedPattern: e.DetectedPattern,
		RequiredSetting: e.RequiredSetting,
		CommandPath:     e.CommandPath,
		RunID:           e.RunID,
		PrivilegeInfo:   e.PrivilegeInfo,
		Message:         e.Error(),
	})
}

// NewSecurityViolationError creates a new SecurityViolationError
func NewSecurityViolationError(command, detectedRisk, detectedPattern, requiredSetting, commandPath, runID string, privilegeInfo *PrivilegeEscalationInfo, underlying error) *SecurityViolationError {
	return &SecurityViolationError{
		Command:         command,
		DetectedRisk:    detectedRisk,
		DetectedPattern: detectedPattern,
		RequiredSetting: requiredSetting,
		CommandPath:     commandPath,
		RunID:           runID,
		PrivilegeInfo:   privilegeInfo,
		underlying:      underlying,
	}
}

// IsSecurityViolationError checks if an error is a SecurityViolationError
func IsSecurityViolationError(err error) bool {
	var secErr *SecurityViolationError
	return errors.As(err, &secErr)
}

// GetSecurityViolationError extracts SecurityViolationError from an error chain
func GetSecurityViolationError(err error) (*SecurityViolationError, bool) {
	var secErr *SecurityViolationError
	if errors.As(err, &secErr) {
		return secErr, true
	}
	return nil, false
}
