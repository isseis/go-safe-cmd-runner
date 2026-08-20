# Design: Error Handling in Slack Notifications

## Overview

go-safe-cmd-runner execution is divided into two phases:

**Phase 1 (Log System Initialization and Configuration File Verification)**
- Log system setup (`bootstrap.SetupLogging`)
- Configuration file hash verification and reading (`verification.Manager.VerifyAndReadConfigFile`)
- TOML parsing and schema validation (`bootstrap.LoadAndPrepareConfig`)

**Phase 2 (Slack Handler Registration and Beyond)**
- Reading allowed Slack webhook hosts from the configuration and registering the Slack handlers (`bootstrap.SetupSlackLogging`)
- Hash verification of global/group target files (`verification.Manager.VerifyGlobalFiles`)
- Command execution start

This design creates a limitation: **errors occurring in Phase 1 (configuration file hash verification failure, TOML parsing errors) do not reach the Slack notification channel**. This document explains why this limitation exists, examines alternative approaches, and describes recommended monitoring strategies.

Note that what fails to reach Slack is **limited to verification errors on the configuration file itself**. Hash verification of global/group target files runs in Phase 2, so those failures are notified to Slack as usual.

## The Core Issue

### Why Notifications Cannot Be Sent

Slack handler registration requires reading allowed Slack hosts from the configuration file:

```
Configuration file read failure → Cannot read Slack configuration → Cannot register Slack handler
```

This dependency is a **structural constraint**. If the configuration file itself is tampered with, we cannot read Slack notification targets from that same configuration.

### Error Scenarios

- **Hash verification failure**: When a file has been tampered with
- **TOML parsing error**: When the configuration file is corrupted
- **Schema validation error**: When configuration fields are invalid

These are **the highest priority detection events**. Configuration file tampering threatens system integrity and is a critical security incident.

## Alternative Approaches Considered

### Option 1: Hardcoded Fallback Slack Notification

Use a hardcoded Slack webhook host to send notifications when configuration file reading fails.

**Pros**
- Phase 1 errors reach Slack

**Cons**
- Increased maintenance burden for hardcoded values
- Cannot accommodate different Slack hosts per environment
- Security risk: Preventing erroneous messages to unknown hosts becomes complex
- Environment-specific configuration mixed into codebase
- Significant increase in test complexity
- Requires recompilation for binary distribution

### Option 2: Pre-validation with Minimal Payload

Pre-validate allowed hosts list using minimal information (e.g., known Slack IP ranges).

**Pros**
- Slightly reduced data exfiltration risk

**Cons**
- Must accommodate changes to Slack IP ranges
- Still requires two initialization paths
- High test complexity
- Increased maintenance burden
- Cannot ultimately guarantee Slack-based notifications

### Option 3: Rely on External Monitoring Mechanisms (Adopted)

Detect errors through process startup failures, log monitoring, health checks, and other **external monitoring mechanisms**.

**Pros**
- Initialization flow remains simple
- No complex alternative logic needed
- Test complexity does not increase
- Consistent detection strategy with other failures (e.g., permission issues)
- Aligns with standard operational monitoring patterns (Prometheus, Grafana, etc.)
- Flexible and highly extensible

**Cons**
- Does not reach Slack notifications immediately (but is logged)
- External monitoring setup required

## Adopted Design: Rely on External Monitoring

To **reliably detect** configuration file hash verification errors, the following mechanisms are recommended:

### 1. Monitor Process Startup Failures

**With systemd:**
```ini
# Start a notification unit when the service unit fails
[Service]
Type=oneshot
OnFailure=notify-admin@%N.service
```

`OnFailure=` starts only when the unit enters the failed state. If `Restart=on-failure`
is used together with it, the unit keeps restarting until the start limit
(`StartLimitBurst`) is reached and therefore does not enter the failed state, so the
notification does not fire. For batch execution, it is more reliable not to set
`Restart=` and to treat a single failure as failed as it is.

Monitor startup failures:
```bash
systemctl status go-safe-cmd-runner.service
journalctl -u go-safe-cmd-runner.service -f
```

**With cron:**
```bash
# Monitor for abnormal cron execution (exit code != 0)
# Email notification configured
MAILTO=admin@example.com
0 2 * * * /usr/local/bin/runner --config /etc/go-safe-cmd-runner/config.toml
```

### 2. Monitor Log Files

**Log file format (prerequisite)**

The log file is created as a new file for each execution, named
`<log directory>/<hostname>_<UTC timestamp>_<run_id>.json`
(`internal/runner/bootstrap/logger.go`). There is no single appended `runner.log`, so
monitoring targets **the set of files in the directory**. The content is one JSON
record per line (slog's `JSONHandler`), with `hostname` / `pid` / `run_id` /
`schema_version` attached in addition to `time` / `level` / `msg`.

**Immediate detection:**
```bash
# Immediately detect verification errors (follow new files in the log directory)
tail -F /var/log/go-safe-cmd-runner/*.json | grep -i "verification failed"
```

The actual `msg` values to target are `Global file verification failed` and
`CRITICAL: Global file verification failed - program will terminate`
(`internal/verification/manager.go`). Verification and parsing failures of the
configuration file itself are recorded as
`Failed to verify and read the configuration file` / `Failed to load the configuration`
(`internal/runner/bootstrap/config.go`).

**Using log monitoring tools:**

Filebeat (only one output can be enabled; specifying Elasticsearch and Logstash at the
same time causes an error at startup):
```yaml
filebeat.inputs:
- type: filestream
  id: go-safe-cmd-runner
  enabled: true
  paths:
    - /var/log/go-safe-cmd-runner/*.json
  parsers:
    - ndjson:
        target: ""
  fields:
    module: go-safe-cmd-runner

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
```

Fluentd (`@type slack` requires `fluent-plugin-slack`):
```xml
<source>
  @type tail
  path /var/log/go-safe-cmd-runner/*.json
  pos_file /var/log/go-safe-cmd-runner.pos
  tag go-safe-cmd-runner
  <parse>
    @type json
  </parse>
</source>

<match go-safe-cmd-runner>
  @type copy
  <store>
    @type elasticsearch
    host elasticsearch
    port 9200
    logstash_format true
  </store>
  <store>
    @type slack
    webhook_url "#{ENV['SLACK_WEBHOOK_URL']}"
    message_keys message,level
  </store>
</match>
```

### 3. Health Checks

Verify that periodic execution schedules are working as expected:

```bash
#!/bin/bash
set -euo pipefail
LOG_DIR=/var/log/go-safe-cmd-runner

# Check if last run timestamp is recent.
# The execution start marker is "Logger initialized", emitted when the logger is
# initialized (internal/runner/bootstrap/logger.go). Logs are a separate file per
# execution, so target the newest file.
LATEST=$(ls -1t "$LOG_DIR"/*.json 2>/dev/null | head -1)
LAST_RUN=$(
    [[ -n "$LATEST" ]] &&
    jq -r 'select(.msg == "Logger initialized") | .time' "$LATEST" | tail -1
)
CURRENT_TIME=$(date -u +"%Y-%m-%dT%H")
if [[ -z "$LAST_RUN" || "$LAST_RUN" != "$CURRENT_TIME"* ]]; then
    echo "WARNING: Last run was not in the current hour"
    # Send Slack notification (configured separately)
fi

# Check whether the most recent execution has verification errors
if [[ -n "$LATEST" ]] && jq -e 'select(.level == "ERROR" and (.msg | test("verification failed"; "i")))' \
        "$LATEST" >/dev/null; then
    echo "ALERT: Verification failures detected"
    # Send Slack notification (configured separately)
fi
```

`.time` is the log output time (UTC), so the current time being compared is also
aligned with `date -u`.

## Current Implementation Status

The log system is initialized at the beginning of Phase 1 (`bootstrap.SetupLogging`), so
configuration file verification and parsing errors occurring after that are guaranteed
to be recorded in the log file. Only the Slack handlers are added later.

**`internal/verification/manager.go`**
```go
func (m *Manager) VerifyAndReadConfigFile(configPath string) ([]byte, error)
// Performs hash verification and reading of the configuration file in one step (avoids TOCTOU)
// Returns error on verification failure
```

**`internal/runner/bootstrap/config.go`**
```go
func LoadAndPrepareConfig(...) (*runnertypes.ConfigSpec, error)
// Returns verification failures and TOML parsing failures as PreExecutionError
// The cause is held in Err, so it can be distinguished with errors.Is/As
```

**`cmd/runner/main.go`**
```go
// Executes in the order SetupLogging → LoadAndPrepareConfig → SetupSlackLogging.
// Errors from LoadAndPrepareConfig return before Slack handler registration, so
// they remain in the log file but do not reach Slack.
// Configuration tampering detection → process startup failure → strong signal
```

### Important Guarantees

| Item | Guarantee |
|------|-----------|
| **Log Recording** | ✓ All errors are guaranteed to be logged |
| **Startup Failure** | ✓ Config hash verification failure → process startup failure |
| **External Monitoring Feasibility** | ✓ Reliably detectable via log monitoring and process monitoring |

## Tradeoff Analysis

| Item | Current Design | Alternatives |
|------|---|---|
| Initialization Flow Complexity | ⭐⭐ (low) | ⭐⭐⭐⭐ (high) |
| Test Complexity | ⭐⭐ (low) | ⭐⭐⭐⭐ (high) |
| Log Recording | ✓ (guaranteed) | ✓ (guaranteed) |
| Slack Immediate Notification | ✗ (none) | ✓ (available) |
| Maintenance Burden | ⭐⭐ (low) | ⭐⭐⭐⭐ (high) |
| Operational Monitoring Flexibility | ✓ (excellent) | ✓ (excellent) |

## Design Rationale

1. **Complexity vs. Benefit**: Implementation and maintenance cost of alternatives is significantly high, while external monitoring achieves the same goal.

2. **Security Best Practice**: When configuration file tampering occurs, process startup itself fails. This "startup failure" is the strongest warning signal and more effective than asynchronous Slack notifications.

3. **Operational Flexibility**: External monitoring mechanisms are flexible to adapt to host configuration changes and can combine multiple detection patterns (log monitoring, health checks, metrics monitoring).

4. **Consistency**: Provides unified detection strategy across other failure scenarios (permission issues, disk space issues, etc.).

## Conclusion

**Configuration file hash verification errors**:
1. **Are guaranteed to be logged** ← Evidence is never lost
2. **Cause process startup failure** ← Reliable and trustworthy detection signal
3. **Reliably detectable via external monitoring** ← Log monitoring, process monitoring, health checks, etc.

Therefore, **combining simple architecture design with external monitoring is operationally more efficient than implementing complex alternative logic**. This architectural tradeoff is **acceptable and recommended**.

---

**Related Issue**: [Issue #1018](https://github.com/isseis/go-safe-cmd-runner/issues/1018)
**Implementation Reference**: [slack_async_delivery.md](slack_async_delivery.md)
