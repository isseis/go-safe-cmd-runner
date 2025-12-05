# 詳細仕様書: vars テーブル形式への変更

## 0. 既存機能活用方針

この実装では、重複開発を避け既存の変数展開インフラを最大限活用する：

- **ExpandString 関数**: 再帰的な変数展開エンジン、循環参照検出
  - `ExpandString()`: 文字列中の `%{VAR}` を展開（エクスポート済み）
  - `expandStringRecursive()`: 再帰展開と循環検出（内部関数、リファクタリング対象）
- **ValidateVariableName 関数**: 変数名のバリデーション
  - 英字/アンダースコアで始まり、英数字/アンダースコアのみ
  - 予約プレフィックス（`__runner_`）の拒否
- **既存エラー型**: 変数展開関連エラー（`ErrCircularReferenceDetail`, `ErrUndefinedVariableDetail` 等）

**アーキテクチャ上の決定**:
- `ProcessVars` の入力型を `[]string` から `map[string]interface{}` に変更
- 出力を `(map[string]string, map[string][]string, error)` に変更（配列変数対応）
- 型検証とサイズ制限を `ProcessVars` 内で実施

**コード重複の回避（リファクタリング）**:
- 既存の `expandStringRecursive` と `varExpander` の展開ロジックが重複するため、共通化する
- `expandStringWithResolver()`: コア展開ロジック（エスケープシーケンス、`%{VAR}` パース、循環検出）を抽出
- `variableResolver` 型: 変数解決の戦略を関数型で抽象化
  - 既存の `ExpandString`: `expandedVars` から即時解決
  - 新規の `varExpander`: `expandedVars` と `rawVars` から遅延解決
- このリファクタリングにより、変数展開ロジックを **1箇所に集約** し、メンテナンス性を向上

**新規追加する構造体と関数**:
- `variableResolver` 型: 変数名を展開値に解決する関数型
- `expandStringWithResolver()`: 共通の変数展開ロジック（既存の `expandStringRecursive` を置き換え）
- `varExpander` 構造体: 変数展開の状態を管理
  - Goの `map` 反復順序は非決定的であるため、変数 A が変数 B を参照している場合、B が先に展開されている保証がない
  - `expandedVars`（展開済み変数）と `rawVars`（未展開変数）をフィールドとして持ち、遅延展開を行う
  - メモ化により展開結果を `expandedVars` にキャッシュ
  - これにより、**map の反復順序に依存しない正しい変数展開**が実現される
- `varExpander.expandString()`: 公開メソッド、変数展開のエントリポイント
- `varExpander.resolveVariable()`: 内部メソッド、遅延展開とメモ化

これにより**実証済みセキュリティ機能を継承**し、**配列変数サポートを追加**できる。

## 1. 実装詳細仕様

### 1.1 パッケージ構成詳細

```
# 変更対象コンポーネント
internal/runner/runnertypes/spec.go       # Vars フィールドの型変更
internal/runner/runnertypes/runtime.go    # ExpandedArrayVars フィールド追加
internal/runner/config/expansion.go       # ProcessVars の更新、expandStringRecursive のリファクタリング
internal/runner/config/errors.go          # 新規エラー型の追加
internal/runner/config/loader.go          # 旧形式検出エラーの追加

# 既存コンポーネント（再利用）
internal/runner/security/validation.go    # ValidateVariableName（変更なし）
```

### 1.2 型定義とインターフェース

#### 1.2.1 GlobalSpec 構造体の変更

```go
// internal/runner/runnertypes/spec.go

type GlobalSpec struct {
    // ... existing fields ...

    // Vars defines internal variables as a TOML table.
    // Each key-value pair defines a variable where:
    //   - Key: variable name (must pass ValidateVariableName)
    //   - Value: string or []string (other types are rejected)
    //
    // Example TOML:
    //   [global.vars]
    //   base_dir = "/opt/myapp"
    //   config_files = ["config.yml", "secrets.yml"]
    //
    // Changed from: Vars []string `toml:"vars"` (array-based format)
    Vars map[string]interface{} `toml:"vars"`
}
```

#### 1.2.2 GroupSpec 構造体の変更

```go
// internal/runner/runnertypes/spec.go

type GroupSpec struct {
    // ... existing fields ...

    // Vars defines group-level internal variables as a TOML table.
    // See GlobalSpec.Vars for format details.
    //
    // Example TOML:
    //   [[groups]]
    //   name = "deploy"
    //   [groups.vars]
    //   deploy_target = "production"
    //
    // Changed from: Vars []string `toml:"vars"` (array-based format)
    Vars map[string]interface{} `toml:"vars"`
}
```

#### 1.2.3 CommandSpec 構造体の変更

```go
// internal/runner/runnertypes/spec.go

type CommandSpec struct {
    // ... existing fields ...

    // Vars defines command-level internal variables as a TOML table.
    // See GlobalSpec.Vars for format details.
    //
    // Example TOML:
    //   [[groups.commands]]
    //   name = "backup"
    //   [groups.commands.vars]
    //   backup_suffix = ".bak"
    //
    // Changed from: Vars []string `toml:"vars"` (array-based format)
    Vars map[string]interface{} `toml:"vars"`
}
```

#### 1.2.4 RuntimeGlobal 構造体の拡張

```go
// internal/runner/runnertypes/runtime.go

type RuntimeGlobal struct {
    // ... existing fields ...

    // ExpandedVars contains string variables with all variable references expanded.
    // This includes variables from both env_import and vars sections.
    ExpandedVars map[string]string

    // ExpandedArrayVars contains array variables with all variable references expanded.
    // Each array element has been individually expanded using ExpandString.
    // This is populated by ProcessVars when processing array-type variables.
    //
    // Example:
    //   TOML: config_files = ["%{base_dir}/config.yml", "%{base_dir}/secrets.yml"]
    //   After expansion: {"config_files": ["/opt/myapp/config.yml", "/opt/myapp/secrets.yml"]}
    ExpandedArrayVars map[string][]string
}
```

#### 1.2.5 RuntimeGroup 構造体の拡張

```go
// internal/runner/runnertypes/runtime.go

type RuntimeGroup struct {
    // ... existing fields ...

    // ExpandedVars contains string variables with all variable references expanded.
    ExpandedVars map[string]string

    // ExpandedArrayVars contains array variables with all variable references expanded.
    // See RuntimeGlobal.ExpandedArrayVars for details.
    ExpandedArrayVars map[string][]string
}
```

#### 1.2.6 RuntimeCommand 構造体の拡張

```go
// internal/runner/runnertypes/runtime.go

type RuntimeCommand struct {
    // ... existing fields ...

    // ExpandedVars contains string variables with all variable references expanded.
    ExpandedVars map[string]string

    // ExpandedArrayVars contains array variables with all variable references expanded.
    // See RuntimeGlobal.ExpandedArrayVars for details.
    ExpandedArrayVars map[string][]string
}
```

### 1.3 制限値定数

```go
// internal/runner/config/expansion.go

const (
    // MaxRecursionDepth is the maximum depth for variable expansion (existing)
    MaxRecursionDepth = 100

    // MaxVarsPerLevel is the maximum number of variables allowed per level
    // (global, group, or command). This prevents DoS attacks via excessive
    // variable definitions.
    MaxVarsPerLevel = 1000

    // MaxArrayElements is the maximum number of elements allowed in an array
    // variable. This prevents DoS attacks via large arrays.
    MaxArrayElements = 1000

    // MaxStringValueLen is the maximum length (in bytes) allowed for a string
    // value. This prevents memory exhaustion via extremely long strings.
    MaxStringValueLen = 10 * 1024 // 10KB
)
```

### 1.4 既存の ExpandString のリファクタリング

既存の `expandStringRecursive` 関数と、新規の `varExpander` 構造体が同じ展開ロジックを持つため、
コードの重複を避けるため、共通の展開ロジックを `expandStringWithResolver` 関数として抽出します。

#### 1.4.1 既存の expandStringRecursive の置き換え

既存の `expandStringRecursive` 関数を、`expandStringWithResolver` 関数を使用する実装に変更します。

```go
// expandStringRecursive performs recursive expansion with circular reference detection.
// This is a wrapper around expandStringWithResolver for backward compatibility.
func expandStringRecursive(
    input string,
    expandedVars map[string]string,
    level string,
    field string,
    visited map[string]struct{},
    expansionChain []string,
    depth int,
) (string, error) {
    // Create a resolver that looks up variables from expandedVars
    // and recursively expands them
    resolver := func(
        varName string,
        resolverField string,
        resolverVisited map[string]struct{},
        resolverChain []string,
        resolverDepth int,
    ) (string, error) {
        // Check if variable is defined
        value, exists := expandedVars[varName]
        if !exists {
            return "", &ErrUndefinedVariableDetail{
                Level:        level,
                Field:        resolverField,
                VariableName: varName,
                Context:      input,
            }
        }

        // Mark as visited for circular reference detection
        resolverVisited[varName] = struct{}{}

        // Recursively expand the value
        expandedValue, err := expandStringRecursive(
            value,
            expandedVars,
            level,
            resolverField,
            resolverVisited,
            append(resolverChain, varName),
            resolverDepth+1,
        )
        if err != nil {
            return "", err
        }

        // Unmark after expansion
        delete(resolverVisited, varName)

        return expandedValue, nil
    }

    return expandStringWithResolver(input, resolver, level, field, visited, expansionChain, depth)
}
```

この実装により、既存の `expandStringRecursive` は `expandStringWithResolver` を使用するラッパーとなり、
変数展開のコアロジック（エスケープシーケンス処理、`%{VAR}` パース、循環検出）は `expandStringWithResolver` に集約されます。

### 1.5 ProcessVars 関数の更新

#### 1.5.1 関数シグネチャ

```go
// internal/runner/config/expansion.go

// ProcessVars processes vars definitions from a TOML table and expands them
// using baseExpandedVars and baseExpandedArrays.
//
// Parameters:
//   - vars: Variable definitions from TOML (map[string]interface{})
//   - baseExpandedVars: Previously expanded string variables (inherited)
//   - baseExpandedArrays: Previously expanded array variables (inherited)
//   - level: Context for error messages (e.g., "global", "group[deploy]")
//
// Returns:
//   - map[string]string: Expanded string variables (includes base + new)
//   - map[string][]string: Expanded array variables (includes base + new)
//   - error: Validation or expansion error
//
// Processing steps:
//  1. Check total variable count against MaxVarsPerLevel
//  2. For each variable:
//     a. Validate variable name using ValidateVariableName
//     b. Check type consistency with base variables
//     c. Validate value type (string or []interface{})
//     d. Validate size limits
//     e. Expand using ExpandString
//     f. Store in appropriate output map
//
// Type consistency rule:
//   - A variable defined as string cannot be overridden as array
//   - A variable defined as array cannot be overridden as string
//   - Same type override is allowed (value replacement)
//
// Empty arrays are allowed and useful for clearing inherited variables.
func ProcessVars(
    vars map[string]interface{},
    baseExpandedVars map[string]string,
    baseExpandedArrays map[string][]string,
    level string,
) (map[string]string, map[string][]string, error)
```

#### 1.5.2 実装

**重要な設計上の考慮事項: map反復順序と変数依存関係**

Goの `map` は反復順序が非決定的であるため、変数の処理順序は保証されません。
例えば、以下の定義では `config_path` が `base_dir` より先に処理される可能性があります：

```toml
[global.vars]
base_dir = "/opt"
config_path = "%{base_dir}/config.yml"
```

この問題を解決するため、実装は2段階で行います：

1. **Phase 1: 検証と型チェック (Validation and type checking)** - すべての変数を検証し、型チェックを行い、未展開の値を一時マップに格納
2. **Phase 2: 遅延解決による展開 (Expansion with lazy resolution)** - `varExpander` を使用して未展開変数を動的に解決

既存の `ExpandString` は `expandedVars` に存在する変数のみを参照可能なため、
**未展開変数マップ** (`rawVars`) も参照可能な `varExpander` 構造体を導入します。

**コード重複の回避**: 既存の `expandStringRecursive` と変数展開ロジックを共有するため、
共通の展開ロジックを抽出した `expandStringWithResolver` 関数を導入します。
これにより、変数解決の戦略（即時解決 vs 遅延解決）のみが異なる実装を、
コア展開ロジックを重複させることなく実現できます。

```go
// variableResolver is a function type that resolves a variable name to its expanded value.
// It is called during variable expansion to look up and expand variable references.
//
// Parameters:
//   - varName: the variable name to resolve (without %{} syntax)
//   - field: field name for error messages
//   - visited: map tracking currently-being-expanded variables (for circular detection)
//   - expansionChain: ordered list of variable names in current expansion path
//   - depth: current recursion depth
//
// Returns:
//   - string: the expanded value of the variable
//   - error: resolution error (e.g., undefined variable, type mismatch)
type variableResolver func(
    varName string,
    field string,
    visited map[string]struct{},
    expansionChain []string,
    depth int,
) (string, error)

// expandStringWithResolver performs recursive variable expansion using a custom resolver.
// This is the core expansion logic shared by both ExpandString and varExpander.
//
// Parameters:
//   - input: the string to expand (may contain %{VAR} references and escape sequences)
//   - resolver: function to resolve variable names to their values
//   - level: context for error messages (e.g., "global", "group[deploy]")
//   - field: field name for error messages (e.g., "vars", "env.PATH")
//   - visited: tracks variables currently being expanded (for circular reference detection)
//   - expansionChain: ordered list of variable names in the current expansion path
//   - depth: current recursion depth
//
// Returns:
//   - string: the fully expanded string
//   - error: expansion error (syntax error, undefined variable, circular reference, etc.)
func expandStringWithResolver(
    input string,
    resolver variableResolver,
    level string,
    field string,
    visited map[string]struct{},
    expansionChain []string,
    depth int,
) (string, error) {
    // Check recursion depth
    if depth >= MaxRecursionDepth {
        return "", &ErrMaxRecursionDepthExceededDetail{
            Level:    level,
            Field:    field,
            MaxDepth: MaxRecursionDepth,
            Context:  input,
        }
    }

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
                return "", &ErrInvalidEscapeSequenceDetail{
                    Level:    level,
                    Field:    field,
                    Sequence: input[i : i+2],
                    Context:  input,
                }
            }
        }

        // Handle %{VAR} expansion
        if input[i] == '%' && i+1 < len(input) && input[i+1] == '{' {
            const openBraceLen = 2
            closeIdx := strings.IndexByte(input[i+openBraceLen:], '}')
            if closeIdx == -1 {
                return "", &ErrUnclosedVariableReferenceDetail{
                    Level:   level,
                    Field:   field,
                    Context: input,
                }
            }
            closeIdx += i + openBraceLen

            varName := input[i+openBraceLen : closeIdx]

            // Validate variable name
            if err := security.ValidateVariableName(varName); err != nil {
                return "", &ErrInvalidVariableNameDetail{
                    Level:        level,
                    Field:        field,
                    VariableName: varName,
                    Reason:       err.Error(),
                }
            }

            // Check for circular reference
            if _, ok := visited[varName]; ok {
                return "", &ErrCircularReferenceDetail{
                    Level:        level,
                    Field:        field,
                    VariableName: varName,
                    Chain:        append(expansionChain, varName),
                }
            }

            // Resolve variable using the provided resolver
            value, err := resolver(varName, field, visited, expansionChain, depth)
            if err != nil {
                return "", err
            }

            result.WriteString(value)
            i = closeIdx + 1
            continue
        }

        result.WriteByte(input[i])
        i++
    }

    return result.String(), nil
}

// varExpander handles variable expansion with lazy resolution.
// It maintains state for memoization and circular reference detection.
//
// SIDE EFFECT: The expandString method modifies the expandedVars map by adding
// newly expanded variables to it. This is intentional for memoization.
type varExpander struct {
    // expandedVars contains already-expanded string variables.
    // Also used for memoization of newly expanded variables.
    expandedVars map[string]string

    // rawVars contains not-yet-expanded variable definitions.
    rawVars map[string]interface{}

    // level is the context for error messages (e.g., "global", "group[deploy]").
    level string
}

// newVarExpander creates a new varExpander instance.
func newVarExpander(
    expandedVars map[string]string,
    rawVars map[string]interface{},
    level string,
) *varExpander {
    return &varExpander{
        expandedVars: expandedVars,
        rawVars:      rawVars,
        level:        level,
    }
}

// expandString expands variable references in the input string.
// It resolves references to both already-expanded and raw variables.
//
// Parameters:
//   - input: the string containing %{VAR} references to expand
//   - field: field name for error messages (e.g., "vars.config_path")
//
// Returns the expanded string or an error.
func (e *varExpander) expandString(input string, field string) (string, error) {
    visited := make(map[string]struct{})
    expansionChain := make([]string, 0)

    // Use expandStringWithResolver with varExpander's resolver
    resolver := func(
        varName string,
        resolverField string,
        resolverVisited map[string]struct{},
        resolverChain []string,
        resolverDepth int,
    ) (string, error) {
        return e.resolveVariable(varName, resolverField, resolverVisited, resolverChain, resolverDepth)
    }

    return expandStringWithResolver(input, resolver, e.level, field, visited, expansionChain, 0)
}

// resolveVariable looks up and expands a variable by name.
// It checks expandedVars first, then rawVars for lazy expansion.
func (e *varExpander) resolveVariable(
    varName string,
    field string,
    visited map[string]struct{},
    expansionChain []string,
    depth int,
) (string, error) {
    // First, check already-expanded variables (includes memoized results)
    if v, ok := e.expandedVars[varName]; ok {
        return v, nil
    }

    // Check raw vars for lazy expansion
    rawVal, ok := e.rawVars[varName]
    if !ok {
        return "", &ErrUndefinedVariableDetail{
            Level:        e.level,
            Field:        field,
            VariableName: varName,
            Context:      "",
            Chain:        append(expansionChain, varName),
        }
    }

    // Handle based on type
    switch rv := rawVal.(type) {
    case string:
        // Mark as visited before recursive expansion
        visited[varName] = struct{}{}

        // Create a resolver for recursive expansion
        resolver := func(
            resolverVarName string,
            resolverField string,
            resolverVisited map[string]struct{},
            resolverChain []string,
            resolverDepth int,
        ) (string, error) {
            return e.resolveVariable(resolverVarName, resolverField, resolverVisited, resolverChain, resolverDepth)
        }

        // Expand the raw value using the shared expansion logic
        expanded, err := expandStringWithResolver(
            rv,
            resolver,
            e.level,
            field,
            visited,
            append(expansionChain, varName),
            depth+1,
        )
        if err != nil {
            return "", err
        }

        // Unmark after expansion
        delete(visited, varName)

        // Cache the expanded value for future references (memoization)
        e.expandedVars[varName] = expanded

        return expanded, nil

    default:
        // Array variable cannot be referenced in string context
        return "", &ErrArrayVariableInStringContextDetail{
            Level:        e.level,
            Field:        field,
            VariableName: varName,
            Chain:        append(expansionChain, varName),
        }
    }
}

func ProcessVars(
    vars map[string]interface{},
    baseExpandedVars map[string]string,
    baseExpandedArrays map[string][]string,
    level string,
) (map[string]string, map[string][]string, error) {
    // Handle nil/empty input
    if len(vars) == 0 {
        return maps.Clone(baseExpandedVars), maps.Clone(baseExpandedArrays), nil
    }

    // 1. Check total variable count
    if len(vars) > MaxVarsPerLevel {
        return nil, nil, &ErrTooManyVariablesDetail{
            Level:    level,
            Count:    len(vars),
            MaxCount: MaxVarsPerLevel,
        }
    }

    // ========================================
    // Phase 1: Validation and type checking
    // ========================================

    // Separate string and array variables
    stringVars := make(map[string]string)
    arrayVars := make(map[string][]interface{})

    for varName, rawValue := range vars {
        // 1a. Validate variable name
        if err := security.ValidateVariableName(varName); err != nil {
            return nil, nil, err
        }

        // 1b. Check type consistency with base variables
        _, existsAsString := baseExpandedVars[varName]
        _, existsAsArray := baseExpandedArrays[varName]

        switch v := rawValue.(type) {
        case string:
            if existsAsArray {
                return nil, nil, &ErrTypeMismatchDetail{
                    Level:        level,
                    VariableName: varName,
                    ExpectedType: "array",
                    ActualType:   "string",
                }
            }

            // Validate string length
            if len(v) > MaxStringValueLen {
                return nil, nil, &ErrValueTooLongDetail{
                    Level:        level,
                    VariableName: varName,
                    Length:       len(v),
                    MaxLength:    MaxStringValueLen,
                }
            }

            stringVars[varName] = v

        case []interface{}:
            if existsAsString {
                return nil, nil, &ErrTypeMismatchDetail{
                    Level:        level,
                    VariableName: varName,
                    ExpectedType: "string",
                    ActualType:   "array",
                }
            }

            // Validate array size (empty arrays are allowed)
            if len(v) > MaxArrayElements {
                return nil, nil, &ErrArrayTooLargeDetail{
                    Level:        level,
                    VariableName: varName,
                    Count:        len(v),
                    MaxCount:     MaxArrayElements,
                }
            }

            // Validate each element is a string and check length
            for i, elem := range v {
                str, ok := elem.(string)
                if !ok {
                    return nil, nil, &ErrInvalidArrayElementDetail{
                        Level:        level,
                        VariableName: varName,
                        Index:        i,
                        ExpectedType: "string",
                        ActualType:   fmt.Sprintf("%T", elem),
                    }
                }
                if len(str) > MaxStringValueLen {
                    return nil, nil, &ErrArrayElementTooLongDetail{
                        Level:        level,
                        VariableName: varName,
                        Index:        i,
                        Length:       len(str),
                        MaxLength:    MaxStringValueLen,
                    }
                }
            }

            arrayVars[varName] = v

        default:
            return nil, nil, &ErrUnsupportedTypeDetail{
                Level:        level,
                VariableName: varName,
                ActualType:   fmt.Sprintf("%T", rawValue),
            }
        }
    }

    // ========================================
    // Phase 2: Expansion with lazy resolution
    // ========================================

    // Start with copies of base variables
    expandedStrings := maps.Clone(baseExpandedVars)
    if expandedStrings == nil {
        expandedStrings = make(map[string]string)
    }
    expandedArrays := maps.Clone(baseExpandedArrays)
    if expandedArrays == nil {
        expandedArrays = make(map[string][]string)
    }

    // Convert stringVars to interface{} map for rawVars parameter
    // We reconstruct rawVars from validated maps (stringVars/arrayVars) instead of
    // using the original vars map to ensure only validated and supported types
    // are exposed to the expansion logic.
    rawVars := make(map[string]interface{}, len(stringVars)+len(arrayVars))
    for k, v := range stringVars {
        rawVars[k] = v
    }
    for k, v := range arrayVars {
        rawVars[k] = v
    }

    // Create expander for lazy variable resolution
    expander := newVarExpander(expandedStrings, rawVars, level)

    // Expand string variables (order-independent due to lazy resolution)
    for varName, rawValue := range stringVars {
        expanded, err := expander.expandString(
            rawValue,
            fmt.Sprintf("vars.%s", varName),
        )
        if err != nil {
            return nil, nil, err
        }
        expandedStrings[varName] = expanded
    }

    // Expand array variables
    for varName, rawArray := range arrayVars {
        expandedArray := make([]string, len(rawArray))
        for i, elem := range rawArray {
            str := elem.(string) // Already validated in Phase 1

            expanded, err := expander.expandString(
                str,
                fmt.Sprintf("vars.%s[%d]", varName, i),
            )
            if err != nil {
                return nil, nil, err
            }
            expandedArray[i] = expanded
        }
        expandedArrays[varName] = expandedArray
    }

    return expandedStrings, expandedArrays, nil
}
```

### 1.6 エラー型定義

```go
// internal/runner/config/errors.go

// ===========================================
// Existing error type modification
// ===========================================

// ErrUndefinedVariableDetail needs to be extended with Chain field.
// Add the following field to the existing struct:
//
// type ErrUndefinedVariableDetail struct {
//     Level        string
//     Field        string
//     VariableName string
//     Context      string
//     Chain        []string // NEW: expansion path leading to this error
// }
//
// Update the Error() method to include the chain in the message:
//
// func (e *ErrUndefinedVariableDetail) Error() string {
//     msg := fmt.Sprintf(
//         "undefined variable %q referenced in %s.%s",
//         e.VariableName, e.Level, e.Field,
//     )
//     if len(e.Chain) > 0 {
//         msg += fmt.Sprintf(" (expansion path: %s)", strings.Join(e.Chain, " -> "))
//     }
//     return msg
// }

// ===========================================
// New error types
// ===========================================

// ErrTooManyVariablesDetail is returned when the number of variables exceeds
// MaxVarsPerLevel.
type ErrTooManyVariablesDetail struct {
    Level    string
    Count    int
    MaxCount int
}

func (e *ErrTooManyVariablesDetail) Error() string {
    return fmt.Sprintf(
        "too many variables in %s: got %d, max %d",
        e.Level, e.Count, e.MaxCount,
    )
}

// ErrTypeMismatchDetail is returned when a variable is redefined with a
// different type (string vs array).
type ErrTypeMismatchDetail struct {
    Level        string
    VariableName string
    ExpectedType string
    ActualType   string
}

func (e *ErrTypeMismatchDetail) Error() string {
    return fmt.Sprintf(
        "variable %q type mismatch in %s: already defined as %s, cannot redefine as %s",
        e.VariableName, e.Level, e.ExpectedType, e.ActualType,
    )
}

// ErrValueTooLongDetail is returned when a string value exceeds MaxStringValueLen.
type ErrValueTooLongDetail struct {
    Level        string
    VariableName string
    Length       int
    MaxLength    int
}

func (e *ErrValueTooLongDetail) Error() string {
    return fmt.Sprintf(
        "variable %q value too long in %s: got %d bytes, max %d",
        e.VariableName, e.Level, e.Length, e.MaxLength,
    )
}

// ErrArrayTooLargeDetail is returned when an array variable exceeds MaxArrayElements.
type ErrArrayTooLargeDetail struct {
    Level        string
    VariableName string
    Count        int
    MaxCount     int
}

func (e *ErrArrayTooLargeDetail) Error() string {
    return fmt.Sprintf(
        "variable %q array too large in %s: got %d elements, max %d",
        e.VariableName, e.Level, e.Count, e.MaxCount,
    )
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
    return fmt.Sprintf(
        "variable %q has invalid array element at index %d in %s: expected %s, got %s",
        e.VariableName, e.Index, e.Level, e.ExpectedType, e.ActualType,
    )
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
    return fmt.Sprintf(
        "variable %q array element %d too long in %s: got %d bytes, max %d",
        e.VariableName, e.Index, e.Level, e.Length, e.MaxLength,
    )
}

// ErrUnsupportedTypeDetail is returned when a variable value has an unsupported type.
type ErrUnsupportedTypeDetail struct {
    Level        string
    VariableName string
    ActualType   string
}

func (e *ErrUnsupportedTypeDetail) Error() string {
    return fmt.Sprintf(
        "variable %q has unsupported type %s in %s: only string and []string are supported",
        e.VariableName, e.ActualType, e.Level,
    )
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
    msg := fmt.Sprintf(
        "cannot reference array variable %q in string context at %s.%s: "+
            "array variables can only be used where array values are expected",
        e.VariableName, e.Level, e.Field,
    )
    if len(e.Chain) > 0 {
        msg += fmt.Sprintf(" (expansion path: %s)", strings.Join(e.Chain, " -> "))
    }
    return msg
}
```

### 1.7 ExpandGlobal の更新

```go
// internal/runner/config/expansion.go

func ExpandGlobal(spec *runnertypes.GlobalSpec) (*runnertypes.RuntimeGlobal, error) {
    // Create RuntimeGlobal (unchanged)
    runtime, err := runnertypes.NewRuntimeGlobal(spec)
    if err != nil {
        return nil, fmt.Errorf("failed to create RuntimeGlobal: %w", err)
    }

    // 0. Parse system environment (unchanged)
    runtime.SystemEnv = environment.NewFilter(spec.EnvAllowed).ParseSystemEnvironment()

    // 0.5. Generate automatic variables (unchanged)
    autoVars := variable.GenerateGlobalAutoVars(nil)
    runtime.ExpandedVars = autoVars
    runtime.ExpandedArrayVars = make(map[string][]string) // Initialize array vars

    // 1. Process FromEnv (unchanged)
    fromEnvVars, err := ProcessFromEnv(spec.EnvImport, spec.EnvAllowed, runtime.SystemEnv, "global")
    if err != nil {
        return nil, fmt.Errorf("failed to process global from_env: %w", err)
    }
    for k, v := range fromEnvVars {
        runtime.ExpandedVars[k] = v
    }

    // 2. Process Vars (UPDATED)
    expandedVars, expandedArrays, err := ProcessVars(
        spec.Vars,
        runtime.ExpandedVars,
        runtime.ExpandedArrayVars,
        "global",
    )
    if err != nil {
        return nil, fmt.Errorf("failed to process global vars: %w", err)
    }
    runtime.ExpandedVars = expandedVars
    runtime.ExpandedArrayVars = expandedArrays

    // 3. Expand Env (unchanged)
    expandedEnv, err := ProcessEnv(spec.EnvVars, runtime.ExpandedVars, "global")
    if err != nil {
        return nil, fmt.Errorf("failed to process global env: %w", err)
    }
    runtime.ExpandedEnv = expandedEnv

    // 4. Expand VerifyFiles (unchanged)
    runtime.ExpandedVerifyFiles = make([]string, len(spec.VerifyFiles))
    for i, file := range spec.VerifyFiles {
        expandedFile, err := ExpandString(file, runtime.ExpandedVars, "global", fmt.Sprintf("verify_files[%d]", i))
        if err != nil {
            return nil, err
        }
        runtime.ExpandedVerifyFiles[i] = expandedFile
    }

    return runtime, nil
}
```

### 1.8 ExpandGroup の更新

```go
// internal/runner/config/expansion.go

func ExpandGroup(spec *runnertypes.GroupSpec, globalRuntime *runnertypes.RuntimeGlobal) (*runnertypes.RuntimeGroup, error) {
    runtime, err := runnertypes.NewRuntimeGroup(spec)
    if err != nil {
        return nil, fmt.Errorf("failed to create RuntimeGroup: %w", err)
    }

    // Set inheritance mode (unchanged)
    runtime.EnvAllowlistInheritanceMode = runnertypes.DetermineEnvAllowlistInheritanceMode(spec.EnvAllowed)

    // 1. Inherit global variables
    if globalRuntime != nil {
        maps.Copy(runtime.ExpandedVars, globalRuntime.ExpandedVars)
        maps.Copy(runtime.ExpandedArrayVars, globalRuntime.ExpandedArrayVars) // NEW: inherit array vars
    }

    // 2. Process FromEnv (unchanged)
    if len(spec.EnvImport) > 0 {
        var globalAllowlist []string
        var systemEnv map[string]string
        if globalRuntime != nil {
            globalAllowlist = globalRuntime.EnvAllowlist()
            systemEnv = globalRuntime.SystemEnv
        }

        effectiveAllowlist := determineEffectiveEnvAllowlist(spec.EnvAllowed, globalAllowlist)

        fromEnvVars, err := ProcessFromEnv(spec.EnvImport, effectiveAllowlist, systemEnv, fmt.Sprintf("group[%s]", spec.Name))
        if err != nil {
            return nil, fmt.Errorf("failed to process group[%s] from_env: %w", spec.Name, err)
        }

        maps.Copy(runtime.ExpandedVars, fromEnvVars)
    }

    // 3. Process Vars (UPDATED)
    expandedVars, expandedArrays, err := ProcessVars(
        spec.Vars,
        runtime.ExpandedVars,
        runtime.ExpandedArrayVars,
        fmt.Sprintf("group[%s]", spec.Name),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to process group[%s] vars: %w", spec.Name, err)
    }
    runtime.ExpandedVars = expandedVars
    runtime.ExpandedArrayVars = expandedArrays

    // 4. Expand Env (unchanged)
    expandedEnv, err := ProcessEnv(spec.EnvVars, runtime.ExpandedVars, fmt.Sprintf("group[%s]", spec.Name))
    if err != nil {
        return nil, fmt.Errorf("failed to process group[%s] env: %w", spec.Name, err)
    }
    runtime.ExpandedEnv = expandedEnv

    // 5. Expand VerifyFiles (unchanged)
    runtime.ExpandedVerifyFiles = make([]string, len(spec.VerifyFiles))
    for i, file := range spec.VerifyFiles {
        expandedFile, err := ExpandString(file, runtime.ExpandedVars, fmt.Sprintf("group[%s]", spec.Name), fmt.Sprintf("verify_files[%d]", i))
        if err != nil {
            return nil, err
        }
        runtime.ExpandedVerifyFiles[i] = expandedFile
    }

    // 6. Expand CmdAllowed (unchanged)
    expandedCmdAllowed, err := expandCmdAllowed(
        spec.CmdAllowed,
        runtime.ExpandedVars,
        spec.Name,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to expand cmd_allowed for group[%s]: %w", spec.Name, err)
    }
    runtime.ExpandedCmdAllowed = expandedCmdAllowed

    return runtime, nil
}
```

### 1.9 ExpandCommand の更新

```go
// internal/runner/config/expansion.go

func ExpandCommand(spec *runnertypes.CommandSpec, runtimeGroup *runnertypes.RuntimeGroup, globalRuntime *runnertypes.RuntimeGlobal, globalTimeout common.Timeout, globalOutputSizeLimit common.OutputSizeLimit) (*runnertypes.RuntimeCommand, error) {
    groupName := runnertypes.ExtractGroupName(runtimeGroup)
    runtime, err := runnertypes.NewRuntimeCommand(spec, globalTimeout, globalOutputSizeLimit, groupName)
    if err != nil {
        return nil, fmt.Errorf("failed to create RuntimeCommand for command[%s]: %w", spec.Name, err)
    }

    // 1. Inherit group variables
    if runtimeGroup != nil {
        maps.Copy(runtime.ExpandedVars, runtimeGroup.ExpandedVars)
        maps.Copy(runtime.ExpandedArrayVars, runtimeGroup.ExpandedArrayVars) // NEW: inherit array vars
    }

    // 2. Process FromEnv (unchanged)
    if len(spec.EnvImport) > 0 {
        var globalAllowlist []string
        var systemEnv map[string]string
        if globalRuntime != nil {
            globalAllowlist = globalRuntime.EnvAllowlist()
            systemEnv = globalRuntime.SystemEnv
        }

        var groupAllowlist []string
        if runtimeGroup != nil && runtimeGroup.Spec != nil {
            groupAllowlist = runtimeGroup.Spec.EnvAllowed
        }

        effectiveAllowlist := determineEffectiveEnvAllowlist(groupAllowlist, globalAllowlist)

        fromEnvVars, err := ProcessFromEnv(spec.EnvImport, effectiveAllowlist, systemEnv, fmt.Sprintf("command[%s]", spec.Name))
        if err != nil {
            return nil, fmt.Errorf("failed to process command[%s] from_env: %w", spec.Name, err)
        }

        maps.Copy(runtime.ExpandedVars, fromEnvVars)
    }

    // 3. Process Vars (UPDATED)
    expandedVars, expandedArrays, err := ProcessVars(
        spec.Vars,
        runtime.ExpandedVars,
        runtime.ExpandedArrayVars,
        fmt.Sprintf("command[%s]", spec.Name),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to process command[%s] vars: %w", spec.Name, err)
    }
    runtime.ExpandedVars = expandedVars
    runtime.ExpandedArrayVars = expandedArrays

    level := fmt.Sprintf("command[%s]", spec.Name)

    // 4. Expand Cmd (unchanged)
    expandedCmd, err := ExpandString(spec.Cmd, runtime.ExpandedVars, level, "cmd")
    if err != nil {
        return nil, err
    }
    runtime.ExpandedCmd = expandedCmd

    // 5. Expand Args (unchanged)
    runtime.ExpandedArgs = make([]string, len(spec.Args))
    for i, arg := range spec.Args {
        expandedArg, err := ExpandString(arg, runtime.ExpandedVars, level, fmt.Sprintf("args[%d]", i))
        if err != nil {
            return nil, err
        }
        runtime.ExpandedArgs[i] = expandedArg
    }

    // 6. Expand Env (unchanged)
    expandedEnv, err := ProcessEnv(spec.EnvVars, runtime.ExpandedVars, level)
    if err != nil {
        return nil, fmt.Errorf("failed to process command[%s] env: %w", spec.Name, err)
    }
    runtime.ExpandedEnv = expandedEnv

    return runtime, nil
}
```

### 1.10 旧形式検出エラー

```go
// internal/runner/config/loader.go

// detectLegacyVarsFormat checks if the TOML parsing error might be due to
// legacy vars format (vars = ["key=value"]) and returns a helpful error message.
func detectLegacyVarsFormat(err error) error {
    errStr := err.Error()
    // BurntSushi/toml returns type mismatch errors like:
    // "cannot decode [] into map[string]interface{}"
    if strings.Contains(errStr, "cannot decode") &&
        strings.Contains(errStr, "vars") &&
        strings.Contains(errStr, "[]") {
        return fmt.Errorf(
            "vars array format (vars = [\"key=value\"]) is no longer supported; "+
                "please migrate to table format ([*.vars] section). "+
                "See documentation for migration guide. Original error: %w",
            err,
        )
    }
    return err
}

// LoadConfig loads configuration from TOML file
func LoadConfig(configPath string) (*runnertypes.ConfigSpec, error) {
    var spec runnertypes.ConfigSpec
    _, err := toml.DecodeFile(configPath, &spec)
    if err != nil {
        return nil, detectLegacyVarsFormat(err)
    }
    // ... rest of loading logic ...
}
```

### 1.11 NewRuntimeGlobal の更新

```go
// internal/runner/runnertypes/runtime.go

func NewRuntimeGlobal(spec *GlobalSpec) (*RuntimeGlobal, error) {
    if spec == nil {
        return nil, ErrNilSpec
    }

    return &RuntimeGlobal{
        Spec:                spec,
        timeout:             common.NewFromIntPtr(spec.Timeout),
        ExpandedVerifyFiles: []string{},
        ExpandedEnv:         make(map[string]string),
        ExpandedVars:        make(map[string]string),
        ExpandedArrayVars:   make(map[string][]string), // NEW
        SystemEnv:           make(map[string]string),
    }, nil
}
```

### 1.12 NewRuntimeGroup の更新

```go
// internal/runner/runnertypes/runtime.go

func NewRuntimeGroup(spec *GroupSpec) (*RuntimeGroup, error) {
    if spec == nil {
        return nil, ErrNilSpec
    }
    return &RuntimeGroup{
        Spec:                spec,
        ExpandedVerifyFiles: []string{},
        ExpandedEnv:         make(map[string]string),
        ExpandedVars:        make(map[string]string),
        ExpandedArrayVars:   make(map[string][]string), // NEW
        Commands:            []*RuntimeCommand{},
    }, nil
}
```

### 1.13 NewRuntimeCommand の更新

```go
// internal/runner/runnertypes/runtime.go

func NewRuntimeCommand(spec *CommandSpec, globalTimeout common.Timeout, globalOutputSizeLimit common.OutputSizeLimit, groupName string) (*RuntimeCommand, error) {
    if spec == nil {
        return nil, ErrNilSpec
    }

    commandTimeout := common.NewFromIntPtr(spec.Timeout)
    effectiveTimeout, resolutionContext := common.ResolveTimeout(
        commandTimeout,
        common.NewUnsetTimeout(),
        globalTimeout,
        spec.Name,
        groupName,
    )

    commandOutputSizeLimit := common.NewOutputSizeLimitFromPtr(spec.OutputSizeLimit)
    effectiveOutputSizeLimit := common.ResolveOutputSizeLimit(
        commandOutputSizeLimit,
        globalOutputSizeLimit,
    )

    return &RuntimeCommand{
        Spec:                     spec,
        timeout:                  commandTimeout,
        ExpandedArgs:             []string{},
        ExpandedEnv:              make(map[string]string),
        ExpandedVars:             make(map[string]string),
        ExpandedArrayVars:        make(map[string][]string), // NEW
        EffectiveTimeout:         effectiveTimeout,
        TimeoutResolution:        resolutionContext,
        EffectiveOutputSizeLimit: effectiveOutputSizeLimit,
    }, nil
}
```

## 2. テストケース仕様

### 2.1 ProcessVars 単体テスト

```go
func TestProcessVars(t *testing.T) {
    tests := []struct {
        name               string
        vars               map[string]interface{}
        baseExpandedVars   map[string]string
        baseExpandedArrays map[string][]string
        level              string
        wantStrings        map[string]string
        wantArrays         map[string][]string
        wantErr            bool
        wantErrType        interface{} // for errors.As
    }{
        // Basic string variable
        {
            name:               "basic string variable",
            vars:               map[string]interface{}{"base_dir": "/opt/myapp"},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantStrings:        map[string]string{"base_dir": "/opt/myapp"},
            wantArrays:         map[string][]string{},
        },
        // Basic array variable
        {
            name:               "basic array variable",
            vars:               map[string]interface{}{"files": []interface{}{"a.txt", "b.txt"}},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantStrings:        map[string]string{},
            wantArrays:         map[string][]string{"files": {"a.txt", "b.txt"}},
        },
        // Variable expansion in string
        {
            name: "variable expansion in string",
            vars: map[string]interface{}{"config_path": "%{base_dir}/config.yml"},
            baseExpandedVars: map[string]string{"base_dir": "/opt/myapp"},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantStrings:        map[string]string{
                "base_dir":    "/opt/myapp",
                "config_path": "/opt/myapp/config.yml",
            },
            wantArrays: map[string][]string{},
        },
        // Variable expansion in array elements
        {
            name: "variable expansion in array elements",
            vars: map[string]interface{}{
                "files": []interface{}{"%{base_dir}/a.txt", "%{base_dir}/b.txt"},
            },
            baseExpandedVars:   map[string]string{"base_dir": "/opt"},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantStrings:        map[string]string{"base_dir": "/opt"},
            wantArrays:         map[string][]string{"files": {"/opt/a.txt", "/opt/b.txt"}},
        },
        // Empty array (allowed)
        {
            name:               "empty array allowed",
            vars:               map[string]interface{}{"files": []interface{}{}},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantStrings:        map[string]string{},
            wantArrays:         map[string][]string{"files": {}},
        },
        // Override string with same type (allowed)
        {
            name:               "override string with string",
            vars:               map[string]interface{}{"base_dir": "/var/myapp"},
            baseExpandedVars:   map[string]string{"base_dir": "/opt/myapp"},
            baseExpandedArrays: map[string][]string{},
            level:              "group[test]",
            wantStrings:        map[string]string{"base_dir": "/var/myapp"},
            wantArrays:         map[string][]string{},
        },
        // Type mismatch: string -> array (error)
        {
            name:               "type mismatch string to array",
            vars:               map[string]interface{}{"base_dir": []interface{}{"/opt"}},
            baseExpandedVars:   map[string]string{"base_dir": "/opt/myapp"},
            baseExpandedArrays: map[string][]string{},
            level:              "group[test]",
            wantErr:            true,
            wantErrType:        &ErrTypeMismatchDetail{},
        },
        // Type mismatch: array -> string (error)
        {
            name:               "type mismatch array to string",
            vars:               map[string]interface{}{"files": "/opt/file.txt"},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{"files": {"a.txt"}},
            level:              "group[test]",
            wantErr:            true,
            wantErrType:        &ErrTypeMismatchDetail{},
        },
        // Unsupported type (error)
        {
            name:               "unsupported type int",
            vars:               map[string]interface{}{"count": int64(42)},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrUnsupportedTypeDetail{},
        },
        // Invalid array element type (error)
        {
            name:               "invalid array element type",
            vars:               map[string]interface{}{"mixed": []interface{}{"a", 42}},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrInvalidArrayElementDetail{},
        },
        // Too many variables (error)
        {
            name:               "too many variables",
            vars:               generateLargeVarsMap(MaxVarsPerLevel + 1),
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrTooManyVariablesDetail{},
        },
        // Array too large (error)
        {
            name:               "array too large",
            vars:               map[string]interface{}{"large": generateLargeArray(MaxArrayElements + 1)},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrArrayTooLargeDetail{},
        },
        // String value too long (error)
        {
            name:               "string value too long",
            vars:               map[string]interface{}{"long": strings.Repeat("a", MaxStringValueLen+1)},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrValueTooLongDetail{},
        },
        // Undefined variable reference (error)
        {
            name:               "undefined variable reference",
            vars:               map[string]interface{}{"path": "%{undefined}/file"},
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrUndefinedVariableDetail{},
        },
        // Circular reference (error)
        {
            name: "circular reference",
            vars: map[string]interface{}{
                "a": "%{b}",
                "b": "%{a}",
            },
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrCircularReferenceDetail{},
        },
        // Order-independent expansion (important for map iteration)
        // This test verifies that lazy expansion handles forward references correctly.
        // Since Go maps have non-deterministic iteration order, config_path may be
        // processed before base_dir. The varExpander's lazy resolution mechanism
        // should correctly expand config_path even if base_dir hasn't been processed yet.
        {
            name: "order independent expansion - forward reference",
            vars: map[string]interface{}{
                "config_path": "%{base_dir}/config.yml", // References base_dir (may be processed later)
                "base_dir":    "/opt",
            },
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantStrings: map[string]string{
                "base_dir":    "/opt",
                "config_path": "/opt/config.yml", // Should be correctly expanded via lazy resolution
            },
            wantArrays: map[string][]string{},
        },
        // Chained dependencies
        // This test verifies that lazy expansion handles multi-level dependencies correctly.
        // Variable c depends on b, which depends on a. Regardless of map iteration order,
        // all variables should be correctly expanded through lazy resolution and memoization.
        {
            name: "chained dependencies",
            vars: map[string]interface{}{
                "c": "%{b}/c",  // Depends on b
                "a": "/opt",
                "b": "%{a}/b",  // Depends on a
            },
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantStrings: map[string]string{
                "a": "/opt",
                "b": "/opt/b",      // Expanded via lazy resolution
                "c": "/opt/b/c",    // Expanded via lazy resolution (multi-level)
            },
            wantArrays: map[string][]string{},
        },
        // Array variable referenced in string context (error)
        {
            name: "array variable in string context",
            vars: map[string]interface{}{
                "files":  []interface{}{"a.txt", "b.txt"},
                "result": "%{files}",
            },
            baseExpandedVars:   map[string]string{},
            baseExpandedArrays: map[string][]string{},
            level:              "global",
            wantErr:            true,
            wantErrType:        &ErrArrayVariableInStringContextDetail{},
        },
        // Reference base string var from new var
        {
            name: "reference base string var from new var",
            vars: map[string]interface{}{
                "new_path": "%{base_dir}/new",
            },
            baseExpandedVars:   map[string]string{"base_dir": "/opt"},
            baseExpandedArrays: map[string][]string{},
            level:              "group[test]",
            wantStrings: map[string]string{
                "base_dir": "/opt",
                "new_path": "/opt/new",
            },
            wantArrays: map[string][]string{},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            gotStrings, gotArrays, err := ProcessVars(
                tt.vars,
                tt.baseExpandedVars,
                tt.baseExpandedArrays,
                tt.level,
            )

            if tt.wantErr {
                require.Error(t, err)
                if tt.wantErrType != nil {
                    assert.ErrorAs(t, err, tt.wantErrType)
                }
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tt.wantStrings, gotStrings)
            assert.Equal(t, tt.wantArrays, gotArrays)
        })
    }
}

// Helper functions for test data generation
func generateLargeVarsMap(n int) map[string]interface{} {
    result := make(map[string]interface{}, n)
    for i := 0; i < n; i++ {
        result[fmt.Sprintf("var_%d", i)] = "value"
    }
    return result
}

func generateLargeArray(n int) []interface{} {
    result := make([]interface{}, n)
    for i := 0; i < n; i++ {
        result[i] = "element"
    }
    return result
}
```

### 2.2 統合テスト

```go
func TestVarsTableFormatIntegration(t *testing.T) {
    tomlContent := `
version = "1.0"

[global.vars]
base_dir = "/opt/myapp"
env_type = "production"
config_files = ["%{base_dir}/config.yml", "%{base_dir}/secrets.yml"]

[[groups]]
name = "deploy"

[groups.vars]
deploy_target = "%{env_type}"
deploy_path = "/var/www/%{deploy_target}"

[[groups.commands]]
name = "backup"
cmd = "/bin/cp"

[groups.commands.vars]
backup_suffix = ".bak"
`

    // Load and expand
    loader := NewLoader()
    config, err := loader.LoadFromBytes([]byte(tomlContent))
    require.NoError(t, err)

    // Verify global expansion
    assert.Equal(t, "/opt/myapp", config.Global.ExpandedVars["base_dir"])
    assert.Equal(t, "production", config.Global.ExpandedVars["env_type"])
    assert.Equal(t, []string{
        "/opt/myapp/config.yml",
        "/opt/myapp/secrets.yml",
    }, config.Global.ExpandedArrayVars["config_files"])

    // Expand group and verify
    runtimeGroup, err := ExpandGroup(&config.Groups[0], config.Global)
    require.NoError(t, err)
    assert.Equal(t, "production", runtimeGroup.ExpandedVars["deploy_target"])
    assert.Equal(t, "/var/www/production", runtimeGroup.ExpandedVars["deploy_path"])
}
```

### 2.3 旧形式検出テスト

```go
func TestLegacyVarsFormatDetection(t *testing.T) {
    // This TOML uses the old array-based format
    legacyTOML := `
version = "1.0"

[global]
vars = ["base_dir=/opt/myapp", "env_type=production"]

[[groups]]
name = "test"
`

    loader := NewLoader()
    _, err := loader.LoadFromBytes([]byte(legacyTOML))

    require.Error(t, err)
    assert.Contains(t, err.Error(), "no longer supported")
    assert.Contains(t, err.Error(), "table format")
}
```

### 2.4 ベンチマークテスト

```go
func BenchmarkProcessVars(b *testing.B) {
    vars := map[string]interface{}{
        "base_dir":     "/opt/myapp",
        "env_type":     "production",
        "config_path":  "%{base_dir}/%{env_type}/config.yml",
        "config_files": []interface{}{"%{base_dir}/a.yml", "%{base_dir}/b.yml"},
    }
    baseVars := map[string]string{}
    baseArrays := map[string][]string{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _, _ = ProcessVars(vars, baseVars, baseArrays, "benchmark")
    }
}

func BenchmarkProcessVarsLargeArray(b *testing.B) {
    elements := make([]interface{}, 100)
    for i := 0; i < 100; i++ {
        elements[i] = fmt.Sprintf("/path/to/file_%d.txt", i)
    }
    vars := map[string]interface{}{"files": elements}
    baseVars := map[string]string{}
    baseArrays := map[string][]string{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _, _ = ProcessVars(vars, baseVars, baseArrays, "benchmark")
    }
}

func BenchmarkProcessVarsWithDependencies(b *testing.B) {
    // Test case with forward references to measure lazy expansion overhead
    vars := map[string]interface{}{
        "d": "%{c}/d",
        "c": "%{b}/c",
        "b": "%{a}/b",
        "a": "/opt",
    }
    baseVars := map[string]string{}
    baseArrays := map[string][]string{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _, _ = ProcessVars(vars, baseVars, baseArrays, "benchmark")
    }
}

func BenchmarkProcessVarsManyVariables(b *testing.B) {
    // Test with many variables but no dependencies
    vars := make(map[string]interface{}, 500)
    for i := 0; i < 500; i++ {
        vars[fmt.Sprintf("var_%d", i)] = fmt.Sprintf("value_%d", i)
    }
    baseVars := map[string]string{}
    baseArrays := map[string][]string{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _, _ = ProcessVars(vars, baseVars, baseArrays, "benchmark")
    }
}
```

## 3. 実装チェックリスト

### 3.1 Phase 1: 型定義の変更

- [ ] `spec.go`: `GlobalSpec.Vars` を `map[string]interface{}` に変更
- [ ] `spec.go`: `GroupSpec.Vars` を `map[string]interface{}` に変更
- [ ] `spec.go`: `CommandSpec.Vars` を `map[string]interface{}` に変更
- [ ] `runtime.go`: `RuntimeGlobal.ExpandedArrayVars` フィールド追加
- [ ] `runtime.go`: `RuntimeGroup.ExpandedArrayVars` フィールド追加
- [ ] `runtime.go`: `RuntimeCommand.ExpandedArrayVars` フィールド追加
- [ ] `runtime.go`: `NewRuntimeGlobal` で `ExpandedArrayVars` を初期化
- [ ] `runtime.go`: `NewRuntimeGroup` で `ExpandedArrayVars` を初期化
- [ ] `runtime.go`: `NewRuntimeCommand` で `ExpandedArrayVars` を初期化

### 3.2 Phase 2: 変数展開ロジックの更新

#### リファクタリング（セクション 1.4）
- [ ] `expansion.go`: `variableResolver` 型の定義
- [ ] `expansion.go`: `expandStringWithResolver` 関数の実装
- [ ] `expansion.go`: `expandStringRecursive` のリファクタリング（`expandStringWithResolver` を使用）

#### 新規実装（セクション 1.5, 1.6）
- [ ] `expansion.go`: 制限値定数の追加
- [ ] `errors.go`: 新規エラー型の追加
- [ ] `errors.go`: `ErrArrayVariableInStringContextDetail` エラー型の追加
- [ ] `expansion.go`: `varExpander` 構造体の追加
- [ ] `expansion.go`: `newVarExpander` コンストラクタの追加
- [ ] `expansion.go`: `varExpander.expandString` メソッドの追加
- [ ] `expansion.go`: `expandStringRecursive` 内部関数の更新
- [ ] `expansion.go`: `varExpander.resolveVariable` メソッドの追加
- [ ] `expansion.go`: `ProcessVars` のシグネチャ変更
- [ ] `expansion.go`: 型検証の実装（Phase 1: 検証とパース）
- [ ] `expansion.go`: サイズ検証の実装
- [ ] `expansion.go`: 型整合性検証の実装
- [ ] `expansion.go`: 配列変数の展開処理の実装（Phase 2: 遅延展開）

#### 既存関数の更新（セクション 1.7, 1.8, 1.9）
- [ ] `expansion.go`: `ExpandGlobal` の更新
- [ ] `expansion.go`: `ExpandGroup` の更新
- [ ] `expansion.go`: `ExpandCommand` の更新

### 3.3 Phase 3: 統合とテスト

- [ ] `loader.go`: 旧形式検出エラーの実装
- [ ] `expansion_test.go`: `ProcessVars` の単体テスト
- [ ] `expansion_test.go`: 型整合性のテスト
- [ ] `expansion_test.go`: サイズ制限のテスト
- [ ] `expansion_test.go`: 順序非依存展開のテスト（前方参照、連鎖依存）
- [ ] `expansion_test.go`: 配列変数の文字列コンテキスト参照エラーのテスト
- [ ] `expansion_test.go`: `varExpander` のテスト
- [ ] `loader_test.go`: 旧形式検出のテスト
- [ ] 統合テストの作成
- [ ] ベンチマークテストの作成

### 3.4 Phase 4: サンプルとドキュメント

- [ ] `sample/*.toml`: 全サンプルファイルの更新
- [ ] `cmd/runner/testdata/*.toml`: テストデータの更新
- [ ] `README.md`, `README.ja.md`: ドキュメント更新
- [ ] `CHANGELOG.md`: 変更履歴の追加

## 4. セキュリティ考慮事項

### 4.1 入力検証

| 検証項目 | 実装箇所 | 説明 |
|---------|---------|------|
| 変数名バリデーション | `validateVariableName()` | 英字/アンダースコアで始まり、英数字/アンダースコアのみ |
| 予約プレフィックス拒否 | `ValidateVariableName()` | `__runner_` で始まる変数名を拒否 |
| 型検証 | `ProcessVars()` Phase 1 | 文字列または文字列配列のみ許可 |
| サイズ制限 | `ProcessVars()` Phase 1 | 変数数、配列要素数、文字列長 |
| 循環依存検出 | `expandStringWithResolver()` | visited マップで検出 |
| 展開深度制限 | `expandStringWithResolver()` | `MaxRecursionDepth` (100) |

### 4.2 機密情報漏洩防止

変数には機密情報（パスワード、APIキー、トークンなど）が含まれる可能性があるため、
以下のガイドラインに従って機密情報の漏洩を防止する：

#### 4.2.1 エラーメッセージ

エラーメッセージには以下の情報のみを含める：

**含めて良い情報**:
- 変数名（値は含まない）
- レベル情報（global, group[name], command[name]）
- 型情報（期待型と実際の型）
- インデックス情報（配列の場合）

**含めてはいけない情報**:
- 変数の実際の値（展開前、展開後ともに）
- 変数値を含むコンテキスト文字列
- ファイルシステムの内部パス
- 環境変数の値

#### 4.2.2 ログ出力

**原則**: 変数の値は絶対にログに出力しない

**ログに含めて良い情報**:
- 変数名
- 変数の型（string, array）
- 展開状態（expanded, pending, failed）
- エラーの種類
- 変数の個数

**ログに含めてはいけない情報**:
- 変数の値（展開前、展開後ともに）
- 部分的な値（プレフィックス、サフィックスも禁止）

詳細は「セクション 5.2 ログ出力」を参照。

### 4.3 リソース制限

| リソース | 制限値 | 定数名 | 根拠 |
|---------|--------|--------|------|
| 変数数/レベル | 1000 | `MaxVarsPerLevel` | メモリ使用量を制限 |
| 配列要素数/変数 | 1000 | `MaxArrayElements` | 展開処理時間を制限 |
| 文字列長/値 | 10KB | `MaxStringValueLen` | メモリ使用量を制限 |
| 展開深度 | 100 | `MaxRecursionDepth` | スタックオーバーフロー防止 |

### 4.4 変数値のセキュリティ検証

変数値に対するセキュリティ検証（コマンドインジェクション、パストラバーサル等）は **vars定義時には実施しない**。
これらは最終的な使用時点（cmd, args, env等への展開時）で既存の検証ロジックにより実施される：

- `ValidateEnvironmentValue`: 環境変数として使用する場合
- `validateCommandPath`: コマンドパスとして使用する場合
- `ValidateOutputPath`: 出力パスとして使用する場合

この設計により、コンテキスト依存の検証を適切な場所で実施し、DRY原則を維持する。

## 5. 運用考慮事項 (SRE)

### 5.1 監視項目

| メトリクス | 説明 | アラート閾値 |
|-----------|------|-------------|
| 設定ファイル読み込み時間 | P50, P95, P99 | P99 > 500ms |
| 変数展開エラー率 | エラー数 / 総リクエスト数 | > 1% |
| 制限値到達イベント | 各制限値への到達回数 | > 0 (warning) |

### 5.2 ログ出力

| レベル | イベント | 出力内容 | セキュリティ制約 |
|--------|---------|---------|-----------------|
| INFO | 設定ファイル読み込み成功 | ファイルパス、変数数 | - |
| DEBUG | 変数展開処理 | 変数名、型、展開状態 | **値は出力禁止** |
| ERROR | 変数展開エラー | レベル、フィールド、エラー種別 | **値は出力禁止** |
| WARN | 制限値接近 | 変数数が閾値の80%を超えた場合 | - |

**セキュリティ要件（機密情報漏洩防止）**:

変数の**値**は絶対にログに出力してはならない。理由:
- 変数には機密情報（パスワード、APIキー、トークンなど）が含まれる可能性がある
- ログファイルは複数の管理者がアクセス可能であり、機密情報の漏洩リスクがある
- デバッグ目的でも、値のログ出力は禁止

**ログに含めて良い情報**:
- 変数名（例: `api_token`, `db_password`）
- 変数の型（`string`, `array`）
- 展開状態（`expanded`, `pending`, `failed`）
- エラーの種類（`circular_reference`, `undefined_variable`, `type_mismatch`）
- 変数の個数（例: `10 variables expanded`）

**ログに含めてはいけない情報**:
- 変数の値（展開前、展開後ともに禁止）
- 変数値を含むエラーコンテキスト
- 部分的な値（プレフィックス、サフィックスも禁止）

**実装上の注意**:
- エラー型の `Context` フィールドには値を含めない
- デバッグログでは変数名と状態のみを出力
- 既存の `internal/redaction` パッケージとの統合は不要（値を出力しないため）

### 5.3 トラブルシューティング

| 症状 | 可能性のある原因 | 対処法 |
|------|------------------|--------|
| `vars array format is no longer supported` | 旧形式の設定ファイル | テーブル形式に移行 |
| `circular reference detected` | 変数間の循環参照 | 依存関係を見直し |
| `undefined variable` | 未定義変数への参照 | 変数定義を追加、スペルミス確認 |
| `unsupported type` | 非文字列/非配列の値 | 値を文字列または文字列配列に修正 |
| `type mismatch` | 継承変数の型変更 | 同じ型で上書きするか、別名を使用 |
| `array variable in string context` | 配列変数を文字列コンテキストで参照 | 文字列変数を使用するか、配列展開構文を確認 |

**注意**: TOMLパーサーの制限により、エラーメッセージには正確な行番号が含まれない場合があります。エラーが発生したレベル（`global`, `group[name]`, `command[name]`）と変数名を特定の手がかりとしてください。

### 5.4 パフォーマンス特性

- **時間計算量**: O(n * m) - n は変数数、m は平均依存数
- **空間計算量**: O(n) - 展開済み変数のメモリ使用量
- **遅延展開**: 各変数は最大1回のみ展開される（メモ化による最適化）

通常のユースケース（変数数 < 100）では、展開処理は数ミリ秒以内に完了する。

## 6. 参照

- 要件定義書: `01_requirements.md`
- アーキテクチャ設計書: `02_architecture.md`
- Task 0026: 変数展開機能の実装
- Task 0033: vars/env の分離
- Task 0062: コマンドテンプレート機能（配列変数のユースケース）
