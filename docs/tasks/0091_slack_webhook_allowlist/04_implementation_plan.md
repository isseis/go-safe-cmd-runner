# 実装計画: Slack webhook URL ホスト allowlist

- [ ] 1. TOML・設定構造体の拡張 (AC-L2-1, AC-L2-19)
  - [ ] `internal/runner/runnertypes/spec.go` に `SlackAllowedHost` を追加
  - [ ] `internal/runner/bootstrap/logger.go` の `LoggerConfig` に追加
  - [ ] `internal/runner/bootstrap/environment.go` の `SetupLoggingOptions` に追加

- [ ] 2. Slack ホスト検証ロジックの実装 (AC-L2-5〜AC-L2-9, AC-L2-13〜AC-L2-18)
  - [ ] `internal/logging/slack_handler.go` の `SlackHandlerOptions` に `AllowedHost` を追加
  - [ ] `validateWebhookURL` に `allowedHost` 引数を追加し、検証ロジックを実装
  - [ ] `NewSlackHandler` 内で `validateWebhookURL` の呼び出しを修正
  - [ ] `internal/logging/slack_handler_test.go` にホスト検証のテストケースを追加

- [ ] 3. 新しい段階的ロギング設定の実装 (AC-L2-2, AC-L2-3, AC-L2-4, AC-L2-10)
  - [ ] `internal/runner/bootstrap/environment.go` に `SetupSlackLogging` を追加
  - [ ] `internal/runner/bootstrap/logger.go` に `AddSlackHandlers` を追加
  - [ ] テストのアップデート `internal/runner/bootstrap/environment_test.go` (AC-L2-19, AC-L2-20)

- [ ] 4. 起動フローの Phase 1/2 分割 (AC-L2-11, AC-L2-12)
  - [ ] `cmd/runner/main.go` の `main` 関数で `SetupLogging` に Slack URL を渡さないよう変更
  - [ ] 設定読み込み後、`SetupSlackLogging` を呼び出すように変更
