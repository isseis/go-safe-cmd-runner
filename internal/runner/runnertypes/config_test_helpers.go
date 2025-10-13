//go:build test

package runnertypes

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// WithGlobalVariablesForTest is a test-only convenience method that accepts a slice
// and converts it to a set internally. This simplifies test code by avoiding the
// verbose map[string]struct{}{} syntax.
//
// This method is only available in test builds (via build tag).
//
// Example:
//
//	builder.WithGlobalVariablesForTest([]string{"VAR1", "VAR2"})
//
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithGlobalVariablesForTest(vars []string) *AllowlistResolutionBuilder {
	return b.WithGlobalVariablesSet(common.SliceToSet(vars))
}

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
		WithGlobalVariablesForTest(globalVars).
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
		WithGlobalVariablesForTest(globalVars).
		WithGroupVariables(groupVars).
		Build()
}
