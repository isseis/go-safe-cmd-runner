// Package expansion provides variable expansion functionality for cmd/args
// with support for both $VAR and ${VAR} formats using unified regex approach.
package expansion

import (
	"regexp"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
)

// Unified regular expression approach: handle both formats simultaneously using named groups
var (
	// Both formats unified pattern: $VAR or ${VAR}
	unifiedVariablePattern = regexp.MustCompile(`\$(\{(?P<braced>[a-zA-Z_][0-9a-zA-Z_]*)\}|(?P<simple>[a-zA-Z_][0-9a-zA-Z_]*))`)

	// Named capture group configuration:
	// - braced: ${VAR_NAME} format VAR_NAME part
	// - simple: $VAR_NAME format VAR_NAME part

	// Pre-computed subexpression indices for performance
	bracedGroupIndex = unifiedVariablePattern.SubexpIndex("braced")
	simpleGroupIndex = unifiedVariablePattern.SubexpIndex("simple")

	// Legacy pattern removed as unified pattern handles all cases
)

// variableParser handles both format parsing
type variableParser struct {
	// Simple implementation using only regular expressions
}

// NewVariableParser creates a new parser
func NewVariableParser() VariableParser {
	return &variableParser{}
}

// ReplaceVariables implements unified pattern with simple approach
func (p *variableParser) ReplaceVariables(text string, resolver VariableResolver) (string, error) {
	if !strings.Contains(text, "$") {
		return text, nil
	}

	result := text
	maxIterations := 15 // Extended from existing 10 to 15
	var resolutionError error

	for i := 0; i < maxIterations && strings.Contains(result, "$"); i++ {
		oldResult := result

		// Unified pattern processes both formats simultaneously (resolves overlap issues fundamentally)
		result = unifiedVariablePattern.ReplaceAllStringFunc(result, func(match string) string {
			submatches := unifiedVariablePattern.FindStringSubmatch(match)

			// Get variable name using pre-computed indices
			var varName string
			if bracedGroupIndex != -1 && bracedGroupIndex < len(submatches) && submatches[bracedGroupIndex] != "" {
				// ${VAR} format: variable name from braced group
				varName = submatches[bracedGroupIndex]
			} else if simpleGroupIndex != -1 && simpleGroupIndex < len(submatches) && submatches[simpleGroupIndex] != "" {
				// $VAR format: variable name from simple group
				varName = submatches[simpleGroupIndex]
			}

			if varName == "" {
				return match // Invalid match
			}

			return p.resolveVariableWithErrorHandling(varName, resolver, &resolutionError, match)
		})

		if result == oldResult {
			break // No change = processing complete
		}
	}

	if resolutionError != nil {
		return "", resolutionError
	}

	// Circular reference check - only check for valid variable patterns, not literal $ characters
	if unifiedVariablePattern.MatchString(result) {
		return "", environment.ErrCircularReference
	}

	return result, nil
}

// resolveVariableWithErrorHandling resolves variable and handles errors uniformly
func (p *variableParser) resolveVariableWithErrorHandling(varName string, resolver VariableResolver, resolutionError *error, originalMatch string) string {
	resolvedValue, err := resolver.ResolveVariable(varName)
	if err != nil {
		if *resolutionError == nil {
			*resolutionError = err
		}
		return originalMatch // Maintain original string on error
	}
	return resolvedValue
}
