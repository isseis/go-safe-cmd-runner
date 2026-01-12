// Package config provides configuration loading and validation for the command runner.
package config

import (
	"fmt"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/variable"
)

// Template field parameter usage constraints:
//
//   Field    | ${param} | ${?param} | ${@param} | In Key (env_vars only) | Override at call site
//   ---------|----------|-----------|-----------|------------------------|----------------------
//   cmd      |    ✓     |     ✓     |     ✗     |          N/A           |        ✗
//   args     |    ✓     |     ✓     |     ✓     |          N/A           |        ✗
//   env_vars |    ✓     |     ✓     |  ✓ (※1)  |        ✗ (※2)         |        ✗
//   workdir  |    ✓     |     ✓     |     ✗     |          N/A           |   ✓ (※3)
//
// Rationale:
//   - cmd, workdir: Must expand to exactly one string value
//   - args: Can expand to multiple strings (array expansion at element level)
//   - env_vars:
//     ※1 Array expansion is allowed at element level (e.g., env_vars = ["${@vars}"])
//        but NOT in VALUE part (e.g., env_vars = ["PATH=${@paths}"] is invalid)
//     ※2 KEY part (before '=') cannot contain any placeholders (security constraint)
//   - workdir override:
//     ※3 Caller can override template's workdir to adjust execution context
//        (useful when coordinating output directories between commands)
//
// Examples:
//   ✓ cmd = "${binary}"                    # OK: single value
//   ✗ cmd = "${@bins}"                     # Error: array not allowed
//   ✓ args = ["${@flags}", "${file}"]      # OK: array expansion
//   ✓ env_vars = ["${@env_vars}"]          # OK: element-level array expansion
//   ✗ env_vars = ["PATH=${@paths}"]        # Error: array in VALUE part
//   ✗ env_vars = ["${key}=value"]          # Error: placeholder in KEY part
//   ✓ env_vars = ["KEY=${value}"]          # OK: placeholder in VALUE part only
//   ✓ workdir = "${dir}"                   # OK: single value (in template)
//   ✗ workdir = "${@dirs}"                 # Error: array not allowed
//   ✓ workdir = "%{work_dir}/temp"         # OK: override at call site (uses %{} not ${})

// placeholderType represents the type of a template placeholder.
type placeholderType int

const (
	placeholderRequired placeholderType = iota // ${param}
	placeholderOptional                        // ${?param}
	placeholderArray                           // ${@param}
)

// placeholderPrefixLen is the length of the placeholder prefix "${".
const placeholderPrefixLen = 2

// field name
const workDirKey = "workdir"

// placeholder represents a parsed placeholder in a template string.
type placeholder struct {
	fullMatch string          // The full match including ${...}
	name      string          // The parameter name
	ptype     placeholderType // The placeholder type
	start     int             // Start position in the input string
	end       int             // End position in the input string
}

// parsePlaceholders extracts all placeholders from an input string.
//
// Grammar:
//
//	placeholder := "${" modifier? name "}"
//	modifier    := "?" | "@"
//	name        := [A-Za-z_][A-Za-z0-9_]*
//
// Returns placeholders in order of appearance.
func parsePlaceholders(input string) ([]placeholder, error) {
	var placeholders []placeholder
	i := 0

	for i < len(input) {
		// Handle escape sequence (\$, \\)
		if i+1 < len(input) && input[i] == '\\' {
			nextChar := input[i+1]
			if nextChar == '$' || nextChar == '\\' {
				i += 2
				continue
			}
		}

		// Check for placeholder start
		if i+placeholderPrefixLen < len(input) && input[i] == '$' && input[i+1] == '{' {
			// Find closing brace
			closeIdx := strings.IndexByte(input[i+placeholderPrefixLen:], '}')
			if closeIdx == -1 {
				return nil, &ErrUnclosedPlaceholder{
					Input:    input,
					Position: i,
				}
			}
			closeIdx += i + placeholderPrefixLen

			// Extract content between ${ and }
			content := input[i+placeholderPrefixLen : closeIdx]
			if content == "" {
				return nil, &ErrEmptyPlaceholder{
					Input:    input,
					Position: i,
				}
			}

			// Determine type and extract name
			var ptype placeholderType
			var name string

			switch content[0] {
			case '?':
				ptype = placeholderOptional
				name = content[1:]
			case '@':
				ptype = placeholderArray
				name = content[1:]
			default:
				ptype = placeholderRequired
				name = content
			}

			// Validate name
			if name == "" {
				return nil, &ErrEmptyPlaceholderName{
					Input:    input,
					Position: i,
				}
			}
			if err := security.ValidateVariableName(name); err != nil {
				return nil, &ErrInvalidPlaceholderName{
					Input:    input,
					Position: i,
					Name:     name,
					Reason:   err.Error(),
				}
			}

			placeholders = append(placeholders, placeholder{
				fullMatch: input[i : closeIdx+1],
				name:      name,
				ptype:     ptype,
				start:     i,
				end:       closeIdx + 1,
			})

			i = closeIdx + 1
			continue
		}

		i++
	}

	return placeholders, nil
}

// applyEscapeSequences applies escape sequence transformation.
// Supported escape sequences:
//   - \$ -> $
//   - \\ -> \
//
// This is consistent with the existing variable expansion escape sequences:
//   - \% -> %
//   - \\ -> \
func applyEscapeSequences(input string) string {
	var result strings.Builder
	i := 0

	for i < len(input) {
		if i+1 < len(input) && input[i] == '\\' {
			nextChar := input[i+1]
			if nextChar == '$' || nextChar == '\\' {
				result.WriteByte(nextChar)
				i += 2
				continue
			}
		}
		result.WriteByte(input[i])
		i++
	}

	return result.String()
}

// expandSingleArg expands placeholders in a single argument string.
//
// Expansion modes:
//  1. Pure array placeholder: "${@param}" alone in the string
//     - Returns array elements directly
//  2. Pure optional placeholder: "${?param}" alone in the string
//     - Returns empty slice if param is empty/missing
//  3. String replacement: any other case
//     - Replaces placeholders with string values
//     - ${?param} with empty value removes that portion
//     - ${@param} in mixed context is an error
func expandSingleArg(
	arg string,
	params map[string]any,
	templateName string,
	field string,
) ([]string, error) {
	placeholders, err := parsePlaceholders(arg)
	if err != nil {
		return nil, err
	}

	// No placeholders - return as-is (after applying escape sequences)
	if len(placeholders) == 0 {
		return []string{applyEscapeSequences(arg)}, nil
	}

	// Check for pure array placeholder
	if len(placeholders) == 1 && placeholders[0].ptype == placeholderArray {
		ph := placeholders[0]
		if arg == ph.fullMatch {
			// Pure array placeholder
			return expandArrayPlaceholder(ph.name, params, templateName, field)
		}
		// Array placeholder in mixed context
		return nil, &ErrArrayInMixedContext{
			TemplateName: templateName,
			Field:        field,
			ParamName:    ph.name,
		}
	}

	// Check for pure optional placeholder
	if len(placeholders) == 1 && placeholders[0].ptype == placeholderOptional {
		ph := placeholders[0]
		if arg == ph.fullMatch {
			// Pure optional placeholder
			return expandOptionalPlaceholder(ph.name, params, templateName, field)
		}
	}

	// String replacement mode
	return expandStringPlaceholders(arg, placeholders, params, templateName, field)
}

// expandArrayPlaceholder expands a ${@param} placeholder.
func expandArrayPlaceholder(
	name string,
	params map[string]any,
	templateName string,
	field string,
) ([]string, error) {
	// Array placeholders are not allowed in workdir field - it must
	// expand to a single value (one directory path)
	if field == workDirKey {
		return nil, &ErrArrayInMixedContext{
			TemplateName: templateName,
			Field:        field,
			ParamName:    name,
		}
	}

	value, exists := params[name]
	if !exists {
		// Array param not provided - return empty (element removed)
		return []string{}, nil
	}

	// Type check
	switch v := value.(type) {
	case []any:
		result := make([]string, len(v))
		for i, elem := range v {
			str, ok := elem.(string)
			if !ok {
				return nil, &ErrTemplateInvalidArrayElement{
					TemplateName: templateName,
					Field:        field,
					ParamName:    name,
					Index:        i,
					ActualType:   fmt.Sprintf("%T", elem),
				}
			}
			result[i] = str
		}
		return result, nil

	case []string:
		return v, nil

	case string:
		return nil, &ErrTemplateTypeMismatch{
			TemplateName: templateName,
			Field:        field,
			ParamName:    name,
			Expected:     "array",
			Actual:       "string",
		}

	default:
		return nil, &ErrUnsupportedParamType{
			TemplateName: templateName,
			Field:        field,
			ParamName:    name,
			ActualType:   fmt.Sprintf("%T", value),
		}
	}
}

// expandOptionalPlaceholder expands a ${?param} placeholder.
func expandOptionalPlaceholder(
	name string,
	params map[string]any,
	templateName string,
	field string,
) ([]string, error) {
	value, exists := params[name]
	if !exists {
		return []string{}, nil // Element removed
	}

	str, ok := value.(string)
	if !ok {
		return nil, &ErrTemplateTypeMismatch{
			TemplateName: templateName,
			Field:        field,
			ParamName:    name,
			Expected:     "string",
			Actual:       fmt.Sprintf("%T", value),
		}
	}

	if str == "" {
		return []string{}, nil // Element removed
	}

	return []string{str}, nil
}

// expandStringPlaceholders performs string replacement for placeholders.
func expandStringPlaceholders(
	input string,
	placeholders []placeholder,
	params map[string]any,
	templateName string,
	field string,
) ([]string, error) {
	result := input

	// Process placeholders in reverse order to maintain positions
	for i := len(placeholders) - 1; i >= 0; i-- {
		ph := placeholders[i]

		// Array placeholders in mixed context are not allowed
		if ph.ptype == placeholderArray {
			return nil, &ErrArrayInMixedContext{
				TemplateName: templateName,
				Field:        field,
				ParamName:    ph.name,
			}
		}

		value, exists := params[ph.name]

		switch ph.ptype {
		case placeholderRequired:
			if !exists {
				return nil, &ErrRequiredParamMissing{
					TemplateName: templateName,
					Field:        field,
					ParamName:    ph.name,
				}
			}
			str, ok := value.(string)
			if !ok {
				return nil, &ErrTemplateTypeMismatch{
					TemplateName: templateName,
					Field:        field,
					ParamName:    ph.name,
					Expected:     "string",
					Actual:       fmt.Sprintf("%T", value),
				}
			}
			result = result[:ph.start] + str + result[ph.end:]

		case placeholderOptional:
			var replacement string
			if exists {
				str, ok := value.(string)
				if !ok {
					return nil, &ErrTemplateTypeMismatch{
						TemplateName: templateName,
						Field:        field,
						ParamName:    ph.name,
						Expected:     "string",
						Actual:       fmt.Sprintf("%T", value),
					}
				}
				replacement = str
			}
			result = result[:ph.start] + replacement + result[ph.end:]
		}
	}

	// Apply escape sequences after placeholder expansion
	result = applyEscapeSequences(result)

	// Check if result is empty after optional expansion
	if result == "" {
		return []string{}, nil
	}

	return []string{result}, nil
}

// ExpandTemplateArgs expands all placeholders in a template's args array.
func ExpandTemplateArgs(
	args []string,
	params map[string]any,
	templateName string,
) ([]string, error) {
	var result []string

	for i, arg := range args {
		field := fmt.Sprintf("args[%d]", i)
		expanded, err := expandSingleArg(arg, params, templateName, field)
		if err != nil {
			return nil, err
		}
		result = append(result, expanded...)
	}

	return result, nil
}

// ExpandTemplateEnv expands all placeholders in a template's env_vars array.
// Each element must expand to valid KEY=VALUE format(s).
// Placeholders in the KEY part are forbidden for security reasons.
func ExpandTemplateEnv(
	env []string,
	params map[string]any,
	templateName string,
) ([]string, error) {
	result := make([]string, 0, len(env))

	for i, envEntry := range env {
		field := fmt.Sprintf("env_vars[%d]", i)

		// Pre-validate: check if KEY part contains placeholders (before expansion)
		if err := validateEnvPre(envEntry, templateName, field); err != nil {
			return nil, err
		}

		// Expand the entry (may expand to multiple elements for ${@param})
		expanded, err := expandSingleArg(envEntry, params, templateName, field)
		if err != nil {
			return nil, err
		}

		// Skip if expansion resulted in empty (e.g., ${?param} with missing/empty value)
		if len(expanded) == 0 {
			continue
		}

		// Post-validate: check each expanded element is in KEY=VALUE format
		for j, entry := range expanded {
			shouldInclude, err := validateEnvPost(entry, templateName, field, j)
			if err != nil {
				return nil, err
			}
			if shouldInclude {
				result = append(result, entry)
			}
		}
	}

	// Validate no duplicate keys in the expanded env array
	if err := validateEnvUnique(result, templateName); err != nil {
		return nil, err
	}

	return result, nil
}

// validateEnvPre validates env_vars entry before placeholder expansion.
// This checks that the KEY part (before '=') does not contain placeholders.
func validateEnvPre(entry, templateName, _ string) error {
	// Check if this is a pure placeholder (entire element is ${...} or ${?...} or ${@...})
	placeholders, err := parsePlaceholders(entry)
	if err != nil {
		return err
	}

	// If entire entry is a single placeholder, we'll validate after expansion
	if len(placeholders) == 1 && entry == placeholders[0].fullMatch {
		return nil
	}

	// Parse KEY=VALUE to check KEY part
	idx := strings.IndexByte(entry, '=')
	if idx == -1 {
		// No '=' found - could be invalid format or pure placeholder
		// Will be caught in post-validation
		return nil
	}

	key := entry[:idx]

	// Check that KEY part does not contain placeholders (security requirement)
	keyPlaceholders, err := parsePlaceholders(key)
	if err != nil {
		return fmt.Errorf("failed to parse env_vars key %q: %w", key, err)
	}
	if len(keyPlaceholders) > 0 {
		return &ErrPlaceholderInEnvKey{
			TemplateName: templateName,
			EnvEntry:     entry,
			Key:          key,
		}
	}

	return nil
}

// validateEnvPost validates that an env_vars entry is in KEY=VALUE format
// after placeholder expansion.
// Returns (shouldInclude=false, nil) if the entry should be skipped (empty VALUE).
func validateEnvPost(entry, templateName, field string, expandedIndex int) (bool, error) {
	// Check KEY=VALUE format
	idx := strings.IndexByte(entry, '=')
	if idx == -1 {
		return false, &ErrTemplateInvalidEnvFormat{
			TemplateName:  templateName,
			Field:         field,
			ExpandedIndex: expandedIndex,
			Entry:         entry,
		}
	}

	// Check if VALUE part is empty (e.g., "PATH=" from "PATH=${?path}" with empty/missing param)
	// In this case, skip the entire entry
	value := entry[idx+1:]
	if value == "" {
		return false, nil
	}

	// Note: KEY part placeholder validation is done in validateEnvPre
	// This function only validates the format after expansion

	return true, nil
}

// validateEnvUnique validates that there are no duplicate environment variable keys
// in the expanded env_vars array.
func validateEnvUnique(env []string, templateName string) error {
	seen := make(map[string]struct{}, len(env))

	for _, entry := range env {
		// Extract KEY from "KEY=VALUE"
		idx := strings.IndexByte(entry, '=')
		if idx == -1 {
			// Format error should have been caught by validateEnvPost
			continue
		}

		key := entry[:idx]

		// Check for duplicate
		if _, exists := seen[key]; exists {
			return &ErrDuplicateEnvVariableDetail{
				TemplateName: templateName,
				Field:        "env_vars",
				EnvKey:       key,
			}
		}
		seen[key] = struct{}{}
	}

	return nil
}

// ValidateTemplateName validates that a template name is valid and not reserved.
//
// Rules:
//  1. Must pass ValidateVariableName (letter/underscore start, alphanumeric)
//  2. Must not start with "__" (reserved for future use)
func ValidateTemplateName(name string) error {
	// Basic variable name validation
	if err := security.ValidateVariableName(name); err != nil {
		return &ErrInvalidTemplateName{
			Name:   name,
			Reason: err.Error(),
		}
	}

	// Check for reserved prefix
	if strings.HasPrefix(name, "__") {
		return &ErrReservedTemplateName{
			Name: name,
		}
	}

	return nil
}

// ValidateTemplateDefinition validates a template definition for security.
//
// This function enforces variable reference rules in templates:
//   - Global variables (%{GlobalVar} - uppercase start) ARE allowed
//   - Local variables (%{local_var} - lowercase start) are NOT allowed
//
// Rationale:
//   - Templates are reused across multiple groups with different variable contexts
//   - Global variables are safe because they're defined once at [global.vars]
//   - Local variables would create context-dependent security issues
//   - Variable references should be explicit in params for local variables
//
// Note: This function only validates that %{} references follow naming rules.
// The actual validation of whether global variables are defined happens in
// ValidateTemplateVariableReferences() which is called after global vars are loaded.
func ValidateTemplateDefinition(
	name string,
	template *runnertypes.CommandTemplate,
) error {
	// Check cmd is not empty (REQUIRED field)
	if template.Cmd == "" {
		return &ErrMissingRequiredField{
			TemplateName: name,
			Field:        "cmd",
		}
	}

	// Check cmd for local variable references (lowercase/underscore start)
	if err := validateGlobalOnly(template.Cmd, name, "cmd"); err != nil {
		return err
	}

	// Check args for local variable references
	for i, arg := range template.Args {
		if err := validateGlobalOnly(arg, name, fmt.Sprintf("args[%d]", i)); err != nil {
			return err
		}
	}

	// Check env_vars for local variable references
	for i, env := range template.EnvVars {
		if err := validateGlobalOnly(env, name, fmt.Sprintf("env_vars[%d]", i)); err != nil {
			return err
		}
	}

	// Check workdir for local variable references (if non-nil)
	if template.WorkDir != nil {
		if err := validateGlobalOnly(*template.WorkDir, name, workDirKey); err != nil {
			return err
		}
	}

	return nil
}

// validateGlobalOnly checks that a string does not contain
// references to local variables (lowercase or underscore start).
// Global variable references (uppercase start) are allowed.
func validateGlobalOnly(input, templateName, field string) error {
	// Empty strings cannot contain variable references, skip validation
	if input == "" {
		return nil
	}

	// Collect all %{VAR} references
	var refs []string
	refCollector := func(
		varName string,
		_ string,
		_ map[string]struct{},
		_ []string,
		_ int,
	) (string, error) {
		refs = append(refs, varName)
		return "", nil
	}

	// Use processVarRefs to extract variable names
	_, err := processVarRefs(
		input,
		refCollector,
		fmt.Sprintf("template[%s]", templateName),
		field,
		make(map[string]struct{}),
		make([]string, 0),
		0,
	)
	if err != nil {
		// processVarRefs validates variable names and reports errors
		return err
	}

	// Check each reference - only global variables (uppercase start) are allowed
	for _, varName := range refs {
		if len(varName) == 0 {
			continue
		}

		scope, err := variable.DetermineScope(varName)
		if err != nil {
			return fmt.Errorf("template %q field %q: invalid variable name %q: %w", templateName, field, varName, err)
		}

		if scope != variable.ScopeGlobal {
			return &ErrLocalVariableInTemplate{
				TemplateName: templateName,
				Field:        field,
				VariableName: varName,
			}
		}
	}

	return nil
}

// ValidateParams validates template parameter values.
//
// This function validates:
//  1. Parameter names must be valid variable names (letter/underscore start, alphanumeric)
//  2. Parameter values must be string or []string ([]any with string elements)
//
// NOTE: %{var} references in params values ARE allowed (NF-006) because they
// will be expanded after template expansion using the group's variable context.
func ValidateParams(params map[string]any, templateName string) error {
	for paramName, value := range params {
		// Validate parameter name
		if err := security.ValidateVariableName(paramName); err != nil {
			return &ErrInvalidParamName{
				TemplateName: templateName,
				ParamName:    paramName,
				Reason:       err.Error(),
			}
		}

		// Validate parameter value type
		switch v := value.(type) {
		case string:
			// String is valid, %{var} references allowed (NF-006)
			continue
		case []any:
			// Check that all elements are strings
			for i, elem := range v {
				if _, ok := elem.(string); !ok {
					return &ErrTemplateInvalidArrayElement{
						TemplateName: templateName,
						Field:        "params",
						ParamName:    paramName,
						Index:        i,
						ActualType:   fmt.Sprintf("%T", elem),
					}
				}
			}
		case []string:
			// Already validated as string array
			continue
		default:
			return &ErrUnsupportedParamType{
				TemplateName: templateName,
				Field:        "params",
				ParamName:    paramName,
				ActualType:   fmt.Sprintf("%T", value),
			}
		}
	}

	return nil
}

// validateCmdSpec validates that template and command fields
// are mutually exclusive in a CommandSpec.
//
// When Template is set, the following fields MUST NOT be set:
//   - Cmd
//   - Args
//   - Env
//
// The following fields CAN be set with Template (override template defaults):
//   - WorkDir (execution context)
//   - OutputFile (output redirection)
//   - Timeout, RiskLevel, etc. (execution parameters)
//
// The Name and Params fields are allowed with Template.
//
// This enforces separation between:
//   - Template: defines command execution logic (cmd, args, env_vars)
//   - Caller: specifies execution context (workdir, output, etc.)
func validateCmdSpec(
	groupName string,
	commandIndex int,
	spec *runnertypes.CommandSpec,
) error {
	if spec.Template == "" {
		// Not using template, normal command definition
		// Cmd is required
		if spec.Cmd == "" {
			return &ErrMissingRequiredField{
				GroupName:    groupName,
				CommandIndex: commandIndex,
				Field:        "cmd",
			}
		}
		return nil
	}

	// Using template, check for conflicting fields
	if spec.Cmd != "" {
		return &ErrTemplateFieldConflict{
			GroupName:    groupName,
			CommandIndex: commandIndex,
			TemplateName: spec.Template,
			Field:        "cmd",
		}
	}

	if spec.Args != nil {
		return &ErrTemplateFieldConflict{
			GroupName:    groupName,
			CommandIndex: commandIndex,
			TemplateName: spec.Template,
			Field:        "args",
		}
	}

	if spec.EnvVars != nil {
		return &ErrTemplateFieldConflict{
			GroupName:    groupName,
			CommandIndex: commandIndex,
			TemplateName: spec.Template,
			Field:        "env_vars",
		}
	}

	// WorkDir is allowed with Template (overrides template default)
	// OutputFile is allowed with Template (specifies output redirection)
	// Timeout, RiskLevel, etc. are allowed with Template (execution parameters)

	// Name and Params are allowed with Template
	return nil
}

// ExpandTemplateEnvImport expands all placeholders in a template's env_import array.
// Each element can be:
//   - A simple string like "CC" (no expansion needed)
//   - A mapping like "internal_name=SYSTEM_VAR" (no expansion needed)
//   - A placeholder like "${?extra_env}" (expands to value or removed if empty)
//   - An array placeholder like "${@env_vars}" (expands to multiple elements)
func ExpandTemplateEnvImport(
	envImport []string,
	params map[string]any,
	templateName string,
) ([]string, error) {
	result := make([]string, 0, len(envImport))

	for i, entry := range envImport {
		field := fmt.Sprintf("env_import[%d]", i)

		// Expand the entry (may expand to multiple elements for ${@param})
		expanded, err := expandSingleArg(entry, params, templateName, field)
		if err != nil {
			return nil, err
		}

		// Skip if expansion resulted in empty (e.g., ${?param} with missing/empty value)
		if len(expanded) == 0 {
			continue
		}

		result = append(result, expanded...)
	}

	return result, nil
}

// ExpandTemplateVars expands all placeholders in a template's vars map.
// Only the values are expanded, not the keys.
// String values and array element values can contain placeholders.
func ExpandTemplateVars(
	vars map[string]any,
	params map[string]any,
	templateName string,
) (map[string]any, error) {
	if len(vars) == 0 {
		return make(map[string]any), nil
	}

	result := make(map[string]any, len(vars))

	for varName, varValue := range vars {
		field := fmt.Sprintf("vars.%s", varName)

		switch v := varValue.(type) {
		case string:
			// Expand string value
			// Note: expandSingleArg returns []string, but for vars values,
			// we need to handle different cases:
			// - ${param} or ${?param} with value: single string (join if multiple, but should be one)
			// - ${?param} with empty/missing: should result in empty string
			// - ${@param}: not allowed in vars string values (mixed context)
			expanded, err := expandSingleArg(v, params, templateName, field)
			if err != nil {
				return nil, err
			}

			switch len(expanded) {
			case 0:
				// Optional parameter was empty/missing - store as empty string
				result[varName] = ""
			case 1:
				result[varName] = expanded[0]
			default:
				// This shouldn't happen for string values (array expansion should error)
				return nil, &ErrTemplateVarUnexpectedMultipleValues{
					TemplateName: templateName,
					Field:        field,
				}
			}

		case []any:
			// Expand array elements
			expandedArray := make([]any, 0, len(v))
			for i, elem := range v {
				str, ok := elem.(string)
				if !ok {
					return nil, &ErrTemplateInvalidArrayElement{
						TemplateName: templateName,
						Field:        field,
						ParamName:    varName,
						Index:        i,
						ActualType:   fmt.Sprintf("%T", elem),
					}
				}

				elemField := fmt.Sprintf("%s[%d]", field, i)
				expanded, err := expandSingleArg(str, params, templateName, elemField)
				if err != nil {
					return nil, err
				}

				// Add expanded elements (may be 0, 1, or multiple)
				for _, exp := range expanded {
					expandedArray = append(expandedArray, exp)
				}
			}
			result[varName] = expandedArray

		case []string:
			// Expand array elements
			expandedArray := make([]any, 0, len(v))
			for i, str := range v {
				elemField := fmt.Sprintf("%s[%d]", field, i)
				expanded, err := expandSingleArg(str, params, templateName, elemField)
				if err != nil {
					return nil, err
				}

				// Add expanded elements (may be 0, 1, or multiple)
				for _, exp := range expanded {
					expandedArray = append(expandedArray, exp)
				}
			}
			result[varName] = expandedArray

		default:
			return nil, &ErrUnsupportedParamType{
				TemplateName: templateName,
				Field:        "vars",
				ParamName:    varName,
				ActualType:   fmt.Sprintf("%T", varValue),
			}
		}
	}

	return result, nil
}

// CollectUsedParams extracts all parameter names used in a template.
// This is used for:
//  1. Required params validation
//  2. Unused params warning
func CollectUsedParams(template *runnertypes.CommandTemplate) (map[string]struct{}, error) {
	used := make(map[string]struct{})

	// Collect from basic fields
	if err := collectFromBasicFields(template, used); err != nil {
		return nil, err
	}

	// Collect from env_vars
	if err := collectFromEnvVars(template.EnvVars, used); err != nil {
		return nil, err
	}

	// Collect from optional fields
	if err := collectFromOptionalFields(template, used); err != nil {
		return nil, err
	}

	// Collect from vars
	if err := collectFromVars(template.Vars, used); err != nil {
		return nil, err
	}

	return used, nil
}

// collectFromBasicFields collects params from cmd and args.
func collectFromBasicFields(template *runnertypes.CommandTemplate, used map[string]struct{}) error {
	if err := collectFromString(template.Cmd, used); err != nil {
		return err
	}

	for _, arg := range template.Args {
		if err := collectFromString(arg, used); err != nil {
			return err
		}
	}

	return nil
}

// collectFromEnvVars collects params from env_vars array.
func collectFromEnvVars(envVars []string, used map[string]struct{}) error {
	for _, env := range envVars {
		if idx := strings.IndexByte(env, '='); idx != -1 {
			// KEY=VALUE format - collect from value part only
			if err := collectFromString(env[idx+1:], used); err != nil {
				return err
			}
		} else {
			// No '=' - this might be element-level expansion like "${@env_vars}"
			if err := collectFromString(env, used); err != nil {
				return err
			}
		}
	}
	return nil
}

// collectFromOptionalFields collects params from optional pointer fields and env_import.
func collectFromOptionalFields(template *runnertypes.CommandTemplate, used map[string]struct{}) error {
	// Collect from workdir (if non-nil and non-empty)
	if template.WorkDir != nil && *template.WorkDir != "" {
		if err := collectFromString(*template.WorkDir, used); err != nil {
			return err
		}
	}

	// Collect from output_file (if non-nil and non-empty)
	if template.OutputFile != nil && *template.OutputFile != "" {
		if err := collectFromString(*template.OutputFile, used); err != nil {
			return err
		}
	}

	// Collect from env_import
	for _, envImport := range template.EnvImport {
		if err := collectFromString(envImport, used); err != nil {
			return err
		}
	}

	return nil
}

// collectFromVars collects params from vars map values.
func collectFromVars(vars map[string]any, used map[string]struct{}) error {
	for _, varValue := range vars {
		switch v := varValue.(type) {
		case string:
			if err := collectFromString(v, used); err != nil {
				return err
			}
		case []any:
			for _, elem := range v {
				if str, ok := elem.(string); ok {
					if err := collectFromString(str, used); err != nil {
						return err
					}
				}
			}
		case []string:
			for _, str := range v {
				if err := collectFromString(str, used); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// collectFromString extracts parameter names from placeholders in a string.
func collectFromString(input string, used map[string]struct{}) error {
	placeholders, err := parsePlaceholders(input)
	if err != nil {
		return err
	}

	for _, ph := range placeholders {
		used[ph.name] = struct{}{}
	}

	return nil
}

// ValidateTemplateVars validates that all variable references in a template
// follow the rules for template variables:
// - Only global variables (uppercase start) can be referenced
// - All referenced variables must be defined in [global.vars]
//
// This function checks all fields that may contain variable references:
// - cmd, workdir (string fields)
// - args (string array)
// - env (string array with KEY=value format)
func ValidateTemplateVars(
	template *runnertypes.CommandTemplate,
	templateName string,
	globalVars map[string]string,
) error {
	// Check cmd field
	if template.Cmd != "" {
		if err := validateFieldVars(template.Cmd, templateName, "cmd", globalVars); err != nil {
			return err
		}
	}

	// Check workdir field (if non-nil and non-empty)
	if template.WorkDir != nil && *template.WorkDir != "" {
		if err := validateFieldVars(*template.WorkDir, templateName, "workdir", globalVars); err != nil {
			return err
		}
	}

	// Check args array
	for i, arg := range template.Args {
		fieldName := fmt.Sprintf("args[%d]", i)
		if err := validateFieldVars(arg, templateName, fieldName, globalVars); err != nil {
			return err
		}
	}

	// Check env_vars array
	for i, envMapping := range template.EnvVars {
		// Parse "KEY=value" format
		const envSplitParts = 2
		parts := strings.SplitN(envMapping, "=", envSplitParts)
		if len(parts) != envSplitParts {
			// Invalid format - this should have been caught during parsing
			// but we check here for defensive programming
			continue
		}
		value := parts[1]

		fieldName := fmt.Sprintf("env_vars[%d]", i)
		if err := validateFieldVars(value, templateName, fieldName, globalVars); err != nil {
			return err
		}
	}

	return nil
}

// validateFieldVars validates variable references in a single string field
// by collecting all %{VAR} references and checking each one against scope and definition rules.
//
// This function reuses the parsing logic from processVarRefs (which is used for actual variable
// expansion) to ensure consistent extraction of variable references across the codebase.
func validateFieldVars(
	input string,
	templateName string,
	fieldName string,
	globalVars map[string]string,
) error {
	// Import the variable package for scope determination
	// Note: This creates a dependency from config to variable package
	// which is acceptable since variable scope validation is a core feature

	// Collect all %{VAR} references by using a resolver that saves variable names
	// instead of expanding them. This reuses the same parsing logic as expansion.go.
	var collectedRefs []string
	refCollector := func(
		varName string,
		_ string,
		_ map[string]struct{},
		_ []string,
		_ int,
	) (string, error) {
		// Just collect the variable name without resolving
		collectedRefs = append(collectedRefs, varName)
		// Return non-empty string to satisfy resolver interface
		return "", nil
	}

	// Use processVarRefs to extract variable names using the same logic as expansion
	// If processVarRefs returns an error (e.g., unclosed %{), report it as-is
	_, err := processVarRefs(
		input,
		refCollector,
		fmt.Sprintf("template[%s]", templateName),
		fieldName,
		make(map[string]struct{}), // empty visited set
		make([]string, 0),         // empty expansion chain
		0,                         // depth 0
	)
	if err != nil {
		// processVarRefs validates variable names and reports errors
		// If there's a parsing error, return it as-is
		return err
	}

	// Now validate each collected reference
	for _, varName := range collectedRefs {
		if len(varName) == 0 {
			continue
		}

		scope, err := variable.DetermineScope(varName)
		if err != nil {
			return fmt.Errorf("template %q field %q: invalid variable name %q: %w", templateName, fieldName, varName, err)
		}

		// Check if it's a local variable (not allowed in templates)
		if scope != variable.ScopeGlobal {
			return &ErrLocalVariableInTemplate{
				TemplateName: templateName,
				Field:        fieldName,
				VariableName: varName,
			}
		}

		// Check if the global variable is defined
		if _, exists := globalVars[varName]; !exists {
			return &ErrUndefinedGlobalVariableInTemplate{
				TemplateName: templateName,
				Field:        fieldName,
				VariableName: varName,
			}
		}
	}

	return nil
}

// ValidateAllTemplates validates all templates in the configuration
// This is called during config loading, after global vars are processed
func ValidateAllTemplates(
	templates map[string]runnertypes.CommandTemplate,
	globalVars map[string]string,
) error {
	for templateName, template := range templates {
		if err := ValidateTemplateVars(&template, templateName, globalVars); err != nil {
			return err
		}
	}
	return nil
}
