package runnertypes

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Contains(t, msg, "security violation")
	assert.Contains(t, msg, "rm -rf /")
	assert.Contains(t, msg, "high")
	assert.Contains(t, msg, "allow_high_risk=true")
	assert.Contains(t, msg, "/bin/rm")
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
	assert.Contains(t, msg, "privilege escalation detected")
	assert.Contains(t, msg, "type: sudo")
	assert.Contains(t, msg, "requires: [root]")
	assert.Contains(t, msg, "escalation pattern: sudo prefix")
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
	assert.Contains(t, msg, "privilege escalation detected")
	// When patterns are the same, should not add "escalation pattern:"
	assert.NotContains(t, msg, "escalation pattern:", "should not contain 'escalation pattern:' when patterns are the same")
	// Should contain "Risk pattern: sudo"
	assert.Contains(t, msg, "Risk pattern: sudo")
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
		assert.Fail(t, "Is() should return false for non-SecurityViolationError")
	}
}

// TestSecurityViolationError_Unwrap tests the Unwrap() method
func TestSecurityViolationError_Unwrap(t *testing.T) {
	err := &SecurityViolationError{
		Command:      "test",
		DetectedRisk: "high",
	}

	if err.Unwrap() != nil {
		assert.Nil(t, err.Unwrap())
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
	require.NoError(t, jsonErr)

	// Unmarshal to verify structure
	var decoded SecurityViolationError
	unmarshalErr := json.Unmarshal(data, &decoded)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "test command", decoded.Command)
	assert.Equal(t, "high", decoded.DetectedRisk)
	assert.Equal(t, "run-123", decoded.RunID)
	require.NotNil(t, decoded.PrivilegeInfo)
	assert.True(t, decoded.PrivilegeInfo.IsPrivilegeEscalation)
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

	require.NotNil(t, err)
	assert.Equal(t, "test command", err.Command)
	assert.Equal(t, "high", err.DetectedRisk)
	assert.Equal(t, "pattern", err.DetectedPattern)
	assert.Equal(t, "setting", err.RequiredSetting)
	assert.Equal(t, "/path/to/cmd", err.CommandPath)
	assert.Equal(t, "run-123", err.RunID)
	assert.Equal(t, privilegeInfo, err.PrivilegeInfo)
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

	require.NotNil(t, err)
	assert.Nil(t, err.PrivilegeInfo)
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
			assert.Equal(t, tt.want, got)
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
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.wantNil, sve == nil)
			if tt.checkData && sve != nil {
				assert.Equal(t, "test", sve.Command)
				assert.Equal(t, "high", sve.DetectedRisk)
			}
		})
	}
}
