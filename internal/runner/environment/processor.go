// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"regexp"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// MaxVariableExpansionIterations defines the maximum number of expansion iterations to prevent infinite loops
const MaxVariableExpansionIterations = 15

// ErrCircularReference is returned when a circular variable reference is detected.
var ErrCircularReference = errors.New("circular variable reference detected")

// CommandEnvProcessor handles command-specific environment variable processing
type CommandEnvProcessor struct {
	filter *Filter
	logger *slog.Logger
}

// NewCommandEnvProcessor creates a new processor for command environment variables
func NewCommandEnvProcessor(filter *Filter) *CommandEnvProcessor {
	return &CommandEnvProcessor{
		filter: filter,
		logger: slog.Default(),
	}
}

// ProcessCommandEnvironment processes Command.Env variables without allowlist checks
func (p *CommandEnvProcessor) ProcessCommandEnvironment(
	cmd runnertypes.Command,
	baseEnvVars map[string]string,
	group *runnertypes.CommandGroup,
) (map[string]string, error) {
	// Create a copy of base environment variables
	envVars := make(map[string]string, len(baseEnvVars))
	maps.Copy(envVars, baseEnvVars)

	// Process each Command.Env entry
	for i, env := range cmd.Env {
		variable, value, ok := ParseEnvVariable(env)
		if !ok {
			return nil, fmt.Errorf("invalid environment variable format in Command.Env in command %s, env_index: %d, env_entry: %s: %w", cmd.Name, i, env, ErrMalformedEnvVariable)
		}

		// Basic validation (but no allowlist check)
		if err := p.validateBasicEnvVariable(variable, value); err != nil {
			return nil, fmt.Errorf("malformed command environment variable %s in command %s: %w",
				variable, cmd.Name, err)
		}

		// Resolve variable references with security policy
		resolvedValue, err := p.resolveVariableReferencesForCommandEnv(value, envVars, group)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve variable references in %s for command %s: %w",
				variable, cmd.Name, err)
		}

		envVars[variable] = resolvedValue

		p.logger.Debug("Processed command environment variable",
			"command", cmd.Name,
			"variable", variable,
			"value_length", len(resolvedValue))
	}

	return envVars, nil
}

// validateBasicEnvVariable performs basic validation on environment variables
// without allowlist checks (which are bypassed for Command.Env)
func (p *CommandEnvProcessor) validateBasicEnvVariable(name, value string) error {
	// Use the filter's existing validation methods
	if err := p.filter.ValidateVariableName(name); err != nil {
		return err
	}

	if err := p.filter.ValidateVariableValue(value); err != nil {
		// Return security error for dangerous variable values
		if errors.Is(err, ErrDangerousVariableValue) {
			return fmt.Errorf("%w: command environment variable %s contains dangerous pattern", security.ErrUnsafeEnvironmentVar, name)
		}
		return err
	}

	return nil
}

var (
	variableReferenceRegex = regexp.MustCompile(`\$\{([^}]+)\}`)
	simpleVariableRegex    = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
)

// resolveVariableReferencesForCommandEnv resolves variable references for Command.Env values
func (p *CommandEnvProcessor) resolveVariableReferencesForCommandEnv(
	value string,
	envVars map[string]string,
	group *runnertypes.CommandGroup,
) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}

	result := value
	maxIterations := MaxVariableExpansionIterations
	var resolutionError error

	for i := 0; i < maxIterations && strings.Contains(result, "${"); i++ {
		oldResult := result

		result = variableReferenceRegex.ReplaceAllStringFunc(result, func(match string) string {
			varName := match[2 : len(match)-1] // Remove ${ and }

			resolvedValue, err := p.resolveVariableWithSecurityPolicy(varName, envVars, group)
			if err != nil {
				if resolutionError == nil {
					resolutionError = fmt.Errorf("failed to resolve variable reference ${%s}: %w", varName, err)
				}
				return match // Continue processing other variables
			}

			return resolvedValue
		})

		if result == oldResult {
			// No more substitutions were made, we're done
			break
		}
	}

	if resolutionError != nil {
		return "", resolutionError
	}

	// Check if we exceeded max iterations and still have unresolved references
	// This indicates potential circular reference, but we need to be careful about malformed references
	if strings.Contains(result, "${") {
		// Check if the remaining references are well-formed (have closing braces)
		if variableReferenceRegex.MatchString(result) {
			// Well-formed references remaining after max iterations = circular reference
			return "", fmt.Errorf("%w: exceeded maximum resolution iterations (%d)", ErrCircularReference, maxIterations)
		}
		// Malformed references (like ${UNCLOSED) are left as-is
	}

	return result, nil
}

// resolveVariableWithSecurityPolicy resolves a variable reference with appropriate security checks
func (p *CommandEnvProcessor) resolveVariableWithSecurityPolicy(
	varName string,
	envVars map[string]string,
	group *runnertypes.CommandGroup,
) (string, error) {
	// Priority 1: Check existing resolved variables (Command.Env + trusted sources)
	if val, exists := envVars[varName]; exists {
		p.logger.Debug("Variable resolved from trusted source",
			"variable", varName,
			"source", "resolved_env_vars")
		return val, nil
	}

	// Priority 2: Check system environment with allowlist validation
	if sysVal, exists := os.LookupEnv(varName); exists {
		return p.resolveSystemVariable(varName, sysVal, group)
	}

	// Priority 3: Variable not found
	return "", fmt.Errorf("%w: %s", ErrVariableNotFound, varName)
}

// resolveSystemVariable resolves a system environment variable with allowlist checks
func (p *CommandEnvProcessor) resolveSystemVariable(
	varName, sysVal string,
	group *runnertypes.CommandGroup,
) (string, error) {
	allowed, err := p.filter.resolveAllowedVariable(varName, group)
	if err != nil {
		p.logger.Error("Failed to check variable allowlist",
			"variable", varName,
			"group", group.Name,
			"error", err)
		return "", fmt.Errorf("allowlist check failed for variable %s: %w", varName, err)
	}

	if !allowed {
		p.logger.Warn("Command.Env references disallowed system variable",
			"variable", varName,
			"group", group.Name)
		return "", fmt.Errorf("%w: variable '%s' is not allowed in group '%s'", ErrVariableNotAllowed, varName, group.Name)
	}

	p.logger.Debug("System variable resolved for Command.Env",
		"variable", varName,
		"group", group.Name)
	return sysVal, nil
}

// ResolveVariableReferencesUnified resolves both $VAR and ${VAR} formats for cmd/args expansion
func (p *CommandEnvProcessor) ResolveVariableReferencesUnified(
	value string,
	envVars map[string]string,
	group *runnertypes.CommandGroup,
) (string, error) {
	if !strings.Contains(value, "$") {
		return value, nil
	}

	result := value
	maxIterations := MaxVariableExpansionIterations
	var resolutionError error

	for i := 0; i < maxIterations && strings.Contains(result, "$"); i++ {
		oldResult := result

		// Process ${VAR} format first (existing logic)
		result = variableReferenceRegex.ReplaceAllStringFunc(result, func(match string) string {
			varName := match[2 : len(match)-1] // Remove ${ and }

			resolvedValue, err := p.resolveVariableWithSecurityPolicy(varName, envVars, group)
			if err != nil {
				if resolutionError == nil {
					resolutionError = fmt.Errorf("failed to resolve variable reference ${%s}: %w", varName, err)
				}
				return match // Continue processing other variables
			}

			return resolvedValue
		})

		// Process $VAR format (new functionality)
		result = p.replaceSimpleVariables(result, envVars, group, &resolutionError)

		if result == oldResult {
			// No more substitutions were made, we're done
			break
		}
	}

	if resolutionError != nil {
		return "", resolutionError
	}

	// Check if we exceeded max iterations and still have unresolved references
	if strings.Contains(result, "$") {
		if variableReferenceRegex.MatchString(result) || simpleVariableRegex.MatchString(result) {
			return "", fmt.Errorf("%w: exceeded maximum resolution iterations (%d)", ErrCircularReference, maxIterations)
		}
	}

	return result, nil
}

// replaceSimpleVariables handles $VAR format replacement with overlap prevention
func (p *CommandEnvProcessor) replaceSimpleVariables(
	text string,
	envVars map[string]string,
	group *runnertypes.CommandGroup,
	resolutionError *error,
) string {
	return simpleVariableRegex.ReplaceAllStringFunc(text, func(match string) string {
		varName := match[1:] // Remove $

		resolvedValue, err := p.resolveVariableWithSecurityPolicy(varName, envVars, group)
		if err != nil {
			if *resolutionError == nil {
				*resolutionError = fmt.Errorf("failed to resolve variable reference $%s: %w", varName, err)
			}
			return match // Continue processing other variables
		}

		return resolvedValue
	})
}
