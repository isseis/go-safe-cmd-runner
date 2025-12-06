// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"
	"maps"
	"path/filepath"
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

	// MaxVarsPerLevel is the maximum number of variables allowed per level
	// (global, group, or command). This prevents DoS attacks via excessive
	// variable definitions.
	MaxVarsPerLevel = 1000

	// MaxArrayElements is the maximum number of elements allowed in an array
	// variable. This prevents DoS attacks via large arrays.
	MaxArrayElements = 1000

	// MaxStringValueLen is the maximum length (in bytes) allowed for a string
	// value. This prevents memory exhaustion via extremely long strings.
	MaxStringValueLen = 10 * 1024 // 10KB
)

// variableResolver is a function type that resolves a variable name to its expanded value.
// It is called during variable expansion to look up and expand variable references.
//
// Parameters:
//   - varName: the variable name to resolve (without %{} syntax)
//   - field: field name for error messages
//   - visited: map tracking currently-being-expanded variables (for circular detection)
//   - expansionChain: ordered list of variable names in current expansion path
//   - depth: current recursion depth
//
// Returns:
//   - string: the expanded value of the variable
//   - error: resolution error (e.g., undefined variable, type mismatch)
type variableResolver func(
	varName string,
	field string,
	visited map[string]struct{},
	expansionChain []string,
	depth int,
) (string, error)

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
// This is a wrapper around expandStringWithResolver for backward compatibility.
func expandStringRecursive(
	input string,
	expandedVars map[string]string,
	level string,
	field string,
	visited map[string]struct{},
	expansionChain []string,
	depth int,
) (string, error) {
	// Create a resolver that looks up variables from expandedVars
	// and recursively expands them
	resolver := func(
		varName string,
		resolverField string,
		resolverVisited map[string]struct{},
		resolverChain []string,
		resolverDepth int,
	) (string, error) {
		// Check if variable is defined
		value, exists := expandedVars[varName]
		if !exists {
			return "", &ErrUndefinedVariableDetail{
				Level:        level,
				Field:        resolverField,
				VariableName: varName,
				Context:      input,
			}
		}

		// Mark as visited for circular reference detection
		resolverVisited[varName] = struct{}{}

		// Recursively expand the value
		expandedValue, err := expandStringRecursive(
			value,
			expandedVars,
			level,
			resolverField,
			resolverVisited,
			append(resolverChain, varName),
			resolverDepth+1,
		)
		if err != nil {
			return "", err
		}

		// Unmark after expansion
		delete(resolverVisited, varName)

		return expandedValue, nil
	}

	return expandStringWithResolver(input, resolver, level, field, visited, expansionChain, depth)
}

// expandStringWithResolver performs recursive variable expansion using a custom resolver.
// This is the core expansion logic shared by both ExpandString and varExpander.
//
// Parameters:
//   - input: the string to expand (may contain %{VAR} references and escape sequences)
//   - resolver: function to resolve variable names to their values
//   - level: context for error messages (e.g., "global", "group[deploy]")
//   - field: field name for error messages (e.g., "vars", "env.PATH")
//   - visited: tracks variables currently being expanded (for circular reference detection)
//   - expansionChain: ordered list of variable names in the current expansion path
//   - depth: current recursion depth
//
// Returns:
//   - string: the fully expanded string
//   - error: expansion error (syntax error, undefined variable, circular reference, etc.)
func expandStringWithResolver(
	input string,
	resolver variableResolver,
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
					Chain:        append(expansionChain, varName),
				}
			}

			// Resolve variable using the provided resolver
			value, err := resolver(varName, field, visited, expansionChain, depth)
			if err != nil {
				return "", err
			}

			result.WriteString(value)
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

// varExpander handles variable expansion with lazy resolution.
// It maintains state for memoization and circular reference detection.
//
// SIDE EFFECT: The expandString method modifies the expandedVars map by adding
// newly expanded variables to it. This is intentional for memoization.
type varExpander struct {
	// expandedVars contains already-expanded string variables.
	// Also used for memoization of newly expanded variables.
	expandedVars map[string]string

	// expandedArrayVars contains already-expanded array variables.
	expandedArrayVars map[string][]string

	// rawVars contains not-yet-expanded variable definitions.
	rawVars map[string]interface{}

	// level is the context for error messages (e.g., "global", "group[deploy]").
	level string
}

// newVarExpander creates a new varExpander instance.
func newVarExpander(
	expandedVars map[string]string,
	expandedArrayVars map[string][]string,
	rawVars map[string]interface{},
	level string,
) *varExpander {
	return &varExpander{
		expandedVars:      expandedVars,
		expandedArrayVars: expandedArrayVars,
		rawVars:           rawVars,
		level:             level,
	}
}

// expandString expands variable references in the input string.
// It resolves references to both already-expanded and raw variables.
//
// Parameters:
//   - input: the string containing %{VAR} references to expand
//   - field: field name for error messages (e.g., "vars.config_path")
//
// Returns the expanded string or an error.
func (e *varExpander) expandString(input string, field string) (string, error) {
	visited := make(map[string]struct{})
	expansionChain := make([]string, 0)

	// Use expandStringWithResolver with varExpander's resolver
	resolver := func(
		varName string,
		resolverField string,
		resolverVisited map[string]struct{},
		resolverChain []string,
		resolverDepth int,
	) (string, error) {
		return e.resolveVariable(varName, resolverField, resolverVisited, resolverChain, resolverDepth)
	}

	return expandStringWithResolver(input, resolver, e.level, field, visited, expansionChain, 0)
}

// resolveVariable looks up and expands a variable by name.
// It checks expandedVars first, then rawVars for lazy expansion.
func (e *varExpander) resolveVariable(
	varName string,
	field string,
	visited map[string]struct{},
	expansionChain []string,
	depth int,
) (string, error) {
	// First, check already-expanded variables (includes memoized results)
	if v, ok := e.expandedVars[varName]; ok {
		return v, nil
	}

	// Check if it's an array variable (cannot be used in string context)
	if _, ok := e.expandedArrayVars[varName]; ok {
		return "", &ErrArrayVariableInStringContextDetail{
			Level:        e.level,
			Field:        field,
			VariableName: varName,
			Chain:        append(expansionChain, varName),
		}
	}

	// Check raw vars for lazy expansion
	rawVal, ok := e.rawVars[varName]
	if !ok {
		return "", &ErrUndefinedVariableDetail{
			Level:        e.level,
			Field:        field,
			VariableName: varName,
			Context:      "",
			Chain:        append(expansionChain, varName),
		}
	}

	// Handle based on type
	switch rv := rawVal.(type) {
	case string:
		// Mark as visited before recursive expansion
		visited[varName] = struct{}{}

		// Create a resolver for recursive expansion
		resolver := func(
			resolverVarName string,
			resolverField string,
			resolverVisited map[string]struct{},
			resolverChain []string,
			resolverDepth int,
		) (string, error) {
			return e.resolveVariable(resolverVarName, resolverField, resolverVisited, resolverChain, resolverDepth)
		}

		// Expand the raw value using the shared expansion logic
		expanded, err := expandStringWithResolver(
			rv,
			resolver,
			e.level,
			field,
			visited,
			append(expansionChain, varName),
			depth+1,
		)
		if err != nil {
			return "", err
		}

		// Unmark after expansion
		delete(visited, varName)

		// Cache the expanded value for future references (memoization)
		e.expandedVars[varName] = expanded

		return expanded, nil

	case []interface{}:
		// Array variable referenced in string context
		return "", &ErrArrayVariableInStringContextDetail{
			Level:        e.level,
			Field:        field,
			VariableName: varName,
			Chain:        append(expansionChain, varName),
		}

	default:
		// This shouldn't happen as we validate types in ProcessVars
		return "", &ErrUnsupportedTypeDetail{
			Level:        e.level,
			VariableName: varName,
			ActualType:   fmt.Sprintf("%T", rawVal),
		}
	}
}

// ProcessVars processes vars definitions from a TOML table and expands them
// using baseExpandedVars and baseExpandedArrays.
//
// Parameters:
//   - vars: Variable definitions from TOML (map[string]interface{})
//   - baseExpandedVars: Previously expanded string variables (inherited)
//   - baseExpandedArrays: Previously expanded array variables (inherited)
//   - level: Context for error messages (e.g., "global", "group[deploy]")
//
// Returns:
//   - map[string]string: Expanded string variables (includes base + new)
//   - map[string][]string: Expanded array variables (includes base + new)
//   - error: Validation or expansion error
//
// Processing steps:
//  1. Check total variable count against MaxVarsPerLevel
//  2. For each variable:
//     a. Validate variable name using ValidateVariableName
//     b. Check type consistency with base variables
//     c. Validate value type (string or []interface{})
//     d. Validate size limits
//     e. Expand using ExpandString
//     f. Store in appropriate output map
//
// Type consistency rule:
//   - A variable defined as string cannot be overridden as array
//   - A variable defined as array cannot be overridden as string
//   - Same type override is allowed (value replacement)
//
// Empty arrays are allowed and useful for clearing inherited variables.
func ProcessVars(
	vars map[string]interface{},
	baseExpandedVars map[string]string,
	baseExpandedArrays map[string][]string,
	level string,
) (map[string]string, map[string][]string, error) {
	// Handle nil vars map
	if vars == nil {
		return cloneBaseVars(baseExpandedVars, baseExpandedArrays)
	}

	// Check total variable count
	if len(vars) > MaxVarsPerLevel {
		return nil, nil, &ErrTooManyVariablesDetail{
			Level:    level,
			Count:    len(vars),
			MaxCount: MaxVarsPerLevel,
		}
	}

	// Phase 1: Validation and type checking
	stringVars, arrayVars, err := validateAndClassifyVars(vars, baseExpandedVars, baseExpandedArrays, level)
	if err != nil {
		return nil, nil, err
	}

	// Phase 2: Expansion with lazy resolution
	return expandVarsWithLazyResolution(vars, stringVars, arrayVars, baseExpandedVars, baseExpandedArrays, level)
}

// cloneBaseVars creates copies of base variables.
func cloneBaseVars(
	baseExpandedVars map[string]string,
	baseExpandedArrays map[string][]string,
) (map[string]string, map[string][]string, error) {
	expandedStrings := maps.Clone(baseExpandedVars)
	if expandedStrings == nil {
		expandedStrings = make(map[string]string)
	}
	expandedArrays := maps.Clone(baseExpandedArrays)
	if expandedArrays == nil {
		expandedArrays = make(map[string][]string)
	}
	return expandedStrings, expandedArrays, nil
}

// validateAndClassifyVars validates all variables and classifies them by type.
func validateAndClassifyVars(
	vars map[string]interface{},
	baseExpandedVars map[string]string,
	baseExpandedArrays map[string][]string,
	level string,
) (map[string]string, map[string][]interface{}, error) {
	stringVars := make(map[string]string)
	arrayVars := make(map[string][]interface{})

	for varName, rawValue := range vars {
		// Validate variable name
		if err := validateVariableName(varName, level, "vars"); err != nil {
			return nil, nil, err
		}

		// Determine the type and validate
		switch v := rawValue.(type) {
		case string:
			if err := validateStringVar(varName, v, baseExpandedArrays, level); err != nil {
				return nil, nil, err
			}
			stringVars[varName] = v

		case []interface{}:
			if err := validateArrayVar(varName, v, baseExpandedVars, level); err != nil {
				return nil, nil, err
			}
			arrayVars[varName] = v

		default:
			return nil, nil, &ErrUnsupportedTypeDetail{
				Level:        level,
				VariableName: varName,
				ActualType:   fmt.Sprintf("%T", rawValue),
			}
		}
	}

	return stringVars, arrayVars, nil
}

// validateStringVar validates a string variable.
func validateStringVar(
	varName string,
	value string,
	baseExpandedArrays map[string][]string,
	level string,
) error {
	// Check if overriding an array variable with a string
	if _, ok := baseExpandedArrays[varName]; ok {
		return &ErrTypeMismatchDetail{
			Level:        level,
			VariableName: varName,
			ExpectedType: "array",
			ActualType:   "string",
		}
	}

	// Check string length
	if len(value) > MaxStringValueLen {
		return &ErrValueTooLongDetail{
			Level:        level,
			VariableName: varName,
			Length:       len(value),
			MaxLength:    MaxStringValueLen,
		}
	}

	return nil
}

// validateArrayVar validates an array variable.
func validateArrayVar(
	varName string,
	value []interface{},
	baseExpandedVars map[string]string,
	level string,
) error {
	// Check if overriding a string variable with an array
	if _, ok := baseExpandedVars[varName]; ok {
		return &ErrTypeMismatchDetail{
			Level:        level,
			VariableName: varName,
			ExpectedType: "string",
			ActualType:   "array",
		}
	}

	// Check array size
	if len(value) > MaxArrayElements {
		return &ErrArrayTooLargeDetail{
			Level:        level,
			VariableName: varName,
			Count:        len(value),
			MaxCount:     MaxArrayElements,
		}
	}

	// Validate each array element
	for i, elem := range value {
		str, ok := elem.(string)
		if !ok {
			return &ErrInvalidArrayElementDetail{
				Level:        level,
				VariableName: varName,
				Index:        i,
				ExpectedType: "string",
				ActualType:   fmt.Sprintf("%T", elem),
			}
		}
		if len(str) > MaxStringValueLen {
			return &ErrArrayElementTooLongDetail{
				Level:        level,
				VariableName: varName,
				Index:        i,
				Length:       len(str),
				MaxLength:    MaxStringValueLen,
			}
		}
	}

	return nil
}

// expandVarsWithLazyResolution expands variables using lazy resolution.
func expandVarsWithLazyResolution(
	vars map[string]interface{},
	stringVars map[string]string,
	arrayVars map[string][]interface{},
	baseExpandedVars map[string]string,
	baseExpandedArrays map[string][]string,
	level string,
) (map[string]string, map[string][]string, error) {
	// Start with copies of base variables
	expandedStrings := maps.Clone(baseExpandedVars)
	if expandedStrings == nil {
		expandedStrings = make(map[string]string)
	}
	expandedArrays := maps.Clone(baseExpandedArrays)
	if expandedArrays == nil {
		expandedArrays = make(map[string][]string)
	}

	// Create expander for lazy variable resolution
	expander := newVarExpander(expandedStrings, expandedArrays, vars, level)

	// Expand string variables (order-independent due to lazy resolution)
	for varName, rawValue := range stringVars {
		expanded, err := expander.expandString(
			rawValue,
			fmt.Sprintf("vars.%s", varName),
		)
		if err != nil {
			return nil, nil, err
		}
		expandedStrings[varName] = expanded
	}

	// Expand array variables
	for varName, rawArray := range arrayVars {
		expandedArray := make([]string, len(rawArray))
		for i, elem := range rawArray {
			str := elem.(string) // Already validated in Phase 1

			expanded, err := expander.expandString(
				str,
				fmt.Sprintf("vars.%s[%d]", varName, i),
			)
			if err != nil {
				return nil, nil, err
			}
			expandedArray[i] = expanded
		}
		expandedArrays[varName] = expandedArray
	}

	return expandedStrings, expandedArrays, nil
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
	expandedVars, expandedArrays, err := ProcessVars(spec.Vars, runtime.ExpandedVars, runtime.ExpandedArrayVars, "global")
	if err != nil {
		return nil, fmt.Errorf("failed to process global vars: %w", err)
	}
	runtime.ExpandedVars = expandedVars
	runtime.ExpandedArrayVars = expandedArrays

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
//  1. Duplicate detection (raw string level): detect configuration errors
//  2. Variable expansion: %{var} -> actual value
//  3. Empty string validation: reject empty strings
//  4. Absolute path validation: must start with '/'
//  5. Path length validation: must not exceed MaxPathLength
//  6. Symbolic link resolution: filepath.EvalSymlinks
//  7. Duplicate detection (resolved path level): detect paths pointing to same file
//
// Parameters:
//   - rawPaths: List of paths to expand (may contain variable references)
//   - vars: Variable map for expansion (%{key} -> value)
//   - groupName: Group name for error messages
//
// Returns:
//   - map[string]struct{}: Expanded and normalized path set for O(1) lookup
//   - error: Expansion or validation error
func expandCmdAllowed(
	rawPaths []string,
	vars map[string]string,
	groupName string,
) (map[string]struct{}, error) {
	// 1. Check for duplicate raw strings (before expansion)
	seenRaw := make(map[string]int, len(rawPaths))
	for i, rawPath := range rawPaths {
		if firstIdx, exists := seenRaw[rawPath]; exists {
			return nil, &ErrDuplicatePathDetail{
				Level:      fmt.Sprintf("group[%s]", groupName),
				Field:      "cmd_allowed",
				Path:       rawPath,
				FirstIndex: firstIdx,
				DupeIndex:  i,
			}
		}
		seenRaw[rawPath] = i
	}

	result := make(map[string]struct{}, len(rawPaths))

	for i, rawPath := range rawPaths {
		// 2. Empty string check
		if rawPath == "" {
			return nil, fmt.Errorf("group[%s] cmd_allowed[%d]: %w", groupName, i, ErrEmptyPath)
		}

		// 3. Variable expansion
		expanded, err := ExpandString(rawPath, vars, fmt.Sprintf("group[%s]", groupName), fmt.Sprintf("cmd_allowed[%d]", i))
		if err != nil {
			return nil, fmt.Errorf("group[%s] cmd_allowed[%d] '%s': %w", groupName, i, rawPath, err)
		}

		// 4. Absolute path validation
		if !filepath.IsAbs(expanded) {
			return nil, &InvalidPathError{
				Path:   expanded,
				Reason: "cmd_allowed paths must be absolute (start with '/')",
			}
		}

		// 5. Path length validation
		const MaxPathLength = security.DefaultMaxPathLength
		if len(expanded) > MaxPathLength {
			return nil, &InvalidPathError{
				Path:   expanded,
				Reason: fmt.Sprintf("path length exceeds maximum (%d)", MaxPathLength),
			}
		}

		// 6. Symbolic link resolution and normalization
		normalized, err := filepath.EvalSymlinks(expanded)
		if err != nil {
			return nil, fmt.Errorf("group[%s] cmd_allowed[%d] '%s': failed to resolve path: %w", groupName, i, expanded, err)
		}

		// 7. Check for duplicate resolved paths
		if _, exists := result[normalized]; exists {
			return nil, &ErrDuplicateResolvedPathDetail{
				Level:        fmt.Sprintf("group[%s]", groupName),
				Field:        "cmd_allowed",
				OriginalPath: rawPath,
				ResolvedPath: normalized,
			}
		}

		result[normalized] = struct{}{}
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
		maps.Copy(runtime.ExpandedArrayVars, globalRuntime.ExpandedArrayVars)
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
	expandedVars, expandedArrays, err := ProcessVars(spec.Vars, runtime.ExpandedVars, runtime.ExpandedArrayVars, fmt.Sprintf("group[%s]", spec.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to process group[%s] vars: %w", spec.Name, err)
	}
	runtime.ExpandedVars = expandedVars
	runtime.ExpandedArrayVars = expandedArrays

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
	expandedCmdAllowed, err := expandCmdAllowed(
		spec.CmdAllowed,
		runtime.ExpandedVars,
		spec.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to expand cmd_allowed for group[%s]: %w", spec.Name, err)
	}
	runtime.ExpandedCmdAllowed = expandedCmdAllowed

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
		maps.Copy(runtime.ExpandedArrayVars, runtimeGroup.ExpandedArrayVars)
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
	expandedVars, expandedArrays, err := ProcessVars(spec.Vars, runtime.ExpandedVars, runtime.ExpandedArrayVars, fmt.Sprintf("command[%s]", spec.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to process command[%s] vars: %w", spec.Name, err)
	}
	runtime.ExpandedVars = expandedVars
	runtime.ExpandedArrayVars = expandedArrays

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
