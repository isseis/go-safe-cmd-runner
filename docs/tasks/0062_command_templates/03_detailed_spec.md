# コマンドテンプレート機能 - 詳細仕様書

## 1. 型定義

### 1.1 CommandTemplate 構造体

```go
// CommandTemplate represents a reusable command definition.
// Templates are defined in the [command_templates] section of TOML and
// can be referenced by CommandSpec using the Template field.
//
// Template parameters use the following syntax:
//   - ${param}   : Required string parameter
//   - ${?param}  : Optional string parameter (removed if empty)
//   - ${@param}  : Array parameter (elements are expanded in place)
//   - \$         : Literal $ character (in TOML: \\$)
//
// Example TOML:
//
//	[command_templates.restic_backup]
//	cmd = "restic"
//	args = ["${@verbose_flags}", "backup", "${path}"]
type CommandTemplate struct {
	// Cmd is the command path (may contain template parameters)
	// REQUIRED field
	Cmd string `toml:"cmd"`

	// Args is the list of command arguments (may contain template parameters)
	// Optional, defaults to empty array
	Args []string `toml:"args"`

	// Env is the list of environment variables in KEY=VALUE format
	// (may contain template parameters in the VALUE part)
	// Optional, defaults to empty array
	Env []string `toml:"env"`

	// WorkDir is the working directory for the command (optional)
	WorkDir string `toml:"workdir"`

	// Timeout specifies the command timeout in seconds (optional)
	// nil: inherit from group/global, 0: unlimited, positive: timeout in seconds
	Timeout *int32 `toml:"timeout"`

	// OutputSizeLimit specifies the maximum output size in bytes (optional)
	// nil: inherit from global, 0: unlimited, positive: limit in bytes
	OutputSizeLimit *int64 `toml:"output_size_limit"`

	// RiskLevel specifies the maximum allowed risk level (optional)
	// Valid values: "low", "medium", "high"
	RiskLevel string `toml:"risk_level"`

	// NOTE: The "name" field is NOT allowed in template definitions.
	// Command names must be specified in the [[groups.commands]] section
	// when referencing the template.
}
```

### 1.2 ConfigSpec への追加

```go
// ConfigSpec represents the root configuration structure loaded from TOML file.
type ConfigSpec struct {
	// Version specifies the configuration file version (e.g., "1.0")
	Version string `toml:"version"`

	// Global contains global-level configuration
	Global GlobalSpec `toml:"global"`

	// CommandTemplates contains reusable command template definitions.
	// Templates are defined using TOML table syntax:
	//   [command_templates.template_name]
	//   cmd = "..."
	//   args = [...]
	//
	// Template names must:
	//   - Start with a letter or underscore
	//   - Contain only letters, digits, and underscores
	//   - Not start with "__" (reserved for future use)
	CommandTemplates map[string]CommandTemplate `toml:"command_templates"`

	// Groups contains all command groups defined in the configuration
	Groups []GroupSpec `toml:"groups"`
}
```

### 1.3 CommandSpec への追加

```go
// CommandSpec represents a single command configuration loaded from TOML file.
type CommandSpec struct {
	// Basic information
	Name        string `toml:"name"`        // Command name (REQUIRED, must be unique within group)
	Description string `toml:"description"` // Human-readable description

	// Template reference (mutually exclusive with Cmd, Args, Env, WorkDir)
	// When Template is set, the command definition is loaded from the
	// referenced CommandTemplate and Params are applied.
	Template string `toml:"template"`

	// Params contains template parameter values.
	// Each key corresponds to a parameter placeholder in the template.
	// Values can be:
	//   - string: for ${param} and ${?param} placeholders
	//   - []any: for ${@param} placeholders (elements must be string)
	//
	// Params can contain variable references (%{var}) which will be expanded
	// AFTER template expansion (see F-006 in requirements.md).
	//
	// Example TOML:
	//   [[groups.commands]]
	//   name = "backup_volumes"  # REQUIRED (must be unique within group)
	//   template = "restic_backup"
	//   params.verbose_flags = ["-q"]
	//   params.path = "%{backup_dir}/data"  # %{} is allowed in params
	Params map[string]interface{} `toml:"params"`

	// Command definition (raw values, not yet expanded)
	// These fields are MUTUALLY EXCLUSIVE with Template:
	//   - If Template is set, these fields MUST NOT be set (validation error)
	//   - If Template is not set, Cmd is REQUIRED
	Cmd     string   `toml:"cmd"`     // Command path (may contain variables like %{VAR})
	Args    []string `toml:"args"`    // Command arguments (may contain variables)
	Env     []string `toml:"env"`     // Environment variables (KEY=VALUE format)
	WorkDir string   `toml:"workdir"` // Working directory

	// ... (other existing fields remain unchanged)
}
```

## 2. パラメータ展開アルゴリズム

### 2.1 展開関数のシグネチャ

```go
// ExpandTemplateParams expands template parameter placeholders in a string.
//
// Parameters:
//   - input: String containing placeholders (${param}, ${?param}, ${@param})
//   - params: Map of parameter names to values (string or []string)
//   - templateName: Name of the template (for error messages)
//   - field: Field name being expanded (for error messages)
//
// Returns:
//   - []string: Expanded strings (may be multiple for array expansion)
//   - error: Expansion error
//
// Expansion rules:
//   - ${param}  : Replace with string value, error if not found
//   - ${?param} : Replace with string value, remove element if empty/not found
//   - ${@param} : Replace with array elements, remove element if empty/not found
//   - \$        : Replace with literal $ (escape sequence)
func ExpandTemplateParams(
	input string,
	params map[string]interface{},
	templateName string,
	field string,
) ([]string, error)
```

### 2.2 展開アルゴリズムの詳細

```go
// ExpandTemplateArgs expands template placeholders in an args array.
//
// This function handles the expansion of all three placeholder types
// and produces the final args array.
//
// Algorithm:
//  1. For each element in the input array:
//     a. Scan for placeholders (${...}, ${?...}, ${@...})
//     b. Classify the element:
//        - Pure ${@param}: array expansion mode
//        - Pure ${?param}: optional mode
//        - Mixed or ${param}: string replacement mode
//     c. Apply expansion based on classification
//  2. Concatenate all results
//  3. Apply escape sequence transformation (\$ -> $, \\ -> \)
//
// Example:
//   Input:  ["${@flags}", "backup", "${?verbose}", "${path}"]
//   Params: {flags: ["-q"], verbose: "", path: "/data"}
//   Output: ["-q", "backup", "/data"]
func ExpandTemplateArgs(
	args []string,
	params map[string]interface{},
	templateName string,
) ([]string, error) {
	var result []string

	for i, arg := range args {
		expanded, err := expandSingleArg(arg, params, templateName, fmt.Sprintf("args[%d]", i))
		if err != nil {
			return nil, err
		}
		result = append(result, expanded...)
	}

	// Apply escape sequence transformation
	for i := range result {
		result[i] = applyEscapeSequences(result[i])
	}

	return result, nil
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
```

### 2.3 プレースホルダー解析

```go
// placeholderType represents the type of a template placeholder
type placeholderType int

const (
	placeholderRequired placeholderType = iota // ${param}
	placeholderOptional                        // ${?param}
	placeholderArray                           // ${@param}
)

// placeholder represents a parsed placeholder in a template string
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
//   placeholder := "${" modifier? name "}"
//   modifier    := "?" | "@"
//   name        := [A-Za-z_][A-Za-z0-9_]*
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
		if i+2 < len(input) && input[i] == '$' && input[i+1] == '{' {
			// Find closing brace
			closeIdx := strings.IndexByte(input[i+2:], '}')
			if closeIdx == -1 {
				return nil, &ErrUnclosedPlaceholder{
					Input:    input,
					Position: i,
				}
			}
			closeIdx += i + 2

			// Extract content between ${ and }
			content := input[i+2 : closeIdx]
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
```

### 2.4 単一引数の展開

```go
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

	// No placeholders - return as-is
	if len(placeholders) == 0 {
		return []string{arg}, nil
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
				return nil, &ErrInvalidArrayElement{
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
		return nil, &ErrTypeMismatch{
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
		return nil, &ErrTypeMismatch{
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
				return nil, &ErrTypeMismatch{
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
					return nil, &ErrTypeMismatch{
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

	// Check if result is empty after optional expansion
	if result == "" {
		return []string{}, nil
	}

	return []string{result}, nil
}
```

## 3. セキュリティ検証

### 3.1 params 値の検証

```go
// ValidateParams validates all parameter values for security.
//
// This function performs the following checks:
//  1. Parameter name validation
//  2. Type validation
//
// Note: Variable references (%{var}) are allowed in params to enable
// local variable usage (NF-006).
//
// Note: Command injection and path traversal validation is NOT performed here.
// These checks are applied to the expanded command definition after template
// and variable expansion, allowing context-appropriate validation based on
// where the parameter values are actually used (cmd, args, env, etc.).
func ValidateParams(
	params map[string]interface{},
	templateName string,
) error {
	for name, value := range params {
		// Validate parameter name
		if err := security.ValidateVariableName(name); err != nil {
			return &ErrInvalidParamName{
				TemplateName: templateName,
				ParamName:    name,
				Reason:       err.Error(),
			}
		}

		// Validate value based on type
		switch v := value.(type) {
		case string:
			if err := validateParamString(v, name, templateName); err != nil {
				return err
			}

		case []interface{}:
			for i, elem := range v {
				str, ok := elem.(string)
				if !ok {
					return &ErrInvalidArrayElement{
						TemplateName: templateName,
						Field:        "params",
						ParamName:    name,
						Index:        i,
						ActualType:   fmt.Sprintf("%T", elem),
					}
				}
				if err := validateParamString(str, fmt.Sprintf("%s[%d]", name, i), templateName); err != nil {
					return err
				}
			}

		default:
			return &ErrUnsupportedParamType{
				TemplateName: templateName,
				Field:        "params",
				ParamName:    name,
				ActualType:   fmt.Sprintf("%T", value),
			}
		}
	}

	return nil
}

// validateParamString validates a single string parameter value.
func validateParamString(value, paramName, templateName string) error {
	// No security checks on raw param values.
	// Variable references (%{var}) are allowed in params (NF-006).
	// Security checks (command injection, path traversal, etc.)
	// are performed on the expanded command definition.
	return nil
}
```

### 3.2 テンプレート名の検証

```go
// ValidateTemplateName validates a template name.
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
```

### 3.3 テンプレート定義の検証

```go
// ValidateTemplateDefinition validates a template definition for security.
//
// This function enforces NF-006: Variable references (%{var}) are NOT allowed
// in template definitions to prevent context-dependent security issues.
//
// Rationale:
//  - Templates are reused across multiple groups with different variable contexts
//  - A variable reference safe in one group may expose secrets in another group
//  - Variable references should be explicit in params, not hidden in templates
//
// Example attack scenario prevented by this check:
//   [command_templates.dangerous]
//   cmd = "echo"
//   args = ["%{secret_password}"]  # FORBIDDEN: context-dependent risk
//
//   # Safe in development group
//   [[groups]]
//   name = "dev"
//   [groups.vars]
//   secret_password = "dev_message"
//
//   # But leaks secrets in production group
//   [[groups]]
//   name = "prod"
//   [groups.vars]
//   secret_password = "prod_db_pass_xyz"  # LEAKED!
//
// Safe alternative:
//   [command_templates.safe]
//   cmd = "echo"
//   args = ["${message}"]  # Parameter reference only
//
//   [[groups.commands]]
//   template = "safe"
//   params.message = "%{secret_password}"  # Explicit, visible
func ValidateTemplateDefinition(
	name string,
	template *CommandTemplate,
) error {
	// NOTE: Since CommandTemplate is parsed from TOML, there's no "Name" field
	// in the struct itself. The template name comes from the TOML table key.
	// However, we need to validate that the TOML doesn't contain a "name" field.
	// This check should be done during TOML parsing in the loader.

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
```

### 3.4 CommandSpec の排他性検証

```go
// ValidateCommandSpecExclusivity validates that template and command fields
// are mutually exclusive in a CommandSpec.
//
// When Template is set, the following fields MUST NOT be set:
//   - Cmd
//   - Args
//   - Env
//   - WorkDir
//
// The Name field is allowed with Template (to specify the command name).
//
// This enforces the "complete exclusivity" design (Option A) where
// templates provide all command execution fields, and the calling site
// can only specify Name and Params.
func ValidateCommandSpecExclusivity(
	groupName string,
	commandIndex int,
	spec *CommandSpec,
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

	if len(spec.Env) > 0 {
		return &ErrTemplateFieldConflict{
			GroupName:    groupName,
			CommandIndex: commandIndex,
			TemplateName: spec.Template,
			Field:        "env",
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
```

### 3.5 使用パラメータの収集

```go
// CollectUsedParams extracts all parameter names used in a template.
// This is used for:
//  1. Required params validation
//  2. Unused params warning
func CollectUsedParams(template *CommandTemplate) (map[string]struct{}, error) {
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

	// Collect from env
	for _, env := range template.Env {
		// Only check the value part (after =)
		if idx := strings.IndexByte(env, '='); idx != -1 {
			if err := collectFromString(env[idx+1:], used); err != nil {
				return nil, err
			}
		}
	}

	return used, nil
}

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
```

## 4. テンプレート展開の統合

### 4.1 ExpandCommand への統合

```go
// ExpandCommand expands a CommandSpec into a RuntimeCommand.
// If the CommandSpec references a template, it first applies the template.
//
// Processing order:
//  1. Template resolution (if template field is set)
//  2. Template definition validation (NF-006: %{ pattern forbidden)
//  3. Template parameter expansion (${...})
//  4. Variable inheritance from group
//  5. Variable expansion (%{...} - allowed in params)
//  6. Security validation (NF-005: applied to expanded command)
//
// Security validation (step 6) includes:
//  - cmd_allowed / AllowedCommands check
//  - Command injection pattern detection in cmd and args
//  - Path traversal detection
//  - Environment variable validation
//
// These validations are applied to the EXPANDED command, ensuring that
// context-appropriate checks are performed based on where parameter values
// are actually used (cmd, args, env, etc.).
func ExpandCommand(
	spec *runnertypes.CommandSpec,
	templates map[string]runnertypes.CommandTemplate, // Added parameter
	runtimeGroup *runnertypes.RuntimeGroup,
	globalRuntime *runnertypes.RuntimeGlobal,
	globalTimeout common.Timeout,
	globalOutputSizeLimit common.OutputSizeLimit,
) (*runnertypes.RuntimeCommand, error) {
	// Check for template reference
	if spec.Template != "" {
		// Validate mutual exclusivity
		if spec.Cmd != "" || len(spec.Args) > 0 || len(spec.EnvVars) > 0 {
			return nil, &ErrTemplateFieldConflict{
				CommandName:  spec.Name,
				TemplateName: spec.Template,
			}
		}

		// Resolve template
		template, ok := templates[spec.Template]
		if !ok {
			return nil, &ErrTemplateNotFound{
				CommandName:  spec.Name,
				TemplateName: spec.Template,
			}
		}

		// Validate template definition (NF-006: no %{ in template)
		if err := ValidateTemplateDefinition(spec.Template, &template); err != nil {
			return nil, fmt.Errorf("command[%s]: %w", spec.Name, err)
		}

		// Validate params (name validation only)
		if err := ValidateParams(spec.Params, spec.Template); err != nil {
			return nil, fmt.Errorf("command[%s]: %w", spec.Name, err)
		}

		// Expand template into a new CommandSpec
		expandedSpec, warnings, err := expandTemplateToSpec(spec, &template)
		if err != nil {
			return nil, fmt.Errorf("command[%s]: %w", spec.Name, err)
		}

		// Log warnings for unused params
		for _, w := range warnings {
			// Use logging package to emit warning
			logging.Warn("command[%s]: %s", spec.Name, w)
		}

		// Continue with the expanded spec
		spec = expandedSpec
	}

	// Continue with existing expansion logic...
	// The expanded CommandSpec goes through:
	//  - Variable expansion (%{...})
	//  - Security validation (NF-005)
	// This ensures all security checks are applied to the final expanded values.
	// (rest of the function remains the same)
}

// expandTemplateToSpec expands a template into a CommandSpec.
// Returns the expanded spec, any warnings, and any error.
func expandTemplateToSpec(
	cmdSpec *runnertypes.CommandSpec,
	template *runnertypes.CommandTemplate,
) (*runnertypes.CommandSpec, []string, error) {
	var warnings []string
	params := cmdSpec.Params
	templateName := cmdSpec.Template

	// Collect used parameters for warning detection
	usedParams, err := CollectUsedParams(template)
	if err != nil {
		return nil, nil, err
	}

	// Check for unused params
	for paramName := range params {
		if _, ok := usedParams[paramName]; !ok {
			warnings = append(warnings, fmt.Sprintf(
				"unused parameter %q in template %q",
				paramName, templateName,
			))
		}
	}

	// Expand cmd
	expandedCmd, err := expandTemplateString(template.Cmd, params, templateName, "cmd")
	if err != nil {
		return nil, nil, err
	}

	// Expand args
	expandedArgs, err := ExpandTemplateArgs(template.Args, params, templateName)
	if err != nil {
		return nil, nil, err
	}

	// Expand env
	expandedEnv, err := expandTemplateEnv(template.Env, params, templateName)
	if err != nil {
		return nil, nil, err
	}

	// Create expanded CommandSpec
	expanded := &runnertypes.CommandSpec{
		Name:            cmdSpec.Name,
		Description:     cmdSpec.Description,
		Cmd:             expandedCmd,
		Args:            expandedArgs,
		EnvVars:         expandedEnv,
		WorkDir:         template.WorkDir,
		Timeout:         template.Timeout,
		OutputSizeLimit: template.OutputSizeLimit,
		RiskLevel:       template.RiskLevel,
		// Inherit other fields from cmdSpec if needed
		Vars:      cmdSpec.Vars,
		EnvImport: cmdSpec.EnvImport,
	}

	return expanded, warnings, nil
}

// expandTemplateString expands a template string that should produce a single value.
func expandTemplateString(
	input string,
	params map[string]interface{},
	templateName string,
	field string,
) (string, error) {
	results, err := expandSingleArg(input, params, templateName, field)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", nil
	}

	if len(results) > 1 {
		return "", &ErrMultipleValuesInStringContext{
			TemplateName: templateName,
			Field:        field,
		}
	}

	// Apply escape sequence transformation
	return applyEscapeSequences(results[0]), nil
}

// expandTemplateEnv expands template parameters in env array.
func expandTemplateEnv(
	env []string,
	params map[string]interface{},
	templateName string,
) ([]string, error) {
	var result []string

	for i, envVar := range env {
		// Parse KEY=VALUE
		idx := strings.IndexByte(envVar, '=')
		if idx == -1 {
			return nil, fmt.Errorf("invalid env format at index %d: %s", i, envVar)
		}

		key := envVar[:idx]
		value := envVar[idx+1:]

		// Expand value
		expandedValue, err := expandTemplateString(value, params, templateName, fmt.Sprintf("env[%d]", i))
		if err != nil {
			return nil, err
		}

		result = append(result, key+"="+expandedValue)
	}

	return result, nil
}
```

## 5. エラー型定義

```go
// Template-related errors

// ErrTemplateNotFound is returned when a referenced template does not exist
type ErrTemplateNotFound struct {
	CommandName  string
	TemplateName string
}

func (e *ErrTemplateNotFound) Error() string {
	return fmt.Sprintf("template %q not found (referenced by command %q)",
		e.TemplateName, e.CommandName)
}

// ErrTemplateFieldConflict is returned when both template and execution fields are specified
type ErrTemplateFieldConflict struct {
	GroupName    string
	CommandIndex int
	TemplateName string
	Field        string // "cmd", "args", "env", "workdir"
}

func (e *ErrTemplateFieldConflict) Error() string {
	return fmt.Sprintf("group[%s] command[%d]: cannot specify both \"template\" and \"%s\" fields in command definition",
		e.GroupName, e.CommandIndex, e.Field)
}

// ErrDuplicateTemplateName is returned when a template name is defined more than once
type ErrDuplicateTemplateName struct {
	Name string
}

func (e *ErrDuplicateTemplateName) Error() string {
	return fmt.Sprintf("duplicate template name %q", e.Name)
}

// ErrInvalidTemplateName is returned when a template name is invalid
type ErrInvalidTemplateName struct {
	Name   string
	Reason string
}

func (e *ErrInvalidTemplateName) Error() string {
	return fmt.Sprintf("invalid template name %q: %s", e.Name, e.Reason)
}

// ErrReservedTemplateName is returned when a template name uses a reserved prefix
type ErrReservedTemplateName struct {
	Name string
}

func (e *ErrReservedTemplateName) Error() string {
	return fmt.Sprintf("template name %q uses reserved prefix '__'", e.Name)
}

// ErrTemplateContainsNameField is returned when a template definition contains a "name" field
type ErrTemplateContainsNameField struct {
	TemplateName string
}

func (e *ErrTemplateContainsNameField) Error() string {
	return fmt.Sprintf("template definition %q cannot contain \"name\" field",
		e.TemplateName)
}

// ErrMissingRequiredField is returned when a required field is missing
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

// ErrRequiredParamMissing is returned when a required parameter is not provided
type ErrRequiredParamMissing struct {
	TemplateName string
	Field        string
	ParamName    string
}

func (e *ErrRequiredParamMissing) Error() string {
	return fmt.Sprintf("template %q %s: required parameter %q not provided",
		e.TemplateName, e.Field, e.ParamName)
}

// ErrTypeMismatch is returned when a parameter value has the wrong type
type ErrTypeMismatch struct {
	TemplateName string
	Field        string
	ParamName    string
	Expected     string
	Actual       string
}

func (e *ErrTypeMismatch) Error() string {
	return fmt.Sprintf("template %q %s: parameter %q expected %s, got %s",
		e.TemplateName, e.Field, e.ParamName, e.Expected, e.Actual)
}

// ErrForbiddenPatternInTemplate is returned when a template definition contains
// a forbidden variable reference pattern (%{var}) - enforces NF-006
type ErrForbiddenPatternInTemplate struct {
	TemplateName string
	Field        string
	Value        string
}

func (e *ErrForbiddenPatternInTemplate) Error() string {
	return fmt.Sprintf("template %q contains forbidden pattern \"%%{\" in %s: variable references are not allowed in template definitions for security reasons (see NF-006)",
		e.TemplateName, e.Field)
}


// ErrArrayInMixedContext is returned when ${@param} is used in a mixed context
type ErrArrayInMixedContext struct {
	TemplateName string
	Field        string
	ParamName    string
}

func (e *ErrArrayInMixedContext) Error() string {
	return fmt.Sprintf("template %q %s: array parameter ${@%s} cannot be used in mixed context",
		e.TemplateName, e.Field, e.ParamName)
}

// ErrInvalidArrayElement is returned when an array parameter contains non-string elements
type ErrInvalidArrayElement struct {
	TemplateName string
	Field        string
	ParamName    string
	Index        int
	ActualType   string
}

func (e *ErrInvalidArrayElement) Error() string {
	return fmt.Sprintf("template %q %s: array parameter %q contains non-string element at index %d (type: %s)",
		e.TemplateName, e.Field, e.ParamName, e.Index, e.ActualType)
}

// ErrUnsupportedParamType is returned when a parameter has an unsupported type
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

// ErrInvalidParamName is returned when a parameter name is invalid
type ErrInvalidParamName struct {
	TemplateName string
	ParamName    string
	Reason       string
}

func (e *ErrInvalidParamName) Error() string {
	return fmt.Sprintf("template %q: invalid parameter name %q: %s",
		e.TemplateName, e.ParamName, e.Reason)
}

// ErrEmptyPlaceholderName is returned when a placeholder has an empty name
type ErrEmptyPlaceholderName struct {
	Input    string
	Position int
}

func (e *ErrEmptyPlaceholderName) Error() string {
	return fmt.Sprintf("empty placeholder name at position %d in %q", e.Position, e.Input)
}

// ErrMultipleValuesInStringContext is returned when array expansion produces multiple values in a string context
type ErrMultipleValuesInStringContext struct {
	TemplateName string
	Field        string
}

func (e *ErrMultipleValuesInStringContext) Error() string {
	return fmt.Sprintf("template %q %s: array expansion produced multiple values in string context",
		e.TemplateName, e.Field)
}

// Placeholder parsing errors

// ErrUnclosedPlaceholder is returned when a placeholder is not closed
type ErrUnclosedPlaceholder struct {
	Input    string
	Position int
}

func (e *ErrUnclosedPlaceholder) Error() string {
	return fmt.Sprintf("unclosed placeholder at position %d in %q", e.Position, e.Input)
}

// ErrEmptyPlaceholder is returned when a placeholder is empty
type ErrEmptyPlaceholder struct {
	Input    string
	Position int
}

func (e *ErrEmptyPlaceholder) Error() string {
	return fmt.Sprintf("empty placeholder at position %d in %q", e.Position, e.Input)
}

// ErrInvalidPlaceholderName is returned when a placeholder name is invalid
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
```

## 6. TOML 読み込みの修正

### 6.1 Loader への統合

```go
// LoadConfig loads and validates configuration from byte content.
// This function now also validates command templates.
func (l *Loader) LoadConfig(content []byte) (*runnertypes.ConfigSpec, error) {
	// Parse the config content
	var cfg runnertypes.ConfigSpec
	if err := toml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate command templates
	if err := ValidateTemplates(cfg.CommandTemplates); err != nil {
		return nil, err
	}

	// Apply default values
	ApplyGlobalDefaults(&cfg.Global)
	for i := range cfg.Groups {
		for j := range cfg.Groups[i].Commands {
			ApplyCommandDefaults(&cfg.Groups[i].Commands[j])
		}
	}

	// Validate timeout values are non-negative
	if err := ValidateTimeouts(&cfg); err != nil {
		return nil, err
	}

	// Validate group names
	if err := ValidateGroupNames(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ValidateTemplates validates all command templates in the configuration.
func ValidateTemplates(templates map[string]runnertypes.CommandTemplate) error {
	for name, template := range templates {
		// Validate template name
		if err := ValidateTemplateName(name); err != nil {
			return err
		}

		// Validate template content
		if err := validateTemplateContent(&template, name); err != nil {
			return err
		}
	}

	return nil
}

// validateTemplateContent validates the content of a single template.
func validateTemplateContent(template *runnertypes.CommandTemplate, name string) error {
	// cmd is required
	if template.Cmd == "" {
		return fmt.Errorf("template %q: cmd is required", name)
	}

	// Validate placeholder syntax in all fields
	if _, err := parsePlaceholders(template.Cmd); err != nil {
		return fmt.Errorf("template %q cmd: %w", name, err)
	}

	for i, arg := range template.Args {
		if _, err := parsePlaceholders(arg); err != nil {
			return fmt.Errorf("template %q args[%d]: %w", name, i, err)
		}
	}

	for i, env := range template.Env {
		// Validate only the value part
		if idx := strings.IndexByte(env, '='); idx != -1 {
			if _, err := parsePlaceholders(env[idx+1:]); err != nil {
				return fmt.Errorf("template %q env[%d]: %w", name, i, err)
			}
		}
	}

	return nil
}
```

## 7. 使用例

### 7.1 基本的なテンプレート使用

```toml
# テンプレート定義
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]

# グループでの使用
[[groups]]
name = "daily_backup"

[[groups.commands]]
name = "backup_data"
template = "restic_backup"
params.path = "/data"
# 展開結果: cmd = "restic", args = ["backup", "/data"]
```

### 7.2 オプショナルパラメータ

```toml
[command_templates.restic_backup_with_opts]
cmd = "restic"
args = ["${?verbose}", "backup", "${path}"]

[[groups.commands]]
name = "backup_verbose"
template = "restic_backup_with_opts"
params.verbose = "-v"
params.path = "/data"
# 展開結果: args = ["-v", "backup", "/data"]

[[groups.commands]]
name = "backup_quiet"
template = "restic_backup_with_opts"
params.verbose = ""
params.path = "/data"
# 展開結果: args = ["backup", "/data"]
```

### 7.3 配列パラメータ

```toml
[command_templates.restic_full]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]

[[groups.commands]]
name = "backup_with_flags"
template = "restic_full"
params.flags = ["-v", "--no-cache"]
params.path = "/data"
# 展開結果: args = ["-v", "--no-cache", "backup", "/data"]

[[groups.commands]]
name = "backup_no_flags"
template = "restic_full"
params.flags = []
params.path = "/data"
# 展開結果: args = ["backup", "/data"]
```

### 7.4 %{var} との組み合わせ

```toml
[command_templates.restic_group_backup]
cmd = "restic"
args = ["backup", "${path}"]

[[groups]]
name = "production"

[groups.vars]
group_root = "/data/prod"

[[groups.commands]]
name = "backup_volumes"
template = "restic_group_backup"
params.path = "%{group_root}/volumes"
# Step 1 (template expansion): args = ["backup", "%{group_root}/volumes"]
# Step 2 (variable expansion): args = ["backup", "/data/prod/volumes"]
```

### 7.5 リテラル $ のエスケープ

```toml
[command_templates.echo_cost]
cmd = "echo"
args = ["The cost is \\$100 for ${item}"]

[[groups.commands]]
name = "show_cost"
template = "echo_cost"
params.item = "widget"
# 展開結果: args = ["The cost is $100 for widget"]

# 複数のエスケープの例
[command_templates.path_example]
cmd = "echo"
args = ["Path: C:\\\\Users\\\\${user}\\\\file.txt"]

[[groups.commands]]
name = "show_path"
template = "path_example"
params.user = "alice"
# 展開結果: args = ["Path: C:\\Users\\alice\\file.txt"]
```

## 8. テストケース一覧

### 8.1 正常系テスト

| テスト名 | 説明 | 入力 | 期待結果 |
|----------|------|------|----------|
| TestBasicStringParam | 基本的な文字列パラメータ | `${path}`, params.path = "/data" | `["/data"]` |
| TestOptionalWithValue | 値ありオプショナル | `${?flag}`, params.flag = "-v" | `["-v"]` |
| TestOptionalEmpty | 空オプショナル | `${?flag}`, params.flag = "" | `[]` |
| TestOptionalMissing | 未指定オプショナル | `${?flag}`, params = {} | `[]` |
| TestArrayExpansion | 配列展開 | `${@flags}`, params.flags = ["-a", "-b"] | `["-a", "-b"]` |
| TestArrayEmpty | 空配列 | `${@flags}`, params.flags = [] | `[]` |
| TestDollarEscape | $ エスケープ | `\$100` | `["$100"]` |
| TestBackslashEscape | \ エスケープ | `C:\\\\path` | `["C:\\path"]` |
| TestMixedPlaceholders | 混合 | `["${@a}", "${b}", "${?c}"]` | 各種組み合わせ |
| TestNoPlaceholders | プレースホルダーなし | `["backup", "/data"]` | `["backup", "/data"]` |

### 8.2 異常系テスト

| テスト名 | 説明 | 入力 | 期待エラー |
|----------|------|------|------------|
| TestRequiredMissing | 必須パラメータ未指定 | `${path}`, params = {} | ErrRequiredParamMissing |
| TestTypeMismatch | 型不一致 | `${path}`, params.path = [] | ErrTypeMismatch |
| TestUnclosedPlaceholder | 閉じ忘れ | `${path` | ErrUnclosedPlaceholder |
| TestArrayInMixed | 配列の混合コンテキスト | `"pre${@arr}post"` | ErrArrayInMixedContext |
| TestVarRefInTemplate | テンプレート定義で%{使用 | template.args = ["%{var}"] | ErrForbiddenPatternInTemplate |
| TestTemplateNotFound | 未定義テンプレート | template = "nonexistent" | ErrTemplateNotFound |
| TestTemplateFieldConflict | 排他違反 | template + cmd | ErrTemplateFieldConflict |
| TestReservedTemplateName | 予約名 | name = "__reserved" | ErrReservedTemplateName |
| TestVarRefInParams | params内で%{使用 | params.p = "%{var}" | 許可される（エラーなし） |

### 8.3 統合テスト

| テスト名 | 説明 |
|----------|------|
| TestTemplateWithVarExpansion | テンプレート展開 + %{var} 展開 |
| TestTemplateSecurityValidation | 展開後のセキュリティ検証 |
| TestTemplateCmdAllowedCheck | 展開後の cmd_allowed チェック |
| TestBackwardCompatibility | 既存設定の動作確認 |
| TestSampleConfigs | サンプル設定ファイルの動作確認 |

## 9. ファイル構成

```
internal/runner/
├── config/
│   ├── loader.go                    # 修正: テンプレート読み込み・検証追加
│   ├── template_expansion.go        # 新規: テンプレート展開ロジック
│   ├── template_expansion_test.go   # 新規: 展開ロジックのユニットテスト
│   ├── template_errors.go           # 新規: エラー型定義
│   ├── expansion.go                 # 修正: ExpandCommand にテンプレート統合
│   └── expansion_test.go            # 修正: テンプレート統合テスト追加
└── runnertypes/
    └── spec.go                      # 修正: CommandTemplate, CommandSpec.Template 追加
```
