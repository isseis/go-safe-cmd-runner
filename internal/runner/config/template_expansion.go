// Package config provides configuration loading and validation for the command runner.
package config

import (
	"fmt"
	"strings"

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
