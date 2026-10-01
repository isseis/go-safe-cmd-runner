// Package config provides configuration loading and validation for the command runner.
package config

import (
	"strconv"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// Template-related errors

// ErrTemplateNotFound is returned when a referenced template does not exist.
type ErrTemplateNotFound struct {
	CommandName  string
	TemplateName string
}

// StructuredMessage declares the referenced template and the command that
// referenced it as identifiers, quoted the way fmt's %q rendered them.
func (e *ErrTemplateNotFound) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("template "),
		errmsg.Quoted(errmsg.Ident(e.TemplateName)),
		errmsg.Const(" not found (referenced by command "),
		errmsg.Quoted(errmsg.Ident(e.CommandName)),
		errmsg.Const(")"),
	)
}

func (e *ErrTemplateNotFound) Error() string {
	return e.StructuredMessage().String()
}

// ErrTemplateFieldConflict is returned when both template and execution fields are specified.
type ErrTemplateFieldConflict struct {
	GroupName    string
	CommandIndex int
	TemplateName string
	Field        Field // The conflicting field
}

// StructuredMessage declares the group name through its own level parts and
// the conflicting field through its own parts. TemplateName is not rendered.
func (e *ErrTemplateFieldConflict) StructuredMessage() errmsg.Message {
	parts := groupLevel(e.GroupName).parts()
	parts = append(parts,
		errmsg.Const(" command["),
		errmsg.Text(strconv.Itoa(e.CommandIndex)),
		errmsg.Const("]: cannot specify both \"template\" and "),
		errmsg.Quoted(e.Field.parts()...),
		errmsg.Const(" fields in command definition"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrTemplateFieldConflict) Error() string {
	return e.StructuredMessage().String()
}

// ErrDuplicateTemplateName is returned when a template name is defined more than once.
// When multiple locations are provided, it means the template is defined in multiple files.
type ErrDuplicateTemplateName struct {
	Name string

	// Locations is the list of file paths where the template is defined.
	// If empty or has only one element, only Name is used for the error message.
	Locations []string
}

// StructuredMessage declares the template name as a quoted identifier; the
// definition locations are paths.
func (e *ErrDuplicateTemplateName) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("duplicate template name ")}
	if len(e.Locations) > 1 {
		parts = []errmsg.Part{errmsg.Const("duplicate command template name ")}
	}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.Name)))
	if len(e.Locations) > 1 {
		parts = append(parts, errmsg.Const("\n  Defined in:\n    - "))
		for i, location := range e.Locations {
			if i > 0 {
				parts = append(parts, errmsg.Const("\n    - "))
			}
			parts = append(parts, errmsg.Path(location))
		}
	}
	return errmsg.NewMessage(parts...)
}

func (e *ErrDuplicateTemplateName) Error() string {
	return e.StructuredMessage().String()
}

// ErrInvalidTemplateName is returned when a template name is invalid.
type ErrInvalidTemplateName struct {
	Name   string
	Reason string
}

// StructuredMessage declares the rejected name and the reason as text.
func (e *ErrInvalidTemplateName) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("invalid template name "),
		errmsg.Quoted(errmsg.Text(e.Name)),
		errmsg.Const(": "),
		errmsg.Text(e.Reason),
	)
}

func (e *ErrInvalidTemplateName) Error() string {
	return e.StructuredMessage().String()
}

// ErrReservedTemplateName is returned when a template name uses a reserved prefix.
type ErrReservedTemplateName struct {
	Name string
}

// StructuredMessage declares the rejected name as text.
func (e *ErrReservedTemplateName) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("template name "),
		errmsg.Quoted(errmsg.Text(e.Name)),
		errmsg.Const(" uses reserved prefix \"__\""),
	)
}

func (e *ErrReservedTemplateName) Error() string {
	return e.StructuredMessage().String()
}

// ErrTemplateContainsNameField is returned when a template definition contains a "name" field.
type ErrTemplateContainsNameField struct {
	TemplateName string
}

// StructuredMessage declares the template name as a quoted identifier; it is a
// name the operator wrote before the name validation runs.
func (e *ErrTemplateContainsNameField) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("template definition "),
		errmsg.Quoted(errmsg.Ident(e.TemplateName)),
		errmsg.Const(" cannot contain \"name\" field"),
	)
}

func (e *ErrTemplateContainsNameField) Error() string {
	return e.StructuredMessage().String()
}

// ErrMissingRequiredField is returned when a required field is missing.
type ErrMissingRequiredField struct {
	TemplateName string
	GroupName    string
	CommandIndex int
	Field        Field
}

// StructuredMessage declares the template or group name through the declared
// role, and the missing field through its own parts.
func (e *ErrMissingRequiredField) StructuredMessage() errmsg.Message {
	var parts []errmsg.Part
	if e.TemplateName != "" {
		parts = []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName))}
	} else {
		parts = groupLevel(e.GroupName).parts()
		parts = append(parts, errmsg.Const(" command["), errmsg.Text(strconv.Itoa(e.CommandIndex)), errmsg.Const("]"))
	}
	parts = append(parts, errmsg.Const(": required field "), errmsg.Quoted(e.Field.parts()...), errmsg.Const(" is missing"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrMissingRequiredField) Error() string {
	return e.StructuredMessage().String()
}

// Parameter-related errors

// ErrRequiredParamMissing is returned when a required parameter is not provided.
type ErrRequiredParamMissing struct {
	TemplateName string
	Field        Field
	ParamName    string
}

// StructuredMessage declares the template and parameter names as quoted
// identifiers and the field through its own parts.
func (e *ErrRequiredParamMissing) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName)), errmsg.Const(" ")}
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": required parameter "), errmsg.Quoted(errmsg.Ident(e.ParamName)), errmsg.Const(" not provided"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrRequiredParamMissing) Error() string {
	return e.StructuredMessage().String()
}

// ErrTemplateTypeMismatch is returned when a parameter value has the wrong type.
type ErrTemplateTypeMismatch struct {
	TemplateName string
	Field        Field
	ParamName    string
	Expected     string
	Actual       string
}

// StructuredMessage declares the template and parameter names as quoted
// identifiers, the field through its own parts and the type names as text.
func (e *ErrTemplateTypeMismatch) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName)), errmsg.Const(" ")}
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": parameter "), errmsg.Quoted(errmsg.Ident(e.ParamName)))
	parts = append(parts, errmsg.Const(" expected "), errmsg.Text(e.Expected), errmsg.Const(", got "), errmsg.Text(e.Actual))
	return errmsg.NewMessage(parts...)
}

func (e *ErrTemplateTypeMismatch) Error() string {
	return e.StructuredMessage().String()
}

// ErrPlaceholderInEnvKey is returned when a placeholder is used in an env key.
type ErrPlaceholderInEnvKey struct {
	TemplateName string
	EnvEntry     string
	Key          string
}

// StructuredMessage declares the template name as a quoted identifier; the key
// and the raw entry stay text because their origin cannot be proven.
func (e *ErrPlaceholderInEnvKey) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("template "),
		errmsg.Quoted(errmsg.Ident(e.TemplateName)),
		errmsg.Const(" env: placeholder in key "),
		errmsg.Quoted(errmsg.Text(e.Key)),
		errmsg.Const(" is not allowed (env entry: "),
		errmsg.Quoted(errmsg.Text(e.EnvEntry)),
		errmsg.Const(") - only values can contain placeholders"),
	)
}

func (e *ErrPlaceholderInEnvKey) Error() string {
	return e.StructuredMessage().String()
}

// ErrTemplateInvalidEnvFormat is returned when a template env entry is not in KEY=VALUE format.
type ErrTemplateInvalidEnvFormat struct {
	TemplateName  string
	Field         Field
	ExpandedIndex int // Index within expanded array elements
	Entry         string
}

// StructuredMessage declares the template name as a quoted identifier, the
// field through its own parts and the entry as quoted text.
func (e *ErrTemplateInvalidEnvFormat) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName)), errmsg.Const(" ")}
	parts = append(parts, e.Field.parts()...)
	if e.ExpandedIndex > 0 {
		parts = append(parts, errmsg.Const(": invalid env format in expanded element ["), errmsg.Text(strconv.Itoa(e.ExpandedIndex)), errmsg.Const("]: "))
	} else {
		parts = append(parts, errmsg.Const(": invalid env format: "))
	}
	parts = append(parts, errmsg.Quoted(errmsg.Text(e.Entry)), errmsg.Const(" (expected KEY=VALUE format)"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrTemplateInvalidEnvFormat) Error() string {
	return e.StructuredMessage().String()
}

// ErrArrayInMixedContext is returned when ${@param} is used in a mixed context.
type ErrArrayInMixedContext struct {
	TemplateName string
	Field        Field
	ParamName    string
}

// StructuredMessage declares the template and parameter names as identifiers
// and the field through its own parts.
func (e *ErrArrayInMixedContext) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName)), errmsg.Const(" ")}
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": array parameter ${@"), errmsg.Ident(e.ParamName), errmsg.Const("} cannot be used in mixed context"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrArrayInMixedContext) Error() string {
	return e.StructuredMessage().String()
}

// ErrTemplateInvalidArrayElement is returned when an array parameter contains non-string elements.
type ErrTemplateInvalidArrayElement struct {
	TemplateName string
	Field        Field
	ParamName    string
	Index        int
	ActualType   string
}

// StructuredMessage declares the template and parameter names as quoted
// identifiers, the field through its own parts and the index and type as text.
func (e *ErrTemplateInvalidArrayElement) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName)), errmsg.Const(" ")}
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": array parameter "), errmsg.Quoted(errmsg.Ident(e.ParamName)))
	parts = append(parts, errmsg.Const(" contains non-string element at index "), errmsg.Text(strconv.Itoa(e.Index)), errmsg.Const(" (type: "), errmsg.Text(e.ActualType), errmsg.Const(")"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrTemplateInvalidArrayElement) Error() string {
	return e.StructuredMessage().String()
}

// ErrUnsupportedParamType is returned when a parameter has an unsupported type.
type ErrUnsupportedParamType struct {
	TemplateName string
	Field        Field
	ParamName    string
	ActualType   string
}

// StructuredMessage declares the template and parameter names as quoted
// identifiers, the field through its own parts and the type as text.
func (e *ErrUnsupportedParamType) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName)), errmsg.Const(" ")}
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": parameter "), errmsg.Quoted(errmsg.Ident(e.ParamName)))
	parts = append(parts, errmsg.Const(" has unsupported type "), errmsg.Text(e.ActualType), errmsg.Const(" (expected string or []string)"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrUnsupportedParamType) Error() string {
	return e.StructuredMessage().String()
}

// ErrEmptyPlaceholderName is returned when a placeholder has an empty name.
type ErrEmptyPlaceholderName struct {
	Input    string
	Position int
}

// StructuredMessage declares the input and the position as text.
func (e *ErrEmptyPlaceholderName) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("empty placeholder name at position "),
		errmsg.Text(strconv.Itoa(e.Position)),
		errmsg.Const(" in "),
		errmsg.Quoted(errmsg.Text(e.Input)),
	)
}

func (e *ErrEmptyPlaceholderName) Error() string {
	return e.StructuredMessage().String()
}

// Placeholder parsing errors

// ErrUnclosedPlaceholder is returned when a placeholder is not closed.
type ErrUnclosedPlaceholder struct {
	Input    string
	Position int
}

// StructuredMessage declares the input and the position as text.
func (e *ErrUnclosedPlaceholder) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("unclosed placeholder at position "),
		errmsg.Text(strconv.Itoa(e.Position)),
		errmsg.Const(" in "),
		errmsg.Quoted(errmsg.Text(e.Input)),
	)
}

func (e *ErrUnclosedPlaceholder) Error() string {
	return e.StructuredMessage().String()
}

// ErrEmptyPlaceholder is returned when a placeholder is empty.
type ErrEmptyPlaceholder struct {
	Input    string
	Position int
}

// StructuredMessage declares the input and the position as text.
func (e *ErrEmptyPlaceholder) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("empty placeholder at position "),
		errmsg.Text(strconv.Itoa(e.Position)),
		errmsg.Const(" in "),
		errmsg.Quoted(errmsg.Text(e.Input)),
	)
}

func (e *ErrEmptyPlaceholder) Error() string {
	return e.StructuredMessage().String()
}

// ErrInvalidPlaceholderName is returned when a placeholder name is invalid.
type ErrInvalidPlaceholderName struct {
	Input    string
	Position int
	Name     string
	Reason   string
}

// StructuredMessage declares the rejected name, the input and the reason as
// text; the position is text.
func (e *ErrInvalidPlaceholderName) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("invalid placeholder name "),
		errmsg.Quoted(errmsg.Text(e.Name)),
		errmsg.Const(" at position "),
		errmsg.Text(strconv.Itoa(e.Position)),
		errmsg.Const(" in "),
		errmsg.Quoted(errmsg.Text(e.Input)),
		errmsg.Const(": "),
		errmsg.Text(e.Reason),
	)
}

func (e *ErrInvalidPlaceholderName) Error() string {
	return e.StructuredMessage().String()
}

// ErrTemplateCmdNotSingleValue is returned when template cmd field doesn't resolve to exactly one value.
type ErrTemplateCmdNotSingleValue struct {
	TemplateName string
	ResultCount  int
}

// StructuredMessage declares the template name as a quoted identifier; the
// result count is text.
func (e *ErrTemplateCmdNotSingleValue) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName))}
	if e.ResultCount == 0 {
		parts = append(parts, errmsg.Const(": cmd field must resolve to exactly one non-empty value, got 0 values (check optional placeholders)"))
	} else {
		parts = append(parts, errmsg.Const(": cmd field must resolve to exactly one value, got "), errmsg.Text(strconv.Itoa(e.ResultCount)), errmsg.Const(" values"))
	}
	return errmsg.NewMessage(parts...)
}

func (e *ErrTemplateCmdNotSingleValue) Error() string {
	return e.StructuredMessage().String()
}

// ErrDuplicateEnvVariableDetail is returned when duplicate environment variable keys are detected in template env.
type ErrDuplicateEnvVariableDetail struct {
	TemplateName string
	Field        Field
	EnvKey       string
}

// StructuredMessage declares the template name as a quoted identifier and the
// field through its own parts; the duplicate key stays text because it may come
// from a parameter value.
func (e *ErrDuplicateEnvVariableDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template "), errmsg.Quoted(errmsg.Ident(e.TemplateName)), errmsg.Const(" ")}
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": duplicate environment variable key "), errmsg.Quoted(errmsg.Text(e.EnvKey)))
	return errmsg.NewMessage(parts...)
}

func (e *ErrDuplicateEnvVariableDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrDuplicateEnvVariableDetail) Unwrap() error {
	return ErrDuplicateEnvVariable
}

// ErrTemplateVarUnexpectedMultipleValues is returned when a template var string expansion results in multiple values.
type ErrTemplateVarUnexpectedMultipleValues struct {
	TemplateName string
	Field        Field
}

// StructuredMessage declares the template name as a quoted identifier and the
// field through its own quoted parts.
func (e *ErrTemplateVarUnexpectedMultipleValues) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("template "),
		errmsg.Quoted(errmsg.Ident(e.TemplateName)),
		errmsg.Const(" field "),
		errmsg.Quoted(e.Field.parts()...),
		errmsg.Const(": unexpected multiple values from expansion"),
	)
}

func (e *ErrTemplateVarUnexpectedMultipleValues) Error() string {
	return e.StructuredMessage().String()
}
