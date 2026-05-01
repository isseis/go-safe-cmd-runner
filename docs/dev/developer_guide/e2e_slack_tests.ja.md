# Slack Redaction E2E Tests

このドキュメントでは、Slack webhook redaction 機能の integration test と E2E test について説明します。

## Test Structure

### Integration Tests (デフォルト - `make test` で実行)

**ファイル**: `internal/runner/e2e_slack_redaction_test.go`
**Build Tag**: `test`

これらの test は、複数の log handler (mock Slack handler を含む) において sensitive data が適切に redaction されることを検証します。これらの test は `make test` でデフォルトで実行されます。

#### Tests
- `TestIntegration_SlackRedaction`: GroupExecutor と MockSlackHandler を使用した redaction の test
- `TestE2E_MultiHandlerLogging`: stderr、file、mock Slack handler での redaction の test

**目的**: 外部サービスを必要とせずに redaction logic が正しく動作することを検証します。

### E2E Tests (手動 - 明示的な tag が必要)

**ファイル**: `internal/runner/e2e_slack_webhook_test.go`
**Build Tag**: `e2e && test`

これらの test は、Slack webhook への完全な HTTP flow を検証します。これらはデフォルトの test 実行からは除外されます。

#### Tests

##### TestE2E_SlackWebhookWithMockServer
ローカル HTTPS mock server と self-signed certificate を使用した HTTP webhook flow を test します。

**要件**:
- 外部依存なし
- 環境変数不要

**実行**:
```bash
go test -tags 'e2e test' -v ./internal/runner -run TestE2E_SlackWebhookWithMockServer
```

**test する内容**:
- ✅ SlackHandler が HTTPS request を実行できる
- ✅ Self-signed certificate が動作する (httptest.NewTLSServer)
- ✅ Webhook payload が server によって受信される
- ✅ RedactingHandler wrapper が HTTP flow を破壊しない

**test しない内容**:
- ❌ Command 実行 (この test context では command は実行されない)
- ❌ Output redaction (redaction する command output がない)
- Redaction の test については、`TestIntegration_SlackRedaction` を使用してください

## Manual Testing with Real Slack Webhooks

実際の Slack webhook を使用した redaction を含む完全な end-to-end flow の manual testing については:

### Setup
1. Slack webhook URL を作成:
   - Slack workspace の設定に移動
   - Incoming Webhook を作成
   - Webhook URL をコピー (形式: `https://hooks.slack.com/services/YOUR/WEBHOOK/URL`)

2. Test 設定ファイルを設定 (例: `test-config.toml`):
   ```toml
   version = "1.0"

   [global]
   timeout = 30

   [logging]
   slack_webhook_url = "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
   slack_run_id = "manual-test-redaction"

   [[groups]]
   name = "redaction-test"

   [[groups.commands]]
   name = "test-sensitive-output"
   cmd = "/bin/sh"
   args = ["-c", "echo 'API response: api_key=secret123 password=mypassword token=xyz789'"]
   ```

3. Command を実行:
   ```bash
   ./build/runner -config test-config.toml
   ```

### Expected Results
Slack channel でメッセージを確認してください。以下が表示されるはずです:
- Group 実行状態メッセージ
- Sensitive data (api_key、password、token) が `[REDACTED]` で置換されている
- Command output は次のように表示される: `API response: api_key=[REDACTED] password=[REDACTED] token=[REDACTED]`

### Verification Checklist
- [ ] Slack メッセージが受信される
- [ ] Group 名がメッセージに表示される
- [ ] Sensitive pattern (api_key=、password=、token=) が redaction される
- [ ] 実際の値の代わりに `[REDACTED]` placeholder が表示される
- [ ] 実際の secret 値 (secret123、mypassword、xyz789) が Slack に表示されない

## Known Limitations

### E2E Test Command Execution

実際の Slack webhook を使用した `Runner.ExecuteAll()` による自動化された E2E test は、test 環境で確実に動作しないため削除されました。Dry-run verification manager を使用した Runner は command を実行せず、以下の結果となります:
- Command Count: Slack メッセージ内で 0
- Redaction を test する command output がない

**回避策**:
- 自動化された testing の場合: Integration test (`TestIntegration_SlackRedaction` と `TestE2E_MultiHandlerLogging`) を使用してください。これらは redaction logic を包括的に検証します
- End-to-end 検証の場合: Manual testing を使用してください (上記セクション参照)

**根本原因**:
Runner は command を実行するために適切に設定された verification manager を必要とします。Dry-run verification manager はファイル verification をスキップしますが、test context での command 実行も防ぎます。

## Running All Tests

```bash
# Integration tests のみ (デフォルト)
make test

# Mock server を使用した E2E test (外部依存なし)
go test -tags 'e2e test' -v ./internal/runner -run TestE2E_SlackWebhookWithMockServer

# E2E を含むすべての test
go test -tags 'e2e test' -v ./internal/runner
```

## Summary

- **CI/CD の場合**: `make test` を使用 - redaction logic を包括的に検証する integration test を実行
- **HTTP Flow Testing の場合**: `TestE2E_SlackWebhookWithMockServer` を使用 - HTTPS webhook 通信を検証
- **Manual End-to-End Testing の場合**: 上記の manual testing guide に従い、実際の Slack webhook を使用した完全な flow を検証
- **Development の場合**: Integration test (`TestIntegration_SlackRedaction` と `TestE2E_MultiHandlerLogging`) が最も包括的な自動化された redaction testing を提供
