//go:build test

package runnertypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetermineEnvAllowlistInheritanceMode_Inherit_NilSlice(t *testing.T) {
	// Arrange
	var envAllowed []string

	// Act
	mode := DetermineEnvAllowlistInheritanceMode(envAllowed)

	// Assert
	assert.Equal(t, InheritanceModeInherit, mode,
		"nil slice should result in Inherit mode")
}

func TestDetermineEnvAllowlistInheritanceMode_Reject_EmptySlice(t *testing.T) {
	// Arrange
	envAllowed := []string{}

	// Act
	mode := DetermineEnvAllowlistInheritanceMode(envAllowed)

	// Assert
	assert.Equal(t, InheritanceModeReject, mode,
		"empty slice should result in Reject mode")
}

func TestDetermineEnvAllowlistInheritanceMode_Explicit_SingleElement(t *testing.T) {
	// Arrange
	envAllowed := []string{"VAR"}

	// Act
	mode := DetermineEnvAllowlistInheritanceMode(envAllowed)

	// Assert
	assert.Equal(t, InheritanceModeExplicit, mode,
		"single element slice should result in Explicit mode")
}

func TestDetermineEnvAllowlistInheritanceMode_Explicit_MultipleElements(t *testing.T) {
	// Arrange
	envAllowed := []string{"VAR1", "VAR2", "VAR3"}

	// Act
	mode := DetermineEnvAllowlistInheritanceMode(envAllowed)

	// Assert
	assert.Equal(t, InheritanceModeExplicit, mode,
		"multiple elements slice should result in Explicit mode")
}

// TestDetermineEnvAllowlistInheritanceMode is a table-driven test that
// comprehensively tests all cases for inheritance mode determination
func TestDetermineEnvAllowlistInheritanceMode(t *testing.T) {
	tests := []struct {
		name       string
		envAllowed []string
		expected   InheritanceMode
	}{
		{
			name:       "Inherit_NilSlice",
			envAllowed: nil,
			expected:   InheritanceModeInherit,
		},
		{
			name:       "Reject_EmptySlice",
			envAllowed: []string{},
			expected:   InheritanceModeReject,
		},
		{
			name:       "Explicit_SingleElement",
			envAllowed: []string{"VAR"},
			expected:   InheritanceModeExplicit,
		},
		{
			name:       "Explicit_TwoElements",
			envAllowed: []string{"VAR1", "VAR2"},
			expected:   InheritanceModeExplicit,
		},
		{
			name:       "Explicit_ManyElements",
			envAllowed: []string{"A", "B", "C", "D", "E"},
			expected:   InheritanceModeExplicit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := DetermineEnvAllowlistInheritanceMode(tt.envAllowed)
			assert.Equal(t, tt.expected, mode)
		})
	}
}
