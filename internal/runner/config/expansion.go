// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"
	"maps"
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

// configFieldsToExpand holds the raw configuration fields that need expansion
type configFieldsToExpand struct {
	// env is the raw environment variable definitions (e.g., ["VAR=%{value}"])
	env []string
	// verifyFiles is the list of file paths to verify (may contain %{VAR} references)
	verifyFiles []string
	// expandedVars is the map of internal variables to use for expansion
	expandedVars map[string]string
	// level is the configuration level identifier for logging (e.g., "global", "group[name]")
	level string
}

// expandedConfigFields holds the expanded results
type expandedConfigFields struct {
	// expandedEnv contains the fully expanded environment variables (VAR -> value)
	expandedEnv map[string]string
	// expandedVerifyFiles contains the fully expanded file paths to verify
	expandedVerifyFiles []string
}

// expandConfigFields expands env and verify_files using internal variables
func expandConfigFields(fields configFieldsToExpand) (expandedConfigFields, error) {
	var result expandedConfigFields

	// Expand env
	expandedEnv, err := ProcessEnv(fields.env, fields.expandedVars, fields.level)
	if err != nil {
		return result, err
	}
	result.expandedEnv = expandedEnv

	// Expand verify_files
	expandedFiles := make([]string, len(fields.verifyFiles))
	for i, file := range fields.verifyFiles {
		expanded, err := ExpandString(file, fields.expandedVars, fields.level, "verify_files")
		if err != nil {
			return result, fmt.Errorf("failed to expand verify_files[%d]: %w", i, err)
		}
		expandedFiles[i] = expanded
	}
	result.expandedVerifyFiles = expandedFiles

	return result, nil
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

// ExpandGlobalConfig expands Global-level configuration (from_env, vars, env, verify_files)
func ExpandGlobalConfig(global *runnertypes.GlobalConfig, filter *environment.Filter) error {
	const level = "global"
	systemEnv := filter.ParseSystemEnvironment()
	baseExpandedVars, err := ProcessFromEnv(global.FromEnv, global.EnvAllowlist, systemEnv, level)
	if err != nil {
		return err
	}

	// Merge auto variables (auto variables take precedence)
	autoVars := variable.NewAutoVarProvider().Generate()
	maps.Copy(baseExpandedVars, autoVars)

	// Process vars
	expandedVars, err := ProcessVars(global.Vars, baseExpandedVars, level)
	if err != nil {
		return err
	}
	global.ExpandedVars = expandedVars

	// Expand env and verify_files
	// Note: Unlike vars/from_env which use merge strategy between Global and Group levels,
	// env and verify_files are processed at each level independently:
	// - env: Maps with same key override, different keys coexist (processed per level)
	// - verify_files: Separate responsibility - Global files verified at startup,
	//   Group files verified before group execution (no automatic merging)
	fields := configFieldsToExpand{
		env:          global.Env,
		verifyFiles:  global.VerifyFiles,
		expandedVars: global.ExpandedVars,
		level:        level,
	}
	expanded, err := expandConfigFields(fields)
	if err != nil {
		return err
	}

	global.ExpandedEnv = expanded.expandedEnv
	global.ExpandedVerifyFiles = expanded.expandedVerifyFiles
	return nil
}

// ExpandGroupConfig expands Group-level configuration with from_env merging
// from_env uses Merge strategy:
// - If Group.FromEnv is nil or [], inherit Global's from_env variables
// - If Group.FromEnv is defined, merge with Global's from_env (Group's values take priority for same keys)
func ExpandGroupConfig(group *runnertypes.CommandGroup, global *runnertypes.GlobalConfig, filter *environment.Filter) error {
	level := fmt.Sprintf("group[%s]", group.Name)

	// Determine base internal variables with from_env merging
	// Start with Global's expanded vars (includes from_env results)
	baseExpandedVars := maps.Clone(global.ExpandedVars)

	// If Group defines from_env, merge it with global's vars
	systemEnv := filter.ParseSystemEnvironment()
	envAllowlist := determineEffectiveEnvAllowlist(group.EnvAllowlist, global.EnvAllowlist)
	groupFromEnvVars, err := ProcessFromEnv(group.FromEnv, envAllowlist, systemEnv, level)
	if err != nil {
		return err
	}
	// Merge: Group's from_env overrides Global's variables with same name
	maps.Copy(baseExpandedVars, groupFromEnvVars)
	// If Group.FromEnv is nil or [], just inherit Global's ExpandedVars (already done above)

	// Process vars
	expandedVars, err := ProcessVars(group.Vars, baseExpandedVars, level)
	if err != nil {
		return err
	}
	group.ExpandedVars = expandedVars

	// Expand env and verify_files
	// Note: Unlike vars/from_env which merge Global and Group levels,
	// env and verify_files are processed independently at each level:
	// - env: Group.ExpandedEnv contains only Group-level definitions (not merged with Global.ExpandedEnv).
	//   At command execution, Global.ExpandedEnv and Group.ExpandedEnv are merged dynamically.
	// - verify_files: Group.ExpandedVerifyFiles contains only Group-level files.
	//   Global.ExpandedVerifyFiles are verified separately at startup.
	//   This separation avoids redundant verification of Global files for each Group.
	fields := configFieldsToExpand{
		env:          group.Env,
		verifyFiles:  group.VerifyFiles,
		expandedVars: group.ExpandedVars,
		level:        level,
	}
	expanded, err := expandConfigFields(fields)
	if err != nil {
		return err
	}

	group.ExpandedEnv = expanded.expandedEnv
	group.ExpandedVerifyFiles = expanded.expandedVerifyFiles
	return nil
}

// ExpandCommandConfig expands Command-level configuration
func ExpandCommandConfig(
	cmd *runnertypes.Command,
	group *runnertypes.CommandGroup,
	global *runnertypes.GlobalConfig,
	filter *environment.Filter,
) error {
	if group == nil {
		return ErrNilGroup
	}

	level := fmt.Sprintf("command[%s]", cmd.Name)

	// Determine base internal variables based on from_env
	// Process command-level from_env
	systemEnv := filter.ParseSystemEnvironment()
	// Use group's allowlist or global's allowlist (with inheritance)
	envAllowlist := determineEffectiveEnvAllowlist(group.EnvAllowlist, global.EnvAllowlist)
	fromEnvVars, err := ProcessFromEnv(cmd.FromEnv, envAllowlist, systemEnv, level)
	if err != nil {
		return err
	}
	// Merge with group's expanded vars
	baseExpandedVars := maps.Clone(group.ExpandedVars)
	maps.Copy(baseExpandedVars, fromEnvVars)

	// Process vars
	expandedVars, err := ProcessVars(cmd.Vars, baseExpandedVars, level)
	if err != nil {
		return err
	}
	cmd.ExpandedVars = expandedVars

	// Expand env
	expandedEnv, err := ProcessEnv(cmd.Env, cmd.ExpandedVars, level)
	if err != nil {
		return err
	}
	cmd.ExpandedEnv = expandedEnv

	// Expand cmd
	expandedCmd, err := ExpandString(cmd.Cmd, cmd.ExpandedVars, level, "cmd")
	if err != nil {
		return err
	}
	cmd.ExpandedCmd = expandedCmd

	// Expand args
	expandedArgs := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		expanded, err := ExpandString(arg, cmd.ExpandedVars, level, fmt.Sprintf("args[%d]", i))
		if err != nil {
			return err
		}
		expandedArgs[i] = expanded
	}
	cmd.ExpandedArgs = expandedArgs

	return nil
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
	runtime := &runnertypes.RuntimeGlobal{
		Spec:         spec,
		ExpandedVars: make(map[string]string),
		ExpandedEnv:  make(map[string]string),
	}

	// 1. Process FromEnv
	// Build system environment map from os.Environ()
	systemEnv := environment.NewFilter(spec.EnvAllowlist).ParseSystemEnvironment()
	fromEnvVars, err := ProcessFromEnv(spec.FromEnv, spec.EnvAllowlist, systemEnv, "global")
	if err != nil {
		return nil, fmt.Errorf("failed to process global from_env: %w", err)
	}
	runtime.ExpandedVars = fromEnvVars

	// 2. Process Vars
	expandedVars, err := ProcessVars(spec.Vars, runtime.ExpandedVars, "global")
	if err != nil {
		return nil, fmt.Errorf("failed to process global vars: %w", err)
	}
	runtime.ExpandedVars = expandedVars

	// 3. Expand Env
	expandedEnv, err := ProcessEnv(spec.Env, runtime.ExpandedVars, "global")
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

// ExpandGroup expands a GroupSpec into a RuntimeGroup.
//
// This function processes:
// 1. Inherits global variables
// 2. FromEnv: Imports system environment variables as internal variables (group-level) (NOT IMPLEMENTED YET)
// 3. Vars: Defines internal variables (group-level)
// 4. Env: Expands environment variables using internal variables
// 5. VerifyFiles: Expands file paths using internal variables
//
// Parameters:
//   - spec: The group configuration spec to expand
//   - globalVars: Global-level internal variables (from RuntimeGlobal.ExpandedVars)
//
// Returns:
//   - *RuntimeGroup: The expanded runtime group configuration
//   - error: An error if expansion fails
//
// Note:
//   - Commands are NOT expanded by this function. They are expanded separately
//     by GroupExecutor using ExpandCommand() for each command.
func ExpandGroup(spec *runnertypes.GroupSpec, globalVars map[string]string) (*runnertypes.RuntimeGroup, error) {
	runtime := &runnertypes.RuntimeGroup{
		Spec:         spec,
		ExpandedVars: make(map[string]string),
		ExpandedEnv:  make(map[string]string),
		Commands:     make([]*runnertypes.RuntimeCommand, 0),
	}

	// 1. Inherit global variables
	maps.Copy(runtime.ExpandedVars, globalVars)

	// 2. Process FromEnv (group-level)
	// TODO (Task 0033): Implement FromEnv processing, which imports specified system environment variables
	// as internal variables for the group. For now, skip FromEnv processing.

	// 3. Process Vars (group-level)
	expandedVars, err := ProcessVars(spec.Vars, runtime.ExpandedVars, fmt.Sprintf("group[%s]", spec.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to process group[%s] vars: %w", spec.Name, err)
	}
	runtime.ExpandedVars = expandedVars

	// 4. Expand Env
	expandedEnv, err := ProcessEnv(spec.Env, runtime.ExpandedVars, fmt.Sprintf("group[%s]", spec.Name))
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
//
// Returns:
//   - *RuntimeCommand: The expanded runtime command configuration
//   - error: An error if expansion fails
//
// Note:
//   - EffectiveWorkDir and EffectiveTimeout are NOT set by this function.
//     They are set by GroupExecutor after expansion.
func ExpandCommand(spec *runnertypes.CommandSpec, groupVars map[string]string, _ string) (*runnertypes.RuntimeCommand, error) {
	runtime := &runnertypes.RuntimeCommand{
		Spec:         spec,
		ExpandedVars: make(map[string]string),
		ExpandedEnv:  make(map[string]string),
	}

	// 1. Inherit group variables
	maps.Copy(runtime.ExpandedVars, groupVars)

	// 2. Process FromEnv (command-level)
	// TODO (Task 0033): Implement FromEnv processing, which imports specified system environment variables
	// as internal variables for this command. For now, skip FromEnv processing.

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
	expandedEnv, err := ProcessEnv(spec.Env, runtime.ExpandedVars, level)
	if err != nil {
		return nil, fmt.Errorf("failed to process command[%s] env: %w", spec.Name, err)
	}
	runtime.ExpandedEnv = expandedEnv

	// Note: EffectiveWorkDir and EffectiveTimeout are not set here
	return runtime, nil
}
