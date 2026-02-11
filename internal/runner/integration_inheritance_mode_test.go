//go:build test

package runner_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/debuginfo"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInheritanceModeTracking_Inherit verifies that inheritance mode is correctly
// tracked through the entire pipeline when a group inherits global env_allowlist.
func TestInheritanceModeTracking_Inherit(t *testing.T) {
	t.Parallel()

	// Arrange: Create configuration with global allowlist, group without allowlist
	configSpec := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			EnvAllowed: []string{"VAR1", "VAR2"},
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:        "test-group",
				Description: "Test group for Inherit mode",
				// EnvAllowed is nil - should inherit from global
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "echo test",
					},
				},
			},
		},
	}

	// Act: Process through Expander pipeline
	groupSpec := &configSpec.Groups[0]

	// Create RuntimeGlobal
	runtimeGlobal, err := config.ExpandGlobal(&configSpec.Global)
	require.NoError(t, err, "Global expansion should succeed")

	// Expand group (this sets EnvAllowlistInheritanceMode)
	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
	require.NoError(t, err, "Group expansion should succeed")

	// Assert: Verify inheritance mode is set correctly
	assert.Equal(t, runnertypes.InheritanceModeInherit, runtimeGroup.EnvAllowlistInheritanceMode,
		"Inheritance mode should be Inherit when group has nil allowlist")

	// Verify debug output using FormatInheritanceAnalysisText
	analysis := debuginfo.CollectInheritanceAnalysis(runtimeGlobal, runtimeGroup, resource.DetailLevelDetailed)
	output := debuginfo.FormatInheritanceAnalysisText(analysis, runtimeGroup.Spec.Name)

	assert.Contains(t, output, "Inheriting Global env_allowlist",
		"Debug output should indicate inheritance")
	assert.Contains(t, output, "Allowlist (2): VAR1, VAR2",
		"Debug output should show global allowlist")
}

// TestInheritanceModeTracking_Explicit verifies that inheritance mode is correctly
// tracked when a group explicitly defines its own env_allowlist.
func TestInheritanceModeTracking_Explicit(t *testing.T) {
	t.Parallel()

	// Arrange: Create configuration with different global and group allowlists
	configSpec := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			EnvAllowed: []string{"VAR1", "VAR2", "VAR3"},
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:        "test-group",
				Description: "Test group for Explicit mode",
				EnvAllowed:  []string{"VAR1"}, // Explicit allowlist
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "echo test",
					},
				},
			},
		},
	}

	// Act: Process through Expander pipeline
	groupSpec := &configSpec.Groups[0]

	// Create RuntimeGlobal
	runtimeGlobal, err := config.ExpandGlobal(&configSpec.Global)
	require.NoError(t, err, "Global expansion should succeed")

	// Expand group (this sets EnvAllowlistInheritanceMode)
	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
	require.NoError(t, err, "Group expansion should succeed")

	// Assert: Verify inheritance mode is set correctly
	assert.Equal(t, runnertypes.InheritanceModeExplicit, runtimeGroup.EnvAllowlistInheritanceMode,
		"Inheritance mode should be Explicit when group has non-empty allowlist")

	// Verify debug output using FormatInheritanceAnalysisText with DetailLevelFull to get RemovedAllowlistVariables
	analysis := debuginfo.CollectInheritanceAnalysis(runtimeGlobal, runtimeGroup, resource.DetailLevelFull)
	output := debuginfo.FormatInheritanceAnalysisText(analysis, runtimeGroup.Spec.Name)

	assert.Contains(t, output, "Using group-specific env_allowlist",
		"Debug output should indicate explicit allowlist")
	assert.Contains(t, output, "Group allowlist (1): VAR1",
		"Debug output should show group allowlist")
	assert.Contains(t, output, "Removed from Global allowlist: VAR2, VAR3",
		"Debug output should show removed variables")
}

// TestInheritanceModeTracking_Reject verifies that inheritance mode is correctly
// tracked when a group explicitly rejects all environment variables.
func TestInheritanceModeTracking_Reject(t *testing.T) {
	t.Parallel()

	// Arrange: Create configuration with global allowlist, group with empty allowlist
	configSpec := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			EnvAllowed: []string{"VAR1", "VAR2"},
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:        "test-group",
				Description: "Test group for Reject mode",
				EnvAllowed:  []string{}, // Empty allowlist - explicit rejection
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "echo test",
					},
				},
			},
		},
	}

	// Act: Process through Expander pipeline
	groupSpec := &configSpec.Groups[0]

	// Create RuntimeGlobal
	runtimeGlobal, err := config.ExpandGlobal(&configSpec.Global)
	require.NoError(t, err, "Global expansion should succeed")

	// Expand group (this sets EnvAllowlistInheritanceMode)
	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
	require.NoError(t, err, "Group expansion should succeed")

	// Assert: Verify inheritance mode is set correctly
	assert.Equal(t, runnertypes.InheritanceModeReject, runtimeGroup.EnvAllowlistInheritanceMode,
		"Inheritance mode should be Reject when group has empty allowlist")

	// Verify debug output using FormatInheritanceAnalysisText
	analysis := debuginfo.CollectInheritanceAnalysis(runtimeGlobal, runtimeGroup, resource.DetailLevelDetailed)
	output := debuginfo.FormatInheritanceAnalysisText(analysis, runtimeGroup.Spec.Name)

	assert.Contains(t, output, "Rejecting all environment variables",
		"Debug output should indicate rejection of all variables")
	assert.Contains(t, output, "(Group has empty env_allowlist defined, blocking all env inheritance)",
		"Debug output should explain rejection reason")
}

// TestInheritanceModeTracking_CompleteFlow verifies inheritance mode tracking
// through a complete configuration with multiple groups demonstrating all modes.
func TestInheritanceModeTracking_CompleteFlow(t *testing.T) {
	t.Parallel()

	// Arrange: Create configuration with all three inheritance modes
	configSpec := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			EnvAllowed: []string{"GLOBAL_VAR1", "GLOBAL_VAR2", "GLOBAL_VAR3"},
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:        "inherit-group",
				Description: "Group that inherits global allowlist",
				// EnvAllowed is nil - Inherit mode
				Commands: []runnertypes.CommandSpec{
					{Name: "cmd1", Cmd: "echo inherit"},
				},
			},
			{
				Name:        "explicit-group",
				Description: "Group with explicit allowlist",
				EnvAllowed:  []string{"EXPLICIT_VAR"},
				Commands: []runnertypes.CommandSpec{
					{Name: "cmd2", Cmd: "echo explicit"},
				},
			},
			{
				Name:        "reject-group",
				Description: "Group that rejects all variables",
				EnvAllowed:  []string{}, // Empty - Reject mode
				Commands: []runnertypes.CommandSpec{
					{Name: "cmd3", Cmd: "echo reject"},
				},
			},
		},
	}

	// Act & Assert: Process each group through the pipeline
	runtimeGlobal, err := config.ExpandGlobal(&configSpec.Global)
	require.NoError(t, err)

	testCases := []struct {
		groupIndex   int
		groupName    string
		expectedMode runnertypes.InheritanceMode
	}{
		{0, "inherit-group", runnertypes.InheritanceModeInherit},
		{1, "explicit-group", runnertypes.InheritanceModeExplicit},
		{2, "reject-group", runnertypes.InheritanceModeReject},
	}

	for _, tc := range testCases {
		tc := tc // Capture loop variable
		t.Run(tc.groupName, func(t *testing.T) {
			groupSpec := &configSpec.Groups[tc.groupIndex]

			// Expand group
			runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
			require.NoError(t, err, "Group expansion should succeed")

			// Verify inheritance mode
			assert.Equal(t, tc.expectedMode, runtimeGroup.EnvAllowlistInheritanceMode,
				"Inheritance mode should be %v for %s", tc.expectedMode, tc.groupName)

			// Verify debug output contains expected text using FormatInheritanceAnalysisText
			analysis := debuginfo.CollectInheritanceAnalysis(runtimeGlobal, runtimeGroup, resource.DetailLevelDetailed)
			output := debuginfo.FormatInheritanceAnalysisText(analysis, runtimeGroup.Spec.Name)

			switch tc.expectedMode {
			case runnertypes.InheritanceModeInherit:
				assert.Contains(t, output, "Inheriting Global env_allowlist")
			case runnertypes.InheritanceModeExplicit:
				assert.Contains(t, output, "Using group-specific env_allowlist")
			case runnertypes.InheritanceModeReject:
				assert.Contains(t, output, "Rejecting all environment variables")
			}
		})
	}
}
