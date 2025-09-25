# 詳細仕様書: コマンド・引数内環境変数展開機能

## 1. 実装詳細仕様

### 1.1 パッケージ構成詳細

```
internal/runner/expansion/
├── expander.go          # 統合展開エンジン（cmd/args 用）
├── parser.go           # ${VAR}形式パーサー
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

// VariableParser は${VAR}形式パーサー
type VariableParser interface {
    // ${VAR}形式のみをサポート
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
   - マッチした場合は変数を展開、未定義変数は空文字列として展開
   - 正規表現にマッチしない、たとえば `$` の次が `{` ではない、もしくは対応する `}` がない場合にはエラーを返す
4. その他の文字はそのまま結果に追加

**新しいエラー型**:
```go
// ErrInvalidEscapeSequence is returned when an invalid escape sequence is detected
var ErrInvalidEscapeSequence = errors.New("invalid escape sequence")

// ErrInvalidVariableFormat is returned when $ is found but not followed by valid variable syntax
var ErrInvalidVariableFormat = errors.New("invalid variable format")
```

### 1.4 パーサー仕様 (${VAR} 形式のみ)

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
2. 循環参照検出 (`ErrCircularReference`)
3. アクセス不許可 (`ErrVariableNotAllowed`) ※ システム環境に存在するが allowlist 外

※ 未定義変数（ローカル/システムいずれにも存在しない）は空文字列として展開され、エラーとして扱わない

循環参照は「自身または親階層で訪問済みの変数を再び解決しようとした」タイミングで即時判定し、未定義より優先する。これにより無限再帰防止の反復上限 (以前は MaxIteration) を不要化した。

#### 1.4.3 実装メモ
- 再帰中の訪問集合は map を再利用し、深さ展開後に delete することで追加割当を削減
- `${VAR}` のネスト展開結果は都度再帰呼び出しで構築し、逐次的に `strings.Builder` へ書き込む
- コマンド定義内の変数相互参照を許容するため 2 パス処理 (先に未展開値投入→後段で展開) を採用

```go
package expansion

import (
    "regexp"
    "strings"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
)

// ${VAR}形式のみのシンプルなアプローチ
var (
    // ${VAR}形式のパターンのみ
    bracedVariablePattern = regexp.MustCompile(`\$\{([a-zA-Z_][0-9a-zA-Z_]*)\}`)

    // 既存コードとの完全互換性のためのパターン
    legacyBracedPattern = regexp.MustCompile(`\$\{([^}]+)\}`)
)

// variableParser は両形式対応パーサー
type variableParser struct {
    // シンプルな実装で正規表現のみ使用
}

// NewVariableParser は新しいパーサーを作成
func NewVariableParser() VariableParser {
    return &variableParser{}
}

// ReplaceVariables は文字ベースのスキャンによるエスケープ処理と変数展開
func (p *variableParser) ReplaceVariables(text string, resolver VariableResolver) (string, error) {
    if !strings.Contains(text, "$") {
        return text, nil
    }

    // エスケープ処理を含む1文字ずつのスキャン処理
    processed, err := p.processEscapesAndValidateVariables(text)
    if err != nil {
        return "", err
    }

    result := processed
    maxIterations := 15 // 既存の 10 から 15 に拡張
    var resolutionError error

    for i := 0; i < maxIterations && strings.Contains(result, "$"); i++ {
        oldResult := result

        // ${VAR}形式のみをサポート
        result = bracedVariablePattern.ReplaceAllStringFunc(result, func(match string) string {
            submatches := bracedVariablePattern.FindStringSubmatch(match)
            if len(submatches) < 2 {
                return match // 不正なマッチ
            }

            varName := submatches[1]
            return p.resolveVariableWithErrorHandling(varName, resolver, &resolutionError, match)
        })

        if result == oldResult {
            break // 変化なし = 処理完了
        }
    }

    if resolutionError != nil {
        return "", resolutionError
    }

    // 循環参照チェック
    if strings.Contains(result, "$") && bracedVariablePattern.MatchString(result) {
        return "", environment.ErrCircularReference
    }

    return result, nil
}

// processEscapesAndValidateVariables は文字列を1文字ずつスキャンしてエスケープ処理と変数形式検証を行う
func (p *variableParser) processEscapesAndValidateVariables(text string) (string, error) {
    var result strings.Builder
    i := 0

    for i < len(text) {
        ch := text[i]

        switch ch {
        case '\\':
            // エスケープシーケンス処理
            if i+1 >= len(text) {
                return "", ErrInvalidEscapeSequence // 文字列末尾の \
            }

            next := text[i+1]
            switch next {
            case '$', '\\':
                result.WriteByte(next) // エスケープされた文字をリテラルとして追加
                i += 2 // 2文字消費
            default:
                return "", ErrInvalidEscapeSequence // 無効なエスケープ
            }

        case '$':
            // 変数形式の検証
            if i+1 >= len(text) || text[i+1] != '{' {
                return "", ErrInvalidVariableFormat // $ の後に { がない
            }

            // 対応する } を探す
            closeIndex := strings.IndexByte(text[i:], '}')
            if closeIndex == -1 {
                return "", ErrInvalidVariableFormat // 対応する } がない
            }

            // ${...} 全体を結果に追加
            varRef := text[i : i+closeIndex+1]
            result.WriteString(varRef)
            i += closeIndex + 1

        default:
            result.WriteByte(ch)
            i++
        }
    }

    return result.String(), nil
}

// resolveVariableWithErrorHandling は変数を解決し、エラーを統一的に処理
func (p *variableParser) resolveVariableWithErrorHandling(varName string, resolver VariableResolver, resolutionError *error, originalMatch string) string {
    resolvedValue, err := resolver.ResolveVariable(varName)
    if err != nil {
        if *resolutionError == nil {
            *resolutionError = err
        }
        return originalMatch // エラー時は元の文字列を維持
    }
    return resolvedValue
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
    // ここで ${VAR} 形式が一貫してサポートされる
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
// 2. エスケープ処理の追加
// 3. 変数形式の厳格な検証

func (p *CommandEnvProcessor) ResolveVariableReferencesUnified(
    value string,
    envVars map[string]string,
    group *runnertypes.CommandGroup,
) (string, error) {
    if !strings.Contains(value, "$") {
        return value, nil
    }

    // エスケープ処理と変数形式の検証を先に実行
    processed, err := p.processEscapesAndValidateVariables(value)
    if err != nil {
        return "", err
    }

    result := processed
    maxIterations := 15 // 既存の 10 から 15 に拡張
    var resolutionError error

    for i := 0; i < maxIterations && strings.Contains(result, "$"); i++ {
        oldResult := result

        // ${VAR} 形式のみを処理（$VAR サポートは削除）
        result = variableReferenceRegex.ReplaceAllStringFunc(result, p.resolveVariableFunc)

        if result == oldResult {
            break // 変化なし = 処理完了
        }
    }

    if resolutionError != nil {
        return "", resolutionError
    }

    // 循環参照チェック
    if strings.Contains(result, "$") && variableReferenceRegex.MatchString(result) {
        return "", fmt.Errorf("%w: exceeded maximum resolution iterations (%d)", ErrCircularReference, maxIterations)
    }

    return result, nil
}

// processEscapesAndValidateVariables はCommandEnvProcessor用のエスケープ処理と変数形式検証
func (p *CommandEnvProcessor) processEscapesAndValidateVariables(text string) (string, error) {
    var result strings.Builder
    i := 0

    for i < len(text) {
        ch := text[i]

        switch ch {
        case '\\':
            // エスケープシーケンス処理
            if i+1 >= len(text) {
                return "", ErrInvalidEscapeSequence
            }

            next := text[i+1]
            switch next {
            case '$', '\\':
                result.WriteByte(next)
                i += 2
            default:
                return "", ErrInvalidEscapeSequence
            }

        case '$':
            // 変数形式の検証 - ${VAR} 形式のみ許可
            if i+1 >= len(text) || text[i+1] != '{' {
                return "", ErrInvalidVariableFormat
            }

            // 対応する } を探す
            closeIndex := strings.IndexByte(text[i:], '}')
            if closeIndex == -1 {
                return "", ErrInvalidVariableFormat
            }

            // ${...} 全体を結果に追加
            varRef := text[i : i+closeIndex+1]
            result.WriteString(varRef)
            i += closeIndex + 1

        default:
            result.WriteByte(ch)
            i++
        }
    }

    return result.String(), nil
}

// 既存のテストケースを拡張してエラーケースもカバー
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

    // コマンド名の展開（展開処理中にバリデーションも実行される）
    if expandedCmd, err := expander.Expand(ctx, c.Cmd, env, allowlist); err != nil {
        return fmt.Errorf("failed to expand command: %w", err)
    } else {
        c.Cmd = expandedCmd
    }

    // 引数の展開（展開処理中にバリデーションも実行される）
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
    return "", nil // 未定義変数は空文字列として扱う
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
            name: "no circular reference - both formats",
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
            expectErr: true, // 既存の反復制限で検出
        },
        {
            name: "indirect circular reference",
            env: map[string]string{
                "A": "${B}",
                "B": "${C}",
                "C": "${A}",
            },
            testValue: "${A}",
            expectErr: true, // 既存の反復制限で検出
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
            testValue: `{"user": "$USER", "home": "${HOME}"}`,
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
    // 統一正規表現アプローチのパフォーマンステスト
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
