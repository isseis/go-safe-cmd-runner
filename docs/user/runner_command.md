# runner Command User Guide

This guide explains how to use the main execution command `runner` of go-safe-cmd-runner.

## Table of Contents

- [1. Overview](#1-overview)
- [2. Quick Start](#2-quick-start)
- [3. Command Line Flags Details](#3-command-line-flags-details)
- [4. Environment Variables](#4-environment-variables)
- [5. Practical Examples](#5-practical-examples)
- [6. Troubleshooting](#6-troubleshooting)
- [7. Related Documentation](#7-related-documentation)

## 1. Overview

### 1.1 What is the runner command?

`runner` is the main command of go-safe-cmd-runner that safely executes commands based on TOML configuration files.

### 1.2 Main Use Cases

- **Secure Batch Processing**: Group multiple commands and execute them sequentially
- **Privilege Delegation**: Safely delegate specific administrative tasks to regular users
- **Automation Tasks**: Automate backups, deployments, and system maintenance
- **Auditing and Logging**: Record and track execution history

### 1.3 Basic Usage Flow

```
1. Create TOML configuration file
   ↓
2. Record hash values of executable binaries (record command)
   ↓
3. Validate configuration file (-validate flag)
   ↓
4. Check operation with dry run (-dry-run flag)
   ↓
5. Production execution (runner command)
```

## 2. Quick Start

### 2.1 Minimal Configuration Execution

```bash
# 1. Create configuration file (config.toml)
cat > config.toml << 'EOF'
version = "1.0"

[[groups]]
name = "hello"

[[groups.commands]]
name = "greet"
cmd = "/bin/echo"
args = ["Hello, World!"]
EOF

# 2. Execute
runner -config config.toml
```

### 2.2 Preparation: Creating Hash Files

For security, you need to record hash values of configuration files and binaries before execution.

```bash
# Record configuration file hash
record -file config.toml -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# Record executable binary hash
record -file /usr/local/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

For details, refer to the [record command guide](record_command.md).

### 2.3 About Configuration Files

For detailed instructions on writing TOML configuration files, refer to:

- [TOML Configuration File User Guide](toml_config/README.md)

## 3. Command Line Flags Details

### 3.1 Required Flags

#### `-config <path>`

**Overview**

Specifies the path to the TOML format configuration file.

**Syntax**

```bash
runner -config <path>
```

**Parameters**

- `<path>`: Absolute or relative path to the configuration file (required)

**Usage Examples**

```bash
# Specify with relative path
runner -config config.toml

# Specify with absolute path
runner -config /etc/go-safe-cmd-runner/production.toml

# Specify from home directory
runner -config ~/configs/backup.toml
```

**Notes**

- Configuration files must have their hash values recorded in advance
- Error occurs if file does not exist
- Execution is aborted if configuration file validation fails

### 3.2 Execution Mode Control

#### `-dry-run`

**Overview**

Simulates and displays execution content without actually executing commands.

**Syntax**

```bash
runner -config <path> -dry-run
```

**Usage Examples**

```bash
# Basic dry run
runner -config config.toml -dry-run

# Specify detail level and format
runner -config config.toml -dry-run -dry-run-detail full -dry-run-format json
```

**Use Cases**

- **Confirmation after configuration changes**: Check if it works as intended after modifying configuration files
- **Understanding impact scope**: Check which commands will be executed in advance
- **Security check**: Verify risk assessment results
- **Debugging**: Check variable expansion and environment variable states

**Dry Run Characteristics**

- File verification is executed (hash value checking)
- Actual commands are not executed
- Environment variable expansion results can be confirmed
- Risk assessment results are displayed

#### `-dry-run-format <format>`

**Overview**

Specifies the output format for dry run execution.

**Syntax**

```bash
runner -config <path> -dry-run -dry-run-format <format>
```

**Options**

- `text`: Human-readable text format (default)
- `json`: Machine-processable JSON format

### 3.3 Validation Mode

#### `-validate`

**Overview**

Validates the configuration file without executing commands.

**Syntax**

```bash
runner -config <path> -validate
```

**Validation Items**

- TOML syntax validation
- Required field checking
- File path validation
- Environment variable setting validation
- Risk level setting validation

## 4. Environment Variables

### 4.1 Color Control

#### `CLICOLOR`

Controls color output in interactive mode.

```bash
export CLICOLOR=1  # Enable color
runner -config config.toml
```

#### `NO_COLOR`

Disables color output regardless of other settings.

```bash
export NO_COLOR=1  # Disable color
runner -config config.toml
```

#### `CLICOLOR_FORCE`

Forces color output regardless of terminal type.

```bash
export CLICOLOR_FORCE=1  # Force color
runner -config config.toml
```

### 4.2 Slack Integration

#### `GSCR_SLACK_WEBHOOK_URL`

Sets the Slack webhook URL for notifications.

```bash
export GSCR_SLACK_WEBHOOK_URL="https://hooks.slack.com/services/..."
runner -config config.toml
```

## 5. Practical Examples

### 5.1 Database Backup Example

```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowlist = ["PATH", "HOME", "PGPASSWORD"]

[[groups]]
name = "database_backup"
description = "PostgreSQL database backup"

[[groups.commands]]
name = "backup_main_db"
description = "Backup main database"
cmd = "/usr/bin/pg_dump"
args = ["-h", "localhost", "-U", "backup_user", "main_db"]
output = "/var/backups/main_db.sql"
run_as_user = "postgres"
max_risk_level = "medium"
timeout = 1800
```

Execute:

```bash
# Validate configuration
runner -config backup.toml -validate

# Dry run
runner -config backup.toml -dry-run

# Execute
runner -config backup.toml
```

### 5.2 System Maintenance Example

```toml
version = "1.0"

[global]
timeout = 600
log_level = "info"
env_allowlist = ["PATH"]

[[groups]]
name = "maintenance"
description = "System maintenance tasks"
priority = 1

[[groups.commands]]
name = "cleanup_logs"
description = "Clean up old log files"
cmd = "/usr/bin/find"
args = ["/var/log", "-name", "*.log", "-mtime", "+30", "-delete"]
max_risk_level = "high"
```

### 5.3 CI/CD Integration Example

```bash
#!/bin/bash
# CI/CD script example

set -e

# Environment setup
export GSCR_SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL}"
export NO_COLOR=1  # Disable color in CI

# Validate configuration
runner -config deploy.toml -validate

# Execute deployment
if runner -config deploy.toml -dry-run; then
    echo "Dry run successful, proceeding with deployment"
    runner -config deploy.toml
else
    echo "Dry run failed, aborting deployment"
    exit 1
fi
```

## 6. Troubleshooting

### 6.1 Common Errors

#### Configuration File Not Found

**Error Message:**
```
Error: failed to read config file: open config.toml: no such file or directory
```

**Solution:**
- Check if the file path is correct
- Use absolute path if necessary
- Ensure the file exists

#### Hash Verification Failed

**Error Message:**
```
Error: file verification failed: hash mismatch for /usr/bin/pg_dump
```

**Solution:**
- Re-record the hash with `record` command
- Check if the file was modified after hash recording

#### Permission Denied

**Error Message:**
```
Error: permission denied: cannot execute /usr/bin/restricted-command
```

**Solution:**
- Check file permissions
- Ensure the runner binary has setuid bit set
- Verify user/group execution settings

### 6.2 Debug Options

#### Enable Debug Logging

```bash
runner -config config.toml -log-level debug
```

#### Check File Verification

```bash
verify -file /path/to/file -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

## 7. Related Documentation

- [record Command Guide](record_command.md) - Hash file management
- [verify Command Guide](verify_command.md) - File verification
- [TOML Configuration File Guide](toml_config/README.md) - Configuration file writing
- [Security Risk Assessment](security-risk-assessment.md) - Risk levels and security controls
- [Project README](../../README.md) - Overall overview and installation
