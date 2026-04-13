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

- [ ] 5. Phase 1 ハンドラ状態の保持機構を追加 (AC-L2-11 の前提)
  - **背景**: 現行の `SetupLoggerWithConfig` はすべてのハンドラをローカル変数で組み立て `slog.SetDefault` まで完結させる。Phase 2 (`AddSlackHandlers`) が Slack ハンドラを追加するには、Phase 1 で作成したコンソール・ファイルハンドラ群を後から参照できる必要がある。
  - [ ] `internal/runner/bootstrap/logger.go` にパッケージレベルの `phase1BaseHandlers []slog.Handler` 変数を追加する
    - `SetupLoggerWithConfig` の末尾で Slack ハンドラを除いたハンドラ群 (= 既存の `failureHandlers` と同一集合) を `phase1BaseHandlers` に保存する
    - `phase1BaseHandlers` は `AddSlackHandlers` のみが読み取り専用で参照する
  - [ ] `phase1BaseHandlers` が nil のとき `AddSlackHandlers` を呼び出したらエラーを返すガードを追加する

- [ ] 6. 段階的ロギング初期化の実装 (AC-L2-10, AC-L2-11, AC-L2-12)
  - [ ] `internal/runner/bootstrap/logger.go` の `SetupLoggerWithConfig` から Slack ハンドラ生成ブロック (現行 :133-164) を**削除**する
    - `LoggerConfig` から `SlackWebhookURLSuccess/Error` フィールド自体も削除する (Phase 2 専用の `AddSlackHandlers` に役割を移譲するため)
  - [ ] `internal/runner/bootstrap/environment.go` の `SetupLoggingOptions` から `SlackWebhookURLSuccess/Error` フィールドを削除し、`SetupLogging` の `LoggerConfig` 生成からも除去する
    - これにより `SetupLogging` は Phase 1 専用となりコンパイルレベルで Slack URL を受け付けなくなる
  - [ ] `internal/runner/bootstrap/logger.go` に `AddSlackHandlers(config LoggerConfig) error` を追加する
    - `phase1BaseHandlers` が nil の場合はエラーを返す
    - `config.SlackWebhookURLSuccess` が設定されている場合: `newSlackHandlerFunc` (後述タスク 7 で追加する factory 変数) で成功ハンドラを生成し、エラーがあれば即座に返す
    - `config.SlackWebhookURLError` が設定されている場合: 同様に処理
    - `phase1BaseHandlers` + Slack ハンドラを結合した `allHandlers` で `MultiHandler` → `RedactingHandler` を再構築し `slog.SetDefault` を更新する
    - `failureHandlers` (Slack 除外) も `phase1BaseHandlers` から再構築する
    - `redactionErrorCollector` と `redactionReporter` も再初期化する
  - [ ] `internal/runner/bootstrap/environment.go` に `SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions) error` を追加する
    - `slackConfig` の両 URL が空の場合は何もせず `nil` を返す
    - `LoggerConfig{SlackWebhookURLSuccess: slackConfig.SuccessURL, SlackWebhookURLError: slackConfig.ErrorURL, SlackAllowedHost: opts.SlackAllowedHost, RunID: opts.RunID, DryRun: opts.DryRun}` を組み立てて `AddSlackHandlers` を呼ぶ
    - `AddSlackHandlers` が返したエラーを `PreExecutionError{Type: ErrorTypeConfigParsing}` にラップして返す (AC-L2-10)

- [ ] 7. Slack handler factory の差し替え機構を追加 (AC-L2-19 のテスト前提)
  - **背景**: 現行の `logger.go:137-158` は `logging.NewSlackHandler` を直接呼び出しており、テストから受け取った `SlackHandlerOptions` を検査できない
  - [ ] `internal/runner/bootstrap/logger.go` にパッケージレベルの factory 変数を追加する
    ```go
    var newSlackHandlerFunc = logging.NewSlackHandler
    ```
  - [ ] `AddSlackHandlers` 内の `logging.NewSlackHandler` 直接呼び出しをすべて `newSlackHandlerFunc` 経由に変更する
  - [ ] テストファイル内で `newSlackHandlerFunc` を差し替えてキャプチャするヘルパーを用意し、テスト終了後に元の値に戻す

- [ ] 8. 起動フローの Phase 1/2 分割 (AC-L2-11, AC-L2-12)
  - [ ] `cmd/runner/main.go` の `SetupLogging` 呼び出しを確認し、削除済みの `SlackWebhookURLSuccess/Error` フィールドへの代入を除去する (タスク 6 の変更に伴うコンパイルエラー修正)
  - [ ] `LoadAndPrepareConfig` の直後に `SetupSlackLogging(slackConfig, SetupLoggingOptions{SlackAllowedHost: cfg.Global.SlackAllowedHost, RunID: runID, DryRun: dryRun})` を呼び出す
  - [ ] `SetupSlackLogging` のエラーを既存の設定エラー処理と同様にハンドルする

- [ ] 9. 伝播テストの追加 (AC-L2-19, AC-L2-20)
  - [ ] AC-L2-19: `SlackHandlerOptions.AllowedHost` への伝播確認
    - タスク 7 で追加した `newSlackHandlerFunc` をテスト内で差し替え、受け取った `SlackHandlerOptions.AllowedHost` が期待値と一致することを確認する
  - [ ] AC-L2-20: 起動フロー統合テスト
    - 前提: `slackConfig.ErrorURL` = 有効 HTTPS URL、`SetupLoggingOptions.SlackAllowedHost` = `""` (未設定)
    - `SetupSlackLogging` が返すエラーを `errors.As(err, &preExecErr)` で検査し `preExecErr.Type == ErrorTypeConfigParsing` であることを確認する
