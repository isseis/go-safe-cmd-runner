// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

const (
	// maxVarNamePrefixLength is the maximum length of variable name prefix shown in error messages
	// for unclosed variable references. Longer names are truncated with "..." suffix.
	maxVarNamePrefixLength = 20
)

var (
	// ErrCircularReference is returned when a circular variable reference is detected.
	ErrCircularReference = errors.New("circular variable reference detected")
	// ErrInvalidEscapeSequence is returned when an invalid escape sequence is detected.
	ErrInvalidEscapeSequence = errors.New("invalid escape sequence (only \\$ and \\\\ are allowed)")
	// ErrUnclosedVariable is returned when a variable expansion is not properly closed.
	ErrUnclosedVariable = errors.New("unclosed variable reference (missing closing '}')")
	// ErrInvalidVariableFormat is returned when $ is found but not followed by valid variable syntax.
	ErrInvalidVariableFormat = errors.New("invalid variable format (use ${VAR} syntax)")
)

// VariableExpander handles variable expansion for command strings and environment maps.
// It provides core functionality for expanding ${VAR} syntax in both command-line strings
// (cmd/args) and environment variable values.
type VariableExpander struct {
	filter    *Filter
	logger    *slog.Logger
	validator *security.Validator // Cached security validator to avoid repeated creation
}

// NewVariableExpander creates a new VariableExpander.
func NewVariableExpander(filter *Filter) *VariableExpander {
	// Create logger first to ensure consistent context for all logs
	logger := slog.Default().With("component", "VariableExpander")

	// Create a cached security validator with default config
	// This avoids creating a new validator (and compiling regexes) on every validation call
	validator, err := security.NewValidator(nil)
	if err != nil {
		// Fall back to nil validator - validation will be skipped
		// This should not happen with default config, but handle gracefully
		logger.Warn("Failed to create security validator, validation will be limited", "error", err)
	}

	return &VariableExpander{
		filter:    filter,
		logger:    logger,
		validator: validator,
	}
}

// ExpandCommandEnv expands Command.Env variables with priority environment variables.
// This is used during configuration loading (Phase 1) to pre-expand Command.Env.
// Returns a map of expanded environment variables ready to merge with system environment.
//
// The baseEnv parameter provides high-priority variables that take precedence over Command.Env:
//   - In production: Contains automatic variables (__RUNNER_DATETIME, __RUNNER_PID) that
//     Command.Env CANNOT override
//   - In testing: Usually nil or empty map for simple test scenarios
//   - Variables from Command.Env that conflict with baseEnv are silently ignored with a
//     warning log to prevent accidental override of automatic variables
//
// It uses a two-pass approach:
//  1. First pass: Add all variables from baseEnv, then add non-conflicting variables from
//     the command's `Env` block. This allows Command.Env to reference baseEnv variables
//     (e.g., OUTPUT_FILE=output-${__RUNNER_DATETIME}.txt) while preventing override.
//  2. Second pass: Iterate over the map and expand any variables in the values.
func (p *VariableExpander) ExpandCommandEnv(cmd *runnertypes.Command, groupName string, groupEnvAllowList []string, baseEnv map[string]string) (map[string]string, error) {
	p.logger.Debug("Starting command environment expansion",
		"command", cmd.Name,
		"group", groupName,
		"env_count", len(cmd.Env),
		"base_env_count", len(baseEnv))

	// Create expansion environment by merging baseEnv with command env
	// This allows command env to reference baseEnv variables during expansion
	expansionEnv := make(map[string]string, len(baseEnv)+len(cmd.Env))
	maps.Copy(expansionEnv, baseEnv)

	// Track which variables are defined in cmd.Env (not from baseEnv)
	cmdEnvVars := make(map[string]bool, len(cmd.Env))

	// First pass: Populate the environment with unexpanded values from the command.
	// Command env variables CANNOT override baseEnv variables (e.g., automatic variables).
	for i, envStr := range cmd.Env {
		varName, varValue, ok := common.ParseEnvVariable(envStr)
		if !ok {
			return nil, fmt.Errorf("invalid environment variable format in Command.Env in command %s, env_index: %d, env_entry: %s: %w", cmd.Name, i, envStr, ErrMalformedEnvVariable)
		}
		// Validate only the name at this stage.
		if err := p.validateBasicEnvVariable(varName, ""); err != nil {
			return nil, fmt.Errorf("malformed command environment variable %s in command %s: %w",
				varName, cmd.Name, err)
		}

		// Skip variables that are already in baseEnv (e.g., automatic variables)
		// This ensures baseEnv variables cannot be overridden by cmd.Env
		_, existsInBase := baseEnv[varName]
		if !existsInBase {
			expansionEnv[varName] = varValue
			cmdEnvVars[varName] = true
			p.logger.Debug("Added command environment variable",
				"command", cmd.Name,
				"variable", varName,
				"value_length", len(varValue))
		} else {
			// Warn when cmd.Env variable is ignored due to baseEnv conflict
			p.logger.Warn("Command environment variable ignored due to conflict with base environment",
				"command", cmd.Name,
				"variable", varName,
				"cmd_value", varValue,
				"base_value", baseEnv[varName])
		}
	}

	// Second pass: Expand all variables (both baseEnv and cmd.Env).
	for name := range expansionEnv {
		value := expansionEnv[name]
		// Create a new visited map for each variable expansion to prevent false circular reference detection.
		// The key insight: when expanding "PATH=/custom/bin:${PATH}", we want ${PATH} to resolve to
		// the system PATH, not the partially-expanded value in expansionEnv["PATH"].
		// By marking the current variable as visited, ExpandString will skip it in expansionEnv and
		// fall back to system environment lookup.
		visited := map[string]bool{name: true}
		expandedValue, err := p.ExpandString(value, expansionEnv, groupEnvAllowList, groupName, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to expand variable %s in command %s: %w", name, cmd.Name, err)
		}
		expansionEnv[name] = expandedValue
	}

	// Build the result map containing only cmd.Env variables (not baseEnv)
	// baseEnv variables will be merged later in the caller
	result := make(map[string]string, len(cmdEnvVars))
	for name := range cmdEnvVars {
		result[name] = expansionEnv[name]
	}

	// Final validation pass on the cmd.Env variables only.
	for name, value := range result {
		if err := p.validateBasicEnvVariable(name, value); err != nil {
			return nil, fmt.Errorf("validation failed for expanded variable %s in command %s: %w", name, cmd.Name, err)
		}
	}

	p.logger.Debug("Command environment expansion completed",
		"command", cmd.Name,
		"group", groupName,
		"variables_expanded", len(result))

	return result, nil
}

// validateBasicEnvVariable validates the name and optionally the value of an environment variable.
// This method uses the cached security validator to avoid repeated validator creation.
func (p *VariableExpander) validateBasicEnvVariable(varName, varValue string) error {
	// Validate name using security package which returns detailed errors.
	if err := security.ValidateVariableName(varName); err != nil {
		if varName == "" {
			return ErrVariableNameEmpty
		}
		// Preserve and return the detailed error from security
		return fmt.Errorf("%w: %s", ErrInvalidVariableName, err.Error())
	}

	// Only validate non-empty values post expansion.
	// Use the cached validator to avoid creating a new one on each call.
	if varValue != "" && p.validator != nil {
		if err := p.validator.ValidateEnvironmentValue(varName, varValue); err != nil {
			return err
		}
	}
	return nil
}

// ExpandString expands variables in a single string, handling escape sequences.
// It performs recursive variable expansion with circular reference detection.
// This method is used for expanding both command-line strings and environment variable values.
func (p *VariableExpander) ExpandString(value string, envVars map[string]string, allowlist []string, groupName string, visited map[string]bool) (string, error) {
	p.logger.Debug("Starting variable expansion",
		"value", value,
		"group", groupName,
		"visited_count", len(visited))

	var result strings.Builder
	inputChars := []rune(value)
	i := 0
	for i < len(inputChars) {
		switch inputChars[i] {
		case '\\':
			nextIdx, err := p.handleEscapeSequence(inputChars, i, &result)
			if err != nil {
				p.logger.Error("Escape sequence processing failed",
					"position", i,
					"error", err)
				return "", err
			}
			i = nextIdx
		case '$':
			nextIdx, err := p.handleVariableExpansion(inputChars, i, envVars, allowlist, groupName, visited, &result)
			if err != nil {
				p.logger.Error("Variable expansion failed",
					"position", i,
					"error", err)
				return "", err
			}
			i = nextIdx
		default:
			result.WriteRune(inputChars[i])
			i++
		}
	}
	expandedValue := result.String()
	p.logger.Debug("Variable expansion completed",
		"original", value,
		"expanded", expandedValue,
		"group", groupName)
	return expandedValue, nil
}

// handleEscapeSequence processes escape sequences like \$ and \\
func (p *VariableExpander) handleEscapeSequence(inputChars []rune, i int, result *strings.Builder) (int, error) {
	if i+1 >= len(inputChars) {
		return 0, fmt.Errorf("%w at position %d (trailing backslash)", ErrInvalidEscapeSequence, i)
	}
	nextChar := inputChars[i+1]
	if nextChar == '$' || nextChar == '\\' {
		p.logger.Debug("Processed escape sequence",
			"position", i,
			"sequence", fmt.Sprintf("\\%c", nextChar),
			"result", string(nextChar))
		result.WriteRune(nextChar)
		return i + 2, nil //nolint:mnd // Skip escape sequences like \$ and \\

	}
	return 0, fmt.Errorf("%w at position %d: \\%c", ErrInvalidEscapeSequence, i, nextChar)
}

// handleVariableExpansion processes variable expansion like ${VAR}
func (p *VariableExpander) handleVariableExpansion(inputChars []rune, i int, envVars map[string]string, allowlist []string, groupName string, visited map[string]bool, result *strings.Builder) (int, error) {
	// Strict validation: $ must be followed by {VAR} format
	if i+1 >= len(inputChars) || inputChars[i+1] != '{' {
		return 0, fmt.Errorf("%w at position %d", ErrInvalidVariableFormat, i)
	}

	// Find the closing brace
	start := i + 2 //nolint:mnd // Skip past the opening "${" sequence
	end := -1
	for j := start; j < len(inputChars); j++ {
		if inputChars[j] == '}' {
			end = j
			break
		}
	}
	if end == -1 {
		var varNamePrefix string
		if len(inputChars)-start > maxVarNamePrefixLength {
			varNamePrefix = string(inputChars[start:start+maxVarNamePrefixLength]) + "..."
		} else {
			varNamePrefix = string(inputChars[start:])
		}
		return 0, fmt.Errorf("%w at position %d: ${%s", ErrUnclosedVariable, i, varNamePrefix)
	}

	// Extract and validate variable name
	varName := string(inputChars[start:end])
	if err := security.ValidateVariableName(varName); err != nil {
		return 0, fmt.Errorf("%w at position %d: %s: %w", ErrInvalidVariableName, i, varName, err)
	}

	p.logger.Debug("Found variable reference",
		"variable", varName,
		"position", i,
		"group", groupName)

	// Resolve variable value
	valStr, err := p.resolveVariable(varName, envVars, allowlist, groupName, visited)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve variable ${%s} at position %d: %w", varName, i, err)
	}

	p.logger.Debug("Resolved variable value",
		"variable", varName,
		"value", valStr,
		"value_length", len(valStr))

	// Recursively expand the value
	expanded, err := p.ExpandString(valStr, envVars, allowlist, groupName, visited)
	if err != nil {
		return 0, fmt.Errorf("failed to expand nested variable ${%s} at position %d: %w", varName, i, err)
	}
	result.WriteString(expanded)
	delete(visited, varName)
	return end + 1, nil
}

// resolveVariable resolves a variable value from local env or system environment
// Strategy for handling self-references (e.g., PATH=/custom/bin:${PATH}):
//   - If the variable was pre-marked as visited by ExpandCommandEnv, skip local lookup
//     and go directly to system environment. This allows ${PATH} to refer to system PATH.
//   - If not pre-marked but found in envVars during recursive expansion, this is a true
//     circular reference and should be rejected.
func (p *VariableExpander) resolveVariable(varName string, envVars map[string]string, allowlist []string, groupName string, visited map[string]bool) (string, error) {
	// Check if variable is already being expanded (circular reference detection)
	wasVisited := visited[varName]

	// Look up variable value
	val, foundLocal := envVars[varName]
	if wasVisited && foundLocal {
		// Variable was pre-marked as visited - skip local lookup to enable self-reference
		p.logger.Debug("Skipping local lookup for self-reference",
			"variable", varName,
			"group", groupName)
		foundLocal = false
	}

	if foundLocal {
		// Found in local env and not pre-visited - mark as visited and use it
		p.logger.Debug("Resolved variable from command env",
			"variable", varName,
			"value_length", len(val),
			"group", groupName)
		visited[varName] = true
		return val, nil
	}

	// Not found locally (or was pre-visited) - try system environment
	sysVal, foundSys := os.LookupEnv(varName)
	if foundSys {
		// allowlist check only for system variables
		if !p.filter.IsVariableAccessAllowed(varName, allowlist, groupName) {
			p.logger.Warn("system variable access blocked by allowlist",
				"variable", varName,
				"group", groupName,
				"allowlist", allowlist)
			// Generate context-specific error message
			if groupName == "" {
				return "", fmt.Errorf("%w: %s", ErrVariableNotAllowed, varName)
			}
			return "", fmt.Errorf("%w: %s (not in allowlist for group %s)", ErrVariableNotAllowed, varName, groupName)
		}
		p.logger.Debug("Resolved variable from system environment",
			"variable", varName,
			"value_length", len(sysVal),
			"group", groupName)
		return sysVal, nil
	}

	if wasVisited {
		// Variable was pre-visited but not found in system - this is circular reference
		p.logger.Error("Circular reference detected",
			"variable", varName,
			"group", groupName)
		return "", fmt.Errorf("%w: %s (self-reference without system fallback)", ErrCircularReference, varName)
	}

	// variable not found anywhere - return error
	p.logger.Error("Variable not found",
		"variable", varName,
		"group", groupName)
	return "", fmt.Errorf("%w: %s (not found in command env or system environment)", ErrVariableNotFound, varName)
}

// ExpandStrings expands variables in multiple strings, handling them as a batch.
// This is a utility function for expanding command arguments and other string arrays.
func (p *VariableExpander) ExpandStrings(texts []string, envVars map[string]string, allowlist []string, groupName string) ([]string, error) {
	if texts == nil {
		return nil, nil
	}

	p.logger.Debug("Starting batch string expansion",
		"count", len(texts),
		"group", groupName)

	// Always allocate a new slice for the result even when len(texts)==0.
	// This ensures we don't return the caller's underlying slice and
	// keeps behavior consistent between empty and non-empty inputs.
	result := make([]string, len(texts))
	for i, text := range texts {
		expanded, err := p.ExpandString(text, envVars, allowlist, groupName, make(map[string]bool))
		if err != nil {
			p.logger.Error("Batch string expansion failed",
				"index", i,
				"text", text,
				"error", err)
			return nil, fmt.Errorf("failed to expand argument at index %d (%q): %w", i, text, err)
		}
		result[i] = expanded
	}

	p.logger.Debug("Batch string expansion completed",
		"count", len(texts),
		"group", groupName)

	return result, nil
}
