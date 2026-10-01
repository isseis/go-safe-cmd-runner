package config

import (
	"errors"
	"strconv"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// Variable value type names used in type-mismatch error details.
const (
	typeNameString = "string"
	typeNameArray  = "array"
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

	// ErrDuplicateCommandName is returned when two commands in the same group
	// share a name. A notification scope is "group/command", so two commands
	// with one name in one group would render as the same scope.
	ErrDuplicateCommandName = errors.New("duplicate command name in group")

	// ErrEmptyCommandName is returned when a command has an empty name. Unlike
	// group names, command names were never required to be present.
	ErrEmptyCommandName = errors.New("command has empty name")

	// ErrIdentifierContainsControlCharacter is returned when a command name
	// contains a control character (Unicode general category Cc) or a
	// format-control character (category Cf). Such a name cannot be pointed at
	// in a notification, a log line or an audit record, and bidirectional
	// format controls can make it read as a different name than it is. Group
	// names cannot contain these characters: GroupNamePattern already rejects
	// them.
	ErrIdentifierContainsControlCharacter = errors.New("identifier contains a control character")

	// ErrIdentifierNotDisplayable is returned when a command name retains no
	// character that survives the display-safe interpolation contract and is not
	// Unicode White_Space. A name made only of spaces, for example, is visually
	// empty after interpolation, so a notification cannot point at it.
	ErrIdentifierNotDisplayable = errors.New("identifier has no displayable content")

	// ErrIdentifierTooLong is returned when a group or command name is longer
	// than common.MaxIdentifierBytes. The interpolation contract never truncates
	// identifiers, because two names that differ only beyond a cut would render
	// as the same scope.
	ErrIdentifierTooLong = errors.New("identifier exceeds maximum length")

	// ErrNilConfig is returned when configuration is nil
	ErrNilConfig = errors.New("configuration must not be nil")

	// ErrNegativeTimeout indicates that a timeout value is negative
	ErrNegativeTimeout = errors.New("timeout must not be negative")

	// ErrNegativeOutputSizeLimit indicates that an output_size_limit value is
	// negative. An accepted value is non-negative; 0 means unlimited.
	ErrNegativeOutputSizeLimit = errors.New("output_size_limit must not be negative")

	// ErrInvalidPath is returned when a path validation fails
	ErrInvalidPath = errors.New("invalid path")

	// ErrEmptyPath is returned when a path is empty
	ErrEmptyPath = errors.New("path cannot be empty")

	// ErrInvalidWorkDir is returned when working directory path is not absolute
	ErrInvalidWorkDir = errors.New("working directory must be an absolute path")

	// ErrInvalidEnvImport is returned when env_import contains variables not in env_allowed
	ErrInvalidEnvImport = errors.New("environment variable in env_import is not in env_allowed")

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

	// ErrForbiddenEnvVar is returned when a forbidden environment variable (e.g. LD_LIBRARY_PATH)
	// is referenced in env_import (from_env field).
	// Note: forbidden variables listed in env_allowlist are silently ineffective because
	// the executor unconditionally removes them before spawning the child process.
	ErrForbiddenEnvVar = errors.New("environment variable is forbidden")
)

// levelKind is the kind of a Level. The zero value is levelNone.
type levelKind int

const (
	levelNone levelKind = iota
	levelGlobal
	levelGroup
	levelCommand
	levelTemplate
)

// Level is where a value was being expanded. The zero value is "no level"
// and renders as the empty string.
type Level struct {
	kind levelKind
	name string
}

func globalLevel() Level              { return Level{kind: levelGlobal} }
func groupLevel(name string) Level    { return Level{kind: levelGroup, name: name} }
func commandLevel(name string) Level  { return Level{kind: levelCommand, name: name} }
func templateLevel(name string) Level { return Level{kind: levelTemplate, name: name} }

// GroupLevel declares a group scope. It is the only constructor exported for
// callers outside this package (ExpandWorkDir's callers).
func GroupLevel(name string) Level { return groupLevel(name) }

// CommandLevel declares a command scope.
func CommandLevel(name string) Level { return commandLevel(name) }

// String renders the level the way it has always been rendered for error
// messages. The zero value renders as the empty string.
func (l Level) String() string {
	switch l.kind {
	case levelGlobal:
		return "global"
	case levelGroup:
		return "group[" + l.name + "]"
	case levelCommand:
		return "command[" + l.name + "]"
	case levelTemplate:
		return "template[" + l.name + "]"
	default:
		return ""
	}
}

// parts builds the message parts for the level, declaring the scope name as an
// identifier so it survives whole-value redaction. The zero value has no parts.
func (l Level) parts() []errmsg.Part {
	switch l.kind {
	case levelGlobal:
		return []errmsg.Part{errmsg.Const("global")}
	case levelGroup:
		return []errmsg.Part{errmsg.Const("group["), errmsg.Ident(l.name), errmsg.Const("]")}
	case levelCommand:
		return []errmsg.Part{errmsg.Const("command["), errmsg.Ident(l.name), errmsg.Const("]")}
	case levelTemplate:
		return []errmsg.Part{errmsg.Const("template["), errmsg.Ident(l.name), errmsg.Const("]")}
	default:
		return nil
	}
}

// fieldKey is the configuration field being expanded. The zero value is
// fieldNone.
type fieldKey int

const (
	fieldNone fieldKey = iota
	fieldCmd
	fieldArgs
	fieldEnv
	fieldEnvVars
	fieldEnvImport
	fieldWorkdir
	fieldVerifyFiles
	fieldCmdAllowed
	fieldVars
	fieldOutputFile
	// fieldKeyCount is the number of keys, so tests can range over every key;
	// it must stay last.
	fieldKeyCount
)

// Field is the configuration field being expanded. The zero value is
// "no field" and renders as the empty string. The variable name and the index
// each render only when their presence flag is set, so an empty name still
// renders as "vars." the way fmt.Sprintf("vars.%s", "") always did.
type Field struct {
	key      fieldKey
	name     string // variable name; only the vars constructors set hasName
	hasName  bool
	index    int
	hasIndex bool
}

func cmdField() Field              { return Field{key: fieldCmd} }
func envField() Field              { return Field{key: fieldEnv} }
func envVarsField(index int) Field { return Field{key: fieldEnvVars, index: index, hasIndex: true} }
func envVarsFieldNoIndex() Field   { return Field{key: fieldEnvVars} }
func envImportField() Field        { return Field{key: fieldEnvImport} }
func workdirField() Field          { return Field{key: fieldWorkdir} }
func outputFileField() Field       { return Field{key: fieldOutputFile} }
func argsField(index int) Field    { return Field{key: fieldArgs, index: index, hasIndex: true} }
func argsFieldNoIndex() Field      { return Field{key: fieldArgs} }
func verifyFilesField(index int) Field {
	return Field{key: fieldVerifyFiles, index: index, hasIndex: true}
}

func cmdAllowedField(index int) Field {
	return Field{key: fieldCmdAllowed, index: index, hasIndex: true}
}
func cmdAllowedFieldNoIndex() Field { return Field{key: fieldCmdAllowed} }
func varsField() Field              { return Field{key: fieldVars} }
func varField(name string) Field    { return Field{key: fieldVars, name: name, hasName: true} }
func varElementField(name string, index int) Field {
	return Field{key: fieldVars, name: name, hasName: true, index: index, hasIndex: true}
}

// String renders the field the way it has always been rendered for error
// messages: the key, then ".<name>" and "[<index>]" when present. The zero
// value renders as the empty string.
func (f Field) String() string {
	var key string
	switch f.key {
	case fieldCmd:
		key = "cmd"
	case fieldArgs:
		key = "args"
	case fieldEnv:
		key = "env"
	case fieldEnvVars:
		key = "env_vars"
	case fieldEnvImport:
		key = "env_import"
	case fieldWorkdir:
		key = "workdir"
	case fieldVerifyFiles:
		key = "verify_files"
	case fieldCmdAllowed:
		key = "cmd_allowed"
	case fieldVars:
		key = "vars"
	case fieldOutputFile:
		key = "output_file"
	default:
		return ""
	}
	if f.hasName {
		key += "." + f.name
	}
	if f.hasIndex {
		key += "[" + strconv.Itoa(f.index) + "]"
	}
	return key
}

// parts builds the message parts for the field: the key text is constant, the
// variable name is an identifier and the index is text. The zero value has no
// parts.
func (f Field) parts() []errmsg.Part {
	var parts []errmsg.Part
	switch f.key {
	case fieldCmd:
		parts = []errmsg.Part{errmsg.Const("cmd")}
	case fieldArgs:
		parts = []errmsg.Part{errmsg.Const("args")}
	case fieldEnv:
		parts = []errmsg.Part{errmsg.Const("env")}
	case fieldEnvVars:
		parts = []errmsg.Part{errmsg.Const("env_vars")}
	case fieldEnvImport:
		parts = []errmsg.Part{errmsg.Const("env_import")}
	case fieldWorkdir:
		parts = []errmsg.Part{errmsg.Const("workdir")}
	case fieldVerifyFiles:
		parts = []errmsg.Part{errmsg.Const("verify_files")}
	case fieldCmdAllowed:
		parts = []errmsg.Part{errmsg.Const("cmd_allowed")}
	case fieldVars:
		parts = []errmsg.Part{errmsg.Const("vars")}
	case fieldOutputFile:
		parts = []errmsg.Part{errmsg.Const("output_file")}
	default:
		return nil
	}
	if f.hasName {
		parts = append(parts, errmsg.Const("."), errmsg.Ident(f.name))
	}
	if f.hasIndex {
		parts = append(parts, errmsg.Const("["), errmsg.Text(strconv.Itoa(f.index)), errmsg.Const("]"))
	}
	return parts
}

// ErrInvalidVariableNameDetail provides detailed information about invalid variable names.
// This error type wraps ErrInvalidVariableName and is used for internal variable validation
// in vars and from_env fields.
type ErrInvalidVariableNameDetail struct {
	Level        Level
	Field        Field
	VariableName string
	Reason       string
}

// StructuredMessage declares the level and field through their own parts; the
// rejected name and the reason stay text.
func (e *ErrInvalidVariableNameDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("invalid variable name in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.VariableName),
		errmsg.Const("' ("),
		errmsg.Text(e.Reason),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidVariableNameDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrInvalidVariableNameDetail) Unwrap() error {
	return ErrInvalidVariableName
}

// ErrInvalidSystemVariableNameDetail provides detailed information about invalid system variable names
type ErrInvalidSystemVariableNameDetail struct {
	Level              Level
	Field              Field
	SystemVariableName string
	Reason             string
}

// StructuredMessage declares the level and field through their own parts; the
// rejected name and the reason stay text.
func (e *ErrInvalidSystemVariableNameDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("invalid system variable name in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.SystemVariableName),
		errmsg.Const("' ("),
		errmsg.Text(e.Reason),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidSystemVariableNameDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrInvalidSystemVariableNameDetail) Unwrap() error {
	return ErrInvalidSystemVariableName
}

// ErrReservedVariablePrefixDetail provides detailed information about reserved prefix errors
type ErrReservedVariablePrefixDetail struct {
	Level        Level
	Field        Field
	VariableName string
	Prefix       string
}

// StructuredMessage declares the level and field through their own parts; the
// rejected name and the reserved prefix stay text.
func (e *ErrReservedVariablePrefixDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("reserved variable prefix in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.VariableName),
		errmsg.Const("' (prefix '"),
		errmsg.Text(e.Prefix),
		errmsg.Const("' is reserved)"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrReservedVariablePrefixDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrReservedVariablePrefixDetail) Unwrap() error {
	return ErrReservedVariablePrefix
}

// ErrVariableNotInAllowlistDetail provides detailed information about allowlist violations
type ErrVariableNotInAllowlistDetail struct {
	Level           Level
	SystemVarName   string
	InternalVarName string
	Allowlist       []string
}

// StructuredMessage declares the system and internal names as identifiers, so a
// name that looks like a secret still reaches the report.
func (e *ErrVariableNotInAllowlistDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("system environment variable '")}
	parts = append(parts, errmsg.Ident(e.SystemVarName))
	parts = append(parts, errmsg.Const("' not in allowlist (referenced as '"))
	parts = append(parts, errmsg.Ident(e.InternalVarName))
	parts = append(parts, errmsg.Const("' in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const(".from_env)"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrVariableNotInAllowlistDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrVariableNotInAllowlistDetail) Unwrap() error {
	return ErrVariableNotInAllowlist
}

// ErrCircularReferenceDetail provides detailed information about circular references
type ErrCircularReferenceDetail struct {
	Level        Level
	Field        Field
	VariableName string
	Chain        []string
}

// StructuredMessage declares the variable name and every name on the cycle as
// identifiers; the brackets and separators are constants.
func (e *ErrCircularReferenceDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("circular reference in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": '"))
	parts = append(parts, errmsg.Ident(e.VariableName))
	parts = append(parts, errmsg.Const("' (chain: ["))
	for i, name := range e.Chain {
		if i > 0 {
			parts = append(parts, errmsg.Const(" "))
		}
		parts = append(parts, errmsg.Ident(name))
	}
	parts = append(parts, errmsg.Const("])"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrCircularReferenceDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrCircularReferenceDetail) Unwrap() error {
	return ErrCircularReference
}

// ErrUndefinedVariableDetail provides detailed information about undefined variables
type ErrUndefinedVariableDetail struct {
	Level        Level
	Field        Field
	VariableName string
	Context      string
	Chain        []string // expansion path leading to this error
}

// StructuredMessage declares the referenced variable, the scope name and the
// expansion path as identifiers; the raw template context stays text.
func (e *ErrUndefinedVariableDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("undefined variable in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Ident(e.VariableName),
		errmsg.Const("' (context: "),
		errmsg.Text(e.Context),
		errmsg.Const(")"),
	)
	if len(e.Chain) > 0 {
		parts = append(parts, errmsg.Const(" (expansion path: "))
		for i, name := range e.Chain {
			if i > 0 {
				parts = append(parts, errmsg.Const(" -> "))
			}
			parts = append(parts, errmsg.Ident(name))
		}
		parts = append(parts, errmsg.Const(")"))
	}
	return errmsg.NewMessage(parts...)
}

func (e *ErrUndefinedVariableDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrUndefinedVariableDetail) Unwrap() error {
	return ErrUndefinedVariable
}

// ErrInvalidEscapeSequenceDetail provides detailed information about invalid escape sequences
type ErrInvalidEscapeSequenceDetail struct {
	Level    Level
	Field    Field
	Sequence string
	Context  string
}

// StructuredMessage declares the level and field through their own parts; the
// sequence and the raw context stay text.
func (e *ErrInvalidEscapeSequenceDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("invalid escape sequence in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.Sequence),
		errmsg.Const("' (context: "),
		errmsg.Text(e.Context),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidEscapeSequenceDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrInvalidEscapeSequenceDetail) Unwrap() error {
	return ErrInvalidEscapeSequence
}

// ErrUnclosedVariableReferenceDetail provides detailed information about unclosed variable references
type ErrUnclosedVariableReferenceDetail struct {
	Level   Level
	Field   Field
	Context string
}

// StructuredMessage declares the level and field through their own parts; the
// raw context stays text.
func (e *ErrUnclosedVariableReferenceDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("unclosed variable reference in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": missing closing '}' (context: "),
		errmsg.Text(e.Context),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrUnclosedVariableReferenceDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrUnclosedVariableReferenceDetail) Unwrap() error {
	return ErrUnclosedVariableReference
}

// ErrMaxRecursionDepthExceededDetail provides detailed information about recursion depth limit
type ErrMaxRecursionDepthExceededDetail struct {
	Level    Level
	Field    Field
	MaxDepth int
	Context  string
}

// StructuredMessage declares the level and field through their own parts; the
// limit and the raw context stay text.
func (e *ErrMaxRecursionDepthExceededDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("maximum recursion depth exceeded in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": limit "),
		errmsg.Text(strconv.Itoa(e.MaxDepth)),
		errmsg.Const(" (context: "),
		errmsg.Text(e.Context),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrMaxRecursionDepthExceededDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrMaxRecursionDepthExceededDetail) Unwrap() error {
	return ErrMaxRecursionDepthExceeded
}

// ErrReservedVariableNameDetail is an alias for ErrReservedVariablePrefixDetail
// to maintain consistency with test naming conventions
type ErrReservedVariableNameDetail = ErrReservedVariablePrefixDetail

// ErrInvalidEnvImportFormatDetail provides detailed information about invalid env_import format
type ErrInvalidEnvImportFormatDetail struct {
	Level   Level
	Mapping string
	Reason  string
}

// StructuredMessage declares the level through its own parts; the raw mapping
// and the reason stay text.
func (e *ErrInvalidEnvImportFormatDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("invalid env_import format in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.Mapping),
		errmsg.Const("' ("),
		errmsg.Text(e.Reason),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidEnvImportFormatDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrInvalidEnvImportFormatDetail) Unwrap() error {
	return ErrInvalidEnvImportFormat
}

// ErrInvalidEnvFormatDetail provides detailed information about invalid env format
type ErrInvalidEnvFormatDetail struct {
	Level   Level
	Mapping string
	Reason  string
}

// StructuredMessage declares the level through its own parts; the raw mapping
// and the reason stay text.
func (e *ErrInvalidEnvFormatDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("invalid env format in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.Mapping),
		errmsg.Const("' ("),
		errmsg.Text(e.Reason),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidEnvFormatDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrInvalidEnvFormatDetail) Unwrap() error {
	return ErrInvalidEnvFormat
}

// ErrInvalidEnvKeyDetail provides detailed information about invalid environment variable key
type ErrInvalidEnvKeyDetail struct {
	Level   Level
	Key     string
	Context string
	Reason  string
}

// StructuredMessage declares the level through its own parts; the rejected key
// and the raw entry stay text because the key's origin cannot be proven.
func (e *ErrInvalidEnvKeyDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("invalid environment variable key in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.Key),
		errmsg.Const("' (context: "),
		errmsg.Text(e.Context),
		errmsg.Const(", reason: "),
		errmsg.Text(e.Reason),
		errmsg.Const(")"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidEnvKeyDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrInvalidEnvKeyDetail) Unwrap() error {
	return ErrInvalidEnvKey
}

// ErrDuplicateVariableDefinitionDetail provides detailed information about duplicate variable definitions
type ErrDuplicateVariableDefinitionDetail struct {
	Level        Level
	Field        Field
	VariableName string
}

// StructuredMessage declares the variable name as an identifier; the level and
// field carry their own declared roles.
func (e *ErrDuplicateVariableDefinitionDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("duplicate variable definition in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Ident(e.VariableName),
		errmsg.Const("' is defined multiple times"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrDuplicateVariableDefinitionDetail) Error() string {
	return e.StructuredMessage().String()
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

// StructuredMessage declares the rejected path and the reason as text: the
// value may still hold expanded variable values, so it is not treated as an
// accepted path.
func (e *InvalidPathError) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("invalid path '"),
		errmsg.Text(e.Path),
		errmsg.Const("': "),
		errmsg.Text(e.Reason),
	)
}

func (e *InvalidPathError) Error() string {
	return e.StructuredMessage().String()
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
	Level      Level  // e.g., "group[mygroup]"
	Field      Field  // e.g., "cmd_allowed"
	Path       string // The duplicated path string
	FirstIndex int    // Index of first occurrence
	DupeIndex  int    // Index of duplicate occurrence
}

// StructuredMessage declares the level and field through their own parts; the
// raw path string and the indices stay text.
func (e *ErrDuplicatePathDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("duplicate path in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.Path),
		errmsg.Const("' appears at index "),
		errmsg.Text(strconv.Itoa(e.FirstIndex)),
		errmsg.Const(" and "),
		errmsg.Text(strconv.Itoa(e.DupeIndex)),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrDuplicatePathDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrDuplicatePathDetail) Unwrap() error {
	return ErrDuplicatePath
}

// ErrDuplicateResolvedPathDetail provides detailed information about paths that resolve to the same file.
// This error is returned when different path strings (potentially after variable expansion)
// resolve to the same actual file after symlink resolution.
type ErrDuplicateResolvedPathDetail struct {
	Level        Level  // e.g., "group[mygroup]"
	Field        Field  // e.g., "cmd_allowed"
	OriginalPath string // The original path from config
	ResolvedPath string // The resolved path that is duplicated
}

// StructuredMessage declares the level and field through their own parts; the
// original pre-expansion path stays text and the resolved path is a path.
func (e *ErrDuplicateResolvedPathDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("duplicate resolved path in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": '"),
		errmsg.Text(e.OriginalPath),
		errmsg.Const("' resolves to '"),
		errmsg.Path(e.ResolvedPath),
		errmsg.Const("' which is already in the list"),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrDuplicateResolvedPathDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrDuplicateResolvedPathDetail) Unwrap() error {
	return ErrDuplicateResolvedPath
}

// ===========================================
// New error types for vars table format
// ===========================================

// ErrTooManyVariablesDetail is returned when the number of variables exceeds MaxVarsPerLevel.
type ErrTooManyVariablesDetail struct {
	Level    Level
	Count    int
	MaxCount int
}

// StructuredMessage declares the level through its own parts; the counts stay
// text.
func (e *ErrTooManyVariablesDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("too many variables in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": got "),
		errmsg.Text(strconv.Itoa(e.Count)),
		errmsg.Const(", max "),
		errmsg.Text(strconv.Itoa(e.MaxCount)),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrTooManyVariablesDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrTooManyVariablesDetail) Unwrap() error {
	return ErrTooManyVariables
}

// ErrTypeMismatchDetail is returned when a variable is redefined with a different type (string vs array).
type ErrTypeMismatchDetail struct {
	Level        Level
	VariableName string
	ExpectedType string
	ActualType   string
}

// StructuredMessage declares the variable name as an identifier, quoted the way
// fmt's %q rendered it; the level and the type names carry their own roles.
func (e *ErrTypeMismatchDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" type mismatch in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": already defined as "),
		errmsg.Text(e.ExpectedType),
		errmsg.Const(", cannot redefine as "),
		errmsg.Text(e.ActualType),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrTypeMismatchDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrTypeMismatchDetail) Unwrap() error {
	return ErrTypeMismatch
}

// ErrValueTooLongDetail is returned when a string value exceeds MaxStringValueLen.
type ErrValueTooLongDetail struct {
	Level        Level
	VariableName string
	Length       int
	MaxLength    int
}

// StructuredMessage declares the variable name as an identifier, quoted the way
// fmt's %q rendered it; the level and the byte counts carry their own roles.
func (e *ErrValueTooLongDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" value too long in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": got "),
		errmsg.Text(strconv.Itoa(e.Length)),
		errmsg.Const(" bytes, max "),
		errmsg.Text(strconv.Itoa(e.MaxLength)),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrValueTooLongDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrValueTooLongDetail) Unwrap() error {
	return ErrValueTooLong
}

// ErrArrayTooLargeDetail is returned when an array variable exceeds MaxArrayElements.
type ErrArrayTooLargeDetail struct {
	Level        Level
	VariableName string
	Count        int
	MaxCount     int
}

// StructuredMessage declares the variable name as an identifier, quoted the way
// fmt's %q rendered it; the level and the counts carry their own roles.
func (e *ErrArrayTooLargeDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" array too large in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": got "),
		errmsg.Text(strconv.Itoa(e.Count)),
		errmsg.Const(" elements, max "),
		errmsg.Text(strconv.Itoa(e.MaxCount)),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrArrayTooLargeDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrArrayTooLargeDetail) Unwrap() error {
	return ErrArrayTooLarge
}

// ErrInvalidArrayElementDetail is returned when an array element is not a string.
type ErrInvalidArrayElementDetail struct {
	Level        Level
	VariableName string
	Index        int
	ExpectedType string
	ActualType   string
}

// StructuredMessage declares the variable name as an identifier, quoted the way
// fmt's %q rendered it; the level, the index and the type names carry their own
// roles.
func (e *ErrInvalidArrayElementDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" has invalid array element at index "))
	parts = append(parts, errmsg.Text(strconv.Itoa(e.Index)))
	parts = append(parts, errmsg.Const(" in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": expected "),
		errmsg.Text(e.ExpectedType),
		errmsg.Const(", got "),
		errmsg.Text(e.ActualType),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidArrayElementDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrInvalidArrayElementDetail) Unwrap() error {
	return ErrInvalidArrayElement
}

// ErrArrayElementTooLongDetail is returned when an array element exceeds MaxStringValueLen.
type ErrArrayElementTooLongDetail struct {
	Level        Level
	VariableName string
	Index        int
	Length       int
	MaxLength    int
}

// StructuredMessage declares the variable name as an identifier, quoted the way
// fmt's %q rendered it; the level and the counts carry their own roles.
func (e *ErrArrayElementTooLongDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" array element "))
	parts = append(parts, errmsg.Text(strconv.Itoa(e.Index)))
	parts = append(parts, errmsg.Const(" too long in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts,
		errmsg.Const(": got "),
		errmsg.Text(strconv.Itoa(e.Length)),
		errmsg.Const(" bytes, max "),
		errmsg.Text(strconv.Itoa(e.MaxLength)),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrArrayElementTooLongDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrArrayElementTooLongDetail) Unwrap() error {
	return ErrValueTooLong
}

// ErrUnsupportedTypeDetail is returned when a variable value has an unsupported type.
type ErrUnsupportedTypeDetail struct {
	Level        Level
	VariableName string
	ActualType   string
}

// StructuredMessage declares the variable name as an identifier, quoted the way
// fmt's %q rendered it; the level and the type name carry their own roles.
func (e *ErrUnsupportedTypeDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" has unsupported type "))
	parts = append(parts, errmsg.Text(e.ActualType))
	parts = append(parts, errmsg.Const(" in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const(": only string and []string are supported"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrUnsupportedTypeDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrUnsupportedTypeDetail) Unwrap() error {
	return ErrUnsupportedType
}

// ErrArrayVariableInStringContextDetail is returned when an array variable
// is referenced in a string context (e.g., "%{array_var}" in a string value).
type ErrArrayVariableInStringContextDetail struct {
	Level        Level
	Field        Field
	VariableName string
	Chain        []string // expansion path leading to this error
}

// StructuredMessage declares the variable name and the expansion path as
// identifiers, quoted the way fmt's %q rendered them; the level and field carry
// their own roles.
func (e *ErrArrayVariableInStringContextDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("cannot reference array variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" in string context at "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts, errmsg.Const(": array variables can only be used where array values are expected"))
	if len(e.Chain) > 0 {
		parts = append(parts, errmsg.Const(" (expansion path: "))
		for i, name := range e.Chain {
			if i > 0 {
				parts = append(parts, errmsg.Const(" -> "))
			}
			parts = append(parts, errmsg.Ident(name))
		}
		parts = append(parts, errmsg.Const(")"))
	}
	return errmsg.NewMessage(parts...)
}

func (e *ErrArrayVariableInStringContextDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrArrayVariableInStringContextDetail) Unwrap() error {
	return ErrArrayVariableInStringContext
}

// ErrEnvImportVarsConflictDetail provides detailed information about conflicts
// between env_import and vars definitions.
// This error is returned when the same variable name is defined in both env_import
// and vars, either at the same level or across different levels (global/group/command).
type ErrEnvImportVarsConflictDetail struct {
	Level          Level  // Current level (e.g., "global", "group[deploy]", "command[build]")
	VariableName   string // The conflicting variable name
	EnvImportLevel Level  // Level where env_import defined this variable
	VarsLevel      Level  // Level where vars defined this variable
}

// StructuredMessage declares the conflicting variable name as an identifier,
// quoted the way fmt's %q rendered it; the three levels carry their own roles.
func (e *ErrEnvImportVarsConflictDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("variable ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" conflicts between env_import and vars in "))
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const(": defined in env_import at "))
	parts = append(parts, e.EnvImportLevel.parts()...)
	parts = append(parts, errmsg.Const(" and vars at "))
	parts = append(parts, e.VarsLevel.parts()...)
	return errmsg.NewMessage(parts...)
}

func (e *ErrEnvImportVarsConflictDetail) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrEnvImportVarsConflictDetail) Unwrap() error {
	return ErrEnvImportVarsConflict
}

// ErrLocalVariableInTemplate is returned when a template references a local variable
type ErrLocalVariableInTemplate struct {
	TemplateName string
	Field        Field // e.g., "cmd", "args[0]", "env[PATH]"
	VariableName string
}

// StructuredMessage declares the template name, the field and the variable name
// as quoted parts, so the middle dot of the field keeps its own roles.
func (e *ErrLocalVariableInTemplate) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.TemplateName)))
	parts = append(parts, errmsg.Const(" field "))
	parts = append(parts, errmsg.Quoted(e.Field.parts()...))
	parts = append(parts, errmsg.Const(": cannot reference local variable "))
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" (templates can only reference global variables starting with uppercase)"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrLocalVariableInTemplate) Error() string {
	return e.StructuredMessage().String()
}

// ErrUndefinedGlobalVariableInTemplate is returned when a template references an undefined global variable
type ErrUndefinedGlobalVariableInTemplate struct {
	TemplateName string
	Field        Field
	VariableName string
}

// StructuredMessage declares the template name, the field and the variable name
// as quoted parts, so the middle dot of the field keeps its own roles.
func (e *ErrUndefinedGlobalVariableInTemplate) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("template ")}
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.TemplateName)))
	parts = append(parts, errmsg.Const(" field "))
	parts = append(parts, errmsg.Quoted(e.Field.parts()...))
	parts = append(parts, errmsg.Const(": global variable "))
	parts = append(parts, errmsg.Quoted(errmsg.Ident(e.VariableName)))
	parts = append(parts, errmsg.Const(" is not defined in [global.vars]"))
	return errmsg.NewMessage(parts...)
}

func (e *ErrUndefinedGlobalVariableInTemplate) Error() string {
	return e.StructuredMessage().String()
}

// ErrInvalidVariableScopeDetail is returned when a variable name doesn't match the expected scope
type ErrInvalidVariableScopeDetail struct {
	Level        Level
	Field        Field
	VariableName string
	// Err is the underlying scope-validation failure. It is kept as an error
	// rather than flattened into text so callers can tell the reasons apart
	// with errors.Is/As instead of matching the rendered message.
	Err error
}

// StructuredMessage declares the variable name as quoted text and keeps the
// underlying scope failure as a cause, so errors.Is and errors.AsType reach it.
func (e *ErrInvalidVariableScopeDetail) StructuredMessage() errmsg.Message {
	parts := []errmsg.Part{errmsg.Const("invalid variable scope in ")}
	parts = append(parts, e.Level.parts()...)
	parts = append(parts, errmsg.Const("."))
	parts = append(parts, e.Field.parts()...)
	parts = append(parts,
		errmsg.Const(": variable "),
		errmsg.Quoted(errmsg.Text(e.VariableName)),
		errmsg.Const(" - "),
		errmsg.Cause(e.Err),
	)
	return errmsg.NewMessage(parts...)
}

func (e *ErrInvalidVariableScopeDetail) Error() string {
	return e.StructuredMessage().String()
}

// Unwrap returns the underlying scope-validation failure.
func (e *ErrInvalidVariableScopeDetail) Unwrap() error { return e.Err }

// ErrIncludedFileNotFound is returned when an included file does not exist.
type ErrIncludedFileNotFound struct {
	// IncludePath is the path as written in the includes array
	IncludePath string

	// ResolvedPath is the resolved absolute path
	ResolvedPath string

	// ReferencedFrom is the path of the config file containing the include
	ReferencedFrom string
}

// StructuredMessage declares the three paths as paths; the fixed lines and
// indentation are constants.
func (e *ErrIncludedFileNotFound) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("included file not found\n  Include path: "),
		errmsg.Path(e.IncludePath),
		errmsg.Const(" (as written)\n  Resolved path: "),
		errmsg.Path(e.ResolvedPath),
		errmsg.Const("\n  Referenced from: "),
		errmsg.Path(e.ReferencedFrom),
	)
}

func (e *ErrIncludedFileNotFound) Error() string {
	return e.StructuredMessage().String()
}

// ErrTemplateFileInvalidFormat is returned when a template file contains
// unknown fields or sections.
type ErrTemplateFileInvalidFormat struct {
	// TemplateFile is the path to the template file
	TemplateFile string

	// ParseError is the original error from go-toml
	ParseError error
}

// StructuredMessage declares the template file as a path and keeps the parser
// error as a cause, so its structure is preserved when it has one.
func (e *ErrTemplateFileInvalidFormat) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("template file contains invalid fields or sections\n  File: "),
		errmsg.Path(e.TemplateFile),
		errmsg.Const("\n  Template files can only contain 'version' and 'command_templates'\n  Detail: "),
		errmsg.Cause(e.ParseError),
	)
}

func (e *ErrTemplateFileInvalidFormat) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrTemplateFileInvalidFormat) Unwrap() error {
	return e.ParseError
}
