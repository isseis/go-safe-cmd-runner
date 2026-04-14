# 実装計画: Slack webhook URL ホスト allowlist

- [x] 1. TOML・設定構造体の拡張 (AC-L2-1, AC-L2-3, AC-L2-4)
  - [x] `internal/runner/runnertypes/spec.go` の `GlobalSpec` に `SlackAllowedHost string` フィールドを追加 (AC-L2-1)
  - [x] `internal/runner/bootstrap/environment.go` の `SetupLoggingOptions` に `SlackAllowedHost string` フィールドを追加 (AC-L2-3)
  - [x] `internal/runner/bootstrap/logger.go` に Phase 2 専用の `SlackLoggerConfig` 構造体を新規追加し `AllowedHost string` フィールドを持たせる (AC-L2-4、タスク 6 でも再掲)
  - [x] `internal/runner/bootstrap/config.go` の `LoadAndPrepareConfig` に `normalizeSlackAllowedHost` 呼び出しを追加する
    - 検証と正規化を兼ねる: `url.Parse("https://" + host + "/")` でパースし、`u.Hostname()` が空でなく `u.Port()` が空であることを確認する
    - IPv6 ブラケット記法 (`[::1]`) は `u.Hostname()` が `::1` を返すことで自動的に正規化される
    - 正規化済みの値を `cfg.Global.SlackAllowedHost` に書き戻すことで以降の全層が正規化値を参照する
    - 違反した場合は `ErrorTypeConfigParsing` にラップして返す (詳細仕様 §2.9)

- [x] 2. Slack ホスト検証ロジックの実装 (AC-L2-2, AC-L2-5〜AC-L2-9)
  - [x] `internal/logging/slack_handler.go` の `SlackHandlerOptions` に `AllowedHost string` フィールドを追加
  - [x] `validateWebhookURL(webhookURL string)` → `validateWebhookURL(webhookURL, allowedHost string)` にシグネチャ変更
    - `allowedHost` が空の場合は `ErrInvalidWebhookURL` を返す (AC-L2-7)
    - `strings.ToLower(parsedURL.Hostname()) != strings.ToLower(allowedHost)` の場合は `ErrInvalidWebhookURL` を返す (AC-L2-5, AC-L2-6, AC-L2-8)
    - 既存の HTTPS スキーム・ホスト名存在チェックは維持する (AC-L2-9)
  - [x] `NewSlackHandler` 内の `validateWebhookURL` 呼び出しに `opts.AllowedHost` を追加

- [x] 3. 既存テストの修正 (AC-L2-18)
  - [x] `internal/logging/slack_handler_test.go` の既存 `validateWebhookURL` テストに `allowedHost` 引数を追加
    - 正常系テスト: `allowedHost` に適切なホスト名 (例: `hooks.slack.com`) を設定
    - 異常系テスト (HTTPS チェック等): `allowedHost` に任意のホストを設定 (HTTPS チェックが先行するため到達しない)

- [x] 4. ホスト検証テストの追加 (AC-L2-13〜AC-L2-17)
  - [x] AC-L2-13: `allowedHost=""` の場合に `ErrInvalidWebhookURL` が返ることを確認
  - [x] AC-L2-14: ホスト不一致 (`evil.example.com` vs `hooks.slack.com`) でエラーになることを確認
  - [x] AC-L2-15: ホスト一致 (`hooks.slack.com`) で `nil` が返ることを確認
  - [x] AC-L2-16: 大文字ホスト (`HOOKS.SLACK.COM`) が `hooks.slack.com` の許可設定で通過することを確認
  - [x] AC-L2-17: ポート番号付き URL (`https://hooks.slack.com:443/...`) が正しく処理されることを確認
  - [x] 各テストは `errors.Is(err, ErrInvalidWebhookURL)` で検証する

- [x] 5. Phase 1 ハンドラ状態の保持機構を追加 (AC-L2-11 の前提)
  - **背景**: 現行の `SetupLoggerWithConfig` はすべてのハンドラをローカル変数で組み立て `slog.SetDefault` まで完結させる。Phase 2 (`AddSlackHandlers`) が Slack ハンドラを追加するには、Phase 1 で作成したコンソール・ファイルハンドラ群と `failureLogger` を後から参照できる必要がある。
  - [x] `internal/runner/bootstrap/logger.go` にパッケージレベルの変数を追加する
    - `phase1BaseHandlers []slog.Handler`: `SetupLoggerWithConfig` の末尾で Slack ハンドラを除いたハンドラ群 (= 既存の `failureHandlers` と同一集合) を保存する。`AddSlackHandlers` のみが読み取り専用で参照する
    - `phase1FailureLogger *slog.Logger`: Phase 1 で作成した `failureLogger` を保存する。`AddSlackHandlers` が `RedactingHandler` 再構築時に継続使用する
  - [x] `phase1BaseHandlers` が nil のとき `AddSlackHandlers` を呼び出したらエラーを返すガードを追加する

- [x] 6. 段階的ロギング初期化の実装 (AC-L2-10, AC-L2-11, AC-L2-12)
  - [x] `internal/runner/bootstrap/logger.go` の `SetupLoggerWithConfig` から Slack ハンドラ生成ブロックを**削除**する
    - `LoggerConfig` から `SlackWebhookURLSuccess/Error/AllowedHost` フィールドも**削除**する (`LoggerConfig` は Phase 1 専用)
  - [x] `internal/runner/bootstrap/logger.go` に `AddSlackHandlers(config SlackLoggerConfig) error` を完全実装する
    - `phase1BaseHandlers` が nil の場合はエラーを返す
    - `newSlackHandlerFunc` factory 経由でハンドラを生成し `slog.SetDefault` を更新する
    - `phase1FailureLogger` と `redactionErrorCollector` を継続使用する
  - [x] `internal/runner/bootstrap/environment.go` の `SetupLoggingOptions` から `SlackWebhookURLSuccess/Error` フィールドを削除する
  - [x] `internal/runner/bootstrap/environment.go` に `SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions) error` を追加する

- [x] 7. Slack handler factory の差し替え機構を追加 (AC-L2-19 のテスト前提)
  - [x] `internal/runner/bootstrap/logger.go` に `newSlackHandlerFunc` factory 変数を追加する
  - [x] `AddSlackHandlers` 内で `newSlackHandlerFunc` 経由で Slack ハンドラを生成する

- [x] 8. 起動フローの Phase 1/2 分割 (AC-L2-11, AC-L2-12)
  - [x] `cmd/runner/main.go` の `SetupLogging` から `SlackWebhookURLSuccess/Error` フィールドを削除する
  - [x] `LoadAndPrepareConfig` の直後に `SetupSlackLogging` を呼び出す
  - [x] `SetupSlackLogging` のエラーを既存の設定エラー処理と同様にハンドルする

- [x] 9. 伝播テストの追加 (AC-L2-19, AC-L2-20)
  - [x] AC-L2-19: `SlackHandlerOptions.AllowedHost` への伝播確認
    - タスク 7 で追加した `newSlackHandlerFunc` をテスト内で差し替え、受け取った `SlackHandlerOptions.AllowedHost` が期待値と一致することを確認する
  - [x] AC-L2-20: 起動フロー統合テスト
    - 前提: `slackConfig.ErrorURL` = 有効 HTTPS URL、`SetupLoggingOptions.SlackAllowedHost` = `""` (未設定)
    - `SetupSlackLogging` が返すエラーを `errors.As(err, &preExecErr)` で検査し `preExecErr.Type == ErrorTypeConfigParsing` であることを確認する
