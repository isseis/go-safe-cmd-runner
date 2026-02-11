package variable

import (
	"fmt"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Scope represents the scope of a variable (global or local)
type Scope int

const (
	// ScopeError represents an invalid scope (error sentinel value)
	// Returned by DetermineScope when the variable name is invalid
	ScopeError Scope = iota

	// ScopeGlobal represents a global variable (uppercase start)
	// Defined in: [global.vars]
	// Accessible from: templates, params
	ScopeGlobal

	// ScopeLocal represents a local variable (lowercase/underscore start)
	// Defined in: [groups.vars], [groups.commands.vars]
	// Accessible from: params only
	ScopeLocal
)

// String returns the string representation of the scope
func (s Scope) String() string {
	switch s {
	case ScopeGlobal:
		return "global"
	case ScopeLocal:
		return "local"
	case ScopeError:
		return "error"
	default:
		return "unknown"
	}
}

// DetermineScope determines the scope of a variable based on its name.
// It returns the scope and an error if the name is invalid.
//
// Naming rules:
//   - Names starting with "__" (double underscore): reserved, always invalid
//   - Names starting with uppercase (A-Z): global scope
//   - Names starting with lowercase (a-z) or single underscore: local scope
//   - All other characters: invalid
//
// Valid characters after the first character: A-Z, a-z, 0-9, _
//
// Examples:
//   - "AwsPath" → ScopeGlobal, nil
//   - "AWS_PATH" → ScopeGlobal, nil
//   - "data_dir" → ScopeLocal, nil
//   - "_internal" → ScopeLocal, nil
//   - "__reserved" → ScopeError, ErrReservedVariableName
//   - "123invalid" → ScopeError, ErrInvalidVariableName
func DetermineScope(name string) (Scope, error) {
	if name == "" {
		return ScopeError, &ErrInvalidVariableName{
			Name:   name,
			Reason: "variable name cannot be empty",
		}
	}

	// Check for reserved prefix
	if strings.HasPrefix(name, "__") {
		return ScopeError, &ErrReservedVariableName{
			Name: name,
		}
	}

	// Check first character to determine scope
	first := rune(name[0])
	switch {
	case first >= 'A' && first <= 'Z':
		// Uppercase start → global
		return ScopeGlobal, nil
	case first >= 'a' && first <= 'z':
		// Lowercase start → local
		return ScopeLocal, nil
	case first == '_':
		// Single underscore start → local
		return ScopeLocal, nil
	default:
		return ScopeError, &ErrInvalidVariableName{
			Name:   name,
			Reason: fmt.Sprintf("variable name must start with A-Z, a-z, or _ (got %q)", first),
		}
	}
}

// ValidateVariableNameForScope validates that a variable name follows
// the naming rules for the specified scope.
//
// This function first determines the scope from the name and then
// checks if it matches the expected scope.
//
// Parameters:
//   - name: the variable name to validate
//   - expectedScope: the scope where the variable is being defined
//   - location: human-readable location for error messages (e.g., "[global.vars]")
//
// Returns:
//   - error: validation error if the name is invalid or doesn't match the expected scope
func ValidateVariableNameForScope(name string, expectedScope Scope, location string) error {
	// First, determine the scope from the name
	actualScope, err := DetermineScope(name)
	if err != nil {
		return fmt.Errorf("%s: %w", location, err)
	}

	// Check if the scope matches
	if actualScope != expectedScope {
		return &ErrScopeMismatch{
			Name:          name,
			Location:      location,
			ExpectedScope: expectedScope,
			ActualScope:   actualScope,
		}
	}

	// Validate full variable name using existing security validation
	// This checks for valid characters throughout the name
	if err := security.ValidateVariableName(name); err != nil {
		return fmt.Errorf("%s: variable %q: %w", location, name, err)
	}

	return nil
}

// ErrReservedVariableName is returned when a variable name uses the reserved prefix
type ErrReservedVariableName struct {
	Name string
}

func (e *ErrReservedVariableName) Error() string {
	return fmt.Sprintf("variable name %q is reserved (starts with '__')", e.Name)
}

// ErrInvalidVariableName is returned when a variable name doesn't follow naming rules
type ErrInvalidVariableName struct {
	Name   string
	Reason string
}

func (e *ErrInvalidVariableName) Error() string {
	return fmt.Sprintf("invalid variable name %q: %s", e.Name, e.Reason)
}

// ErrScopeMismatch is returned when a variable is defined in the wrong scope
type ErrScopeMismatch struct {
	Name          string
	Location      string
	ExpectedScope Scope
	ActualScope   Scope
}

func (e *ErrScopeMismatch) Error() string {
	return fmt.Sprintf(
		"%s: variable %q must be %s (starts with %s), but is defined as %s (starts with %s)",
		e.Location,
		e.Name,
		e.ExpectedScope,
		e.scopeStartCharDescription(e.ExpectedScope),
		e.ActualScope,
		e.scopeStartCharDescription(e.ActualScope),
	)
}

func (e *ErrScopeMismatch) scopeStartCharDescription(scope Scope) string {
	switch scope {
	case ScopeGlobal:
		return "uppercase A-Z"
	case ScopeLocal:
		return "lowercase a-z or underscore _"
	default:
		return "unknown"
	}
}

// ErrUndefinedGlobalVariable is returned when a global variable is not defined
type ErrUndefinedGlobalVariable struct {
	Name string
}

func (e *ErrUndefinedGlobalVariable) Error() string {
	return fmt.Sprintf("undefined global variable %q", e.Name)
}

// ErrUndefinedLocalVariable is returned when a local variable is not defined
type ErrUndefinedLocalVariable struct {
	Name string
}

func (e *ErrUndefinedLocalVariable) Error() string {
	return fmt.Sprintf("undefined local variable %q", e.Name)
}
