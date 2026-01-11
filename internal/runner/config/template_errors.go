// Package config provides configuration loading and validation for the command runner.
package config

import "fmt"

// Template-related errors

// ErrTemplateNotFound is returned when a referenced template does not exist.
type ErrTemplateNotFound struct {
	CommandName  string
	TemplateName string
}

func (e *ErrTemplateNotFound) Error() string {
	return fmt.Sprintf("template %q not found (referenced by command %q)",
		e.TemplateName, e.CommandName)
}

// ErrTemplateFieldConflict is returned when both template and execution fields are specified.
type ErrTemplateFieldConflict struct {
	GroupName    string
	CommandIndex int
	TemplateName string
	Field        string // "cmd", "args", "env", "workdir"
}

func (e *ErrTemplateFieldConflict) Error() string {
	return fmt.Sprintf("group[%s] command[%d]: cannot specify both \"template\" and %q fields in command definition",
		e.GroupName, e.CommandIndex, e.Field)
}

// ErrDuplicateTemplateName is returned when a template name is defined more than once.
type ErrDuplicateTemplateName struct {
	Name string
}

func (e *ErrDuplicateTemplateName) Error() string {
	return fmt.Sprintf("duplicate template name %q", e.Name)
}

// ErrInvalidTemplateName is returned when a template name is invalid.
type ErrInvalidTemplateName struct {
	Name   string
	Reason string
}

func (e *ErrInvalidTemplateName) Error() string {
	return fmt.Sprintf("invalid template name %q: %s", e.Name, e.Reason)
}

// ErrReservedTemplateName is returned when a template name uses a reserved prefix.
type ErrReservedTemplateName struct {
	Name string
}

func (e *ErrReservedTemplateName) Error() string {
	return fmt.Sprintf("template name %q uses reserved prefix \"__\"", e.Name)
}

// ErrTemplateContainsNameField is returned when a template definition contains a "name" field.
type ErrTemplateContainsNameField struct {
	TemplateName string
}

func (e *ErrTemplateContainsNameField) Error() string {
	return fmt.Sprintf("template definition %q cannot contain \"name\" field",
		e.TemplateName)
}

// ErrMissingRequiredField is returned when a required field is missing.
type ErrMissingRequiredField struct {
	TemplateName string
	GroupName    string
	CommandIndex int
	Field        string
}

func (e *ErrMissingRequiredField) Error() string {
	if e.TemplateName != "" {
		return fmt.Sprintf("template %q: required field %q is missing",
			e.TemplateName, e.Field)
	}
	return fmt.Sprintf("group[%s] command[%d]: required field %q is missing",
		e.GroupName, e.CommandIndex, e.Field)
}

// Parameter-related errors

// ErrRequiredParamMissing is returned when a required parameter is not provided.
type ErrRequiredParamMissing struct {
	TemplateName string
	Field        string
	ParamName    string
}

func (e *ErrRequiredParamMissing) Error() string {
	return fmt.Sprintf("template %q %s: required parameter %q not provided",
		e.TemplateName, e.Field, e.ParamName)
}

// ErrTemplateTypeMismatch is returned when a parameter value has the wrong type.
type ErrTemplateTypeMismatch struct {
	TemplateName string
	Field        string
	ParamName    string
	Expected     string
	Actual       string
}

func (e *ErrTemplateTypeMismatch) Error() string {
	return fmt.Sprintf("template %q %s: parameter %q expected %s, got %s",
		e.TemplateName, e.Field, e.ParamName, e.Expected, e.Actual)
}

// ErrForbiddenPatternInTemplate is returned when a template definition contains
// a forbidden variable reference pattern (%{var}) - enforces NF-006.
type ErrForbiddenPatternInTemplate struct {
	TemplateName string
	Field        string
	Value        string
}

func (e *ErrForbiddenPatternInTemplate) Error() string {
	return fmt.Sprintf("template %q contains forbidden pattern \"%%{\" in %s: variable references are not allowed in template definitions for security reasons (see NF-006)",
		e.TemplateName, e.Field)
}

// ErrPlaceholderInEnvKey is returned when a placeholder is used in an env key.
type ErrPlaceholderInEnvKey struct {
	TemplateName string
	EnvEntry     string
	Key          string
}

func (e *ErrPlaceholderInEnvKey) Error() string {
	return fmt.Sprintf("template %q env: placeholder in key %q is not allowed (env entry: %q) - only values can contain placeholders",
		e.TemplateName, e.Key, e.EnvEntry)
}

// ErrTemplateInvalidEnvFormat is returned when a template env entry is not in KEY=VALUE format.
type ErrTemplateInvalidEnvFormat struct {
	TemplateName  string
	Field         string // e.g., "env[0]"
	ExpandedIndex int    // Index within expanded array elements
	Entry         string
}

func (e *ErrTemplateInvalidEnvFormat) Error() string {
	if e.ExpandedIndex > 0 {
		return fmt.Sprintf("template %q %s: invalid env format in expanded element [%d]: %q (expected KEY=VALUE format)",
			e.TemplateName, e.Field, e.ExpandedIndex, e.Entry)
	}
	return fmt.Sprintf("template %q %s: invalid env format: %q (expected KEY=VALUE format)",
		e.TemplateName, e.Field, e.Entry)
}

// ErrArrayInMixedContext is returned when ${@param} is used in a mixed context.
type ErrArrayInMixedContext struct {
	TemplateName string
	Field        string
	ParamName    string
}

func (e *ErrArrayInMixedContext) Error() string {
	return fmt.Sprintf("template %q %s: array parameter ${@%s} cannot be used in mixed context",
		e.TemplateName, e.Field, e.ParamName)
}

// ErrTemplateInvalidArrayElement is returned when an array parameter contains non-string elements.
type ErrTemplateInvalidArrayElement struct {
	TemplateName string
	Field        string
	ParamName    string
	Index        int
	ActualType   string
}

func (e *ErrTemplateInvalidArrayElement) Error() string {
	return fmt.Sprintf("template %q %s: array parameter %q contains non-string element at index %d (type: %s)",
		e.TemplateName, e.Field, e.ParamName, e.Index, e.ActualType)
}

// ErrUnsupportedParamType is returned when a parameter has an unsupported type.
type ErrUnsupportedParamType struct {
	TemplateName string
	Field        string
	ParamName    string
	ActualType   string
}

func (e *ErrUnsupportedParamType) Error() string {
	return fmt.Sprintf("template %q %s: parameter %q has unsupported type %s (expected string or []string)",
		e.TemplateName, e.Field, e.ParamName, e.ActualType)
}

// ErrInvalidParamName is returned when a parameter name is invalid.
type ErrInvalidParamName struct {
	TemplateName string
	ParamName    string
	Reason       string
}

func (e *ErrInvalidParamName) Error() string {
	return fmt.Sprintf("template %q: invalid parameter name %q: %s",
		e.TemplateName, e.ParamName, e.Reason)
}

// ErrEmptyPlaceholderName is returned when a placeholder has an empty name.
type ErrEmptyPlaceholderName struct {
	Input    string
	Position int
}

func (e *ErrEmptyPlaceholderName) Error() string {
	return fmt.Sprintf("empty placeholder name at position %d in %q", e.Position, e.Input)
}

// ErrMultipleValuesInStringContext is returned when array expansion produces
// multiple values in a string context.
type ErrMultipleValuesInStringContext struct {
	TemplateName string
	Field        string
}

func (e *ErrMultipleValuesInStringContext) Error() string {
	return fmt.Sprintf("template %q %s: array expansion produced multiple values in string context",
		e.TemplateName, e.Field)
}

// Placeholder parsing errors

// ErrUnclosedPlaceholder is returned when a placeholder is not closed.
type ErrUnclosedPlaceholder struct {
	Input    string
	Position int
}

func (e *ErrUnclosedPlaceholder) Error() string {
	return fmt.Sprintf("unclosed placeholder at position %d in %q", e.Position, e.Input)
}

// ErrEmptyPlaceholder is returned when a placeholder is empty.
type ErrEmptyPlaceholder struct {
	Input    string
	Position int
}

func (e *ErrEmptyPlaceholder) Error() string {
	return fmt.Sprintf("empty placeholder at position %d in %q", e.Position, e.Input)
}

// ErrInvalidPlaceholderName is returned when a placeholder name is invalid.
type ErrInvalidPlaceholderName struct {
	Input    string
	Position int
	Name     string
	Reason   string
}

func (e *ErrInvalidPlaceholderName) Error() string {
	return fmt.Sprintf("invalid placeholder name %q at position %d in %q: %s",
		e.Name, e.Position, e.Input, e.Reason)
}

// ErrTemplateCmdNotSingleValue is returned when template cmd field doesn't resolve to exactly one value.
type ErrTemplateCmdNotSingleValue struct {
	TemplateName string
	ResultCount  int
}

func (e *ErrTemplateCmdNotSingleValue) Error() string {
	if e.ResultCount == 0 {
		return fmt.Sprintf("template %q: cmd field must resolve to exactly one non-empty value, got 0 values (check optional placeholders)",
			e.TemplateName)
	}
	return fmt.Sprintf("template %q: cmd field must resolve to exactly one value, got %d values",
		e.TemplateName, e.ResultCount)
}

// ErrDuplicateEnvVariableDetail is returned when duplicate environment variable keys are detected in template env.
type ErrDuplicateEnvVariableDetail struct {
	TemplateName string
	Field        string // "env"
	EnvKey       string
}

func (e *ErrDuplicateEnvVariableDetail) Error() string {
	return fmt.Sprintf("template %q %s: duplicate environment variable key %q",
		e.TemplateName, e.Field, e.EnvKey)
}

func (e *ErrDuplicateEnvVariableDetail) Unwrap() error {
	return ErrDuplicateEnvVariable
}

// ErrTemplateVarUnexpectedMultipleValues is returned when a template var string expansion results in multiple values.
type ErrTemplateVarUnexpectedMultipleValues struct {
	TemplateName string
	Field        string
}

func (e *ErrTemplateVarUnexpectedMultipleValues) Error() string {
	return fmt.Sprintf("template %q field %q: unexpected multiple values from expansion", e.TemplateName, e.Field)
}
