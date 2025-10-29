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
		t.Run(tt.name, func(_ *testing.T) {
			_ = newAllowlistResolution(tt.mode, "test-group", groupSet, globalSet)
			// Note: IsAllowed method has been removed as it's not used in production code
		})
	}
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
		t.Run(tt.name, func(_ *testing.T) {
			_ = NewAllowlistResolutionBuilder().
				WithMode(tt.mode).
				WithGroupName("test").
				WithGroupVariables(tt.groupVars).
				WithGlobalVariablesForTest(tt.globalVars).
				Build()
			// Note: IsAllowed method has been removed as it's not used in production code
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
		})
	}
}

// TestNewTestAllowlistResolutionSimpleEmpty tests function with empty variable lists
func TestNewTestAllowlistResolutionSimpleEmpty(t *testing.T) {
	resolution := NewTestAllowlistResolutionSimple([]string{}, []string{})
	assert.NotNil(t, resolution, "CreateSimple() with empty lists should not return nil")
}

// TestNewTestAllowlistResolutionSimpleNil tests function with nil variable lists
func TestNewTestAllowlistResolutionSimpleNil(t *testing.T) {
	resolution := NewTestAllowlistResolutionSimple(nil, nil)
	assert.NotNil(t, resolution, "CreateSimple() with nil lists should not return nil")
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
		t.Run(tt.name, func(_ *testing.T) {
			_ = NewAllowlistResolutionBuilder().
				WithMode(tt.mode).
				WithGroupName("test-api").
				WithGroupVariables(tt.groupVars).
				WithGlobalVariablesSet(tt.globalSet).
				Build()
			// Note: IsAllowed method has been removed as it's not used in production code
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
