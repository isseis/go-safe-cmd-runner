package config

import (
	"errors"
	"fmt"
	"strings"
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

	// ErrInvalidEnvImportFormat is returned when env_import entry is not in 'internal_name=SYSTEM_VAR' format
	ErrInvalidEnvImportFormat = errors.New("invalid env_import format")

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

	// ErrNegativeTimeout indicates that a timeout value is negative
	ErrNegativeTimeout = errors.New("timeout must not be negative")

	// ErrInvalidPath is returned when a path validation fails
	ErrInvalidPath = errors.New("invalid path")

	// ErrEmptyPath is returned when a path is empty
	ErrEmptyPath = errors.New("path cannot be empty")

	// ErrDuplicatePath is returned when duplicate paths are found in cmd_allowed
	ErrDuplicatePath = errors.New("duplicate path in cmd_allowed")

	// ErrDuplicateResolvedPath is returned when different paths resolve to the same file
	ErrDuplicateResolvedPath = errors.New("duplicate resolved path in cmd_allowed")

	// ErrTooManyVariables is returned when the number of variables exceeds the limit
	ErrTooManyVariables = errors.New("too many variables")

	// ErrTypeMismatch is returned when a variable is redefined with a different type
	ErrTypeMismatch = errors.New("variable type mismatch")

	// ErrValueTooLong is returned when a string value exceeds the maximum length
	ErrValueTooLong = errors.New("value too long")

	// ErrArrayTooLarge is returned when an array exceeds the maximum number of elements
	ErrArrayTooLarge = errors.New("array too large")

	// ErrInvalidArrayElement is returned when an array element has an invalid type
	ErrInvalidArrayElement = errors.New("invalid array element")

	// ErrUnsupportedType is returned when a variable has an unsupported type
	ErrUnsupportedType = errors.New("unsupported variable type")

	// ErrArrayVariableInStringContext is returned when an array variable is used in string context
	ErrArrayVariableInStringContext = errors.New("array variable used in string context")

	// ErrEnvImportVarsConflict is returned when the same variable is defined in both env_import and vars
	ErrEnvImportVarsConflict = errors.New("variable defined in both env_import and vars")
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
	Chain        []string // expansion path leading to this error
}

func (e *ErrUndefinedVariableDetail) Error() string {
	msg := fmt.Sprintf("undefined variable in %s.%s: '%s' (context: %s)", e.Level, e.Field, e.VariableName, e.Context)
	if len(e.Chain) > 0 {
		msg += fmt.Sprintf(" (expansion path: %s)", strings.Join(e.Chain, " -> "))
	}
	return msg
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

// ErrInvalidEnvImportFormatDetail provides detailed information about invalid env_import format
type ErrInvalidEnvImportFormatDetail struct {
	Level   string
	Mapping string
	Reason  string
}

func (e *ErrInvalidEnvImportFormatDetail) Error() string {
	return fmt.Sprintf("invalid env_import format in %s: '%s' (%s)", e.Level, e.Mapping, e.Reason)
}

func (e *ErrInvalidEnvImportFormatDetail) Unwrap() error {
	return ErrInvalidEnvImportFormat
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

// InvalidPathError provides detailed information about path validation failures.
// This error type wraps ErrInvalidPath and is used when paths don't meet
// requirements (e.g., not absolute, too long, failed to resolve symlinks).
type InvalidPathError struct {
	Path   string // The invalid path
	Reason string // Reason why the path is invalid
}

func (e *InvalidPathError) Error() string {
	return fmt.Sprintf("invalid path '%s': %s", e.Path, e.Reason)
}

func (e *InvalidPathError) Unwrap() error {
	return ErrInvalidPath
}

// Is implements error comparison for InvalidPathError.
func (e *InvalidPathError) Is(target error) bool {
	_, ok := target.(*InvalidPathError)
	return ok
}

// ErrDuplicatePathDetail provides detailed information about duplicate paths in cmd_allowed.
// This error is returned when the same path string appears multiple times in the configuration.
type ErrDuplicatePathDetail struct {
	Level      string // e.g., "group[mygroup]"
	Field      string // e.g., "cmd_allowed"
	Path       string // The duplicated path string
	FirstIndex int    // Index of first occurrence
	DupeIndex  int    // Index of duplicate occurrence
}

func (e *ErrDuplicatePathDetail) Error() string {
	return fmt.Sprintf("duplicate path in %s.%s: '%s' appears at index %d and %d", e.Level, e.Field, e.Path, e.FirstIndex, e.DupeIndex)
}

func (e *ErrDuplicatePathDetail) Unwrap() error {
	return ErrDuplicatePath
}

// ErrDuplicateResolvedPathDetail provides detailed information about paths that resolve to the same file.
// This error is returned when different path strings (potentially after variable expansion)
// resolve to the same actual file after symlink resolution.
type ErrDuplicateResolvedPathDetail struct {
	Level        string // e.g., "group[mygroup]"
	Field        string // e.g., "cmd_allowed"
	OriginalPath string // The original path from config
	ResolvedPath string // The resolved path that is duplicated
}

func (e *ErrDuplicateResolvedPathDetail) Error() string {
	return fmt.Sprintf("duplicate resolved path in %s.%s: '%s' resolves to '%s' which is already in the list", e.Level, e.Field, e.OriginalPath, e.ResolvedPath)
}

func (e *ErrDuplicateResolvedPathDetail) Unwrap() error {
	return ErrDuplicateResolvedPath
}

// ===========================================
// New error types for vars table format
// ===========================================

// ErrTooManyVariablesDetail is returned when the number of variables exceeds MaxVarsPerLevel.
type ErrTooManyVariablesDetail struct {
	Level    string
	Count    int
	MaxCount int
}

func (e *ErrTooManyVariablesDetail) Error() string {
	return fmt.Sprintf("too many variables in %s: got %d, max %d", e.Level, e.Count, e.MaxCount)
}

func (e *ErrTooManyVariablesDetail) Unwrap() error {
	return ErrTooManyVariables
}

// ErrTypeMismatchDetail is returned when a variable is redefined with a different type (string vs array).
type ErrTypeMismatchDetail struct {
	Level        string
	VariableName string
	ExpectedType string
	ActualType   string
}

func (e *ErrTypeMismatchDetail) Error() string {
	return fmt.Sprintf("variable %q type mismatch in %s: already defined as %s, cannot redefine as %s",
		e.VariableName, e.Level, e.ExpectedType, e.ActualType)
}

func (e *ErrTypeMismatchDetail) Unwrap() error {
	return ErrTypeMismatch
}

// ErrValueTooLongDetail is returned when a string value exceeds MaxStringValueLen.
type ErrValueTooLongDetail struct {
	Level        string
	VariableName string
	Length       int
	MaxLength    int
}

func (e *ErrValueTooLongDetail) Error() string {
	return fmt.Sprintf("variable %q value too long in %s: got %d bytes, max %d",
		e.VariableName, e.Level, e.Length, e.MaxLength)
}

func (e *ErrValueTooLongDetail) Unwrap() error {
	return ErrValueTooLong
}

// ErrArrayTooLargeDetail is returned when an array variable exceeds MaxArrayElements.
type ErrArrayTooLargeDetail struct {
	Level        string
	VariableName string
	Count        int
	MaxCount     int
}

func (e *ErrArrayTooLargeDetail) Error() string {
	return fmt.Sprintf("variable %q array too large in %s: got %d elements, max %d",
		e.VariableName, e.Level, e.Count, e.MaxCount)
}

func (e *ErrArrayTooLargeDetail) Unwrap() error {
	return ErrArrayTooLarge
}

// ErrInvalidArrayElementDetail is returned when an array element is not a string.
type ErrInvalidArrayElementDetail struct {
	Level        string
	VariableName string
	Index        int
	ExpectedType string
	ActualType   string
}

func (e *ErrInvalidArrayElementDetail) Error() string {
	return fmt.Sprintf("variable %q has invalid array element at index %d in %s: expected %s, got %s",
		e.VariableName, e.Index, e.Level, e.ExpectedType, e.ActualType)
}

func (e *ErrInvalidArrayElementDetail) Unwrap() error {
	return ErrInvalidArrayElement
}

// ErrArrayElementTooLongDetail is returned when an array element exceeds MaxStringValueLen.
type ErrArrayElementTooLongDetail struct {
	Level        string
	VariableName string
	Index        int
	Length       int
	MaxLength    int
}

func (e *ErrArrayElementTooLongDetail) Error() string {
	return fmt.Sprintf("variable %q array element %d too long in %s: got %d bytes, max %d",
		e.VariableName, e.Index, e.Level, e.Length, e.MaxLength)
}

func (e *ErrArrayElementTooLongDetail) Unwrap() error {
	return ErrValueTooLong
}

// ErrUnsupportedTypeDetail is returned when a variable value has an unsupported type.
type ErrUnsupportedTypeDetail struct {
	Level        string
	VariableName string
	ActualType   string
}

func (e *ErrUnsupportedTypeDetail) Error() string {
	return fmt.Sprintf("variable %q has unsupported type %s in %s: only string and []string are supported",
		e.VariableName, e.ActualType, e.Level)
}

func (e *ErrUnsupportedTypeDetail) Unwrap() error {
	return ErrUnsupportedType
}

// ErrArrayVariableInStringContextDetail is returned when an array variable
// is referenced in a string context (e.g., "%{array_var}" in a string value).
type ErrArrayVariableInStringContextDetail struct {
	Level        string
	Field        string
	VariableName string
	Chain        []string // expansion path leading to this error
}

func (e *ErrArrayVariableInStringContextDetail) Error() string {
	msg := fmt.Sprintf("cannot reference array variable %q in string context at %s.%s: "+
		"array variables can only be used where array values are expected",
		e.VariableName, e.Level, e.Field)
	if len(e.Chain) > 0 {
		msg += fmt.Sprintf(" (expansion path: %s)", strings.Join(e.Chain, " -> "))
	}
	return msg
}

func (e *ErrArrayVariableInStringContextDetail) Unwrap() error {
	return ErrArrayVariableInStringContext
}

// ErrEnvImportVarsConflictDetail provides detailed information about conflicts
// between env_import and vars definitions.
// This error is returned when the same variable name is defined in both env_import
// and vars, either at the same level or across different levels (global/group/command).
type ErrEnvImportVarsConflictDetail struct {
	Level          string // Current level (e.g., "global", "group[deploy]", "command[build]")
	VariableName   string // The conflicting variable name
	EnvImportLevel string // Level where env_import defined this variable
	VarsLevel      string // Level where vars defined this variable
}

func (e *ErrEnvImportVarsConflictDetail) Error() string {
	return fmt.Sprintf("variable %q conflicts between env_import and vars in %s: "+
		"defined in env_import at %s and vars at %s",
		e.VariableName, e.Level, e.EnvImportLevel, e.VarsLevel)
}

func (e *ErrEnvImportVarsConflictDetail) Unwrap() error {
	return ErrEnvImportVarsConflict
}

// ErrLocalVariableInTemplate is returned when a template references a local variable
type ErrLocalVariableInTemplate struct {
	TemplateName string
	Field        string // e.g., "cmd", "args[0]", "env[PATH]"
	VariableName string
}

func (e *ErrLocalVariableInTemplate) Error() string {
	return fmt.Sprintf(
		"template %q field %q: cannot reference local variable %q (templates can only reference global variables starting with uppercase)",
		e.TemplateName,
		e.Field,
		e.VariableName,
	)
}

// ErrUndefinedGlobalVariableInTemplate is returned when a template references an undefined global variable
type ErrUndefinedGlobalVariableInTemplate struct {
	TemplateName string
	Field        string
	VariableName string
}

func (e *ErrUndefinedGlobalVariableInTemplate) Error() string {
	return fmt.Sprintf(
		"template %q field %q: global variable %q is not defined in [global.vars]",
		e.TemplateName,
		e.Field,
		e.VariableName,
	)
}

// ErrInvalidVariableScopeDetail is returned when a variable name doesn't match the expected scope
type ErrInvalidVariableScopeDetail struct {
	Level        string
	Field        string
	VariableName string
	Reason       string
}

func (e *ErrInvalidVariableScopeDetail) Error() string {
	return fmt.Sprintf(
		"invalid variable scope in %s.%s: variable %q - %s",
		e.Level,
		e.Field,
		e.VariableName,
		e.Reason,
	)
}
