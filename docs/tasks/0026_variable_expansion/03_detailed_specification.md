# 詳細仕様書: コマンド・引数内環境変数展開機能

## 1. 実装詳細仕様

### 1.1 パッケージ構成詳細

```
# 既存コンポーネント拡張
internal/runner/environment/processor.go  # CommandEnvProcessor.Expand による変数展開
internal/runner/security/validator.go    # 既存のセキュリティ検証を活用
internal/runner/config/validator.go      # コマンド設定の検証
```

### 1.2 型定義とインターフェース

#### 1.2.1 CommandEnvProcessor の直接使用

```go
package environment

// CommandEnvProcessor は環境変数処理を担当し、cmd/args の変数展開を直接実行する
type CommandEnvProcessor struct {
    filter *Filter
    logger *slog.Logger
}

// Expand は変数を展開し、エスケープ処理と循環参照検出を含む
func (p *CommandEnvProcessor) Expand(
    value string,
    envVars map[string]string,
    allowlist []string,
    groupName string,
    visited map[string]bool,
) (string, error)

// ProcessCommandEnvironment はコマンド環境の処理を行う
func (p *CommandEnvProcessor) ProcessCommandEnvironment(
    cmd runnertypes.Command,
    baseEnvVars map[string]string,
    group *runnertypes.CommandGroup,
) (map[string]string, error)

```

#### 1.2.2 シンプルなエラー定義 (types.go に統合)

```go
// 既存の Environment Package のエラーを活用
var (
    // 既存の ErrCircularReference を流用
    ErrCircularReference = environment.ErrCircularReference

    // 既存の Security エラーを活用
    ErrVariableNotAllowed = environment.ErrVariableNotAllowed
    ErrVariableNotFound   = environment.ErrVariableNotFound

    // 新しいエラー型
    ErrInvalidEscapeSequence = errors.New("invalid escape sequence")
    ErrInvalidVariableFormat = errors.New("invalid variable format")
)

// シンプルなエラーファクトリ
func NewExpansionError(message string, cause error) error {
    if cause != nil {
        return fmt.Errorf("%s: %w", message, cause)
    }
    return errors.New(message)
}

// 既存の errors.Is でエラー判定が可能
func IsCircularReferenceError(err error) bool {
    return errors.Is(err, ErrCircularReference)
}

func IsSecurityViolationError(err error) bool {
    return errors.Is(err, ErrVariableNotAllowed)
}

func IsVariableNotFoundError(err error) bool {
    return errors.Is(err, ErrVariableNotFound)
}

func IsInvalidEscapeSequenceError(err error) bool {
    return errors.Is(err, ErrInvalidEscapeSequence)
}

func IsInvalidVariableFormatError(err error) bool {
    return errors.Is(err, ErrInvalidVariableFormat)
}
```

### 1.3 エスケープシーケンス仕様

#### 1.3.1 エスケープ機能の設計

**エスケープ仕様**:
- `\$` → `$` (リテラル) - 変数展開を抑制
- `\\` → `\` (リテラル) - バックスラッシュのエスケープ
- `\` + (その他の文字) → `ErrInvalidEscapeSequence` エラー
- 文字列末尾の `\` → `ErrInvalidEscapeSequence` エラー

**実装アルゴリズム**:
文字列を1文字ずつスキャンし、エスケープ状態を追跡する方式を採用：

1. 入力文字列を左から右へ1文字ずつスキャン
2. `\` が見つかった場合:
   - 次の文字が `$` または `\` であれば、次の文字をリテラルとして結果に追加
   - 次の文字がそれ以外、または文字列の終端である場合は、`ErrInvalidEscapeSequence` エラーを返す
3. `$` が見つかった場合（ただし、`\` でエスケープされていない）:
   - 既存の正規表現を使用して変数パターンをマッチング
   - マッチした場合は変数を展開、未定義変数はエラー
   - 正規表現にマッチしない、たとえば `$` の次が `{` ではない、もしくは対応する `}` がない場合にはエラーを返す
4. その他の文字はそのまま結果に追加

**新しいエラー型**:
```go
// ErrInvalidEscapeSequence is returned when an invalid escape sequence is detected
var ErrInvalidEscapeSequence = errors.New("invalid escape sequence")

// ErrInvalidVariableFormat is returned when $ is found but not followed by valid variable syntax
var ErrInvalidVariableFormat = errors.New("invalid variable format")
```

### 1.4 変数展開仕様 (${VAR} 形式のみ)

#### 1.4.1 採用形式

変数参照形式は安全で明示的な `${VAR}` のみをサポートする。

理由:
- `${...}` はトークン境界が明確で曖昧さがない
- エスケープ仕様 (`\$` → `$`) と衝突しにくい
- 既存のCommand.Envとの一貫性を保持
- 実装の複雑性とメンテナンスコストを最小化

#### 1.4.2 エラー優先順位

展開時のエラーは以下の優先順位で決定される:
1. エスケープ/構文エラー (`ErrInvalidEscapeSequence`, `ErrInvalidVariableFormat`)
2. 変数名エラー (`ErrInvalidVariableName`)
3. 循環参照検出 (`ErrCircularReference`)
4. アクセス不許可 (`ErrVariableNotAllowed`) ※ システム環境に存在するが allowlist 外
5. 未定義 (`ErrVariableNotFound`)

#### 1.4.3 実装メモ
- 再帰中の訪問集合は map を再利用し、深さ展開後に delete することで追加割当を削減
- `${VAR}` のネスト展開結果は都度再帰呼び出しで構築し、逐次的に `strings.Builder` へ書き込む
- コマンド定義内の変数相互参照を許容するため 2 パス処理 (先に未展開値投入→後段で展開) を採用

### 1.5 既存セキュリティ検証とのシンプルな統合

#### 1.5.1 既存 Security Validator をそのまま活用

```go
package expansion

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// 既存の Environment Processor と Security Validator をそのまま活用
type SecurityValidator struct {
    envProcessor *environment.CommandEnvProcessor
    validator    *security.Validator
}

// NewSecurityValidator は既存コンポーネントを組み合わせ
func NewSecurityValidator(envProcessor *environment.CommandEnvProcessor, validator *security.Validator) *SecurityValidator {
    return &SecurityValidator{
        envProcessor: envProcessor,
        validator:    validator,
    }
}

// ValidateAndExpand は既存のロジックを直接活用
func (sv *SecurityValidator) ValidateAndExpand(
    text string,
    envVars map[string]string,
    group *runnertypes.CommandGroup,
) (string, error) {
    // Expand メソッドを使用して ${VAR} 形式の変数展開を実行
    // ここで ${VAR} 形式が一貫してサポートされる
    return sv.envProcessor.Expand(text, envVars, group, make(map[string]bool))
}

// 既存の allowlist 検証、Command.Env 優先ポリシーはそのまま使用
// 新しいコードを書かずに既存の実績あるコードを活用
```

### 1.5 循環参照検出仕様（visited mapアルゴリズムを採用）

#### 1.5.1 visited mapによる循環参照検出

```go
// Expand は変数を展開し、エスケープ処理を含む1文字スキャン方式で実装
func (p *CommandEnvProcessor) Expand(
    value string,
    envVars map[string]string,
    allowlist []string,
    groupName string,
    visited map[string]bool,
) (string, error) {
    var result strings.Builder
    runes := []rune(value)
    i := 0

    for i < len(runes) {
        switch runes[i] {
        case '\\':
            // エスケープシーケンス処理
            if i+1 >= len(runes) {
                return "", ErrInvalidEscapeSequence
            }
            nextChar := runes[i+1]
            if nextChar == '$' || nextChar == '\\' {
                result.WriteRune(nextChar)
                i += 2
            } else {
                return "", fmt.Errorf("%w: \\%c", ErrInvalidEscapeSequence, nextChar)
            }

        case '$':
            // ${VAR}形式のみをサポート
            if i+1 >= len(runes) || runes[i+1] != '{' {
                return "", ErrInvalidVariableFormat
            }

            start := i + 2
            end := -1
            for j := start; j < len(runes); j++ {
                if runes[j] == '}' {
                    end = j
                    break
                }
            }
            if end == -1 {
                return "", ErrUnclosedVariable
            }

            varName := string(runes[start:end])
            if err := security.ValidateVariableName(varName); err != nil {
                return "", fmt.Errorf("%w: %s: %w", ErrInvalidVariableName, varName, err)
            }

            if visited[varName] {
                return "", fmt.Errorf("%w: %s", ErrCircularReference, varName)
            }

            visited[varName] = true

            // 変数の値を取得（local -> system の順）
            val, foundLocal := envVars[varName]
            var valStr string
            var found bool

            if foundLocal {
                valStr, found = val, true
            } else {
                sysVal, foundSys := os.LookupEnv(varName)
                if foundSys {
                    // Create temporary CommandGroup for filter compatibility
                    tempGroup := &runnertypes.CommandGroup{
                        EnvAllowlist: allowlist,
                        Name:         groupName,
                    }
                    if !p.filter.IsVariableAccessAllowed(varName, tempGroup) {
                        return "", fmt.Errorf("%w: %s", ErrVariableNotAllowed, varName)
                    }
                    valStr, found = sysVal, true
                }
            }

            if !found {
                return "", fmt.Errorf("%w: %s", ErrVariableNotFound, varName)
            }

            // 再帰的に展開
            expanded, err := p.Expand(valStr, envVars, allowlist, groupName, visited)
            if err != nil {
                return "", fmt.Errorf("failed to expand nested variable ${%s}: %w", varName, err)
            }

            result.WriteString(expanded)
            delete(visited, varName)
            i = end + 1

        default:
            result.WriteRune(runes[i])
            i++
        }
    }

    return result.String(), nil
}
```

### 1.6 cmd/args 用変数展開の実装

#### 1.6.1 CommandEnvProcessor の直接使用

cmd/args の変数展開には `CommandEnvProcessor.Expand` メソッドを直接使用する。

主な特徴:
- 既存の実績あるコードを最大限活用
- thin wrapper を避けたシンプルな設計
- 直接的で理解しやすい実装

```go
package config

import (
    "fmt"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// CommandEnvProcessor を直接使用した変数展開
func expandVariables(processor *environment.CommandEnvProcessor, text string, envVars map[string]string, allowlist []string, groupName string) (string, error) {
    // CommandEnvProcessor.Expand を直接使用（シンプルなAPI）
    return processor.Expand(text, envVars, allowlist, groupName, make(map[string]bool))
}

// 複数文字列の一括展開（ユーティリティ関数）
func expandAll(processor *environment.CommandEnvProcessor, texts []string, envVars map[string]string, allowlist []string, groupName string) ([]string, error) {
    if len(texts) == 0 {
        return texts, nil
    }

    result := make([]string, len(texts))
    for i, text := range texts {
        expanded, err := expandVariables(processor, text, envVars, allowlist, groupName)
        if err != nil {
            return nil, fmt.Errorf("failed to expand text[%d]: %w", i, err)
        }
        result[i] = expanded
    }
    return result, nil
}
```

### 1.7 設定統合仕様

#### 1.7.1 Config Parser統合 (internal/runner/config/command.go への追加)

```go
// Command構造体への変数展開統合
func (c *Command) ExpandVariables(processor *environment.CommandEnvProcessor, allowlist []string, groupName string) error {
    // 環境変数マップを構築
    env, err := c.BuildEnvironmentMap()
    if err != nil {
        return fmt.Errorf("failed to build environment map: %w", err)
    }

    // コマンド名の展開（シンプルなAPI）
    expandedCmd, err := processor.Expand(c.Cmd, env, allowlist, groupName, make(map[string]bool))
    if err != nil {
        return fmt.Errorf("failed to expand command: %w", err)
    }
    c.Cmd = expandedCmd

    // 引数の展開
    expandedArgs, err := expandAll(processor, c.Args, env, allowlist, groupName)
    if err != nil {
        return fmt.Errorf("failed to expand args: %w", err)
    }
    c.Args = expandedArgs

    return nil
}

// BuildEnvironmentMap は環境変数マップを構築
func (c *Command) BuildEnvironmentMap() (map[string]string, error) {
    env := make(map[string]string)

    for _, envVar := range c.Env {
        parts := strings.SplitN(envVar, "=", 2)
        if len(parts) != 2 {
            return nil, fmt.Errorf("invalid environment variable format: %s", envVar)
        }
        env[parts[0]] = parts[1]
    }

    return env, nil
}
```

### 1.8 テストケース仕様

#### 1.8.1 単体テストケース

```go
// expansion_test.go

package expansion

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestVariableParser_ReplaceVariables(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        env       map[string]string
        expected  string
        expectErr bool
    }{
        {
            name:     "simple variable",
            input:    "${{HOME}",
            env:      map[string]string{"HOME": "/home/user"},
            expected: "/home/user",
            expectErr: false,
        },
        {
            name:     "braced variable",
            input:    "${{USER}",
            env:      map[string]string{"USER": "testuser"},
            expected: "testuser",
            expectErr: false,
        },
        {
            name:     "multiple variables",
            input:    "${HOME}/bin/${APP_NAME}",
            env:      map[string]string{"HOME": "/home/user", "APP_NAME": "myapp"},
            expected: "/home/user/bin/myapp",
            expectErr: false,
        },
        {
            name:     "prefix_${VAR}_suffix clear case",
            input:    "prefix_${HOME}_suffix",
            env:      map[string]string{"HOME": "user"},
            expected: "prefix_user_suffix", // 明確な変数境界
            expectErr: false,
        },
        {
            name:     "JSON with variable - braced format processing",
            input:    `{"key": "${VALUE}"}`,
            env:      map[string]string{"VALUE": "test"},
            expected: `{"key": "test"}`, // ${VAR}形式でJSON内の${VALUE}が正しく展開
            expectErr: false,
        },
        {
            name:     "multiple braced variables in JSON",
            input:    `{"user": "${USER}", "home": "${HOME}"}`,
            env:      map[string]string{"USER": "testuser", "HOME": "/home/testuser"},
            expected: `{"user": "testuser", "home": "/home/testuser"}`, // 複数の${VAR}変数
            expectErr: false,
        },
        {
            name:     "braced pattern handles complex cases",
            input:    "before_${VAR}_middle_${VAR2}_after",
            env:      map[string]string{"VAR": "value1", "VAR2": "value2"},
            expected: "before_value1_middle_value2_after", // 複数の${VAR}変数処理
            expectErr: false,
        },
        {
            name:     "similar variable names with clear boundaries",
            input:    "${HOME} and ${HOME_DIR} and ${HOME_SUFFIX}",
            env:      map[string]string{"HOME": "user", "HOME_DIR": "/home/user", "HOME_SUFFIX": "fallback"},
            expected: "user and /home/user and fallback", // 明確な変数境界
            expectErr: false,
        },
        {
            name:     "standard braced format",
            input:    "prefix_${HOME}_suffix",
            env:      map[string]string{"HOME": "user"},
            expected: "prefix_user_suffix",
            expectErr: false,
        },
        {
            name:     "glob patterns as literals",
            input:    "${{HOME}/*.txt",
            env:      map[string]string{"HOME": "/home/user"},
            expected: "/home/user/*.txt", // * はリテラル文字として扱われる
            expectErr: false,
        },
        {
            name:     "no variables",
            input:    "/usr/bin/ls",
            env:      map[string]string{},
            expected: "/usr/bin/ls",
            expectErr: false,
        },
        {
            name:     "undefined variable",
            input:    "${UNDEFINED",
            env:      map[string]string{},
            expected: "",
            expectErr: false,
        },
        // エスケープシーケンステスト
        {
            name:     "escape dollar sign",
            input:    `\$FOO`,
            env:      map[string]string{"FOO": "value"},
            expected: "$FOO",
            expectErr: false,
        },
        {
            name:     "escape backslash",
            input:    `\\FOO`,
            env:      map[string]string{},
            expected: `\FOO`,
            expectErr: false,
        },
        {
            name:     "escaped dollar with variable expansion",
            input:    `\$FOO and ${BAR}`,
            env:      map[string]string{"FOO": "foo", "BAR": "bar"},
            expected: "$FOO and bar",
            expectErr: false,
        },
        {
            name:     "backslash before variable",
            input:    `\\${FOO}`,
            env:      map[string]string{"FOO": "value"},
            expected: `\value`,
            expectErr: false,
        },
        {
            name:     "multiple escapes",
            input:    `\$FOO \$BAR \\baz`,
            env:      map[string]string{"FOO": "f", "BAR": "b"},
            expected: "$FOO $BAR \\baz",
            expectErr: false,
        },
        {
            name:     "escaped in braces context",
            input:    `\${FOO} ${BAR}`,
            env:      map[string]string{"FOO": "foo", "BAR": "bar"},
            expected: "${FOO} bar",
            expectErr: false,
        },
        {
            name:     "invalid escape sequence - letter",
            input:    `\U`,
            env:      map[string]string{},
            expected: "",
            expectErr: true,
        },
        {
            name:     "invalid escape sequence - number",
            input:    `\1`,
            env:      map[string]string{},
            expected: "",
            expectErr: true,
        },
        {
            name:     "trailing backslash",
            input:    `FOO\`,
            env:      map[string]string{},
            expected: "",
            expectErr: true,
        },
        // 無効な変数形式のテストケース
        {
            name:     "dollar without braces",
            input:    "$HOME",
            env:      map[string]string{"HOME": "/home/user"},
            expected: "",
            expectErr: true,
        },
        {
            name:     "dollar at end",
            input:    "path$",
            env:      map[string]string{},
            expected: "",
            expectErr: true,
        },
        {
            name:     "unclosed brace",
            input:    "${HOME",
            env:      map[string]string{"HOME": "/home/user"},
            expected: "",
            expectErr: true,
        },
        {
            name:     "dollar with invalid character",
            input:    "$@INVALID",
            env:      map[string]string{},
            expected: "",
            expectErr: true,
        },
        {
            name:     "mixed valid and invalid formats",
            input:    "${HOME} and $USER",
            env:      map[string]string{"HOME": "/home/user", "USER": "testuser"},
            expected: "",
            expectErr: true,
        },
    }

    // テスト用のResolver実装
    parser := NewVariableParser()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resolver := &testVariableResolver{env: tt.env}
            result, err := parser.ReplaceVariables(tt.input, resolver)

            if tt.expectErr {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}

// testVariableResolver はテスト用のVariableResolver実装
type testVariableResolver struct {
    env map[string]string
}

func (r *testVariableResolver) ResolveVariable(name string) (string, error) {
    if value, exists := r.env[name]; exists {
        return value, nil
    }
    return "", fmt.Errorf("%w: %s", ErrVariableNotAllowed, varName) // 未定義変数はエラーとして扱う
}

func TestVariableExpander_Expand(t *testing.T) {
    tests := []struct {
        name      string
        text      string
        env       map[string]string
        allowlist []string
        expected  string
        expectErr bool
    }{
        {
            name: "simple ${VAR} expansion",
            text:  "${DOCKER_CMD",
            env:  map[string]string{"DOCKER_CMD": "/usr/bin/docker"},
            allowlist: []string{},
            expected:  "/usr/bin/docker",
            expectErr: false,
        },
        {
            name: "braced ${VAR} expansion",
            text:  "${{TOOL_DIR}/script",
            env:  map[string]string{"TOOL_DIR": "/opt/tools"},
            allowlist: []string{},
            expected:  "/opt/tools/script",
            expectErr: false,
        },
        {
            name: "multiple braced format expansion",
            text:  "${HOME}/${USER}_config",
            env:  map[string]string{"HOME": "/home/user", "USER": "testuser"},
            allowlist: []string{},
            expected:  "/home/user/testuser_config",
            expectErr: false,
        },
        {
            name: "circular reference detection",
            text:  "${A",
            env:  map[string]string{"A": "${B}", "B": "${A}"},
            allowlist: []string{},
            expected:  "",
            expectErr: true,
        },
        {
            name: "glob pattern preserved as literal",
            text:  "${{FIND_CMD}",
            env:  map[string]string{"FIND_CMD": "/usr/bin/find /path/*.txt"},
            allowlist: []string{},
            expected:  "/usr/bin/find /path/*.txt", // * はリテラルとして保持
            expectErr: false,
        },
    }

    // 既存の CommandEnvProcessor を使用したエンジンを作成
    envProcessor := environment.NewCommandEnvProcessor(nil) // 実際のテストでは適切なfilterを渡す
    expander := NewVariableExpander(envProcessor)
    ctx := context.Background()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := expander.Expand(ctx, tt.text, tt.env, tt.allowlist)
            if tt.expectErr {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}

func TestVariableExpander_ExpandAll(t *testing.T) {
    tests := []struct {
        name      string
        texts     []string
        env       map[string]string
        allowlist []string
        expected  []string
        expectErr bool
    }{
        {
            name: "expand multiple texts",
            texts: []string{"${HOME}/bin", "${USER}.log", "prefix_${APP}_suffix"},
            env: map[string]string{
                "HOME": "/home/user",
                "USER": "testuser",
                "APP": "myapp",
            },
            allowlist: []string{},
            expected: []string{"/home/user/bin", "testuser.log", "prefix_myapp_suffix"},
            expectErr: false,
        },
        {
            name: "empty list",
            texts: []string{},
            env: map[string]string{},
            allowlist: []string{},
            expected: []string{},
            expectErr: false,
        },
        {
            name: "undefined variable in second text",
            texts: []string{"${HOME}", "${UNDEFINED}"},
            env: map[string]string{"HOME": "/home/user"},
            allowlist: []string{},
            expected: []string{"/home/user", ""},
            expectErr: false,
        },
    }

    // 既存の CommandEnvProcessor を使用したエンジンを作成
    envProcessor := environment.NewCommandEnvProcessor(nil) // 実際のテストでは適切なfilterを渡す
    expander := NewVariableExpander(envProcessor)
    ctx := context.Background()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := expander.ExpandAll(ctx, tt.texts, tt.env, tt.allowlist)
            if tt.expectErr {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}

func TestCircularReferenceDetection_IterativeBased(t *testing.T) {
    tests := []struct {
        name      string
        env       map[string]string
        testValue string
        expectErr bool
    }{
        {
            name: "no circular reference - ${VAR} format",
            env: map[string]string{
                "A": "value_a",
                "B": "${A}",
                "C": "${B}/suffix",
            },
            testValue: "${C}",
            expectErr: false,
        },
        {
            name: "direct circular reference",
            env: map[string]string{
                "A": "${B}",
                "B": "${A}",
            },
            testValue: "${A}",
            expectErr: true, // visited mapで循環参照を検出
        },
        {
            name: "indirect circular reference",
            env: map[string]string{
                "A": "${B}",
                "B": "${C}",
                "C": "${A}",
            },
            testValue: "${A}",
            expectErr: true, // visited mapで循環参照を検出
        },
        {
            name: "self reference",
            env: map[string]string{
                "A": "${A}",
            },
            testValue: "${A}",
            expectErr: true,
        },
        {
            name: "JSON with variables - no false circular detection",
            env: map[string]string{
                "USER": "testuser",
                "HOME": "/home/testuser",
            },
            testValue: `{"user": "${USER}", "home": "${HOME}"}`,
            expectErr: false, // JSON内の変数が正しく処理される
        },
    }

    // 既存の Environment Processor を使用
    envProcessor := environment.NewCommandEnvProcessor(nil)
    expander := NewVariableExpander(envProcessor)
    ctx := context.Background()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := expander.Expand(ctx, tt.testValue, tt.env, []string{})

            if tt.expectErr {
                assert.Error(t, err)
                // 既存の ErrCircularReference が返されることを確認
                assert.True(t, IsCircularReferenceError(err), "Expected circular reference error")
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 1.9 性能仕様

#### 1.9.1 ベンチマークテスト

```go
func BenchmarkVariableExpansion(b *testing.B) {
    // ${VAR}形式専用のパフォーマンステスト
    parser := NewVariableParser()
    resolver := &testVariableResolver{
        env: map[string]string{
            "HOME": "/home/user",
            "BIN":  "/usr/bin",
            "APP":  "myapp",
            "PATTERN": "*.txt", // グロブパターンはリテラル扱い
        },
    }

    b.Run("braced_pattern_simple", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _, err := parser.ReplaceVariables("${HOME}/bin/${APP}", resolver)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("braced_pattern_multiple", func(b *testing.B) {
        testString := "--input ${HOME}/data --output ${BIN}/output"
        for i := 0; i < b.N; i++ {
            _, err := parser.ReplaceVariables(testString, resolver)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("braced_pattern_complex", func(b *testing.B) {
        // ${VAR}形式で複雑なケースを処理
        testString := "prefix_${APP}_suffix and ${HOME}/*.txt"
        for i := 0; i < b.N; i++ {
            _, err := parser.ReplaceVariables(testString, resolver)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("braced_pattern_json_case", func(b *testing.B) {
        // JSONケースでのパフォーマンス確認
        jsonString := `{"user": "${USER}", "home": "${HOME}", "pattern": "${PATTERN}"}`
        resolver.env["USER"] = "testuser" // テスト用に追加
        for i := 0; i < b.N; i++ {
            result, err := parser.ReplaceVariables(jsonString, resolver)
            if err != nil {
                b.Fatal(err)
            }
            // 結果が正しく展開されることを確認
            expected := `{"user": "testuser", "home": "/home/user", "pattern": "*.txt"}`
            if result != expected {
                b.Fatalf("Expected '%s', got '%s'", expected, result)
            }
        }
    })
}
```

### 1.10 シンプルな統合仕様

#### 1.10.1 既存コードを活用した統合ポイント

1. **Environment Processor 拡張**: 既存の `processor.go` に visited mapによる循環参照検出を実装
2. **Config Parser 統合**: シンプルな cmd/args 展開処理を追加
3. **Security Validator 活用**: 既存の allowlist 検証と Command.Env 優先ポリシーをそのまま使用
4. **Error Handling 一元化**: 既存のエラー型を流用して統一性を維持

#### 1.10.2 互換性保証とシンプルな実装

- **完全互換性**: 環境変数参照のない設定ファイルは無変更で動作
- **既存コード維持**: Command.Env 処理は実績ある既存コードを拡張のみ
- **直感的な実装**: 複雑なDFSではなく理解しやすいvisited mapによる循環参照検出
- **保守性重視**: 新しい開発者でも簡単に理解・修正可能なコード

このシンプルなアプローチにより、既存の実績あるコードを最大限活用しつつ、要件を満たす堅牢で高性能な変数展開機能を実現できます。
