package config

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// DetermineEffectiveAllowlist determines the effective environment variable allowlist for a group.
//
// Allowlist inheritance rules:
// - If group.EnvAllowlist == nil: inherit global.EnvAllowlist
// - If group.EnvAllowlist != nil: use group.EnvAllowlist (override global)
// - If group.EnvAllowlist == []: reject all system environment variables
//
// Parameters:
//   - group: The command group to determine allowlist for
//   - global: The global configuration containing the global allowlist
//
// Returns:
//   - The effective allowlist for the group (may be nil, empty slice, or populated slice)
func DetermineEffectiveAllowlist(group *runnertypes.CommandGroup, global *runnertypes.GlobalConfig) []string {
	if group.EnvAllowlist == nil {
		// Inherit from global allowlist
		return global.EnvAllowlist
	}
	// Use group's own allowlist (may be empty to reject all)
	return group.EnvAllowlist
}
