//go:build test

package runnertypes

// NewTestAllowlistResolutionSimple creates a simple AllowlistResolution for basic testing.
// Uses InheritanceModeInherit by default with "test-group" as the group name.
//
// Parameters:
//   - globalVars: global environment variables for the allowlist
//   - groupVars: group-specific environment variables for the allowlist
//
// Returns: *AllowlistResolution configured with Inherit mode and "test-group" name
func NewTestAllowlistResolutionSimple(
	globalVars []string,
	groupVars []string,
) *AllowlistResolution {
	return NewAllowlistResolutionBuilder().
		WithMode(InheritanceModeInherit).
		WithGroupName("test-group").
		WithGlobalVariables(globalVars).
		WithGroupVariables(groupVars).
		Build()
}

// NewTestAllowlistResolutionWithMode creates AllowlistResolution with specific inheritance mode.
// Supports all current inheritance modes: Inherit, Explicit, Reject.
// Uses "test-group" as the group name.
//
// Parameters:
//   - mode: the inheritance mode to use
//   - globalVars: global environment variables for the allowlist
//   - groupVars: group-specific environment variables for the allowlist
//
// Returns: *AllowlistResolution configured with the specified mode and "test-group" name
func NewTestAllowlistResolutionWithMode(
	mode InheritanceMode,
	globalVars []string,
	groupVars []string,
) *AllowlistResolution {
	return NewAllowlistResolutionBuilder().
		WithMode(mode).
		WithGroupName("test-group").
		WithGlobalVariables(globalVars).
		WithGroupVariables(groupVars).
		Build()
}
