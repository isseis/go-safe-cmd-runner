// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"
	"maps"
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
				chain := append(expansionChain, varName) //nolint:gocritic // intentionally creating new slice to avoid modifying caller's chain
				return "", &ErrCircularReferenceDetail{
					Level:        level,
					Field:        field,
					VariableName: varName,
					Chain:        chain,
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
			newChain := append(expansionChain, varName) //nolint:gocritic // intentionally creating new slice to avoid modifying caller's chain

			// Recursively expand the value
			expandedValue, err := expandStringRecursive(
				value,
				expandedVars,
				level,
				field,
				visited,
				newChain,
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
		if err := validateVariableNameWithDetail(internalName, level, "from_env"); err != nil {
			return nil, err
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
	// Start with a copy of base variables
	result := maps.Clone(baseExpandedVars)

	// Phase 1: Parse and validate all variable definitions
	type varDef struct {
		name  string
		value string
	}
	definitions := make([]varDef, 0, len(vars))

	for _, definition := range vars {
		varName, varValue, ok := common.ParseKeyValue(definition)
		if !ok {
			return nil, &ErrInvalidVarsFormatDetail{
				Level:      level,
				Definition: definition,
				Reason:     "must be in 'var_name=value' format",
			}
		}

		// Validate variable name
		if err := validateVariableNameWithDetail(varName, level, "vars"); err != nil {
			return nil, err
		}

		definitions = append(definitions, varDef{name: varName, value: varValue})
	}

	// Phase 2: Sequential expansion
	for _, def := range definitions {
		// Expand using current result map (includes baseVars + previously defined vars)
		expandedValue, err := ExpandString(def.value, result, level, "vars")
		if err != nil {
			return nil, err
		}

		// Add to result map for subsequent variables to reference
		result[def.name] = expandedValue
	}

	return result, nil
}

// ProcessEnv processes env definitions and expands them using internal variables.
// Note: env variables cannot reference other env variables, only internal variables.
func ProcessEnv(
	env []string,
	internalVars map[string]string,
	level string,
) (map[string]string, error) {
	result := make(map[string]string)

	for _, definition := range env {
		envVarName, envVarValue, ok := common.ParseKeyValue(definition)
		if !ok {
			return nil, &ErrInvalidEnvFormatDetail{
				Level:      level,
				Definition: definition,
				Reason:     "must be in 'VAR=value' format",
			}
		}

		// Validate environment variable name
		if err := security.ValidateVariableName(envVarName); err != nil {
			return nil, &ErrInvalidEnvKeyDetail{
				Level:   level,
				Key:     envVarName,
				Context: definition,
				Reason:  err.Error(),
			}
		}

		// Expand value using internal variables
		expandedValue, err := ExpandString(envVarValue, internalVars, level, "env")
		if err != nil {
			return nil, err
		}

		result[envVarName] = expandedValue
	}

	return result, nil
}

// configFieldsToExpand holds the raw configuration fields that need expansion
type configFieldsToExpand struct {
	env         []string
	verifyFiles []string
}

// expandedConfigFields holds the expanded results
type expandedConfigFields struct {
	expandedEnv         map[string]string
	expandedVerifyFiles []string
}

// expandConfigFields expands env and verify_files using internal variables
func expandConfigFields(fields configFieldsToExpand, baseInternalVars map[string]string, level string) (expandedConfigFields, error) {
	var result expandedConfigFields

	// Expand env
	if len(fields.env) > 0 {
		expandedEnv, err := ProcessEnv(fields.env, baseInternalVars, level)
		if err != nil {
			return result, err
		}
		result.expandedEnv = expandedEnv
	} else {
		result.expandedEnv = make(map[string]string)
	}

	// Expand verify_files
	if len(fields.verifyFiles) > 0 {
		expandedFiles := make([]string, len(fields.verifyFiles))
		for i, file := range fields.verifyFiles {
			expanded, err := ExpandString(file, baseInternalVars, level, "verify_files")
			if err != nil {
				return result, fmt.Errorf("failed to expand verify_files[%d]: %w", i, err)
			}
			expandedFiles[i] = expanded
		}
		result.expandedVerifyFiles = expandedFiles
	}

	return result, nil
}

// ExpandGlobalConfig expands Global-level configuration (from_env, vars, env, verify_files)
func ExpandGlobalConfig(global *runnertypes.GlobalConfig, filter *environment.Filter) error {
	level := "global"

	// Process from_env
	var baseInternalVars map[string]string
	if len(global.FromEnv) > 0 {
		systemEnv := filter.ParseSystemEnvironment()
		fromEnvVars, err := ProcessFromEnv(global.FromEnv, global.EnvAllowlist, systemEnv, level)
		if err != nil {
			return err
		}
		baseInternalVars = fromEnvVars
	} else {
		baseInternalVars = make(map[string]string)
	}

	// Merge auto variables (auto variables take precedence)
	autoVars := variable.NewAutoVarProvider(nil).Generate()
	maps.Copy(baseInternalVars, autoVars)

	// Process vars
	if len(global.Vars) > 0 {
		expandedVars, err := ProcessVars(global.Vars, baseInternalVars, level)
		if err != nil {
			return err
		}
		global.ExpandedVars = expandedVars
	} else {
		global.ExpandedVars = baseInternalVars
	}

	// Expand env and verify_files
	fields := configFieldsToExpand{
		env:         global.Env,
		verifyFiles: global.VerifyFiles,
	}
	expanded, err := expandConfigFields(fields, global.ExpandedVars, level)
	if err != nil {
		return err
	}

	global.ExpandedEnv = expanded.expandedEnv
	if len(expanded.expandedVerifyFiles) > 0 {
		global.ExpandedVerifyFiles = expanded.expandedVerifyFiles
	} else {
		global.ExpandedVerifyFiles = []string{}
	}

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
	baseInternalVars := maps.Clone(global.ExpandedVars)

	// If Group defines from_env, merge it with global's vars
	if len(group.FromEnv) > 0 {
		systemEnv := filter.ParseSystemEnvironment()
		groupAllowlist := group.EnvAllowlist
		if groupAllowlist == nil {
			groupAllowlist = global.EnvAllowlist
		}
		groupFromEnvVars, err := ProcessFromEnv(group.FromEnv, groupAllowlist, systemEnv, level)
		if err != nil {
			return err
		}
		// Merge: Group's from_env overrides Global's variables with same name
		maps.Copy(baseInternalVars, groupFromEnvVars)
	}
	// If Group.FromEnv is nil or [], just inherit Global's ExpandedVars (already done above)

	// Process vars
	if len(group.Vars) > 0 {
		expandedVars, err := ProcessVars(group.Vars, baseInternalVars, level)
		if err != nil {
			return err
		}
		group.ExpandedVars = expandedVars
	} else {
		group.ExpandedVars = baseInternalVars
	}

	// Expand env and verify_files
	fields := configFieldsToExpand{
		env:         group.Env,
		verifyFiles: group.VerifyFiles,
	}
	expanded, err := expandConfigFields(fields, group.ExpandedVars, level)
	if err != nil {
		return err
	}

	group.ExpandedEnv = expanded.expandedEnv
	if len(expanded.expandedVerifyFiles) > 0 {
		group.ExpandedVerifyFiles = expanded.expandedVerifyFiles
	} else {
		group.ExpandedVerifyFiles = []string{}
	}

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
	var baseInternalVars map[string]string
	if len(cmd.FromEnv) > 0 {
		// Process command-level from_env
		systemEnv := filter.ParseSystemEnvironment()
		// Use group's allowlist or global's allowlist
		cmdAllowlist := group.EnvAllowlist
		if cmdAllowlist == nil {
			cmdAllowlist = global.EnvAllowlist
		}
		fromEnvVars, err := ProcessFromEnv(cmd.FromEnv, cmdAllowlist, systemEnv, level)
		if err != nil {
			return err
		}
		// Merge with group's expanded vars
		baseInternalVars = maps.Clone(group.ExpandedVars)
		for k, v := range fromEnvVars {
			baseInternalVars[k] = v
		}
	} else {
		// Inherit from Group
		baseInternalVars = maps.Clone(group.ExpandedVars)
	}

	// Process vars
	if len(cmd.Vars) > 0 {
		expandedVars, err := ProcessVars(cmd.Vars, baseInternalVars, level)
		if err != nil {
			return err
		}
		cmd.ExpandedVars = expandedVars
	} else {
		cmd.ExpandedVars = baseInternalVars
	}

	// Expand env
	if len(cmd.Env) > 0 {
		expandedEnv, err := ProcessEnv(cmd.Env, cmd.ExpandedVars, level)
		if err != nil {
			return err
		}
		cmd.ExpandedEnv = expandedEnv
	} else {
		cmd.ExpandedEnv = make(map[string]string)
	}

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
