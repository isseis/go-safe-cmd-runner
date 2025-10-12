//go:build test

package runnertypes

import (
	"testing"
)

// TestNewAllowlistResolution tests the new Phase 2 constructor
func TestNewAllowlistResolution(t *testing.T) {
	tests := []struct {
		name      string
		mode      InheritanceMode
		groupName string
		groupSet  map[string]struct{}
		globalSet map[string]struct{}
		wantPanic bool
		panicMsg  string
	}{
		{
			name:      "valid inherit mode",
			mode:      InheritanceModeInherit,
			groupName: "test-group",
			groupSet:  map[string]struct{}{"GROUP_VAR": {}},
			globalSet: map[string]struct{}{"GLOBAL_VAR": {}},
			wantPanic: false,
		},
		{
			name:      "valid explicit mode",
			mode:      InheritanceModeExplicit,
			groupName: "test-group",
			groupSet:  map[string]struct{}{"GROUP_VAR": {}},
			globalSet: map[string]struct{}{"GLOBAL_VAR": {}},
			wantPanic: false,
		},
		{
			name:      "valid reject mode",
			mode:      InheritanceModeReject,
			groupName: "test-group",
			groupSet:  map[string]struct{}{"GROUP_VAR": {}},
			globalSet: map[string]struct{}{"GLOBAL_VAR": {}},
			wantPanic: false,
		},
		{
			name:      "nil group set",
			mode:      InheritanceModeInherit,
			groupName: "test-group",
			groupSet:  nil,
			globalSet: map[string]struct{}{"GLOBAL_VAR": {}},
			wantPanic: true,
			panicMsg:  "NewAllowlistResolution: groupSet cannot be nil",
		},
		{
			name:      "nil global set",
			mode:      InheritanceModeInherit,
			groupName: "test-group",
			groupSet:  map[string]struct{}{"GROUP_VAR": {}},
			globalSet: nil,
			wantPanic: true,
			panicMsg:  "NewAllowlistResolution: globalSet cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r != nil {
						if r != tt.panicMsg {
							t.Errorf("NewAllowlistResolution() panic = %v, want %v", r, tt.panicMsg)
						}
					} else {
						t.Errorf("NewAllowlistResolution() did not panic, expected panic with message: %v", tt.panicMsg)
					}
				}()
			}

			resolution := NewAllowlistResolution(tt.mode, tt.groupName, tt.groupSet, tt.globalSet)

			if !tt.wantPanic {
				// Basic validation
				if resolution == nil {
					t.Error("NewAllowlistResolution() returned nil")
					return
				}

				if resolution.Mode != tt.mode {
					t.Errorf("Mode = %v, want %v", resolution.Mode, tt.mode)
				}

				if resolution.GroupName != tt.groupName {
					t.Errorf("GroupName = %v, want %v", resolution.GroupName, tt.groupName)
				}

				// Verify effectiveSet is computed
				if resolution.effectiveSet == nil {
					t.Error("effectiveSet is nil after constructor")
				}

				// Verify internal sets are properly assigned
				if resolution.groupAllowlistSet == nil {
					t.Error("groupAllowlistSet is nil after constructor")
				}

				if resolution.globalAllowlistSet == nil {
					t.Error("globalAllowlistSet is nil after constructor")
				}
			}
		})
	}
}

// TestComputeEffectiveSet tests the computeEffectiveSet method
func TestComputeEffectiveSet(t *testing.T) {
	groupSet := map[string]struct{}{"GROUP_VAR": {}, "SHARED_VAR": {}}
	globalSet := map[string]struct{}{"GLOBAL_VAR": {}, "SHARED_VAR": {}}

	tests := []struct {
		name     string
		mode     InheritanceMode
		expected map[string]struct{}
	}{
		{
			name:     "inherit mode uses global set",
			mode:     InheritanceModeInherit,
			expected: globalSet,
		},
		{
			name:     "explicit mode uses group set",
			mode:     InheritanceModeExplicit,
			expected: groupSet,
		},
		{
			name:     "reject mode uses empty set",
			mode:     InheritanceModeReject,
			expected: map[string]struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := NewAllowlistResolution(tt.mode, "test-group", groupSet, globalSet)

			// For inherit/explicit/reject modes, verify effectiveSet has correct content
			switch tt.mode {
			case InheritanceModeInherit:
				// Should have same content as globalSet
				for key := range globalSet {
					if _, exists := resolution.effectiveSet[key]; !exists {
						t.Errorf("inherit mode effectiveSet missing global key: %s", key)
					}
				}
			case InheritanceModeExplicit:
				// Should have same content as groupSet
				for key := range groupSet {
					if _, exists := resolution.effectiveSet[key]; !exists {
						t.Errorf("explicit mode effectiveSet missing group key: %s", key)
					}
				}
			case InheritanceModeReject:
				if len(resolution.effectiveSet) != 0 {
					t.Error("reject mode should have empty effectiveSet")
				}
			}

			// Verify the effective set has the expected content
			if len(resolution.effectiveSet) != len(tt.expected) {
				t.Errorf("effectiveSet size = %d, want %d", len(resolution.effectiveSet), len(tt.expected))
			}

			for key := range tt.expected {
				if _, exists := resolution.effectiveSet[key]; !exists {
					t.Errorf("effectiveSet missing key: %s", key)
				}
			}
		})
	}
}

// TestSetToSortedSlice tests the setToSortedSlice helper method
func TestSetToSortedSlice(t *testing.T) {
	// Create a minimal valid resolution for testing the helper method
	resolution := NewAllowlistResolution(
		InheritanceModeInherit,
		"test",
		map[string]struct{}{}, // empty group set
		map[string]struct{}{}, // empty global set
	)

	tests := []struct {
		name     string
		input    map[string]struct{}
		expected []string
	}{
		{
			name:     "empty set",
			input:    map[string]struct{}{},
			expected: []string{},
		},
		{
			name:     "single item",
			input:    map[string]struct{}{"VAR": {}},
			expected: []string{"VAR"},
		},
		{
			name:     "multiple items sorted",
			input:    map[string]struct{}{"C": {}, "A": {}, "B": {}},
			expected: []string{"A", "B", "C"},
		},
		{
			name:     "nil set",
			input:    nil,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolution.setToSortedSlice(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("length = %d, want %d", len(result), len(tt.expected))
				return
			}

			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("result[%d] = %s, want %s", i, v, tt.expected[i])
				}
			}
		})
	}
}

// TestIsAllowedOptimized tests the optimized Phase 2 IsAllowed method
func TestIsAllowedOptimized(t *testing.T) {
	groupSet := map[string]struct{}{"GROUP_VAR": {}, "SHARED_VAR": {}}
	globalSet := map[string]struct{}{"GLOBAL_VAR": {}, "SHARED_VAR": {}}

	tests := []struct {
		name     string
		mode     InheritanceMode
		variable string
		expected bool
	}{
		// Inherit mode tests
		{
			name:     "inherit mode allows global variable",
			mode:     InheritanceModeInherit,
			variable: "GLOBAL_VAR",
			expected: true,
		},
		{
			name:     "inherit mode allows shared variable",
			mode:     InheritanceModeInherit,
			variable: "SHARED_VAR",
			expected: true,
		},
		{
			name:     "inherit mode denies group-only variable",
			mode:     InheritanceModeInherit,
			variable: "GROUP_VAR",
			expected: false,
		},
		{
			name:     "inherit mode denies unknown variable",
			mode:     InheritanceModeInherit,
			variable: "UNKNOWN_VAR",
			expected: false,
		},

		// Explicit mode tests
		{
			name:     "explicit mode allows group variable",
			mode:     InheritanceModeExplicit,
			variable: "GROUP_VAR",
			expected: true,
		},
		{
			name:     "explicit mode allows shared variable",
			mode:     InheritanceModeExplicit,
			variable: "SHARED_VAR",
			expected: true,
		},
		{
			name:     "explicit mode denies global-only variable",
			mode:     InheritanceModeExplicit,
			variable: "GLOBAL_VAR",
			expected: false,
		},
		{
			name:     "explicit mode denies unknown variable",
			mode:     InheritanceModeExplicit,
			variable: "UNKNOWN_VAR",
			expected: false,
		},

		// Reject mode tests
		{
			name:     "reject mode denies group variable",
			mode:     InheritanceModeReject,
			variable: "GROUP_VAR",
			expected: false,
		},
		{
			name:     "reject mode denies global variable",
			mode:     InheritanceModeReject,
			variable: "GLOBAL_VAR",
			expected: false,
		},
		{
			name:     "reject mode denies shared variable",
			mode:     InheritanceModeReject,
			variable: "SHARED_VAR",
			expected: false,
		},
		{
			name:     "reject mode denies unknown variable",
			mode:     InheritanceModeReject,
			variable: "UNKNOWN_VAR",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := NewAllowlistResolution(tt.mode, "test-group", groupSet, globalSet)
			result := resolution.IsAllowed(tt.variable)

			if result != tt.expected {
				t.Errorf("IsAllowed(%s) = %v, want %v", tt.variable, result, tt.expected)
			}
		})
	}
}

// TestIsAllowedEdgeCases tests edge cases for the optimized IsAllowed method
func TestIsAllowedEdgeCases(t *testing.T) {
	groupSet := map[string]struct{}{"GROUP_VAR": {}}
	globalSet := map[string]struct{}{"GLOBAL_VAR": {}}

	t.Run("nil receiver panics", func(t *testing.T) {
		var resolution *AllowlistResolution
		defer func() {
			if r := recover(); r == nil {
				t.Error("IsAllowed() did not panic with nil receiver")
			}
		}()
		_ = resolution.IsAllowed("ANY_VAR")
	})

	t.Run("empty variable name returns false", func(t *testing.T) {
		resolution := NewAllowlistResolution(InheritanceModeInherit, "test-group", groupSet, globalSet)
		result := resolution.IsAllowed("")
		if result != false {
			t.Errorf("IsAllowed(\"\") = %v, want false", result)
		}
	})

	t.Run("uninitialized object panics", func(t *testing.T) {
		// Create an AllowlistResolution without using NewAllowlistResolution
		resolution := &AllowlistResolution{
			Mode:               InheritanceModeInherit,
			GroupName:          "test-group",
			groupAllowlistSet:  groupSet,
			globalAllowlistSet: globalSet,
			// effectiveSet is nil - this should cause panic
		}

		defer func() {
			if r := recover(); r != nil {
				expectedMsg := "AllowlistResolution: effectiveSet is nil - object not properly initialized"
				if r != expectedMsg {
					t.Errorf("panic message = %v, want %v", r, expectedMsg)
				}
			} else {
				t.Error("IsAllowed() should panic with uninitialized effectiveSet")
			}
		}()

		resolution.IsAllowed("ANY_VAR")
	})
}

// TestLazyEvaluationGetters tests the Phase 2 lazy evaluation getter methods
func TestLazyEvaluationGetters(t *testing.T) {
	groupSet := map[string]struct{}{"GROUP_B": {}, "GROUP_A": {}, "SHARED": {}}
	globalSet := map[string]struct{}{"GLOBAL_B": {}, "GLOBAL_A": {}, "SHARED": {}}

	tests := []struct {
		name              string
		mode              InheritanceMode
		expectedGroup     []string
		expectedGlobal    []string
		expectedEffective []string
	}{
		{
			name:              "inherit mode",
			mode:              InheritanceModeInherit,
			expectedGroup:     []string{"GROUP_A", "GROUP_B", "SHARED"},
			expectedGlobal:    []string{"GLOBAL_A", "GLOBAL_B", "SHARED"},
			expectedEffective: []string{"GLOBAL_A", "GLOBAL_B", "SHARED"},
		},
		{
			name:              "explicit mode",
			mode:              InheritanceModeExplicit,
			expectedGroup:     []string{"GROUP_A", "GROUP_B", "SHARED"},
			expectedGlobal:    []string{"GLOBAL_A", "GLOBAL_B", "SHARED"},
			expectedEffective: []string{"GROUP_A", "GROUP_B", "SHARED"},
		},
		{
			name:              "reject mode",
			mode:              InheritanceModeReject,
			expectedGroup:     []string{"GROUP_A", "GROUP_B", "SHARED"},
			expectedGlobal:    []string{"GLOBAL_A", "GLOBAL_B", "SHARED"},
			expectedEffective: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := NewAllowlistResolution(tt.mode, "test-group", groupSet, globalSet)

			// Test GetGroupAllowlist
			groupResult := resolution.GetGroupAllowlist()
			if !slicesEqual(groupResult, tt.expectedGroup) {
				t.Errorf("GetGroupAllowlist() = %v, want %v", groupResult, tt.expectedGroup)
			}

			// Test GetGlobalAllowlist
			globalResult := resolution.GetGlobalAllowlist()
			if !slicesEqual(globalResult, tt.expectedGlobal) {
				t.Errorf("GetGlobalAllowlist() = %v, want %v", globalResult, tt.expectedGlobal)
			}

			// Test GetEffectiveList
			effectiveResult := resolution.GetEffectiveList()
			if !slicesEqual(effectiveResult, tt.expectedEffective) {
				t.Errorf("GetEffectiveList() = %v, want %v", effectiveResult, tt.expectedEffective)
			}

			// Test GetEffectiveSize
			effectiveSize := resolution.GetEffectiveSize()
			if effectiveSize != len(tt.expectedEffective) {
				t.Errorf("GetEffectiveSize() = %d, want %d", effectiveSize, len(tt.expectedEffective))
			}

			// Test caching - call again and verify same result
			groupResult2 := resolution.GetGroupAllowlist()
			if !slicesEqual(groupResult2, tt.expectedGroup) {
				t.Errorf("GetGroupAllowlist() cached = %v, want %v", groupResult2, tt.expectedGroup)
			}

			globalResult2 := resolution.GetGlobalAllowlist()
			if !slicesEqual(globalResult2, tt.expectedGlobal) {
				t.Errorf("GetGlobalAllowlist() cached = %v, want %v", globalResult2, tt.expectedGlobal)
			}

			effectiveResult2 := resolution.GetEffectiveList()
			if !slicesEqual(effectiveResult2, tt.expectedEffective) {
				t.Errorf("GetEffectiveList() cached = %v, want %v", effectiveResult2, tt.expectedEffective)
			}
		})
	}
}

// TestEffectiveSetInvariants tests that effectiveSet invariants are enforced.
// effectiveSet must always be initialized via NewAllowlistResolution.
func TestEffectiveSetInvariants(t *testing.T) {
	t.Run("effectiveSet_nil_causes_panic_in_GetEffectiveList", func(t *testing.T) {
		// effectiveSet being nil is a bug and should panic
		resolution := &AllowlistResolution{
			Mode:         InheritanceModeInherit,
			GroupName:    "test-group",
			effectiveSet: nil, // BUG: should never be nil
		}

		// GetEffectiveList should panic when effectiveSet is nil
		defer func() {
			if r := recover(); r == nil {
				t.Error("GetEffectiveList() did not panic with nil effectiveSet")
			}
		}()
		_ = resolution.GetEffectiveList()
	})

	t.Run("effectiveSet_nil_causes_panic_in_GetEffectiveSize", func(t *testing.T) {
		// effectiveSet being nil is a bug and should panic
		resolution := &AllowlistResolution{
			Mode:         InheritanceModeInherit,
			GroupName:    "test-group",
			effectiveSet: nil, // BUG: should never be nil
		}

		// GetEffectiveSize should panic when effectiveSet is nil
		defer func() {
			if r := recover(); r == nil {
				t.Error("GetEffectiveSize() did not panic with nil effectiveSet")
			}
		}()
		_ = resolution.GetEffectiveSize()
	})
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
