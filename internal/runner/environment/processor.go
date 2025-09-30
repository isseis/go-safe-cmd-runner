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
	filter *Filter
	logger *slog.Logger
}

// NewVariableExpander creates a new VariableExpander.
func NewVariableExpander(filter *Filter) *VariableExpander {
	return &VariableExpander{
		filter: filter,
		logger: slog.Default().With("component", "VariableExpander"),
	}
}

// BuildEnvironmentMap builds the final environment variable map for command execution.
// It uses a two-pass approach:
//  1. First pass: Add all variables from the command's `Env` block to the environment map.
//     This allows for self-references and inter-references within the `Env` block.
//  2. Second pass: Iterate over the map and expand any variables in the values.
func (p *VariableExpander) BuildEnvironmentMap(cmd runnertypes.Command, baseEnvVars map[string]string, group *runnertypes.CommandGroup) (map[string]string, error) {
	finalEnv := make(map[string]string)
	for k, v := range baseEnvVars {
		finalEnv[k] = v
	}

	// First pass: Populate the environment with unexpanded values from the command.
	for i, envStr := range cmd.Env {
		varName, varValue, ok := ParseEnvVariable(envStr)
		if !ok {
			return nil, fmt.Errorf("invalid environment variable format in Command.Env in command %s, env_index: %d, env_entry: %s: %w", cmd.Name, i, envStr, ErrMalformedEnvVariable)
		}
		// Validate only the name at this stage.
		if err := validateBasicEnvVariable(varName, ""); err != nil {
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
		expandedValue, err := p.ExpandString(value, finalEnv, group.EnvAllowlist, group.Name, make(map[string]bool))
		if err != nil {
			return nil, fmt.Errorf("failed to expand variable %s: %w", name, err)
		}
		finalEnv[name] = expandedValue
	}

	// Final validation pass on the fully expanded values.
	for name, value := range finalEnv {
		if err := validateBasicEnvVariable(name, value); err != nil {
			return nil, fmt.Errorf("validation failed for expanded variable %s: %w", name, err)
		}
	}

	return finalEnv, nil
}

// validateBasicEnvVariable validates the name and optionally the value of an environment variable.
func validateBasicEnvVariable(varName, varValue string) error {
	// Validate name using security package which returns detailed errors.
	if err := security.ValidateVariableName(varName); err != nil {
		if varName == "" {
			return ErrVariableNameEmpty
		}
		// Preserve and return the detailed error from security
		return fmt.Errorf("%w: %s", ErrInvalidVariableName, err.Error())
	}

	// Only validate non-empty values post expansion. Use security.IsVariableValueSafe
	// which provides detailed errors about unsafe patterns.
	if varValue != "" {
		if err := security.IsVariableValueSafe(varName, varValue); err != nil {
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
			if i+1 >= len(runes) {
				return "", ErrInvalidEscapeSequence
			}
			nextChar := runes[i+1]
			if nextChar == '$' || nextChar == '\\' {
				result.WriteRune(nextChar)
				i += 2
			} else {
				return "", fmt.Errorf("%w: \\%c", ErrInvalidEscapeSequence, nextChar)
			}
		case '$':
			// Strict validation: $ must be followed by {VAR} format
			if i+1 >= len(runes) || runes[i+1] != '{' {
				return "", ErrInvalidVariableFormat
			}

			start := i + 2 //nolint:mnd // Skip past the opening "${" sequence
			end := -1
			for j := start; j < len(runes); j++ {
				if runes[j] == '}' {
					end = j
					break
				}
			}
			if end == -1 {
				return "", ErrUnclosedVariable
			}
			varName := string(runes[start:end])
			if err := security.ValidateVariableName(varName); err != nil {
				return "", fmt.Errorf("%w: %s: %w", ErrInvalidVariableName, varName, err)
			}
			if visited[varName] {
				return "", fmt.Errorf("%w: %s", ErrCircularReference, varName)
			}
			// Mark visited (depth-first). We'll remove manually after expansion.
			visited[varName] = true

			// 1. existence check (local then system) – differentiate not found
			val, foundLocal := envVars[varName]
			var ( // track where value came from
				valStr string
				found  bool
			)
			if foundLocal {
				valStr, found = val, true
			} else {
				sysVal, foundSys := os.LookupEnv(varName)
				if foundSys {
					// 2. allowlist check only after confirming existence
					if !p.filter.IsVariableAccessAllowed(varName, allowlist, groupName) {
						p.logger.Warn("system variable access not allowed", "variable", varName, "group", groupName)
						return "", fmt.Errorf("%w: %s", ErrVariableNotAllowed, varName)
					}
					valStr, found = sysVal, true
				}
			}
			if !found { // variable not found anywhere - return error
				return "", fmt.Errorf("%w: %s", ErrVariableNotFound, varName)
			}
			expanded, err := p.ExpandString(valStr, envVars, allowlist, groupName, visited)
			if err != nil {
				return "", fmt.Errorf("failed to expand nested variable ${%s}: %w", varName, err)
			}
			result.WriteString(expanded)
			delete(visited, varName)
			i = end + 1
		default:
			result.WriteRune(runes[i])
			i++
		}
	}
	return result.String(), nil
}

// ExpandStrings expands variables in multiple strings, handling them as a batch.
// This is a utility function for expanding command arguments and other string arrays.
func (p *VariableExpander) ExpandStrings(texts []string, envVars map[string]string, allowlist []string, groupName string) ([]string, error) {
	if len(texts) == 0 {
		return texts, nil
	}

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
