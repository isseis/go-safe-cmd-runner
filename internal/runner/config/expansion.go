// Package config provides configuration management and variable expansion for commands.
package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
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

	// GlobalEnv contains expanded global environment variables (Global.ExpandedEnv)
	// that Command.Env can reference. If nil, an empty map is used.
	GlobalEnv map[string]string

	// GroupEnv contains expanded group environment variables (Group.ExpandedEnv)
	// that Command.Env can reference. If nil, an empty map is used.
	GroupEnv map[string]string

	// EnvAllowlist is the effective allowlist of system environment variables allowed for expansion.
	// This field should contain the allowlist after inheritance has been resolved
	// (e.g., via DetermineEffectiveAllowlist: group.EnvAllowlist ?? global.EnvAllowlist).
	EnvAllowlist []string

	// Group is the command group containing the command (used for logging and allowlist access)
	Group *runnertypes.CommandGroup
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
	if expCxt.Group == nil {
		return "", nil, nil, ErrNilGroup
	}

	// Extract context fields
	cmd := expCxt.Command
	expander := expCxt.Expander
	allowlist := expCxt.EnvAllowlist
	group := expCxt.Group

	// Use empty map if AutoEnv is nil
	autoEnv := expCxt.AutoEnv
	if autoEnv == nil {
		autoEnv = map[string]string{}
	}

	// Use empty map if GlobalEnv is nil
	globalEnv := expCxt.GlobalEnv
	if globalEnv == nil {
		globalEnv = map[string]string{}
	}

	// Use empty map if GroupEnv is nil
	groupEnv := expCxt.GroupEnv
	if groupEnv == nil {
		groupEnv = map[string]string{}
	}

	// Expand Command.Env variables (this handles cases like PATH=/custom/bin:${PATH})
	// Pass autoEnv to:
	// 1. Allow Command.Env to reference automatic variables (e.g., OUTPUT=${__RUNNER_DATETIME}.log)
	// 2. Prevent Command.Env from overriding automatic variables (silently ignored with warning)
	// Also pass globalEnv and groupEnv so Command.Env can reference those variables
	// Note: allowlist inheritance (group.EnvAllowlist ?? globalAllowlist) is handled internally in ExpandCommandEnv
	if err := ExpandCommandEnv(cmd, group, allowlist, expander, globalEnv, groupEnv, autoEnv); err != nil {
		return "", nil, nil, fmt.Errorf("%w: %v", ErrCommandEnvExpansionFailed, err)
	}

	// Merge command environment with automatic environment variables
	// Auto env variables are added last, taking precedence over command env for same keys
	env := make(map[string]string, len(cmd.ExpandedEnv)+len(autoEnv))
	maps.Copy(env, cmd.ExpandedEnv)
	maps.Copy(env, autoEnv)

	// Expand command name
	expandedCmd, err := expander.ExpandString(cmd.Cmd, env, allowlist, group.Name, make(map[string]bool))
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand command: %w", err)
	}

	// Expand arguments
	expandedArgs, err := expander.ExpandStrings(cmd.Args, env, allowlist, group.Name)
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
	referenceEnv map[string]string,
	highPriorityEnv map[string]string,
	allowlist []string,
	contextName string,
	expander *environment.VariableExpander,
	failureErr error,
) error {
	// Expand variables using VariableExpander with two-pass approach:
	// 1. First pass: Try expansion with current variable excluded (for self-reference support)
	// 2. Second pass (on "not found" error): Try with current variable marked as visited (for circular detection)
	for key, value := range envMap {
		if strings.Contains(value, "${") {
			// First pass: Support self-reference (e.g., PATH=${PATH}:/new)
			// Construct environment excluding the current variable, so ${PATH} resolves
			// to the value from reference environment (global/group/system)
			tempEnv := make(map[string]string, len(referenceEnv)+len(envMap)+len(highPriorityEnv))
			maps.Copy(tempEnv, referenceEnv)
			for k, v := range envMap {
				if k != key {
					tempEnv[k] = v
				}
			}
			if highPriorityEnv != nil {
				maps.Copy(tempEnv, highPriorityEnv)
			}

			expandedValue, err := expander.ExpandString(
				value,
				tempEnv,
				allowlist,
				contextName,
				make(map[string]bool), // Empty visited map for first pass
			)
			if err != nil {
				// If first pass failed with "variable reference not found", this might be
				// a circular reference (e.g., VAR1=${VAR2}, VAR2=${VAR1}).
				// Try second pass with full environment and visited map for circular detection.
				if strings.Contains(err.Error(), "variable reference not found") {
					fullEnv := make(map[string]string, len(referenceEnv)+len(envMap)+len(highPriorityEnv))
					maps.Copy(fullEnv, referenceEnv)
					maps.Copy(fullEnv, envMap)
					if highPriorityEnv != nil {
						maps.Copy(fullEnv, highPriorityEnv)
					}

					visited := map[string]bool{key: true} // Mark current variable as visited
					expandedValue, err = expander.ExpandString(
						value,
						fullEnv,
						allowlist,
						contextName,
						visited,
					)
				}

				if err != nil {
					return fmt.Errorf("%w: failed to expand variable %q in %s: %w",
						failureErr, key, contextName, err)
				}
			}

			envMap[key] = expandedValue
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

// expandEnvironment is a generic helper function to expand environment variables
// for global, group, and command levels. It centralizes the logic for parsing,
// validating, and expanding environment variables, while allowing for level-specific
// configurations through the expansionParameters struct.
//
// The function performs the following steps:
// 1. Parses and validates the input environment variable list (e.g., ["KEY=VALUE"]).
// 2. Filters out variables that conflict with a high-priority base environment (if provided).
// 3. Validates variable names against reserved prefixes.
// 4. Constructs a combined environment for expansion, respecting priority order:
//
//   - High-priority base environment (e.g., auto-env)
//
//   - Current level's environment variables
//
//   - Reference environments (e.g., global.env, system env)
//
//     5. Expands variables using expandEnvMap, which handles self-references by marking
//     the current variable as visited in the VariableExpander.
//     6. Performs a final validation on the expanded values for security.
func expandEnvironment(params expansionParameters) (map[string]string, error) {
	// 1. Handle nil or empty env list
	if len(params.envList) == 0 {
		return make(map[string]string), nil
	}

	// 2. Parse environment variables (without full validation yet)
	envMap := make(map[string]string)
	for _, envVar := range params.envList {
		key, value, ok := common.ParseEnvVariable(envVar)
		if !ok {
			return nil, fmt.Errorf("%w: %w: %q in %s", params.failureErr, ErrMalformedEnvVariable, envVar, params.contextName)
		}
		if _, exists := envMap[key]; exists {
			return nil, fmt.Errorf("%w: %w: duplicate key %q in %s", params.failureErr, ErrDuplicateEnvVariable, key, params.contextName)
		}
		envMap[key] = value
	}

	// 3. Filter out variables that conflict with the high-priority base environment
	// This is primarily for command.env to prevent overriding auto-env variables.
	if params.highPriorityBaseEnv != nil {
		for key := range envMap {
			if _, exists := params.highPriorityBaseEnv[key]; exists {
				// Log the conflict if a logger is provided (optional)
				// Note: Conflicting variables are silently ignored as a security measure.
				delete(envMap, key)
			}
		}
	}

	// 4. Validate variable names against reserved prefixes (e.g., "__RUNNER_") now that
	// conflicting auto-env vars have been removed.
	if err := environment.ValidateUserEnvNames(envMap); err != nil {
		return nil, fmt.Errorf("%w: %w in %s: %w", params.failureErr, ErrReservedEnvPrefix, params.contextName, err)
	}
	for key := range envMap {
		if err := security.ValidateVariableName(key); err != nil {
			return nil, fmt.Errorf("%w: %w in %s: %w", params.failureErr, ErrInvalidEnvKey, params.contextName, err)
		}
	}

	// 5. Construct the combined environment for expansion
	// The combined environment includes all reference environments (system, global, group, auto)
	// plus the current level's envMap. Priority: highPriorityBaseEnv > envMap > referenceEnvs
	referenceEnv := make(map[string]string)
	for _, ref := range params.referenceEnvs {
		if ref != nil {
			maps.Copy(referenceEnv, ref)
		}
	}

	combinedEnv := make(map[string]string)
	maps.Copy(combinedEnv, referenceEnv)
	maps.Copy(combinedEnv, envMap)
	if params.highPriorityBaseEnv != nil {
		maps.Copy(combinedEnv, params.highPriorityBaseEnv)
	}

	// 6. Expand variables using expandEnvMap
	// Self-references (e.g., PATH=${PATH}:/new) are handled by temporarily excluding
	// the current variable from envMap during expansion, allowing ${PATH} to resolve
	// to the value in the reference environment.
	if err := expandEnvMap(
		envMap,
		referenceEnv,
		params.highPriorityBaseEnv,
		params.allowlist,
		params.contextName,
		params.expander,
		params.failureErr,
	); err != nil {
		return nil, err
	}

	// 7. Final validation on expanded values
	validator, err := security.NewValidator(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create validator: %v", params.failureErr, err)
	}
	for name, value := range envMap {
		if err := validator.ValidateEnvironmentValue(name, value); err != nil {
			return nil, fmt.Errorf("%w: validation failed for expanded variable %s in %s: %w", params.failureErr, name, params.contextName, err)
		}
	}

	return envMap, nil
}

// expansionParameters holds all the necessary information for the expandEnvironment function.
type expansionParameters struct {
	envList             []string
	contextName         string
	allowlist           []string
	referenceEnvs       []map[string]string
	highPriorityBaseEnv map[string]string
	expander            *environment.VariableExpander
	failureErr          error
}

// buildExpansionParams creates expansionParameters for environment variable expansion.
// This centralizes the common logic shared by ExpandGlobalEnv, ExpandGroupEnv, and ExpandCommandEnv.
//
// Parameters:
//   - envList: The list of environment variables to expand (e.g., Global.Env, Group.Env, Command.Env)
//   - contextName: Context name for logging (e.g., "global.env", "group.env:deploy")
//   - allowlist: Effective allowlist for this expansion
//   - globalEnv: Global.ExpandedEnv (nil if not applicable)
//   - groupEnv: Group.ExpandedEnv (nil if not applicable)
//   - autoEnv: Automatic environment variables (nil if not applicable)
//   - expander: Variable expander instance
//   - failureErr: Error to return on failure
func buildExpansionParams(
	envList []string,
	contextName string,
	allowlist []string,
	globalEnv map[string]string,
	groupEnv map[string]string,
	autoEnv map[string]string,
	expander *environment.VariableExpander,
	failureErr error,
) expansionParameters {
	// Filter system environment based on the allowlist
	filter := environment.NewFilter(allowlist)
	systemEnv := filter.ParseSystemEnvironment(func(varName string) bool {
		return slices.Contains(allowlist, varName)
	})

	// Build reference environments in priority order (lower index = lower priority)
	// Priority: groupEnv > globalEnv > autoEnv > systemEnv
	var referenceEnvs []map[string]string
	referenceEnvs = append(referenceEnvs, systemEnv)
	if autoEnv != nil {
		referenceEnvs = append(referenceEnvs, autoEnv)
	}
	if globalEnv != nil {
		referenceEnvs = append(referenceEnvs, globalEnv)
	}
	if groupEnv != nil {
		referenceEnvs = append(referenceEnvs, groupEnv)
	}

	return expansionParameters{
		envList:             envList,
		contextName:         contextName,
		allowlist:           allowlist,
		referenceEnvs:       referenceEnvs,
		highPriorityBaseEnv: autoEnv, // autoEnv always takes precedence
		expander:            expander,
		failureErr:          failureErr,
	}
}

// expandEnvInternal is an internal helper function that unifies the environment variable
// expansion logic for Global.Env, Group.Env, and Command.Env.
//
// This function centralizes the common expansion logic and allowlist inheritance calculation,
// reducing code duplication and improving maintainability.
//
// Parameters:
//   - envList: The list of environment variables to expand (e.g., cfg.Env, group.Env, cmd.Env)
//   - contextName: A descriptive name for error messages (e.g., "global.env", "group.env:mygroup")
//   - localAllowlist: The local-level allowlist (Global/Group/Command level)
//   - globalAllowlist: The global allowlist for inheritance calculation (nil for Global level)
//   - globalEnv: The expanded Global.Env for reference (nil for Global level)
//   - groupEnv: The expanded Group.Env for reference (nil for Global/Group level)
//   - autoEnv: Automatic environment variables (__RUNNER_DATETIME, __RUNNER_PID)
//   - expander: The variable expander for performing secure expansion
//   - failureErr: The sentinel error to wrap on failure
//   - outputTarget: Pointer to the field where the expanded result should be stored
//
// Allowlist inheritance rules:
//   - If localAllowlist is nil and globalAllowlist is not nil, inherit globalAllowlist
//   - Otherwise, use localAllowlist (which may be nil, empty slice, or populated)
func expandEnvInternal(
	envList []string,
	contextName string,
	localAllowlist []string,
	globalAllowlist []string,
	globalEnv map[string]string,
	groupEnv map[string]string,
	autoEnv map[string]string,
	expander *environment.VariableExpander,
	failureErr error,
	outputTarget *map[string]string,
) error {
	// Determine the effective allowlist with inheritance
	effectiveAllowlist := localAllowlist
	if effectiveAllowlist == nil && globalAllowlist != nil {
		effectiveAllowlist = globalAllowlist
	}

	// Build expansion parameters
	params := buildExpansionParams(
		envList,
		contextName,
		effectiveAllowlist,
		globalEnv,
		groupEnv,
		autoEnv,
		expander,
		failureErr,
	)

	// Call the generic expansion function
	expandedEnv, err := expandEnvironment(params)
	if err != nil {
		return err
	}

	// Store the expanded environment in the target
	*outputTarget = expandedEnv
	return nil
}

// ExpandGlobalEnv expands environment variables in Global.Env.
func ExpandGlobalEnv(
	cfg *runnertypes.GlobalConfig,
	expander *environment.VariableExpander,
	autoEnv map[string]string,
) error {
	// Input validation
	if cfg == nil {
		return ErrNilConfig
	}
	if expander == nil {
		return ErrNilExpander
	}

	return expandEnvInternal(
		cfg.Env,                     // envList
		"global.env",                // contextName
		cfg.EnvAllowlist,            // localAllowlist
		nil,                         // globalAllowlist (no inheritance at global level)
		nil,                         // globalEnv (self-expansion)
		nil,                         // groupEnv (not applicable)
		autoEnv,                     // autoEnv
		expander,                    // expander
		ErrGlobalEnvExpansionFailed, // failureErr
		&cfg.ExpandedEnv,            // outputTarget
	)
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

	return expandEnvInternal(
		group.Env,                               // envList
		fmt.Sprintf("group.env:%s", group.Name), // contextName
		group.EnvAllowlist,                      // localAllowlist
		globalAllowlist,                         // globalAllowlist (for inheritance)
		globalEnv,                               // globalEnv (Global.ExpandedEnv)
		nil,                                     // groupEnv (self-expansion)
		autoEnv,                                 // autoEnv
		expander,                                // expander
		ErrGroupEnvExpansionFailed,              // failureErr
		&group.ExpandedEnv,                      // outputTarget
	)
}

// ExpandCommandEnv expands Command.Env variables with reference to global, group, and automatic environment variables.
// This is used during configuration loading (Phase 1) to pre-expand Command.Env.
//
// Variable reference priority (what Command.Env can reference):
//  1. Group.ExpandedEnv variables (groupEnv parameter)
//  2. Global.ExpandedEnv variables (globalEnv parameter)
//  3. Automatic variables (__RUNNER_DATETIME, __RUNNER_PID) (autoEnv parameter)
//  4. System environment variables (subject to allowlist)
//
// Variable override priority (what takes precedence in final result):
//   - autoEnv > Command.Env (Command.Env CANNOT override automatic variables)
//   - Variables from Command.Env that conflict with autoEnv are silently ignored with a warning log
//
// Allowlist inheritance:
//   - Uses group.EnvAllowlist if defined, otherwise inherits from globalAllowlist
//
// Parameters:
//   - cmd: The command containing environment variables to expand
//   - group: The command group containing the command (for logging and allowlist access)
//   - globalAllowlist: The global environment variable allowlist (used for inheritance if group.EnvAllowlist is nil)
//   - expander: The variable expander for performing secure expansion
//   - globalEnv: Global.ExpandedEnv variables that Command.Env can reference; can be nil or empty
//   - groupEnv: Group.ExpandedEnv variables that Command.Env can reference; can be nil or empty
//   - autoEnv: Automatic environment variables (__RUNNER_DATETIME, __RUNNER_PID); can be nil or empty in tests
//
// Returns:
//   - error: Any error that occurred during expansion
func ExpandCommandEnv(
	cmd *runnertypes.Command,
	group *runnertypes.CommandGroup,
	globalAllowlist []string,
	expander *environment.VariableExpander,
	globalEnv map[string]string,
	groupEnv map[string]string,
	autoEnv map[string]string,
) error {
	// Input validation
	if cmd == nil {
		return ErrNilCommand
	}
	if group == nil {
		return ErrNilGroup
	}
	if expander == nil {
		return ErrNilExpander
	}

	return expandEnvInternal(
		cmd.Env, // envList
		fmt.Sprintf("command.env:%s (group:%s)", cmd.Name, group.Name), // contextName
		group.EnvAllowlist,           // localAllowlist (group's allowlist for inheritance calculation)
		globalAllowlist,              // globalAllowlist (for inheritance when group.EnvAllowlist is nil)
		globalEnv,                    // globalEnv (Global.ExpandedEnv)
		groupEnv,                     // groupEnv (Group.ExpandedEnv)
		autoEnv,                      // autoEnv
		expander,                     // expander
		ErrCommandEnvExpansionFailed, // failureErr
		&cmd.ExpandedEnv,             // outputTarget
	)
}
