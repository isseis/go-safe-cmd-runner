# 実装計画書：コマンド出力の Slack 通知における機密情報 Redaction

## 1. 実装概要

### 1.1 目的

コマンド出力に含まれる機密情報（パスワード、API トークン、秘密鍵など）が redact されずに Slack に送信される問題を解決する。要件定義書・アーキテクチャ設計書・詳細設計書に基づいた確実な開発を行う。

### 1.2 実装方針

- **テスト駆動開発（TDD）**：各機能の実装前にテストを作成
- **段階的実装**：Phase 1から順次実装し、各Phaseで動作確認
- **Defense in Depth**：案1（主対策）と案2（補完策）による二重防御
- **Fail-secure 原則**：redaction 失敗時は安全なプレースホルダーで置換
- **セキュリティ優先**：機密情報漏洩のリスクを最小化

### 1.3 実装スコープ

本タスクで実装する機能:
- **案2（補完策）**：CommandResult 作成時の早期 redaction
  - `ValidatorInterface` への `SanitizeOutputForLogging` メソッド追加
  - `GroupExecutor` での `SanitizeOutputForLogging` 呼び出し
- **案1（主対策）**：RedactingHandler の拡張
  - `slog.KindAny` 型の処理
  - LogValuer インターフェースの解決
  - スライス型の処理
  - 再帰深度制限
  - Panic からの復旧
  - 失敗時のログ記録（Slack 以外の出力先）
- **Fail-secure 改善**：RedactText の正規表現コンパイル失敗時の処理
- **テストの拡充**：Unit、Integration、E2E テスト

### 1.4 実装しない項目（将来的な拡張）

以下は本タスクの範囲外とし、将来的な拡張として検討：
- プレースホルダーの統一（`LogPlaceholder` と `TextPlaceholder` の統合）
- 型判定結果のキャッシュ（パフォーマンス最適化）
- カスタム redaction パターンの TOML 設定サポート
- 機械学習ベースの機密情報検出

---

## 2. 実装フェーズ

### Phase 1: 案2（CommandResult 作成時の Redaction）

**目的**：早期に redaction を適用し、第1層の防御を確立

**実装期間**：3日間

#### 2.1.1 ValidatorInterface の拡張

**ファイル**：`internal/runner/security/interfaces.go`

**タスク**：
- [ ] `ValidatorInterface` に `SanitizeOutputForLogging(output string) string` メソッドを追加

**実装内容**：
```go
// internal/runner/security/interfaces.go

type ValidatorInterface interface {
    // Existing methods
    ValidateAllEnvironmentVars(envVars map[string]string) error
    ValidateEnvironmentValue(key, value string) error
    ValidateCommand(command string) error

    // New method for Case 2 (補完策)
    SanitizeOutputForLogging(output string) string
}
```

**テスト**：
- [ ] コンパイルが通ることを確認（`Validator` は既に実装済み）
- [ ] モック実装の更新は次のセクションで実施

**完了基準**：
- ✅ インターフェースが更新される
- ✅ `Validator` が `ValidatorInterface` を満たす
- ✅ コンパイルエラーがない

#### 2.1.2 モック実装の更新

**ファイル**：`internal/runner/security/testing/testify_mocks.go`

**タスク**：
- [ ] `MockValidator` に `SanitizeOutputForLogging` メソッドを追加

**実装内容**：
```go
// internal/runner/security/testing/testify_mocks.go

func (m *MockValidator) SanitizeOutputForLogging(output string) string {
    args := m.Called(output)
    return args.String(0)
}
```

**テスト**：
- [ ] モックを使用する既存のテストがパスすることを確認

**完了基準**：
- ✅ モック実装が追加される
- ✅ 既存のテストがすべてパス

#### 2.1.3 GroupExecutor での呼び出し

**ファイル**：`internal/runner/group_executor.go`

**タスク**：
- [ ] `executeAllCommands()` メソッドを変更し、`CommandResult` 作成前に `SanitizeOutputForLogging` を呼び出す

**実装内容**：
```go
// internal/runner/group_executor.go

func (ge *DefaultGroupExecutor) executeAllCommands(...) ([]common.CommandResult, error) {
    // ... existing code ...

    // 案2：CommandResult 作成時に redact
    sanitizedStdout := ge.validator.SanitizeOutputForLogging(stdout)
    sanitizedStderr := ge.validator.SanitizeOutputForLogging(stderr)

    cmdResult := common.CommandResult{
        CommandResultFields: common.CommandResultFields{
            Name:     cmdSpec.Name,
            ExitCode: exitCode,
            Output:   sanitizedStdout,  // Redact 済み
            Stderr:   sanitizedStderr,  // Redact 済み
        },
    }

    results = append(results, cmdResult)
    // ...
}
```

**テスト**：
- [ ] 既存のテストがパスすることを確認
- [ ] 新しい統合テストを追加（次のセクション）

**完了基準**：
- ✅ `SanitizeOutputForLogging` が呼び出される
- ✅ 既存のテストがすべてパス

#### 2.1.4 統合テストの追加

**ファイル**：`internal/runner/group_executor_test.go`

**タスク**：
- [ ] `SanitizeOutputForLogging` が呼び出されることを検証するテストを追加

**実装内容**：
```go
// internal/runner/group_executor_test.go

func TestGroupExecutor_SanitizeOutputForLogging(t *testing.T) {
    // Setup
    mockValidator := &security_testing.MockValidator{}
    mockValidator.On("SanitizeOutputForLogging", "password=secret123").Return("password=[REDACTED]")
    mockValidator.On("SanitizeOutputForLogging", "").Return("")
    // ... 他のモックの設定 ...

    executor := NewDefaultGroupExecutor(..., mockValidator, ...)

    // Test
    cmdSpec := &runnertypes.Command{
        Name: "test_cmd",
        Cmd:  "/bin/echo",
        Args: []string{"password=secret123"},
    }

    results, err := executor.executeAllCommands(...)
    require.NoError(t, err)

    // Verify
    mockValidator.AssertCalled(t, "SanitizeOutputForLogging", mock.Anything)
    assert.Equal(t, "password=[REDACTED]", results[0].Output)
}
```

**完了基準**：
- ✅ テストがパス
- ✅ `SanitizeOutputForLogging` が正しく呼び出されることを確認

#### 2.1.5 E2E テスト（案2のみ）

**ファイル**：テスト専用ディレクトリまたは既存のテストファイル

**タスク**：
- [ ] 実際のコマンド実行から Slack 送信までの E2E テストを作成
- [ ] モック Slack エンドポイントを使用

**実装内容**：
```go
func TestE2E_Case2_RedactionAtCreation(t *testing.T) {
    // Setup: モック Slack handler
    mockSlackHandler := &MockSlackHandler{messages: make([]string, 0)}

    // 本番の Validator を使用
    validator, _ := security.NewValidator(&security.Config{
        LoggingOptions: security.LoggingOptions{
            RedactSensitiveInfo: true,
        },
    })

    // GroupExecutor を作成
    executor := NewDefaultGroupExecutor(..., validator, ...)

    // Logger を設定（RedactingHandler はまだ拡張されていない）
    logger := slog.New(mockSlackHandler)

    // Test: パスワードを含むコマンドを実行
    cmdSpec := &runnertypes.Command{
        Name: "test_cmd",
        Cmd:  "/bin/echo",
        Args: []string{"password=secret123"},
    }

    results, err := executor.executeAllCommands(...)
    require.NoError(t, err)

    // Log に結果を記録
    logger.Info("Command execution completed", "results", results)

    // Verify: Slack に送信されたメッセージを確認
    assert.Len(t, mockSlackHandler.messages, 1)
    message := mockSlackHandler.messages[0]

    // 案2により既に redact されている
    assert.Contains(t, message, "password=[REDACTED]")
    assert.NotContains(t, message, "secret123")
}
```

**完了基準**：
- ✅ E2E テストがパス
- ✅ 機密情報が Slack に送信されないことを確認

#### 2.1.6 Phase 1 の完了確認

**完了基準**：
- ✅ すべてのタスクが完了
- ✅ すべてのテストがパス
- ✅ コードレビューが完了
- ✅ ドキュメントが更新される（必要に応じて）

**期待される結果**：
- CommandResult 作成時に機密情報が redact される
- 案2（補完策）が正常に動作する
- 案1が実装されていなくても、ある程度の保護が得られる

---

### Phase 2: 案1（RedactingHandler の拡張）- 基本実装

**目的**：RedactingHandler で `slog.KindAny` 型を処理できるようにする

**実装期間**：5日間

#### 2.2.1 エラー型の定義

**ファイル**：`internal/redaction/errors.go`（新規作成）

**タスク**：
- [ ] エラー型を定義

**実装内容**：
```go
// internal/redaction/errors.go

package redaction

import "fmt"

// ErrRedactionDepthExceeded is returned when recursion depth limit is reached
type ErrRedactionDepthExceeded struct {
    Key   string
    Depth int
}

func (e *ErrRedactionDepthExceeded) Error() string {
    return fmt.Sprintf("redaction depth limit (%d) exceeded for attribute %q", e.Depth, e.Key)
}

// ErrLogValuePanic is returned when LogValue() panics
type ErrLogValuePanic struct {
    Key          string
    PanicValue   any
    StackTrace   string
}

func (e *ErrLogValuePanic) Error() string {
    return fmt.Sprintf("LogValue() panicked for attribute %q: %v", e.Key, e.PanicValue)
}

// ErrRegexCompilationFailed is returned when regex compilation fails
type ErrRegexCompilationFailed struct {
    Pattern string
    Err     error
}

func (e *ErrRegexCompilationFailed) Error() string {
    return fmt.Sprintf("failed to compile regex pattern %q: %v", e.Pattern, e.Err)
}
```

**テスト**：
- [ ] エラー型が正しく定義されていることを確認
- [ ] `Error()` メソッドが適切なメッセージを返すことを確認

**完了基準**：
- ✅ エラー型が定義される
- ✅ テストがパス

#### 2.2.2 RedactionContext の追加

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] `RedactionContext` 構造体を定義
- [ ] `maxRedactionDepth` 定数を定義

**実装内容**：
```go
// internal/redaction/redactor.go

// RedactionContext holds context information for recursive redaction
type RedactionContext struct {
    depth int  // Current recursion depth
}

// maxRedactionDepth is the maximum depth for recursive redaction
// to prevent infinite recursion and DoS attacks
const maxRedactionDepth = 10

// RedactionFailurePlaceholder is used when redaction itself fails
const RedactionFailurePlaceholder = "[REDACTION FAILED - OUTPUT SUPPRESSED]"
```

**完了基準**：
- ✅ 構造体と定数が定義される
- ✅ コンパイルエラーがない

#### 2.2.3 redactLogAttributeWithContext の実装

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] 既存の `RedactLogAttribute` を変更せず、内部実装として `redactLogAttributeWithContext` を追加
- [ ] `RedactLogAttribute` から `redactLogAttributeWithContext` を呼び出す

**実装内容**：
```go
// internal/redaction/redactor.go

// RedactLogAttribute redacts sensitive information from a log attribute
// This is the public API entry point
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    return c.redactLogAttributeWithContext(attr, RedactionContext{depth: 0})
}

// redactLogAttributeWithContext is the internal implementation with context
func (c *Config) redactLogAttributeWithContext(attr slog.Attr, ctx RedactionContext) slog.Attr {
    key := attr.Key
    value := attr.Value

    // Check for sensitive patterns in the key
    if c.Patterns.IsSensitiveKey(key) {
        return slog.Attr{Key: key, Value: slog.StringValue(c.TextPlaceholder)}
    }

    // Process based on value kind
    switch value.Kind() {
    case slog.KindString:
        // Existing string redaction logic
        strValue := value.String()
        redactedText := c.RedactText(strValue)
        if redactedText != strValue {
            return slog.Attr{Key: key, Value: slog.StringValue(redactedText)}
        }
        if c.Patterns.IsSensitiveValue(strValue) {
            return slog.Attr{Key: key, Value: slog.StringValue(c.LogPlaceholder)}
        }
        return attr

    case slog.KindGroup:
        // Existing group redaction logic (recursive)
        groupAttrs := value.Group()
        redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
        for _, groupAttr := range groupAttrs {
            redactedGroupAttrs = append(redactedGroupAttrs, c.redactLogAttributeWithContext(groupAttr, ctx))
        }
        return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}

    case slog.KindAny:
        // NEW: Handle KindAny (will be implemented in next steps)
        return c.processKindAny(key, value, ctx)

    default:
        // Other types: pass through
        return attr
    }
}
```

**テスト**：
- [ ] 既存のテストがすべてパス（動作変更なし）
- [ ] `KindAny` の処理は次のステップで実装

**完了基準**：
- ✅ `redactLogAttributeWithContext` が実装される
- ✅ 既存のテストがすべてパス
- ✅ コンパイルエラーがない（`processKindAny` は次で実装）

#### 2.2.4 processKindAny の基本実装

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] `processKindAny` メソッドを実装
- [ ] 型判定のロジックを実装（LogValuer、スライス、その他）

**実装内容**：
```go
// internal/redaction/redactor.go

import (
    "reflect"
)

// processKindAny processes slog.KindAny values
func (c *Config) processKindAny(key string, value slog.Value, ctx RedactionContext) slog.Attr {
    anyValue := value.Any()

    // Nil check
    if anyValue == nil {
        return slog.Attr{Key: key, Value: value}
    }

    // 1. Check for LogValuer interface
    if logValuer, ok := anyValue.(slog.LogValuer); ok {
        return c.processLogValuer(key, logValuer, ctx)
    }

    // 2. Check for slice type
    rv := reflect.ValueOf(anyValue)
    if rv.Kind() == reflect.Slice {
        return c.processSlice(key, anyValue, ctx)
    }

    // 3. Unsupported type: pass through
    return slog.Attr{Key: key, Value: value}
}
```

**テスト**：
- [ ] Nil 値の処理をテスト
- [ ] 未対応型のパススルーをテスト

**完了基準**：
- ✅ `processKindAny` が実装される
- ✅ テストがパス

#### 2.2.5 processLogValuer の実装

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] `processLogValuer` メソッドを実装
- [ ] 再帰深度チェックを実装
- [ ] Panic からの復旧を実装

**実装内容**：
```go
// internal/redaction/redactor.go

import (
    "runtime/debug"
)

// processLogValuer processes a LogValuer value and recursively redacts it
func (c *Config) processLogValuer(key string, logValuer slog.LogValuer, ctx RedactionContext) slog.Attr {
    // 1. Check recursion depth
    if ctx.depth >= maxRedactionDepth {
        // Depth limit reached: return partially redacted value (not an error)
        // Log at Debug level
        slog.Debug("Recursion depth limit reached - returning partially redacted value",
            "attribute_key", key,
            "depth", maxRedactionDepth,
            "note", "This is not an error - DoS prevention measure",
        )
        return slog.Attr{Key: key, Value: slog.AnyValue(logValuer)}
    }

    // 2. Call LogValue() with panic recovery
    var resolvedValue slog.Value
    var panicOccurred bool
    var panicValue any

    func() {
        defer func() {
            if r := recover(); r != nil {
                panicOccurred = true
                panicValue = r
                resolvedValue = slog.StringValue(RedactionFailurePlaceholder)

                // Log warning (should be sent to stderr/file, not Slack)
                // Note: Logging from within RedactingHandler requires careful setup
                // to avoid recursion. Use a dedicated failure logger.
                slog.Warn("Redaction failed due to panic in LogValue()",
                    "attribute_key", key,
                    "panic", r,
                    "stack_trace", string(debug.Stack()),
                    "output_destination", "stderr, file, audit",
                )
            }
        }()
        resolvedValue = logValuer.LogValue()
    }()

    if panicOccurred {
        return slog.Attr{Key: key, Value: resolvedValue}
    }

    // 3. Recursively redact the resolved value
    resolvedAttr := slog.Attr{Key: key, Value: resolvedValue}
    nextCtx := RedactionContext{depth: ctx.depth + 1}
    return c.redactLogAttributeWithContext(resolvedAttr, nextCtx)
}
```

**テスト**：
- [ ] 正常な LogValuer の処理をテスト
- [ ] 再帰深度制限をテスト（depth=11）
- [ ] Panic からの復旧をテスト

**完了基準**：
- ✅ `processLogValuer` が実装される
- ✅ すべてのテストがパス

#### 2.2.6 processSlice の実装

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] `processSlice` メソッドを実装
- [ ] スライスの各要素を処理

**実装内容**：
```go
// internal/redaction/redactor.go

// processSlice processes a slice value and redacts LogValuer elements
func (c *Config) processSlice(key string, sliceValue any, ctx RedactionContext) slog.Attr {
    // 1. Check recursion depth
    if ctx.depth >= maxRedactionDepth {
        slog.Debug("Recursion depth limit reached for slice - returning original",
            "attribute_key", key,
            "depth", maxRedactionDepth,
        )
        return slog.Attr{Key: key, Value: slog.AnyValue(sliceValue)}
    }

    // 2. Use reflection to get slice elements
    rv := reflect.ValueOf(sliceValue)
    if rv.Kind() != reflect.Slice {
        // Not a slice (should not happen)
        return slog.Attr{Key: key, Value: slog.AnyValue(sliceValue)}
    }

    // 3. Process each element
    processedElements := make([]any, 0, rv.Len())
    nextCtx := RedactionContext{depth: ctx.depth + 1}

    for i := 0; i < rv.Len(); i++ {
        element := rv.Index(i).Interface()

        // Check if element is LogValuer
        if logValuer, ok := element.(slog.LogValuer); ok {
            // Call LogValue() and redact
            var resolvedValue slog.Value
            var panicOccurred bool

            func() {
                defer func() {
                    if r := recover(); r != nil {
                        panicOccurred = true
                        resolvedValue = slog.StringValue(RedactionFailurePlaceholder)
                        slog.Warn("Redaction failed for slice element",
                            "attribute_key", key,
                            "element_index", i,
                            "panic", r,
                        )
                    }
                }()
                resolvedValue = logValuer.LogValue()
            }()

            if !panicOccurred {
                // Redact the resolved value
                elementKey := fmt.Sprintf("%s[%d]", key, i)
                redactedAttr := c.redactLogAttributeWithContext(
                    slog.Attr{Key: elementKey, Value: resolvedValue},
                    nextCtx,
                )
                processedElements = append(processedElements, redactedAttr.Value.Any())
            } else {
                processedElements = append(processedElements, resolvedValue.Any())
            }
        } else {
            // Non-LogValuer element: keep as-is
            processedElements = append(processedElements, element)
        }
    }

    // 4. Return processed slice (maintain slice type)
    return slog.Attr{Key: key, Value: slog.AnyValue(processedElements)}
}
```

**テスト**：
- [ ] LogValuer スライスの処理をテスト
- [ ] 空スライスの処理をテスト
- [ ] 混在型スライス（LogValuer + non-LogValuer）の処理をテスト

**完了基準**：
- ✅ `processSlice` が実装される
- ✅ すべてのテストがパス

#### 2.2.7 Unit Tests の追加

**ファイル**：`internal/redaction/redactor_test.go`

**タスク**：
- [ ] Unit テストを追加（詳細設計書の 6.1 に基づく）

**テストケース**：
1. LogValuer single
2. LogValuer slice
3. Non-LogValuer
4. Deep recursion (depth=11)
5. Panic handling
6. Nil LogValue
7. Empty slice
8. Mixed slice

**完了基準**：
- ✅ すべての Unit テストがパス
- ✅ テストカバレッジが 90% 以上

#### 2.2.8 Phase 2 の完了確認

**完了基準**：
- ✅ すべてのタスクが完了
- ✅ すべてのテストがパス
- ✅ コードレビューが完了

**期待される結果**：
- RedactingHandler が `slog.KindAny` 型を処理できる
- LogValuer と スライス型の redaction が動作する
- 再帰深度制限と panic 復旧が動作する

---

### Phase 3: 案1（RedactingHandler の拡張）- ログ記録の改善

**目的**：失敗時のログ記録を Slack 以外の出力先に送信できるようにする

**実装期間**：3日間

#### 2.3.1 RedactingHandler への failureLogger の追加

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] `RedactingHandler` に `failureLogger` フィールドを追加
- [ ] `NewRedactingHandler` のシグネチャを変更

**実装内容**：
```go
// internal/redaction/redactor.go

type RedactingHandler struct {
    handler       slog.Handler
    config        *Config
    failureLogger *slog.Logger  // NEW: Failure logging (stderr/file, not Slack)
}

func NewRedactingHandler(handler slog.Handler, config *Config, failureLogger *slog.Logger) *RedactingHandler {
    if config == nil {
        config = DefaultConfig()
    }
    if failureLogger == nil {
        // Default to slog.Default() if not provided
        failureLogger = slog.Default()
    }
    return &RedactingHandler{
        handler:       handler,
        config:        config,
        failureLogger: failureLogger,
    }
}
```

**テスト**：
- [ ] 既存のテストを更新（`NewRedactingHandler` の呼び出しに `nil` を渡す）
- [ ] `failureLogger` が正しく設定されることを確認

**完了基準**：
- ✅ `failureLogger` フィールドが追加される
- ✅ 既存のテストがすべてパス

#### 2.3.2 processLogValuer での failureLogger 使用

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] `processLogValuer` の panic ハンドラーで `c.failureLogger` を使用
- [ ] 同様に `processSlice` でも使用

**実装内容**：
```go
// internal/redaction/redactor.go

// Note: Config does not have failureLogger, so we need to pass it as a parameter
// or make processLogValuer a method of RedactingHandler

// Option 1: Make processLogValuer a method of RedactingHandler
func (r *RedactingHandler) processLogValuerInternal(key string, logValuer slog.LogValuer, ctx RedactionContext) slog.Attr {
    // ... existing code ...

    func() {
        defer func() {
            if rec := recover(); rec != nil {
                panicOccurred = true
                panicValue = rec
                resolvedValue = slog.StringValue(RedactionFailurePlaceholder)

                // Use failureLogger (does not go through RedactingHandler)
                r.failureLogger.Warn("Redaction failed due to panic in LogValue()",
                    "attribute_key", key,
                    "panic", rec,
                    "stack_trace", string(debug.Stack()),
                    "output_destination", "stderr, file, audit",
                )
            }
        }()
        resolvedValue = logValuer.LogValue()
    }()

    // ... rest of the code ...
}
```

**設計上の注意**：
- `processLogValuer` と `processSlice` を `RedactingHandler` のメソッドに変更する必要がある
- または、`Config` に `failureLogger` を追加する（ただし、これは設計的に不自然）

**推奨アプローチ**：
- `RedactingHandler` に内部メソッドを追加し、`Config` の既存メソッドはラッパーとして保持

**テスト**：
- [ ] Panic 発生時に `failureLogger` が呼び出されることを確認
- [ ] ログメッセージが正しいことを確認

**完了基準**：
- ✅ `failureLogger` が使用される
- ✅ テストがパス

#### 2.3.3 logging システムでの初期化更新

**ファイル**：`internal/runner/bootstrap/logging.go` または該当するファイル

**タスク**：
- [ ] `RedactingHandler` の初期化時に `failureLogger` を渡す

**実装内容**：
```go
// 疑似コード

func setupLogging() {
    // 各 handler を作成
    stderrHandler := slog.NewTextHandler(os.Stderr, nil)
    fileHandler := newFileHandler("/var/log/runner/runner.log")
    slackHandler := newSlackHandler(slackWebhookURL)

    // Failure log handler (Slack を含まない)
    failureLogHandler := logging.NewMultiHandler(stderrHandler, fileHandler)
    failureLogger := slog.New(failureLogHandler)

    // RedactingHandler を作成
    redactionConfig := redaction.DefaultConfig()
    redactingHandler := redaction.NewRedactingHandler(
        logging.NewMultiHandler(stderrHandler, fileHandler, slackHandler),
        redactionConfig,
        failureLogger,  // Slack を含まない専用ロガー
    )

    // グローバルロガーとして設定
    slog.SetDefault(slog.New(redactingHandler))
}
```

**テスト**：
- [ ] 統合テストで `failureLogger` が正しく設定されることを確認

**完了基準**：
- ✅ logging システムが更新される
- ✅ テストがパス

#### 2.3.4 Phase 3 の完了確認

**完了基準**：
- ✅ すべてのタスクが完了
- ✅ すべてのテストがパス
- ✅ 失敗ログが Slack に送信されないことを確認

**期待される結果**：
- Redaction の失敗が stderr とファイルログに記録される
- 失敗ログが Slack に送信されない

---

### Phase 4: Fail-secure 改善

**目的**：RedactText の正規表現コンパイル失敗時に fail-secure 動作を実装

**実装期間**：2日間

#### 2.4.1 影響範囲の調査

**タスク**：
- [ ] `RedactText()` を使用している箇所をリストアップ
- [ ] 変更による影響を評価

**調査方法**：
```bash
grep -rn "RedactText" internal/
```

**完了基準**：
- ✅ 影響範囲がドキュメント化される
- ✅ 変更のリスク評価が完了

#### 2.4.2 performXXXRedaction メソッドの変更

**ファイル**：`internal/redaction/redactor.go`

**タスク**：
- [ ] `performSpacePatternRedaction` を fail-secure に変更
- [ ] `performColonPatternRedaction` を fail-secure に変更
- [ ] `performKeyValuePatternRedaction` を fail-secure に変更

**実装内容**：
```go
// internal/redaction/redactor.go

func (c *Config) performSpacePatternRedaction(text, pattern, placeholder string) string {
    escapedPattern := regexp.QuoteMeta(pattern)
    regexPattern := `(?i)(` + escapedPattern + `)(\S+)`

    re, err := regexp.Compile(regexPattern)
    if err != nil {
        // ✅ Fail-secure: 正規表現コンパイル失敗時は全体を置換
        slog.Warn("Regex compilation failed - using safe placeholder",
            "pattern", pattern,
            "error", err.Error(),
            "output_destination", "stderr, file, audit",
        )
        return RedactionFailurePlaceholder
    }

    // ... existing code ...
}

// 他のメソッドも同様に変更
```

**テスト**：
- [ ] 正規表現コンパイル失敗時のテストを追加
- [ ] `RedactionFailurePlaceholder` が返されることを確認

**完了基準**：
- ✅ すべての `performXXXRedaction` メソッドが fail-secure になる
- ✅ テストがパス

#### 2.4.3 既存のテストの更新

**ファイル**：`internal/redaction/redactor_test.go`

**タスク**：
- [ ] 既存のテストが新しい動作でパスすることを確認
- [ ] エラーケースのテストを追加

**完了基準**：
- ✅ すべてのテストがパス
- ✅ カバレッジが維持される

#### 2.4.4 Phase 4 の完了確認

**完了基準**：
- ✅ すべてのタスクが完了
- ✅ すべてのテストがパス
- ✅ コードレビューが完了

**期待される結果**：
- 正規表現コンパイル失敗時に機密情報が漏洩しない
- Fail-secure 原則が徹底される

---

### Phase 5: 統合・E2E テスト

**目的**：案1と案2の二重防御が正しく動作することを確認

**実装期間**：3日間

#### 2.5.1 Integration Tests の追加

**ファイル**：`internal/runner/integration_test.go` または新規ファイル

**タスク**：
- [ ] Full pipeline テストを追加
- [ ] Multi-handler テストを追加
- [ ] 案1・案2の両方が動作することを確認

**実装内容**：
```go
func TestIntegration_DualDefense(t *testing.T) {
    // Setup: 実際の Validator と RedactingHandler を使用
    validator, _ := security.NewValidator(&security.Config{
        LoggingOptions: security.LoggingOptions{
            RedactSensitiveInfo: true,
        },
    })

    // Mock Slack handler
    mockSlackHandler := &MockSlackHandler{messages: make([]string, 0)}

    // RedactingHandler を設定
    redactionConfig := redaction.DefaultConfig()
    failureLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
    redactingHandler := redaction.NewRedactingHandler(mockSlackHandler, redactionConfig, failureLogger)
    logger := slog.New(redactingHandler)

    // GroupExecutor を作成
    executor := NewDefaultGroupExecutor(..., validator, ...)

    // Test: パスワードを含むコマンドを実行
    cmdSpec := &runnertypes.Command{
        Name: "test_cmd",
        Cmd:  "/bin/echo",
        Args: []string{"password=secret123"},
    }

    results, err := executor.executeAllCommands(...)
    require.NoError(t, err)

    // Log に結果を記録
    logger.Info("Command execution completed", "results", results)

    // Verify: Slack に送信されたメッセージを確認
    assert.Len(t, mockSlackHandler.messages, 1)
    message := mockSlackHandler.messages[0]

    // 案2により既に redact されている
    assert.Contains(t, message, "password=[REDACTED]")
    assert.NotContains(t, message, "secret123")
}

func TestIntegration_Case1Only(t *testing.T) {
    // Setup: Validator の redaction を無効化
    validator, _ := security.NewValidator(&security.Config{
        LoggingOptions: security.LoggingOptions{
            RedactSensitiveInfo: false,  // 案2を無効化
        },
    })

    // RedactingHandler は有効（案1のみで保護）
    // ...

    // Test: 案1のみで機密情報が redact されることを確認
    // ...
}
```

**完了基準**：
- ✅ Integration テストがパス
- ✅ 二重防御が正しく動作することを確認

#### 2.5.2 E2E Tests の追加

**ファイル**：テスト専用ディレクトリ

**タスク**：
- [ ] 実際のコマンド実行から Slack 送信までの E2E テストを作成
- [ ] モック Slack エンドポイントを使用

**実装内容**：
```go
func TestE2E_RealCommandWithAPIKey(t *testing.T) {
    // Setup: モック Slack エンドポイント
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        // Verify: API key が redact されていることを確認
        assert.Contains(t, string(body), "api_key=[REDACTED]")
        assert.NotContains(t, string(body), "abc123xyz789")
        w.WriteHeader(http.StatusOK)
    }))
    defer mockServer.Close()

    // Configure Slack webhook URL
    os.Setenv("SLACK_WEBHOOK_URL", mockServer.URL)

    // Run actual command
    // ...

    // Verify: モックサーバーがリクエストを受け取ったことを確認
}
```

**重要**：
- E2E テストでは本番 Slack チャネルを使用しない
- モック HTTP サーバーを使用

**完了基準**：
- ✅ E2E テストがパス
- ✅ 機密情報が Slack に送信されないことを確認

#### 2.5.3 パフォーマンステスト

**ファイル**：`internal/redaction/redactor_test.go`

**タスク**：
- [ ] ベンチマークテストを追加
- [ ] パフォーマンス目標を達成していることを確認

**実装内容**：
```go
func BenchmarkRedactingHandler_String(b *testing.B) { /* ... */ }
func BenchmarkRedactingHandler_LogValuer(b *testing.B) { /* ... */ }
func BenchmarkRedactingHandler_Slice(b *testing.B) { /* ... */ }
```

**目標**：
- RedactingHandler のオーバーヘッド：10% 以内
- LogValuer/Slice：String 処理の 2倍以内

**完了基準**：
- ✅ ベンチマークテストが実行される
- ✅ パフォーマンス目標を達成

#### 2.5.4 Phase 5 の完了確認

**完了基準**：
- ✅ すべてのテストがパス
- ✅ カバレッジが 90% 以上
- ✅ パフォーマンス目標を達成

**期待される結果**：
- 案1と案2の二重防御が正しく動作
- すべてのテストが成功
- パフォーマンスが許容範囲内

---

## 3. デプロイメント計画

### 3.1 段階的ロールアウト

#### Phase 1：案2のみ（開発環境）

**デプロイ内容**：
- CommandResult 作成時の redaction のみ

**検証基準**：
- ✅ 単体テストがすべてパス
- ✅ 統合テストがすべてパス
- ✅ 機密情報が Slack に送信されないことを確認（モック環境）

**完了判断**：
- 上記の検証基準がすべて満たされ、コードレビューが完了

#### Phase 2：案1+案2（開発環境）

**デプロイ内容**：
- RedactingHandler の拡張を追加

**検証基準**：
- ✅ すべてのテストがパス
- ✅ パフォーマンス目標を達成
- ✅ 案1のみで保護できることを確認（案2を無効化したテスト）

**完了判断**：
- 上記の検証基準がすべて満たされ、パフォーマンス目標を達成

#### Phase 3：本番環境へのデプロイ

**デプロイ準備**：
- すべてのテストがパス（カバレッジ 90% 以上）
- パフォーマンスが許容範囲内
- ドキュメントが完成
- ロールバック計画の策定

**デプロイ手順**：
1. 本番環境へのデプロイ前に最終確認
2. デプロイ実行
3. 初期監視期間（24時間）
4. 安定性確認（1週間）

**検証基準（本番環境）**：
- ✅ デプロイ前の最終確認：
  - 本番 Slack Webhook URL が設定されていることを確認
  - テスト用 Webhook URL が削除されていることを確認
  - Redaction が有効化されていることを確認（`RedactSensitiveInfo: true`）
- ✅ 初期監視期間（24時間）：
  - Redaction エラー率 < 0.1%
  - ログ出力レイテンシ < 100ms
  - CPU 使用率増加 < 5%
  - メモリ使用量増加 < 5%
- ✅ セキュリティ検証：
  - **自動スキャン（最優先）**：
    - Slack API を使用して対象期間（過去1週間）のメッセージをエクスポート
    - 検証スクリプトで以下をチェック：
      - `[REDACTED]` プレースホルダーの存在確認
      - `[REDACTION FAILED - OUTPUT SUPPRESSED]` の出現回数（あれば調査）
      - 機密情報パターンのスキャン（`password=`, `Bearer `, `api_key=`, AWS キー形式など）
    - スキャン結果をレポートとして出力
  - **限定的な目視確認（自動スキャンで問題検出時のみ）**：
    - 自動スキャンで疑わしいメッセージが検出された場合のみ実施
    - 検出されたメッセージのみを目視で精査
  - **アクセス制限と監査**：
    - 検証作業は適切な権限とセキュリティクリアランスを持つ担当者のみが実施
    - 検証作業の開始・終了・担当者を監査ログに記録
- ✅ 安定性確認（1週間）：
  - 上記メトリクスが継続的に正常範囲内

**完了判断**：
- 1週間の安定稼働を確認し、問題が発生しないことを確認

### 3.2 ロールバック計画

**ロールバックのトリガー**：
- Redaction エラー率 > 1%
- ログ出力レイテンシ > 200ms
- CPU 使用率増加 > 10%
- メモリ使用量増加 > 10%
- 機密情報が Slack に漏洩したことが確認された場合

**ロールバック手順**：
1. **重大度：High**（機密情報漏洩）
   - 即座にロールバック
   - 旧バージョンに戻す
   - インシデント報告と原因調査

2. **重大度：Medium**（パフォーマンス問題）
   - 案1を無効化（案2のみで運用）
   - 原因調査と修正

3. **重大度：Low**（軽微なエラー）
   - 修正して再デプロイ

---

## 4. 監視とアラート

### 4.1 メトリクス

| メトリクス | 正常範囲 | アラート条件 | 対応 |
|----------|---------|------------|------|
| Redaction エラー率 | < 0.1% | > 1% | 調査・修正 |
| ログ出力レイテンシ | < 100ms | > 200ms | パフォーマンス調査 |
| CPU 使用率増加 | < 5% | > 10% | 最適化の検討 |
| メモリ使用量増加 | < 5% | > 10% | メモリリーク調査 |

### 4.2 ログ出力

**Debug レベル**（オプション、デフォルトで無効）：
- 再帰深度制限到達（DoS 防止のための正常な動作）

**Warn レベル**（必須）：
- Redaction の失敗（panic、正規表現コンパイル失敗など）
- 出力先：stderr、ファイルログ、監査ログ（Slack を含まない）

### 4.3 検証スクリプト

**目的**：本番 Slack チャネルのログを自動的にスキャンし、機密情報が漏洩していないことを確認

**要件**：
- 言語：Python または Go
- 機能：
  - Slack API 経由でのメッセージエクスポート（`conversations.history` API）
  - 正規表現ベースの機密情報パターンマッチング
  - レポート生成（検出結果、統計情報、問題のあるメッセージID）
- セキュリティ：
  - エクスポートしたメッセージは暗号化してローカルに一時保存
  - 検証完了後は自動的に削除
  - スクリプト実行ログを監査証跡として保存

**実装例**（疑似コード）：
```python
# verify_slack_logs.py

import os
import re
from slack_sdk import WebClient
from datetime import datetime, timedelta

# Slack API client
client = WebClient(token=os.environ["SLACK_BOT_TOKEN"])
channel_id = os.environ["SLACK_CHANNEL_ID"]

# Fetch messages from the last week
one_week_ago = (datetime.now() - timedelta(days=7)).timestamp()
messages = client.conversations_history(
    channel=channel_id,
    oldest=one_week_ago,
)

# Patterns to detect
sensitive_patterns = [
    r"password=[^[\s]+(?<!\[REDACTED\])",
    r"api_key=[^[\s]+(?<!\[REDACTED\])",
    r"Bearer [^[\s]+(?<!\[REDACTED\])",
    # ... other patterns
]

# Scan messages
issues = []
for message in messages["messages"]:
    text = message.get("text", "")
    for pattern in sensitive_patterns:
        if re.search(pattern, text, re.IGNORECASE):
            issues.append({
                "timestamp": message["ts"],
                "text": text,
                "pattern": pattern,
            })

# Generate report
if issues:
    print(f"WARNING: {len(issues)} potential issues found!")
    for issue in issues:
        print(f"- Timestamp: {issue['timestamp']}, Pattern: {issue['pattern']}")
else:
    print("OK: No issues found.")

# Count redacted messages
redacted_count = sum(1 for m in messages["messages"] if "[REDACTED]" in m.get("text", ""))
print(f"Redacted messages: {redacted_count}")
```

---

## 5. ドキュメント更新

### 5.1 更新が必要なドキュメント

- [ ] README.md（必要に応じて）
- [ ] docs/tasks/0055_command_output_redaction_for_slack/01_requirements.md（実装結果の反映）
- [ ] docs/tasks/0055_command_output_redaction_for_slack/02_architecture.md（実装結果の反映）
- [ ] docs/tasks/0055_command_output_redaction_for_slack/03_specification.md（実装結果の反映）
- [ ] docs/tasks/0055_command_output_redaction_for_slack/04_implementation_plan.md（進捗の更新）

### 5.2 新規作成が必要なドキュメント

- [ ] 運用手順書（デプロイ・監視・トラブルシューティング）
- [ ] セキュリティ検証手順書（Slack ログのスキャン方法）

---

## 6. タイムライン

### 6.1 全体スケジュール

| Phase | 内容 | 期間 | 担当者 | 開始日 | 完了予定日 |
|-------|------|------|--------|--------|----------|
| Phase 1 | 案2（CommandResult 作成時の Redaction） | 3日 | TBD | TBD | TBD |
| Phase 2 | 案1（RedactingHandler の拡張）- 基本実装 | 5日 | TBD | TBD | TBD |
| Phase 3 | 案1（RedactingHandler の拡張）- ログ記録の改善 | 3日 | TBD | TBD | TBD |
| Phase 4 | Fail-secure 改善 | 2日 | TBD | TBD | TBD |
| Phase 5 | 統合・E2E テスト | 3日 | TBD | TBD | TBD |
| デプロイ | 本番環境へのデプロイ | 2日 | TBD | TBD | TBD |
| 監視 | 安定性確認 | 7日 | TBD | TBD | TBD |

**合計期間**：約25日（3.5週間）

### 6.2 マイルストーン

| マイルストーン | 内容 | 完了予定日 |
|--------------|------|----------|
| M1 | Phase 1 完了（案2実装） | TBD |
| M2 | Phase 2 完了（案1基本実装） | TBD |
| M3 | Phase 3 完了（ログ記録改善） | TBD |
| M4 | Phase 4 完了（Fail-secure 改善） | TBD |
| M5 | Phase 5 完了（統合・E2E テスト） | TBD |
| M6 | 本番デプロイ完了 | TBD |
| M7 | 安定稼働確認 | TBD |

---

## 7. リスク管理

### 7.1 リスク一覧

| リスク | 発生確率 | 影響度 | 対策 |
|-------|---------|--------|------|
| RedactText の変更による既存機能への影響 | 中 | 高 | Phase 4 で慎重な影響範囲調査を実施 |
| パフォーマンス劣化 | 中 | 中 | ベンチマークテストで継続的に監視 |
| logging システムの複雑化 | 低 | 中 | 明確なドキュメント化と単体テスト |
| 本番環境での予期しないエラー | 低 | 高 | 段階的ロールアウトと監視 |
| テストカバレッジ不足 | 中 | 高 | カバレッジ目標 90% を設定 |

### 7.2 対策

**高リスク項目の対策**：
1. **RedactText の変更**：
   - 影響範囲を事前に調査
   - テストケースを追加
   - 段階的なロールアウト

2. **本番環境でのエラー**：
   - 十分なテストカバレッジ
   - 監視とアラートの設定
   - ロールバック計画の準備

---

## 8. 成功基準

### 8.1 機能要件

- ✅ CommandResult 作成時に機密情報が redact される
- ✅ RedactingHandler で `slog.KindAny` 型が処理される
- ✅ LogValuer と スライス型の redaction が動作する
- ✅ 再帰深度制限が正しく動作する
- ✅ Panic からの復旧が正しく動作する
- ✅ 失敗ログが Slack 以外の出力先に記録される
- ✅ 正規表現コンパイル失敗時に fail-secure 動作する

### 8.2 非機能要件

- ✅ テストカバレッジ：90% 以上
- ✅ パフォーマンス：
  - RedactingHandler のオーバーヘッド < 10%
  - ログ出力レイテンシ < 100ms
  - CPU 使用率増加 < 5%
  - メモリ使用量増加 < 5%
- ✅ セキュリティ：
  - すべての既知のパターンが redact される
  - 機密情報が Slack に漏洩しない

### 8.3 デプロイ要件

- ✅ 段階的ロールアウトが完了
- ✅ 本番環境で1週間の安定稼働を確認
- ✅ ドキュメントが完成

---

## 9. まとめ

### 9.1 実装の優先順位

1. **Phase 1（最優先）**：案2（CommandResult 作成時の redaction）
   - すぐに効果が得られる
   - 比較的簡単に実装可能

2. **Phase 2（高優先）**：案1（RedactingHandler の拡張）- 基本実装
   - より包括的な保護
   - 将来的な型にも対応

3. **Phase 3（高優先）**：案1（RedactingHandler の拡張）- ログ記録の改善
   - FR-2.4 要件の遵守

4. **Phase 4（中優先）**：Fail-secure 改善
   - セキュリティ強化

5. **Phase 5（必須）**：統合・E2E テスト
   - 品質保証

### 9.2 実装後の期待される効果

- すべてのログ出力（Slack、ファイル、コンソール）で一貫した機密情報保護
- 将来追加される構造化ログ型にも自動的に redaction を適用
- コーディングミスによる機密情報漏洩のリスクを最小化
- Defense in Depth（多層防御）の原則に従った堅牢なセキュリティ

### 9.3 次のステップ

1. Phase 1 の実装を開始
2. 各 Phase の完了後、次の Phase に進む前にレビューを実施
3. すべての Phase が完了したら、本番環境へのデプロイを実施
