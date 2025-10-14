# 詳細設計書: 内部変数とプロセス環境変数の分離

## 0. ドキュメント概要

本詳細設計書は、要件定義書（01_requirements.md）およびアーキテクチャ設計書（02_architecture.md）に基づき、内部変数とプロセス環境変数の分離機能の実装詳細を定義します。

**設計方針**:
- 既存の環境変数展開インフラ（`environment.Filter`, `environment.VariableExpander`）を最大限活用
- 新規構文 `%{VAR}` による内部変数参照の導入
- `from_env`, `vars` フィールドによる明示的な変数管理
- `env` フィールドはプロセス環境変数専用として再定義

## 1. データ構造設計

### 1.1 GlobalConfig の拡張

```go
// internal/runner/runnertypes/config.go

type GlobalConfig struct {
    // Existing fields
    Timeout           int      `toml:"timeout"`
    WorkDir           string   `toml:"workdir"`
    LogLevel          string   `toml:"log_level"`
    VerifyFiles       []string `toml:"verify_files"`
    SkipStandardPaths bool     `toml:"skip_standard_paths"`
    EnvAllowlist      []string `toml:"env_allowlist"`
    MaxOutputSize     int64    `toml:"max_output_size"`
    Env               []string `toml:"env"`

    // New fields for internal variable system
    FromEnv []string `toml:"from_env"` // System env var import: "internal_name=SYSTEM_VAR"
    Vars    []string `toml:"vars"`     // Internal variable definitions: "var_name=value"

    // Expanded results (populated during configuration loading)
    InternalVars        map[string]string `toml:"-"` // Expanded internal variables (from_env + vars)
    ExpandedVerifyFiles []string          `toml:"-"` // verify_files with %{VAR} expanded
    ExpandedEnv         map[string]string `toml:"-"` // Process environment variables (env field expanded)
}
```

**フィールドの役割**:

| フィールド | 型 | 用途 | 展開タイミング | 構文サポート |
|-----------|---|------|---------------|-------------|
| `FromEnv` | `[]string` | システム環境変数を内部変数として取り込み | 設定読み込み時 | `KEY=VALUE` |
| `Vars` | `[]string` | 内部変数の定義 | 設定読み込み時 | `KEY=VALUE`, `%{VAR}` 参照可能 |
| `Env` | `[]string` | プロセス環境変数の定義 | 設定読み込み時 | `KEY=VALUE`, `%{VAR}` 参照可能 |
| `InternalVars` | `map[string]string` | 展開済み内部変数（`FromEnv` + `Vars`） | 設定読み込み時に構築 | - |
| `ExpandedEnv` | `map[string]string` | 展開済みプロセス環境変数 | 設定読み込み時に構築 | - |

**処理順序**:
1. `FromEnv` を処理 → システム環境変数を `InternalVars` に追加
2. `Vars` を展開 → `%{VAR}` を解決し `InternalVars` に追加
3. `Env` を展開 → `%{VAR}` を解決し `ExpandedEnv` に追加
4. `VerifyFiles` を展開 → `%{VAR}` を解決し `ExpandedVerifyFiles` に追加

### 1.2 CommandGroup の拡張

```go
// internal/runner/runnertypes/config.go

type CommandGroup struct {
    // Existing fields
    Name         string    `toml:"name"`
    Description  string    `toml:"description"`
    Priority     int       `toml:"priority"`
    TempDir      bool      `toml:"temp_dir"`
    WorkDir      string    `toml:"workdir"`
    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`
    EnvAllowlist []string  `toml:"env_allowlist"`
    Env          []string  `toml:"env"`

    // New fields for internal variable system
    FromEnv []string `toml:"from_env"` // System env var import (with inheritance)
    Vars    []string `toml:"vars"`     // Group-level internal variables

    // Expanded results (populated during configuration loading)
    InternalVars        map[string]string `toml:"-"` // Inherited + group-specific internal variables
    ExpandedVerifyFiles []string          `toml:"-"` // verify_files with %{VAR} expanded
    ExpandedEnv         map[string]string `toml:"-"` // Process environment variables (env field expanded)
}
```

**継承ルール**:

| フィールド | 継承動作 | 説明 |
|-----------|---------|------|
| `FromEnv` | Override | Group.FromEnv が nil または定義されていない場合は Global.FromEnv を継承、定義されている場合は上書き（Global.FromEnv は無視） |
| `Vars` | Merge | Global.Vars と Group.Vars をマージ（Group.Vars が優先） |
| `InternalVars` | Merge | Global.InternalVars と Group.InternalVars をマージ |
| `EnvAllowlist` | Override | Task 0031 の仕様通り |
| `Env` | Merge | Global.Env と Group.Env をマージ（実行時にマージ） |

**FromEnv の継承詳細**:
```go
// Pseudo-code for FromEnv inheritance
if group.FromEnv == nil {
    // Not defined in TOML → inherit from global
    group.InternalVars = copyMap(global.InternalVars)
} else if len(group.FromEnv) == 0 {
    // Explicitly set to [] → no system env vars
    group.InternalVars = make(map[string]string)
} else {
    // Explicitly defined → override (global.FromEnv is ignored)
    group.InternalVars = processFromEnv(group.FromEnv, group.EnvAllowlist)
}
```

### 1.3 Command 構造体（変更なし）

```go
// internal/runner/runnertypes/config.go

type Command struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`
    Env         []string `toml:"env"`
    // ... other existing fields ...

    // Expanded results
    ExpandedCmd  string            `toml:"-"`
    ExpandedArgs []string          `toml:"-"`
    ExpandedEnv  map[string]string `toml:"-"`
}
```

**注記**: Command レベルでは `FromEnv` と `Vars` は追加しません（要件定義書で明示）。Command.Env で `%{VAR}` を参照可能です。

### 1.4 エラー型定義

```go
// internal/runner/config/errors.go

// ErrInvalidVariableName is returned when a variable name does not conform to POSIX naming rules.
type ErrInvalidVariableName struct {
    Level       string // "global", "group", "command"
    Field       string // "from_env", "vars", "env"
    VariableName string
    Reason      string
}

func (e *ErrInvalidVariableName) Error() string {
    return fmt.Sprintf("invalid variable name in %s.%s: '%s' (%s)",
        e.Level, e.Field, e.VariableName, e.Reason)
}

// ErrReservedVariableName is returned when a variable name starts with reserved prefix.
type ErrReservedVariableName struct {
    Level        string
    Field        string
    VariableName string
    Prefix       string
}

func (e *ErrReservedVariableName) Error() string {
    return fmt.Sprintf("reserved variable name in %s.%s: '%s' (prefix '%s' is reserved)",
        e.Level, e.Field, e.VariableName, e.Prefix)
}

// ErrVariableNotInAllowlist is returned when from_env references a system env var not in env_allowlist.
type ErrVariableNotInAllowlist struct {
    Level            string
    SystemVarName    string
    InternalVarName  string
    Allowlist        []string
}

func (e *ErrVariableNotInAllowlist) Error() string {
    return fmt.Sprintf("system environment variable '%s' (mapped to '%s' in %s.from_env) is not in env_allowlist: %v",
        e.SystemVarName, e.InternalVarName, e.Level, e.Allowlist)
}

// ErrCircularReference is returned when circular variable reference is detected.
type ErrCircularReference struct {
    Level        string
    Field        string
    VariableName string
    Chain        []string
}

func (e *ErrCircularReference) Error() string {
    return fmt.Sprintf("circular reference detected in %s.%s for variable '%s': %s",
        e.Level, e.Field, e.VariableName, strings.Join(e.Chain, " -> "))
}

// ErrUndefinedVariable is returned when %{VAR} references an undefined variable.
type ErrUndefinedVariable struct {
    Level        string
    Field        string
    VariableName string
    Context      string // The string being expanded
}

func (e *ErrUndefinedVariable) Error() string {
    return fmt.Sprintf("undefined variable '%s' in %s.%s: %s",
        e.VariableName, e.Level, e.Field, e.Context)
}

// ErrInvalidEscapeSequence is returned when an invalid escape sequence is found.
type ErrInvalidEscapeSequence struct {
    Level    string
    Field    string
    Sequence string
    Context  string
}

func (e *ErrInvalidEscapeSequence) Error() string {
    return fmt.Sprintf("invalid escape sequence '%s' in %s.%s: %s (only \\%% and \\\\ are supported)",
        e.Sequence, e.Level, e.Field, e.Context)
}
```

## 2. 変数展開エンジン

### 2.1 内部変数展開の統一インターフェース

```go
// internal/runner/config/expansion.go

// InternalVariableExpander handles expansion of internal variables (%{VAR}).
// It provides unified expansion logic for from_env, vars, env, cmd, args, and verify_files.
type InternalVariableExpander struct {
    logger *logging.Logger
}

// NewInternalVariableExpander creates a new internal variable expander.
func NewInternalVariableExpander(logger *logging.Logger) *InternalVariableExpander {
    return &InternalVariableExpander{
        logger: logger,
    }
}

// ExpandString expands %{VAR} references in a string using the provided internal variables.
// It detects circular references and reports detailed errors.
//
// Parameters:
//   - input: The string to expand
//   - internalVars: Map of available internal variables
//   - level: Configuration level ("global", "group", "command") for error reporting
//   - field: Field name ("vars", "env", "cmd", "args", "verify_files") for error reporting
//
// Returns:
//   - Expanded string
//   - Error if expansion fails (circular reference, undefined variable, invalid escape)
func (e *InternalVariableExpander) ExpandString(
    input string,
    internalVars map[string]string,
    level string,
    field string,
) (string, error) {
    visited := make(map[string]bool)
    return e.expandStringRecursive(input, internalVars, level, field, visited, nil)
}

// expandStringRecursive performs recursive expansion with circular reference detection.
func (e *InternalVariableExpander) expandStringRecursive(
    input string,
    internalVars map[string]string,
    level string,
    field string,
    visited map[string]bool,
    expansionChain []string,
) (string, error) {
    var result strings.Builder
    i := 0

    for i < len(input) {
        // Handle escape sequences
        if input[i] == '\\' && i+1 < len(input) {
            next := input[i+1]
            switch next {
            case '%':
                result.WriteByte('%')
                i += 2
                continue
            case '\\':
                result.WriteByte('\\')
                i += 2
                continue
            default:
                // Invalid escape sequence
                return "", &ErrInvalidEscapeSequence{
                    Level:    level,
                    Field:    field,
                    Sequence: string([]byte{input[i], next}),
                    Context:  input,
                }
            }
        }

        // Handle %{VAR} expansion
        if input[i] == '%' && i+1 < len(input) && input[i+1] == '{' {
            // Find the closing '}'
            closeIdx := strings.IndexByte(input[i+2:], '}')
            if closeIdx == -1 {
                // Unclosed %{
                return "", fmt.Errorf("unclosed %%{ in %s.%s: %s", level, field, input)
            }
            closeIdx += i + 2 // Adjust to absolute position

            varName := input[i+2 : closeIdx]

            // Validate variable name
            if err := validateVariableName(varName); err != nil {
                return "", &ErrInvalidVariableName{
                    Level:        level,
                    Field:        field,
                    VariableName: varName,
                    Reason:       err.Error(),
                }
            }

            // Check for circular reference
            if visited[varName] {
                chain := append(expansionChain, varName)
                return "", &ErrCircularReference{
                    Level:        level,
                    Field:        field,
                    VariableName: varName,
                    Chain:        chain,
                }
            }

            // Lookup variable
            value, ok := internalVars[varName]
            if !ok {
                return "", &ErrUndefinedVariable{
                    Level:        level,
                    Field:        field,
                    VariableName: varName,
                    Context:      input,
                }
            }

            // Recursively expand the value
            visited[varName] = true
            chain := append(expansionChain, varName)
            expandedValue, err := e.expandStringRecursive(value, internalVars, level, field, visited, chain)
            delete(visited, varName)

            if err != nil {
                return "", err
            }

            result.WriteString(expandedValue)
            i = closeIdx + 1
            continue
        }

        // Regular character
        result.WriteByte(input[i])
        i++
    }

    return result.String(), nil
}

// validateVariableName validates a variable name according to POSIX rules.
// Returns an error if the name is invalid.
func validateVariableName(name string) error {
    if len(name) == 0 {
        return fmt.Errorf("variable name cannot be empty")
    }

    // Check reserved prefix
    if strings.HasPrefix(name, "__runner_") {
        return fmt.Errorf("variable name cannot start with reserved prefix '__runner_'")
    }

    // First character must be letter or underscore
    first := name[0]
    if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
        return fmt.Errorf("variable name must start with letter or underscore")
    }

    // Remaining characters must be letter, digit, or underscore
    for i := 1; i < len(name); i++ {
        c := name[i]
        if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
            return fmt.Errorf("variable name can only contain letters, digits, and underscores")
        }
    }

    return nil
}
```

### 2.2 from_env 処理

```go
// internal/runner/config/expansion.go

// ProcessFromEnv processes from_env field and populates internal variables with system environment variables.
//
// Parameters:
//   - fromEnv: Array of "internal_name=SYSTEM_VAR" mappings
//   - envAllowlist: Allowed system environment variable names
//   - systemEnv: System environment variables (obtained from environment.Filter.ParseSystemEnvironment)
//   - level: Configuration level for error reporting
//
// Returns:
//   - Map of internal variables (internal_name -> value)
//   - Error if processing fails
func (e *InternalVariableExpander) ProcessFromEnv(
    fromEnv []string,
    envAllowlist []string,
    systemEnv map[string]string,
    level string,
) (map[string]string, error) {
    result := make(map[string]string)
    allowlistMap := make(map[string]bool)

    // Build allowlist map for fast lookup
    for _, name := range envAllowlist {
        allowlistMap[name] = true
    }

    for _, mapping := range fromEnv {
        // Parse "internal_name=SYSTEM_VAR"
        parts := strings.SplitN(mapping, "=", 2)
        if len(parts) != 2 {
            return nil, fmt.Errorf("invalid from_env mapping in %s: '%s' (expected 'internal_name=SYSTEM_VAR')", level, mapping)
        }

        internalName := parts[0]
        systemVarName := parts[1]

        // Validate internal variable name
        if err := validateVariableName(internalName); err != nil {
            return nil, &ErrInvalidVariableName{
                Level:        level,
                Field:        "from_env",
                VariableName: internalName,
                Reason:       err.Error(),
            }
        }

        // Check reserved prefix
        if strings.HasPrefix(internalName, "__runner_") {
            return nil, &ErrReservedVariableName{
                Level:        level,
                Field:        "from_env",
                VariableName: internalName,
                Prefix:       "__runner_",
            }
        }

        // Validate system variable name
        if err := validateVariableName(systemVarName); err != nil {
            return nil, &ErrInvalidVariableName{
                Level:        level,
                Field:        "from_env",
                VariableName: systemVarName,
                Reason:       "system variable name: " + err.Error(),
            }
        }

        // Check if system variable is in allowlist
        if !allowlistMap[systemVarName] {
            return nil, &ErrVariableNotInAllowlist{
                Level:           level,
                SystemVarName:   systemVarName,
                InternalVarName: internalName,
                Allowlist:       envAllowlist,
            }
        }

        // Get system environment variable value
        value, ok := systemEnv[systemVarName]
        if !ok {
            // System variable not set → store empty string
            value = ""
            e.logger.Warnf("System environment variable '%s' (mapped to '%s' in %s.from_env) is not set",
                systemVarName, internalName, level)
        }

        result[internalName] = value
    }

    return result, nil
}
```

### 2.3 vars 処理

```go
// internal/runner/config/expansion.go

// ProcessVars processes vars field and expands internal variable definitions.
//
// Parameters:
//   - vars: Array of "var_name=value" definitions (value can contain %{VAR} references)
//   - baseInternalVars: Base internal variables (from from_env or parent level)
//   - level: Configuration level for error reporting
//
// Returns:
//   - Map of expanded internal variables (merged with baseInternalVars)
//   - Error if processing fails
func (e *InternalVariableExpander) ProcessVars(
    vars []string,
    baseInternalVars map[string]string,
    level string,
) (map[string]string, error) {
    // Start with base internal variables
    result := make(map[string]string)
    for k, v := range baseInternalVars {
        result[k] = v
    }

    // Process each var definition
    for _, varDef := range vars {
        // Parse "var_name=value"
        parts := strings.SplitN(varDef, "=", 2)
        if len(parts) != 2 {
            return nil, fmt.Errorf("invalid vars definition in %s: '%s' (expected 'var_name=value')", level, varDef)
        }

        varName := parts[0]
        varValue := parts[1]

        // Validate variable name
        if err := validateVariableName(varName); err != nil {
            return nil, &ErrInvalidVariableName{
                Level:        level,
                Field:        "vars",
                VariableName: varName,
                Reason:       err.Error(),
            }
        }

        // Check reserved prefix
        if strings.HasPrefix(varName, "__runner_") {
            return nil, &ErrReservedVariableName{
                Level:        level,
                Field:        "vars",
                VariableName: varName,
                Prefix:       "__runner_",
            }
        }

        // Expand %{VAR} references in the value
        expandedValue, err := e.ExpandString(varValue, result, level, "vars")
        if err != nil {
            return nil, err
        }

        // Store expanded value
        result[varName] = expandedValue
    }

    return result, nil
}
```

### 2.4 env 処理

```go
// internal/runner/config/expansion.go

// ProcessEnv processes env field and expands process environment variable definitions.
//
// Parameters:
//   - env: Array of "VAR=value" definitions (value can contain %{VAR} references)
//   - internalVars: Available internal variables
//   - level: Configuration level for error reporting
//
// Returns:
//   - Map of expanded environment variables
//   - Error if processing fails
//
// Note: env field can only reference internal variables (%{VAR}), not other env variables.
func (e *InternalVariableExpander) ProcessEnv(
    env []string,
    internalVars map[string]string,
    level string,
) (map[string]string, error) {
    result := make(map[string]string)

    for _, envDef := range env {
        // Parse "VAR=value"
        parts := strings.SplitN(envDef, "=", 2)
        if len(parts) != 2 {
            return nil, fmt.Errorf("invalid env definition in %s: '%s' (expected 'VAR=value')", level, envDef)
        }

        envVarName := parts[0]
        envVarValue := parts[1]

        // Validate environment variable name (POSIX)
        if err := validateVariableName(envVarName); err != nil {
            return nil, &ErrInvalidVariableName{
                Level:        level,
                Field:        "env",
                VariableName: envVarName,
                Reason:       err.Error(),
            }
        }

        // Expand %{VAR} references in the value
        expandedValue, err := e.ExpandString(envVarValue, internalVars, level, "env")
        if err != nil {
            return nil, err
        }

        // Store expanded value
        result[envVarName] = expandedValue
    }

    return result, nil
}
```

## 3. 設定読み込みフロー

### 3.1 Global 設定の処理

```go
// internal/runner/config/loader.go

// expandGlobalConfig expands variables in global configuration.
//
// Processing order:
//   1. Process from_env → populate InternalVars with system environment variables
//   2. Process vars → expand and add to InternalVars
//   3. Process env → expand using InternalVars and populate ExpandedEnv
//   4. Expand verify_files → expand using InternalVars and populate ExpandedVerifyFiles
//
// This function modifies the global config in place.
func expandGlobalConfig(
    global *runnertypes.GlobalConfig,
    filter *environment.Filter,
    expander *InternalVariableExpander,
) error {
    // Step 1: Get system environment variables
    systemEnv := filter.ParseSystemEnvironment(nil)

    // Step 2: Process from_env
    fromEnvVars, err := expander.ProcessFromEnv(
        global.FromEnv,
        global.EnvAllowlist,
        systemEnv,
        "global",
    )
    if err != nil {
        return fmt.Errorf("failed to process global.from_env: %w", err)
    }

    // Step 3: Process vars (can reference from_env variables)
    internalVars, err := expander.ProcessVars(
        global.Vars,
        fromEnvVars,
        "global",
    )
    if err != nil {
        return fmt.Errorf("failed to process global.vars: %w", err)
    }
    global.InternalVars = internalVars

    // Step 4: Process env (can reference internal variables)
    expandedEnv, err := expander.ProcessEnv(
        global.Env,
        internalVars,
        "global",
    )
    if err != nil {
        return fmt.Errorf("failed to process global.env: %w", err)
    }
    global.ExpandedEnv = expandedEnv

    // Step 5: Expand verify_files
    expandedVerifyFiles := make([]string, len(global.VerifyFiles))
    for i, path := range global.VerifyFiles {
        expandedPath, err := expander.ExpandString(path, internalVars, "global", "verify_files")
        if err != nil {
            return fmt.Errorf("failed to expand global.verify_files[%d]: %w", i, err)
        }
        expandedVerifyFiles[i] = expandedPath
    }
    global.ExpandedVerifyFiles = expandedVerifyFiles

    return nil
}
```

### 3.2 Group 設定の処理

```go
// internal/runner/config/loader.go

// expandGroupConfig expands variables in group configuration.
//
// Processing order:
//   1. Determine from_env inheritance (override or inherit from global)
//   2. Process vars → expand and merge with inherited internal variables
//   3. Process env → expand using merged internal variables
//   4. Expand verify_files
//
// This function modifies the group config in place.
func expandGroupConfig(
    group *runnertypes.CommandGroup,
    global *runnertypes.GlobalConfig,
    filter *environment.Filter,
    expander *InternalVariableExpander,
) error {
    var baseInternalVars map[string]string

    // Step 1: Determine from_env inheritance
    if group.FromEnv == nil {
        // Not defined in TOML → inherit from global
        baseInternalVars = copyMap(global.InternalVars)
    } else if len(group.FromEnv) == 0 {
        // Explicitly set to [] → no system env vars
        baseInternalVars = make(map[string]string)
    } else {
        // Explicitly defined → override (global.FromEnv is ignored)
        systemEnv := filter.ParseSystemEnvironment(nil)

        // Resolve allowlist (may inherit from global)
        groupAllowlist := group.EnvAllowlist
        if groupAllowlist == nil {
            groupAllowlist = global.EnvAllowlist
        }

        fromEnvVars, err := expander.ProcessFromEnv(
            group.FromEnv,
            groupAllowlist,
            systemEnv,
            fmt.Sprintf("group[%s]", group.Name),
        )
        if err != nil {
            return fmt.Errorf("failed to process group[%s].from_env: %w", group.Name, err)
        }
        baseInternalVars = fromEnvVars
    }

    // Step 2: Process vars (merge with base internal variables)
    internalVars, err := expander.ProcessVars(
        group.Vars,
        baseInternalVars,
        fmt.Sprintf("group[%s]", group.Name),
    )
    if err != nil {
        return fmt.Errorf("failed to process group[%s].vars: %w", group.Name, err)
    }
    group.InternalVars = internalVars

    // Step 3: Process env
    expandedEnv, err := expander.ProcessEnv(
        group.Env,
        internalVars,
        fmt.Sprintf("group[%s]", group.Name),
    )
    if err != nil {
        return fmt.Errorf("failed to process group[%s].env: %w", group.Name, err)
    }
    group.ExpandedEnv = expandedEnv

    // Step 4: Expand verify_files
    expandedVerifyFiles := make([]string, len(group.VerifyFiles))
    for i, path := range group.VerifyFiles {
        expandedPath, err := expander.ExpandString(path, internalVars, fmt.Sprintf("group[%s]", group.Name), "verify_files")
        if err != nil {
            return fmt.Errorf("failed to expand group[%s].verify_files[%d]: %w", group.Name, i, err)
        }
        expandedVerifyFiles[i] = expandedPath
    }
    group.ExpandedVerifyFiles = expandedVerifyFiles

    return nil
}

// copyMap creates a shallow copy of a string map.
func copyMap(m map[string]string) map[string]string {
    result := make(map[string]string, len(m))
    for k, v := range m {
        result[k] = v
    }
    return result
}
```

### 3.3 Command 設定の処理

```go
// internal/runner/config/loader.go

// expandCommandConfig expands variables in command configuration.
//
// Processing order:
//   1. Inherit internal variables from group
//   2. Process env → expand using inherited internal variables
//   3. Expand cmd and args
//
// This function modifies the command config in place.
func expandCommandConfig(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
    expander *InternalVariableExpander,
) error {
    level := fmt.Sprintf("group[%s].command[%s]", group.Name, cmd.Name)

    // Step 1: Inherit internal variables from group
    internalVars := group.InternalVars

    // Step 2: Process env
    expandedEnv, err := expander.ProcessEnv(
        cmd.Env,
        internalVars,
        level,
    )
    if err != nil {
        return fmt.Errorf("failed to process command[%s].env: %w", cmd.Name, err)
    }
    cmd.ExpandedEnv = expandedEnv

    // Step 3: Expand cmd
    expandedCmd, err := expander.ExpandString(cmd.Cmd, internalVars, level, "cmd")
    if err != nil {
        return fmt.Errorf("failed to expand command[%s].cmd: %w", cmd.Name, err)
    }
    cmd.ExpandedCmd = expandedCmd

    // Step 4: Expand args
    expandedArgs := make([]string, len(cmd.Args))
    for i, arg := range cmd.Args {
        expandedArg, err := expander.ExpandString(arg, internalVars, level, "args")
        if err != nil {
            return fmt.Errorf("failed to expand command[%s].args[%d]: %w", cmd.Name, i, err)
        }
        expandedArgs[i] = expandedArg
    }
    cmd.ExpandedArgs = expandedArgs

    return nil
}
```

## 4. 実行時環境変数の構築

### 4.1 環境変数マージ処理

```go
// internal/runner/executor/environment.go

// BuildProcessEnvironment builds the final process environment variables for command execution.
//
// Merge order (lower priority to higher priority):
//   1. System environment variables (filtered by env_allowlist)
//   2. Global.ExpandedEnv
//   3. Group.ExpandedEnv
//   4. Command.ExpandedEnv
//
// Returns:
//   - Map of environment variables to be passed to the child process
func BuildProcessEnvironment(
    global *runnertypes.GlobalConfig,
    group *runnertypes.CommandGroup,
    cmd *runnertypes.Command,
    filter *environment.Filter,
) (map[string]string, error) {
    result := make(map[string]string)

    // Step 1: Get system environment variables (filtered by allowlist)
    systemEnv := filter.ParseSystemEnvironment(nil)
    allowlist := resolveAllowlist(global, group)

    for _, name := range allowlist {
        if value, ok := systemEnv[name]; ok {
            result[name] = value
        }
    }

    // Step 2: Merge Global.ExpandedEnv (overrides system env)
    for k, v := range global.ExpandedEnv {
        result[k] = v
    }

    // Step 3: Merge Group.ExpandedEnv (overrides global env)
    if group != nil {
        for k, v := range group.ExpandedEnv {
            result[k] = v
        }
    }

    // Step 4: Merge Command.ExpandedEnv (overrides group env)
    for k, v := range cmd.ExpandedEnv {
        result[k] = v
    }

    return result, nil
}

// resolveAllowlist determines the effective allowlist for a command.
func resolveAllowlist(global *runnertypes.GlobalConfig, group *runnertypes.CommandGroup) []string {
    if group != nil && group.EnvAllowlist != nil {
        return group.EnvAllowlist
    }
    return global.EnvAllowlist
}
```

## 5. デバッグ・診断機能

### 5.1 Dry-Run での変数展開トレース

```go
// internal/runner/debug/trace.go

// VariableExpansionTrace records the variable expansion process for debugging.
type VariableExpansionTrace struct {
    Level           string            // "global", "group[name]", "command[name]"
    Phase           string            // "from_env", "vars", "env", "cmd", "args", "verify_files"
    Input           string            // Original input string
    Output          string            // Expanded output string
    ReferencedVars  []string          // Variables referenced during expansion
    InternalVars    map[string]string // Available internal variables at this point
    Errors          []error           // Errors encountered during expansion
}

// TraceExpansion records a variable expansion step.
func (t *VariableExpansionTrace) TraceExpansion(
    level string,
    phase string,
    input string,
    output string,
    referencedVars []string,
    internalVars map[string]string,
    err error,
) {
    // Implementation details...
}

// PrintTrace outputs the expansion trace in a human-readable format.
func (t *VariableExpansionTrace) PrintTrace(w io.Writer) {
    fmt.Fprintf(w, "\n=== Variable Expansion Trace ===\n")
    fmt.Fprintf(w, "Level: %s\n", t.Level)
    fmt.Fprintf(w, "Phase: %s\n", t.Phase)
    fmt.Fprintf(w, "Input: %s\n", t.Input)
    fmt.Fprintf(w, "Output: %s\n", t.Output)

    if len(t.ReferencedVars) > 0 {
        fmt.Fprintf(w, "Referenced Variables:\n")
        for _, varName := range t.ReferencedVars {
            if value, ok := t.InternalVars[varName]; ok {
                fmt.Fprintf(w, "  %s = %s\n", varName, value)
            } else {
                fmt.Fprintf(w, "  %s = (undefined)\n", varName)
            }
        }
    }

    if len(t.Errors) > 0 {
        fmt.Fprintf(w, "Errors:\n")
        for _, err := range t.Errors {
            fmt.Fprintf(w, "  - %v\n", err)
        }
    }

    fmt.Fprintf(w, "\n")
}
```

### 5.2 最終環境変数の表示

```go
// internal/runner/debug/environment.go

// PrintFinalEnvironment outputs the final environment variables for debugging.
func PrintFinalEnvironment(
    w io.Writer,
    envVars map[string]string,
    global *runnertypes.GlobalConfig,
    group *runnertypes.CommandGroup,
    cmd *runnertypes.Command,
) {
    fmt.Fprintf(w, "\n=== Final Environment Variables ===\n")
    fmt.Fprintf(w, "Command: %s (Group: %s)\n\n", cmd.Name, group.Name)

    // Sort keys for consistent output
    keys := make([]string, 0, len(envVars))
    for k := range envVars {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    for _, key := range keys {
        value := envVars[key]
        source := determineVariableSource(key, value, global, group, cmd)
        fmt.Fprintf(w, "%s=%s (source: %s)\n", key, value, source)
    }

    fmt.Fprintf(w, "\n")
}

// determineVariableSource identifies where an environment variable came from.
func determineVariableSource(
    key string,
    value string,
    global *runnertypes.GlobalConfig,
    group *runnertypes.CommandGroup,
    cmd *runnertypes.Command,
) string {
    // Check Command.ExpandedEnv
    if cmdValue, ok := cmd.ExpandedEnv[key]; ok && cmdValue == value {
        return "command.env"
    }

    // Check Group.ExpandedEnv
    if group != nil {
        if groupValue, ok := group.ExpandedEnv[key]; ok && groupValue == value {
            return "group.env"
        }
    }

    // Check Global.ExpandedEnv
    if globalValue, ok := global.ExpandedEnv[key]; ok && globalValue == value {
        return "global.env"
    }

    // Must be from system environment (filtered by allowlist)
    return "system (allowlist)"
}
```

### 5.3 from_env 継承状態の表示

```go
// internal/runner/debug/inheritance.go

// PrintFromEnvInheritance outputs the from_env inheritance status for debugging.
func PrintFromEnvInheritance(
    w io.Writer,
    global *runnertypes.GlobalConfig,
    group *runnertypes.CommandGroup,
) {
    fmt.Fprintf(w, "\n=== from_env Inheritance Status ===\n")

    // Global from_env
    fmt.Fprintf(w, "Global.from_env: %d variables\n", len(global.InternalVars))
    if len(global.FromEnv) > 0 {
        for _, mapping := range global.FromEnv {
            parts := strings.SplitN(mapping, "=", 2)
            if len(parts) == 2 {
                internalName := parts[0]
                systemName := parts[1]
                value := global.InternalVars[internalName]
                fmt.Fprintf(w, "  %s = %s (from %s)\n", internalName, value, systemName)
            }
        }
    }
    fmt.Fprintf(w, "\n")

    // Group from_env
    if group == nil {
        return
    }

    fmt.Fprintf(w, "Group[%s].from_env: ", group.Name)

    if group.FromEnv == nil {
        // Inherited from global
        fmt.Fprintf(w, "INHERITED from global (%d variables)\n", len(group.InternalVars))
        for k, v := range group.InternalVars {
            if _, isGlobal := global.InternalVars[k]; isGlobal {
                fmt.Fprintf(w, "  %s = %s (inherited)\n", k, v)
            }
        }
    } else if len(group.FromEnv) == 0 {
        // Explicitly empty
        fmt.Fprintf(w, "EMPTY (no system env vars imported)\n")
    } else {
        // Overridden
        fmt.Fprintf(w, "OVERRIDDEN (%d variables, global discarded)\n", len(group.FromEnv))
        for _, mapping := range group.FromEnv {
            parts := strings.SplitN(mapping, "=", 2)
            if len(parts) == 2 {
                internalName := parts[0]
                systemName := parts[1]
                value := group.InternalVars[internalName]
                fmt.Fprintf(w, "  %s = %s (from %s)\n", internalName, value, systemName)
            }
        }

        // Warn about discarded global variables
        if len(global.FromEnv) > 0 {
            fmt.Fprintf(w, "  ⚠ Note: Global.from_env variables are NOT available:\n")
            for k := range global.InternalVars {
                if _, exists := group.InternalVars[k]; !exists {
                    fmt.Fprintf(w, "    - %s (discarded)\n", k)
                }
            }
        }
    }

    fmt.Fprintf(w, "\n")
}
```

## 6. セキュリティ機能

### 6.1 変数名バリデーション

変数名のバリデーションは `validateVariableName` 関数で実装済み（セクション 2.1 参照）。

**検証項目**:
- POSIX 準拠: `[a-zA-Z_][a-zA-Z0-9_]*`
- 予約プレフィックスの禁止: `__runner_` で始まる名前は使用不可
- 空文字列のチェック

### 6.2 循環参照検出

循環参照の検出は `expandStringRecursive` 関数で実装済み（セクション 2.1 参照）。

**検出方法**:
- `visited` マップで現在展開中の変数を追跡
- 同じ変数が2回目に参照された場合、循環参照エラーを返す
- エラーメッセージに展開チェーンを含める（例: `a -> b -> c -> a`）

### 6.3 エスケープ処理

エスケープシーケンスの処理は `expandStringRecursive` 関数で実装済み（セクション 2.1 参照）。

**サポートするエスケープ**:
- `\%` → `%` （`%{VAR}` の展開を抑止）
- `\\` → `\` （バックスラッシュのリテラル）

**エラー処理**:
- その他の `\x` 形式はエラー（`ErrInvalidEscapeSequence`）

### 6.4 allowlist 制御

allowlist の制御は `ProcessFromEnv` 関数で実装済み（セクション 2.2 参照）。

**制御内容**:
- `from_env` で参照するシステム環境変数は `env_allowlist` に含まれている必要がある
- 含まれていない場合は `ErrVariableNotInAllowlist` エラーを返す
- `env_allowlist` の継承は既存の仕様通り（Group で定義されていない場合は Global を継承）

## 7. テスト戦略

### 7.1 単体テスト

**テスト対象**:
- `validateVariableName`: 変数名のバリデーション
- `ExpandString`: 変数展開の基本機能
- `ProcessFromEnv`: from_env 処理
- `ProcessVars`: vars 処理
- `ProcessEnv`: env 処理

**テストケース**:
1. **正常系**:
   - 単純な変数展開
   - 複数レベルの変数参照
   - エスケープシーケンス
   - 空の値、空の配列

2. **異常系**:
   - 未定義変数の参照
   - 循環参照
   - 不正な変数名
   - 予約プレフィックス
   - 不正なエスケープシーケンス
   - allowlist 違反
   - 不正な KEY=VALUE 形式

### 7.2 統合テスト

**テストシナリオ**:
1. **基本的な変数展開**:
   - Global → Group → Command の階層的展開
   - from_env による環境変数取り込み
   - vars による変数定義
   - env による環境変数設定

2. **継承テスト**:
   - from_env の継承（nil の場合）
   - from_env の上書き（定義されている場合）
   - from_env の空配列（[] の場合）
   - vars のマージ

3. **PATH 拡張テスト**:
   - 段階的な PATH 拡張（アーキテクチャ設計書のシナリオ）

4. **セキュリティテスト**:
   - allowlist 制御
   - 循環参照の検出
   - 不正な変数名の検出

### 7.3 E2E テスト

**テストケース**:
1. **実際の TOML ファイルでのテスト**:
   - `sample/` ディレクトリに新しいサンプルファイルを追加
   - runner 実行時の動作確認
   - dry-run での変数展開トレースの確認

2. **エラーハンドリング**:
   - 不正な設定ファイルでのエラーメッセージ確認
   - エラー発生時の詳細情報の確認

## 8. 性能考慮事項

### 8.1 展開処理の最適化

**最適化ポイント**:
1. **一回限りの展開**: 設定読み込み時に一度だけ展開し、結果をキャッシュ
2. **visited マップの再利用**: 循環参照検出のための visited マップはスタック上で管理
3. **strings.Builder の使用**: 文字列結合は `strings.Builder` で効率化

**性能目標**:
- 変数1個あたりの展開時間: 1ms 以下
- 設定読み込み時間の増加: 既存比 +10% 以内

### 8.2 メモリ効率

**メモリ管理**:
1. **展開済み変数のキャッシュ**: `InternalVars`, `ExpandedEnv` で結果を保持
2. **不要なコピーの回避**: マップの shallow copy のみ
3. **スタック上のデータ構造**: visited マップは関数呼び出しスタック上で管理

**メモリ目標**:
- メモリ使用量の増加: 変数定義総量の2倍以内

## 9. 実装計画

### 9.1 実装フェーズ

**Phase 1: コア機能**（2-3日）
- データ構造の拡張（GlobalConfig, CommandGroup）
- InternalVariableExpander の実装
- validateVariableName の実装
- ExpandString の実装

**Phase 2: 処理ロジック**（3-4日）
- ProcessFromEnv の実装
- ProcessVars の実装
- ProcessEnv の実装
- 設定読み込みフローの実装（expandGlobalConfig, expandGroupConfig, expandCommandConfig）

**Phase 3: 実行時機能**（2日）
- BuildProcessEnvironment の実装
- 既存の executor コードとの統合

**Phase 4: デバッグ機能**（2日）
- 変数展開トレースの実装
- dry-run での診断情報出力

**Phase 5: テスト**（3-4日）
- 単体テストの作成
- 統合テストの作成
- E2E テストの作成

**Phase 6: ドキュメント**（2日）
- サンプルファイルの作成
- ユーザー向けドキュメントの更新
- 開発者向けドキュメントの更新

### 9.2 リスク管理

**リスク項目**:
1. **既存機能への影響**: 既存の環境変数展開機能との互換性
   - 対策: 既存の `${VAR}` 構文はリテラル文字列として処理（展開しない）
   - 検証: 既存のテストが pass することを確認

2. **性能劣化**: 変数展開処理の追加による性能への影響
   - 対策: 一回限りの展開、効率的なデータ構造の使用
   - 検証: ベンチマークテストの実施

3. **複雑性の増加**: 継承ルールの複雑化
   - 対策: 明確なドキュメント、充実したデバッグ機能
   - 検証: ユーザビリティテストの実施

## 10. 既存コードへの影響

### 10.1 変更が必要なファイル

| ファイル | 変更内容 | 影響度 |
|---------|---------|--------|
| `internal/runner/runnertypes/config.go` | GlobalConfig, CommandGroup の拡張 | 中 |
| `internal/runner/config/loader.go` | 設定読み込みフローの追加 | 高 |
| `internal/runner/config/expansion.go` | InternalVariableExpander の追加 | 高（新規） |
| `internal/runner/config/errors.go` | エラー型の追加 | 低 |
| `internal/runner/executor/environment.go` | BuildProcessEnvironment の追加 | 中 |
| `internal/runner/executor/executor.go` | 環境変数構築ロジックの更新 | 中 |
| `internal/runner/debug/trace.go` | デバッグ機能の追加 | 低（新規） |

### 10.2 後方互換性

**互換性の維持**:
1. **既存の `env` フィールド**: 引き続き動作（`%{VAR}` 構文をサポート）
2. **既存の `${VAR}` 構文**: リテラル文字列として処理（展開しない）
3. **既存の `env_allowlist`**: 引き続き動作（継承ルールは変更なし）

**非互換な変更**:
- なし（既存機能は保持）

### 10.3 移行パス

**既存ユーザーへの影響**:
- `from_env`, `vars` フィールドは任意（定義しなくても既存通り動作）
- `%{VAR}` 構文は新規追加（既存の `${VAR}` には影響なし）

**推奨される移行手順**:
1. 既存の設定ファイルはそのまま動作（変更不要）
2. 新しい機能を使用する場合は、段階的に `from_env`, `vars` を追加
3. セキュリティ向上のため、システム環境変数の直接参照を `from_env` 経由に移行（任意）

## 11. まとめ

本詳細設計書は、内部変数とプロセス環境変数の分離機能の完全な実装仕様を定義しました。

**主要な設計決定**:
1. **新規構文 `%{VAR}`**: 内部変数の参照に使用
2. **from_env フィールド**: システム環境変数を内部変数として取り込み
3. **vars フィールド**: 内部変数の定義（他の内部変数を参照可能）
4. **env フィールド**: プロセス環境変数の定義（内部変数を参照可能）
5. **Override 継承方式**: Group.from_env が定義されている場合、Global.from_env を無視

**セキュリティ強化**:
- allowlist による厳格なアクセス制御
- 循環参照の検出
- 変数名のバリデーション
- エスケープシーケンスのサポート

**デバッグ支援**:
- dry-run での変数展開トレース
- 最終環境変数の表示
- from_env 継承状態の可視化

本設計により、セキュアで保守性の高い変数管理システムを実現します。

### 2.2 実行時の処理フロー

```
1. 子プロセス環境変数の構築
   - システム環境変数（env_allowlist でフィルタリング）
   - Global.ExpandedEnv
   - Group.ExpandedEnv
   - Command.ExpandedEnv
   - 上記を優先順位に従ってマージ
   ↓
2. 子プロセスの起動
   - Command.ExpandedCmd
   - Command.ExpandedArgs
   - 構築した環境変数
```

## 3. アルゴリズム設計

### 3.1 循環参照検出アルゴリズム

**重要**: 処理順序（from_env → vars → env）と階層構造（Global → Group → Command）により、各段階で参照するのは既に確定した値です。そのため、自己参照のための特別な処理は不要です。

**循環参照検出が必要なケース**:
- 同一レベル内での変数間の循環参照（例: vars 内で A → B → A）
- 同一変数名での完全な自己参照（例: `A=%{A}`）

**循環参照検出が不要なケース**:
- from_env と vars の間の参照（処理順序により既に確定）
- Global.vars と Group.vars の間の参照（階層構造により一方向のみ）
- 異なる名前の変数による値の拡張（例: `from_env = ["path=PATH"]` → `vars = ["path=/custom:%{path}"]`）

```go
// 同一レベル内での循環参照のみを検出
func detectCircularReference(vars map[string]string, visited map[string]bool,
                           current string, path []string) error {
    if visited[current] {
        return fmt.Errorf("circular reference detected: %s -> %s",
                         strings.Join(path, " -> "), current)
    }

    visited[current] = true

    // 変数値内の %{VAR} 参照を解析
    refs := extractVariableReferences(vars[current])
    for _, ref := range refs {
        // 同一マップ内の変数のみチェック
        if _, exists := vars[ref]; exists {
            if err := detectCircularReference(vars, visited, ref,
                                            append(path, current)); err != nil {
                return err
            }
        }
    }

    delete(visited, current)
    return nil
}
```

### 3.2 変数展開アルゴリズム

**処理方針**:
1. 処理順序に従って段階的に展開（from_env → vars → env）
2. 各段階で参照できるのは既に確定した変数のみ
3. 循環参照チェックは同一マップ内のみ

```go
func expandVariable(value string, availableVars map[string]string,
                   visited map[string]bool, maxDepth int) (string, error) {
    if maxDepth <= 0 {
        return "", fmt.Errorf("maximum expansion depth exceeded")
    }

    result := value
    iterations := 0
    maxIterations := 15 // 無限ループ防止

    for iterations < maxIterations {
        refs := extractVariableReferences(result)
        if len(refs) == 0 {
            break // 展開完了
        }

        changed := false
        for _, ref := range refs {
            // 循環参照チェック（visited マップ）
            if visited[ref] {
                return "", fmt.Errorf("circular reference: %s", ref)
            }

            // 利用可能な変数から値を取得
            if varValue, exists := availableVars[ref]; exists {
                visited[ref] = true
                expanded, err := expandVariable(varValue, availableVars, visited, maxDepth-1)
                delete(visited, ref)

                if err != nil {
                    return "", err
                }

                result = strings.ReplaceAll(result, "%{"+ref+"}", expanded)
                changed = true
            } else {
                return "", fmt.Errorf("undefined variable: %s", ref)
            }
        }

        if !changed {
            break
        }
        iterations++
    }

    if iterations >= maxIterations {
        return "", fmt.Errorf("maximum iterations exceeded (possible infinite loop)")
    }

    return result, nil
}
```

**availableVars の構成**:
- **Global.vars 展開時**: Global.from_env の結果のみ
- **Group.vars 展開時**: Global.from_env（または Group.from_env）+ Global.vars
- **env 展開時**: 該当レベルの InternalVars（from_env + vars の結果）

## 4. インターフェース設計

### 4.1 変数展開インターフェース

```go
type VariableExpander interface {
    ExpandString(value string, context VariableContext) (string, error)
    ExpandStringSlice(values []string, context VariableContext) ([]string, error)
    ValidateReferences(value string, context VariableContext) error
}

type VariableContext interface {
    GetVariable(name string) (string, bool)
    GetAllVariables() map[string]string
    IsSystemVariable(name string) bool
    IsAllowed(name string) bool
}
```

### 4.2 環境変数プロセッサインターフェース

```go
type EnvironmentProcessor interface {
    ProcessGlobalEnv(global *GlobalConfig) error
    ProcessGroupEnv(group *CommandGroup, global *GlobalConfig) error
    ProcessCommandEnv(cmd *Command, group *CommandGroup, global *GlobalConfig) error
    BuildEnvironment(cmd *Command, group *CommandGroup, global *GlobalConfig) (map[string]string, error)
}
```

## 5. エラー処理設計

### 5.1 エラー型定義

```go
type VariableError struct {
    Type     ErrorType
    Variable string
    Location string
    Message  string
    Cause    error
}

type ErrorType int

const (
    ErrorUndefinedVariable ErrorType = iota
    ErrorCircularReference
    ErrorAllowlistViolation
    ErrorSyntaxError
    ErrorExpansionDepthExceeded
)
```

### 5.2 エラーメッセージ設計

- **詳細な位置情報**: global/group/command レベルでの正確なエラー位置
- **修正提案**: 具体的な修正方法の提示
- **関連情報**: 循環参照の場合は参照チェーンの表示

## 6. 設定ファイル検証

### 6.1 検証ルール

1. **構文検証**: TOML 構文の正確性
2. **スキーマ検証**: フィールドの型と必須項目
3. **セキュリティ検証**: allowlist との整合性
4. **参照整合性**: 変数参照の妥当性
5. **循環参照検証**: 変数間の循環参照チェック

### 6.2 検証アルゴリズム

```go
func ValidateConfiguration(config *Config) []ValidationError {
    var errors []ValidationError

    // 1. 構文検証
    errors = append(errors, validateSyntax(config)...)

    // 2. allowlist 検証
    errors = append(errors, validateAllowlist(config)...)

    // 3. 変数参照検証
    errors = append(errors, validateVariableReferences(config)...)

    // 4. 循環参照検証
    errors = append(errors, validateCircularReferences(config)...)

    return errors
}
```

## 7. 性能最適化

### 7.1 キャッシュ戦略

- **展開結果キャッシュ**: 一度展開した変数値のキャッシュ
- **参照関係キャッシュ**: 変数間の依存関係のキャッシュ
- **環境変数マップキャッシュ**: 構築済み環境変数の再利用

### 7.2 遅延評価

- **段階的展開**: 必要になるまで変数展開を遅延
- **オンデマンド構築**: 実行時の環境変数構築

## 8. エスケープシーケンス処理の変更

### 8.1 変更の必要性

**`${VAR}` 構文の廃止に伴う変更**:
- 本タスクで `${VAR}` 構文が完全に廃止される
- `$` 記号は特殊文字ではなくなる
- `\$` エスケープシーケンスは不要になる

### 8.2 変更対象

**`internal/runner/environment/processor.go`**:
```go
// 変更前（Task 0026 の実装）
func (p *VariableExpander) handleEscapeSequence(...) (int, error) {
    // ...
    nextChar := inputChars[i+1]
    if nextChar == '$' || nextChar == '\\' {  // $ と \ をエスケープ
        result.WriteRune(nextChar)
        return i + 2, nil
    }
    // ...
}

// 変更後（本タスク）
func (p *VariableExpander) handleEscapeSequence(...) (int, error) {
    // ...
    nextChar := inputChars[i+1]
    if nextChar == '%' || nextChar == '\\' {  // % と \ をエスケープ
        result.WriteRune(nextChar)
        return i + 2, nil
    }
    // ...
}
```

### 8.3 テストの更新

**変更が必要なテストケース**:
- `internal/runner/config/expansion_test.go`: エスケープシーケンスのテスト
- `internal/runner/environment/processor_test.go`: エスケープ処理のテスト

**更新例**:
```go
// 変更前
{
    name: "escape sequence handling",
    args: []string{"\\${HOME}", "${MESSAGE}"},
    expectedArgs: []string{"${HOME}", "hello"},
}

// 変更後
{
    name: "escape sequence handling",
    args: []string{"\\%{home}", "%{message}"},
    expectedArgs: []string{"%{home}", "hello"},
}
```

## 9. テスト設計

### 9.1 単体テスト

- **変数展開**: 各種パターンでの正常な展開
- **循環参照**: 循環参照の正確な検出
- **エラーハンドリング**: 各種エラー条件の適切な処理
- **エスケープシーケンス**: `\%` と `\\` のエスケープ処理

### 9.2 統合テスト

- **設定読み込み**: 複雑な設定ファイルの正常な処理
- **実行フロー**: end-to-end での動作確認
- **セキュリティ**: allowlist 機能の確実な動作
- **後方互換性**: `${VAR}` 構文の検出とエラー処理

### 9.3 性能テスト

- **スケーラビリティ**: 大量の変数での性能測定
- **メモリ使用量**: メモリ効率の測定
- **レスポンス性能**: 設定読み込み時間の測定
