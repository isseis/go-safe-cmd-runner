# Design: Error Handling in Slack Notifications

## Overview

go-safe-cmd-runner execution is divided into two phases:

**Phase 1 (Verification and Log System Initialization)**
- File hash verification (`internal/verification/manager.go`)
- TOML configuration parsing (`internal/runner/config.go`)
- Log system setup

**Phase 2 (Slack Handler Registration)**
- Reading allowed Slack webhook hosts
- Slack handler initialization (`internal/logging/slack_handler.go`)
- Command execution start

This design creates a limitation: **errors occurring in Phase 1 (configuration file hash verification failure, TOML parsing errors) do not reach the Slack notification channel**. This document explains why this limitation exists, examines alternative approaches, and describes recommended monitoring strategies.

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
```bash
# Set restart policy in service unit
[Service]
Restart=on-failure
RestartSec=10s
OnFailure=notify-admin@%n.service
```

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

**Immediate detection:**
```bash
# Immediately detect hash verification errors
tail -f /var/log/go-safe-cmd-runner/runner.log | grep -i "hash verification failed"
```

**Using log monitoring tools:**

Filebeat:
```yaml
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /var/log/go-safe-cmd-runner/runner.log
  fields:
    module: go-safe-cmd-runner
  processors:
    - add_kubernetes_metadata: ~

output.elasticsearch:
  hosts: ["elasticsearch:9200"]

output.logstash:
  hosts: ["logstash:5000"]
```

Fluentd:
```xml
<source>
  @type tail
  path /var/log/go-safe-cmd-runner/runner.log
  pos_file /var/log/runner.log.pos
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
# Check if last run timestamp is recent
LAST_RUN=$(grep "process started" /var/log/go-safe-cmd-runner/runner.log | tail -1 | cut -d' ' -f1-2)
CURRENT_TIME=$(date +"%Y-%m-%d %H")
if [[ "$LAST_RUN" != "$CURRENT_TIME"* ]]; then
    echo "WARNING: Last run was not in the current hour"
    # Send Slack notification (configured separately)
fi

# Check for recent hash verification errors
if grep -i "hash verification failed" /var/log/go-safe-cmd-runner/runner.log | tail -24h > /dev/null; then
    echo "ALERT: Hash verification failures detected in past 24 hours"
    # Send Slack notification (configured separately)
fi
```

## Current Implementation Status

All Phase 1 errors are guaranteed to be logged:

**`internal/verification/manager.go`**
```go
func (m *Manager) Verify(...) error
// Returns error on hash verification failure
// Caller logs it (error evidence is not lost)
```

**`internal/runner/config.go`**
```go
func (c *Config) Load(...) error
// Returns error on TOML parsing failure
// Also logged to standard error before log system is initialized
```

**`internal/runner/executor.go`**
```go
func (e *Executor) Run(...) error
// Logs all Phase 1 errors
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
