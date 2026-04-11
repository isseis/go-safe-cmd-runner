# GSCR_SLACK_WEBHOOK_URL 廃止環境変数の完全削除 実装計画書

## 実装ステップ

- [ ] **1. `SlackWebhookURLEnvVar` 定数の削除**
  - ファイル: `internal/logging/pre_execution_error.go`
  - 内容: `SlackWebhookURLEnvVar = "GSCR_SLACK_WEBHOOK_URL"` 定数とそのコメントを削除

- [ ] **2. `DeprecatedSlackWebhookError` 型と `ErrDeprecatedSlackWebhook` の削除**
  - ファイル: `internal/runner/bootstrap/environment.go`
  - 内容: 以下を削除
    - `ErrDeprecatedSlackWebhook` 変数
    - `DeprecatedSlackWebhookError` 型定義
    - `Error()` メソッド
    - `Is()` メソッド

- [ ] **3. `ValidateSlackWebhookEnv` から deprecated チェックの削除**
  - ファイル: `internal/runner/bootstrap/environment.go`
  - 内容: `os.Getenv(logging.SlackWebhookURLEnvVar)` を使った if ブロックを削除

- [ ] **4. テストコードの更新**
  - ファイル: `internal/runner/bootstrap/environment_test.go`
  - 内容: 以下を削除・更新
    - テスト構造体の `oldURL` フィールド
    - `t.Setenv(logging.SlackWebhookURLEnvVar, tt.oldURL)` の呼び出し
    - `"deprecated env var set - error"` テストケース
    - `"deprecated env var with new vars - error"` テストケース

- [ ] **5. 日本語ドキュメントの更新**
  - ファイル: `docs/user/runner_command.ja.md`
  - 内容: 「`GSCR_SLACK_WEBHOOK_URL` からの移行」セクションを削除

- [ ] **6. 英語ドキュメントの更新**
  - ファイル: `docs/user/runner_command.md`
  - 内容: 「Migration from `GSCR_SLACK_WEBHOOK_URL`」セクションを削除

- [ ] **7. ビルド・テスト・Lint の確認**
  - `make build` の成功を確認
  - `make test` の全テストパスを確認
  - `make lint` のエラーなし完了を確認
