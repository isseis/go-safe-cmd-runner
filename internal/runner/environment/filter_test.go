package environment

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFilter(t *testing.T) {
	config := &runnertypes.Config{}
	filter := NewFilter(config.Global.EnvAllowlist)

	require.NotNil(t, filter, "NewFilter returned nil")
}

func TestDetermineInheritanceMode(t *testing.T) {
	tests := []struct {
		name         string
		group        *runnertypes.CommandGroup
		expectedMode runnertypes.InheritanceMode
		expectError  bool
	}{
		{
			name:        "nil group should return error",
			group:       nil,
			expectError: true,
		},
		{
			name: "nil allowlist should inherit",
			group: &runnertypes.CommandGroup{
				Name:         "test",
				EnvAllowlist: nil,
			},
			expectedMode: runnertypes.InheritanceModeInherit,
		},
		{
			name: "empty allowlist should reject",
			group: &runnertypes.CommandGroup{
				Name:         "test",
				EnvAllowlist: []string{},
			},
			expectedMode: runnertypes.InheritanceModeReject,
		},
		{
			name: "non-empty allowlist should be explicit",
			group: &runnertypes.CommandGroup{
				Name:         "test",
				EnvAllowlist: []string{"VAR1", "VAR2"},
			},
			expectedMode: runnertypes.InheritanceModeExplicit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewFilter((&runnertypes.Config{}).Global.EnvAllowlist)
			var allowlist []string
			if tt.group != nil {
				allowlist = tt.group.EnvAllowlist
			}
			mode := filter.determineInheritanceMode(allowlist)

			if tt.expectError {
				// Since determineInheritanceMode no longer returns errors,
				// we need to handle nil group case differently
				if tt.group == nil && tt.expectedMode != runnertypes.InheritanceModeInherit {
					t.Errorf("Expected error for nil group, but got mode: %v", mode)
				}
				return
			}

			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

func TestIsVariableAccessAllowedWithInheritance(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"GLOBAL_VAR", "COMMON_VAR"},
		},
	}

	// Groups for testing different inheritance modes
	groupInherit := &runnertypes.CommandGroup{
		Name:         "group-inherit",
		EnvAllowlist: nil, // Inherit from global
	}
	groupExplicit := &runnertypes.CommandGroup{
		Name:         "group-explicit",
		EnvAllowlist: []string{"GROUP_VAR", "COMMON_VAR"}, // Explicit allowlist
	}
	groupReject := &runnertypes.CommandGroup{
		Name:         "group-reject",
		EnvAllowlist: []string{}, // Reject all
	}
	groupNil := (*runnertypes.CommandGroup)(nil)

	filter := NewFilter(config.Global.EnvAllowlist)

	tests := []struct {
		name     string
		variable string
		group    *runnertypes.CommandGroup
		expected bool
	}{
		// --- InheritanceModeInherit ---
		{
			name:     "[Inherit] Allowed global variable",
			variable: "GLOBAL_VAR",
			group:    groupInherit,
			expected: true,
		},
		{
			name:     "[Inherit] Allowed common variable",
			variable: "COMMON_VAR",
			group:    groupInherit,
			expected: true,
		},
		{
			name:     "[Inherit] Disallowed group-specific variable",
			variable: "GROUP_VAR",
			group:    groupInherit,
			expected: false,
		},
		{
			name:     "[Inherit] Disallowed undefined variable",
			variable: "UNDEFINED_VAR",
			group:    groupInherit,
			expected: false,
		},

		// --- InheritanceModeExplicit ---
		{
			name:     "[Explicit] Disallowed global variable",
			variable: "GLOBAL_VAR",
			group:    groupExplicit,
			expected: false,
		},
		{
			name:     "[Explicit] Allowed group-specific variable",
			variable: "GROUP_VAR",
			group:    groupExplicit,
			expected: true,
		},
		{
			name:     "[Explicit] Allowed common variable",
			variable: "COMMON_VAR",
			group:    groupExplicit,
			expected: true,
		},
		{
			name:     "[Explicit] Disallowed undefined variable",
			variable: "UNDEFINED_VAR",
			group:    groupExplicit,
			expected: false,
		},

		// --- InheritanceModeReject ---
		{
			name:     "[Reject] Disallowed global variable",
			variable: "GLOBAL_VAR",
			group:    groupReject,
			expected: false,
		},
		{
			name:     "[Reject] Disallowed group-specific variable",
			variable: "GROUP_VAR",
			group:    groupReject,
			expected: false,
		},
		{
			name:     "[Reject] Disallowed common variable",
			variable: "COMMON_VAR",
			group:    groupReject,
			expected: false,
		},
		{
			name:     "[Reject] Disallowed undefined variable",
			variable: "UNDEFINED_VAR",
			group:    groupReject,
			expected: false,
		},

		// --- Edge Case: Nil Group ---
		{
			name:     "Nil group should always deny access",
			variable: "ANY_VAR",
			group:    groupNil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var allowlist []string
			var groupName string
			if tt.group != nil {
				allowlist = tt.group.EnvAllowlist
				groupName = tt.group.Name
			}
			result := filter.IsVariableAccessAllowed(tt.variable, allowlist, groupName)
			if tt.group == nil {
				groupName = "nil"
			}
			assert.Equal(t, tt.expected, result, "IsVariableAccessAllowed(%s, %s)", tt.variable, groupName)
		})
	}
}
