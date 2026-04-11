# GSCR_SLACK_WEBHOOK_URL 廃止環境変数の完全削除 要件定義書

## 1. 概要

### 1.1 背景

task 0068（Slack Webhook 分離機能）において、旧環境変数 `GSCR_SLACK_WEBHOOK_URL` は廃止（deprecated）とされた。廃止時の実装では、この変数が設定されている場合にエラーメッセージを出力してアプリケーションを即時終了させる「Fail Fast」アプローチが採用された。

移行猶予期間が経過したため、この廃止チェックコード自体と、それに関連するすべてのコード・ドキュメントを完全に削除する。

### 1.2 目的

- 廃止された環境変数 `GSCR_SLACK_WEBHOOK_URL` に関連するコードを完全に削除する
- 移行ガイドセクションをドキュメントから削除する
- コードベースをシンプルに保つ（YAGNI 原則）

### 1.3 スコープ外

- task 0068 で作成されたドキュメント（`docs/tasks/0068_separate_slack_webhooks/`）は歴史的記録として保持する
- `GSCR_SLACK_WEBHOOK_URL_SUCCESS`、`GSCR_SLACK_WEBHOOK_URL_ERROR` に関する変更は行わない

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 旧環境変数 | `GSCR_SLACK_WEBHOOK_URL`（task 0068 で廃止された変数） |
| Fail Fast チェック | 起動時に旧環境変数が設定されているかを確認し、設定されている場合にエラーで終了する処理 |

## 3. 機能要件

### 3.1 ソースコードの削除

#### FR-3.1.1: `SlackWebhookURLEnvVar` 定数の削除

`internal/logging/pre_execution_error.go` から以下の定数を削除する：

```go
// SlackWebhookURLEnvVar is deprecated - kept for migration detection
SlackWebhookURLEnvVar = "GSCR_SLACK_WEBHOOK_URL"
```

#### FR-3.1.2: `DeprecatedSlackWebhookError` 型の削除

`internal/runner/bootstrap/environment.go` から以下を削除する：

- `ErrDeprecatedSlackWebhook` 変数
- `DeprecatedSlackWebhookError` 型定義
- `DeprecatedSlackWebhookError.Error()` メソッド
- `DeprecatedSlackWebhookError.Is()` メソッド

#### FR-3.1.3: `ValidateSlackWebhookEnv` 関数から deprecated チェックの削除

`internal/runner/bootstrap/environment.go` の `ValidateSlackWebhookEnv` 関数から、旧環境変数チェック部分を削除する：

```go
// 削除対象
if os.Getenv(logging.SlackWebhookURLEnvVar) != "" {
    return nil, &DeprecatedSlackWebhookError{}
}
```

### 3.2 テストコードの削除

#### FR-3.2.1: deprecated 変数テストケースの削除

`internal/runner/bootstrap/environment_test.go` から以下のテストケースを削除する：

- `"deprecated env var set - error"` テストケース
- `"deprecated env var with new vars - error"` テストケース
- テスト構造体の `oldURL` フィールド
- `t.Setenv(logging.SlackWebhookURLEnvVar, tt.oldURL)` の呼び出し

### 3.3 ドキュメントの更新

#### FR-3.3.1: 日本語ドキュメントの更新

`docs/user/runner_command.ja.md` から以下のセクションを削除する：

- 「`GSCR_SLACK_WEBHOOK_URL` からの移行」セクション（移行手順のコードブロックを含む）

#### FR-3.3.2: 英語ドキュメントの更新

`docs/user/runner_command.md` から以下のセクションを削除する：

- 「Migration from `GSCR_SLACK_WEBHOOK_URL`」セクション（Migration Steps のコードブロックを含む）

## 4. 非機能要件

### 4.1 後方互換性

旧環境変数 `GSCR_SLACK_WEBHOOK_URL` が設定されている状態でrunnerを実行しても、エラーにならず無視されること（削除後は単に未使用の環境変数として扱われる）。

### 4.2 既存機能への影響なし

`GSCR_SLACK_WEBHOOK_URL_SUCCESS` および `GSCR_SLACK_WEBHOOK_URL_ERROR` の動作は変更しない。

## 5. 受け入れ基準

### AC-1: `SlackWebhookURLEnvVar` 定数の削除

- [ ] `internal/logging/pre_execution_error.go` に `SlackWebhookURLEnvVar` が存在しないこと
- [ ] `SlackWebhookURLEnvVar` を参照するコードが存在しないこと

### AC-2: `DeprecatedSlackWebhookError` 型の削除

- [ ] `internal/runner/bootstrap/environment.go` に `ErrDeprecatedSlackWebhook` が存在しないこと
- [ ] `internal/runner/bootstrap/environment.go` に `DeprecatedSlackWebhookError` が存在しないこと

### AC-3: `ValidateSlackWebhookEnv` からの deprecated チェック削除

- [ ] `ValidateSlackWebhookEnv` 関数が `GSCR_SLACK_WEBHOOK_URL` を参照しないこと
- [ ] `GSCR_SLACK_WEBHOOK_URL` を設定してrunnerを起動してもエラーにならないこと

### AC-4: テストコードの削除

- [ ] deprecated 変数に関するテストケースが削除されていること
- [ ] `logging.SlackWebhookURLEnvVar` への参照がテストコードに存在しないこと
- [ ] 残存テストがすべてパスすること

### AC-5: ドキュメントの更新

- [ ] `docs/user/runner_command.ja.md` に「`GSCR_SLACK_WEBHOOK_URL` からの移行」セクションが存在しないこと
- [ ] `docs/user/runner_command.md` に「Migration from `GSCR_SLACK_WEBHOOK_URL`」セクションが存在しないこと

### AC-6: ビルドとテストの成功

- [ ] `make build` が成功すること
- [ ] `make test` がすべてパスすること
- [ ] `make lint` がエラーなく完了すること

## 6. 削除対象ファイル・コード箇所一覧

| ファイル | 対象 | 種別 |
|---------|------|------|
| `internal/logging/pre_execution_error.go` | `SlackWebhookURLEnvVar` 定数 | ソースコード |
| `internal/runner/bootstrap/environment.go` | `ErrDeprecatedSlackWebhook`、`DeprecatedSlackWebhookError` 型・メソッド | ソースコード |
| `internal/runner/bootstrap/environment.go` | `ValidateSlackWebhookEnv` 内 deprecated チェック | ソースコード |
| `internal/runner/bootstrap/environment_test.go` | deprecated 変数のテストケース・フィールド | テストコード |
| `docs/user/runner_command.ja.md` | 「`GSCR_SLACK_WEBHOOK_URL` からの移行」セクション | ドキュメント |
| `docs/user/runner_command.md` | 「Migration from `GSCR_SLACK_WEBHOOK_URL`」セクション | ドキュメント |
