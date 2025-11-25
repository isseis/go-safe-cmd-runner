// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/variable"
)

const (
	// MaxRecursionDepth is the maximum depth for variable expansion to prevent stack overflow
	MaxRecursionDepth = 100
)

// ExpandString expands %{VAR} references in a string using the provided
// internal variables. It detects circular references and reports detailed
// errors. The function is package-level (stateless) and follows Go conventions.
func ExpandString(
	input string,
	expandedVars map[string]string,
	level string,
	field string,
) (string, error) {
	visited := make(map[string]struct{})
	return expandStringRecursive(input, expandedVars, level, field, visited, nil, 0)
}

// expandStringRecursive performs recursive expansion with circular reference detection.
func expandStringRecursive(
	input string,
	expandedVars map[string]string,
	level string,
	field string,
	visited map[string]struct{},
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
			if _, ok := visited[varName]; ok {
				return "", &ErrCircularReferenceDetail{
					Level:        level,
					Field:        field,
					VariableName: varName,
					Chain:        slices.Insert(expansionChain, len(expansionChain), varName),
				}
			}

			// Check if variable is defined
			value, exists := expandedVars[varName]
			if !exists {
				return "", &ErrUndefinedVariableDetail{
					Level:        level,
					Field:        field,
					VariableName: varName,
					Context:      input,
				}
			}

			// Mark as visited for circular reference detection
			visited[varName] = struct{}{}

			// Recursively expand the value
			expandedValue, err := expandStringRecursive(
				value,
				expandedVars,
				level,
				field,
				visited,
				slices.Insert(expansionChain, len(expansionChain), varName),
				depth+1,
			)
			if err != nil {
				return "", err
			}

			// Unmark after expansion
			delete(visited, varName)

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

// ProcessFromEnv processes from_env mappings and imports system environment variables
// as internal variables. It validates that all referenced system variables are in the allowlist.
func ProcessFromEnv(
	fromEnv []string,
	envAllowlist []string,
	systemEnv map[string]string,
	level string,
) (map[string]string, error) {
	result := make(map[string]string)

	// Build allowlist map for O(1) lookup
	allowlistMap := common.SliceToSet(envAllowlist)
	for _, mapping := range fromEnv {
		internalName, systemVarName, ok := common.ParseKeyValue(mapping)
		if !ok {
			return nil, &ErrInvalidFromEnvFormatDetail{
				Level:   level,
				Mapping: mapping,
				Reason:  "must be in 'internal_name=SYSTEM_VAR' format",
			}
		}

		// Validate internal variable name
		if err := validateVariableName(internalName, level, "from_env"); err != nil {
			return nil, err
		}

		// Check for duplicate definition
		if _, exists := result[internalName]; exists {
			return nil, &ErrDuplicateVariableDefinitionDetail{
				Level:        level,
				Field:        "from_env",
				VariableName: internalName,
			}
		}

		// Validate system variable name
		if err := security.ValidateVariableName(systemVarName); err != nil {
			return nil, &ErrInvalidSystemVariableNameDetail{
				Level:              level,
				Field:              "from_env",
				SystemVariableName: systemVarName,
				Reason:             err.Error(),
			}
		}

		// Check allowlist
		if _, ok := allowlistMap[systemVarName]; !ok {
			return nil, &ErrVariableNotInAllowlistDetail{
				Level:           level,
				SystemVarName:   systemVarName,
				InternalVarName: internalName,
				Allowlist:       envAllowlist,
			}
		}

		// Get value from system environment (empty string if not set)
		value := systemEnv[systemVarName]
		result[internalName] = value
	}

	return result, nil
}

// ProcessVars processes vars definitions and expands them using baseExpandedVars.
// Variables are processed sequentially in definition order, allowing later variables
// to reference earlier ones within the same vars array.
func ProcessVars(vars []string, baseExpandedVars map[string]string, level string) (map[string]string, error) {
	// Step 1: Parse and validate all variable definitions
	type parsedMapping struct {
		name  string
		value string
	}
	parsedMappings := make([]parsedMapping, 0, len(vars))

	for _, mapping := range vars {
		varName, varValue, ok := common.ParseKeyValue(mapping)
		if !ok {
			return nil, &ErrInvalidVarsFormatDetail{
				Level:   level,
				Mapping: mapping,
				Reason:  "must be in 'var_name=value' format",
			}
		}

		// Validate variable name
		if err := validateVariableName(varName, level, "vars"); err != nil {
			return nil, err
		}

		// Check for duplicate definition within this vars array
		// Note: Overriding base variables is allowed
		for _, existing := range parsedMappings {
			if existing.name == varName {
				return nil, &ErrDuplicateVariableDefinitionDetail{
					Level:        level,
					Field:        "vars",
					VariableName: varName,
				}
			}
		}

		parsedMappings = append(parsedMappings, parsedMapping{name: varName, value: varValue})
	}

	// Start with a copy of base variables
	expandedVars := maps.Clone(baseExpandedVars)

	// Step 2: Sequential expansion
	for _, parsedMapping := range parsedMappings {
		// Expand using current result map (includes baseExpandedVars + previously defined vars)
		expandedValue, err := ExpandString(parsedMapping.value, expandedVars, level, "vars")
		if err != nil {
			return nil, err
		}

		// Add to result map for subsequent variables to reference
		expandedVars[parsedMapping.name] = expandedValue
	}

	return expandedVars, nil
}

// ProcessEnv processes env definitions and expands them using internal variables.
// Note: env variables cannot reference other env variables, only internal variables.
func ProcessEnv(
	env []string,
	internalVars map[string]string,
	level string,
) (map[string]string, error) {
	expandedEnvVars := make(map[string]string)

	for _, mapping := range env {
		envVarName, envVarValue, ok := common.ParseKeyValue(mapping)
		if !ok {
			return nil, &ErrInvalidEnvFormatDetail{
				Level:   level,
				Mapping: mapping,
				Reason:  "must be in 'VAR=value' format",
			}
		}

		// Validate environment variable name
		if err := security.ValidateVariableName(envVarName); err != nil {
			return nil, &ErrInvalidEnvKeyDetail{
				Level:   level,
				Key:     envVarName,
				Context: mapping,
				Reason:  err.Error(),
			}
		}

		// Check for duplicate definition
		if _, exists := expandedEnvVars[envVarName]; exists {
			return nil, &ErrDuplicateVariableDefinitionDetail{
				Level:        level,
				Field:        "env",
				VariableName: envVarName,
			}
		}

		// Expand value using internal variables
		expandedValue, err := ExpandString(envVarValue, internalVars, level, "env")
		if err != nil {
			return nil, err
		}

		expandedEnvVars[envVarName] = expandedValue
	}

	return expandedEnvVars, nil
}

// determineEffectiveEnvAllowlist determines the effective env_allowlist for a group.
// Returns group's allowlist if defined, otherwise returns global's allowlist (inheritance).
// This implements the allowlist inheritance rule: nil means inherit, empty array means reject all.
func determineEffectiveEnvAllowlist(groupAllowlist []string, globalAllowlist []string) []string {
	if groupAllowlist != nil {
		return groupAllowlist
	}
	return globalAllowlist
}

// ExpandGlobal expands a GlobalSpec into a RuntimeGlobal.
//
// This function processes:
// 1. FromEnv: Imports system environment variables as internal variables
// 2. Vars: Defines internal variables
// 3. Env: Expands environment variables using internal variables
// 4. VerifyFiles: Expands file paths using internal variables
//
// Parameters:
//   - spec: The global configuration spec to expand
//
// Returns:
//   - *RuntimeGlobal: The expanded runtime global configuration
//   - error: An error if expansion fails (e.g., undefined variable reference)
func ExpandGlobal(spec *runnertypes.GlobalSpec) (*runnertypes.RuntimeGlobal, error) {
	// Create RuntimeGlobal using NewRuntimeGlobal to properly initialize timeout field
	runtime, err := runnertypes.NewRuntimeGlobal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create RuntimeGlobal: %w", err)
	}

	// 0. Parse system environment once and cache it
	// This avoids repeated os.Environ() parsing in ExpandGroup and ExpandCommand
	runtime.SystemEnv = environment.NewFilter(spec.EnvAllowed).ParseSystemEnvironment()

	// 0.5. Generate automatic variables (__runner_datetime and __runner_pid)
	// These are generated once at configuration load time and shared across all commands
	autoVars := variable.GenerateGlobalAutoVars(nil) // nil uses time.Now
	runtime.ExpandedVars = autoVars

	// 1. Process FromEnv
	fromEnvVars, err := ProcessFromEnv(spec.EnvImport, spec.EnvAllowed, runtime.SystemEnv, "global")
	if err != nil {
		return nil, fmt.Errorf("failed to process global from_env: %w", err)
	}
	// Merge fromEnvVars into runtime.ExpandedVars (which already contains autoVars)
	for k, v := range fromEnvVars {
		runtime.ExpandedVars[k] = v
	}

	// 2. Process Vars
	expandedVars, err := ProcessVars(spec.Vars, runtime.ExpandedVars, "global")
	if err != nil {
		return nil, fmt.Errorf("failed to process global vars: %w", err)
	}
	runtime.ExpandedVars = expandedVars

	// 3. Expand Env
	expandedEnv, err := ProcessEnv(spec.EnvVars, runtime.ExpandedVars, "global")
	if err != nil {
		return nil, fmt.Errorf("failed to process global env: %w", err)
	}
	runtime.ExpandedEnv = expandedEnv

	// 4. Expand VerifyFiles
	runtime.ExpandedVerifyFiles = make([]string, len(spec.VerifyFiles))
	for i, file := range spec.VerifyFiles {
		expandedFile, err := ExpandString(file, runtime.ExpandedVars, "global", fmt.Sprintf("verify_files[%d]", i))
		if err != nil {
			return nil, err
		}
		runtime.ExpandedVerifyFiles[i] = expandedFile
	}

	return runtime, nil
}

// expandCmdAllowed expands and validates cmd_allowed paths.
//
// Processing steps:
//  1. Variable expansion: %{var} -> actual value
//  2. Empty string validation: reject empty strings
//  3. Absolute path validation: must start with '/'
//  4. Path length validation: must not exceed MaxPathLength
//  5. Symbolic link resolution: filepath.EvalSymlinks
//  6. Deduplication: remove duplicate paths
//
// Parameters:
//   - rawPaths: List of paths to expand (may contain variable references)
//   - vars: Variable map for expansion (%{key} -> value)
//   - groupName: Group name for error messages
//
// Returns:
//   - []string: Expanded and normalized path list
//   - error: Expansion or validation error
func expandCmdAllowed(
	rawPaths []string,
	vars map[string]string,
	groupName string,
) ([]string, error) {
	result := make([]string, 0, len(rawPaths))
	seen := make(map[string]struct{}) // for deduplication

	for i, rawPath := range rawPaths {
		// 1. Empty string check
		if rawPath == "" {
			return nil, fmt.Errorf("group[%s] cmd_allowed[%d]: %w", groupName, i, ErrEmptyPath)
		}

		// 2. Variable expansion
		expanded, err := ExpandString(rawPath, vars, fmt.Sprintf("group[%s]", groupName), fmt.Sprintf("cmd_allowed[%d]", i))
		if err != nil {
			return nil, fmt.Errorf("group[%s] cmd_allowed[%d] '%s': %w", groupName, i, rawPath, err)
		}

		// 3. Absolute path validation
		if !filepath.IsAbs(expanded) {
			return nil, &InvalidPathError{
				Path:   expanded,
				Reason: "cmd_allowed paths must be absolute (start with '/')",
			}
		}

		// 4. Path length validation
		const MaxPathLength = security.DefaultMaxPathLength
		if len(expanded) > MaxPathLength {
			return nil, &InvalidPathError{
				Path:   expanded,
				Reason: fmt.Sprintf("path length exceeds maximum (%d)", MaxPathLength),
			}
		}

		// 5. Symbolic link resolution and normalization
		normalized, err := filepath.EvalSymlinks(expanded)
		if err != nil {
			return nil, fmt.Errorf("group[%s] cmd_allowed[%d] '%s': failed to resolve path: %w", groupName, i, expanded, err)
		}

		// 6. Deduplication
		if _, exists := seen[normalized]; !exists {
			result = append(result, normalized)
			seen[normalized] = struct{}{}
		}
	}

	return result, nil
}

// ExpandGroup expands a GroupSpec into a RuntimeGroup.
//
// This function processes:
// 1. Inherit global variables
// 2. FromEnv: Imports system environment variables as internal variables (group-level)
// 3. Vars: Defines internal variables (group-level)
// 4. Env: Expands environment variables using internal variables
// 5. VerifyFiles: Expands file paths using internal variables
// 6. CmdAllowed: Expands and validates allowed command paths
//
// Parameters:
//   - spec: The group configuration spec to expand
//   - globalRuntime: The global runtime configuration
//
// Returns:
//   - *RuntimeGroup: The expanded runtime group configuration
//   - error: An error if expansion fails
//
// Note:
//   - Commands are NOT expanded by this function. They are expanded separately
//     by GroupExecutor using ExpandCommand() for each command.
func ExpandGroup(spec *runnertypes.GroupSpec, globalRuntime *runnertypes.RuntimeGlobal) (*runnertypes.RuntimeGroup, error) {
	runtime, err := runnertypes.NewRuntimeGroup(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create RuntimeGroup: %w", err)
	}

	// Set the inheritance mode immediately after RuntimeGroup creation
	runtime.EnvAllowlistInheritanceMode = runnertypes.DetermineEnvAllowlistInheritanceMode(spec.EnvAllowed)

	// 1. Inherit global variables
	if globalRuntime != nil {
		maps.Copy(runtime.ExpandedVars, globalRuntime.ExpandedVars)
	}

	// 2. Process FromEnv (group-level)
	// Implement from_env processing with allowlist inheritance: group.EnvAllowed (if non-nil)
	// overrides global; nil means inherit global allowlist; empty slice means reject all.
	if len(spec.EnvImport) > 0 {
		// Use cached system environment from globalRuntime
		var globalAllowlist []string
		var systemEnv map[string]string
		if globalRuntime != nil {
			globalAllowlist = globalRuntime.EnvAllowlist()
			systemEnv = globalRuntime.SystemEnv
		}

		effectiveAllowlist := determineEffectiveEnvAllowlist(spec.EnvAllowed, globalAllowlist)

		fromEnvVars, err := ProcessFromEnv(spec.EnvImport, effectiveAllowlist, systemEnv, fmt.Sprintf("group[%s]", spec.Name))
		if err != nil {
			return nil, fmt.Errorf("failed to process group[%s] from_env: %w", spec.Name, err)
		}

		// Merge from_env variables into expanded vars (group-level from_env may override inherited vars)
		maps.Copy(runtime.ExpandedVars, fromEnvVars)
	}

	// 3. Process Vars (group-level)
	expandedVars, err := ProcessVars(spec.Vars, runtime.ExpandedVars, fmt.Sprintf("group[%s]", spec.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to process group[%s] vars: %w", spec.Name, err)
	}
	runtime.ExpandedVars = expandedVars

	// 4. Expand Env
	expandedEnv, err := ProcessEnv(spec.EnvVars, runtime.ExpandedVars, fmt.Sprintf("group[%s]", spec.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to process group[%s] env: %w", spec.Name, err)
	}
	runtime.ExpandedEnv = expandedEnv

	// 5. Expand VerifyFiles
	runtime.ExpandedVerifyFiles = make([]string, len(spec.VerifyFiles))
	for i, file := range spec.VerifyFiles {
		expandedFile, err := ExpandString(file, runtime.ExpandedVars, fmt.Sprintf("group[%s]", spec.Name), fmt.Sprintf("verify_files[%d]", i))
		if err != nil {
			return nil, err
		}
		runtime.ExpandedVerifyFiles[i] = expandedFile
	}

	// 6. Expand CmdAllowed
	if len(spec.CmdAllowed) > 0 {
		expandedCmdAllowed, err := expandCmdAllowed(
			spec.CmdAllowed,
			runtime.ExpandedVars,
			spec.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to expand cmd_allowed for group[%s]: %w", spec.Name, err)
		}
		runtime.ExpandedCmdAllowed = expandedCmdAllowed
	}

	// Note: Commands are not expanded at this point
	return runtime, nil
}

// ExpandCommand expands a CommandSpec into a RuntimeCommand.
//
// This function processes:
// 1. Inherits group variables
// 2. FromEnv: Imports system environment variables as internal variables (command-level) (NOT IMPLEMENTED YET)
// 3. Vars: Defines internal variables (command-level)
// 4. Cmd: Expands command path using internal variables
// 5. Args: Expands command arguments using internal variables
// 6. Env: Expands environment variables using internal variables
//
// Parameters:
//   - spec: The command configuration spec to expand
//   - groupVars: Group-level internal variables (from RuntimeGroup.ExpandedVars)
//   - groupName: Group name for error messages (currently unused as spec.Name is used directly)
//   - globalTimeout: Global timeout setting for timeout resolution hierarchy
//   - globalOutputSizeLimit: Global output size limit setting for output size limit resolution
//
// Returns:
//   - *RuntimeCommand: The expanded runtime command configuration with resolved EffectiveTimeout and EffectiveOutputSizeLimit
//   - error: An error if expansion fails
//
// Note:
//   - EffectiveTimeout is set by NewRuntimeCommand using timeout resolution hierarchy.
//   - EffectiveOutputSizeLimit is set by NewRuntimeCommand using output size limit resolution.
//   - EffectiveWorkDir is NOT set by this function; it is set by GroupExecutor after expansion.
func ExpandCommand(spec *runnertypes.CommandSpec, runtimeGroup *runnertypes.RuntimeGroup, globalRuntime *runnertypes.RuntimeGlobal, globalTimeout common.Timeout, globalOutputSizeLimit common.OutputSizeLimit) (*runnertypes.RuntimeCommand, error) {
	// Create RuntimeCommand using NewRuntimeCommand to properly resolve timeout and output size limit
	groupName := runnertypes.ExtractGroupName(runtimeGroup)
	runtime, err := runnertypes.NewRuntimeCommand(spec, globalTimeout, globalOutputSizeLimit, groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to create RuntimeCommand for command[%s]: %w", spec.Name, err)
	}

	// 1. Inherit group variables
	if runtimeGroup != nil {
		maps.Copy(runtime.ExpandedVars, runtimeGroup.ExpandedVars)
	}

	// 2. Process FromEnv (command-level)
	// Command-level from_env uses group's allowlist (if any) with fallback to global allowlist
	if len(spec.EnvImport) > 0 {
		// Use cached system environment from globalRuntime
		var globalAllowlist []string
		var systemEnv map[string]string
		if globalRuntime != nil {
			globalAllowlist = globalRuntime.EnvAllowlist()
			systemEnv = globalRuntime.SystemEnv
		}

		var groupAllowlist []string
		if runtimeGroup != nil && runtimeGroup.Spec != nil {
			groupAllowlist = runtimeGroup.Spec.EnvAllowed
		}

		effectiveAllowlist := determineEffectiveEnvAllowlist(groupAllowlist, globalAllowlist)

		fromEnvVars, err := ProcessFromEnv(spec.EnvImport, effectiveAllowlist, systemEnv, fmt.Sprintf("command[%s]", spec.Name))
		if err != nil {
			return nil, fmt.Errorf("failed to process command[%s] from_env: %w", spec.Name, err)
		}

		// Merge command-level from_env into expanded vars (command-level may override group vars)
		maps.Copy(runtime.ExpandedVars, fromEnvVars)
	}

	// 3. Process Vars (command-level)
	expandedVars, err := ProcessVars(spec.Vars, runtime.ExpandedVars, fmt.Sprintf("command[%s]", spec.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to process command[%s] vars: %w", spec.Name, err)
	}
	runtime.ExpandedVars = expandedVars

	level := fmt.Sprintf("command[%s]", spec.Name)

	// 4. Expand Cmd
	expandedCmd, err := ExpandString(spec.Cmd, runtime.ExpandedVars, level, "cmd")
	if err != nil {
		return nil, err
	}
	runtime.ExpandedCmd = expandedCmd

	// 5. Expand Args
	runtime.ExpandedArgs = make([]string, len(spec.Args))
	for i, arg := range spec.Args {
		expandedArg, err := ExpandString(arg, runtime.ExpandedVars, level, fmt.Sprintf("args[%d]", i))
		if err != nil {
			return nil, err
		}
		runtime.ExpandedArgs[i] = expandedArg
	}

	// 6. Expand Env
	expandedEnv, err := ProcessEnv(spec.EnvVars, runtime.ExpandedVars, level)
	if err != nil {
		return nil, fmt.Errorf("failed to process command[%s] env: %w", spec.Name, err)
	}
	runtime.ExpandedEnv = expandedEnv

	// Note: EffectiveWorkDir and EffectiveTimeout are not set here
	return runtime, nil
}
