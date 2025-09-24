# 詳細仕様書: コマンド・引数内環境変数展開機能

## 1. 実装詳細仕様

### 1.1 パッケージ構成詳細

```
internal/runner/expansion/
├── expander.go          # 統合展開エンジン（cmd/args 用）
├── parser.go           # 両形式対応パーサー（$VAR, ${VAR}）
├── types.go           # シンプルな型定義
└── expansion_test.go  # 統合テスト

# 既存コンポーネント拡張
internal/runner/environment/processor.go  # 両形式サポートと反復上限拡張
internal/runner/security/validator.go    # 既存のセキュリティ検証を活用
```

### 1.2 型定義とインターフェース

#### 1.2.1 シンプルなコア型定義 (types.go)

```go
package expansion

import (
    "context"
)

// VariableExpander は cmd/args 用のシンプルな展開インターフェース
type VariableExpander interface {
    // 既存の反復制限方式を使用したシンプルな展開
    Expand(ctx context.Context, text string, env map[string]string, allowlist []string) (string, error)
    ExpandAll(ctx context.Context, texts []string, env map[string]string, allowlist []string) ([]string, error)
}

// VariableParser は両形式対応パーサー
type VariableParser interface {
    // 既存の正規表現を拡張して両形式をサポート
    ReplaceVariables(text string, resolver VariableResolver) (string, error)
}

// VariableResolver は変数解決インターフェース
type VariableResolver interface {
    ResolveVariable(name string) (string, error)
}

// 既存のSecurity Validatorをそのまま活用
// - ValidateAllEnvironmentVars(envVars map[string]string) error
// - 既存の allowlist 検証ロジックを流用

// ExpansionMetrics は最小限のメトリクス
type ExpansionMetrics struct {
    TotalExpansions   int64
    VariableCount     int
    ErrorCount        int64
    MaxIterations     int  // 反復制限方式の最大反復数
}

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
```

### 1.3 両形式対応パーサー仕様 (parser.go)

#### 1.3.1 既存コードを拡張したシンプルな実装

```go
package expansion

import (
    "regexp"
    "strings"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
)

// 既存の正規表現を拡張
var (
    // 既存: ${VAR} 形式のみ
    bracedPattern = regexp.MustCompile(`\$\{([^}]+)\}`) // 既存の variableReferenceRegex を流用

    // 新規: $VAR 形式を追加
    simplePattern = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
)

// variableParser は両形式対応パーサー
type variableParser struct {
    // シンプルな実装で正規表現のみ使用
}

// NewVariableParser は新しいパーサーを作成
func NewVariableParser() VariableParser {
    return &variableParser{}
}

// ReplaceVariables は既存の反復制限アルゴリズムを使用
func (p *variableParser) ReplaceVariables(text string, resolver VariableResolver) (string, error) {
    if !strings.Contains(text, "$") {
        return text, nil
    }

    result := text
    maxIterations := 15 // 既存の 10 から 15 に拡張
    var resolutionError error

    for i := 0; i < maxIterations && strings.Contains(result, "$"); i++ {
        oldResult := result

        // ${VAR} 形式を先に処理（優先）
        result = bracedPattern.ReplaceAllStringFunc(result, func(match string) string {
            varName := match[2 : len(match)-1] // Remove ${ and }

            resolvedValue, err := resolver.ResolveVariable(varName)
            if err != nil {
                if resolutionError == nil {
                    resolutionError = err
                }
                return match // エラー時は元の文字列を維持
            }

            return resolvedValue
        })

        // $VAR 形式を処理（${VAR}と重複しない範囲のみ）
        result = p.replaceSimpleVars(result, resolver, &resolutionError)

        if result == oldResult {
            break // 変化なし = 処理完了
        }
    }

    if resolutionError != nil {
        return "", resolutionError
    }

    // 循環参照チェック（既存ロジックを流用）
    if strings.Contains(result, "$") {
        if bracedPattern.MatchString(result) || simplePattern.MatchString(result) {
            return "", environment.ErrCircularReference
        }
    }

    return result, nil
}

// replaceSimpleVars は $VAR 形式を処理（重複を防ぐ）
func (p *variableParser) replaceSimpleVars(text string, resolver VariableResolver, resolutionError *error) string {
    return simplePattern.ReplaceAllStringFunc(text, func(match string) string {
        // ${VAR} との重複チェック（シンプルなヒューリスティック）
        if p.isLikelyInsideBraces(text, match) {
            return match // ${VAR} の一部と思われる場合はスキップ
        }

        varName := match[1:] // Remove $

        resolvedValue, err := resolver.ResolveVariable(varName)
        if err != nil {
            if *resolutionError == nil {
                *resolutionError = err
            }
            return match
        }

        return resolvedValue
    })
}

// isLikelyInsideBraces は ${VAR} 形式の一部かどうかをシンプルに判定
func (p *variableParser) isLikelyInsideBraces(text, match string) bool {
    // シンプルなヒューリスティック: ${} が含まれているかチェック
    return strings.Contains(text, "{") && strings.Contains(text, "}")
}
```

### 1.4 既存セキュリティ検証とのシンプルな統合

#### 1.4.1 既存 Security Validator をそのまま活用

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
    // 既存の resolveVariableReferencesForCommandEnv を拡張したメソッドを使用
    // ここで $VAR 形式もサポートされる
    return sv.envProcessor.ResolveVariableReferencesUnified(text, envVars, group)
}

// 既存の allowlist 検証、Command.Env 優先ポリシーはそのまま使用
// 新しいコードを書かずに既存の実績あるコードを活用
```

### 1.5 循環参照検出仕様（既存アルゴリズムを活用）

#### 1.5.1 既存の反復制限方式を拡張

```go
// 既存の processor.go の resolveVariableReferencesForCommandEnv を拡張
// 主な変更点:
// 1. maxIterations: 10 → 15 に拡張
// 2. $VAR 形式のサポート追加
// 3. 両形式の統一処理

func (p *CommandEnvProcessor) ResolveVariableReferencesUnified(
    value string,
    envVars map[string]string,
    group *runnertypes.CommandGroup,
) (string, error) {
    if !strings.Contains(value, "$") {
        return value, nil
    }

    result := value
    maxIterations := 15 // 既存の 10 から 15 に拡張
    var resolutionError error

    for i := 0; i < maxIterations && strings.Contains(result, "$"); i++ {
        oldResult := result

        // ${VAR} 形式を先に処理（既存ロジック）
        result = variableReferenceRegex.ReplaceAllStringFunc(result, p.resolveVariableFunc)

        // $VAR 形式を処理（新規追加）
        result = p.replaceSimpleVariables(result, envVars, group, &resolutionError)

        if result == oldResult {
            break // 変化なし = 処理完了
        }
    }

    if resolutionError != nil {
        return "", resolutionError
    }

    // 循環参照チェック（既存ロジックを流用）
    if strings.Contains(result, "$") {
        if variableReferenceRegex.MatchString(result) || simpleVariableRegex.MatchString(result) {
            return "", fmt.Errorf("%w: exceeded maximum resolution iterations (%d)", ErrCircularReference, maxIterations)
        }
    }

    return result, nil
}

// 新規追加: $VAR 形式用の正規表現
var simpleVariableRegex = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// 新規追加: $VAR 形式を処理するメソッド
func (p *CommandEnvProcessor) replaceSimpleVariables(text string, envVars map[string]string, group *runnertypes.CommandGroup, resolutionError *error) string {
    // 既存の resolveVariableWithSecurityPolicy を流用
    // 重複防止のためのシンプルなヒューリスティックを含む
}

// 既存のテストケースを拡張して $VAR 形式もカバー
// 既存の実績あるアルゴリズムを活用し、複雑なDFSは使用しない
```

### 1.6 cmd/args 用統合展開エンジン仕様 (expander.go)

#### 1.6.1 既存コードを活用したシンプルな実装

```go
package expansion

import (
    "context"
    "fmt"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// variableExpander は cmd/args 用のシンプルな展開エンジン
type variableExpander struct {
    envProcessor *environment.CommandEnvProcessor  // 既存の拡張されたプロセッサー
    metrics      *ExpansionMetrics
}

// NewVariableExpander は既存コンポーネントを活用したエンジンを作成
func NewVariableExpander(envProcessor *environment.CommandEnvProcessor) VariableExpander {
    return &variableExpander{
        envProcessor: envProcessor,
        metrics:      &ExpansionMetrics{MaxIterations: 15},
    }
}

// Expand は既存の反復制限アルゴリズムを使用して展開
func (e *variableExpander) Expand(ctx context.Context, text string, env map[string]string, allowlist []string) (string, error) {
    e.metrics.TotalExpansions++

    // 仮の CommandGroup を作成（allowlist 情報を含む）
    group := &runnertypes.CommandGroup{
        EnvAllowlist: allowlist,
    }

    // 既存の拡張されたメソッドを使用（$VAR と ${VAR} 両形式対応）
    result, err := e.envProcessor.ResolveVariableReferencesUnified(text, env, group)
    if err != nil {
        e.metrics.ErrorCount++
        return "", fmt.Errorf("failed to expand variables: %w", err)
    }

    return result, nil
}

// ExpandAll は複数の文字列を一括で展開
func (e *variableExpander) ExpandAll(ctx context.Context, texts []string, env map[string]string, allowlist []string) ([]string, error) {
    if len(texts) == 0 {
        return texts, nil
    }

    result := make([]string, len(texts))
    for i, text := range texts {
        expanded, err := e.Expand(ctx, text, env, allowlist)
        if err != nil {
            return nil, fmt.Errorf("failed to expand text[%d] '%s': %w", i, text, err)
        }
        result[i] = expanded
    }
    return result, nil
}

// GetMetrics はシンプルなメトリクスを取得
func (e *variableExpander) GetMetrics() ExpansionMetrics {
    return *e.metrics
}

// 既存の実績あるコードを最大限活用し、新しい複雑な実装を回避
// 直感的で理解しやすいコードを保持
```

### 1.7 設定統合仕様

#### 1.7.1 Config Parser統合 (internal/runner/config/command.go への追加)

```go
// Command構造体への変数展開統合
func (c *Command) ExpandVariables(expander expansion.VariableExpander, allowlist []string) error {
    ctx := context.Background()

    // 環境変数マップを構築
    env, err := c.BuildEnvironmentMap()
    if err != nil {
        return fmt.Errorf("failed to build environment map: %w", err)
    }

    // 事前検証
    if err := expander.ValidateVariables(ctx, c.Cmd, c.Args, env, allowlist); err != nil {
        return fmt.Errorf("variable validation failed: %w", err)
    }

    // コマンド名の展開
    if expandedCmd, err := expander.Expand(ctx, c.Cmd, env, allowlist); err != nil {
        return fmt.Errorf("failed to expand command: %w", err)
    } else {
        c.Cmd = expandedCmd
    }

    // 引数の展開
    if expandedArgs, err := expander.ExpandAll(ctx, c.Args, env, allowlist); err != nil {
        return fmt.Errorf("failed to expand args: %w", err)
    } else {
        c.Args = expandedArgs
    }

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

func TestVariableParser_ExtractVariables(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected []VariableRef
    }{
        {
            name:  "simple variable",
            input: "$HOME",
            expected: []VariableRef{
                {Name: "HOME", StartPos: 0, EndPos: 5, Format: FormatSimple, FullMatch: "$HOME"},
            },
        },
        {
            name:  "braced variable",
            input: "${USER}",
            expected: []VariableRef{
                {Name: "USER", StartPos: 0, EndPos: 7, Format: FormatBraced, FullMatch: "${USER}"},
            },
        },
        {
            name:  "mixed variables",
            input: "$HOME/bin/${APP_NAME}",
            expected: []VariableRef{
                {Name: "HOME", StartPos: 0, EndPos: 5, Format: FormatSimple, FullMatch: "$HOME"},
                {Name: "APP_NAME", StartPos: 10, EndPos: 21, Format: FormatBraced, FullMatch: "${APP_NAME}"},
            },
        },
        {
            name:  "prefix_$VAR_suffix problem case",
            input: "prefix_$HOME_suffix",
            expected: []VariableRef{
                // 注意: $HOME_suffix 全体が変数名と認識されてしまう問題
                // このため prefix_${HOME}_suffix 形式が推奨される
                {Name: "HOME_suffix", StartPos: 7, EndPos: 19, Format: FormatSimple, FullMatch: "$HOME_suffix"},
            },
        },
        {
            name:  "recommended braced format",
            input: "prefix_${HOME}_suffix",
            expected: []VariableRef{
                {Name: "HOME", StartPos: 7, EndPos: 14, Format: FormatBraced, FullMatch: "${HOME}"},
            },
        },
        {
            name:  "glob patterns as literals",
            input: "$HOME/*.txt",
            expected: []VariableRef{
                // * はリテラル文字として扱われる
                {Name: "HOME", StartPos: 0, EndPos: 5, Format: FormatSimple, FullMatch: "$HOME"},
            },
        },
        {
            name:     "no variables",
            input:    "/usr/bin/ls",
            expected: []VariableRef{},
        },
    }

    parser := NewVariableParser()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := parser.ExtractVariables(tt.input)
            require.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
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
            name: "simple $VAR expansion",
            text:  "$DOCKER_CMD",
            env:  map[string]string{"DOCKER_CMD": "/usr/bin/docker"},
            allowlist: []string{},
            expected:  "/usr/bin/docker",
            expectErr: false,
        },
        {
            name: "braced ${VAR} expansion",
            text:  "${TOOL_DIR}/script",
            env:  map[string]string{"TOOL_DIR": "/opt/tools"},
            allowlist: []string{},
            expected:  "/opt/tools/script",
            expectErr: false,
        },
        {
            name: "mixed format expansion",
            text:  "$HOME/${USER}_config",
            env:  map[string]string{"HOME": "/home/user", "USER": "testuser"},
            allowlist: []string{},
            expected:  "/home/user/testuser_config",
            expectErr: false,
        },
        {
            name: "circular reference detection",
            text:  "$A",
            env:  map[string]string{"A": "$B", "B": "$A"},
            allowlist: []string{},
            expected:  "",
            expectErr: true,
        },
        {
            name: "glob pattern preserved as literal",
            text:  "${FIND_CMD}",
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
            texts: []string{"$HOME/bin", "${USER}.log", "prefix_${APP}_suffix"},
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
            name: "error in second text",
            texts: []string{"$HOME", "$UNDEFINED"},
            env: map[string]string{"HOME": "/home/user"},
            allowlist: []string{},
            expected: nil,
            expectErr: true,
        },
    }

    expander := NewVariableExpander(nil, 10)
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
            name: "no circular reference - both formats",
            env: map[string]string{
                "A": "value_a",
                "B": "$A",
                "C": "${B}/suffix",
            },
            testValue: "${C}",
            expectErr: false,
        },
        {
            name: "direct circular reference - $VAR format",
            env: map[string]string{
                "A": "$B",
                "B": "$A",
            },
            testValue: "$A",
            expectErr: true, // 既存の反復制限で検出
        },
        {
            name: "indirect circular reference - ${VAR} format",
            env: map[string]string{
                "A": "${B}",
                "B": "${C}",
                "C": "${A}",
            },
            testValue: "${A}",
            expectErr: true, // 既存の反復制限で検出
        },
        {
            name: "mixed format circular reference",
            env: map[string]string{
                "A": "$B",
                "B": "${A}",
            },
            testValue: "$A",
            expectErr: true, // 両形式の循環参照も検出
        },
        {
            name: "self reference",
            env: map[string]string{
                "A": "$A",
            },
            testValue: "$A",
            expectErr: true,
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
    expander := NewVariableExpander(nil, 10)
    ctx := context.Background()
    env := map[string]string{
        "HOME": "/home/user",
        "BIN":  "/usr/bin",
        "APP":  "myapp",
        "PATTERN": "*.txt", // グロブパターンはリテラル扱い
    }
    allowlist := []string{}

    b.Run("simple_expansion", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _, err := expander.Expand(ctx, "$HOME/bin/$APP", env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("complex_args", func(b *testing.B) {
        args := []string{"--input", "$HOME/data", "--output", "${BIN}/output"}
        for i := 0; i < b.N; i++ {
            _, err := expander.ExpandAll(ctx, args, env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("braced_format_recommended", func(b *testing.B) {
        // prefix_${VAR}_suffix 形式（推奨）
        for i := 0; i < b.N; i++ {
            _, err := expander.Expand(ctx, "prefix_${APP}_suffix", env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("glob_pattern_literal", func(b *testing.B) {
        // グロブパターンがリテラル扱いされることを確認
        args := []string{"$HOME/$PATTERN"}
        for i := 0; i < b.N; i++ {
            result, err := expander.ExpandAll(ctx, args, env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
            // 結果は "/home/user/*.txt" となる（* は展開されない）
            if result[0] != "/home/user/*.txt" {
                b.Fatalf("Expected '/home/user/*.txt', got '%s'", result[0])
            }
        }
    })
}
```

### 1.10 シンプルな統合仕様

#### 1.10.1 既存コードを活用した統合ポイント

1. **Environment Processor 拡張**: 既存の `processor.go` に $VAR サポートと反復上限拡張
2. **Config Parser 統合**: シンプルな cmd/args 展開処理を追加
3. **Security Validator 活用**: 既存の allowlist 検証と Command.Env 優先ポリシーをそのまま使用
4. **Error Handling 一元化**: 既存のエラー型を流用して統一性を維持

#### 1.10.2 互換性保証とシンプルな実装

- **完全互換性**: 環境変数参照のない設定ファイルは無変更で動作
- **既存コード維持**: Command.Env 処理は実績ある既存コードを拡張のみ
- **直感的な実装**: 複雑なDFSではなく理解しやすい反復制限方式
- **保守性重視**: 新しい開発者でも簡単に理解・修正可能なコード

このシンプルなアプローチにより、既存の実績あるコードを最大限活用しつつ、要件を満たす堅牢で高性能な変数展開機能を実現できます。
