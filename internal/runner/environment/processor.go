// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

var (
	// ErrCircularReference is returned when a circular variable reference is detected.
	ErrCircularReference = errors.New("circular variable reference")
	// ErrInvalidEscapeSequence is returned when an invalid escape sequence is detected.
	ErrInvalidEscapeSequence = errors.New("invalid escape sequence")
	// ErrUnclosedVariable is returned when a variable expansion is not properly closed.
	ErrUnclosedVariable = errors.New("unclosed variable")
	// ErrInvalidVariableFormat is returned when $ is found but not followed by valid variable syntax.
	ErrInvalidVariableFormat = errors.New("invalid variable format")
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
	// Create a cached security validator with default config
	// This avoids creating a new validator (and compiling regexes) on every validation call
	validator, err := security.NewValidator(nil)
	if err != nil {
		// Fall back to nil validator - validation will be skipped
		// This should not happen with default config, but handle gracefully
		slog.Default().Warn("Failed to create security validator, validation will be limited", "error", err)
	}

	return &VariableExpander{
		filter:    filter,
		logger:    slog.Default().With("component", "VariableExpander"),
		validator: validator,
	}
}

// ExpandCommandEnv expands Command.Env variables without base environment.
// This is used during configuration loading (Phase 1) to pre-expand Command.Env.
// Returns a map of expanded environment variables ready to merge with system environment.
// It uses a two-pass approach:
//  1. First pass: Add all variables from the command's `Env` block to the environment map.
//     This allows for self-references and inter-references within the `Env` block.
//  2. Second pass: Iterate over the map and expand any variables in the values.
func (p *VariableExpander) ExpandCommandEnv(cmd *runnertypes.Command, groupName string, groupEnvAllowList []string) (map[string]string, error) {
	finalEnv := make(map[string]string)

	// First pass: Populate the environment with unexpanded values from the command.
	for i, envStr := range cmd.Env {
		varName, varValue, ok := ParseEnvVariable(envStr)
		if !ok {
			return nil, fmt.Errorf("invalid environment variable format in Command.Env in command %s, env_index: %d, env_entry: %s: %w", cmd.Name, i, envStr, ErrMalformedEnvVariable)
		}
		// Validate only the name at this stage.
		if err := p.validateBasicEnvVariable(varName, ""); err != nil {
			return nil, fmt.Errorf("malformed command environment variable %s in command %s: %w",
				varName, cmd.Name, err)
		}
		finalEnv[varName] = varValue

		p.logger.Debug("Processed command environment variable",
			"command", cmd.Name,
			"variable", varName,
			"value_length", len(varValue))
	}

	// Second pass: Expand all variables.
	for name := range finalEnv {
		value := finalEnv[name]
		// Create a new visited map for each variable expansion to prevent false circular reference detection.
		// The key insight: when expanding "PATH=/custom/bin:${PATH}", we want ${PATH} to resolve to
		// the system PATH, not the partially-expanded value in finalEnv["PATH"].
		// By marking the current variable as visited, ExpandString will skip it in finalEnv and
		// fall back to system environment lookup.
		visited := map[string]bool{name: true}
		expandedValue, err := p.ExpandString(value, finalEnv, groupEnvAllowList, groupName, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to expand variable %s: %w", name, err)
		}
		finalEnv[name] = expandedValue
	}

	// Final validation pass on the fully expanded values.
	for name, value := range finalEnv {
		if err := p.validateBasicEnvVariable(name, value); err != nil {
			return nil, fmt.Errorf("validation failed for expanded variable %s: %w", name, err)
		}
	}

	return finalEnv, nil
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
			return fmt.Errorf("%w: command environment variable %s: %s", security.ErrUnsafeEnvironmentVar, varName, err.Error())
		}
	}
	return nil
}

// ExpandString expands variables in a single string, handling escape sequences.
// It performs recursive variable expansion with circular reference detection.
// This method is used for expanding both command-line strings and environment variable values.
func (p *VariableExpander) ExpandString(value string, envVars map[string]string, allowlist []string, groupName string, visited map[string]bool) (string, error) {
	var result strings.Builder
	runes := []rune(value)
	i := 0
	for i < len(runes) {
		switch runes[i] {
		case '\\':
			nextIdx, err := p.handleEscapeSequence(runes, i, &result)
			if err != nil {
				return "", err
			}
			i = nextIdx
		case '$':
			nextIdx, err := p.handleVariableExpansion(runes, i, envVars, allowlist, groupName, visited, &result)
			if err != nil {
				return "", err
			}
			i = nextIdx
		default:
			result.WriteRune(runes[i])
			i++
		}
	}
	return result.String(), nil
}

// handleEscapeSequence processes escape sequences like \$ and \\
func (p *VariableExpander) handleEscapeSequence(runes []rune, i int, result *strings.Builder) (int, error) {
	if i+1 >= len(runes) {
		return 0, ErrInvalidEscapeSequence
	}
	nextChar := runes[i+1]
	if nextChar == '$' || nextChar == '\\' {
		result.WriteRune(nextChar)
		return i + 2, nil //nolint:mnd // Skip escape sequences like \$ and \\

	}
	return 0, fmt.Errorf("%w: \\%c", ErrInvalidEscapeSequence, nextChar)
}

// handleVariableExpansion processes variable expansion like ${VAR}
func (p *VariableExpander) handleVariableExpansion(runes []rune, i int, envVars map[string]string, allowlist []string, groupName string, visited map[string]bool, result *strings.Builder) (int, error) {
	// Strict validation: $ must be followed by {VAR} format
	if i+1 >= len(runes) || runes[i+1] != '{' {
		return 0, ErrInvalidVariableFormat
	}

	// Find the closing brace
	start := i + 2 //nolint:mnd // Skip past the opening "${" sequence
	end := -1
	for j := start; j < len(runes); j++ {
		if runes[j] == '}' {
			end = j
			break
		}
	}
	if end == -1 {
		return 0, ErrUnclosedVariable
	}

	// Extract and validate variable name
	varName := string(runes[start:end])
	if err := security.ValidateVariableName(varName); err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrInvalidVariableName, varName, err)
	}

	// Resolve variable value
	valStr, err := p.resolveVariable(varName, envVars, allowlist, groupName, visited)
	if err != nil {
		return 0, err
	}

	// Recursively expand the value
	expanded, err := p.ExpandString(valStr, envVars, allowlist, groupName, visited)
	if err != nil {
		return 0, fmt.Errorf("failed to expand nested variable ${%s}: %w", varName, err)
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
		foundLocal = false
	}

	if foundLocal {
		// Found in local env and not pre-visited - mark as visited and use it
		visited[varName] = true
		return val, nil
	}

	// Not found locally (or was pre-visited) - try system environment
	sysVal, foundSys := os.LookupEnv(varName)
	if foundSys {
		// allowlist check only for system variables
		if !p.filter.IsVariableAccessAllowed(varName, allowlist, groupName) {
			p.logger.Warn("system variable access not allowed", "variable", varName, "group", groupName)
			return "", fmt.Errorf("%w: %s", ErrVariableNotAllowed, varName)
		}
		return sysVal, nil
	}

	if wasVisited {
		// Variable was pre-visited but not found in system - this is circular reference
		return "", fmt.Errorf("%w: %s", ErrCircularReference, varName)
	}

	// variable not found anywhere - return error
	return "", fmt.Errorf("%w: %s", ErrVariableNotFound, varName)
}

// ExpandStrings expands variables in multiple strings, handling them as a batch.
// This is a utility function for expanding command arguments and other string arrays.
func (p *VariableExpander) ExpandStrings(texts []string, envVars map[string]string, allowlist []string, groupName string) ([]string, error) {
	if texts == nil {
		return nil, nil
	}

	// Always allocate a new slice for the result even when len(texts)==0.
	// This ensures we don't return the caller's underlying slice and
	// keeps behavior consistent between empty and non-empty inputs.
	result := make([]string, len(texts))
	for i, text := range texts {
		expanded, err := p.ExpandString(text, envVars, allowlist, groupName, make(map[string]bool))
		if err != nil {
			return nil, fmt.Errorf("failed to expand text[%d]: %w", i, err)
		}
		result[i] = expanded
	}
	return result, nil
}
