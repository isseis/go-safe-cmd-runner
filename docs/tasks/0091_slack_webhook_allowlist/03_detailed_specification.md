# 詳細仕様書: Slack webhook URL ホスト allowlist

## 1. 変更対象ファイル一覧

| ファイル | 変更種別 | 概要 |
|---------|---------|------|
| `internal/runner/runnertypes/spec.go` | 変更 | `GlobalSpec.SlackAllowedHosts` フィールド追加 |
| `internal/logging/slack_handler.go` | 変更 | `SlackHandlerOptions.AllowedHosts` 追加、`validateWebhookURL` シグネチャ変更 |
| `internal/runner/bootstrap/environment.go` | 変更 | `SetupLoggingOptions.SlackAllowedHosts` 追加、`SetupSlackLogging` 新規追加 |
| `internal/runner/bootstrap/logger.go` | 変更 | `LoggerConfig.SlackAllowedHosts` 追加、`AddSlackHandlers` 新規追加 |
| `cmd/runner/main.go` | 変更 | Phase 1/2 に分割した起動フローへ変更 |

---

## 2. インターフェース変更詳細

### 2.1 `GlobalSpec` (`internal/runner/runnertypes/spec.go`)

```go
type GlobalSpec struct {
    // 既存フィールド (省略)

    // 追加フィールド (AC-L2-1)
    // SlackAllowedHosts は Slack webhook URL で許可するホスト名の一覧。
    // 空の場合 Slack 通知機能は使用不可となる。
    SlackAllowedHosts []string `toml:"slack_allowed_hosts"`
}
```

### 2.2 `SlackHandlerOptions` (`internal/logging/slack_handler.go`)

```go
type SlackHandlerOptions struct {
    WebhookURL    string
    RunID         string
    HTTPClient    *http.Client
    BackoffConfig BackoffConfig
    IsDryRun      bool
    LevelMode     SlackHandlerLevelMode

    // 追加フィールド (AC-L2-2)
    // AllowedHosts は webhook URL のホスト名を検証する allowlist。
    // 空の場合、すべての URL がエラーを返す。
    AllowedHosts []string
}
```

### 2.3 `validateWebhookURL` (`internal/logging/slack_handler.go`)

シグネチャ変更 (AC-L2-5):

```go
// 変更前
func validateWebhookURL(webhookURL string) error

// 変更後
func validateWebhookURL(webhookURL string, allowedHosts []string) error
```

実装仕様:
1. `webhookURL` が空 → `ErrInvalidWebhookURL` (既存)
2. URL パース失敗 → `ErrInvalidWebhookURL` (既存)
3. スキームが `https` でない → `ErrInvalidWebhookURL` (既存、AC-L2-9)
4. `Host` が空 → `ErrInvalidWebhookURL` (既存、AC-L2-9)
5. `strings.ToLower(parsedURL.Hostname())` が `allowedHosts` のいずれとも一致しない → `ErrInvalidWebhookURL` (新規、AC-L2-5〜8)

比較方法 (AC-L2-6):

```go
hostname := strings.ToLower(parsedURL.Hostname()) // ポート除去 + 小文字正規化
for _, allowed := range allowedHosts {
    if hostname == strings.ToLower(allowed) {
        return nil
    }
}
return fmt.Errorf("%w: host not in allowlist: %s", ErrInvalidWebhookURL, hostname)
```

呼び出し側の変更:

```go
// NewSlackHandler 内
if err := validateWebhookURL(opts.WebhookURL, opts.AllowedHosts); err != nil {
    return nil, fmt.Errorf("invalid webhook URL: %w", err)
}
```

### 2.4 `SetupLoggingOptions` (`internal/runner/bootstrap/environment.go`)

```go
type SetupLoggingOptions struct {
    LogLevel               slog.Level
    LogDir                 string
    RunID                  string
    ForceInteractive       bool
    ForceQuiet             bool
    ConsoleWriter          io.Writer
    SlackWebhookURLSuccess string
    SlackWebhookURLError   string
    DryRun                 bool

    // 追加フィールド (AC-L2-3)
    // SlackAllowedHosts は webhook URL のホスト名 allowlist。
    // TOML 設定読み込み後に SetupSlackLogging 経由で設定する。
    SlackAllowedHosts []string
}
```

`SetupLogging` は Phase 1 専用とし、Slack に関するフィールドを **使用しない**。Slack URL が渡されても、Phase 1 では Slack ハンドラを生成しない (AC-L2-11)。

### 2.5 `LoggerConfig` (`internal/runner/bootstrap/logger.go`)

```go
type LoggerConfig struct {
    Level                  slog.Level
    LogDir                 string
    RunID                  string
    SlackWebhookURLSuccess string
    SlackWebhookURLError   string
    ConsoleWriter          io.Writer
    DryRun                 bool

    // 追加フィールド (AC-L2-4)
    SlackAllowedHosts []string
}
```

### 2.6 `SetupSlackLogging` (新規追加、`internal/runner/bootstrap/environment.go`)

```go
// SetupSlackLogging は TOML 設定読み込み後に呼び出し、Slack ハンドラを追加する。
// allowlist 検証に失敗した場合は ErrorTypeConfigParsing エラーを返す (AC-L2-10)。
func SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions) error
```

内部処理:
1. `slackConfig.SuccessURL == "" && slackConfig.ErrorURL == ""` の場合は何もせず `nil` を返す
2. `LoggerConfig{SlackAllowedHosts: opts.SlackAllowedHosts, ...}` を構築して `AddSlackHandlers` を呼ぶ
3. `AddSlackHandlers` が返したエラーを `PreExecutionError{Type: ErrorTypeConfigParsing}` にラップして返す

### 2.7 `AddSlackHandlers` (新規追加、`internal/runner/bootstrap/logger.go`)

```go
// AddSlackHandlers は既存のデフォルトロガーに Slack ハンドラを追加して再構築する。
// successURL/errorURL どちらかでも validateWebhookURL が失敗した場合はエラーを返す。
func AddSlackHandlers(config LoggerConfig) error
```

内部処理:
1. `config.SlackWebhookURLSuccess` が設定されている場合:
   - `logging.NewSlackHandler(SlackHandlerOptions{AllowedHosts: config.SlackAllowedHosts, ...})` を呼ぶ (AC-L2-12)
   - エラーがあれば即座に返す
2. `config.SlackWebhookURLError` が設定されている場合: 同様
3. Slack ハンドラを含む新しい `MultiHandler` を構築し、`RedactingHandler` でラップして `slog.SetDefault` を更新する

---

## 3. `cmd/runner/main.go` 起動フロー変更

### 3.1 変更前

```
ValidateSlackWebhookEnv()
SetupLogging(Slack URL を含む)          ← Slack ハンドラ生成
LoadAndPrepareConfig()                  ← TOML 読み込み (SlackAllowedHosts ここで判明)
```

### 3.2 変更後

```
ValidateSlackWebhookEnv()               → slackConfig
SetupLogging(Slack URL を含まない)      ← Phase 1: コンソール・ファイルのみ
LoadAndPrepareConfig()                  ← TOML 読み込み
SetupSlackLogging(slackConfig, opts{    ← Phase 2: allowlist 検証 + Slack ハンドラ追加
    SlackAllowedHosts: cfg.Global.SlackAllowedHosts,
    SlackWebhookURLSuccess: slackConfig.SuccessURL,
    SlackWebhookURLError: slackConfig.ErrorURL,
    ...
})
```

`SetupLogging` の呼び出しから `SlackWebhookURLSuccess/Error` を削除し、`SetupSlackLogging` の呼び出しを `LoadAndPrepareConfig` の直後に追加する。

---

## 4. エラー型仕様

### 4.1 ホスト allowlist 違反エラー (AC-L2-10)

現状、`SetupLoggerWithConfig` が返したエラーは `environment.go:93-99` で `ErrorTypeLogFileOpen` にラップされる。Slack ホスト不許可は設定の問題であり `ErrorTypeConfigParsing` が適切である。

```go
// SetupSlackLogging での処理
if err := AddSlackHandlers(loggerConfig); err != nil {
    return &logging.PreExecutionError{
        Type:      logging.ErrorTypeConfigParsing,   // LogFileOpen ではなく ConfigParsing
        Message:   fmt.Sprintf("Slack webhook URL validation failed: %v", err),
        Component: string(resource.ComponentLogging),
        RunID:     opts.RunID,
    }
}
```

---

## 5. テスト仕様

### 5.1 `validateWebhookURL` のテスト (`internal/logging/slack_handler_test.go`)

| AC | テストケース | 入力 | 期待結果 |
|----|------------|------|---------|
| AC-L2-13 | allowlist 空 | `url="https://hooks.slack.com/..."`, `allowedHosts=[]` | `ErrInvalidWebhookURL` |
| AC-L2-14 | allowlist 不一致 | `url="https://evil.example.com/..."`, `allowedHosts=["hooks.slack.com"]` | `ErrInvalidWebhookURL` |
| AC-L2-15 | allowlist 一致 | `url="https://hooks.slack.com/..."`, `allowedHosts=["hooks.slack.com"]` | `nil` |
| AC-L2-16 | 大文字ホスト | `url="https://HOOKS.SLACK.COM/..."`, `allowedHosts=["hooks.slack.com"]` | `nil` |
| AC-L2-17 | ポート番号付き | `url="https://hooks.slack.com:443/..."`, `allowedHosts=["hooks.slack.com"]` | `nil` |
| AC-L2-18 | HTTP スキーム | `url="http://hooks.slack.com/..."`, `allowedHosts=["hooks.slack.com"]` | `ErrInvalidWebhookURL` |
| AC-L2-18 | ホスト名なし | `url="https:///path"`, `allowedHosts=["hooks.slack.com"]` | `ErrInvalidWebhookURL` |

各テストは `errors.Is(err, ErrInvalidWebhookURL)` で検証する。

### 5.2 allowlist 伝播テスト (`internal/runner/bootstrap/environment_test.go`)

AC-L2-19: `GlobalSpec.SlackAllowedHosts` の値が `SlackHandlerOptions.AllowedHosts` に到達することを確認する。

検証方法: `NewSlackHandler` をモック化し、受け取った `AllowedHosts` が期待値と一致することを確認する。

### 5.3 起動フロー統合テスト (AC-L2-20)

テスト対象: `SetupSlackLogging`

```
前提: slackConfig.ErrorURL = "https://evil.example.com/..." (有効な HTTPS URL)
      SetupLoggingOptions.SlackAllowedHosts = [] (空)

期待: エラーが返される
      errors.Is(err, logging.PreExecutionError) == true
      err.(*logging.PreExecutionError).Type == logging.ErrorTypeConfigParsing
```

### 5.4 既存テストの修正

`validateWebhookURL` のシグネチャ変更に伴い、`slack_handler_test.go` 内の既存テストに `allowedHosts` 引数を追加する。

既存の正常系テストでは `allowedHosts` に適切なホスト名を設定して呼び出す。既存の異常系テスト (HTTPS チェックなど) では `allowedHosts` に任意のホストを設定してよい (HTTPS チェックが先行するため allowlist は到達しない)。
