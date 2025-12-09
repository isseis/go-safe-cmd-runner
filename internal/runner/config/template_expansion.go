// Package config provides configuration loading and validation for the command runner.
package config

import (
	"fmt"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// placeholderType represents the type of a template placeholder.
type placeholderType int

const (
	placeholderRequired placeholderType = iota // ${param}
	placeholderOptional                        // ${?param}
	placeholderArray                           // ${@param}
)

// placeholderPrefixLen is the length of the placeholder prefix "${".
const placeholderPrefixLen = 2

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
	params map[string]interface{},
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
	params map[string]interface{},
	templateName string,
	field string,
) ([]string, error) {
	value, exists := params[name]
	if !exists {
		// Array param not provided - return empty (element removed)
		return []string{}, nil
	}

	// Type check
	switch v := value.(type) {
	case []interface{}:
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
	params map[string]interface{},
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
	params map[string]interface{},
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
	params map[string]interface{},
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
// This function enforces NF-006: Variable references (%{var}) are NOT allowed
// in template definitions to prevent context-dependent security issues.
//
// Rationale:
//   - Templates are reused across multiple groups with different variable contexts
//   - A variable reference safe in one group may expose secrets in another group
//   - Variable references should be explicit in params, not hidden in templates
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

	// Check cmd for forbidden %{ pattern
	if strings.Contains(template.Cmd, "%{") {
		return &ErrForbiddenPatternInTemplate{
			TemplateName: name,
			Field:        "cmd",
			Value:        template.Cmd,
		}
	}

	// Check args for forbidden %{ pattern
	for i, arg := range template.Args {
		if strings.Contains(arg, "%{") {
			return &ErrForbiddenPatternInTemplate{
				TemplateName: name,
				Field:        fmt.Sprintf("args[%d]", i),
				Value:        arg,
			}
		}
	}

	// Check env for forbidden %{ pattern
	for i, env := range template.Env {
		if strings.Contains(env, "%{") {
			return &ErrForbiddenPatternInTemplate{
				TemplateName: name,
				Field:        fmt.Sprintf("env[%d]", i),
				Value:        env,
			}
		}
	}

	// Check workdir for forbidden %{ pattern
	if template.WorkDir != "" && strings.Contains(template.WorkDir, "%{") {
		return &ErrForbiddenPatternInTemplate{
			TemplateName: name,
			Field:        "workdir",
			Value:        template.WorkDir,
		}
	}

	return nil
}

// ValidateParams validates template parameter values.
//
// This function validates:
//  1. Parameter names must be valid variable names (letter/underscore start, alphanumeric)
//  2. Parameter values must be string or []string ([]interface{} with string elements)
//
// NOTE: %{var} references in params values ARE allowed (NF-006) because they
// will be expanded after template expansion using the group's variable context.
func ValidateParams(params map[string]interface{}, templateName string) error {
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
		case []interface{}:
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

// ValidateCommandSpecExclusivity validates that template and command fields
// are mutually exclusive in a CommandSpec.
//
// When Template is set, the following fields MUST NOT be set:
//   - Cmd
//   - Args
//   - Env
//   - WorkDir
//
// The Name and Params fields are allowed with Template.
//
// This enforces the "complete exclusivity" design where templates provide
// all command execution fields, and the calling site can only specify
// Name and Params.
func ValidateCommandSpecExclusivity(
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

	if len(spec.Args) > 0 {
		return &ErrTemplateFieldConflict{
			GroupName:    groupName,
			CommandIndex: commandIndex,
			TemplateName: spec.Template,
			Field:        "args",
		}
	}

	if len(spec.EnvVars) > 0 {
		return &ErrTemplateFieldConflict{
			GroupName:    groupName,
			CommandIndex: commandIndex,
			TemplateName: spec.Template,
			Field:        "env_vars",
		}
	}

	if spec.WorkDir != "" {
		return &ErrTemplateFieldConflict{
			GroupName:    groupName,
			CommandIndex: commandIndex,
			TemplateName: spec.Template,
			Field:        "workdir",
		}
	}

	// Name and Params are allowed with Template
	return nil
}

// CollectUsedParams extracts all parameter names used in a template.
// This is used for:
//  1. Required params validation
//  2. Unused params warning
func CollectUsedParams(template *runnertypes.CommandTemplate) (map[string]struct{}, error) {
	used := make(map[string]struct{})

	// Collect from cmd
	if err := collectFromString(template.Cmd, used); err != nil {
		return nil, err
	}

	// Collect from args
	for _, arg := range template.Args {
		if err := collectFromString(arg, used); err != nil {
			return nil, err
		}
	}

	// Collect from env (only the value part after =)
	for _, env := range template.Env {
		if idx := strings.IndexByte(env, '='); idx != -1 {
			if err := collectFromString(env[idx+1:], used); err != nil {
				return nil, err
			}
		}
	}

	// Collect from workdir
	if template.WorkDir != "" {
		if err := collectFromString(template.WorkDir, used); err != nil {
			return nil, err
		}
	}

	return used, nil
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
