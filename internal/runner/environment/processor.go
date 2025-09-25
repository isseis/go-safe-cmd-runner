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

// CommandEnvProcessor handles the processing of environment variables for a command.
type CommandEnvProcessor struct {
	filter *Filter
	logger *slog.Logger
}

// NewCommandEnvProcessor creates a new CommandEnvProcessor.
func NewCommandEnvProcessor(filter *Filter) *CommandEnvProcessor {
	return &CommandEnvProcessor{
		filter: filter,
		logger: slog.Default().With("component", "CommandEnvProcessor"),
	}
}

// ProcessCommandEnvironment processes and prepares the environment variables for a command.
// It uses a two-pass approach:
//  1. First pass: Add all variables from the command's `Env` block to the environment map.
//     This allows for self-references and inter-references within the `Env` block.
//  2. Second pass: Iterate over the map and expand any variables in the values.
func (p *CommandEnvProcessor) ProcessCommandEnvironment(cmd runnertypes.Command, baseEnvVars map[string]string, group *runnertypes.CommandGroup) (map[string]string, error) {
	finalEnv := make(map[string]string)
	for k, v := range baseEnvVars {
		finalEnv[k] = v
	}

	// First pass: Populate the environment with unexpanded values from the command.
	for _, envStr := range cmd.Env {
		varName, varValue, ok := strings.Cut(envStr, "=")
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrMalformedEnvVariable, envStr)
		}
		// Validate only the name at this stage.
		if err := validateBasicEnvVariable(varName, ""); err != nil {
			return nil, err
		}
		finalEnv[varName] = varValue
	}

	// Second pass: Expand all variables.
	for name := range finalEnv {
		value := finalEnv[name]
		expandedValue, err := p.ExpandVariablesWithEscaping(value, finalEnv, group)
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
	if !ValidateVariableName(varName) {
		if varName == "" {
			return ErrVariableNameEmpty
		}
		return fmt.Errorf("%w: %s", ErrInvalidVariableName, varName)
	}
	if varValue != "" { // only validate non-empty values post expansion
		if err := security.IsVariableValueSafe(varValue); err != nil {
			return fmt.Errorf("%w: %s", security.ErrUnsafeEnvironmentVar, varValue)
		}
	}
	return nil
}

// ExpandVariablesWithEscaping expands variables in a string, handling escape sequences.
// It's the entry point for the recursive expansion logic.
func (p *CommandEnvProcessor) ExpandVariablesWithEscaping(value string, envVars map[string]string, group *runnertypes.CommandGroup) (string, error) {
	return p.expand(value, envVars, group, make(map[string]bool))
}

// expand is the internal recursive function that performs the variable expansion.
func (p *CommandEnvProcessor) expand(value string, envVars map[string]string, group *runnertypes.CommandGroup, visited map[string]bool) (string, error) {
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

			start := i + 2 //nolint:mnd // position after ${
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
					if !p.filter.IsVariableAccessAllowed(varName, group) {
						p.logger.Warn("system variable access not allowed", "variable", varName, "group", group.Name)
						return "", fmt.Errorf("%w: %s", ErrVariableNotAllowed, varName)
					}
					valStr, found = sysVal, true
				}
			}
			if !found { // truly not found anywhere - treat as empty string per shell behavior
				valStr = ""
			}
			expanded, err := p.expand(valStr, envVars, group, visited)
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

// ValidateVariableName checks if a variable name is valid.
// It must start with a letter or underscore, followed by letters, numbers, or underscores.
func ValidateVariableName(name string) bool { // keep bool helper for existing callers
	return security.ValidateVariableName(name) == nil
}

// ValidateVariableValue checks if a variable value contains any dangerous patterns.
// ValidateVariableValue deprecated in favor of security.IsVariableValueSafe (kept for tests already migrated to bool helper)
func ValidateVariableValue(value string) bool { //nolint:revive // compatibility shim
	return security.IsVariableValueSafe(value) == nil
}
