package config

import (
	"errors"
	"fmt"
)

// Configuration loading and expansion errors
var (
	// ErrGlobalEnvExpansionFailed is returned when global environment variable expansion fails
	ErrGlobalEnvExpansionFailed = errors.New("global environment variable expansion failed")

	// ErrGroupEnvExpansionFailed is returned when group environment variable expansion fails
	ErrGroupEnvExpansionFailed = errors.New("group environment variable expansion failed")

	// ErrCommandEnvExpansionFailed is returned when command environment variable expansion fails
	ErrCommandEnvExpansionFailed = errors.New("command environment variable expansion failed")

	// ErrDuplicateEnvVariable is returned when duplicate environment variable keys are detected
	ErrDuplicateEnvVariable = errors.New("duplicate environment variable key")

	// ErrMalformedEnvVariable is returned when an env entry is not in KEY=VALUE format
	ErrMalformedEnvVariable = errors.New("malformed env entry (expected KEY=VALUE format)")

	// ErrInvalidEnvKey is returned when an environment variable key contains invalid characters
	ErrInvalidEnvKey = errors.New("invalid environment variable key")

	// ErrNilGroup is returned when group parameter is nil
	ErrNilGroup = errors.New("group cannot be nil")

	// ErrReservedVariablePrefix is returned when a variable name starts with reserved prefix
	ErrReservedVariablePrefix = errors.New("variable name uses reserved prefix")

	// ErrVariableNotInAllowlist is returned when from_env references a system env var not in env_allowlist
	ErrVariableNotInAllowlist = errors.New("system environment variable not in allowlist")

	// ErrCircularReference is returned when circular variable reference is detected
	ErrCircularReference = errors.New("circular variable reference detected")

	// ErrUndefinedVariable is returned when %{VAR} references an undefined variable
	ErrUndefinedVariable = errors.New("undefined variable")

	// ErrInvalidEscapeSequence is returned when an invalid escape sequence is found
	ErrInvalidEscapeSequence = errors.New("invalid escape sequence")

	// ErrUnclosedVariableReference is returned when %{ is not closed with }
	ErrUnclosedVariableReference = errors.New("unclosed variable reference")

	// ErrMaxRecursionDepthExceeded is returned when variable expansion exceeds maximum recursion depth
	ErrMaxRecursionDepthExceeded = errors.New("maximum recursion depth exceeded")

	// ErrInvalidFromEnvFormat is returned when from_env entry is not in 'internal_name=SYSTEM_VAR' format
	ErrInvalidFromEnvFormat = errors.New("invalid from_env format")

	// ErrInvalidVarsFormat is returned when a vars entry is not in var_name=value format
	ErrInvalidVarsFormat = errors.New("malformed vars entry (expected var_name=value format)")

	// ErrInvalidEnvFormat is returned when an env entry is not in VAR=value format
	ErrInvalidEnvFormat = errors.New("malformed env entry (expected VAR=value format)")

	// ErrInvalidSystemVariableName is returned when system variable name is invalid
	ErrInvalidSystemVariableName = errors.New("invalid system variable name")

	// ErrInvalidVariableName indicates that a variable name is invalid
	ErrInvalidVariableName = errors.New("invalid variable name")

	// ErrDuplicateVariableDefinition is returned when the same variable is defined multiple times in the same scope
	ErrDuplicateVariableDefinition = errors.New("duplicate variable definition")

	// ErrInvalidGroupName is returned when a group name doesn't match the required pattern
	ErrInvalidGroupName = errors.New("invalid group name")

	// ErrEmptyGroupName is returned when a group has an empty name
	ErrEmptyGroupName = errors.New("group has empty name")

	// ErrDuplicateGroupName is returned when duplicate group names are found
	ErrDuplicateGroupName = errors.New("duplicate group name")

	// ErrNilConfig is returned when configuration is nil
	ErrNilConfig = errors.New("configuration must not be nil")
)

// ErrInvalidVariableNameDetail provides detailed information about invalid variable names.
// This error type wraps ErrInvalidVariableName and is used for internal variable validation
// in vars and from_env fields.
type ErrInvalidVariableNameDetail struct {
	Level        string
	Field        string
	VariableName string
	Reason       string
}

func (e *ErrInvalidVariableNameDetail) Error() string {
	return fmt.Sprintf("invalid variable name in %s.%s: '%s' (%s)", e.Level, e.Field, e.VariableName, e.Reason)
}

func (e *ErrInvalidVariableNameDetail) Unwrap() error {
	return ErrInvalidVariableName
}

// ErrInvalidSystemVariableNameDetail provides detailed information about invalid system variable names
type ErrInvalidSystemVariableNameDetail struct {
	Level              string
	Field              string
	SystemVariableName string
	Reason             string
}

func (e *ErrInvalidSystemVariableNameDetail) Error() string {
	return fmt.Sprintf("invalid system variable name in %s.%s: '%s' (%s)", e.Level, e.Field, e.SystemVariableName, e.Reason)
}

func (e *ErrInvalidSystemVariableNameDetail) Unwrap() error {
	return ErrInvalidSystemVariableName
}

// ErrReservedVariablePrefixDetail provides detailed information about reserved prefix errors
type ErrReservedVariablePrefixDetail struct {
	Level        string
	Field        string
	VariableName string
	Prefix       string
}

func (e *ErrReservedVariablePrefixDetail) Error() string {
	return fmt.Sprintf("reserved variable prefix in %s.%s: '%s' (prefix '%s' is reserved)", e.Level, e.Field, e.VariableName, e.Prefix)
}

func (e *ErrReservedVariablePrefixDetail) Unwrap() error {
	return ErrReservedVariablePrefix
}

// ErrVariableNotInAllowlistDetail provides detailed information about allowlist violations
type ErrVariableNotInAllowlistDetail struct {
	Level           string
	SystemVarName   string
	InternalVarName string
	Allowlist       []string
}

func (e *ErrVariableNotInAllowlistDetail) Error() string {
	return fmt.Sprintf("system environment variable '%s' not in allowlist (referenced as '%s' in %s.from_env)", e.SystemVarName, e.InternalVarName, e.Level)
}

func (e *ErrVariableNotInAllowlistDetail) Unwrap() error {
	return ErrVariableNotInAllowlist
}

// ErrCircularReferenceDetail provides detailed information about circular references
type ErrCircularReferenceDetail struct {
	Level        string
	Field        string
	VariableName string
	Chain        []string
}

func (e *ErrCircularReferenceDetail) Error() string {
	return fmt.Sprintf("circular reference in %s.%s: '%s' (chain: %v)", e.Level, e.Field, e.VariableName, e.Chain)
}

func (e *ErrCircularReferenceDetail) Unwrap() error {
	return ErrCircularReference
}

// ErrUndefinedVariableDetail provides detailed information about undefined variables
type ErrUndefinedVariableDetail struct {
	Level        string
	Field        string
	VariableName string
	Context      string
}

func (e *ErrUndefinedVariableDetail) Error() string {
	return fmt.Sprintf("undefined variable in %s.%s: '%s' (context: %s)", e.Level, e.Field, e.VariableName, e.Context)
}

func (e *ErrUndefinedVariableDetail) Unwrap() error {
	return ErrUndefinedVariable
}

// ErrInvalidEscapeSequenceDetail provides detailed information about invalid escape sequences
type ErrInvalidEscapeSequenceDetail struct {
	Level    string
	Field    string
	Sequence string
	Context  string
}

func (e *ErrInvalidEscapeSequenceDetail) Error() string {
	return fmt.Sprintf("invalid escape sequence in %s.%s: '%s' (context: %s)", e.Level, e.Field, e.Sequence, e.Context)
}

func (e *ErrInvalidEscapeSequenceDetail) Unwrap() error {
	return ErrInvalidEscapeSequence
}

// ErrUnclosedVariableReferenceDetail provides detailed information about unclosed variable references
type ErrUnclosedVariableReferenceDetail struct {
	Level   string
	Field   string
	Context string
}

func (e *ErrUnclosedVariableReferenceDetail) Error() string {
	return fmt.Sprintf("unclosed variable reference in %s.%s: missing closing '}' (context: %s)", e.Level, e.Field, e.Context)
}

func (e *ErrUnclosedVariableReferenceDetail) Unwrap() error {
	return ErrUnclosedVariableReference
}

// ErrMaxRecursionDepthExceededDetail provides detailed information about recursion depth limit
type ErrMaxRecursionDepthExceededDetail struct {
	Level    string
	Field    string
	MaxDepth int
	Context  string
}

func (e *ErrMaxRecursionDepthExceededDetail) Error() string {
	return fmt.Sprintf("maximum recursion depth exceeded in %s.%s: limit %d (context: %s)", e.Level, e.Field, e.MaxDepth, e.Context)
}

func (e *ErrMaxRecursionDepthExceededDetail) Unwrap() error {
	return ErrMaxRecursionDepthExceeded
}

// ErrReservedVariableNameDetail is an alias for ErrReservedVariablePrefixDetail
// to maintain consistency with test naming conventions
type ErrReservedVariableNameDetail = ErrReservedVariablePrefixDetail

// ErrInvalidFromEnvFormatDetail provides detailed information about invalid from_env format
type ErrInvalidFromEnvFormatDetail struct {
	Level   string
	Mapping string
	Reason  string
}

func (e *ErrInvalidFromEnvFormatDetail) Error() string {
	return fmt.Sprintf("invalid from_env format in %s: '%s' (%s)", e.Level, e.Mapping, e.Reason)
}

func (e *ErrInvalidFromEnvFormatDetail) Unwrap() error {
	return ErrInvalidFromEnvFormat
}

// ErrInvalidVarsFormatDetail provides detailed information about invalid vars format
type ErrInvalidVarsFormatDetail struct {
	Level   string
	Mapping string
	Reason  string
}

func (e *ErrInvalidVarsFormatDetail) Error() string {
	return fmt.Sprintf("invalid vars format in %s: '%s' (%s)", e.Level, e.Mapping, e.Reason)
}

func (e *ErrInvalidVarsFormatDetail) Unwrap() error {
	return ErrInvalidVarsFormat
}

// ErrInvalidEnvFormatDetail provides detailed information about invalid env format
type ErrInvalidEnvFormatDetail struct {
	Level   string
	Mapping string
	Reason  string
}

func (e *ErrInvalidEnvFormatDetail) Error() string {
	return fmt.Sprintf("invalid env format in %s: '%s' (%s)", e.Level, e.Mapping, e.Reason)
}

func (e *ErrInvalidEnvFormatDetail) Unwrap() error {
	return ErrInvalidEnvFormat
}

// ErrInvalidEnvKeyDetail provides detailed information about invalid environment variable key
type ErrInvalidEnvKeyDetail struct {
	Level   string
	Key     string
	Context string
	Reason  string
}

func (e *ErrInvalidEnvKeyDetail) Error() string {
	return fmt.Sprintf("invalid environment variable key in %s: '%s' (context: %s, reason: %s)", e.Level, e.Key, e.Context, e.Reason)
}

func (e *ErrInvalidEnvKeyDetail) Unwrap() error {
	return ErrInvalidEnvKey
}

// ErrDuplicateVariableDefinitionDetail provides detailed information about duplicate variable definitions
type ErrDuplicateVariableDefinitionDetail struct {
	Level        string
	Field        string
	VariableName string
}

func (e *ErrDuplicateVariableDefinitionDetail) Error() string {
	return fmt.Sprintf("duplicate variable definition in %s.%s: '%s' is defined multiple times", e.Level, e.Field, e.VariableName)
}

func (e *ErrDuplicateVariableDefinitionDetail) Unwrap() error {
	return ErrDuplicateVariableDefinition
}
