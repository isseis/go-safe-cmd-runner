// Package config provides configuration management and variable expansion for commands.
package config

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

const (
	// MaxRecursionDepth is the maximum depth for variable expansion to prevent stack overflow
	MaxRecursionDepth = 100
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

	// GlobalEnvAllowlist is the global allowlist of system environment variables.
	// This should be Global.EnvAllowlist. The allowlist inheritance logic
	// (groupEnvAllowlist ?? globalEnvAllowlist) is handled internally in ExpandCommand.
	GlobalEnvAllowlist []string

	// GroupName is the name of the command group (used for logging context)
	GroupName string

	// GroupEnvAllowlist is the group's environment variable allowlist.
	// This should be Group.EnvAllowlist. If nil, GlobalEnvAllowlist is inherited.
	GroupEnvAllowlist []string
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
	globalAllowlist := expCxt.GlobalEnvAllowlist
	groupName := expCxt.GroupName
	groupEnvAllowlist := expCxt.GroupEnvAllowlist

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
	// Note: allowlist inheritance (groupEnvAllowlist ?? globalAllowlist) is handled internally in ExpandCommandEnv
	if err := ExpandCommandEnv(cmd, expander, autoEnv, globalEnv, globalAllowlist, groupEnv, groupEnvAllowlist, groupName); err != nil {
		return "", nil, nil, fmt.Errorf("%w: %v", ErrCommandEnvExpansionFailed, err)
	}

	// Determine effective allowlist for command name and args expansion
	// Use filter.ResolveAllowlistConfiguration to centralize allowlist inheritance logic
	filter := environment.NewFilter(globalAllowlist)
	resolution := filter.ResolveAllowlistConfiguration(groupEnvAllowlist, groupName)
	effectiveAllowlist := resolution.GetEffectiveList()

	// Merge command environment with global, group, and automatic environment variables
	// Priority order: autoEnv > cmd.ExpandedEnv > groupEnv > globalEnv
	// Auto env variables are added last, taking precedence over all other variables for same keys
	env := make(map[string]string, len(globalEnv)+len(groupEnv)+len(cmd.ExpandedEnv)+len(autoEnv))
	maps.Copy(env, globalEnv)
	maps.Copy(env, groupEnv)
	maps.Copy(env, cmd.ExpandedEnv)
	maps.Copy(env, autoEnv)

	// Expand command name
	expandedCmd, err := expander.ExpandString(cmd.Cmd, env, effectiveAllowlist, groupName, make(map[string]bool))
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand command: %w", err)
	}

	// Expand arguments
	expandedArgs, err := expander.ExpandStrings(cmd.Args, env, effectiveAllowlist, groupName)
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

// buildExpansionEnv constructs an expansion environment by merging reference, current, and auto generated environments.
// If excludeKey is not empty, that key is excluded from envMap to support self-reference.
func buildExpansionEnv(envMap, autoEnv, referenceEnv map[string]string, excludeKey string) map[string]string {
	result := make(map[string]string, len(referenceEnv)+len(envMap)+len(autoEnv))
	maps.Copy(result, referenceEnv)
	for k, v := range envMap {
		if k != excludeKey {
			result[k] = v
		}
	}
	if autoEnv != nil {
		maps.Copy(result, autoEnv)
	}
	return result
}

// tryExpandVariable attempts to expand a variable value using a two-pass approach:
// 1. First pass: Exclude current key to support self-reference (e.g., PATH=${PATH}:/new)
// 2. Second pass: Include current key with visited mark to detect circular references
func tryExpandVariable(
	key, value string,
	envMap map[string]string,
	contextName string,
	expander *environment.VariableExpander,
	autoEnv, referenceEnv map[string]string,
	allowlist []string,
) (string, error) {
	// First pass: Try with current variable excluded (supports self-reference)
	tempEnv := buildExpansionEnv(envMap, autoEnv, referenceEnv, key)
	expandedValue, err := expander.ExpandString(value, tempEnv, allowlist, contextName, make(map[string]bool))

	// If first pass succeeded, return result
	if err == nil {
		return expandedValue, nil
	}

	// If error is not ErrVariableNotFound, return error immediately
	if !errors.Is(err, environment.ErrVariableNotFound) {
		return "", err
	}

	// Second pass: Try with full environment and visited map (detects circular references)
	fullEnv := buildExpansionEnv(envMap, autoEnv, referenceEnv, "")
	visited := map[string]bool{key: true}
	return expander.ExpandString(value, fullEnv, allowlist, contextName, visited)
}

func expandEnvMap(
	envMap map[string]string,
	contextName string,
	expander *environment.VariableExpander,
	autoEnv map[string]string,
	referenceEnv map[string]string,
	allowlist []string,
	failureErr error,
) error {
	for key, value := range envMap {
		// Skip variables without expansion syntax
		if !strings.Contains(value, "${") {
			continue
		}

		expandedValue, err := tryExpandVariable(key, value, envMap, contextName, expander, autoEnv, referenceEnv, allowlist)
		if err != nil {
			// Build context information for error message
			availableVarsCount := len(envMap) + len(referenceEnv)
			if autoEnv != nil {
				availableVarsCount += len(autoEnv)
			}
			allowlistInfo := "none"
			if len(allowlist) > 0 {
				allowlistInfo = fmt.Sprintf("%d vars", len(allowlist))
			}

			return fmt.Errorf("%w: failed to expand variable %q in %s (available: %d vars, allowlist: %s): %w",
				failureErr, key, contextName, availableVarsCount, allowlistInfo, err)
		}

		envMap[key] = expandedValue
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
	allowlist := resolution.GetEffectiveList()

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
//  1. Parses and validates the input environment variable list (e.g., ["KEY=VALUE"]).
//  2. Filters out variables that conflict with a high-priority base environment (if provided).
//  3. Validates variable names against reserved prefixes.
//  4. Constructs a combined environment for expansion, respecting priority order:
//     a. autoEnv (highest priority, cannot be overridden)
//     b. Current level's environment variables
//     c. Reference environments (e.g., global.env, system env)
//  5. Expands variables using expandEnvMap, which handles self-references by marking
//     the current variable as visited in the VariableExpander.
//  6. Performs a final validation on the expanded values for security.
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
		if existingValue, exists := envMap[key]; exists {
			// Provide detailed error message with both definitions
			return nil, fmt.Errorf("%w: %w: duplicate environment variable definition %q in %s (first: %q, duplicate: %q)",
				params.failureErr, ErrDuplicateEnvVariable, key, params.contextName,
				fmt.Sprintf("%s=%s", key, existingValue), fmt.Sprintf("%s=%s", key, value))
		}
		envMap[key] = value
	}

	// 3. Filter out variables that conflict with the high-priority base environment
	// This is primarily for command.env to prevent overriding auto-env variables.
	if params.autoEnv != nil {
		for key := range envMap {
			if _, exists := params.autoEnv[key]; exists {
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
	if params.autoEnv != nil {
		maps.Copy(combinedEnv, params.autoEnv)
	}

	// 6. Expand variables using expandEnvMap
	// Self-references (e.g., PATH=${PATH}:/new) are handled by temporarily excluding
	// the current variable from envMap during expansion, allowing ${PATH} to resolve
	// to the value in the reference environment.
	if err := expandEnvMap(
		envMap,
		params.contextName,
		params.expander,
		params.autoEnv,
		referenceEnv,
		params.allowlist,
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
	envList       []string
	contextName   string
	expander      *environment.VariableExpander
	autoEnv       map[string]string
	referenceEnvs []map[string]string
	allowlist     []string
	failureErr    error
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
	outputTarget *map[string]string,
	expander *environment.VariableExpander,
	autoEnv map[string]string,
	globalEnv map[string]string,
	globalAllowlist []string,
	groupEnv map[string]string,
	localAllowlist []string,
	failureErr error,
) error {
	// Create filter with global allowlist for resolution
	filter := environment.NewFilter(globalAllowlist)

	// Use filter.ResolveAllowlistConfiguration to determine the effective allowlist
	// This centralizes the allowlist inheritance logic
	resolution := filter.ResolveAllowlistConfiguration(localAllowlist, contextName)
	effectiveAllowlist := resolution.GetEffectiveList()

	// Filter system environment based on the resolved allowlist
	// Use the resolution's IsAllowed method for consistent filtering
	systemEnv := filter.ParseSystemEnvironment(func(varName string) bool {
		return resolution.IsAllowed(varName)
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

	params := expansionParameters{
		envList:       envList,
		contextName:   contextName,
		expander:      expander,
		autoEnv:       autoEnv, // autoEnv always takes precedence
		referenceEnvs: referenceEnvs,
		allowlist:     effectiveAllowlist,
		failureErr:    failureErr,
	}

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
		&cfg.ExpandedEnv,            // outputTarget
		expander,                    // expander
		autoEnv,                     // autoEnv
		nil,                         // globalEnv (self-expansion)
		nil,                         // globalAllowlist (no inheritance at global level)
		nil,                         // groupEnv (not applicable)
		cfg.EnvAllowlist,            // localAllowlist (Global.EnvAllowlist)
		ErrGlobalEnvExpansionFailed, // failureErr
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
	expander *environment.VariableExpander,
	autoEnv map[string]string,
	globalEnv map[string]string,
	globalAllowlist []string,
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
		&group.ExpandedEnv,                      // outputTarget
		expander,                                // expander
		autoEnv,                                 // autoEnv
		globalEnv,                               // globalEnv (Global.ExpandedEnv)
		globalAllowlist,                         // globalAllowlist (for inheritance)
		nil,                                     // groupEnv (self-expansion)
		group.EnvAllowlist,                      // localAllowlist (Group.EnvAllowlist)
		ErrGroupEnvExpansionFailed,              // failureErr
	)
}

// ExpandCommandEnv expands Command.Env variables with reference to global, group, and automatic environment variables.
// This is used during configuration loading to pre-expand Command.Env.
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
//   - groupName: The name of the command group (for logging context)
//   - groupEnvAllowlist: The group's environment variable allowlist (for inheritance calculation)
//   - globalAllowlist: The global environment variable allowlist (used for inheritance if groupEnvAllowlist is nil)
//   - expander: The variable expander for performing secure expansion
//   - globalEnv: Global.ExpandedEnv variables that Command.Env can reference; can be nil or empty
//   - groupEnv: Group.ExpandedEnv variables that Command.Env can reference; can be nil or empty
//   - autoEnv: Automatic environment variables (__RUNNER_DATETIME, __RUNNER_PID); can be nil or empty in tests
//
// Returns:
//   - error: Any error that occurred during expansion
func ExpandCommandEnv(
	cmd *runnertypes.Command,
	expander *environment.VariableExpander,
	autoEnv map[string]string,
	globalEnv map[string]string,
	globalAllowlist []string,
	groupEnv map[string]string,
	groupAllowlist []string,
	groupName string,
) error {
	// Input validation
	if cmd == nil {
		return ErrNilCommand
	}
	if expander == nil {
		return ErrNilExpander
	}

	return expandEnvInternal(
		cmd.Env, // envList
		fmt.Sprintf("command.env:%s (group:%s)", cmd.Name, groupName), // contextName
		&cmd.ExpandedEnv,             // outputTarget
		expander,                     // expander
		autoEnv,                      // autoEnv
		globalEnv,                    // globalEnv (Global.ExpandedEnv)
		globalAllowlist,              // globalAllowlist (for inheritance when localAllowlist is nil)
		groupEnv,                     // groupEnv (Group.ExpandedEnv)
		groupAllowlist,               // localAllowlist (group's allowlist for command-level expansion)
		ErrCommandEnvExpansionFailed, // failureErr
	)
}

// ============================================================================
// Phase 2: Internal Variable Expander (%{VAR} syntax)
// ============================================================================

// ExpandString expands %{VAR} references in a string using the provided
// internal variables. It detects circular references and reports detailed
// errors. The function is package-level (stateless) and follows Go conventions.
func ExpandString(
	input string,
	expandedVars map[string]string,
	level string,
	field string,
) (string, error) {
	visited := make(map[string]bool)
	return expandStringRecursive(input, expandedVars, level, field, visited, nil, 0)
}

// expandStringRecursive performs recursive expansion with circular reference detection.
func expandStringRecursive(
	input string,
	expandedVars map[string]string,
	level string,
	field string,
	visited map[string]bool,
	expansionChain []string,
	depth int,
) (string, error) {
	// Check recursion depth to prevent stack overflow
	if depth >= MaxRecursionDepth {
		return "", &ErrMaxRecursionDepthExceededDetail{
			Level:    level,
			Field:    field,
			MaxDepth: MaxRecursionDepth,
			Context:  input,
		}
	}

	var result strings.Builder
	i := 0

	for i < len(input) {
		// Handle escape sequences
		if input[i] == '\\' && i+1 < len(input) {
			next := input[i+1]
			switch next {
			case '%':
				result.WriteByte('%')
				i += 2
				continue
			case '\\':
				result.WriteByte('\\')
				i += 2
				continue
			default:
				// Invalid escape sequence
				return "", &ErrInvalidEscapeSequenceDetail{
					Level:    level,
					Field:    field,
					Sequence: input[i : i+2],
					Context:  input,
				}
			}
		}

		// Handle %{VAR} expansion
		if input[i] == '%' && i+1 < len(input) && input[i+1] == '{' {
			// Find the closing '}'
			const openBraceLen = 2 // Length of "%{"
			closeIdx := strings.IndexByte(input[i+openBraceLen:], '}')
			if closeIdx == -1 {
				// Unclosed %{ - return unclosed variable reference error
				return "", &ErrUnclosedVariableReferenceDetail{
					Level:   level,
					Field:   field,
					Context: input,
				}
			}
			closeIdx += i + openBraceLen // Adjust to absolute position

			varName := input[i+openBraceLen : closeIdx]

			// Validate variable name using existing security validation
			if err := security.ValidateVariableName(varName); err != nil {
				return "", &ErrInvalidVariableNameDetail{
					Level:        level,
					Field:        field,
					VariableName: varName,
					Reason:       err.Error(),
				}
			}

			// Check for circular reference
			if visited[varName] {
				// Copy chain to avoid modifying the passed-in slice
				chain := make([]string, len(expansionChain)+1)
				copy(chain, expansionChain)
				chain[len(chain)-1] = varName
				return "", &ErrCircularReferenceDetail{
					Level:        level,
					Field:        field,
					VariableName: varName,
					Chain:        chain,
				}
			}

			// Lookup variable
			value, ok := expandedVars[varName]
			if !ok {
				return "", &ErrUndefinedVariableDetail{
					Level:        level,
					Field:        field,
					VariableName: varName,
					Context:      input,
				}
			}

			// Recursively expand the value
			// Mark as visited to detect circular references in the current expansion chain
			visited[varName] = true
			// Create new chain for error reporting
			newChain := make([]string, len(expansionChain)+1)
			copy(newChain, expansionChain)
			newChain[len(newChain)-1] = varName
			expandedValue, err := expandStringRecursive(value, expandedVars, level, field, visited, newChain, depth+1)
			// Unmark after recursion completes (allow same variable in different branches)
			delete(visited, varName)

			if err != nil {
				return "", err
			}

			result.WriteString(expandedValue)
			i = closeIdx + 1
			continue
		}

		// Regular character
		result.WriteByte(input[i])
		i++
	}

	return result.String(), nil
}

// ProcessFromEnv processes from_env mappings and imports system environment variables as internal variables.
// It validates internal variable names, checks allowlist, and handles missing system variables.
//
// Parameters:
//   - fromEnv: List of "internal_name=SYSTEM_VAR" mappings
//   - envAllowlist: List of allowed system environment variable names
//   - systemEnv: Map of system environment variables
//   - level: Configuration level for error reporting ("global", "group[name]", etc.)
//
// Returns:
//   - Map of internal variable names to values
//   - Error if validation fails or system var is not in allowlist
func ProcessFromEnv(fromEnv []string, envAllowlist []string, systemEnv map[string]string, level string) (map[string]string, error) {
	if len(fromEnv) == 0 {
		return make(map[string]string), nil
	}

	// Build allowlist map for O(1) lookup
	allowlistMap := make(map[string]bool, len(envAllowlist))
	for _, allowedVar := range envAllowlist {
		allowlistMap[allowedVar] = true
	}

	result := make(map[string]string, len(fromEnv))

	for _, mapping := range fromEnv {
		// Parse "internal_name=SYSTEM_VAR" format
		internalName, systemVarName, ok := common.ParseEnvVariable(mapping)
		if !ok {
			return nil, fmt.Errorf("%w in %s: '%s' (expected 'internal_name=SYSTEM_VAR')", ErrInvalidFromEnvFormat, level, mapping)
		}

		// Validate internal variable name
		if err := validateVariableNameWithDetail(internalName, level, "from_env"); err != nil {
			return nil, err
		}

		// Validate system variable name (should be POSIX compliant)
		// System variables can use reserved prefixes, so we only check POSIX compliance
		if err := security.ValidateVariableName(systemVarName); err != nil {
			return nil, &ErrInvalidSystemVariableNameDetail{
				Level:              level,
				Field:              "from_env",
				SystemVariableName: systemVarName,
				Reason:             err.Error(),
			}
		}

		// Check if system variable is in allowlist
		if !allowlistMap[systemVarName] {
			return nil, &ErrVariableNotInAllowlistDetail{
				Level:           level,
				SystemVarName:   systemVarName,
				InternalVarName: internalName,
				Allowlist:       envAllowlist,
			}
		}

		// Get system environment variable value (empty string if not set)
		value, exists := systemEnv[systemVarName]
		if !exists {
			// Log warning for missing system variable
			// For now, we'll just use empty string
			// TODO: Add logging once logger is available in this context
			value = ""
		}

		result[internalName] = value
	}

	return result, nil
}

// ============================================================================
// Phase 4: vars processing
// ============================================================================

// ProcessVars processes vars field and expands internal variable definitions.
//
// The function processes variables by first parsing and validating all definitions,
// and then expanding them sequentially.
//
// Each variable is expanded in the order it appears in the `vars` array. It can
// reference variables from `baseExpandedVars` or any other variables that have been
// previously defined in the same `vars` array.
//
// NOTE: This sequential approach does not support forward references. For instance,
// in `vars: ["A=%{B}", "B=value"]`, the expansion of `A` will fail because `B` has
// not been processed yet. This results in an `ErrUndefinedVariable`.
//
// Self-extension (e.g., "path=%{path}:/custom") is supported, provided that `path`
// is already defined in `baseExpandedVars`.
//
// Parameters:
//   - vars: Array of "var_name=value" definitions (value can contain %{VAR} references)
//   - baseExpandedVars: Base internal variables (from from_env or parent level)
//   - level: Configuration level for error reporting (e.g., "global", "group[name]", "command[name]")
//
// Returns:
//   - Map of expanded internal variables (merged with baseExpandedVars)
//   - Error if processing fails (invalid format, invalid name, circular reference, undefined variable, etc.)
//
// Example:
//
//	vars := []string{"var1=a", "var2=%{var1}/b", "var3=%{var2}/c"}
//	baseVars := map[string]string{"home": "/home/user"}
//	result, err := ProcessVars(vars, baseVars, "global")
//	// result: {"home": "/home/user", "var1": "a", "var2": "a/b", "var3": "a/b/c"}
func ProcessVars(vars []string, baseExpandedVars map[string]string, level string) (map[string]string, error) {
	// Start with base internal variables (copy to avoid modifying input)
	result := make(map[string]string, len(baseExpandedVars)+len(vars))
	maps.Copy(result, baseExpandedVars)

	// First pass: Parse and validate all vars definitions, store unexpanded
	parsedVars := make([]struct {
		name  string
		value string
	}, 0, len(vars))

	for _, varDef := range vars {
		// Parse "var_name=value" format
		varName, varValue, ok := common.ParseEnvVariable(varDef)
		if !ok {
			return nil, fmt.Errorf("%w in %s: '%s' (expected 'var_name=value')", ErrInvalidVarsFormat, level, varDef)
		}

		// Validate variable name
		if err := validateVariableNameWithDetail(varName, level, "vars"); err != nil {
			return nil, err
		}

		parsedVars = append(parsedVars, struct {
			name  string
			value string
		}{varName, varValue})
	}

	// Second pass: Expand each variable in order
	// Each variable can reference: baseVars + previously defined vars in this array
	for _, pv := range parsedVars {
		// Expand the value using current result map as context
		expandedValue, err := ExpandString(pv.value, result, level, "vars")
		if err != nil {
			return nil, err
		}

		// Update result with expanded value
		result[pv.name] = expandedValue
	}

	return result, nil
}

// ============================================================================

// ProcessEnv processes env field and expands process environment variable definitions.
//
// The function processes the env field which defines environment variables for the
// child process. Each definition can reference internal variables using %{VAR} syntax,
// but cannot reference other env variables (to avoid order dependencies).
//
// Parameters:
//   - env: Array of "VAR=value" definitions (value can contain %{VAR} references to internal variables)
//   - expandedVars: Available internal variables (from from_env and vars)
//   - level: Configuration level for error reporting (e.g., "global", "group[name]", "command[name]")
//
// Returns:
//   - Map of expanded environment variables
//   - Error if processing fails (invalid format, invalid name, undefined internal variable, etc.)
//
// Note: env variables cannot reference other env variables. They can only reference
// internal variables from expandedVars. This prevents order-dependent behavior.
//
// Example:
//
//	env := []string{"BASE_DIR=%{app_dir}", "LOG_DIR=%{app_dir}/logs"}
//	internalVars := map[string]string{"app_dir": "/opt/myapp"}
//	result, err := ProcessEnv(env, internalVars, "global")
//	// result: {"BASE_DIR": "/opt/myapp", "LOG_DIR": "/opt/myapp/logs"}
func ProcessEnv(env []string, expandedVars map[string]string, level string) (map[string]string, error) {
	result := make(map[string]string, len(env))

	for _, envDef := range env {
		// Parse "VAR=value" format
		envVarName, envVarValue, ok := common.ParseEnvVariable(envDef)
		if !ok {
			return nil, fmt.Errorf("%w in %s: '%s' (expected 'VAR=value')", ErrInvalidEnvFormat, level, envDef)
		}

		// Validate environment variable name (POSIX)
		if err := validateVariableNameWithDetail(envVarName, level, "env"); err != nil {
			return nil, err
		}

		// Expand %{VAR} references in the value using internal variables only
		// Note: env variables cannot reference other env variables
		expandedValue, err := ExpandString(envVarValue, expandedVars, level, "env")
		if err != nil {
			return nil, err
		}

		// Store expanded value
		result[envVarName] = expandedValue
	}

	return result, nil
}

// ============================================================================
// Phase 6: Global configuration integration
// ============================================================================

// configFieldsToExpand holds the configuration fields that need expansion.
type configFieldsToExpand struct {
	vars        []string
	env         []string
	verifyFiles []string
}

// expandedConfigFields holds the results of expansion.
type expandedConfigFields struct {
	expandedVars        map[string]string
	expandedEnv         map[string]string
	expandedVerifyFiles []string
}

// expandConfigFields is a helper function that expands vars, env, and verify_files
// given a base set of internal variables. This consolidates the common expansion
// logic shared between ExpandGlobalConfig and ExpandGroupConfig.
//
// Parameters:
//   - fields: The configuration fields to expand (vars, env, verify_files)
//   - baseInternalVars: Base internal variables from from_env processing
//   - level: Context name for error messages (e.g., "global", "group[name]")
//
// Returns:
//   - expandedConfigFields: The expanded results
//   - error: Any error that occurred during expansion
func expandConfigFields(fields configFieldsToExpand, baseInternalVars map[string]string, level string) (expandedConfigFields, error) {
	var result expandedConfigFields

	// Step 1: Process vars to expand internal variable definitions
	expandedVars, err := ProcessVars(fields.vars, baseInternalVars, level)
	if err != nil {
		return result, err
	}
	result.expandedVars = expandedVars

	// Step 2: Process env to expand environment variables
	expandedEnv, err := ProcessEnv(fields.env, expandedVars, level)
	if err != nil {
		return result, fmt.Errorf("failed to process %s env: %w", level, err)
	}
	result.expandedEnv = expandedEnv

	// Step 3: Expand verify_files using internal variables
	expandedVerifyFiles := make([]string, 0, len(fields.verifyFiles))
	for _, filePath := range fields.verifyFiles {
		expandedPath, err := ExpandString(filePath, expandedVars, level, "verify_files")
		if err != nil {
			return result, fmt.Errorf("failed to expand %s verify_files: %w", level, err)
		}
		expandedVerifyFiles = append(expandedVerifyFiles, expandedPath)
	}
	result.expandedVerifyFiles = expandedVerifyFiles

	return result, nil
}

// ExpandGlobalConfig expands all variables in global configuration (from_env, vars, env, verify_files).
//
// Processing order:
//  1. Process from_env → System environment variables to internal variables
//  2. Process vars → Expand internal variable definitions (can reference from_env variables)
//  3. Process env → Expand environment variables (can reference internal variables)
//  4. Process verify_files → Expand file paths (can reference internal variables)
//
// Results are stored in:
//   - global.ExpandedVars: Merged from_env + vars
//   - global.ExpandedEnv: Expanded env field
//   - global.ExpandedVerifyFiles: Expanded verify_files field
//
// Parameters:
//   - global: Global configuration to expand
//   - filter: Environment filter for system environment access
//
// Returns error if any expansion step fails.
func ExpandGlobalConfig(global *runnertypes.GlobalConfig, filter *environment.Filter) error {
	if global == nil {
		return ErrNilConfig
	}

	// Step 1: Get system environment variables
	systemEnv := filter.ParseSystemEnvironment(nil)

	// Step 2: Process from_env to get base internal variables
	baseInternalVars, err := ProcessFromEnv(global.FromEnv, global.EnvAllowlist, systemEnv, "global")
	if err != nil {
		return err
	}

	// Step 3: Expand remaining config fields using helper
	fields := configFieldsToExpand{
		vars:        global.Vars,
		env:         global.Env,
		verifyFiles: global.VerifyFiles,
	}
	expanded, err := expandConfigFields(fields, baseInternalVars, "global")
	if err != nil {
		return err
	}

	// Store results
	global.ExpandedVars = expanded.expandedVars
	global.ExpandedEnv = expanded.expandedEnv
	global.ExpandedVerifyFiles = expanded.expandedVerifyFiles

	return nil
}

// ============================================================================
// Phase 7: Group Configuration Expansion
// ============================================================================

// ResolveGroupFromEnv determines the base internal variables for a group based on from_env inheritance logic.
//
// FromEnv Inheritance Logic:
//   - If group.FromEnv is nil (not defined in TOML): Inherit global.ExpandedVars
//   - If group.FromEnv is [] (explicitly empty): No system env vars imported
//   - If group.FromEnv is defined (non-nil, non-empty): Override global.FromEnv (global.from_env is ignored)
//
// Parameters:
//   - groupFromEnv: The group's from_env field (may be nil, empty, or populated)
//   - groupEnvAllowlist: The group's env_allowlist field (may be nil)
//   - globalExpandedVars: The global.ExpandedVars map (for inheritance)
//   - globalEnvAllowlist: The global.EnvAllowlist (for inheritance when group allowlist is nil)
//   - filter: Environment filter for system environment access
//   - groupName: The group name for error messages
//
// Returns:
//   - map[string]string: The base internal variables for the group
//   - error: Any error that occurred during processing
func ResolveGroupFromEnv(
	groupFromEnv []string,
	groupEnvAllowlist []string,
	globalExpandedVars map[string]string,
	globalEnvAllowlist []string,
	filter *environment.Filter,
	groupName string,
) (map[string]string, error) {
	switch {
	case groupFromEnv == nil:
		// Not defined in TOML → Inherit Global.ExpandedVars
		return maps.Clone(globalExpandedVars), nil

	case len(groupFromEnv) == 0:
		// Explicitly set to [] → No system env vars
		return make(map[string]string), nil

	default:
		// Explicitly defined → Override (global.FromEnv is ignored)
		systemEnv := filter.ParseSystemEnvironment(nil)

		// Determine allowlist (group's allowlist or inherit global's)
		effectiveAllowlist := groupEnvAllowlist
		if effectiveAllowlist == nil {
			effectiveAllowlist = globalEnvAllowlist
		}

		baseInternalVars, err := ProcessFromEnv(
			groupFromEnv,
			effectiveAllowlist,
			systemEnv,
			fmt.Sprintf("group[%s]", groupName),
		)
		if err != nil {
			return nil, err
		}

		return baseInternalVars, nil
	}
}

// ExpandGroupConfig expands all variables in group configuration (from_env, vars, env, verify_files).
//
// FromEnv Inheritance Logic:
//   - If group.FromEnv is nil (not defined in TOML): Inherit global.ExpandedVars
//   - If group.FromEnv is [] (explicitly empty): No system env vars imported
//   - If group.FromEnv is defined (non-nil, non-empty): Override global.FromEnv (global.from_env is ignored)
//
// Processing order:
//  1. Determine from_env inheritance/override
//  2. Process vars → Expand group variable definitions (can reference inherited/imported variables)
//  3. Process env → Expand environment variables (can reference internal variables)
//  4. Process verify_files → Expand file paths (can reference internal variables)
//
// Results are stored in:
//   - group.ExpandedVars: Inherited/imported from_env + group vars
//   - group.ExpandedEnv: Expanded env field
//   - group.ExpandedVerifyFiles: Expanded verify_files field
//
// Parameters:
//   - group: CommandGroup configuration to expand
//   - global: Global configuration (for inheritance)
//   - filter: Environment filter for system environment access
//
// Returns error if any expansion step fails.
func ExpandGroupConfig(group *runnertypes.CommandGroup, global *runnertypes.GlobalConfig, filter *environment.Filter) error {
	if group == nil {
		return ErrNilGroup
	}
	if global == nil {
		return ErrNilConfig
	}

	// Step 1: Determine from_env inheritance using helper function
	baseInternalVars, err := ResolveGroupFromEnv(
		group.FromEnv,
		group.EnvAllowlist,
		global.ExpandedVars,
		global.EnvAllowlist,
		filter,
		group.Name,
	)
	if err != nil {
		return err
	}

	// Step 2: Expand remaining config fields using helper
	fields := configFieldsToExpand{
		vars:        group.Vars,
		env:         group.Env,
		verifyFiles: group.VerifyFiles,
	}
	expanded, err := expandConfigFields(fields, baseInternalVars, fmt.Sprintf("group[%s]", group.Name))
	if err != nil {
		return err
	}

	// Store results
	group.ExpandedVars = expanded.expandedVars
	group.ExpandedEnv = expanded.expandedEnv
	group.ExpandedVerifyFiles = expanded.expandedVerifyFiles

	return nil
}
