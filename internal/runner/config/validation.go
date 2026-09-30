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
// length, and duplicates within their group. Group names also have a length
// limit. The length limit counts
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

		seenCommands := make(map[string]int, len(group.Commands))
		for j, cmd := range group.Commands {
			if err := validateCommandName(cmd.Name, i, j); err != nil {
				return err
			}
			// A notification scope is "group/command", so a name repeated
			// within one group cannot say which command it points at.
			if prevIndex, exists := seenCommands[cmd.Name]; exists {
				return fmt.Errorf("%w at groups[%d].commands[%d] and groups[%d].commands[%d]",
					ErrDuplicateCommandName, i, prevIndex, i, j)
			}
			seenCommands[cmd.Name] = j
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
//   - level: The configuration level (e.g., globalLevel(), groupLevel("mygroup"))
//   - field: The field where the variable appears (e.g., envField(), varsField())
//
// Returns:
//   - nil if valid
//   - *ErrReservedVariablePrefixDetail if the name uses a reserved prefix
//   - *ErrInvalidVariableNameDetail for POSIX validation errors
func validateVariableName(varName string, level Level, field Field) error {
	// First, check POSIX compliance using the existing security package function
	if err := security.ValidateVariableName(varName); err != nil {
		// POSIX validation error from security.ValidateVariableName
		return &ErrInvalidVariableNameDetail{
			Level:        level.String(),
			Field:        field.String(),
			VariableName: varName,
			Reason:       err.Error(),
		}
	}

	// Then, check for reserved prefix (additional check specific to internal variables)
	if strings.HasPrefix(varName, reservedVariablePrefix) {
		return &ErrReservedVariablePrefixDetail{
			Level:        level.String(),
			Field:        field.String(),
			VariableName: varName,
			Prefix:       reservedVariablePrefix,
		}
	}

	// Check variable scope based on level
	// Global variables must start with uppercase, local variables with lowercase
	expectedScope := variable.ScopeLocal
	if level.kind == levelGlobal {
		expectedScope = variable.ScopeGlobal
	}

	location := fmt.Sprintf("%s.%s", level, field)
	if err := variable.ValidateVariableNameForScope(varName, expectedScope, location); err != nil {
		return &ErrInvalidVariableScopeDetail{
			Level:        level.String(),
			Field:        field.String(),
			VariableName: varName,
			Err:          err,
		}
	}

	return nil
}

// negativeSettingMessages collects one message per negative value of a numeric
// setting found in the configuration, naming the setting and the location. The
// global and template scopes name the setting; the command scope names the
// command, its group and its position, and relies on the wrapping sentinel
// ("<setting> must not be negative") to say which setting it refers to — as it
// always has for timeouts. setting is the configuration key ("timeout",
// "output_size_limit"), and the accessors return each scope's raw pointer so
// one walk serves settings of different integer widths.
func negativeSettingMessages[T ~int32 | ~int64](
	cfg *runnertypes.ConfigSpec,
	setting string,
	global *T,
	templateValue func(runnertypes.CommandTemplate) *T,
	commandValue func(runnertypes.CommandSpec) *T,
) []string {
	var msgs []string

	if global != nil && *global < 0 {
		msgs = append(msgs, fmt.Sprintf("global %s got %d", setting, *global))
	}

	for templateName, template := range cfg.CommandTemplates {
		if v := templateValue(template); v != nil && *v < 0 {
			msgs = append(msgs, fmt.Sprintf("template '%s' %s got %d", templateName, setting, *v))
		}
	}

	for groupIdx, group := range cfg.Groups {
		for cmdIdx, cmd := range group.Commands {
			if v := commandValue(cmd); v != nil && *v < 0 {
				msgs = append(msgs, fmt.Sprintf("command '%s' in group '%s' (groups[%d].commands[%d]) got %d",
					cmd.Name, group.Name, groupIdx, cmdIdx, *v))
			}
		}
	}

	return msgs
}

// ValidateTimeouts validates that all timeout values in the configuration are non-negative.
// It checks global timeout, template timeouts, and command-level timeouts.
// Returns an aggregated error containing all negative timeout violations found.
func ValidateTimeouts(cfg *runnertypes.ConfigSpec) error {
	msgs := negativeSettingMessages(cfg, "timeout", cfg.Global.Timeout,
		func(template runnertypes.CommandTemplate) *int32 { return template.Timeout },
		func(cmd runnertypes.CommandSpec) *int32 { return cmd.Timeout })

	if len(msgs) > 0 {
		return fmt.Errorf("%w: %s", ErrNegativeTimeout, strings.Join(msgs, "; "))
	}

	return nil
}

// ValidateOutputSizeLimits validates that all output_size_limit values in the
// configuration are non-negative. It checks the global limit, template limits,
// and command-level limits. Zero and positive values are accepted; zero means
// unlimited. Returns an aggregated error containing all negative violations
// found, each naming its value and location.
func ValidateOutputSizeLimits(cfg *runnertypes.ConfigSpec) error {
	msgs := negativeSettingMessages(cfg, "output_size_limit", cfg.Global.OutputSizeLimit,
		func(template runnertypes.CommandTemplate) *int64 { return template.OutputSizeLimit },
		func(cmd runnertypes.CommandSpec) *int64 { return cmd.OutputSizeLimit })

	if len(msgs) > 0 {
		return fmt.Errorf("%w: %s", ErrNegativeOutputSizeLimit, strings.Join(msgs, "; "))
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
