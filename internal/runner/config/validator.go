package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Static error definitions
var (
	// ErrValidationFailed indicates that configuration validation has failed
	ErrValidationFailed = errors.New("validation failed")
	// ErrInvalidVariableName indicates that a variable name is invalid
	ErrInvalidVariableName = errors.New("invalid variable name")
	// ErrDangerousPattern indicates that a dangerous pattern was detected
	ErrDangerousPattern = errors.New("dangerous pattern detected")
)

// Validator provides comprehensive validation for runner configurations
type Validator struct {
	logger            *slog.Logger
	securityValidator *security.Validator
}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator() *Validator {
	// Create security validator with default configuration
	secConfig := security.DefaultConfig()
	secValidator, err := security.NewValidator(secConfig)
	if err != nil {
		// Fallback to nil if security validator creation fails
		// This allows the main validator to still function
		slog.Warn("Failed to create security validator", "error", err)
		secValidator = nil
	}

	return &Validator{
		logger:            slog.Default(),
		securityValidator: secValidator,
	}
}

// ValidateConfig performs comprehensive validation of the configuration
func (v *Validator) ValidateConfig(config *runnertypes.Config) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:     true,
		Errors:    []ValidationError{},
		Warnings:  []ValidationWarning{},
		Timestamp: time.Now(),
	}

	// Validate global configuration
	v.validateGlobalConfig(&config.Global, result)

	// Validate groups
	seenGroupNames := make(map[string]int)
	for i, group := range config.Groups {
		// Check for duplicate group names before individual validation
		if _, exists := seenGroupNames[group.Name]; exists && group.Name != "" {
			result.Errors = append(result.Errors, ValidationError{
				Type:     "duplicate_group_name",
				Message:  fmt.Sprintf("Group name '%s' appears multiple times", group.Name),
				Location: fmt.Sprintf("groups[%d].name", i),
				Severity: "error",
			})
		}
		seenGroupNames[group.Name] = i

		v.validateGroup(&group, i, &config.Global, result)
	}

	// Calculate summary
	v.calculateSummary(config, result)

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	return result, nil
}

// validateGlobalConfig validates the global configuration settings
func (v *Validator) validateGlobalConfig(global *runnertypes.GlobalConfig, result *ValidationResult) {
	// Validate global allowlist
	v.validateAllowlist(global.EnvAllowlist, "global.env_allowlist", result)

	// Check for empty global allowlist
	if len(global.EnvAllowlist) == 0 {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:       "empty_global_allowlist",
			Message:    "Global environment allowlist is empty",
			Location:   "global.env_allowlist",
			Suggestion: "Consider adding commonly used variables like PATH, HOME, USER to the global allowlist",
		})
	}

	// Check for potentially dangerous variables in global allowlist
	dangerousVars := []string{"LD_LIBRARY_PATH", "LD_PRELOAD", "DYLD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES"}
	for _, dangerousVar := range dangerousVars {
		if slices.Contains(global.EnvAllowlist, dangerousVar) {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Type:       "dangerous_global_variable",
				Message:    fmt.Sprintf("Global allowlist contains potentially dangerous variable: %s", dangerousVar),
				Location:   "global.env_allowlist",
				Suggestion: "Consider removing dangerous variables from global allowlist and add them only to specific groups if needed",
			})
		}
	}
}

// validateGroup validates a command group configuration
func (v *Validator) validateGroup(group *runnertypes.CommandGroup, index int, global *runnertypes.GlobalConfig, result *ValidationResult) {
	groupLocation := fmt.Sprintf("groups[%d]", index)

	// Validate group name
	if group.Name == "" {
		result.Errors = append(result.Errors, ValidationError{
			Type:     "empty_group_name",
			Message:  "Group name cannot be empty",
			Location: fmt.Sprintf("%s.name", groupLocation),
			Severity: "error",
		})
	}

	// Note: Duplicate group names are now checked at the ValidateConfig level before calling this function

	// Validate group allowlist
	if group.EnvAllowlist != nil {
		v.validateAllowlist(group.EnvAllowlist, fmt.Sprintf("%s.env_allowlist", groupLocation), result)

		// Check inheritance mode and provide warnings
		v.analyzeInheritanceMode(group, groupLocation, global, result)
	}

	// Validate working directory
	if group.WorkDir != "" {
		v.validateWorkingDirectory(group.WorkDir, fmt.Sprintf("%s.workdir", groupLocation), result)
	}

	// Validate commands
	for i, cmd := range group.Commands {
		v.validateCommand(&cmd, i, fmt.Sprintf("%s.commands", groupLocation), result)
	}
}

// validateAllowlist validates an environment allowlist
func (v *Validator) validateAllowlist(allowlist []string, location string, result *ValidationResult) {
	seenVars := make(map[string]int)

	for i, variable := range allowlist {
		itemLocation := fmt.Sprintf("%s[%d]", location, i)

		// Check for empty variable names
		if variable == "" {
			result.Errors = append(result.Errors, ValidationError{
				Type:     "empty_variable_name",
				Message:  "Environment variable name cannot be empty",
				Location: itemLocation,
				Severity: "error",
			})
			continue
		}

		// Check for invalid variable names
		if err := v.validateVariableName(variable); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Type:     "invalid_variable_name",
				Message:  fmt.Sprintf("Invalid environment variable name '%s': %s", variable, err.Error()),
				Location: itemLocation,
				Severity: "error",
			})
		}

		// Check for duplicates
		if prevIndex, exists := seenVars[variable]; exists {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Type:       "duplicate_variable",
				Message:    fmt.Sprintf("Variable '%s' appears multiple times in allowlist", variable),
				Location:   itemLocation,
				Suggestion: fmt.Sprintf("Remove duplicate entry (also appears at index %d)", prevIndex),
			})
		}
		seenVars[variable] = i
	}
}

// validateVariableName validates an environment variable name using centralized security validation
func (v *Validator) validateVariableName(name string) error {
	if err := security.ValidateVariableName(name); err != nil {
		// Wrap the security error with our validation error type for consistency
		return fmt.Errorf("%w: %s", ErrInvalidVariableName, err.Error())
	}
	return nil
}

// validateWorkingDirectory validates a working directory path
func (v *Validator) validateWorkingDirectory(path, location string, result *ValidationResult) {
	// Check for empty path
	if strings.TrimSpace(path) == "" {
		result.Errors = append(result.Errors, ValidationError{
			Type:     "empty_working_directory",
			Message:  "Working directory cannot be empty",
			Location: location,
			Severity: "error",
		})
		return
	}

	// Check for potentially dangerous paths
	dangerousPaths := []string{"/", "/bin", "/sbin", "/usr", "/etc", "/var", "/tmp"}
	for _, dangerous := range dangerousPaths {
		if path == dangerous {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Type:       "dangerous_working_directory",
				Message:    fmt.Sprintf("Working directory '%s' is potentially dangerous", path),
				Location:   location,
				Suggestion: "Consider using a more specific subdirectory",
			})
		}
	}
}

// validateCommand validates a command configuration
func (v *Validator) validateCommand(cmd *runnertypes.Command, index int, location string, result *ValidationResult) {
	cmdLocation := fmt.Sprintf("%s[%d]", location, index)

	// Validate command name
	if cmd.Name == "" {
		result.Errors = append(result.Errors, ValidationError{
			Type:     "empty_command_name",
			Message:  "Command name cannot be empty",
			Location: fmt.Sprintf("%s.name", cmdLocation),
			Severity: "error",
		})
	}

	// Validate command path
	if cmd.Cmd == "" {
		result.Errors = append(result.Errors, ValidationError{
			Type:     "empty_command_path",
			Message:  "Command path cannot be empty",
			Location: fmt.Sprintf("%s.cmd", cmdLocation),
			Severity: "error",
		})
	}

	// Validate privileged commands
	if cmd.Privileged {
		v.validatePrivilegedCommand(cmd, cmdLocation, result)
	}

	// Validate command environment variables
	v.validateCommandEnv(cmd.Env, fmt.Sprintf("%s.env", cmdLocation), result)
}

// validateCommandEnv validates command-specific environment variables
func (v *Validator) validateCommandEnv(env []string, location string, result *ValidationResult) {
	seenVars := make(map[string]int)

	for i, envVar := range env {
		itemLocation := fmt.Sprintf("%s[%d]", location, i)

		// Parse environment variable
		varName, varValue, ok := environment.ParseEnvVariable(envVar)
		if !ok {
			result.Errors = append(result.Errors, ValidationError{
				Type:     "invalid_env_format",
				Message:  fmt.Sprintf("Invalid environment variable format: '%s' (expected KEY=VALUE)", envVar),
				Location: itemLocation,
				Severity: "error",
			})
			continue
		}

		// Validate variable name
		if err := v.validateVariableName(varName); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Type:     "invalid_command_env_name",
				Message:  fmt.Sprintf("Invalid command environment variable name '%s': %s", varName, err.Error()),
				Location: itemLocation,
				Severity: "error",
			})
		}

		// Check for duplicates
		if prevIndex, exists := seenVars[varName]; exists {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Type:       "duplicate_command_env",
				Message:    fmt.Sprintf("Command environment variable '%s' appears multiple times", varName),
				Location:   itemLocation,
				Suggestion: fmt.Sprintf("Remove duplicate entry (also appears at index %d)", prevIndex),
			})
		}
		seenVars[varName] = i

		// Check for dangerous patterns in values
		if err := v.validateVariableValue(varValue); err != nil {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Type:       "dangerous_command_env_value",
				Message:    fmt.Sprintf("Command environment variable '%s' contains potentially dangerous value: %s", varName, err.Error()),
				Location:   itemLocation,
				Suggestion: "Review the variable value for potential security issues",
			})
		}
	}
}

// validateVariableValue validates an environment variable value for dangerous patterns
func (v *Validator) validateVariableValue(value string) error {
	// Use centralized security validation
	if err := security.IsVariableValueSafe(value); err != nil {
		// Wrap the security error with our validation error type for consistency
		return fmt.Errorf("%w: %s", ErrDangerousPattern, err.Error())
	}

	return nil
}

// analyzeInheritanceMode analyzes the inheritance mode and provides appropriate warnings
func (v *Validator) analyzeInheritanceMode(group *runnertypes.CommandGroup, location string, global *runnertypes.GlobalConfig, result *ValidationResult) {
	if group.EnvAllowlist == nil {
		// Inherit mode
		if len(global.EnvAllowlist) == 0 {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Type:       "inherit_from_empty_global",
				Message:    "Group inherits from empty global allowlist",
				Location:   fmt.Sprintf("%s.env_allowlist", location),
				Suggestion: "Either add variables to global allowlist or define explicit group allowlist",
			})
		}
	} else if len(group.EnvAllowlist) == 0 {
		// Reject mode
		hasCommandsWithEnv := false
		for _, cmd := range group.Commands {
			if len(cmd.Env) > 0 {
				hasCommandsWithEnv = true
				break
			}
		}

		if hasCommandsWithEnv {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Type:       "reject_mode_with_command_env",
				Message:    "Group rejects all environment variables but has commands with Command.Env",
				Location:   fmt.Sprintf("%s.env_allowlist", location),
				Suggestion: "Command.Env variables will still work, but cannot reference system variables",
			})
		}
	}
	// Explicit mode doesn't need special warnings
}

// calculateSummary calculates validation summary statistics
func (v *Validator) calculateSummary(config *runnertypes.Config, result *ValidationResult) {
	summary := &result.Summary

	summary.TotalGroups = len(config.Groups)
	summary.GlobalAllowlistSize = len(config.Global.EnvAllowlist)

	for _, group := range config.Groups {
		if group.EnvAllowlist != nil {
			summary.GroupsWithAllowlist++
		}

		for _, cmd := range group.Commands {
			summary.TotalCommands++
			if len(cmd.Env) > 0 {
				summary.CommandsWithEnv++
			}
		}
	}
}

// GenerateValidationReport generates a human-readable validation report
func (v *Validator) GenerateValidationReport(result *ValidationResult) (string, error) {
	var buf bytes.Buffer

	// Header
	fmt.Fprintf(&buf, "Configuration Validation Report\n")
	fmt.Fprintf(&buf, "Generated: %s\n", result.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(&buf, "Overall Status: %s\n", v.getStatusString(result.Valid))
	fmt.Fprintf(&buf, "\n")

	// Summary
	fmt.Fprintf(&buf, "Summary:\n")
	fmt.Fprintf(&buf, "  Total Groups: %d\n", result.Summary.TotalGroups)
	fmt.Fprintf(&buf, "  Groups with Explicit Allowlist: %d\n", result.Summary.GroupsWithAllowlist)
	fmt.Fprintf(&buf, "  Global Allowlist Size: %d\n", result.Summary.GlobalAllowlistSize)
	fmt.Fprintf(&buf, "  Total Commands: %d\n", result.Summary.TotalCommands)
	fmt.Fprintf(&buf, "  Commands with Env Variables: %d\n", result.Summary.CommandsWithEnv)
	fmt.Fprintf(&buf, "\n")

	// Errors
	if len(result.Errors) > 0 {
		fmt.Fprintf(&buf, "Errors (%d):\n", len(result.Errors))
		for i, err := range result.Errors {
			fmt.Fprintf(&buf, "  %d. [%s] %s\n", i+1, err.Location, err.Message)
		}
		fmt.Fprintf(&buf, "\n")
	}

	// Warnings
	if len(result.Warnings) > 0 {
		fmt.Fprintf(&buf, "Warnings (%d):\n", len(result.Warnings))
		for i, warning := range result.Warnings {
			fmt.Fprintf(&buf, "  %d. [%s] %s\n", i+1, warning.Location, warning.Message)
			if warning.Suggestion != "" {
				fmt.Fprintf(&buf, "     Suggestion: %s\n", warning.Suggestion)
			}
		}
		fmt.Fprintf(&buf, "\n")
	}

	if len(result.Errors) == 0 && len(result.Warnings) == 0 {
		fmt.Fprintf(&buf, "No issues found.\n")
	}

	return buf.String(), nil
}

// getStatusString returns a string representation of the validation status
func (v *Validator) getStatusString(valid bool) string {
	if valid {
		return "VALID"
	}
	return "INVALID"
}

// validatePrivilegedCommand validates privileged command security
func (v *Validator) validatePrivilegedCommand(cmd *runnertypes.Command, location string, result *ValidationResult) {
	// Skip validation if security validator is not available
	if v.securityValidator == nil {
		return
	}

	// Check for potentially dangerous commands
	if v.securityValidator.IsDangerousPrivilegedCommand(cmd.Cmd) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:       "security",
			Location:   fmt.Sprintf("%s.cmd", location),
			Message:    fmt.Sprintf("Privileged command uses potentially dangerous path: %s", cmd.Cmd),
			Suggestion: "Consider using a safer alternative or additional validation",
		})
	}

	// Check for shell commands
	if v.securityValidator.IsShellCommand(cmd.Cmd) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:       "security",
			Location:   fmt.Sprintf("%s.cmd", location),
			Message:    "Privileged shell commands require extra caution",
			Suggestion: "Avoid using shell commands with privileges or implement strict argument validation",
		})
	}

	// Check for commands with shell metacharacters in arguments
	if v.securityValidator.HasShellMetacharacters(cmd.Args) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:       "security",
			Location:   fmt.Sprintf("%s.args", location),
			Message:    "Command arguments contain shell metacharacters - ensure proper escaping",
			Suggestion: "Use absolute paths and avoid shell metacharacters in arguments",
		})
	}

	// Check for relative paths
	if !filepath.IsAbs(cmd.Cmd) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:       "security",
			Location:   fmt.Sprintf("%s.cmd", location),
			Message:    "Privileged command uses relative path - consider using absolute path for security",
			Suggestion: "Use absolute path to prevent PATH-based attacks",
		})
	}
}
