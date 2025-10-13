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
			panicMsg:  "newAllowlistResolution: groupSet cannot be nil",
		},
		{
			name:      "nil global set",
			mode:      InheritanceModeInherit,
			groupName: "test-group",
			groupSet:  map[string]struct{}{"GROUP_VAR": {}},
			globalSet: nil,
			wantPanic: true,
			panicMsg:  "newAllowlistResolution: globalSet cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r != nil {
						if r != tt.panicMsg {
							t.Errorf("newAllowlistResolution() panic = %v, want %v", r, tt.panicMsg)
						}
					} else {
						t.Errorf("newAllowlistResolution() did not panic, expected panic with message: %v", tt.panicMsg)
					}
				}()
			}

			resolution := newAllowlistResolution(tt.mode, tt.groupName, tt.groupSet, tt.globalSet)

			if !tt.wantPanic {
				// Basic validation
				if resolution == nil {
					t.Error("newAllowlistResolution() returned nil")
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
			resolution := newAllowlistResolution(tt.mode, "test-group", groupSet, globalSet)

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
	resolution := newAllowlistResolution(
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
			resolution := newAllowlistResolution(tt.mode, "test-group", groupSet, globalSet)
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
		resolution := newAllowlistResolution(InheritanceModeInherit, "test-group", groupSet, globalSet)
		result := resolution.IsAllowed("")
		if result != false {
			t.Errorf("IsAllowed(\"\") = %v, want false", result)
		}
	})

	t.Run("uninitialized object panics", func(t *testing.T) {
		// Create an AllowlistResolution without using newAllowlistResolution
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
			resolution := newAllowlistResolution(tt.mode, "test-group", groupSet, globalSet)

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

// TestNewAllowlistResolutionBuilder tests the builder constructor
func TestNewAllowlistResolutionBuilder(t *testing.T) {
	builder := NewAllowlistResolutionBuilder()

	if builder == nil {
		t.Fatal("NewAllowlistResolutionBuilder() returned nil")
	}

	if builder.mode != InheritanceModeInherit {
		t.Errorf("default mode = %v, want %v", builder.mode, InheritanceModeInherit)
	}

	if builder.groupName != "" {
		t.Errorf("default groupName = %q, want empty string", builder.groupName)
	}

	if builder.groupVars != nil {
		t.Errorf("default groupVars = %v, want nil", builder.groupVars)
	}

	if builder.globalVars != nil {
		t.Errorf("default globalVars = %v, want nil", builder.globalVars)
	}
}

// TestAllowlistResolutionBuilder_Chaining tests method chaining
func TestAllowlistResolutionBuilder_Chaining(t *testing.T) {
	builder := NewAllowlistResolutionBuilder()

	// Test that each method returns the builder for chaining
	result1 := builder.WithMode(InheritanceModeExplicit)
	if result1 != builder {
		t.Error("WithMode() did not return the same builder instance")
	}

	result2 := builder.WithGroupName("test-group")
	if result2 != builder {
		t.Error("WithGroupName() did not return the same builder instance")
	}

	result3 := builder.WithGroupVariables([]string{"VAR1"})
	if result3 != builder {
		t.Error("WithGroupVariables() did not return the same builder instance")
	}

	result4 := builder.WithGlobalVariables([]string{"VAR2"})
	if result4 != builder {
		t.Error("WithGlobalVariables() did not return the same builder instance")
	}
}

// TestAllowlistResolutionBuilder_Build tests the Build method
func TestAllowlistResolutionBuilder_Build(t *testing.T) {
	tests := []struct {
		name               string
		mode               InheritanceMode
		groupName          string
		groupVars          []string
		globalVars         []string
		expectedMode       InheritanceMode
		expectedGroupName  string
		expectedGroupSize  int
		expectedGlobalSize int
	}{
		{
			name:               "inherit mode with variables",
			mode:               InheritanceModeInherit,
			groupName:          "build",
			groupVars:          []string{"PATH", "HOME"},
			globalVars:         []string{"USER", "SHELL", "PATH"},
			expectedMode:       InheritanceModeInherit,
			expectedGroupName:  "build",
			expectedGroupSize:  2,
			expectedGlobalSize: 3,
		},
		{
			name:               "explicit mode with variables",
			mode:               InheritanceModeExplicit,
			groupName:          "deploy",
			groupVars:          []string{"DEPLOY_KEY", "DEPLOY_ENV"},
			globalVars:         []string{"USER"},
			expectedMode:       InheritanceModeExplicit,
			expectedGroupName:  "deploy",
			expectedGroupSize:  2,
			expectedGlobalSize: 1,
		},
		{
			name:               "reject mode",
			mode:               InheritanceModeReject,
			groupName:          "restricted",
			groupVars:          []string{"VAR1"},
			globalVars:         []string{"VAR2"},
			expectedMode:       InheritanceModeReject,
			expectedGroupName:  "restricted",
			expectedGroupSize:  1,
			expectedGlobalSize: 1,
		},
		{
			name:               "empty variables",
			mode:               InheritanceModeInherit,
			groupName:          "empty",
			groupVars:          []string{},
			globalVars:         []string{},
			expectedMode:       InheritanceModeInherit,
			expectedGroupName:  "empty",
			expectedGroupSize:  0,
			expectedGlobalSize: 0,
		},
		{
			name:               "nil variables",
			mode:               InheritanceModeInherit,
			groupName:          "nil-vars",
			groupVars:          nil,
			globalVars:         nil,
			expectedMode:       InheritanceModeInherit,
			expectedGroupName:  "nil-vars",
			expectedGroupSize:  0,
			expectedGlobalSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := NewAllowlistResolutionBuilder().
				WithMode(tt.mode).
				WithGroupName(tt.groupName).
				WithGroupVariables(tt.groupVars).
				WithGlobalVariables(tt.globalVars).
				Build()

			if resolution == nil {
				t.Fatal("Build() returned nil")
			}

			if resolution.Mode != tt.expectedMode {
				t.Errorf("Mode = %v, want %v", resolution.Mode, tt.expectedMode)
			}

			if resolution.GroupName != tt.expectedGroupName {
				t.Errorf("GroupName = %q, want %q", resolution.GroupName, tt.expectedGroupName)
			}

			// Verify internal sets are properly initialized
			if resolution.groupAllowlistSet == nil {
				t.Error("groupAllowlistSet is nil")
			}

			if resolution.globalAllowlistSet == nil {
				t.Error("globalAllowlistSet is nil")
			}

			if resolution.effectiveSet == nil {
				t.Error("effectiveSet is nil")
			}

			// Verify set sizes
			if len(resolution.groupAllowlistSet) != tt.expectedGroupSize {
				t.Errorf("groupAllowlistSet size = %d, want %d", len(resolution.groupAllowlistSet), tt.expectedGroupSize)
			}

			if len(resolution.globalAllowlistSet) != tt.expectedGlobalSize {
				t.Errorf("globalAllowlistSet size = %d, want %d", len(resolution.globalAllowlistSet), tt.expectedGlobalSize)
			}

			// Verify getters work correctly
			groupList := resolution.GetGroupAllowlist()
			if len(groupList) != tt.expectedGroupSize {
				t.Errorf("GetGroupAllowlist() size = %d, want %d", len(groupList), tt.expectedGroupSize)
			}

			globalList := resolution.GetGlobalAllowlist()
			if len(globalList) != tt.expectedGlobalSize {
				t.Errorf("GetGlobalAllowlist() size = %d, want %d", len(globalList), tt.expectedGlobalSize)
			}
		})
	}
}

// TestAllowlistResolutionBuilder_FluentInterface tests the full fluent interface
func TestAllowlistResolutionBuilder_FluentInterface(t *testing.T) {
	// Test that we can chain all methods together
	resolution := NewAllowlistResolutionBuilder().
		WithMode(InheritanceModeExplicit).
		WithGroupName("test-group").
		WithGroupVariables([]string{"VAR1", "VAR2", "VAR3"}).
		WithGlobalVariables([]string{"GLOBAL1", "GLOBAL2"}).
		Build()

	if resolution == nil {
		t.Fatal("Build() returned nil")
	}

	// Verify the configuration
	if resolution.Mode != InheritanceModeExplicit {
		t.Errorf("Mode = %v, want %v", resolution.Mode, InheritanceModeExplicit)
	}

	if resolution.GroupName != "test-group" {
		t.Errorf("GroupName = %q, want %q", resolution.GroupName, "test-group")
	}

	// In explicit mode, effective list should match group variables
	effectiveList := resolution.GetEffectiveList()
	if len(effectiveList) != 3 {
		t.Errorf("GetEffectiveList() size = %d, want 3", len(effectiveList))
	}

	// Verify variables are accessible
	if !resolution.IsAllowed("VAR1") {
		t.Error("VAR1 should be allowed in explicit mode")
	}

	if !resolution.IsAllowed("VAR2") {
		t.Error("VAR2 should be allowed in explicit mode")
	}

	if resolution.IsAllowed("GLOBAL1") {
		t.Error("GLOBAL1 should not be allowed in explicit mode")
	}
}

// TestAllowlistResolutionBuilder_DefaultMode tests the default inheritance mode
func TestAllowlistResolutionBuilder_DefaultMode(t *testing.T) {
	// Build without specifying mode - should default to Inherit
	resolution := NewAllowlistResolutionBuilder().
		WithGroupName("default-mode").
		WithGroupVariables([]string{"GROUP_VAR"}).
		WithGlobalVariables([]string{"GLOBAL_VAR"}).
		Build()

	if resolution.Mode != InheritanceModeInherit {
		t.Errorf("Default mode = %v, want %v", resolution.Mode, InheritanceModeInherit)
	}

	// In inherit mode, global variables should be accessible
	if !resolution.IsAllowed("GLOBAL_VAR") {
		t.Error("GLOBAL_VAR should be allowed in default (inherit) mode")
	}

	if resolution.IsAllowed("GROUP_VAR") {
		t.Error("GROUP_VAR should not be allowed in inherit mode (only global allowed)")
	}
}

// TestAllowlistResolutionBuilder_Integration tests builder-created resolution behavior
// This is an integration-style test that verifies the builder produces properly functioning
// AllowlistResolution instances with correct inheritance mode behavior.
func TestAllowlistResolutionBuilder_Integration(t *testing.T) {
	tests := []struct {
		name                  string
		mode                  InheritanceMode
		groupVars             []string
		globalVars            []string
		testVariable          string
		expectedAllowed       bool
		expectedEffectiveSize int
	}{
		{
			name:                  "explicit mode allows group variables",
			mode:                  InheritanceModeExplicit,
			groupVars:             []string{"A", "B", "C"},
			globalVars:            []string{"X", "Y"},
			testVariable:          "A",
			expectedAllowed:       true,
			expectedEffectiveSize: 3,
		},
		{
			name:                  "explicit mode denies global variables",
			mode:                  InheritanceModeExplicit,
			groupVars:             []string{"A", "B", "C"},
			globalVars:            []string{"X", "Y"},
			testVariable:          "X",
			expectedAllowed:       false,
			expectedEffectiveSize: 3,
		},
		{
			name:                  "inherit mode allows global variables",
			mode:                  InheritanceModeInherit,
			groupVars:             []string{"A", "B", "C"},
			globalVars:            []string{"X", "Y"},
			testVariable:          "X",
			expectedAllowed:       true,
			expectedEffectiveSize: 2,
		},
		{
			name:                  "inherit mode denies group variables",
			mode:                  InheritanceModeInherit,
			groupVars:             []string{"A", "B", "C"},
			globalVars:            []string{"X", "Y"},
			testVariable:          "A",
			expectedAllowed:       false,
			expectedEffectiveSize: 2,
		},
		{
			name:                  "reject mode denies all variables",
			mode:                  InheritanceModeReject,
			groupVars:             []string{"A", "B", "C"},
			globalVars:            []string{"X", "Y"},
			testVariable:          "A",
			expectedAllowed:       false,
			expectedEffectiveSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := NewAllowlistResolutionBuilder().
				WithMode(tt.mode).
				WithGroupName("test").
				WithGroupVariables(tt.groupVars).
				WithGlobalVariables(tt.globalVars).
				Build()

			// Test IsAllowed behavior
			allowed := resolution.IsAllowed(tt.testVariable)
			if allowed != tt.expectedAllowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.testVariable, allowed, tt.expectedAllowed)
			}

			// Test effective size
			size := resolution.GetEffectiveSize()
			if size != tt.expectedEffectiveSize {
				t.Errorf("GetEffectiveSize() = %d, want %d", size, tt.expectedEffectiveSize)
			}
		})
	}
}

// TestTestAllowlistResolutionFactoryCreateSimple tests the CreateSimple method
func TestTestAllowlistResolutionFactoryCreateSimple(t *testing.T) {
	factory := TestAllowlistResolutionFactory{}
	globalVars := []string{"PATH", "HOME"}
	groupVars := []string{"APP_ENV", "DEBUG"}

	resolution := factory.CreateSimple(globalVars, groupVars)

	if resolution == nil {
		t.Fatal("CreateSimple() returned nil")
	}

	// Check that it uses InheritanceModeInherit
	if resolution.GetMode() != InheritanceModeInherit {
		t.Errorf("Expected mode %v, got %v", InheritanceModeInherit, resolution.GetMode())
	}

	// Check group name
	if resolution.GetGroupName() != "test-group" {
		t.Errorf("Expected group name 'test-group', got '%s'", resolution.GetGroupName())
	}

	// Check that global variables are accessible (since mode is Inherit)
	for _, variable := range globalVars {
		if !resolution.IsAllowed(variable) {
			t.Errorf("Expected variable '%s' to be allowed in Inherit mode", variable)
		}
	}

	// In Inherit mode, group variables should not be in effective list
	for _, variable := range groupVars {
		if resolution.IsAllowed(variable) {
			t.Errorf("Expected variable '%s' to NOT be allowed in Inherit mode", variable)
		}
	}
}

// TestTestAllowlistResolutionFactoryCreateWithMode tests the CreateWithMode method
func TestTestAllowlistResolutionFactoryCreateWithMode(t *testing.T) {
	factory := TestAllowlistResolutionFactory{}
	globalVars := []string{"PATH", "HOME"}
	groupVars := []string{"APP_ENV", "DEBUG"}

	tests := []struct {
		name                  string
		mode                  InheritanceMode
		expectedGlobalAllowed bool
		expectedGroupAllowed  bool
	}{
		{
			name:                  "inherit mode",
			mode:                  InheritanceModeInherit,
			expectedGlobalAllowed: true,
			expectedGroupAllowed:  false,
		},
		{
			name:                  "explicit mode",
			mode:                  InheritanceModeExplicit,
			expectedGlobalAllowed: false,
			expectedGroupAllowed:  true,
		},
		{
			name:                  "reject mode",
			mode:                  InheritanceModeReject,
			expectedGlobalAllowed: false,
			expectedGroupAllowed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := factory.CreateWithMode(tt.mode, globalVars, groupVars)

			if resolution == nil {
				t.Fatal("CreateWithMode() returned nil")
			}

			// Check mode
			if resolution.GetMode() != tt.mode {
				t.Errorf("Expected mode %v, got %v", tt.mode, resolution.GetMode())
			}

			// Check group name
			if resolution.GetGroupName() != "test-group" {
				t.Errorf("Expected group name 'test-group', got '%s'", resolution.GetGroupName())
			}

			// Check global variables
			for _, variable := range globalVars {
				allowed := resolution.IsAllowed(variable)
				if allowed != tt.expectedGlobalAllowed {
					t.Errorf("Global variable '%s': expected allowed=%v, got %v",
						variable, tt.expectedGlobalAllowed, allowed)
				}
			}

			// Check group variables
			for _, variable := range groupVars {
				allowed := resolution.IsAllowed(variable)
				if allowed != tt.expectedGroupAllowed {
					t.Errorf("Group variable '%s': expected allowed=%v, got %v",
						variable, tt.expectedGroupAllowed, allowed)
				}
			}
		})
	}
}

// TestTestAllowlistResolutionFactoryEmpty tests factory with empty variable lists
func TestTestAllowlistResolutionFactoryEmpty(t *testing.T) {
	factory := TestAllowlistResolutionFactory{}

	resolution := factory.CreateSimple([]string{}, []string{})
	if resolution == nil {
		t.Fatal("CreateSimple() with empty lists returned nil")
	}

	// Should not allow any variables
	testVars := []string{"PATH", "HOME", "APP_ENV"}
	for _, variable := range testVars {
		if resolution.IsAllowed(variable) {
			t.Errorf("Expected variable '%s' to be disallowed with empty lists", variable)
		}
	}

	// Effective size should be 0
	if resolution.GetEffectiveSize() != 0 {
		t.Errorf("Expected effective size 0, got %d", resolution.GetEffectiveSize())
	}
}

// TestTestAllowlistResolutionFactoryNil tests factory with nil variable lists
func TestTestAllowlistResolutionFactoryNil(t *testing.T) {
	factory := TestAllowlistResolutionFactory{}

	resolution := factory.CreateSimple(nil, nil)
	if resolution == nil {
		t.Fatal("CreateSimple() with nil lists returned nil")
	}

	// Should not allow any variables
	testVars := []string{"PATH", "HOME", "APP_ENV"}
	for _, variable := range testVars {
		if resolution.IsAllowed(variable) {
			t.Errorf("Expected variable '%s' to be disallowed with nil lists", variable)
		}
	}

	// Effective size should be 0
	if resolution.GetEffectiveSize() != 0 {
		t.Errorf("Expected effective size 0, got %d", resolution.GetEffectiveSize())
	}
}

// TestAllowlistResolutionBuilder_SetBasedAPI tests the set-based builder methods
func TestAllowlistResolutionBuilder_SetBasedAPI(t *testing.T) {
	tests := []struct {
		name                  string
		mode                  InheritanceMode
		groupSet              map[string]struct{}
		globalSet             map[string]struct{}
		testVariable          string
		expectedAllowed       bool
		expectedEffectiveSize int
	}{
		{
			name:                  "explicit mode with sets allows group variables",
			mode:                  InheritanceModeExplicit,
			groupSet:              map[string]struct{}{"A": {}, "B": {}, "C": {}},
			globalSet:             map[string]struct{}{"X": {}, "Y": {}},
			testVariable:          "A",
			expectedAllowed:       true,
			expectedEffectiveSize: 3,
		},
		{
			name:                  "explicit mode with sets denies global variables",
			mode:                  InheritanceModeExplicit,
			groupSet:              map[string]struct{}{"A": {}, "B": {}, "C": {}},
			globalSet:             map[string]struct{}{"X": {}, "Y": {}},
			testVariable:          "X",
			expectedAllowed:       false,
			expectedEffectiveSize: 3,
		},
		{
			name:                  "inherit mode with sets allows global variables",
			mode:                  InheritanceModeInherit,
			groupSet:              map[string]struct{}{"A": {}, "B": {}, "C": {}},
			globalSet:             map[string]struct{}{"X": {}, "Y": {}},
			testVariable:          "X",
			expectedAllowed:       true,
			expectedEffectiveSize: 2,
		},
		{
			name:                  "empty sets",
			mode:                  InheritanceModeInherit,
			groupSet:              map[string]struct{}{},
			globalSet:             map[string]struct{}{},
			testVariable:          "ANY",
			expectedAllowed:       false,
			expectedEffectiveSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := NewAllowlistResolutionBuilder().
				WithMode(tt.mode).
				WithGroupName("test-set-api").
				WithGroupVariablesSet(tt.groupSet).
				WithGlobalVariablesSet(tt.globalSet).
				Build()

			// Test IsAllowed behavior
			allowed := resolution.IsAllowed(tt.testVariable)
			if allowed != tt.expectedAllowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.testVariable, allowed, tt.expectedAllowed)
			}

			// Test effective size
			size := resolution.GetEffectiveSize()
			if size != tt.expectedEffectiveSize {
				t.Errorf("GetEffectiveSize() = %d, want %d", size, tt.expectedEffectiveSize)
			}
		})
	}
}

// TestAllowlistResolutionBuilder_SetPriority tests that sets take precedence over slices
func TestAllowlistResolutionBuilder_SetPriority(t *testing.T) {
	groupSlice := []string{"SLICE_VAR"}
	globalSlice := []string{"GLOBAL_SLICE"}
	groupSet := map[string]struct{}{"SET_VAR": {}}
	globalSet := map[string]struct{}{"GLOBAL_SET": {}}

	// Set both slices and sets - sets should take precedence
	resolution := NewAllowlistResolutionBuilder().
		WithMode(InheritanceModeExplicit).
		WithGroupName("priority-test").
		WithGroupVariables(groupSlice).
		WithGlobalVariables(globalSlice).
		WithGroupVariablesSet(groupSet).
		WithGlobalVariablesSet(globalSet).
		Build()

	// In explicit mode, only group variables should be allowed
	// Since set takes precedence, only SET_VAR should be allowed
	if !resolution.IsAllowed("SET_VAR") {
		t.Error("SET_VAR should be allowed (from set)")
	}

	if resolution.IsAllowed("SLICE_VAR") {
		t.Error("SLICE_VAR should not be allowed (set takes precedence)")
	}

	if resolution.IsAllowed("GLOBAL_SET") {
		t.Error("GLOBAL_SET should not be allowed in explicit mode")
	}

	if resolution.IsAllowed("GLOBAL_SLICE") {
		t.Error("GLOBAL_SLICE should not be allowed in explicit mode")
	}

	// Effective size should be 1 (only SET_VAR)
	if resolution.GetEffectiveSize() != 1 {
		t.Errorf("GetEffectiveSize() = %d, want 1", resolution.GetEffectiveSize())
	}
}

// TestAllowlistResolutionBuilder_SliceAndSetEquivalence tests that slice and set APIs produce equivalent results
func TestAllowlistResolutionBuilder_SliceAndSetEquivalence(t *testing.T) {
	groupVars := []string{"A", "B", "C"}
	globalVars := []string{"X", "Y", "Z"}

	groupSet := map[string]struct{}{"A": {}, "B": {}, "C": {}}
	globalSet := map[string]struct{}{"X": {}, "Y": {}, "Z": {}}

	// Create two resolutions - one with slices, one with sets
	resolutionSlice := NewAllowlistResolutionBuilder().
		WithMode(InheritanceModeExplicit).
		WithGroupName("slice-test").
		WithGroupVariables(groupVars).
		WithGlobalVariables(globalVars).
		Build()

	resolutionSet := NewAllowlistResolutionBuilder().
		WithMode(InheritanceModeExplicit).
		WithGroupName("set-test").
		WithGroupVariablesSet(groupSet).
		WithGlobalVariablesSet(globalSet).
		Build()

	// Both should behave identically
	testVars := []string{"A", "B", "C", "X", "Y", "Z", "UNKNOWN"}
	for _, v := range testVars {
		sliceResult := resolutionSlice.IsAllowed(v)
		setResult := resolutionSet.IsAllowed(v)

		if sliceResult != setResult {
			t.Errorf("IsAllowed(%q) slice=%v, set=%v - should be equal", v, sliceResult, setResult)
		}
	}

	// Verify effective sizes match
	if resolutionSlice.GetEffectiveSize() != resolutionSet.GetEffectiveSize() {
		t.Errorf("GetEffectiveSize() slice=%d, set=%d - should be equal",
			resolutionSlice.GetEffectiveSize(), resolutionSet.GetEffectiveSize())
	}
}
