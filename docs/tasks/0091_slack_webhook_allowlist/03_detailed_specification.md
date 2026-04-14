# 詳細仕様書: Slack webhook URL ホスト allowlist

## 1. 変更対象ファイル一覧

| ファイル | 変更種別 | 概要 |
|---------|---------|------|
| `internal/runner/runnertypes/spec.go` | 変更 | `GlobalSpec.SlackAllowedHost` フィールド追加 |
| `internal/logging/slack_handler.go` | 変更 | `SlackHandlerOptions.AllowedHost` 追加、`validateWebhookURL` シグネチャ変更 |
| `internal/runner/bootstrap/environment.go` | 変更 | `SetupLoggingOptions.SlackAllowedHost` 追加、`SetupSlackLogging` 新規追加 |
| `internal/runner/bootstrap/logger.go` | 変更 | `SlackLoggerConfig` 新規追加、`AddSlackHandlers` 新規追加 |
| `internal/runner/bootstrap/config.go` | 変更 | `slack_allowed_host` フォーマット検証を `LoadAndPrepareConfig` に追加 |
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
    // 値はポート番号を含まない純粋なホスト名であること (例: "hooks.slack.com")。
    // ポート番号付き ("hooks.slack.com:443") や前後の空白は設定エラーとなる。
    SlackAllowedHost string `toml:"slack_allowed_host"`
}
```

**フォーマット制約** (`slack_allowed_host` の有効な値):

| 条件 | 例 (TOML の値) | 正規化後 | 結果 |
|------|--------------|---------|------|
| ホスト名 | `"hooks.slack.com"` | `"hooks.slack.com"` | OK |
| IPv6 ブラケット記法 | `"[::1]"` | `"::1"` | OK (ブラケットを除去して正規化) |
| 空文字列 | `""` | `""` | OK (Slack 無効化) |
| ポート番号付き | `"hooks.slack.com:443"` | — | 設定エラー |
| 前後に空白 | `" hooks.slack.com "` | — | 設定エラー |
| スキーム付き | `"https://hooks.slack.com"` | — | 設定エラー |
| パス付き | `"hooks.slack.com/path"` | — | 設定エラー |

TOML の値をそのまま比較に使うのではなく、`LoadAndPrepareConfig` 内で `url.Hostname()` を用いて正規化した値を `cfg.Global.SlackAllowedHost` に書き戻す (§2.9 参照)。以降の全層 (`SetupLoggingOptions` → `SlackLoggerConfig` → `SlackHandlerOptions`) は正規化済みの値を参照するため、`validateWebhookURL` での `parsedURL.Hostname()` との比較が IPv6 を含む任意のホスト形式で正しく機能する。

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

**前提**: `allowedHost` はポート番号を含まない純粋なホスト名であること。この制約は呼び出し前の設定読み込み時 (§2.9) に検証済みであるため、`validateWebhookURL` 内では追加の正規化を行わず `strings.ToLower` による大文字小文字正規化のみを実施する。

実装仕様:
1. `webhookURL` が空 → `ErrInvalidWebhookURL` (既存)
2. URL パース失敗 → `ErrInvalidWebhookURL` (既存)
3. スキームが `https` でない → `ErrInvalidWebhookURL` (既存、AC-L2-9)
4. `Host` が空 → `ErrInvalidWebhookURL` (既存、AC-L2-9)
5. `allowedHost` が空文字列 → `ErrInvalidWebhookURL` (新規、AC-L2-7)
6. `strings.ToLower(parsedURL.Hostname())` が `strings.ToLower(allowedHost)` と一致しない → `ErrInvalidWebhookURL` (新規、AC-L2-5〜6, AC-L2-8)

比較方法 (AC-L2-6, AC-L2-7):

```go
if allowedHost == "" {
    return fmt.Errorf("%w: allowed host is not configured", ErrInvalidWebhookURL)
}
hostname := strings.ToLower(parsedURL.Hostname()) // ポート除去 + 小文字正規化
normalizedAllowedHost := strings.ToLower(allowedHost)
if hostname != normalizedAllowedHost {
    return fmt.Errorf("%w: host not allowed: %s (allowed: %s)", ErrInvalidWebhookURL, hostname, normalizedAllowedHost)
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

既存の `SlackWebhookURLSuccess/Error` フィールドを**削除**し、`SlackAllowedHost` を追加する (AC-L2-3)。Slack URL は `SetupLoggingOptions` では管理せず、`SetupSlackLogging` に渡す `SlackWebhookConfig` で別途受け取る。

```go
type SetupLoggingOptions struct {
    LogLevel         slog.Level
    LogDir           string
    RunID            string
    ForceInteractive bool
    ForceQuiet       bool
    ConsoleWriter    io.Writer
    DryRun           bool

    // 追加フィールド (AC-L2-3)
    // SlackAllowedHost は TOML から読んだ許可ホスト名。
    // SetupSlackLogging が SlackLoggerConfig.AllowedHost に転送する。
    SlackAllowedHost string
}
```

`SetupLogging` (Phase 1) は Slack フィールドを一切持たず、コンパイルレベルで Slack URL を受け付けない。Slack URL は Phase 2 の `SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions)` に `SlackWebhookConfig` として渡す (AC-L2-11)。

### 2.5 `LoggerConfig` (`internal/runner/bootstrap/logger.go`)

Phase 1 専用。Slack フィールドは一切持たない。既存フィールドから `SlackWebhookURLSuccess/Error` を**削除**し、新規フィールドは追加しない。

```go
type LoggerConfig struct {
    Level         slog.Level
    LogDir        string
    RunID         string
    ConsoleWriter io.Writer
    DryRun        bool
}
```

### 2.6 `SlackLoggerConfig` (新規追加、`internal/runner/bootstrap/logger.go`)

Phase 2 (`AddSlackHandlers`) 専用の設定構造体 (AC-L2-4)。`LoggerConfig` とは独立して定義することで、Phase 1/2 の責務を型レベルで分離する。

```go
// SlackLoggerConfig は AddSlackHandlers に渡す Slack ハンドラ専用の設定。
type SlackLoggerConfig struct {
    WebhookURLSuccess string // 成功通知用 webhook URL (INFO)
    WebhookURLError   string // エラー通知用 webhook URL (WARN/ERROR)
    AllowedHost       string // 許可ホスト名 (AC-L2-4)
    RunID             string
    DryRun            bool
}
```

### 2.7 `SetupSlackLogging` (新規追加、`internal/runner/bootstrap/environment.go`)

```go
// SetupSlackLogging は TOML 設定読み込み後に呼び出し、Slack ハンドラを追加する。
// ホスト検証に失敗した場合は ErrorTypeConfigParsing エラーを返す (AC-L2-10)。
func SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions) error
```

内部処理:
1. `slackConfig.SuccessURL == "" && slackConfig.ErrorURL == ""` の場合は何もせず `nil` を返す
2. 以下の `SlackLoggerConfig` を構築して `AddSlackHandlers` を呼ぶ:
   ```go
   SlackLoggerConfig{
       WebhookURLSuccess: slackConfig.SuccessURL,
       WebhookURLError:   slackConfig.ErrorURL,
       AllowedHost:       opts.SlackAllowedHost,
       RunID:             opts.RunID,
       DryRun:            opts.DryRun,
   }
   ```
3. `AddSlackHandlers` が返したエラーを `PreExecutionError{Type: ErrorTypeConfigParsing}` にラップして返す

### 2.8 `AddSlackHandlers` (新規追加、`internal/runner/bootstrap/logger.go`)

```go
// AddSlackHandlers は既存のデフォルトロガーに Slack ハンドラを追加して再構築する。
// successURL/errorURL どちらかでも validateWebhookURL が失敗した場合はエラーを返す。
func AddSlackHandlers(config SlackLoggerConfig) error
```

内部処理:
1. `phase1BaseHandlers` が nil の場合はエラーを返す
2. `config.WebhookURLSuccess` が設定されている場合:
   - `newSlackHandlerFunc(SlackHandlerOptions{AllowedHost: config.AllowedHost, ...})` を呼ぶ (AC-L2-12)
   - エラーがあれば即座に返す
3. `config.WebhookURLError` が設定されている場合: 同様
4. `phase1BaseHandlers` + Slack ハンドラを結合した `allHandlers` で新しい `MultiHandler` を構築する
5. Phase 1 で生成済みの `phase1FailureLogger` と `redactionErrorCollector` を使って `RedactingHandler` を再構築し、`slog.SetDefault` を更新する

**再利用するオブジェクト (Phase 1 で生成したものをそのまま継続使用する):**

- `phase1FailureLogger`: Slack を除外したハンドラ群 (`failureHandlers`) は Phase 1 と Phase 2 で内容が変わらないため、Phase 1 の `failureLogger` を継続使用する
- `redactionErrorCollector`: Phase 1 の動作中に蓄積されたエラー記録を保持するため、再初期化しない
- `redactionReporter`: `phase1FailureLogger` および `redactionErrorCollector` が変わらないため、再初期化しない

### 2.9 `slack_allowed_host` 正規化・検証 (`internal/runner/bootstrap/config.go`)

`LoadAndPrepareConfig` 内で `cfg.Global.SlackAllowedHost` を **正規化しつつ検証** する。正規化済みの値を `cfg.Global.SlackAllowedHost` に書き戻すことで、以降の全層が一貫した正規化値を参照できる。

**正規化関数** (検証と正規化を兼ねる):

```go
// normalizeSlackAllowedHost は host を正規化された許可ホスト名に変換する。
// host が空文字列の場合は ("", nil) を返す (Slack 無効)。
// 正規化は url.Parse を経由した Hostname() の取得で行う。
// これにより IPv6 ブラケット記法 ([::1] → ::1) も適切に処理される。
// ポート番号・スキーム・パス・空白など不正な値は error を返す。
func normalizeSlackAllowedHost(host string) (string, error) {
    if host == "" {
        return "", nil
    }
    u, err := url.Parse("https://" + host + "/")
    if err != nil || u.Hostname() == "" {
        return "", fmt.Errorf("slack_allowed_host must be a valid hostname without port or whitespace (got %q)", host)
    }
    return u.Hostname(), nil // 正規化済みホスト名 (ブラケット除去・スキーム除去済み)
}
```

正規化の例:

| TOML の値 | `u.Hostname()` | 結果 |
|----------|---------------|------|
| `"hooks.slack.com"` | `"hooks.slack.com"` | OK → そのまま |
| `"[::1]"` | `"::1"` | OK → ブラケット除去 |
| `"hooks.slack.com:443"` | `"hooks.slack.com"` → ポートが脱落し不正 | エラー (*) |
| `" hooks.slack.com "` | `""` | エラー |
| `"https://hooks.slack.com"` | `""` | エラー |

(*) ポート付きの場合は `u.Hostname()` がポートを除去するため見かけ上は有効に見えるが、スキームなしで `"https://" + host + "/"` を構築するため `"https://hooks.slack.com:443/"` となり `u.Hostname()` は `"hooks.slack.com"` を返す。この値は非空なのでエラーにならない点に注意が必要。ポートを明示的に拒否するには `u.Port() != ""` でガードを追加する:

```go
func normalizeSlackAllowedHost(host string) (string, error) {
    if host == "" {
        return "", nil
    }
    u, err := url.Parse("https://" + host + "/")
    if err != nil || u.Hostname() == "" || u.Port() != "" {
        return "", fmt.Errorf("slack_allowed_host must be a valid hostname without port or whitespace (got %q)", host)
    }
    return u.Hostname(), nil
}
```

`LoadAndPrepareConfig` での利用:

```go
normalizedHost, err := normalizeSlackAllowedHost(cfg.Global.SlackAllowedHost)
if err != nil {
    return nil, &logging.PreExecutionError{
        Type:      logging.ErrorTypeConfigParsing,
        Message:   err.Error(),
        Component: string(resource.ComponentConfig),
        RunID:     runID,
    }
}
cfg.Global.SlackAllowedHost = normalizedHost // 正規化済み値に更新
```

これにより `validateWebhookURL` での `parsedURL.Hostname()` との比較が、通常のホスト名・IPv6 を問わず正しく機能する (`parsedURL.Hostname()` もブラケットを除去するため両辺が同じ形式になる)。

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
if err := AddSlackHandlers(slackLoggerConfig); err != nil {
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
