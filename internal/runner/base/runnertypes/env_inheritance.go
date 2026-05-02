package runnertypes

// DetermineEnvAllowlistInheritanceMode determines the inheritance mode
// from the GroupSpec's env_allowlist configuration.
//
// This function serves as the single source of truth for inheritance mode
// determination. All locations that need to determine inheritance mode
// must use this function.
//
// Determination rules:
//
//   - envAllowed == nil        → InheritanceModeInherit
//     When the env_allowlist field is undefined in the TOML file.
//     The group inherits the global env_allowlist configuration.
//
//   - len(envAllowed) == 0     → InheritanceModeReject
//     When env_allowlist = [] is explicitly set in the TOML file.
//     The group rejects all environment variables.
//
//   - len(envAllowed) > 0      → InheritanceModeExplicit
//     When env_allowlist = ["VAR1", "VAR2"] is explicitly set in the TOML file.
//     The group uses its own explicit allowlist.
//
// Go language specification:
//
//	var nilSlice []string     // nil
//	emptySlice := []string{}  // non-nil, length 0
//
//	nilSlice == nil           // true
//	emptySlice == nil         // false
//	len(emptySlice) == 0      // true
//
// Parameters:
//
//	envAllowed: The value of GroupSpec.EnvAllowed
//	            nil means the field is undefined in TOML
//	            non-nil with length 0 means an explicit empty array in TOML
//	            non-nil with length > 0 means an explicit variable list
//
// Returns:
//
//	InheritanceMode: The determined inheritance mode
//	                 - InheritanceModeInherit:  Inherits global allowlist
//	                 - InheritanceModeExplicit: Uses group-specific allowlist
//	                 - InheritanceModeReject:   Rejects all environment variables
//
// Usage example:
//
//	// Usage in Validator
//	mode := runnertypes.DetermineEnvAllowlistInheritanceMode(group.EnvAllowed)
//	switch mode {
//	case runnertypes.InheritanceModeInherit:
//	    // Validate global allowlist
//	case runnertypes.InheritanceModeReject:
//	    // Validate reject mode
//	case runnertypes.InheritanceModeExplicit:
//	    // Validate explicit mode
//	}
//
//	// Usage in Expander
//	runtimeGroup.EnvAllowlistInheritanceMode =
//	    runnertypes.DetermineEnvAllowlistInheritanceMode(group.EnvAllowed)
func DetermineEnvAllowlistInheritanceMode(envAllowed []string) InheritanceMode {
	// Rule 1: nil check
	// When the field is undefined in TOML, the Go slice is nil
	if envAllowed == nil {
		return InheritanceModeInherit
	}

	// Rule 2: empty array check
	// When env_allowlist = [] is explicitly set in TOML,
	// the slice is non-nil with length 0
	if len(envAllowed) == 0 {
		return InheritanceModeReject
	}

	// Rule 3: explicit variable list
	// When len(envAllowed) > 0
	return InheritanceModeExplicit
}
