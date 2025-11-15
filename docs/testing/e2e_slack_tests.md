# Slack Redaction E2E Tests

This document describes the integration and E2E tests for Slack webhook redaction functionality.

## Test Structure

### Integration Tests (Default - Run with `make test`)

**File**: `internal/runner/e2e_slack_redaction_test.go`
**Build Tag**: `test`

These tests verify that sensitive data is properly redacted across multiple log handlers including a mock Slack handler. They run by default with `make test`.

#### Tests
- `TestIntegration_SlackRedaction`: Tests redaction with GroupExecutor and MockSlackHandler
- `TestE2E_MultiHandlerLogging`: Tests redaction across stderr, file, and mock Slack handlers

**Purpose**: Verify the redaction logic works correctly without requiring external services.

### E2E Tests (Manual - Require explicit tag)

**File**: `internal/runner/e2e_slack_webhook_test.go`
**Build Tag**: `e2e && test`

These tests verify the complete HTTP flow to Slack webhooks. They are excluded from default test runs.

#### Tests

##### TestE2E_SlackWebhookWithMockServer
Tests HTTP webhook flow using a local HTTPS mock server with self-signed certificates.

**Requirements**:
- No external dependencies
- No environment variables needed

**Run**:
```bash
go test -tags 'e2e test' -v ./internal/runner -run TestE2E_SlackWebhookWithMockServer
```

**What it tests**:
- ✅ SlackHandler can make HTTPS requests
- ✅ Self-signed certificates work (httptest.NewTLSServer)
- ✅ Webhook payload is received by the server
- ✅ RedactingHandler wrapper doesn't break HTTP flow

**What it does NOT test**:
- ❌ Command execution (commands don't run in this test context)
- ❌ Output redaction (no command output to redact)
- For redaction testing, use `TestIntegration_SlackRedaction`

## Manual Testing with Real Slack Webhooks

For manual testing of the complete end-to-end flow including redaction with real Slack webhooks:

### Setup
1. Create a Slack webhook URL:
   - Go to your Slack workspace settings
   - Create an Incoming Webhook
   - Copy the webhook URL (format: `https://hooks.slack.com/services/YOUR/WEBHOOK/URL`)

2. Configure your test configuration file (e.g., `test-config.toml`):
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

3. Run the command:
   ```bash
   ./build/runner -config test-config.toml
   ```

### Expected Results
Check your Slack channel for messages. You should see:
- Group execution status messages
- Sensitive data (api_key, password, token) replaced with `[REDACTED]`
- Command output should appear as: `API response: api_key=[REDACTED] password=[REDACTED] token=[REDACTED]`

### Verification Checklist
- [ ] Slack messages are received
- [ ] Group name appears in messages
- [ ] Sensitive patterns (api_key=, password=, token=) are redacted
- [ ] `[REDACTED]` placeholder appears instead of actual values
- [ ] No actual secret values (secret123, mypassword, xyz789) appear in Slack

## Known Limitations

### E2E Test Command Execution

Automated E2E tests using `Runner.ExecuteAll()` with real Slack webhooks were removed because they don't work reliably in the test environment. The Runner with dry-run verification manager doesn't execute commands, resulting in:
- Command Count: 0 in Slack messages
- No command output to test redaction on

**Workaround**:
- For automated testing: Use the integration tests (`TestIntegration_SlackRedaction` and `TestE2E_MultiHandlerLogging`) which verify redaction logic comprehensively
- For end-to-end verification: Use manual testing (see above section)

**Root Cause**:
The Runner requires a properly configured verification manager to execute commands. The dry-run verification manager skips file verification but also prevents command execution in test context.

## Running All Tests

```bash
# Integration tests only (default)
make test

# E2E test with mock server (no external dependencies)
go test -tags 'e2e test' -v ./internal/runner -run TestE2E_SlackWebhookWithMockServer

# All tests including E2E
go test -tags 'e2e test' -v ./internal/runner
```

## Summary

- **For CI/CD**: Use `make test` - runs integration tests that verify redaction logic comprehensively
- **For HTTP Flow Testing**: Use `TestE2E_SlackWebhookWithMockServer` - verifies HTTPS webhook communication
- **For Manual End-to-End Testing**: Follow the manual testing guide above to verify complete flow with real Slack webhooks
- **For Development**: Integration tests (`TestIntegration_SlackRedaction` and `TestE2E_MultiHandlerLogging`) provide the most comprehensive automated redaction testing
