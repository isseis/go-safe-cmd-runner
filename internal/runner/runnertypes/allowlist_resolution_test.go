//go:build test

package runnertypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewAllowlistResolution tests the allowlist resolution constructor
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
				assert.NotNil(t, resolution, "newAllowlistResolution() should not return nil")

				assert.Equal(t, tt.mode, resolution.Mode)

				assert.Equal(t, tt.groupName, resolution.GroupName)

				// Verify effectiveSet is computed
				assert.NotNil(t, resolution.effectiveSet, "effectiveSet should not be nil after constructor")

				// Verify internal sets are properly assigned
				assert.NotNil(t, resolution.groupAllowlistSet, "groupAllowlistSet should not be nil after constructor")

				assert.NotNil(t, resolution.globalAllowlistSet, "globalAllowlistSet should not be nil after constructor")
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
					assert.Contains(t, resolution.effectiveSet, key, "inherit mode effectiveSet should contain global key")
				}
			case InheritanceModeExplicit:
				// Should have same content as groupSet
				for key := range groupSet {
					assert.Contains(t, resolution.effectiveSet, key, "explicit mode effectiveSet should contain group key")
				}
			case InheritanceModeReject:
				assert.Empty(t, resolution.effectiveSet, "reject mode should have empty effectiveSet")
			}

			// Verify the effective set has the expected content
			assert.Equal(t, len(tt.expected), len(resolution.effectiveSet))

			for key := range tt.expected {
				assert.Contains(t, resolution.effectiveSet, key, "effectiveSet should contain key")
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

			assert.Equal(t, len(tt.expected), len(result))

			for i, v := range result {
				assert.Equal(t, tt.expected[i], v)
			}
		})
	}
}

// TestIsAllowedOptimized tests the optimized IsAllowed method
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

			assert.Equal(t, tt.expected, result)
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
		assert.False(t, result, "IsAllowed(\"\") should return false")
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

// TestLazyEvaluationGetters tests the lazy evaluation getter methods
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
			assert.True(t, slicesEqual(groupResult, tt.expectedGroup), "GetGroupAllowlist() should match expected")

			// Test GetGlobalAllowlist
			globalResult := resolution.GetGlobalAllowlist()
			assert.True(t, slicesEqual(globalResult, tt.expectedGlobal), "GetGlobalAllowlist() should match expected")

			// Test GetEffectiveList
			effectiveResult := resolution.GetEffectiveList()
			assert.True(t, slicesEqual(effectiveResult, tt.expectedEffective), "GetEffectiveList() should match expected")

			// Test GetEffectiveSize
			effectiveSize := resolution.GetEffectiveSize()
			assert.Equal(t, len(tt.expectedEffective), effectiveSize)

			// Test caching - call again and verify same result
			groupResult2 := resolution.GetGroupAllowlist()
			assert.True(t, slicesEqual(groupResult2, tt.expectedGroup), "GetGroupAllowlist() cached should match expected")

			globalResult2 := resolution.GetGlobalAllowlist()
			assert.True(t, slicesEqual(globalResult2, tt.expectedGlobal), "GetGlobalAllowlist() cached should match expected")

			effectiveResult2 := resolution.GetEffectiveList()
			assert.True(t, slicesEqual(effectiveResult2, tt.expectedEffective), "GetEffectiveList() cached should match expected")
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

	assert.Equal(t, InheritanceModeInherit, builder.mode)

	assert.Empty(t, builder.groupName)

	assert.Nil(t, builder.groupVars)

	assert.Nil(t, builder.globalSet)
}

// TestAllowlistResolutionBuilder_Chaining tests method chaining
func TestAllowlistResolutionBuilder_Chaining(t *testing.T) {
	builder := NewAllowlistResolutionBuilder()

	// Test that each method returns the builder for chaining
	result1 := builder.WithMode(InheritanceModeExplicit)
	assert.Same(t, builder, result1, "WithMode() should return the same builder instance")

	result2 := builder.WithGroupName("test-group")
	assert.Same(t, builder, result2, "WithGroupName() should return the same builder instance")

	result3 := builder.WithGroupVariables([]string{"VAR1"})
	assert.Same(t, builder, result3, "WithGroupVariables() should return the same builder instance")

	result4 := builder.WithGlobalVariablesForTest([]string{"VAR2"})
	assert.Same(t, builder, result4, "WithGlobalVariablesForTest() should return the same builder instance")
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
				WithGlobalVariablesForTest(tt.globalVars).
				Build()

			assert.NotNil(t, resolution, "Build() should not return nil")

			assert.Equal(t, tt.expectedMode, resolution.Mode)

			assert.Equal(t, tt.expectedGroupName, resolution.GroupName)

			// Verify internal sets are properly initialized
			assert.NotNil(t, resolution.groupAllowlistSet, "groupAllowlistSet should not be nil")

			assert.NotNil(t, resolution.globalAllowlistSet, "globalAllowlistSet should not be nil")

			assert.NotNil(t, resolution.effectiveSet, "effectiveSet should not be nil")

			// Verify set sizes
			assert.Equal(t, tt.expectedGroupSize, len(resolution.groupAllowlistSet))

			assert.Equal(t, tt.expectedGlobalSize, len(resolution.globalAllowlistSet))

			// Verify getters work correctly
			groupList := resolution.GetGroupAllowlist()
			assert.Equal(t, tt.expectedGroupSize, len(groupList))

			globalList := resolution.GetGlobalAllowlist()
			assert.Equal(t, tt.expectedGlobalSize, len(globalList))
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
		WithGlobalVariablesForTest([]string{"GLOBAL1", "GLOBAL2"}).
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

	assert.True(t, resolution.IsAllowed("VAR2"), "VAR2 should be allowed in explicit mode")

	assert.False(t, resolution.IsAllowed("GLOBAL1"), "GLOBAL1 should not be allowed in explicit mode")
}

// TestAllowlistResolutionBuilder_DefaultMode tests the default inheritance mode
func TestAllowlistResolutionBuilder_DefaultMode(t *testing.T) {
	// Build without specifying mode - should default to Inherit
	resolution := NewAllowlistResolutionBuilder().
		WithGroupName("default-mode").
		WithGroupVariables([]string{"GROUP_VAR"}).
		WithGlobalVariablesForTest([]string{"GLOBAL_VAR"}).
		Build()

	assert.Equal(t, InheritanceModeInherit, resolution.Mode)

	// In inherit mode, global variables should be accessible
	assert.True(t, resolution.IsAllowed("GLOBAL_VAR"), "GLOBAL_VAR should be allowed in default (inherit) mode")

	assert.False(t, resolution.IsAllowed("GROUP_VAR"), "GROUP_VAR should not be allowed in inherit mode (only global allowed)")
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
				WithGlobalVariablesForTest(tt.globalVars).
				Build()

			// Test IsAllowed behavior
			allowed := resolution.IsAllowed(tt.testVariable)
			assert.Equal(t, tt.expectedAllowed, allowed)

			// Test effective size
			size := resolution.GetEffectiveSize()
			assert.Equal(t, tt.expectedEffectiveSize, size)
		})
	}
}

// TestNewTestAllowlistResolutionSimple tests the NewTestAllowlistResolutionSimple function
func TestNewTestAllowlistResolutionSimple(t *testing.T) {
	globalVars := []string{"PATH", "HOME"}
	groupVars := []string{"APP_ENV", "DEBUG"}

	resolution := NewTestAllowlistResolutionSimple(globalVars, groupVars)

	assert.NotNil(t, resolution, "CreateSimple() should not return nil")

	// Check that it uses InheritanceModeInherit
	assert.Equal(t, InheritanceModeInherit, resolution.GetMode())

	// Check group name
	assert.Equal(t, "test-group", resolution.GetGroupName())

	// Check that global variables are accessible (since mode is Inherit)
	for _, variable := range globalVars {
		assert.True(t, resolution.IsAllowed(variable), "Variable '%s' should be allowed in Inherit mode", variable)
	}

	// In Inherit mode, group variables should not be in effective list
	for _, variable := range groupVars {
		assert.False(t, resolution.IsAllowed(variable), "Variable '%s' should NOT be allowed in Inherit mode", variable)
	}
}

// TestNewTestAllowlistResolutionWithMode tests the NewTestAllowlistResolutionWithMode function
func TestNewTestAllowlistResolutionWithMode(t *testing.T) {
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
			resolution := NewTestAllowlistResolutionWithMode(tt.mode, globalVars, groupVars)

			assert.NotNil(t, resolution, "CreateWithMode() should not return nil")

			// Check mode
			assert.Equal(t, tt.mode, resolution.GetMode())

			// Check group name
			assert.Equal(t, "test-group", resolution.GetGroupName())

			// Check global variables
			for _, variable := range globalVars {
				allowed := resolution.IsAllowed(variable)
				assert.Equal(t, tt.expectedGlobalAllowed, allowed,
					"Global variable '%s' allowed status should match", variable)
			}

			// Check group variables
			for _, variable := range groupVars {
				allowed := resolution.IsAllowed(variable)
				assert.Equal(t, tt.expectedGroupAllowed, allowed,
					"Group variable '%s' allowed status should match", variable)
			}
		})
	}
}

// TestNewTestAllowlistResolutionSimpleEmpty tests function with empty variable lists
func TestNewTestAllowlistResolutionSimpleEmpty(t *testing.T) {
	resolution := NewTestAllowlistResolutionSimple([]string{}, []string{})
	assert.NotNil(t, resolution, "CreateSimple() with empty lists should not return nil")

	// Should not allow any variables
	testVars := []string{"PATH", "HOME", "APP_ENV"}
	for _, variable := range testVars {
		assert.False(t, resolution.IsAllowed(variable), "Variable '%s' should be disallowed with empty lists", variable)
	}

	// Effective size should be 0
	assert.Equal(t, 0, resolution.GetEffectiveSize())
}

// TestNewTestAllowlistResolutionSimpleNil tests function with nil variable lists
func TestNewTestAllowlistResolutionSimpleNil(t *testing.T) {
	resolution := NewTestAllowlistResolutionSimple(nil, nil)
	assert.NotNil(t, resolution, "CreateSimple() with nil lists should not return nil")

	// Should not allow any variables
	testVars := []string{"PATH", "HOME", "APP_ENV"}
	for _, variable := range testVars {
		assert.False(t, resolution.IsAllowed(variable), "Variable '%s' should be disallowed with nil lists", variable)
	}

	// Effective size should be 0
	assert.Equal(t, 0, resolution.GetEffectiveSize())
}

// TestAllowlistResolutionBuilder_SetBasedAPI tests the set-based builder methods
func TestAllowlistResolutionBuilder_SetBasedAPI(t *testing.T) {
	tests := []struct {
		name                  string
		mode                  InheritanceMode
		groupVars             []string
		globalSet             map[string]struct{}
		testVariable          string
		expectedAllowed       bool
		expectedEffectiveSize int
	}{
		{
			name:                  "explicit mode allows group variables",
			mode:                  InheritanceModeExplicit,
			groupVars:             []string{"A", "B", "C"},
			globalSet:             map[string]struct{}{"X": {}, "Y": {}},
			testVariable:          "A",
			expectedAllowed:       true,
			expectedEffectiveSize: 3,
		},
		{
			name:                  "explicit mode denies global variables",
			mode:                  InheritanceModeExplicit,
			groupVars:             []string{"A", "B", "C"},
			globalSet:             map[string]struct{}{"X": {}, "Y": {}},
			testVariable:          "X",
			expectedAllowed:       false,
			expectedEffectiveSize: 3,
		},
		{
			name:                  "inherit mode allows global variables",
			mode:                  InheritanceModeInherit,
			groupVars:             []string{"A", "B", "C"},
			globalSet:             map[string]struct{}{"X": {}, "Y": {}},
			testVariable:          "X",
			expectedAllowed:       true,
			expectedEffectiveSize: 2,
		},
		{
			name:                  "empty variables",
			mode:                  InheritanceModeInherit,
			groupVars:             []string{},
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
				WithGroupName("test-api").
				WithGroupVariables(tt.groupVars).
				WithGlobalVariablesSet(tt.globalSet).
				Build()

			// Test IsAllowed behavior
			allowed := resolution.IsAllowed(tt.testVariable)
			if allowed != tt.expectedAllowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.testVariable, allowed, tt.expectedAllowed)
			}

			// Test effective size
			size := resolution.GetEffectiveSize()
			assert.Equal(t, tt.expectedEffectiveSize, size)
		})
	}
}

// TestAllowlistResolutionBuilder_ValidConfiguration tests that Build() works with valid configurations
func TestAllowlistResolutionBuilder_ValidConfiguration(t *testing.T) {
	t.Run("does not panic with group slice and global set", func(t *testing.T) {
		resolution := NewAllowlistResolutionBuilder().
			WithGroupVariables([]string{"VAR1"}).
			WithGlobalVariablesSet(map[string]struct{}{"VAR2": {}}).
			Build()

		assert.NotNil(t, resolution, "Build() should not return nil with valid configuration")
	})

	t.Run("does not panic with only group variables", func(t *testing.T) {
		resolution := NewAllowlistResolutionBuilder().
			WithGroupVariables([]string{"VAR1"}).
			Build()

		assert.NotNil(t, resolution, "Build() should not return nil with valid configuration")
	})

	t.Run("does not panic with only global variables", func(t *testing.T) {
		resolution := NewAllowlistResolutionBuilder().
			WithGlobalVariablesSet(map[string]struct{}{"VAR2": {}}).
			Build()

		assert.NotNil(t, resolution, "Build() should not return nil with valid configuration")
	})
}
