# 詳細設計書：コマンド出力の Slack 通知における機密情報 Redaction

## 0. ドキュメント概要

本詳細設計書は、要件定義書（01_requirements.md）およびアーキテクチャ設計書（02_architecture.md）に基づき、コマンド出力の Slack 通知における機密情報 redaction 機能の実装詳細を定義します。

**設計方針**:
- **Defense in Depth**：二重の防御層による多層防御
- **案1（主対策）**：RedactingHandler による `slog.KindAny` 型の処理
- **案2（補完策）**：CommandResult 作成時の早期 redaction
- **Fail-secure 原則**：redaction 失敗時は安全なプレースホルダーで置換
- **既存インフラの活用**：`redaction.Config` と `security.Validator` の再利用

## 1. データ構造設計

### 1.1 redaction.Config の拡張

既存の `redaction.Config` 構造体は以下の通りです：

```go
// internal/redaction/redactor.go

type Config struct {
    // Placeholder is the placeholder used for redaction (e.g., "[REDACTED]")
    // 02_architecture.md に基づき、LogPlaceholder と TextPlaceholder を統一
    Placeholder string

    // Patterns contains the sensitive patterns to detect
    Patterns *SensitivePatterns

    // KeyValuePatterns contains keys for key=value or header redaction
    KeyValuePatterns []string
}
```

**設計方針**：
- アーキテクチャ設計書（02_architecture.md）の 590-608 行目に基づき、`LogPlaceholder` と `TextPlaceholder` を単一の `Placeholder` フィールドに統一
- すべての redaction で `"[REDACTED]"` を使用（より明示的で視認性が高く、ログ検索が容易）
- エラー用プレースホルダーは定数として定義

**エラー用プレースホルダー**：

```go
// internal/redaction/redactor.go

// RedactionFailurePlaceholder is used when redaction itself fails
const RedactionFailurePlaceholder = "[REDACTION FAILED - OUTPUT SUPPRESSED]"
```

### 1.2 RedactionContext（新規）

再帰深度を管理するための内部コンテキスト構造体：

```go
// internal/redaction/redactor.go

// RedactionContext holds context information for recursive redaction
type RedactionContext struct {
    depth int  // Current recursion depth
}

// maxRedactionDepth is the maximum depth for recursive redaction
// to prevent infinite recursion and DoS attacks
const maxRedactionDepth = 10
```

**設計意図**：
- 無限再帰の防止（DoS 攻撃対策）
- 深度制限到達時は部分的に redact された値を返す（エラー扱いしない）
- 将来的に他のコンテキスト情報（処理時間制限など）を追加可能

### 1.3 security.ValidatorInterface の拡張

既存の `ValidatorInterface` に `SanitizeOutputForLogging` メソッドを追加：

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

**理由**：
- `GroupExecutor` がインターフェース経由で `SanitizeOutputForLogging` を呼び出せるようにする
- モックの実装が容易になり、テストが簡単に
- 疎結合を維持し、将来的な実装の差し替えが可能

**実装への影響**：
- `security.Validator` は既に `SanitizeOutputForLogging` メソッドを実装済み
- インターフェースに追加するだけでコンパイルが通る
- モック実装（`testing/testify_mocks.go`）の更新が必要

---

## 2. 案1：RedactingHandler の拡張設計

### 2.1 処理対象の型

#### 2.1.1 対応する型

```go
// 処理対象の型（疑似コード）
switch attr.Value.Kind() {
case slog.KindAny:
    // 以下の型を処理
    // 1. slog.LogValuer インターフェース実装型
    //    - common.CommandResult
    //    - audit.ExecutionResult（将来的に）
    // 2. スライス型
    //    - []common.CommandResult
    //    - []slog.LogValuer（汎用対応）
}
```

#### 2.1.2 LogValuer インターフェースの確認

```go
// slog.LogValuer インターフェース（標準ライブラリ）
type LogValuer interface {
    LogValue() slog.Value
}
```

`common.CommandResult` は `LogValue()` メソッドを実装しているため、このインターフェースを満たします。

#### 2.1.3 型判定のアルゴリズム

```go
// 疑似コード
// processKindAny is now a method of RedactingHandler
func (h *RedactingHandler) processKindAny(key string, value slog.Value, ctx RedactionContext) (slog.Attr, error) {
    anyValue := value.Any()

    // 1. LogValuer インターフェースのチェック
    if logValuer, ok := anyValue.(slog.LogValuer); ok {
        return h.processLogValuer(key, logValuer, ctx)
    }

    // 2. スライス型のチェック
    if isSlice(anyValue) {
        return h.processSlice(key, anyValue, ctx)
    }

    // 3. 未対応の型はスキップ
    return slog.Attr{Key: key, Value: value}, nil
}
```

### 2.2 LogValuer 処理の詳細

#### 2.2.1 処理フロー

**設計方針の変更**：
- `processLogValuer` および関連メソッドを `*Config` ではなく `*RedactingHandler` のメソッドとして定義
- **理由**：失敗時のログ記録に `failureLogger` へのアクセスが必要
- **影響範囲**：以下のメソッドも `*RedactingHandler` に移行
  - `redactLogAttributeWithContext`（`RedactLogAttribute` から呼ばれる内部実装）
  - `processKindAny`
  - `processLogValuer`
  - `processSlice`

**呼び出しチェイン**：
```
RedactingHandler.Handle()
  → RedactingHandler.redactLogAttributeWithContext()
    → (for KindAny) RedactingHandler.processKindAny()
      → RedactingHandler.processLogValuer()  // failureLogger にアクセス
      → RedactingHandler.processSlice()      // failureLogger にアクセス
```

**公開 API の変更なし**：
- `Config.RedactLogAttribute()` は既存の公開 API として維持
- 内部で `RedactingHandler` のメソッドを呼び出す形に変更

```go
// internal/redaction/redactor.go

// processLogValuer processes a LogValuer value and recursively redacts it
// This is now a method of RedactingHandler to access failureLogger
func (h *RedactingHandler) processLogValuer(key string, logValuer slog.LogValuer, ctx RedactionContext) (slog.Attr, error) {
    // 1. 再帰深度チェック
    if ctx.depth >= maxRedactionDepth {
        // 深度制限到達：部分的に redact された値を返す（エラーにしない）
        // Debug レベルでログ記録
        return slog.Attr{Key: key, Value: slog.AnyValue(logValuer)}, nil
    }

    // 2. LogValue() を呼び出し（panic からの復旧が必要）
    var resolvedValue slog.Value
    func() {
        defer func() {
            if r := recover(); r != nil {
                // Panic が発生した場合、安全なプレースホルダーで置換
                resolvedValue = slog.StringValue(RedactionFailurePlaceholder)
                // Warning レベルでログ記録（Slack 以外の出力先）
                h.failureLogger.WarnContext(context.Background(),
                    "Redaction failed due to panic in LogValue()",
                    "attribute_key", key,
                    "panic", r,
                    "stack_trace", string(debug.Stack()),
                    "output_destination", "stderr, file, audit",
                )
            }
        }()
        resolvedValue = logValuer.LogValue()
    }()

    // 3. 解決された値を再帰的に redact
    resolvedAttr := slog.Attr{Key: key, Value: resolvedValue}
    nextCtx := RedactionContext{depth: ctx.depth + 1}
    return h.redactLogAttributeWithContext(resolvedAttr, nextCtx), nil
}
```

**重要な設計ポイント**：
- **panic からの復旧**：`LogValue()` が panic しても継続可能
- **再帰深度制限**：DoS 攻撃を防ぐため、深度 10 で停止
- **深度制限到達時の動作**：
  - エラーを返さず、部分的に redact された値を返す
  - `slog.Debug` レベルでログ記録（監視・チューニング用）
  - 通常のデータ構造では深度 10 に達することは稀
- **失敗時の動作（Fail-secure）**：
  - Panic 発生時は `RedactionFailurePlaceholder` で全体を置換
  - `slog.Warn` レベルでログ記録（Slack 以外の出力先に記録）

#### 2.2.2 エラーハンドリングの詳細

**Panic 発生時のログ記録**：

```go
// 疑似コード
defer func() {
    if r := recover(); r != nil {
        resolvedValue = slog.StringValue(RedactionFailurePlaceholder)

        // Slack 以外の出力先（stderr、ファイル、監査ログ）に記録
        // 注意：RedactingHandler 自身のログは再帰を避けるため、
        //       非 RedactingHandler 経路を使用する必要がある
        logRedactionFailure(slog.LevelWarn, map[string]any{
            "attribute_key": key,
            "error": fmt.Sprintf("panic: %v", r),
            "stack_trace": string(debug.Stack()),
            "output_destination": "stderr, file, audit",
        })
    }
}()
```

**深度制限到達時のログ記録**：

```go
// 疑似コード
if ctx.depth >= maxRedactionDepth {
    // Debug レベルでログ記録（これは問題ではない）
    logDepthLimitReached(slog.LevelDebug, map[string]any{
        "attribute_key": key,
        "depth": maxRedactionDepth,
        "note": "This is not an error - DoS prevention measure",
    })

    return slog.Attr{Key: key, Value: slog.AnyValue(logValuer)}, nil
}
```

### 2.3 スライス処理の詳細

#### 2.3.1 処理フロー

```go
// internal/redaction/redactor.go

// processSlice processes a slice value and redacts LogValuer elements
// This is now a method of RedactingHandler to access failureLogger
func (h *RedactingHandler) processSlice(key string, sliceValue any, ctx RedactionContext) (slog.Attr, error) {
    // 1. 再帰深度チェック
    if ctx.depth >= maxRedactionDepth {
        // 深度制限到達：元の値を返す
        return slog.Attr{Key: key, Value: slog.AnyValue(sliceValue)}, nil
    }

    // 2. リフレクションでスライスの要素を取得
    rv := reflect.ValueOf(sliceValue)
    if rv.Kind() != reflect.Slice {
        // スライスでない場合はスキップ
        return slog.Attr{Key: key, Value: slog.AnyValue(sliceValue)}, nil
    }

    // 3. 各要素を処理
    processedElements := make([]any, 0, rv.Len())
    nextCtx := RedactionContext{depth: ctx.depth + 1}

    for i := 0; i < rv.Len(); i++ {
        element := rv.Index(i).Interface()

        // LogValuer インターフェースをチェック
        if logValuer, ok := element.(slog.LogValuer); ok {
            // LogValue() を呼び出して解決
            var resolvedValue slog.Value
            func() {
                defer func() {
                    if r != nil {
                        // Panic 発生時は安全なプレースホルダーで置換
                        resolvedValue = slog.StringValue(RedactionFailurePlaceholder)
                        // Warning レベルでログ記録
                        h.failureLogger.WarnContext(context.Background(),
                            "Redaction failed due to panic in LogValue() within slice",
                            "attribute_key", elementKey,
                            "slice_index", i,
                            "panic", r,
                            "stack_trace", string(debug.Stack()),
                            "output_destination", "stderr, file, audit",
                        )
                    }
                }()
                resolvedValue = logValuer.LogValue()
            }()

            // 解決された値を redact
            elementKey := fmt.Sprintf("%s[%d]", key, i)
            redactedAttr := h.redactLogAttributeWithContext(
                slog.Attr{Key: elementKey, Value: resolvedValue},
                nextCtx,
            )

            // 処理済みの値をスライスに追加
            processedElements = append(processedElements, redactedAttr.Value.Any())
        } else {
            // LogValuer でない要素はそのまま
            processedElements = append(processedElements, element)
        }
    }

    // 4. 処理済みスライスを返す（スライス型を維持）
    return slog.Attr{Key: key, Value: slog.AnyValue(processedElements)}, nil
}
```

**重要な設計ポイント**：
- **スライス型の維持**：処理後もスライス型として返す（オブジェクトやマップに変換しない）
- **要素ごとの処理**：各要素が LogValuer の場合のみ処理
- **再帰深度**：スライス内の各要素で深度をインクリメント
- **Panic からの復旧**：各要素の `LogValue()` 呼び出し時に panic をキャッチ

#### 2.3.2 型安全性の考慮

```go
// リフレクションを使用する際の型安全性チェック
func isSlice(value any) bool {
    if value == nil {
        return false
    }
    rv := reflect.ValueOf(value)
    return rv.Kind() == reflect.Slice
}
```

### 2.4 RedactLogAttribute の拡張

#### 2.4.1 既存の実装

```go
// 既存の RedactLogAttribute（簡略版）
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    switch attr.Value.Kind() {
    case slog.KindString:
        // 文字列を redact
        return c.redactStringAttribute(attr)
    case slog.KindGroup:
        // グループを再帰的に redact
        return c.redactGroupAttribute(attr)
    default:
        // その他はスルー
        return attr
    }
}
```

#### 2.4.2 拡張後の実装

```go
// internal/redaction/redactor.go

// RedactLogAttribute redacts sensitive information from a log attribute
// This is the public API entry point (maintained for backward compatibility)
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    // Note: This method delegates to RedactingHandler when used within logging context
    // For direct use without a handler, it processes only String and Group kinds
    return c.redactLogAttributeBasic(attr)
}

// redactLogAttributeBasic handles basic redaction without LogValuer support
func (c *Config) redactLogAttributeBasic(attr slog.Attr) slog.Attr {
    key := attr.Key
    value := attr.Value

    // Check for sensitive patterns in the key
    if c.Patterns.IsSensitiveKey(key) {
        return slog.Attr{Key: key, Value: slog.StringValue(c.Placeholder)}
    }

    // Process based on value kind
    switch value.Kind() {
    case slog.KindString:
        // Redact string values
        strValue := value.String()
        redactedText := c.RedactText(strValue)
        if redactedText != strValue {
            return slog.Attr{Key: key, Value: slog.StringValue(redactedText)}
        }
        if c.Patterns.IsSensitiveValue(strValue) {
            return slog.Attr{Key: key, Value: slog.StringValue(c.Placeholder)}
        }
        return attr

    case slog.KindGroup:
        // Handle group values recursively
        groupAttrs := value.Group()
        redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
        for _, groupAttr := range groupAttrs {
            redactedGroupAttrs = append(redactedGroupAttrs, c.redactLogAttributeBasic(groupAttr))
        }
        return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}

    default:
        // Other types: pass through (KindAny is handled by RedactingHandler)
        return attr
    }
}

// redactLogAttributeWithContext is the internal implementation with full redaction support
// This is now a method of RedactingHandler to support LogValuer and access failureLogger
func (h *RedactingHandler) redactLogAttributeWithContext(attr slog.Attr, ctx RedactionContext) slog.Attr {
    key := attr.Key
    value := attr.Value

    // Check for sensitive patterns in the key
    if h.config.Patterns.IsSensitiveKey(key) {
        return slog.Attr{Key: key, Value: slog.StringValue(h.config.Placeholder)}
    }

    // Process based on value kind
    switch value.Kind() {
    case slog.KindString:
        // Redact string values
        strValue := value.String()
        redactedText := h.config.RedactText(strValue)
        if redactedText != strValue {
            return slog.Attr{Key: key, Value: slog.StringValue(redactedText)}
        }
        if h.config.Patterns.IsSensitiveValue(strValue) {
            return slog.Attr{Key: key, Value: slog.StringValue(h.config.Placeholder)}
        }
        return attr

    case slog.KindGroup:
        // Handle group values recursively
        groupAttrs := value.Group()
        redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
        for _, groupAttr := range groupAttrs {
            redactedGroupAttrs = append(redactedGroupAttrs, h.redactLogAttributeWithContext(groupAttr, ctx))
        }
        return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}

    case slog.KindAny:
        // NEW: Handle KindAny (LogValuer, slices, etc.)
        processedAttr, err := h.processKindAny(key, value, ctx)
        if err != nil {
            // エラー時は安全なプレースホルダーで置換
            return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
        }
        return processedAttr

    default:
        // Other types: pass through
        return attr
    }
}
```

**変更点**：
1. **責任の分離**：
   - `Config.RedactLogAttribute()`：公開 API、基本的な redaction のみ（String, Group）
   - `RedactingHandler.redactLogAttributeWithContext()`：完全な redaction（LogValuer, スライス対応）
2. **KindAny の処理追加**：`processKindAny` を呼び出し
3. **エラーハンドリング**：処理失敗時は `RedactionFailurePlaceholder` で置換
4. **Placeholder の統一**：すべて `c.Placeholder` を使用

### 2.5 RedactText の Fail-secure 改善

アーキテクチャ設計書で指摘された通り、現在の `RedactText()` 実装は正規表現コンパイル失敗時に元のテキストを返す fail-open 動作となっています。これを fail-secure に修正します。

#### 2.5.1 現在の問題

```go
// 現在の実装（redactor.go:110-112, 136-139, 174-177）
re, err := regexp.Compile(regexPattern)
if err != nil {
    // Fallback to original text if regex compilation fails
    return text  // ❌ Fail-open: 機密情報が漏洩する可能性
}
```

#### 2.5.2 改善後の実装

```go
// internal/redaction/redactor.go

func (c *Config) performSpacePatternRedaction(text, pattern, placeholder string) string {
    escapedPattern := regexp.QuoteMeta(pattern)
    regexPattern := `(?i)(` + escapedPattern + `)(\S+)`

    re, err := regexp.Compile(regexPattern)
    if err != nil {
        // ✅ Fail-secure: 正規表現コンパイル失敗時は安全なプレースホルダーで全体を置換
        logRegexCompilationFailure(slog.LevelWarn, map[string]any{
            "pattern": pattern,
            "error": err.Error(),
            "output_destination": "stderr, file, audit",
        })
        return RedactionFailurePlaceholder
    }

    // 正常な処理...
    return re.ReplaceAllStringFunc(text, func(match string) string {
        // ...
    })
}

// 他の performXXXRedaction メソッドも同様に修正
```

**重要な変更点**：
- 正規表現コンパイル失敗時は `RedactionFailurePlaceholder` を返す
- `slog.Warn` レベルでエラーをログ記録（Slack 以外の出力先）
- これにより、機密情報が含まれる可能性がある場合は出力全体を抑制

---

## 3. 案2：CommandResult 作成時の Redaction 設計

### 3.1 GroupExecutor の変更点

#### 3.1.1 現在の実装

```go
// internal/runner/group_executor.go（簡略版）

func (ge *DefaultGroupExecutor) executeAllCommands(...) ([]common.CommandResult, error) {
    // ...

    cmdResult := common.CommandResult{
        CommandResultFields: common.CommandResultFields{
            Name:     cmdSpec.Name,
            ExitCode: exitCode,
            Output:   stdout,   // ❌ 生データ
            Stderr:   stderr,   // ❌ 生データ
        },
    }

    results = append(results, cmdResult)
    // ...
}
```

#### 3.1.2 改善後の実装

```go
// internal/runner/group_executor.go

func (ge *DefaultGroupExecutor) executeAllCommands(...) ([]common.CommandResult, error) {
    // ...

    // ✅ 案2：CommandResult 作成時に redact
    sanitizedStdout := ge.validator.SanitizeOutputForLogging(stdout)
    sanitizedStderr := ge.validator.SanitizeOutputForLogging(stderr)

    cmdResult := common.CommandResult{
        CommandResultFields: common.CommandResultFields{
            Name:     cmdSpec.Name,
            ExitCode: exitCode,
            Output:   sanitizedStdout,  // ✅ Redact 済み
            Stderr:   sanitizedStderr,  // ✅ Redact 済み
        },
    }

    results = append(results, cmdResult)
    // ...
}
```

**変更点**：
- `stdout`, `stderr` を `CommandResult` に格納する前に `SanitizeOutputForLogging` を呼び出し
- `validator` は既に `DefaultGroupExecutor` のフィールドとして存在
- `ValidatorInterface` に `SanitizeOutputForLogging` メソッドを追加する必要がある

### 3.2 security.Validator の実装確認

既に `SanitizeOutputForLogging` メソッドは実装済みです：

```go
// internal/runner/security/logging_security.go

func (v *Validator) SanitizeOutputForLogging(output string) string {
    if output == "" {
        return ""
    }

    // Redact sensitive information if enabled
    if v.config.LoggingOptions.RedactSensitiveInfo {
        output = v.redactSensitivePatterns(output)
    }

    // Truncate stdout if configured
    if v.config.LoggingOptions.TruncateStdout &&
       v.config.LoggingOptions.MaxStdoutLength > 0 &&
       len(output) > v.config.LoggingOptions.MaxStdoutLength {
        output = output[:v.config.LoggingOptions.MaxStdoutLength] +
                 "...[truncated for security]"
    }

    return output
}
```

**確認事項**：
- ✅ 既に実装済み
- ✅ `redactSensitivePatterns` は `redaction.Config.RedactText` を使用
- ⚠️ `ValidatorInterface` への追加が必要

### 3.3 redactSensitivePatterns の Fail-secure 改善

`redactSensitivePatterns` は内部で `redaction.Config.RedactText()` を呼び出しています。セクション 2.5 で `RedactText()` を fail-secure に改善するため、`redactSensitivePatterns` も自動的に fail-secure になります。

```go
// internal/runner/security/logging_security.go

func (v *Validator) redactSensitivePatterns(text string) string {
    // Use the new common redaction functionality
    result := v.redactionConfig.RedactText(text)

    // RedactText() が RedactionFailurePlaceholder を返した場合、
    // その値がそのまま返される（fail-secure）
    return result
}
```

**追加のエラーハンドリング**：

```go
func (v *Validator) SanitizeOutputForLogging(output string) string {
    if output == "" {
        return ""
    }

    // Redact sensitive information if enabled
    if v.config.LoggingOptions.RedactSensitiveInfo {
        redacted := v.redactSensitivePatterns(output)

        // Redaction が失敗した場合のチェック
        if redacted == redaction.RedactionFailurePlaceholder {
            // Warning レベルでログ記録（Slack 以外の出力先）
            logRedactionFailure(slog.LevelWarn, map[string]any{
                "context": "SanitizeOutputForLogging",
                "output_destination": "stderr, file, audit",
            })
        }

        output = redacted
    }

    // Truncate stdout if configured...

    return output
}
```

---

## 4. エラー型定義

### 4.1 エラー型の追加

既存の `redaction` パッケージにはエラー型が定義されていないため、新規に追加します：

```go
// internal/redaction/errors.go

package redaction

import "fmt"

// ErrRedactionDepthExceeded is returned when recursion depth limit is reached
// Note: この型は内部使用のみで、実際には depth 到達時もエラーを返さない
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

**設計意図**：
- **明示的なエラー型**：`errors.Is()` や `errors.As()` でエラーの種類を判定可能
- **詳細な情報**：デバッグと監査に必要な情報を含む
- **内部使用**：これらのエラー型は主にログ記録とテストで使用

---

## 5. ログ記録の設計

### 5.1 失敗時のログ記録（FR-2.4 要件）

アーキテクチャ設計書の FR-2.4 要件に従い、redaction の失敗は **Slack 以外の出力先**（stderr、ファイルログ、監査ログ）に記録する必要があります。

#### 5.1.1 課題

`RedactingHandler` 自身がログを出力すると、再帰的に `RedactingHandler.Handle()` が呼ばれる可能性があります。これを避けるため、非 RedactingHandler 経路でログを記録する必要があります。

#### 5.1.2 解決策：専用ロガーの使用

```go
// internal/redaction/redactor.go

// RedactingHandler は専用のロガーを持つ
type RedactingHandler struct {
    handler       slog.Handler
    config        *Config
    failureLogger *slog.Logger  // NEW: 失敗ログ用の専用ロガー
}

// NewRedactingHandler creates a new redacting handler
func NewRedactingHandler(handler slog.Handler, config *Config, failureLogger *slog.Logger) *RedactingHandler {
    if config == nil {
        config = DefaultConfig()
    }
    // failureLogger が nil の場合はデフォルトロガーを使用
    if failureLogger == nil {
        failureLogger = slog.Default()
    }
    return &RedactingHandler{
        handler:       handler,
        config:        config,
        failureLogger: failureLogger,
    }
}

// processLogValuer 内で失敗ログを記録
func (r *RedactingHandler) processLogValuerInternal(...) {
    defer func() {
        if rec := recover(); rec != nil {
            // 専用ロガーを使用（RedactingHandler を経由しない）
            r.failureLogger.WarnContext(ctx,
                "Redaction failed due to panic in LogValue()",
                "attribute_key", key,
                "panic", rec,
                "stack_trace", string(debug.Stack()),
                "output_destination", "stderr, file, audit",
            )
            resolvedValue = slog.StringValue(RedactionFailurePlaceholder)
        }
    }()
    // ...
}
```

**実装のポイント**：
- `failureLogger` は RedactingHandler を経由しない専用ロガー
- 初期化時に MultiHandler の構成を調整し、Slack 以外の出力先のみを含むロガーを渡す
- これにより、失敗ログが Slack に送信されることを防ぐ

#### 5.1.3 logging システムでの初期化例

```go
// 疑似コード：logging システムでの初期化

func setupLogging() {
    // 各 handler を作成
    stderrHandler := slog.NewTextHandler(os.Stderr, nil)
    fileHandler := newFileHandler("/var/log/runner/runner.log")
    slackHandler := newSlackHandler(slackWebhookURL)

    // Redaction 用の失敗ログハンドラー（Slack を含まない）
    failureLogHandler := logging.NewMultiHandler(stderrHandler, fileHandler)
    failureLogger := slog.New(failureLogHandler)

    // RedactingHandler を作成（failureLogger を渡す）
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

### 5.2 深度制限到達時のログ記録

深度制限到達は DoS 攻撃防止のための正常な動作なので、Debug レベルでログ記録します：

```go
// internal/redaction/redactor.go

func (c *Config) processLogValuer(key string, logValuer slog.LogValuer, ctx RedactionContext) (slog.Attr, error) {
    if ctx.depth >= maxRedactionDepth {
        // Debug レベルでログ記録（これは問題ではない）
        slog.DebugContext(context.Background(),
            "Recursion depth limit reached - returning partially redacted value",
            "attribute_key", key,
            "depth", maxRedactionDepth,
            "note", "This is not an error - DoS prevention measure",
        )
        return slog.Attr{Key: key, Value: slog.AnyValue(logValuer)}, nil
    }
    // ...
}
```

**ログレベルの使い分け**：
- `slog.Debug`：深度制限到達（正常な動作）
- `slog.Warn`：redaction 失敗（異常な状態、要対応）

---

## 6. テスト設計

### 6.1 Unit Tests（RedactingHandler）

#### 6.1.1 テストケース一覧

| テストケース | 入力 | 期待される出力 | 目的 |
|------------|------|--------------|------|
| LogValuer single | `CommandResult{Output: "password=secret"}` | `Output: "password=[REDACTED]"` | LogValuer 処理の検証 |
| LogValuer slice | `[]CommandResult` with passwords | 各要素が redact される | スライス処理の検証 |
| Non-LogValuer | `slog.Any("data", 123)` | `123` (pass through) | 未対応型のスキップ |
| Deep recursion | Nested structures (depth=11) | Depth 10 で停止、部分的に redact | 再帰深度制限の検証 |
| Panic handling | `LogValue()` panics | `RedactionFailurePlaceholder` | Panic からの復旧 |
| Nil LogValue | `LogValue()` returns nil or empty | Handle gracefully | Nil 値の堅牢性 |
| Empty slice | `[]CommandResult{}` | Pass through empty | 空スライスの処理 |
| Mixed slice | LogValuer + non-LogValuer | Process only LogValuer | 混在型スライス |
| FR-2.4: Failure logging | `LogValue()` panics | `slog.Warn` to stderr/file (not Slack) | 失敗ログの出力先検証 |
| Regex compilation failure | Invalid regex pattern | `RedactionFailurePlaceholder` | Fail-secure 動作の検証 |

#### 6.1.2 テストコード例

```go
// internal/redaction/redactor_test.go

func TestRedactingHandler_LogValuerSingle(t *testing.T) {
    // Setup
    var buf bytes.Buffer
    handler := slog.NewJSONHandler(&buf, nil)
    config := DefaultConfig()
    redactingHandler := NewRedactingHandler(handler, config, nil)
    logger := slog.New(redactingHandler)

    // Test data
    cmdResult := common.CommandResult{
        CommandResultFields: common.CommandResultFields{
            Name:     "test_cmd",
            ExitCode: 0,
            Output:   "password=secret123",
            Stderr:   "",
        },
    }

    // Execute
    logger.Info("Command executed", "result", cmdResult)

    // Verify
    output := buf.String()
    assert.Contains(t, output, "password=[REDACTED]")
    assert.NotContains(t, output, "secret123")
}

func TestRedactingHandler_DeepRecursion(t *testing.T) {
    // Setup
    config := DefaultConfig()

    // Create deeply nested structure (depth > 10)
    type NestedLogValuer struct {
        depth int
        next  *NestedLogValuer
    }

    var createNested func(int) *NestedLogValuer
    createNested = func(d int) *NestedLogValuer {
        if d == 0 {
            return &NestedLogValuer{depth: d, next: nil}
        }
        return &NestedLogValuer{depth: d, next: createNested(d - 1)}
    }

    func (n *NestedLogValuer) LogValue() slog.Value {
        if n.next == nil {
            return slog.StringValue("leaf")
        }
        return slog.AnyValue(n.next)
    }

    nested := createNested(15)  // 深度 15
    attr := slog.Any("nested", nested)

    // Execute
    result := config.RedactLogAttribute(attr)

    // Verify: 深度 10 で停止するため、一部が処理されずに残る
    // （具体的な検証内容は実装に依存）
    assert.NotNil(t, result)
}

func TestRedactingHandler_PanicHandling(t *testing.T) {
    // Setup
    var buf bytes.Buffer
    handler := slog.NewJSONHandler(&buf, nil)
    config := DefaultConfig()

    // 失敗ログ用のバッファ
    var failureBuf bytes.Buffer
    failureHandler := slog.NewJSONHandler(&failureBuf, nil)
    failureLogger := slog.New(failureHandler)

    redactingHandler := NewRedactingHandler(handler, config, failureLogger)
    logger := slog.New(redactingHandler)

    // Test data: LogValue() が panic する
    type PanicLogValuer struct{}
    func (p PanicLogValuer) LogValue() slog.Value {
        panic("intentional panic for testing")
    }

    panicVal := PanicLogValuer{}

    // Execute
    logger.Info("Test message", "data", panicVal)

    // Verify
    output := buf.String()
    assert.Contains(t, output, RedactionFailurePlaceholder)

    // 失敗ログが記録されていることを確認
    failureLog := failureBuf.String()
    assert.Contains(t, failureLog, "Redaction failed")
    assert.Contains(t, failureLog, "panic")
}

func TestRedactText_RegexCompilationFailure(t *testing.T) {
    // Setup: 不正な正規表現パターンをテスト用に注入
    // 正規表現コンパイルを意図的に失敗させることで、fail-secure 動作を検証

    testCases := []struct {
        name            string
        keyValuePattern string  // 不正な正規表現パターン
        inputText       string
        expectedResult  string
    }{
        {
            name:            "unclosed_bracket",
            keyValuePattern: "password[",  // 閉じられていない [ はコンパイルエラー
            inputText:       "password=secret123",
            expectedResult:  RedactionFailurePlaceholder,
        },
        {
            name:            "invalid_repetition",
            keyValuePattern: "token*+",  // 不正な繰り返し指定
            inputText:       "token=abc123",
            expectedResult:  RedactionFailurePlaceholder,
        },
        {
            name:            "unclosed_parenthesis",
            keyValuePattern: "api_key(",  // 閉じられていない (
            inputText:       "api_key=xyz789",
            expectedResult:  RedactionFailurePlaceholder,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // 不正なパターンを含む Config を作成
            config := &Config{
                Placeholder:      "[REDACTED]",
                Patterns:         DefaultSensitivePatterns(),
                KeyValuePatterns: []string{tc.keyValuePattern},
            }

            // Execute: performKeyValueRedaction 内で regexp.Compile が失敗する
            result := config.RedactText(tc.inputText)

            // Verify: Fail-secure 動作により RedactionFailurePlaceholder を返す
            assert.Equal(t, tc.expectedResult, result,
                "正規表現コンパイル失敗時は RedactionFailurePlaceholder を返す必要がある")
        })
    }
}

func TestPerformKeyValueRedaction_InvalidPattern(t *testing.T) {
    // Setup: performKeyValueRedaction メソッドを直接テスト
    config := &Config{
        Placeholder: "[REDACTED]",
    }

    testCases := []struct {
        name     string
        pattern  string  // 不正な正規表現パターン
        text     string
        expected string
    }{
        {
            name:     "bracket_not_closed",
            pattern:  "secret[",
            text:     "secret=value",
            expected: RedactionFailurePlaceholder,
        },
        {
            name:     "invalid_escape",
            pattern:  "token\\k",  // 不正なエスケープシーケンス
            text:     "token=abc",
            expected: RedactionFailurePlaceholder,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Execute: 内部メソッドを直接テスト
            result := config.performKeyValueRedaction(tc.text, tc.pattern, config.Placeholder)

            // Verify
            assert.Equal(t, tc.expected, result)
        })
    }
}
```

### 6.2 Integration Tests

#### 6.2.1 テストケース一覧

| テストケース | シナリオ | 検証項目 |
|------------|---------|---------|
| Full pipeline | Command 実行 → GroupExecutor → Log → RedactingHandler → Slack | エンドツーエンドの redaction |
| Multi-handler | 同じログを Slack + File + Console に出力 | 一貫した redaction |
| Validator integration | GroupExecutor で `SanitizeOutputForLogging` を呼び出し | 案2の動作確認 |
| Both defenses | 案1・案2の両方が動作することを確認 | 二重防御の検証 |

#### 6.2.2 テストコード例

```go
// internal/runner/group_executor_test.go

func TestGroupExecutor_RedactionIntegration(t *testing.T) {
    // Setup: 実際の Validator と RedactingHandler を使用

    // Slack handler をモック
    mockSlackHandler := &MockSlackHandler{
        messages: make([]string, 0),
    }

    // RedactingHandler を設定
    config := redaction.DefaultConfig()
    redactingHandler := redaction.NewRedactingHandler(mockSlackHandler, config, nil)
    logger := slog.New(redactingHandler)

    // GroupExecutor を作成（Validator 付き）
    validator, _ := security.NewValidator(&security.Config{
        LoggingOptions: security.LoggingOptions{
            RedactSensitiveInfo: true,
        },
    })
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
```

### 6.3 E2E Tests

#### 6.3.1 テストケース一覧

| テストケース | シナリオ | 検証項目 |
|------------|---------|---------|
| Real command output | 実際のコマンド実行（API キーを含む） | Slack に送信されるデータの検証 |
| Known patterns | すべてのデフォルトパターンをテスト | パターンマッチングの網羅性 |
| Performance | 大量出力（10MB）の処理 | パフォーマンス劣化の確認 |

#### 6.3.2 テスト環境の要件

**重要**：E2E テストでは本番 Slack チャネルを使用しないこと。

1. **テスト専用 Slack ワークスペース/チャネル**：
   - 本番環境とは完全に分離
   - テストデータの投稿が許可された環境

2. **モック Slack エンドポイント**：
   - HTTP サーバーをローカルで起動
   - Slack API の動作をシミュレート
   - 送信されたペイロードをキャプチャして検証

3. **環境変数による切り替え**：
   ```bash
   # テスト環境
   SLACK_WEBHOOK_URL=http://localhost:8080/test-slack

   # 本番環境（E2E テストでは使用しない）
   SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
   ```

---

## 7. パフォーマンス設計

### 7.1 パフォーマンス目標

| メトリクス | 目標 | 測定方法 |
|----------|------|---------|
| RedactingHandler のオーバーヘッド | 10% 以内 | ベンチマークテスト |
| ログ出力レイテンシ | 100ms 以内 | 統合テスト |
| メモリ使用量増加 | 5% 以内 | プロファイリング |
| CPU 使用率増加 | 5% 以内 | プロファイリング |

### 7.2 最適化戦略

#### 7.2.1 早期リターン

```go
// Fast path for common cases
// This is now a method of RedactingHandler
func (h *RedactingHandler) redactLogAttributeWithContext(attr slog.Attr, ctx RedactionContext) slog.Attr {
    value := attr.Value

    // Fast path: 最も頻繁な型を先にチェック
    switch value.Kind() {
    case slog.KindString:
        // String は最も頻繁に使われるため、最初にチェック
        return h.redactStringAttribute(attr)
    case slog.KindGroup:
        // Group も比較的頻繁
        return h.redactGroupAttribute(attr, ctx)
    case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool:
        // プリミティブ型は redaction 不要、早期リターン
        return attr
    case slog.KindAny:
        // KindAny は頻度が低いため、最後にチェック
        processedAttr, err := h.processKindAny(attr.Key, value, ctx)
        if err != nil {
            // エラー時は安全なプレースホルダーで置換
            return slog.Attr{Key: attr.Key, Value: slog.StringValue(RedactionFailurePlaceholder)}
        }
        return processedAttr
    default:
        return attr
    }
}
```

#### 7.2.2 型判定のキャッシュ（将来的な最適化）

アーキテクチャ設計書で言及された通り、型判定結果をキャッシュすることで、リフレクションのコストを削減できます：

```go
// 将来的な最適化案（疑似コード）
type typeCache struct {
    mu    sync.RWMutex
    cache map[reflect.Type]typeInfo
}

type typeInfo struct {
    isLogValuer bool
    isSlice     bool
}

var globalTypeCache = &typeCache{
    cache: make(map[reflect.Type]typeInfo),
}

func (tc *typeCache) getTypeInfo(value any) typeInfo {
    t := reflect.TypeOf(value)

    // Read lock でキャッシュをチェック
    tc.mu.RLock()
    info, ok := tc.cache[t]
    tc.mu.RUnlock()

    if ok {
        return info
    }

    // キャッシュになければ判定を実行
    info = typeInfo{
        isLogValuer: implements(t, logValuerInterface),
        isSlice:     t.Kind() == reflect.Slice,
    }

    // Write lock でキャッシュに追加
    tc.mu.Lock()
    tc.cache[t] = info
    tc.mu.Unlock()

    return info
}
```

**注意**：
- 初期実装では型キャッシュを導入しない
- ベンチマークテストで必要性を確認してから実装

#### 7.2.3 正規表現の最適化

既に `redaction.Config` 内でコンパイル済みの正規表現を保持しているため、追加の最適化は不要です。

### 7.3 ベンチマークテスト

```go
// internal/redaction/redactor_test.go

func BenchmarkRedactingHandler_String(b *testing.B) {
    handler := slog.NewTextHandler(io.Discard, nil)
    config := DefaultConfig()
    redactingHandler := NewRedactingHandler(handler, config, nil)
    logger := slog.New(redactingHandler)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        logger.Info("test message", "data", "password=secret123")
    }
}

func BenchmarkRedactingHandler_LogValuer(b *testing.B) {
    handler := slog.NewTextHandler(io.Discard, nil)
    config := DefaultConfig()
    redactingHandler := NewRedactingHandler(handler, config, nil)
    logger := slog.New(redactingHandler)

    cmdResult := common.CommandResult{
        CommandResultFields: common.CommandResultFields{
            Name:     "test_cmd",
            ExitCode: 0,
            Output:   "password=secret123",
            Stderr:   "",
        },
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        logger.Info("test message", "result", cmdResult)
    }
}

func BenchmarkRedactingHandler_Slice(b *testing.B) {
    handler := slog.NewTextHandler(io.Discard, nil)
    config := DefaultConfig()
    redactingHandler := NewRedactingHandler(handler, config, nil)
    logger := slog.New(redactingHandler)

    results := []common.CommandResult{
        {CommandResultFields: common.CommandResultFields{Output: "password=secret1"}},
        {CommandResultFields: common.CommandResultFields{Output: "token=abc123"}},
        {CommandResultFields: common.CommandResultFields{Output: "api_key=xyz789"}},
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        logger.Info("test message", "results", results)
    }
}
```

**目標**：
- String/Group：既存と同等（オーバーヘッドなし）
- LogValuer/Slice：既存の String 処理の 2倍以内

---

## 8. セキュリティ設計

### 8.1 脅威モデル

#### 8.1.1 脅威1：機密情報の漏洩（現在の問題）

**攻撃シナリオ**：
1. コマンド出力に API キーが含まれる
2. Redaction が適用されない
3. Slack チャネルに投稿される
4. チャネルメンバー全員が閲覧可能

**対策**：
- 案1（RedactingHandler）+ 案2（早期 redaction）による二重防御
- 既知の機密情報パターンを自動検出
- Fail-secure 原則に基づく実装

#### 8.1.2 脅威2：Redaction のバイパス

**攻撃シナリオ**：
1. 攻撃者が特殊な文字エンコーディングを使用
2. Redaction パターンをバイパス
3. 機密情報が漏洩

**対策**：
- 正規表現パターンは大文字小文字を区別しない（`(?i)`）
- 柔軟なパターンマッチング（key=value, Bearer, Authorization: など）
- 将来的にはエンコーディング変換も検討

#### 8.1.3 脅威3：DoS 攻撃

**攻撃シナリオ**：
1. 攻撃者が深くネストした構造を送信
2. RedactingHandler が無限再帰に陥る
3. スタックオーバーフローまたはメモリ枯渇

**対策**：
- 再帰深度制限（maxRedactionDepth = 10）
- 深度制限到達時は部分的に redact された値を返す（サービス継続）
- Panic からの復旧

### 8.2 セキュリティテスト

#### 8.2.1 既知のパターンのテスト

すべての既知の機密情報パターンが redact されることを検証：

```go
func TestRedaction_AllKnownPatterns(t *testing.T) {
    config := redaction.DefaultConfig()

    testCases := []struct {
        name     string
        input    string
        expected string
    }{
        {"password", "password=secret123", "password=[REDACTED]"},
        {"token", "token=abc123xyz", "token=[REDACTED]"},
        {"api_key", "api_key=xyz789", "api_key=[REDACTED]"},
        {"Bearer", "Bearer eyJhbGc...", "Bearer [REDACTED]"},
        {"Basic", "Authorization: Basic dXNlcjpwYXNz", "Authorization: Basic [REDACTED]"},
        {"AWS_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY=wJalr...", "AWS_SECRET_ACCESS_KEY=[REDACTED]"},
        // ... 他のパターン
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result := config.RedactText(tc.input)
            assert.Equal(t, tc.expected, result)
        })
    }
}
```

#### 8.2.2 エッジケースのテスト

```go
func TestRedaction_EdgeCases(t *testing.T) {
    config := redaction.DefaultConfig()

    testCases := []struct {
        name     string
        input    string
        expected string
    }{
        {"mixed_case", "PaSsWoRd=secret", "PaSsWoRd=[REDACTED]"},
        {"special_chars", `password="secret@123"`, `password=[REDACTED]`},
        {"multiline", "line1\npassword=secret\nline3", "line1\npassword=[REDACTED]\nline3"},
        {"empty_value", "password=", "password="},
        {"very_long", "password=" + strings.Repeat("x", 10000), "password=[REDACTED]"},
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result := config.RedactText(tc.input)
            assert.Equal(t, tc.expected, result)
        })
    }
}
```

---

## 9. 設定とカスタマイズ

### 9.1 Redaction の有効化/無効化

既存の `security.Config` を使用：

```go
// internal/runner/security/config.go

type LoggingOptions struct {
    RedactSensitiveInfo    bool  // Redaction の有効/無効
    IncludeErrorDetails    bool  // エラー詳細の出力
    TruncateStdout         bool  // 出力の切り詰め
    MaxStdoutLength        int   // 最大出力長
    MaxErrorMessageLength  int   // 最大エラーメッセージ長
}
```

**デフォルト値**：
```go
func DefaultLoggingOptions() LoggingOptions {
    return LoggingOptions{
        RedactSensitiveInfo:    true,  // デフォルトで有効
        IncludeErrorDetails:    true,
        TruncateStdout:         false,
        MaxStdoutLength:        10000,
        MaxErrorMessageLength:  500,
    }
}
```

### 9.2 Redaction パターンのカスタマイズ

現時点では `DefaultSensitivePatterns()` と `DefaultKeyValuePatterns()` を使用：

```go
// internal/redaction/sensitive_patterns.go

func DefaultKeyValuePatterns() []string {
    return []string{
        "password",
        "token",
        "secret",
        "api_key",
        "apikey",
        "access_key",
        "private_key",
        "Bearer ",
        "Basic ",
        "Authorization:",
        "X-API-Key:",
        // ... 他のパターン
    }
}
```

**将来的な拡張**：
- TOML 設定ファイルでカスタムパターンを追加
- ユーザー定義のパターンをサポート

### 9.3 再帰深度の設定

```go
// internal/redaction/redactor.go

// maxRedactionDepth is the maximum depth for recursive redaction
const maxRedactionDepth = 10
```

**カスタマイズ**：
- 現時点では定数として固定
- 将来的に `Config` のフィールドとして設定可能にする余地を残す

---

## 10. 実装の依存関係

### 10.1 パッケージ依存関係

```
internal/redaction
├── (標準ライブラリ)
│   ├── context
│   ├── log/slog
│   ├── reflect
│   ├── regexp
│   └── runtime/debug
└── (既存の依存なし)

internal/runner/security
├── internal/redaction  ← 既に依存
└── internal/common

internal/runner
├── internal/runner/security
├── internal/common
└── internal/runner/runnertypes
```

### 10.2 新規追加するファイル

```
internal/redaction/
├── errors.go           (新規)

internal/runner/security/
└── interfaces.go       (既存、拡張)
```

### 10.3 変更するファイル

```
internal/redaction/
├── redactor.go         (拡張)
└── redactor_test.go    (拡張)

internal/runner/
├── group_executor.go   (変更)
└── group_executor_test.go (拡張)

internal/runner/security/
├── interfaces.go       (拡張)
├── logging_security.go (変更)
└── logging_security_test.go (拡張)
```

---

## 11. まとめ

### 11.1 設計の利点

1. **Defense in Depth**：二重の防御層により、高い安全性を確保
2. **包括的な保護**：すべてのログ出力で一貫した redaction
3. **Fail-secure**：失敗時も機密情報を漏らさない設計
4. **既存インフラの活用**：`redaction.Config` と `security.Validator` を再利用
5. **保守性**：明確な責任分離とインターフェースベースの設計

### 11.2 実装の優先順位

1. **Phase 1（優先度：高）**：Placeholder の統一
   - `LogPlaceholder` と `TextPlaceholder` を単一の `Placeholder` に統一
   - 設計方針に基づく基盤整備
   - 後続のフェーズの実装をシンプルにする

2. **Phase 2（優先度：高）**：案2（CommandResult 作成時の redaction）
   - 比較的簡単に実装可能
   - すぐに効果が得られる

3. **Phase 3（優先度：高）**：案1（RedactingHandler の拡張）
   - より包括的な保護
   - 将来的な型にも対応

4. **Phase 4（優先度：中）**：RedactText の fail-secure 改善
   - セキュリティ強化
   - 慎重な影響範囲調査が必要

### 11.3 次のステップ

実装計画書（04_implementation_plan.md）では以下を定義します：
- 具体的な実装手順
- マイルストーンとスケジュール
- テスト計画の詳細
- デプロイメント計画
