package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// reservedVariablePrefix is the prefix reserved for internal variables
const reservedVariablePrefix = "__runner_"

// GroupNamePattern defines the naming rule for groups.
// Allowed characters follow the environment variable convention: [A-Za-z_][A-Za-z0-9_]*.
var GroupNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateGroupName validates a single group name against the naming convention.
// Returns a detailed error if the name is invalid.
func validateGroupName(name string) error {
	if !GroupNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q must match pattern [A-Za-z_][A-Za-z0-9_]*", ErrInvalidGroupName, name)
	}
	return nil
}

// ValidateGroupNames validates all group names in the configuration.
// It checks for:
// 1. Empty group names
// 2. Invalid characters in group names (must match [A-Za-z_][A-Za-z0-9_]*)
// 3. Duplicate group names
//
// This function is called during configuration loading to ensure early validation.
func ValidateGroupNames(cfg *runnertypes.ConfigSpec) error {
	if cfg == nil {
		return ErrNilConfig
	}

	seen := make(map[string]int, len(cfg.Groups))

	for i, group := range cfg.Groups {
		// Check for empty group name
		if group.Name == "" {
			return fmt.Errorf("%w at index %d", ErrEmptyGroupName, i)
		}

		// Validate group name pattern
		if err := validateGroupName(group.Name); err != nil {
			return fmt.Errorf("group at index %d: %w", i, err)
		}

		// Check for duplicate group names
		if prevIndex, exists := seen[group.Name]; exists {
			return fmt.Errorf("%w: %q at indices %d and %d", ErrDuplicateGroupName, group.Name, prevIndex, i)
		}
		seen[group.Name] = i
	}

	return nil
}

// validateVariableName validates a variable name and returns a detailed error
// if validation fails. This helper function standardizes error handling across
// ProcessEnv, ProcessFromEnv, and ProcessVars.
//
// The function performs two checks:
// 1. POSIX compliance using security.ValidateVariableName (empty name, pattern matching)
// 2. Reserved prefix check (names starting with "__runner_" are rejected)
//
// Parameters:
//   - varName: The variable name to validate
//   - level: The configuration level (e.g., "global", "group:mygroup", "cmd:mycmd")
//   - field: The field name where the variable appears (e.g., "env", "from_env", "vars")
//
// Returns:
//   - nil if valid
//   - *ErrReservedVariablePrefixDetail if the name uses a reserved prefix
//   - *ErrInvalidVariableNameDetail for POSIX validation errors
func validateVariableName(varName, level, field string) error {
	// First, check POSIX compliance using the existing security package function
	if err := security.ValidateVariableName(varName); err != nil {
		// POSIX validation error from security.ValidateVariableName
		return &ErrInvalidVariableNameDetail{
			Level:        level,
			Field:        field,
			VariableName: varName,
			Reason:       err.Error(),
		}
	}

	// Then, check for reserved prefix (additional check specific to internal variables)
	if strings.HasPrefix(varName, reservedVariablePrefix) {
		return &ErrReservedVariablePrefixDetail{
			Level:        level,
			Field:        field,
			VariableName: varName,
			Prefix:       reservedVariablePrefix,
		}
	}

	return nil
}

// ValidateTimeouts validates that all timeout values in the configuration are non-negative.
// It checks both global timeout and command-level timeouts.
// Returns an aggregated error containing all negative timeout violations found.
func ValidateTimeouts(cfg *runnertypes.ConfigSpec) error {
	var errors []string

	// Check global timeout
	if cfg.Global.Timeout != nil && *cfg.Global.Timeout < 0 {
		errors = append(errors, fmt.Sprintf("global timeout got %d", *cfg.Global.Timeout))
	}

	// Check command-level timeouts
	for groupIdx, group := range cfg.Groups {
		for cmdIdx, cmd := range group.Commands {
			if cmd.Timeout != nil && *cmd.Timeout < 0 {
				errors = append(errors, fmt.Sprintf("command '%s' in group '%s' (groups[%d].commands[%d]) got %d",
					cmd.Name, group.Name, groupIdx, cmdIdx, *cmd.Timeout))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%w: %s", ErrNegativeTimeout, strings.Join(errors, "; "))
	}

	return nil
}
