package config

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/variable"
)

// reservedVariablePrefix is the prefix reserved for internal variables
const reservedVariablePrefix = "__runner_"

// GroupNamePattern defines the naming rule for groups.
// Allowed characters follow the environment variable convention: [A-Za-z_][A-Za-z0-9_]*.
var GroupNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateGroupName validates a single group name against the naming convention.
// Returns a detailed error if the name is invalid. The name itself is not
// echoed: a rejected identifier can be a credential shape, and these errors
// reach stderr without redaction.
func validateGroupName(name string) error {
	if !GroupNamePattern.MatchString(name) {
		return fmt.Errorf("%w: must match pattern [A-Za-z_][A-Za-z0-9_]*", ErrInvalidGroupName)
	}
	return nil
}

// ValidateIdentifiers validates the group and command names in the configuration.
// Group names are checked for emptiness, pattern and duplicates; command names
// for emptiness, control and format-control characters, displayable content,
// and length. Group names also have a length limit. The length limit counts
// bytes, matching the display-safe interpolation contract that never truncates
// identifiers.
//
// This function is called during configuration loading to ensure early validation.
func ValidateIdentifiers(cfg *runnertypes.ConfigSpec) error {
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

		if err := validateIdentifierLength(group.Name, fmt.Sprintf("groups[%d]", i)); err != nil {
			return err
		}

		// Check for duplicate group names
		if prevIndex, exists := seen[group.Name]; exists {
			return fmt.Errorf("%w at indices %d and %d", ErrDuplicateGroupName, prevIndex, i)
		}
		seen[group.Name] = i

		for j, cmd := range group.Commands {
			if err := validateCommandName(cmd.Name, i, j); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateCommandName rejects a command name that cannot identify the command
// in a notification. The control-character check and the displayable-content
// check are independent: a name can contain control characters and still leave
// displayable characters ("backup\nother"), and a name without any control
// character can still render as blank ("   "). The message names the position
// and the check but never the value: a rejected identifier can itself be a
// credential, and these errors reach stderr without redaction.
func validateCommandName(name string, groupIdx, cmdIdx int) error {
	position := fmt.Sprintf("groups[%d].commands[%d]", groupIdx, cmdIdx)

	if name == "" {
		return fmt.Errorf("%w at %s", ErrEmptyCommandName, position)
	}
	if strings.ContainsFunc(name, isControlOrFormatCharacter) {
		return fmt.Errorf("%w at %s", ErrIdentifierContainsControlCharacter, position)
	}
	if !common.HasDisplayableContent(name) {
		return fmt.Errorf("%w at %s", ErrIdentifierNotDisplayable, position)
	}
	return validateIdentifierLength(name, position)
}

// validateIdentifierLength rejects a group or command name longer than
// common.MaxIdentifierBytes. It reports the measured and allowed lengths rather
// than echoing a value that may be arbitrarily large.
func validateIdentifierLength(name, position string) error {
	if len(name) <= common.MaxIdentifierBytes {
		return nil
	}
	return fmt.Errorf("%w at %s: %d bytes, max %d",
		ErrIdentifierTooLong, position, len(name), common.MaxIdentifierBytes)
}

// isControlOrFormatCharacter reports whether r is a control character (Unicode
// general category Cc) or a format-control character (category Cf). Format
// controls include the bidirectional overrides that can make a name read as a
// different name than it is.
func isControlOrFormatCharacter(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// validateVariableName validates a variable name and returns a detailed error
// if validation fails. This helper function standardizes error handling across
// ProcessEnv, ProcessEnvImport, and ProcessVars.
//
// The function performs two checks:
// 1. POSIX compliance using security.ValidateVariableName (empty name, pattern matching)
// 2. Reserved prefix check (names starting with "__runner_" are rejected)
//
// Parameters:
//   - varName: The variable name to validate
//   - level: The configuration level (e.g., "global", "group:mygroup", "cmd:mycmd")
//   - field: The field name where the variable appears (e.g., "env", "env_import", "vars")
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

	// Check variable scope based on level
	// Global variables must start with uppercase, local variables with lowercase
	expectedScope := variable.ScopeLocal
	if level == "global" {
		expectedScope = variable.ScopeGlobal
	}

	location := fmt.Sprintf("%s.%s", level, field)
	if err := variable.ValidateVariableNameForScope(varName, expectedScope, location); err != nil {
		return &ErrInvalidVariableScopeDetail{
			Level:        level,
			Field:        field,
			VariableName: varName,
			Err:          err,
		}
	}

	return nil
}

// ValidateTimeouts validates that all timeout values in the configuration are non-negative.
// It checks global timeout, template timeouts, and command-level timeouts.
// Returns an aggregated error containing all negative timeout violations found.
func ValidateTimeouts(cfg *runnertypes.ConfigSpec) error {
	var msgs []string

	// Check global timeout
	if cfg.Global.Timeout != nil && *cfg.Global.Timeout < 0 {
		msgs = append(msgs, fmt.Sprintf("global timeout got %d", *cfg.Global.Timeout))
	}

	// Check template timeouts
	for templateName, template := range cfg.CommandTemplates {
		if template.Timeout != nil && *template.Timeout < 0 {
			msgs = append(msgs, fmt.Sprintf("template '%s' timeout got %d",
				templateName, *template.Timeout))
		}
	}

	// Check command-level timeouts
	for groupIdx, group := range cfg.Groups {
		for cmdIdx, cmd := range group.Commands {
			if cmd.Timeout != nil && *cmd.Timeout < 0 {
				msgs = append(msgs, fmt.Sprintf("command '%s' in group '%s' (groups[%d].commands[%d]) got %d",
					cmd.Name, group.Name, groupIdx, cmdIdx, *cmd.Timeout))
			}
		}
	}

	if len(msgs) > 0 {
		return fmt.Errorf("%w: %s", ErrNegativeTimeout, strings.Join(msgs, "; "))
	}

	return nil
}

// ValidateCommands validates all commands in the configuration.
// It checks for:
// - Command spec exclusivity (template vs. cmd/args/env fields)
// - Missing required fields
//
// This function is called during configuration loading to ensure early validation.
func ValidateCommands(cfg *runnertypes.ConfigSpec) error {
	if cfg == nil {
		return ErrNilConfig
	}

	for groupIdx, group := range cfg.Groups {
		for cmdIdx, cmd := range group.Commands {
			// Validate command spec exclusivity
			if err := validateCmdSpec(group.Name, cmdIdx, &cmd); err != nil {
				return fmt.Errorf("group[%d] (%s): %w", groupIdx, group.Name, err)
			}
		}
	}

	return nil
}

// Note: Working directory validation (absolute path check) is performed at expansion time
// in group_executor.go (resolveGroupWorkDir and resolveCommandWorkDir).
// This ensures consistent validation regardless of whether the path contains variables.
