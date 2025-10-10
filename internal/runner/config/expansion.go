// Package config provides configuration management and variable expansion for commands.
package config

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

var (
	// ErrNilExpansionContext is returned when ExpansionContext is nil
	ErrNilExpansionContext = errors.New("expansion context cannot be nil")
	// ErrNilCommand is returned when Command in ExpansionContext is nil
	ErrNilCommand = errors.New("command cannot be nil")
	// ErrNilExpander is returned when Expander in ExpansionContext is nil
	ErrNilExpander = errors.New("expander cannot be nil")
	// ErrNilConfig is returned when config parameter is nil
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrGlobalVerifyFilesExpansionFailed is returned when global verify_files expansion fails
	ErrGlobalVerifyFilesExpansionFailed = errors.New("failed to expand global verify_files")
	// ErrGroupVerifyFilesExpansionFailed is returned when group verify_files expansion fails
	ErrGroupVerifyFilesExpansionFailed = errors.New("failed to expand group verify_files")
)

// ExpansionContext contains all context needed for expanding command variables.
// It groups related parameters to improve readability and maintainability.
type ExpansionContext struct {
	// Command is the command to expand
	Command *runnertypes.Command

	// Expander performs variable expansion with security checks
	Expander *environment.VariableExpander

	// AutoEnv contains automatic environment variables (e.g., __RUNNER_DATETIME, __RUNNER_PID)
	// that take precedence over Command.Env and are available for expansion.
	// If nil, an empty map is used (no automatic environment variables).
	AutoEnv map[string]string

	// EnvAllowlist is the list of system environment variables allowed for expansion
	EnvAllowlist []string

	// GroupName is the name of the command group (used for logging and error messages)
	GroupName string
}

// ExpandCommand expands variables in a single command's Cmd, Args, and Env fields,
// including automatic environment variables provided in the context.
//
// The AutoEnv in the context contains automatic environment variables that take precedence
// over Command.Env and are available for expansion:
//   - Command.Env can REFERENCE automatic variables (e.g., OUTPUT=${__RUNNER_DATETIME}.log)
//   - Command.Env CANNOT OVERRIDE automatic variables (conflicts are ignored with warning)
//   - If AutoEnv is nil, an empty map is used (no automatic environment variables)
func ExpandCommand(expCxt *ExpansionContext) (string, []string, map[string]string, error) {
	// Validate context
	if expCxt == nil {
		return "", nil, nil, ErrNilExpansionContext
	}
	if expCxt.Command == nil {
		return "", nil, nil, ErrNilCommand
	}
	if expCxt.Expander == nil {
		return "", nil, nil, ErrNilExpander
	}

	// Extract context fields
	cmd := expCxt.Command
	expander := expCxt.Expander
	allowlist := expCxt.EnvAllowlist
	groupName := expCxt.GroupName

	// Use empty map if AutoEnv is nil
	autoEnv := expCxt.AutoEnv
	if autoEnv == nil {
		autoEnv = map[string]string{}
	}

	// Expand Command.Env variables (this handles cases like PATH=/custom/bin:${PATH})
	// Pass autoEnv as baseEnv to:
	// 1. Allow Command.Env to reference automatic variables (e.g., OUTPUT=${__RUNNER_DATETIME}.log)
	// 2. Prevent Command.Env from overriding automatic variables (silently ignored with warning)
	commandEnv, err := expander.ExpandCommandEnv(cmd, groupName, allowlist, autoEnv)
	if err != nil {
		return "", nil, nil, fmt.Errorf("%w: %v", ErrCommandEnvExpansionFailed, err)
	}

	// Merge command environment with automatic environment variables
	// Auto env variables are added last, taking precedence over command env for same keys
	env := make(map[string]string, len(commandEnv)+len(autoEnv))
	maps.Copy(env, commandEnv)
	maps.Copy(env, autoEnv)

	// Expand command name
	expandedCmd, err := expander.ExpandString(cmd.Cmd, env, allowlist, groupName, make(map[string]bool))
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand command: %w", err)
	}

	// Expand command arguments
	expandedArgs, err := expander.ExpandStrings(cmd.Args, env, allowlist, groupName)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand args: %w", err)
	}

	return expandedCmd, expandedArgs, env, nil
}

// VerifyFilesExpansionError represents an error that occurred during verify_files expansion
type VerifyFilesExpansionError struct {
	Level     string   // "global" or group name
	Index     int      // verify_files array index
	Path      string   // path being expanded
	Cause     error    // root cause error
	Allowlist []string // applied allowlist
}

// Error implements the error interface
func (e *VerifyFilesExpansionError) Error() string {
	level := e.Level
	if level == "" {
		level = "global"
	}
	return fmt.Sprintf("failed to expand verify_files[%d] (%q) at %s level: %v", e.Index, e.Path, level, e.Cause)
}

// Unwrap returns the underlying cause error
func (e *VerifyFilesExpansionError) Unwrap() error {
	return e.Cause
}

// Is checks if the target error matches this error or the sentinel errors
func (e *VerifyFilesExpansionError) Is(target error) bool {
	if errors.Is(target, ErrGlobalVerifyFilesExpansionFailed) && e.Level == "" {
		return true
	}
	if errors.Is(target, ErrGroupVerifyFilesExpansionFailed) && e.Level != "" {
		return true
	}
	_, ok := target.(*VerifyFilesExpansionError)
	return ok
}

// expandVerifyFiles is a helper function that expands environment variables in verify_files paths.
// It encapsulates the common logic shared by ExpandGlobalVerifyFiles and ExpandGroupVerifyFiles.
// The envVars parameter contains Global/Group.ExpandedEnv variables that take precedence over system env.
func expandVerifyFiles(
	paths []string,
	allowlist []string,
	level string,
	envVars map[string]string, // Global/Group.ExpandedEnv variables (high priority)
	filter *environment.Filter,
	expander *environment.VariableExpander,
) ([]string, error) {
	// Handle empty verify_files
	if len(paths) == 0 {
		return []string{}, nil
	}

	// Use existing Filter.ParseSystemEnvironment() with allowlist predicate
	// Only include environment variables that are in the allowlist
	allowlistSet := make(map[string]bool, len(allowlist))
	for _, varName := range allowlist {
		allowlistSet[varName] = true
	}
	systemEnv := filter.ParseSystemEnvironment(func(varName string) bool {
		return allowlistSet[varName]
	})

	// Merge envVars (Global/Group.Env) with systemEnv (envVars takes precedence)
	combinedEnv := make(map[string]string, len(systemEnv)+len(envVars))
	maps.Copy(combinedEnv, systemEnv) // System environment variables
	maps.Copy(combinedEnv, envVars)   // Global/Group environment variables (override system)

	// Expand all paths using existing VariableExpander.ExpandString()
	expanded := make([]string, 0, len(paths))
	for i, path := range paths {
		expandedPath, err := expander.ExpandString(
			path,
			combinedEnv, // Use combined environment (Global/Group + System)
			allowlist,
			level,
			make(map[string]bool),
		)
		if err != nil {
			return nil, &VerifyFilesExpansionError{
				Level:     level,
				Index:     i,
				Path:      path,
				Cause:     err,
				Allowlist: allowlist,
			}
		}
		expanded = append(expanded, expandedPath)
	}

	return expanded, nil
}

func expandEnvMap(
	envMap map[string]string,
	combinedEnv map[string]string,
	referenceEnv map[string]string,
	allowlist []string,
	contextName string,
	expander *environment.VariableExpander,
	failureErr error,
) error {
	// Expand variables using VariableExpander
	for key, value := range envMap {
		if strings.Contains(value, "${") {
			// First try: Use reference environment for self-reference support
			expansionEnv := make(map[string]string)

			// Add reference environment (system + global + auto env, excluding current envMap)
			maps.Copy(expansionEnv, referenceEnv)

			// Add other envMap variables that have been expanded already
			for k, v := range envMap {
				if k != key { // Skip the variable currently being expanded
					expansionEnv[k] = v
				}
			}

			// Use empty visited map - we handle self-reference by excluding current variable
			visited := make(map[string]bool)
			expandedValue, err := expander.ExpandString(
				value,
				expansionEnv, // envVars: Reference environment + other expanded variables
				allowlist,    // allowlist: Effective allowlist
				contextName,  // groupName: Context name for logging
				visited,      // visited: empty for clean expansion
			)
			if err != nil {
				// If expansion failed with "variable reference not found", this might be circular reference
				// Try again with the full combinedEnv and visited map for proper circular detection
				if strings.Contains(err.Error(), "variable reference not found") {
					visited := map[string]bool{key: true}
					expandedValue, err = expander.ExpandString(
						value,
						combinedEnv, // envVars: Full combined environment
						allowlist,   // allowlist: Effective allowlist
						contextName, // groupName: Context name for logging
						visited,     // visited: mark current variable to enable circular detection
					)
				}

				if err != nil {
					return fmt.Errorf("%w: failed to expand variable %q in %s: %v",
						failureErr, key, contextName, err)
				}
			}

			envMap[key] = expandedValue

			// Update combinedEnv with the newly expanded value for subsequent expansions
			combinedEnv[key] = expandedValue
		}
	}
	return nil
}

// ExpandGlobalVerifyFiles expands environment variables in global verify_files.
// Uses existing Filter.ParseSystemEnvironment() and VariableExpander.ExpandString().
// Now supports Global.ExpandedEnv variables with higher priority than system variables.
// Returns VerifyFilesExpansionError on failure, which wraps the underlying cause.
func ExpandGlobalVerifyFiles(
	global *runnertypes.GlobalConfig,
	filter *environment.Filter,
	expander *environment.VariableExpander,
) error {
	if global == nil {
		return ErrNilConfig
	}

	expanded, err := expandVerifyFiles(
		global.VerifyFiles,
		global.EnvAllowlist,
		"",                 // Empty string indicates global level (not a group name)
		global.ExpandedEnv, // Global.ExpandedEnv variables
		filter,
		expander,
	)
	if err != nil {
		return err
	}

	global.ExpandedVerifyFiles = expanded
	return nil
}

// ExpandGroupVerifyFiles expands environment variables in group verify_files with Global.Env integration.
// Combines Group.ExpandedEnv and Global.ExpandedEnv with proper priority ordering.
// Uses existing Filter.ResolveAllowlistConfiguration() and VariableExpander.ExpandString().
// Returns VerifyFilesExpansionError on failure, which wraps the underlying cause.
//
// Variable priority for expansion: Group.ExpandedEnv > Global.ExpandedEnv > System Environment
func ExpandGroupVerifyFiles(
	group *runnertypes.CommandGroup,
	globalConfig *runnertypes.GlobalConfig,
	filter *environment.Filter,
	expander *environment.VariableExpander,
) error {
	if group == nil {
		return ErrNilConfig
	}

	// Use existing Filter.ResolveAllowlistConfiguration() for allowlist determination
	resolution := filter.ResolveAllowlistConfiguration(group.EnvAllowlist, group.Name)
	allowlist := resolution.EffectiveList

	// Merge Global.ExpandedEnv and Group.ExpandedEnv with proper priority
	// Priority: Group.ExpandedEnv > Global.ExpandedEnv
	combinedEnv := make(map[string]string)

	// Start with global environment as base
	if globalConfig != nil && globalConfig.ExpandedEnv != nil {
		maps.Copy(combinedEnv, globalConfig.ExpandedEnv)
	}

	// Add group environment variables (higher priority)
	if group.ExpandedEnv != nil {
		maps.Copy(combinedEnv, group.ExpandedEnv)
	}

	expanded, err := expandVerifyFiles(
		group.VerifyFiles,
		allowlist,
		group.Name,
		combinedEnv, // Combined environment: Group.ExpandedEnv + Global.ExpandedEnv
		filter,
		expander,
	)
	if err != nil {
		return err
	}

	group.ExpandedVerifyFiles = expanded
	return nil
}

// ExpandGlobalEnv expands environment variables in Global.Env.
// This function validates the environment variable format, checks for duplicates,
// and expands variables using the existing VariableExpander.
//
// The function follows these steps:
// 1. Input validation: returns nil if cfg.Env is nil or empty
// 2. Parse and validate each KEY=VALUE entry
// 3. Check for duplicate keys
// 4. Validate KEY names using existing security validators
// 5. Expand variables using VariableExpander.ExpandString()
// 6. Store results in cfg.ExpandedEnv
//
// Variable resolution order within Global.Env:
// - Automatic environment variables (__RUNNER_PID, __RUNNER_DATETIME)
// - Global.Env variables (same level references)
// - System environment variables (filtered by allowlist)
//
// Self-reference (e.g., PATH=/custom:${PATH}) is supported by referencing
// the system environment variable, not the partially expanded value.
func ExpandGlobalEnv(
	cfg *runnertypes.GlobalConfig,
	expander *environment.VariableExpander,
	autoEnv map[string]string,
) error {
	// Input validation: nil or empty env list
	if cfg == nil {
		return ErrNilConfig
	}
	if len(cfg.Env) == 0 {
		cfg.ExpandedEnv = nil
		return nil
	}

	// Validate and parse environment variables in a single pass
	envMap, err := validateAndParseEnvList(cfg.Env, "global.env")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGlobalEnvExpansionFailed, err)
	}

	// Create combined environment for variable resolution
	// Priority: Automatic environment variables > Global.Env variables > System environment variables

	// Start with system environment variables (filtered by allowlist)
	filter := environment.NewFilter(cfg.EnvAllowlist)
	systemEnv := filter.ParseSystemEnvironment(func(varName string) bool {
		for _, allowed := range cfg.EnvAllowlist {
			if varName == allowed {
				return true
			}
		}
		return false
	})

	combinedEnv := make(map[string]string, len(systemEnv)+len(envMap)+len(autoEnv))
	maps.Copy(combinedEnv, systemEnv) // System environment variables as base

	// Save reference environment before adding global variables (for self-reference resolution)
	referenceEnv := make(map[string]string)
	maps.Copy(referenceEnv, combinedEnv)
	if autoEnv != nil {
		maps.Copy(referenceEnv, autoEnv) // Include automatic environment variables in reference
	}

	maps.Copy(combinedEnv, envMap) // Global.Env variables (higher priority)
	if autoEnv != nil {
		maps.Copy(combinedEnv, autoEnv) // Automatic environment variables (highest priority)
	}

	// Expand variables using common helper with reference environment
	if err := expandEnvMap(
		envMap,
		combinedEnv,      // combinedEnv: Combined environment (Global.Env + Automatic)
		referenceEnv,     // referenceEnv: System + Automatic environment (for self-reference)
		cfg.EnvAllowlist, // allowlist: Global allowlist
		"global.env",     // contextName: Context for error messages
		expander,
		ErrGlobalEnvExpansionFailed,
	); err != nil {
		return err
	}

	// Store expanded environment variables
	cfg.ExpandedEnv = envMap
	return nil
}

// ExpandGroupEnv expands environment variables in Group.Env with references to Global.Env and system environment.
//
// The expansion follows these rules:
// 1. Group.Env variables can reference automatic environment variables (__RUNNER_PID, __RUNNER_DATETIME)
// 2. Group.Env variables can reference Global.ExpandedEnv variables
// 3. Group.Env variables can reference system environment variables (subject to allowlist)
// 4. Priority: Group.Env > Automatic Environment > Global.ExpandedEnv > System Environment
// 5. Self-reference (e.g., PATH=/custom:${PATH}) is supported
// 6. Allowlist inheritance: if group.EnvAllowlist == nil, inherit from globalAllowlist
//
// Parameters:
//   - group: The command group containing environment variables to expand
//   - globalEnv: The already expanded global environment variables (Global.ExpandedEnv)
//   - globalAllowlist: The global environment variable allowlist
//   - expander: The variable expander for performing secure expansion
//   - autoEnv: The automatic environment variables (__RUNNER_PID, __RUNNER_DATETIME)
//
// Returns:
//   - error: Any error that occurred during expansion
func ExpandGroupEnv(
	group *runnertypes.CommandGroup,
	globalEnv map[string]string,
	globalAllowlist []string,
	expander *environment.VariableExpander,
	autoEnv map[string]string,
) error {
	// Input validation
	if group == nil {
		return ErrNilGroup
	}
	if expander == nil {
		return ErrNilExpander
	}

	// Handle nil or empty group env
	if len(group.Env) == 0 {
		group.ExpandedEnv = map[string]string{}
		return nil
	}

	// Determine effective allowlist using allowlist inheritance rules
	effectiveAllowlist := determineEffectiveAllowlist(group, &runnertypes.GlobalConfig{EnvAllowlist: globalAllowlist})

	// Validate and parse environment variables in a single pass
	envMap, err := validateAndParseEnvList(group.Env, fmt.Sprintf("group.env:%s", group.Name))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGroupEnvExpansionFailed, err)
	}

	// Create combined environment for variable resolution
	// Priority: Group.Env (envMap) > Automatic Environment > Global.ExpandedEnv > System Environment

	// Start with system environment variables (filtered by effective allowlist)
	filter := environment.NewFilter(effectiveAllowlist)
	systemEnv := filter.ParseSystemEnvironment(func(varName string) bool {
		for _, allowed := range effectiveAllowlist {
			if varName == allowed {
				return true
			}
		}
		return false
	})

	combinedEnv := make(map[string]string)
	maps.Copy(combinedEnv, systemEnv) // System environment variables as base

	// Add global environment variables (higher priority than system)
	if globalEnv != nil {
		maps.Copy(combinedEnv, globalEnv)
	}

	// Add automatic environment variables (higher priority than global)
	if autoEnv != nil {
		maps.Copy(combinedEnv, autoEnv)
	}

	// Save reference environment before adding group variables (for self-reference resolution)
	referenceEnv := make(map[string]string)
	maps.Copy(referenceEnv, combinedEnv)

	// Add group environment variables (highest priority)
	maps.Copy(combinedEnv, envMap)

	// Expand variables using common helper
	contextName := fmt.Sprintf("group.env:%s", group.Name)
	if err := expandEnvMap(
		envMap,
		combinedEnv,        // combinedEnv: Combined environment (Group + Global)
		referenceEnv,       // referenceEnv: Environment without group variables (for self-reference)
		effectiveAllowlist, // allowlist: Effective allowlist (inherited or overridden)
		contextName,        // contextName: Context for error messages
		expander,
		ErrGroupEnvExpansionFailed,
	); err != nil {
		return err
	}

	// Store expanded environment variables (only Group-level variables)
	group.ExpandedEnv = envMap
	return nil
}
