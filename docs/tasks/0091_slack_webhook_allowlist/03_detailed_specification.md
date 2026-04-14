# 詳細仕様書: Slack webhook URL ホスト allowlist

## 1. 変更対象ファイル一覧

| ファイル | 変更種別 | 概要 |
|---------|---------|------|
| `internal/runner/runnertypes/spec.go` | 変更 | `GlobalSpec.SlackAllowedHost` フィールド追加 |
| `internal/logging/slack_handler.go` | 変更 | `SlackHandlerOptions.AllowedHost` 追加、`validateWebhookURL` シグネチャ変更 |
| `internal/runner/bootstrap/environment.go` | 変更 | `SetupLoggingOptions.SlackAllowedHost` 追加、`SetupSlackLogging` 新規追加 |
| `internal/runner/bootstrap/logger.go` | 変更 | `LoggerConfig.SlackAllowedHost` 追加、`AddSlackHandlers` 新規追加 |
| `cmd/runner/main.go` | 変更 | Phase 1/2 に分割した起動フローへ変更 |

---

## 2. インターフェース変更詳細

### 2.1 `GlobalSpec` (`internal/runner/runnertypes/spec.go`)

```go
type GlobalSpec struct {
    // 既存フィールド (省略)

    // 追加フィールド (AC-L2-1)
    // SlackAllowedHost は Slack webhook URL で許可するホスト名。
    // 空文字列の場合 Slack 通知機能は使用不可となる。
    SlackAllowedHost string `toml:"slack_allowed_host"`
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
    // AllowedHost は webhook URL のホスト名を検証する許可ホスト。
    // 空文字列の場合、すべての URL がエラーを返す。
    AllowedHost string
}
```

### 2.3 `validateWebhookURL` (`internal/logging/slack_handler.go`)

シグネチャ変更 (AC-L2-5):

```go
// 変更前
func validateWebhookURL(webhookURL string) error

// 変更後
func validateWebhookURL(webhookURL string, allowedHost string) error
```

実装仕様:
1. `webhookURL` が空 → `ErrInvalidWebhookURL` (既存)
2. URL パース失敗 → `ErrInvalidWebhookURL` (既存)
3. スキームが `https` でない → `ErrInvalidWebhookURL` (既存、AC-L2-9)
4. `Host` が空 → `ErrInvalidWebhookURL` (既存、AC-L2-9)
5. `strings.ToLower(parsedURL.Hostname())` が `strings.ToLower(allowedHost)` と一致しない → `ErrInvalidWebhookURL` (新規、AC-L2-5〜8)

比較方法 (AC-L2-6):

```go
hostname := strings.ToLower(parsedURL.Hostname()) // ポート除去 + 小文字正規化
if hostname != strings.ToLower(allowedHost) {
    return fmt.Errorf("%w: host not allowed: %s (allowed: %s)", ErrInvalidWebhookURL, hostname, allowedHost)
}
return nil
```

呼び出し側の変更:

```go
// NewSlackHandler 内
if err := validateWebhookURL(opts.WebhookURL, opts.AllowedHost); err != nil {
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
    // SlackAllowedHost は webhook URL のホスト名許可設定。
    // TOML 設定読み込み後に SetupSlackLogging 経由で設定する。
    SlackAllowedHost string
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
    SlackAllowedHost string
}
```

### 2.6 `SetupSlackLogging` (新規追加、`internal/runner/bootstrap/environment.go`)

```go
// SetupSlackLogging は TOML 設定読み込み後に呼び出し、Slack ハンドラを追加する。
// ホスト検証に失敗した場合は ErrorTypeConfigParsing エラーを返す (AC-L2-10)。
func SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions) error
```

内部処理:
1. `slackConfig.SuccessURL == "" && slackConfig.ErrorURL == ""` の場合は何もせず `nil` を返す
2. `slackConfig` から URL を、`opts` から `SlackAllowedHost` などの共通ロギング設定を取り出して `LoggerConfig{...}` を構築し、`AddSlackHandlers` を呼ぶ
3. `AddSlackHandlers` が返したエラーを `PreExecutionError{Type: ErrorTypeConfigParsing}` にラップして返す

### 2.7 `AddSlackHandlers` (新規追加、`internal/runner/bootstrap/logger.go`)

```go
// AddSlackHandlers は既存のデフォルトロガーに Slack ハンドラを追加して再構築する。
// successURL/errorURL どちらかでも validateWebhookURL が失敗した場合はエラーを返す。
func AddSlackHandlers(config LoggerConfig) error
```

内部処理:
1. `phase1BaseHandlers` が nil の場合はエラーを返す
2. `config.SlackWebhookURLSuccess` が設定されている場合:
   - `newSlackHandlerFunc(SlackHandlerOptions{AllowedHost: config.SlackAllowedHost, ...})` を呼ぶ (AC-L2-12)
   - エラーがあれば即座に返す
3. `config.SlackWebhookURLError` が設定されている場合: 同様
4. `phase1BaseHandlers` + Slack ハンドラを結合した `allHandlers` で新しい `MultiHandler` を構築する
5. Phase 1 で生成済みの `failureLogger` と `redactionErrorCollector` を使って `RedactingHandler` を再構築し、`slog.SetDefault` を更新する

**再利用するオブジェクト (Phase 1 で生成したものをそのまま継続使用する):**

- `failureLogger`: Slack を除外したハンドラ群 (`failureHandlers`) は Phase 1 と Phase 2 で内容が変わらないため、`phase1BaseHandlers` から再構築せず Phase 1 の `failureLogger` を継続使用する
- `redactionErrorCollector`: Phase 1 の動作中に蓄積されたエラー記録を保持するため、再初期化しない
- `redactionReporter`: `failureLogger` および `redactionErrorCollector` が変わらないため、再初期化しない

---

## 3. `cmd/runner/main.go` 起動フロー変更

### 3.1 変更前

```
ValidateSlackWebhookEnv()
SetupLogging(Slack URL を含む)          ← Slack ハンドラ生成
LoadAndPrepareConfig()                  ← TOML 読み込み (SlackAllowedHost ここで判明)
```

### 3.2 変更後

```
ValidateSlackWebhookEnv()               → slackConfig
SetupLogging(Slack URL を含まない)      ← Phase 1: コンソール・ファイルのみ
LoadAndPrepareConfig()                  ← TOML 読み込み
SetupSlackLogging(slackConfig, opts{    ← Phase 2: ホスト検証 + Slack ハンドラ追加
    SlackAllowedHost:       cfg.Global.SlackAllowedHost,
    RunID:                  runID,
    DryRun:                 dryRun,
    ...
})
```

`SetupLogging` の呼び出しから `SlackWebhookURLSuccess/Error` を削除し、`SetupSlackLogging` の呼び出しを `LoadAndPrepareConfig` の直後に追加する。Slack webhook URL 自体は `slackConfig` から受け取り、`opts` は許可ホストや `RunID` などの共通設定のみを渡す。

---

## 4. エラー型仕様

### 4.1 ホスト不一致エラー (AC-L2-10)

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
| AC-L2-13 | 許可ホスト未設定 | `url="https://hooks.slack.com/..."`, `allowedHost=""` | `ErrInvalidWebhookURL` |
| AC-L2-14 | ホスト不一致 | `url="https://evil.example.com/..."`, `allowedHost="hooks.slack.com"` | `ErrInvalidWebhookURL` |
| AC-L2-15 | ホスト一致 | `url="https://hooks.slack.com/..."`, `allowedHost="hooks.slack.com"` | `nil` |
| AC-L2-16 | 大文字ホスト | `url="https://HOOKS.SLACK.COM/..."`, `allowedHost="hooks.slack.com"` | `nil` |
| AC-L2-17 | ポート番号付き | `url="https://hooks.slack.com:443/..."`, `allowedHost="hooks.slack.com"` | `nil` |
| AC-L2-18 | HTTP スキーム | `url="http://hooks.slack.com/..."`, `allowedHost="hooks.slack.com"` | `ErrInvalidWebhookURL` |
| AC-L2-18 | ホスト名なし | `url="https:///path"`, `allowedHost="hooks.slack.com"` | `ErrInvalidWebhookURL` |

各テストは `errors.Is(err, ErrInvalidWebhookURL)` で検証する。

### 5.2 許可ホスト伝播テスト (`internal/runner/bootstrap/environment_test.go`)

AC-L2-19: `GlobalSpec.SlackAllowedHost` の値が `SlackHandlerOptions.AllowedHost` に到達することを確認する。

検証方法: `bootstrap` パッケージ内で Slack handler factory を差し替え可能にし、テストで受け取った `SlackHandlerOptions.AllowedHost` が期待値と一致することを確認する。

### 5.3 起動フロー統合テスト (AC-L2-20)

テスト対象: `SetupSlackLogging`

```
前提: slackConfig.ErrorURL = "https://evil.example.com/..." (有効な HTTPS URL)
      SetupLoggingOptions.SlackAllowedHost = "" (未設定)

期待: エラーが返される
    var preExecErr *logging.PreExecutionError
    errors.As(err, &preExecErr) == true
    preExecErr.Type == logging.ErrorTypeConfigParsing
```

### 5.4 既存テストの修正

`validateWebhookURL` のシグネチャ変更に伴い、`slack_handler_test.go` 内の既存テストに `allowedHost` 引数を追加する。

既存の正常系テストでは `allowedHost` に適切なホスト名を設定して呼び出す。既存の異常系テスト (HTTPS チェックなど) では `allowedHost` に任意のホストを設定してよい (HTTPS チェックが先行するため到達しない)。
