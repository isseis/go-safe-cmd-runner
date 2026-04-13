# 実装計画: Slack webhook URL ホスト allowlist

- [ ] 1. TOML・設定構造体の拡張 (AC-L2-1, AC-L2-3, AC-L2-4)
  - [ ] `internal/runner/runnertypes/spec.go` の `GlobalSpec` に `SlackAllowedHost string` フィールドを追加
  - [ ] `internal/runner/bootstrap/logger.go` の `LoggerConfig` に `SlackAllowedHost string` フィールドを追加
  - [ ] `internal/runner/bootstrap/environment.go` の `SetupLoggingOptions` に `SlackAllowedHost string` フィールドを追加

- [ ] 2. Slack ホスト検証ロジックの実装 (AC-L2-2, AC-L2-5〜AC-L2-9)
  - [ ] `internal/logging/slack_handler.go` の `SlackHandlerOptions` に `AllowedHost string` フィールドを追加
  - [ ] `validateWebhookURL(webhookURL string)` → `validateWebhookURL(webhookURL, allowedHost string)` にシグネチャ変更
    - `allowedHost` が空の場合は `ErrInvalidWebhookURL` を返す (AC-L2-7)
    - `strings.ToLower(parsedURL.Hostname()) != strings.ToLower(allowedHost)` の場合は `ErrInvalidWebhookURL` を返す (AC-L2-5, AC-L2-6, AC-L2-8)
    - 既存の HTTPS スキーム・ホスト名存在チェックは維持する (AC-L2-9)
  - [ ] `NewSlackHandler` 内の `validateWebhookURL` 呼び出しに `opts.AllowedHost` を追加

- [ ] 3. 既存テストの修正 (AC-L2-18)
  - [ ] `internal/logging/slack_handler_test.go` の既存 `validateWebhookURL` テストに `allowedHost` 引数を追加
    - 正常系テスト: `allowedHost` に適切なホスト名 (例: `hooks.slack.com`) を設定
    - 異常系テスト (HTTPS チェック等): `allowedHost` に任意のホストを設定 (HTTPS チェックが先行するため到達しない)

- [ ] 4. ホスト検証テストの追加 (AC-L2-13〜AC-L2-17)
  - [ ] AC-L2-13: `allowedHost=""` の場合に `ErrInvalidWebhookURL` が返ることを確認
  - [ ] AC-L2-14: ホスト不一致 (`evil.example.com` vs `hooks.slack.com`) でエラーになることを確認
  - [ ] AC-L2-15: ホスト一致 (`hooks.slack.com`) で `nil` が返ることを確認
  - [ ] AC-L2-16: 大文字ホスト (`HOOKS.SLACK.COM`) が `hooks.slack.com` の許可設定で通過することを確認
  - [ ] AC-L2-17: ポート番号付き URL (`https://hooks.slack.com:443/...`) が正しく処理されることを確認
  - [ ] 各テストは `errors.Is(err, ErrInvalidWebhookURL)` で検証する

- [ ] 5. 段階的ロギング初期化の実装 (AC-L2-10, AC-L2-11, AC-L2-12)
  - [ ] `internal/runner/bootstrap/logger.go` に `AddSlackHandlers(config LoggerConfig) error` を追加
    - `SlackWebhookURLSuccess` が設定されている場合: `logging.NewSlackHandler` を呼び出し、エラーがあれば即座に返す
    - `SlackWebhookURLError` が設定されている場合: 同様に処理
    - Slack ハンドラを含む新しい `MultiHandler` を構築し `RedactingHandler` でラップして `slog.SetDefault` を更新
  - [ ] `internal/runner/bootstrap/environment.go` に `SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions) error` を追加
    - `slackConfig` の両 URL が空の場合は何もせず `nil` を返す
    - `AddSlackHandlers` が返したエラーを `PreExecutionError{Type: ErrorTypeConfigParsing}` にラップして返す (AC-L2-10)
  - [ ] `SetupLogging` は Phase 1 専用とし、Slack ハンドラを生成しないことを確認する

- [ ] 6. 起動フローの Phase 1/2 分割 (AC-L2-11, AC-L2-12)
  - [ ] `cmd/runner/main.go` の `SetupLogging` 呼び出しから `SlackWebhookURLSuccess/Error` を削除する
  - [ ] `LoadAndPrepareConfig` の直後に `SetupSlackLogging(slackConfig, SetupLoggingOptions{SlackAllowedHost: cfg.Global.SlackAllowedHost, ...})` を呼び出す
  - [ ] `SetupSlackLogging` のエラーを既存の設定エラー処理と同様に処理する

- [ ] 7. 伝播テストの追加 (AC-L2-19, AC-L2-20)
  - [ ] AC-L2-19: `SlackHandlerOptions.AllowedHost` への伝播確認
    - `bootstrap` パッケージ内で Slack handler factory を差し替え可能にし、受け取った `AllowedHost` が期待値と一致することを確認
  - [ ] AC-L2-20: 起動フロー統合テスト
    - 前提: `slackConfig.ErrorURL` = 有効 HTTPS URL、`SlackAllowedHost` = `""` (未設定)
    - 期待: `errors.As(err, &preExecErr)` かつ `preExecErr.Type == ErrorTypeConfigParsing`
