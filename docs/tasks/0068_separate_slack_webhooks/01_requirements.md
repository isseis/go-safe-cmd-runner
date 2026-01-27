# Slack Webhook 分離機能 要件定義書

## 1. 概要

### 1.1 背景

現在の runner は実行結果を単一の Slack webhook に通知している。正常終了時と異常終了時のメッセージを同一チャンネルに送信するため、重要なエラー通知が大量の正常通知に埋もれてしまう問題がある。

### 1.2 目的

正常時と異常時で異なる Slack webhook URL を設定可能にし、メッセージの送信先を分離する。これにより、異常通知専用チャンネルを設けてエラー監視を効率化できる。

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 正常通知 | コマンドグループが正常に完了した場合（status=success）の通知 |
| 異常通知 | コマンドグループが失敗した場合（status=error）、またはエラー系メッセージタイプの通知 |
| success webhook | 正常通知を送信する webhook URL |
| error webhook | 異常通知を送信する webhook URL |

## 3. 機能要件

### 3.1 設定方法

#### FR-3.1.1: 環境変数による設定

以下の2つの環境変数で webhook URL を設定できること：

- `GSCR_SLACK_WEBHOOK_URL_SUCCESS`: 正常通知用 webhook URL
- `GSCR_SLACK_WEBHOOK_URL_ERROR`: 異常通知用 webhook URL

**注記**: 環境変数名は既存の `GSCR_` プレフィックス（Go Safe Cmd Runner）に合わせている。

**注記**: Webhook URL は機密情報（URL のみでメッセージ送信が可能）のため、バージョン管理される可能性のある TOML 設定ファイルでの設定はサポートしない。環境変数のみで設定する。

### 3.2 通知の分類

#### FR-3.2.1: ログレベルによる分類

Slack への通知先はログレベルにより決定される：

| ログレベル | 送信先 |
|-----------|--------|
| INFO | success webhook |
| WARN, ERROR | error webhook |

#### FR-3.2.2: command_group_summary のログレベル変更

`command_group_summary` メッセージタイプのログレベルを status に応じて変更する：

| status | ログレベル | 送信先 |
|--------|-----------|--------|
| success | INFO | success webhook |
| error | ERROR | error webhook |

#### FR-3.2.3: その他のメッセージタイプ

以下のメッセージタイプは異常系のため、error webhook に送信される：

| メッセージタイプ | ログレベル | 送信先 |
|-----------------|-----------|--------|
| pre_execution_error | ERROR | error webhook |
| security_alert | WARN または ERROR | error webhook |
| privileged_command_failure | ERROR | error webhook |
| privilege_escalation_failure | WARN | error webhook |

### 3.3 設定の必須条件

#### FR-3.3.1: 設定の組み合わせ

| SUCCESS | ERROR | 動作 |
|---------|-------|------|
| 設定あり | 設定あり | 両方の webhook に通知 |
| 設定なし | 設定あり | エラー通知のみ（正常通知なし） |
| 設定あり | 設定なし | **エラー**（エラー通知なしは危険なため禁止） |
| 設定なし | 設定なし | Slack 通知無効（現行動作と同様） |

#### FR-3.3.2: SUCCESS のみ設定時のエラー

success webhook のみ設定し error webhook を設定しない場合はエラーとする。エラー通知が送信されない設定は運用上危険なため禁止する。

#### FR-3.3.3: 同一 URL の設定

success webhook と error webhook に同一の URL を設定することは許可する。これにより、既存の単一 webhook 運用からの移行が容易になる。

#### FR-3.3.4: CLI ログレベル設定との独立性

CLI の `--log-level` オプションは Slack 通知に影響しない。コンソール出力のログレベルを `WARN` や `ERROR` に設定しても、Slack への正常通知（INFO レベル）は送信される。これにより、コンソールでは重要なログのみ表示しつつ、Slack では全ての通知を受け取ることができる。

### 3.4 後方互換性

#### FR-3.4.1: 既存環境変数の廃止

既存の `GSCR_SLACK_WEBHOOK_URL` 環境変数は廃止する。この環境変数が設定されている場合は、新しい設定方法への移行を促すエラーメッセージを表示し、アプリケーションの起動を停止する（Fail Fast）。

#### FR-3.4.2: TOML 設定の禁止

TOML 設定ファイルに `slack_webhook_url` が記述されている場合は、環境変数への移行を促すエラーメッセージを表示し、アプリケーションの起動を停止する（Fail Fast）。これは機密情報がバージョン管理されることを防ぐためのセキュリティ対策である。

## 4. 非機能要件

### 4.1 セキュリティ

#### NFR-4.1.1: URL 検証

webhook URL は HTTPS スキームのみ許可する（既存動作を維持）。

### 4.2 運用性

#### NFR-4.2.1: エラーメッセージ

設定エラー時は、具体的な問題と解決方法を示すエラーメッセージを表示する。

### 4.3 信頼性

#### NFR-4.3.1: リトライ動作

webhook 送信失敗時のリトライ動作は既存実装を維持する：

| 項目 | 値 |
|------|-----|
| リトライ回数 | 3回 |
| バックオフ方式 | 指数バックオフ（基準値 2秒） |
| HTTP タイムアウト | 5秒 |
| リトライ対象 | サーバーエラー（5xx）、レートリミット（429） |
| リトライ非対象 | クライアントエラー（4xx、429除く） |

## 5. 受け入れ条件

### AC-1: 環境変数による設定

- [ ] `GSCR_SLACK_WEBHOOK_URL_SUCCESS` と `GSCR_SLACK_WEBHOOK_URL_ERROR` の両方を設定した場合、正常に Slack 通知が動作すること

### AC-2: 環境変数の組み合わせ

- [ ] 環境変数のみで設定が完結し、TOML ファイルへの webhook URL 記述が不要であること

### AC-3: 通知の分類

- [ ] コマンドグループ成功時（status=success）は INFO レベルでログ出力され、success webhook に通知されること
- [ ] コマンドグループ失敗時（status=error）は ERROR レベルでログ出力され、error webhook に通知されること
- [ ] pre_execution_error, security_alert, privileged_command_failure, privilege_escalation_failure は error webhook に通知されること

### AC-4: 設定の組み合わせ

- [ ] success webhook のみ設定した場合、エラーとなること
- [ ] error webhook のみ設定した場合、エラー通知のみが送信されること（正常通知は送信されない）
- [ ] 両方未設定の場合、Slack 通知が無効となり正常に動作すること

### AC-5: 後方互換性

- [ ] 既存の `GSCR_SLACK_WEBHOOK_URL` 環境変数が設定されている場合、エラーとなり移行を促すメッセージが表示されること
- [ ] TOML 設定ファイルに `slack_webhook_url` が記述されている場合、エラーとなり環境変数への移行を促すメッセージが表示されること

### AC-6: dry-run モード

- [ ] dry-run モードでは、success/error どちらの webhook にも実際の通知が送信されないこと

### AC-7: 同一 URL 設定

- [ ] success webhook と error webhook に同一の URL を設定した場合、正常に動作すること

## 5.1 テスト方針

### ユニットテスト

Slack 通知のテストは `httptest.NewServer` を使用したモックサーバー方式を採用する（既存実装に準拠）：

```go
// モックサーバーで受信したメッセージを検証
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    var msg SlackMessage
    json.NewDecoder(r.Body).Decode(&msg)
    // メッセージ内容を検証
    w.WriteHeader(http.StatusOK)
}))
```

### テストケース一覧

| テストケース | 検証内容 |
|-------------|---------|
| 両方設定時の振り分け | INFO → success、WARN/ERROR → error |
| ERROR のみ設定時 | INFO は送信されない、WARN/ERROR は送信される |
| SUCCESS のみ設定時 | 起動時エラー |
| 旧環境変数設定時 | 起動時エラー＋移行メッセージ |
| TOML に slack_webhook_url 記述時 | 起動時エラー＋移行メッセージ |
| 同一 URL 設定時 | 正常動作 |
| dry-run モード | 実際の送信なし |

## 6. 実装方針

### 6.1 SlackHandler のアーキテクチャ

SlackHandler を2つ作成し、MultiHandler で束ねる方式を採用する：

- **Success 用 SlackHandler**: INFO レベルのみを処理（`level == slog.LevelInfo`）
- **Error 用 SlackHandler**: WARN 以上を処理（`level >= slog.LevelWarn`）

この方式の利点：
- 既存の MultiHandler アーキテクチャとの親和性が高い
- 各ハンドラの責務が単純（単一責任の原則）
- SlackHandler 自体の変更が最小限

### 6.2 変更対象ファイル（予定）

| ファイル | 変更内容 |
|---------|---------|
| internal/logging/slack_handler.go | Level 設定オプションの追加（現在は INFO 固定） |
| internal/runner/bootstrap/logger.go | LoggerConfig の変更、2つの SlackHandler 作成 |
| internal/runner/bootstrap/environment.go | SetupLoggingOptions の変更、環境変数バリデーション追加 |
| internal/runner/config/loader.go | TOML の slack_webhook_url 検出時のエラー処理 |
| internal/runner/runner.go | logGroupExecutionSummary のログレベル変更（失敗時は ERROR） |

### 6.2 設定例

```bash
export GSCR_SLACK_WEBHOOK_URL_SUCCESS="https://hooks.slack.com/services/T.../B.../success..."
export GSCR_SLACK_WEBHOOK_URL_ERROR="https://hooks.slack.com/services/T.../B.../error..."
```
