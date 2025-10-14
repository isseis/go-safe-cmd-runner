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
				// Unclosed %{ - return static error
				return "", &ErrInvalidEscapeSequenceDetail{
					Level:    level,
					Field:    field,
					Sequence: "%{",
					Context:  input,
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
