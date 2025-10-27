//go:build test

package runnertypes

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestSecurityViolationError_Error tests the Error() method without privilege escalation
func TestSecurityViolationError_Error(t *testing.T) {
	err := &SecurityViolationError{
		Command:         "rm -rf /",
		DetectedRisk:    "high",
		DetectedPattern: "dangerous command",
		RequiredSetting: "allow_high_risk=true",
		CommandPath:     "/bin/rm",
		RunID:           "test-run-123",
	}

	msg := err.Error()
	if !strings.Contains(msg, "security violation") {
		t.Errorf("Error message should contain 'security violation', got: %s", msg)
	}
	if !strings.Contains(msg, "rm -rf /") {
		t.Errorf("Error message should contain command, got: %s", msg)
	}
	if !strings.Contains(msg, "high") {
		t.Errorf("Error message should contain risk level, got: %s", msg)
	}
	if !strings.Contains(msg, "allow_high_risk=true") {
		t.Errorf("Error message should contain required setting, got: %s", msg)
	}
	if !strings.Contains(msg, "/bin/rm") {
		t.Errorf("Error message should contain command path, got: %s", msg)
	}
}

// TestSecurityViolationError_Error_WithPrivilegeEscalation tests the Error() method with privilege escalation
func TestSecurityViolationError_Error_WithPrivilegeEscalation(t *testing.T) {
	err := &SecurityViolationError{
		Command:         "sudo apt-get install",
		DetectedRisk:    "medium",
		DetectedPattern: "sudo command",
		RequiredSetting: "allow_medium_risk=true",
		CommandPath:     "/usr/bin/sudo",
		RunID:           "test-run-456",
		PrivilegeInfo: &PrivilegeEscalationInfo{
			IsPrivilegeEscalation: true,
			EscalationType:        "sudo",
			RequiredPrivileges:    []string{"root"},
			DetectedPattern:       "sudo prefix",
		},
	}

	msg := err.Error()
	if !strings.Contains(msg, "privilege escalation detected") {
		t.Errorf("Error message should contain 'privilege escalation detected', got: %s", msg)
	}
	if !strings.Contains(msg, "type: sudo") {
		t.Errorf("Error message should contain escalation type, got: %s", msg)
	}
	if !strings.Contains(msg, "requires: [root]") {
		t.Errorf("Error message should contain required privileges, got: %s", msg)
	}
	if !strings.Contains(msg, "escalation pattern: sudo prefix") {
		t.Errorf("Error message should contain escalation pattern, got: %s", msg)
	}
}

// TestSecurityViolationError_Error_WithPrivilegeEscalation_SamePattern tests when patterns are the same
func TestSecurityViolationError_Error_WithPrivilegeEscalation_SamePattern(t *testing.T) {
	err := &SecurityViolationError{
		Command:         "sudo ls",
		DetectedRisk:    "medium",
		DetectedPattern: "sudo",
		RequiredSetting: "allow_medium_risk=true",
		CommandPath:     "/usr/bin/sudo",
		RunID:           "test-run-789",
		PrivilegeInfo: &PrivilegeEscalationInfo{
			IsPrivilegeEscalation: true,
			EscalationType:        "sudo",
			RequiredPrivileges:    []string{"root"},
			DetectedPattern:       "sudo", // Same as DetectedPattern
		},
	}

	msg := err.Error()
	if !strings.Contains(msg, "privilege escalation detected") {
		t.Errorf("Error message should contain 'privilege escalation detected', got: %s", msg)
	}
	// When patterns are the same, should not add "escalation pattern:"
	if strings.Contains(msg, "escalation pattern:") {
		t.Errorf("Error message should not contain 'escalation pattern:' when patterns are the same, got: %s", msg)
	}
	// Should contain "Risk pattern: sudo"
	if !strings.Contains(msg, "Risk pattern: sudo") {
		t.Errorf("Error message should contain 'Risk pattern: sudo', got: %s", msg)
	}
}

// TestSecurityViolationError_Is tests the Is() method
func TestSecurityViolationError_Is(t *testing.T) {
	err1 := &SecurityViolationError{
		Command:      "test",
		DetectedRisk: "high",
	}

	err2 := &SecurityViolationError{
		Command:      "other",
		DetectedRisk: "low",
	}

	if !errors.Is(err1, err2) {
		t.Errorf("Is() should return true for SecurityViolationError instances")
	}

	otherErr := errors.New("different error")
	if errors.Is(err1, otherErr) {
		t.Errorf("Is() should return false for non-SecurityViolationError")
	}
}

// TestSecurityViolationError_Unwrap tests the Unwrap() method
func TestSecurityViolationError_Unwrap(t *testing.T) {
	err := &SecurityViolationError{
		Command:      "test",
		DetectedRisk: "high",
	}

	if err.Unwrap() != nil {
		t.Errorf("Unwrap() should return nil")
	}
}

// TestSecurityViolationError_MarshalJSON tests the MarshalJSON() method
func TestSecurityViolationError_MarshalJSON(t *testing.T) {
	err := &SecurityViolationError{
		Command:         "test command",
		DetectedRisk:    "high",
		DetectedPattern: "pattern",
		RequiredSetting: "setting",
		CommandPath:     "/path/to/cmd",
		RunID:           "run-123",
		PrivilegeInfo: &PrivilegeEscalationInfo{
			IsPrivilegeEscalation: true,
			EscalationType:        "sudo",
			RequiredPrivileges:    []string{"root"},
			DetectedPattern:       "sudo",
		},
	}

	data, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatalf("MarshalJSON() failed: %v", jsonErr)
	}

	// Unmarshal to verify structure
	var decoded SecurityViolationError
	if unmarshalErr := json.Unmarshal(data, &decoded); unmarshalErr != nil {
		t.Fatalf("Unmarshal failed: %v", unmarshalErr)
	}

	if decoded.Command != "test command" {
		t.Errorf("Command = %q, want %q", decoded.Command, "test command")
	}
	if decoded.DetectedRisk != "high" {
		t.Errorf("DetectedRisk = %q, want %q", decoded.DetectedRisk, "high")
	}
	if decoded.RunID != "run-123" {
		t.Errorf("RunID = %q, want %q", decoded.RunID, "run-123")
	}
	if decoded.PrivilegeInfo == nil {
		t.Fatal("PrivilegeInfo should not be nil")
	}
	if !decoded.PrivilegeInfo.IsPrivilegeEscalation {
		t.Error("IsPrivilegeEscalation should be true")
	}
}

// TestNewSecurityViolationError tests the constructor
func TestNewSecurityViolationError(t *testing.T) {
	privilegeInfo := &PrivilegeEscalationInfo{
		IsPrivilegeEscalation: true,
		EscalationType:        "sudo",
		RequiredPrivileges:    []string{"root"},
		DetectedPattern:       "sudo",
	}

	err := NewSecurityViolationError(
		"test command",
		"high",
		"pattern",
		"setting",
		"/path/to/cmd",
		"run-123",
		privilegeInfo,
	)

	if err == nil {
		t.Fatal("NewSecurityViolationError() returned nil")
	}
	if err.Command != "test command" {
		t.Errorf("Command = %q, want %q", err.Command, "test command")
	}
	if err.DetectedRisk != "high" {
		t.Errorf("DetectedRisk = %q, want %q", err.DetectedRisk, "high")
	}
	if err.DetectedPattern != "pattern" {
		t.Errorf("DetectedPattern = %q, want %q", err.DetectedPattern, "pattern")
	}
	if err.RequiredSetting != "setting" {
		t.Errorf("RequiredSetting = %q, want %q", err.RequiredSetting, "setting")
	}
	if err.CommandPath != "/path/to/cmd" {
		t.Errorf("CommandPath = %q, want %q", err.CommandPath, "/path/to/cmd")
	}
	if err.RunID != "run-123" {
		t.Errorf("RunID = %q, want %q", err.RunID, "run-123")
	}
	if err.PrivilegeInfo != privilegeInfo {
		t.Error("PrivilegeInfo not set correctly")
	}
}

// TestNewSecurityViolationError_WithoutPrivilegeInfo tests constructor without privilege info
func TestNewSecurityViolationError_WithoutPrivilegeInfo(t *testing.T) {
	err := NewSecurityViolationError(
		"test command",
		"low",
		"pattern",
		"setting",
		"/path/to/cmd",
		"run-456",
		nil,
	)

	if err == nil {
		t.Fatal("NewSecurityViolationError() returned nil")
	}
	if err.PrivilegeInfo != nil {
		t.Error("PrivilegeInfo should be nil")
	}
}

// TestIsSecurityViolationError tests the helper function
func TestIsSecurityViolationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "SecurityViolationError",
			err: &SecurityViolationError{
				Command:      "test",
				DetectedRisk: "high",
			},
			want: true,
		},
		{
			name: "wrapped SecurityViolationError",
			err: func() error {
				sve := &SecurityViolationError{
					Command:      "test",
					DetectedRisk: "high",
				}
				return errors.New("wrapped: " + sve.Error())
			}(),
			want: false,
		},
		{
			name: "other error",
			err:  errors.New("other error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSecurityViolationError(tt.err)
			if got != tt.want {
				t.Errorf("IsSecurityViolationError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAsSecurityViolationError tests the helper function
func TestAsSecurityViolationError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantOk    bool
		wantNil   bool
		checkData bool
	}{
		{
			name: "SecurityViolationError",
			err: &SecurityViolationError{
				Command:      "test",
				DetectedRisk: "high",
			},
			wantOk:    true,
			wantNil:   false,
			checkData: true,
		},
		{
			name:    "other error",
			err:     errors.New("other error"),
			wantOk:  false,
			wantNil: true,
		},
		{
			name:    "nil error",
			err:     nil,
			wantOk:  false,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sve, ok := AsSecurityViolationError(tt.err)
			if ok != tt.wantOk {
				t.Errorf("AsSecurityViolationError() ok = %v, want %v", ok, tt.wantOk)
			}
			if (sve == nil) != tt.wantNil {
				t.Errorf("AsSecurityViolationError() sve is nil = %v, want %v", sve == nil, tt.wantNil)
			}
			if tt.checkData && sve != nil {
				if sve.Command != "test" {
					t.Errorf("Command = %q, want %q", sve.Command, "test")
				}
				if sve.DetectedRisk != "high" {
					t.Errorf("DetectedRisk = %q, want %q", sve.DetectedRisk, "high")
				}
			}
		})
	}
}
