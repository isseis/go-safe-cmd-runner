# Slack Webhook 分離機能 詳細仕様書

## 1. 概要

本文書は、要件定義書（01_requirements.md）およびアーキテクチャ設計書（02_architecture.md）に基づき、実装に必要な詳細仕様を定義する。

## 2. 環境変数仕様

### 2.1 新規環境変数

#### 2.1.1 GSCR_SLACK_WEBHOOK_URL_SUCCESS

| 項目 | 値 |
|------|-----|
| 名前 | `GSCR_SLACK_WEBHOOK_URL_SUCCESS` |
| 型 | 文字列 |
| 必須 | 条件付き（ERROR が設定されている場合は任意） |
| デフォルト | 空文字列（未設定） |
| 検証 | HTTPS スキームのみ許可 |
| 用途 | 正常通知（INFO レベル）の送信先 |

#### 2.1.2 GSCR_SLACK_WEBHOOK_URL_ERROR

| 項目 | 値 |
|------|-----|
| 名前 | `GSCR_SLACK_WEBHOOK_URL_ERROR` |
| 型 | 文字列 |
| 必須 | 条件付き（SUCCESS が設定されている場合は必須） |
| デフォルト | 空文字列（未設定） |
| 検証 | HTTPS スキームのみ許可 |
| 用途 | 異常通知（WARN/ERROR レベル）の送信先 |

### 2.2 廃止環境変数

#### 2.2.1 GSCR_SLACK_WEBHOOK_URL

| 項目 | 値 |
|------|-----|
| 名前 | `GSCR_SLACK_WEBHOOK_URL` |
| 状態 | **廃止** |
| 検出時動作 | 起動エラー（Fail Fast） |

### 2.3 環境変数定数定義

```go
// internal/logging/pre_execution_error.go に追加

const (
    // SlackWebhookURLEnvVar is deprecated - kept for migration detection
    SlackWebhookURLEnvVar = "GSCR_SLACK_WEBHOOK_URL"

    // SlackWebhookURLSuccessEnvVar is the environment variable for success webhook
    SlackWebhookURLSuccessEnvVar = "GSCR_SLACK_WEBHOOK_URL_SUCCESS"

    // SlackWebhookURLErrorEnvVar is the environment variable for error webhook
    SlackWebhookURLErrorEnvVar = "GSCR_SLACK_WEBHOOK_URL_ERROR"
)
```

## 3. SlackHandler 拡張仕様

### 3.1 LevelMode 型定義

```go
// internal/logging/slack_handler.go に追加

// SlackHandlerLevelMode defines how the handler filters log levels
type SlackHandlerLevelMode int

const (
    // LevelModeDefault handles all levels >= configured level (existing behavior)
    LevelModeDefault SlackHandlerLevelMode = iota

    // LevelModeExactInfo handles only INFO level (for success webhook)
    LevelModeExactInfo

    // LevelModeWarnAndAbove handles only WARN and above (for error webhook)
    LevelModeWarnAndAbove
)
```

### 3.2 SlackHandlerOptions 拡張

```go
// SlackHandlerOptions holds configuration for creating a SlackHandler
type SlackHandlerOptions struct {
    WebhookURL    string
    RunID         string
    HTTPClient    *http.Client
    BackoffConfig BackoffConfig
    IsDryRun      bool
    LevelMode     SlackHandlerLevelMode  // NEW
}
```

### 3.3 SlackHandler 構造体拡張

```go
// SlackHandler is a slog.Handler that sends notifications to Slack
type SlackHandler struct {
    webhookURL    string
    runID         string
    httpClient    *http.Client
    level         slog.Level
    attrs         []slog.Attr
    groups        []string
    backoffConfig BackoffConfig
    isDryRun      bool
    levelMode     SlackHandlerLevelMode  // NEW
}
```

### 3.4 Enabled メソッド変更

**変更前:**
```go
func (s *SlackHandler) Enabled(_ context.Context, level slog.Level) bool {
    return level >= s.level
}
```

**変更後:**
```go
func (s *SlackHandler) Enabled(_ context.Context, level slog.Level) bool {
    switch s.levelMode {
    case LevelModeExactInfo:
        return level == slog.LevelInfo
    case LevelModeWarnAndAbove:
        return level >= slog.LevelWarn
    default:
        return level >= s.level
    }
}
```

### 3.5 NewSlackHandler 変更

```go
func NewSlackHandler(opts SlackHandlerOptions) (*SlackHandler, error) {
    if err := validateWebhookURL(opts.WebhookURL); err != nil {
        return nil, fmt.Errorf("invalid webhook URL: %w", err)
    }

    // Apply defaults for optional fields
    httpClient := opts.HTTPClient
    if httpClient == nil {
        httpClient = &http.Client{Timeout: httpTimeout}
    }

    backoffConfig := opts.BackoffConfig
    if backoffConfig.Base == 0 && backoffConfig.RetryCount == 0 {
        backoffConfig = DefaultBackoffConfig
    }

    slog.Debug("Creating Slack handler",
        slog.Bool("webhook_configured", opts.WebhookURL != ""),  // Don't log URL - it contains credentials
        slog.String("run_id", opts.RunID),
        slog.Duration("timeout", httpClient.Timeout),
        slog.Duration("backoff_base", backoffConfig.Base),
        slog.Int("retry_count", backoffConfig.RetryCount),
        slog.Bool("dry_run", opts.IsDryRun),
        slog.Int("level_mode", int(opts.LevelMode)))  // NEW

    return &SlackHandler{
        webhookURL:    opts.WebhookURL,
        runID:         opts.RunID,
        httpClient:    httpClient,
        level:         slog.LevelInfo,
        backoffConfig: backoffConfig,
        isDryRun:      opts.IsDryRun,
        levelMode:     opts.LevelMode,  // NEW
    }, nil
}
```

### 3.6 WithAttrs/WithGroup での levelMode 引き継ぎ

```go
func (s *SlackHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    if len(attrs) == 0 {
        return s
    }

    newAttrs := make([]slog.Attr, len(s.attrs)+len(attrs))
    copy(newAttrs, s.attrs)
    copy(newAttrs[len(s.attrs):], attrs)

    return &SlackHandler{
        webhookURL:    s.webhookURL,
        runID:         s.runID,
        httpClient:    s.httpClient,
        level:         s.level,
        attrs:         newAttrs,
        groups:        s.groups,
        backoffConfig: s.backoffConfig,
        isDryRun:      s.isDryRun,
        levelMode:     s.levelMode,  // NEW: preserve levelMode
    }
}

func (s *SlackHandler) WithGroup(name string) slog.Handler {
    if name == "" {
        return s
    }

    newGroups := make([]string, len(s.groups)+1)
    copy(newGroups, s.groups)
    newGroups[len(s.groups)] = name

    return &SlackHandler{
        webhookURL:    s.webhookURL,
        runID:         s.runID,
        httpClient:    s.httpClient,
        level:         s.level,
        attrs:         s.attrs,
        groups:        newGroups,
        backoffConfig: s.backoffConfig,
        isDryRun:      s.isDryRun,
        levelMode:     s.levelMode,  // NEW: preserve levelMode
    }
}
```

## 4. LoggerConfig 拡張仕様

### 4.1 LoggerConfig 構造体変更

**変更前:**
```go
type LoggerConfig struct {
    Level           slog.Level
    LogDir          string
    RunID           string
    SlackWebhookURL string
    ConsoleWriter   io.Writer
    DryRun          bool
}
```

**変更後:**
```go
type LoggerConfig struct {
    Level                  slog.Level
    LogDir                 string
    RunID                  string
    SlackWebhookURLSuccess string  // NEW: replaces SlackWebhookURL
    SlackWebhookURLError   string  // NEW
    ConsoleWriter          io.Writer
    DryRun                 bool
}
```

### 4.2 SetupLoggingOptions 構造体変更

`SetupLoggingOptions` は `SetupLogging` の引数として使用され、最終的に `LoggerConfig` に変換される。

**変更前:**
```go
// internal/runner/bootstrap/environment.go

type SetupLoggingOptions struct {
    LogLevel         slog.Level
    LogDir           string
    RunID            string
    ForceInteractive bool
    ForceQuiet       bool
    ConsoleWriter    io.Writer
    SlackWebhookURL  string    // Slack webhook URL for notifications
    DryRun           bool
}
```

**変更後:**
```go
type SetupLoggingOptions struct {
    LogLevel               slog.Level
    LogDir                 string
    RunID                  string
    ForceInteractive       bool
    ForceQuiet             bool
    ConsoleWriter          io.Writer
    SlackWebhookURLSuccess string  // NEW: replaces SlackWebhookURL
    SlackWebhookURLError   string  // NEW
    DryRun                 bool
}
```

### 4.3 SetupLogging 変更

```go
// internal/runner/bootstrap/environment.go

func SetupLogging(opts SetupLoggingOptions) error {
    loggerConfig := LoggerConfig{
        Level:                  opts.LogLevel,
        LogDir:                 opts.LogDir,
        RunID:                  opts.RunID,
        SlackWebhookURLSuccess: opts.SlackWebhookURLSuccess,  // CHANGED
        SlackWebhookURLError:   opts.SlackWebhookURLError,    // NEW
        ConsoleWriter:          opts.ConsoleWriter,
        DryRun:                 opts.DryRun,
    }

    if err := SetupLoggerWithConfig(loggerConfig, opts.ForceInteractive, opts.ForceQuiet); err != nil {
        return &logging.PreExecutionError{
            Type:      logging.ErrorTypeLogFileOpen,
            Message:   fmt.Sprintf("Failed to setup logger: %v", err),
            Component: string(resource.ComponentLogging),
            RunID:     opts.RunID,
        }
    }

    return nil
}
```

### 4.4 SetupLoggerWithConfig 変更

```go
func SetupLoggerWithConfig(config LoggerConfig, forceInteractive, forceQuiet bool) error {
    // ... existing code ...

    // 4. Slack notification handlers (optional)
    var slackSuccessHandler, slackErrorHandler slog.Handler

    // Create success handler if URL is provided
    if config.SlackWebhookURLSuccess != "" {
        sh, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
            WebhookURL: config.SlackWebhookURLSuccess,
            RunID:      config.RunID,
            IsDryRun:   config.DryRun,
            LevelMode:  logging.LevelModeExactInfo,
        })
        if err != nil {
            return fmt.Errorf("failed to create success Slack handler: %w", err)
        }
        slackSuccessHandler = sh
        handlers = append(handlers, sh)
    }

    // Create error handler if URL is provided
    if config.SlackWebhookURLError != "" {
        sh, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
            WebhookURL: config.SlackWebhookURLError,
            RunID:      config.RunID,
            IsDryRun:   config.DryRun,
            LevelMode:  logging.LevelModeWarnAndAbove,
        })
        if err != nil {
            return fmt.Errorf("failed to create error Slack handler: %w", err)
        }
        slackErrorHandler = sh
        handlers = append(handlers, sh)
    }

    // Create failure logger (excludes Slack handlers)
    failureHandlers := make([]slog.Handler, 0, len(handlers))
    for _, h := range handlers {
        if h != slackSuccessHandler && h != slackErrorHandler {
            failureHandlers = append(failureHandlers, h)
        }
    }

    // ... rest of existing code ...
}
```

## 5. 環境変数バリデーション仕様

### 5.1 バリデーション関数

```go
// internal/runner/bootstrap/environment.go に追加

// SlackWebhookConfig holds the validated Slack webhook configuration
type SlackWebhookConfig struct {
    SuccessURL string
    ErrorURL   string
}

// ErrDeprecatedSlackWebhook is returned when the deprecated env var is set
var ErrDeprecatedSlackWebhook = errors.New("GSCR_SLACK_WEBHOOK_URL is deprecated")

// ErrSuccessWithoutError is returned when SUCCESS is set but ERROR is not
var ErrSuccessWithoutError = errors.New("GSCR_SLACK_WEBHOOK_URL_SUCCESS requires GSCR_SLACK_WEBHOOK_URL_ERROR")

// ValidateSlackWebhookEnv validates Slack webhook environment variables
func ValidateSlackWebhookEnv() (*SlackWebhookConfig, error) {
    // Check for deprecated environment variable
    if os.Getenv(logging.SlackWebhookURLEnvVar) != "" {
        return nil, fmt.Errorf("%w: please migrate to GSCR_SLACK_WEBHOOK_URL_SUCCESS and GSCR_SLACK_WEBHOOK_URL_ERROR",
            ErrDeprecatedSlackWebhook)
    }

    successURL := os.Getenv(logging.SlackWebhookURLSuccessEnvVar)
    errorURL := os.Getenv(logging.SlackWebhookURLErrorEnvVar)

    // Validate combinations
    if successURL != "" && errorURL == "" {
        return nil, fmt.Errorf("%w: error notifications must be enabled",
            ErrSuccessWithoutError)
    }

    // Both empty is valid (Slack disabled)
    // ERROR only is valid (no success notifications)
    // Both set is valid

    return &SlackWebhookConfig{
        SuccessURL: successURL,
        ErrorURL:   errorURL,
    }, nil
}
```

### 5.2 エラーメッセージフォーマット

```go
// FormatDeprecatedSlackWebhookError formats the error message for deprecated env var
func FormatDeprecatedSlackWebhookError() string {
    return `Error: GSCR_SLACK_WEBHOOK_URL is deprecated.

Please migrate to the new webhook configuration:
  export GSCR_SLACK_WEBHOOK_URL_SUCCESS="<your_webhook_url>"
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"

For more information, see the migration guide at:
  https://github.com/isseis/go-safe-cmd-runner/docs/user/runner_command.md#slack-webhook-configuration`
}

// FormatSuccessWithoutErrorError formats the error message for invalid config
func FormatSuccessWithoutErrorError() string {
    return `Error: Invalid Slack webhook configuration.

GSCR_SLACK_WEBHOOK_URL_SUCCESS is set but GSCR_SLACK_WEBHOOK_URL_ERROR is not.
Error notifications must be enabled to prevent silent failures.

Please set GSCR_SLACK_WEBHOOK_URL_ERROR:
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"

To use the same webhook for both success and error notifications:
  export GSCR_SLACK_WEBHOOK_URL_SUCCESS="<your_webhook_url>"
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"`
}
```

## 6. TOML 設定検証仕様

### 6.1 TOML ローダー変更

既存の `loader.go` には `checkTemplateNameField` 関数があり、同様のパターン（TOML を `map[string]any` にパースして禁止フィールドをチェック）を使用している。一貫性を保つため、`slack_webhook_url` の検証も `loader.go` に追加する。

```go
// internal/runner/config/loader.go に追加

// ErrSlackWebhookInTOML is returned when slack_webhook_url is found in TOML
var ErrSlackWebhookInTOML = errors.New("slack_webhook_url in TOML is not supported")

// checkSlackWebhookField checks if the TOML content contains slack_webhook_url.
// This is done by parsing the TOML content as a map to detect fields that should
// be configured via environment variables for security reasons.
func checkSlackWebhookField(content []byte) error {
    var raw map[string]any
    if err := toml.Unmarshal(content, &raw); err != nil {
        // If we can't parse as map, the structured parse will also fail
        // so we can skip this check
        return nil
    }

    // Check global section for slack_webhook_url
    if global, ok := raw["global"].(map[string]any); ok {
        if _, exists := global["slack_webhook_url"]; exists {
            return ErrSlackWebhookInTOML
        }
    }

    return nil
}
```

### 6.2 loadConfigInternal への統合

```go
// internal/runner/config/loader.go の loadConfigInternal に追加

func (l *Loader) loadConfigInternal(content []byte) (*runnertypes.ConfigSpec, error) {
    // Parse the config content
    var cfg runnertypes.ConfigSpec
    if err := toml.Unmarshal(content, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    // Check for prohibited "name" field in command_templates
    if err := checkTemplateNameField(content); err != nil {
        return nil, err
    }

    // Check for prohibited "slack_webhook_url" in global section
    if err := checkSlackWebhookField(content); err != nil {
        return nil, err
    }

    // ... rest of validation ...
}
```

### 6.3 エラーメッセージフォーマット

```go
// internal/runner/bootstrap/environment.go に追加
// FormatSlackWebhookInTOMLError formats the error message
func FormatSlackWebhookInTOMLError() string {
    return `Error: slack_webhook_url in TOML configuration is not supported.

Webhook URLs contain sensitive credentials and should not be stored in
version-controlled configuration files.

Please use environment variables instead:
  export GSCR_SLACK_WEBHOOK_URL_SUCCESS="<your_webhook_url>"
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"

Remove 'slack_webhook_url' from your TOML configuration file to continue.`
}
```

## 7. Runner 変更仕様

### 7.1 logGroupExecutionSummary 変更

**変更前:**
```go
func (r *Runner) logGroupExecutionSummary(groupSpec *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
    slog.Info(
        "Command group execution completed",
        common.GroupSummaryAttrs.Group, groupSpec.Name,
        common.GroupSummaryAttrs.Status, result.status,
        common.GroupSummaryAttrs.Commands, result.commands,
        common.GroupSummaryAttrs.DurationMs, duration.Milliseconds(),
        "run_id", r.runID,
        "slack_notify", true,
        "message_type", "command_group_summary",
    )
}
```

**変更後:**
```go
func (r *Runner) logGroupExecutionSummary(groupSpec *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
    // Determine log level based on execution status
    logLevel := slog.LevelInfo
    if result.status == GroupExecutionStatusError {
        logLevel = slog.LevelError
    }

    slog.Log(context.Background(), logLevel,
        "Command group execution completed",
        common.GroupSummaryAttrs.Group, groupSpec.Name,
        common.GroupSummaryAttrs.Status, result.status,
        common.GroupSummaryAttrs.Commands, result.commands,
        common.GroupSummaryAttrs.DurationMs, duration.Milliseconds(),
        "run_id", r.runID,
        "slack_notify", true,
        "message_type", "command_group_summary",
    )
}
```

## 8. テスト仕様

### 8.1 SlackHandler LevelMode テスト

| テストID | テストケース | 入力 | 期待結果 |
|----------|-------------|------|---------|
| SH-LM-01 | LevelModeDefault + INFO | INFO | Enabled=true |
| SH-LM-02 | LevelModeDefault + WARN | WARN | Enabled=true |
| SH-LM-03 | LevelModeDefault + ERROR | ERROR | Enabled=true |
| SH-LM-04 | LevelModeExactInfo + INFO | INFO | Enabled=true |
| SH-LM-05 | LevelModeExactInfo + WARN | WARN | Enabled=false |
| SH-LM-06 | LevelModeExactInfo + ERROR | ERROR | Enabled=false |
| SH-LM-07 | LevelModeExactInfo + DEBUG | DEBUG | Enabled=false |
| SH-LM-08 | LevelModeWarnAndAbove + INFO | INFO | Enabled=false |
| SH-LM-09 | LevelModeWarnAndAbove + WARN | WARN | Enabled=true |
| SH-LM-10 | LevelModeWarnAndAbove + ERROR | ERROR | Enabled=true |

### 8.2 環境変数バリデーションテスト

| テストID | テストケース | SUCCESS | ERROR | OLD | 期待結果 |
|----------|-------------|---------|-------|-----|---------|
| ENV-01 | 両方設定 | ✓ | ✓ | - | 正常 |
| ENV-02 | ERROR のみ | - | ✓ | - | 正常 |
| ENV-03 | SUCCESS のみ | ✓ | - | - | ErrSuccessWithoutError |
| ENV-04 | 両方未設定 | - | - | - | 正常（Slack無効） |
| ENV-05 | 旧変数設定 | - | - | ✓ | ErrDeprecatedSlackWebhook |
| ENV-06 | 旧変数+新変数 | ✓ | ✓ | ✓ | ErrDeprecatedSlackWebhook |
| ENV-07 | 同一URL | URL_A | URL_A | - | 正常 |

### 8.3 TOML 設定検証テスト

| テストID | テストケース | 検証内容 |
|----------|-------------|---------|
| TOML-01 | slack_webhook_url 記述時 | TOML に slack_webhook_url がある場合、ErrSlackWebhookInTOML が返される |
| TOML-02 | slack_webhook_url なし | TOML に slack_webhook_url がない場合、正常にパースされる |

### 8.4 統合テスト

| テストID | テストケース | 検証内容 |
|----------|-------------|---------|
| INT-01 | 成功通知の振り分け | INFO ログが SUCCESS webhook にのみ送信される |
| INT-02 | 失敗通知の振り分け | ERROR ログが ERROR webhook にのみ送信される |
| INT-03 | WARN 通知の振り分け | WARN ログが ERROR webhook にのみ送信される |
| INT-04 | ERROR のみ設定時 | INFO ログは送信されない |
| INT-05 | dry-run モード | どちらの webhook にも送信されない |

### 8.5 受け入れ条件とテストの対応

| 受け入れ条件 | 対応テストID |
|-------------|-------------|
| AC-1 | ENV-01, INT-01, INT-02 |
| AC-2 | ENV-01, ENV-02, ENV-04 |
| AC-3 | INT-01, INT-02, INT-03 |
| AC-4 | ENV-02, ENV-03, ENV-04, INT-04 |
| AC-5 | ENV-05, TOML-01 |
| AC-6 | INT-05 |
| AC-7 | ENV-07 |

## 9. ドキュメント更新仕様

### 9.1 更新対象ドキュメント

| ドキュメント | 更新内容 |
|-------------|---------|
| docs/user/runner_command.md | 新しい環境変数の説明、移行手順 |
| docs/user/runner_command.ja.md | 同上（日本語版） |
| README.md | 環境変数一覧の更新 |
| README.ja.md | 同上（日本語版） |
| sample/dot.env.sample | 新しい環境変数のサンプル |

### 9.2 translation_glossary.md 更新

| 日本語 | 英語 |
|--------|------|
| 正常通知 | success notification |
| 異常通知 | error notification |
| Slack webhook 分離 | Slack webhook separation |
