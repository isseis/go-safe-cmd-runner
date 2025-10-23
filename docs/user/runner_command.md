# runner Command User Guide

User guide for the main execution command `runner` of go-safe-cmd-runner.

## Table of Contents

- [1. Overview](#1-overview)
- [2. Quick Start](#2-quick-start)
- [3. Command-Line Flags Details](#3-command-line-flags-details)
- [4. Environment Variables](#4-environment-variables)
- [5. Practical Examples](#5-practical-examples)
- [6. Troubleshooting](#6-troubleshooting)
- [7. Related Documentation](#7-related-documentation)

## 1. Overview

### 1.1 What is the runner Command

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
2. Record hash values (record command)
   - Hash of the TOML configuration file itself (required)
   - Hash of executable binaries
   - Hash of files specified in verify_files
   ↓
3. Validate configuration file (-validate flag)
   ↓
4. Verify operation with dry run (-dry-run flag)
   ↓
5. Production execution (runner command)
```

## 2. Quick Start

### 2.1 Execution with Minimal Configuration

```bash
# 1. Create configuration file (config.toml)
cat > config.toml << 'EOF'
version = "1.0"
skip_standard_paths = true

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

**Important**: The runner command performs hash verification on both the TOML configuration file and executable binaries. This prevents tampering with configuration files and executable files, and protects against TOCTOU attacks (Time-of-check to time-of-use).

You need to record hash values of the following files before execution:

1. **The TOML configuration file itself** (required)
2. Executable binaries specified in the configuration file
3. Files specified in `verify_files`

```bash
# 1. Record hash of the TOML configuration file (most important)
record -file config.toml -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 2. Record hash of executable binaries
record -file /usr/local/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 3. Record hash of files specified in verify_files (e.g., environment config files)
record -file /etc/myapp/database.conf -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

For details, see [record Command Guide](record_command.md).

### 2.3 About Configuration Files

For detailed information on how to write TOML configuration files, see the following documentation:

- [TOML Configuration File User Guide](toml_config/README.md)

## 3. Command-Line Flags Details

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

- **The configuration file must have its hash value recorded in advance (the TOML configuration file itself is also a verification target)**
- An error occurs if the file does not exist
- Execution is aborted if configuration file validation fails
- Reading and verification of the TOML configuration file is performed atomically to prevent TOCTOU attacks

### 3.2 Execution Mode Control

#### `-dry-run`

**Overview**

Simulates and displays the execution content without actually running commands.

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

- **Confirmation after configuration changes**: Verify that changes work as intended
- **Understanding impact scope**: Preview which commands will be executed
- **Security check**: Review risk assessment results
- **Debugging**: Verify variable expansion and environment variable states

**Dry Run Characteristics**

- File verification is performed (hash value checking)
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

**Usage Examples**

**Text Format (Default)**

```bash
runner -config config.toml -dry-run -dry-run-format text
```

Output example:
```
=== Dry Run Analysis ===

Group: backup (Priority: 1)
  Description: Database backup operations

  Command: db_backup
    Description: Backup PostgreSQL database
    Command Path: /usr/bin/pg_dump
    Arguments: ["-U", "postgres", "mydb"]
    Working Directory: /var/backups
    Timeout: 3600s
    Risk Level: medium
    Environment Variables:
      PATH=/sbin:/usr/sbin:/bin:/usr/bin
      HOME=/root
```

**JSON Format**

```bash
runner -config config.toml -dry-run -dry-run-format json
```

Output example:
```json
{
  "groups": [
    {
      "name": "backup",
      "priority": 1,
      "description": "Database backup operations",
      "commands": [
        {
          "name": "db_backup",
          "description": "Backup PostgreSQL database",
          "cmd": "/usr/bin/pg_dump",
          "args": ["-U", "postgres", "mydb"],
          "workdir": "/var/backups",
          "timeout": 3600,
          "risk_level": "medium",
          "env": {
            "PATH": "/sbin:/usr/sbin:/bin:/usr/bin",
            "HOME": "/root"
          }
        }
      ]
    }
  ]
}
```

**Using JSON Format**

```bash
# Filter with jq
runner -config config.toml -dry-run -dry-run-format json | jq '.groups[0].commands[0].cmd'

# Save to file and analyze
runner -config config.toml -dry-run -dry-run-format json > dryrun.json
```

#### `-dry-run-detail <level>`

**Overview**

Specifies the detail level of output during dry run execution.

**Syntax**

```bash
runner -config <path> -dry-run -dry-run-detail <level>
```

**Options**

- `summary`: Display summary information only
- `detailed`: Display detailed information (default)
- `full`: Display all information (environment variables, verified files, etc.)

**Usage Examples and Output Examples**

**summary Level**

```bash
runner -config config.toml -dry-run -dry-run-detail summary
```

Output example:
```
=== Dry Run Summary ===
Total Groups: 2
Total Commands: 5
Estimated Duration: ~180s
```

**detailed Level (Default)**

```bash
runner -config config.toml -dry-run -dry-run-detail detailed
```

Output example:
```
=== Dry Run Analysis ===

Group: backup (Priority: 1)
  Commands: 2

  Command: db_backup
    Path: /usr/bin/pg_dump
    Args: ["-U", "postgres", "mydb"]
    Risk: medium
```

**full Level**

```bash
runner -config config.toml -dry-run -dry-run-detail full
```

Output example:
```
=== Dry Run Analysis (Full Detail) ===

Group: backup (Priority: 1)
  Description: Database backup operations
  Working Directory: /var/backups
  Temp Directory: /tmp/runner-backup
  Environment Variables:
    PATH=/sbin:/usr/sbin:/bin:/usr/bin
    HOME=/root
  Verified Files:
    /usr/bin/pg_dump (SHA256: abc123...)

  Command: db_backup
    Description: Backup PostgreSQL database
    Command Path: /usr/bin/pg_dump
    Arguments: ["-U", "postgres", "mydb"]
    Working Directory: /var/backups
    Timeout: 3600s
    Risk Level: medium
    Risk Factors:
      - Database operation
      - Requires elevated privileges
    Run As User: postgres
    Run As Group: postgres
    Environment Variables:
      PATH=/sbin:/usr/sbin:/bin:/usr/bin
      HOME=/root
      PGPASSWORD=[REDACTED]
```

**Using Detail Levels**

- `summary`: Overview verification in CI/CD, listing large configurations
- `detailed`: Regular verification, checking after configuration changes
- `full`: Debugging, troubleshooting, environment variable verification

#### `-validate`

**Overview**

Validates the syntax and consistency of the configuration file, displays results, and exits. Commands are not executed.

**Syntax**

```bash
runner -config <path> -validate
```

**Usage Examples**

```bash
# Validate configuration file
runner -config config.toml -validate
```

Success output:
```
Configuration validation successful
  Version: 1.0
  Groups: 3
  Total Commands: 8
  Verified Files: 5
```

Error output:
```
Configuration validation failed:
  - Group 'backup': command 'db_backup' has invalid timeout: -1
  - Group 'deploy': duplicate command name 'deploy_app'
  - Global: invalid log level 'trace' (must be: debug, info, warn, error)
```

**Use Cases**

- **CI/CD Pipeline**: Automatically validate configuration files before commit
- **Confirmation after configuration changes**: Verify configuration validity before production execution
- **Development Testing**: Validate immediately while editing configuration files

**CI/CD Usage Example**

```yaml
# .github/workflows/validate-config.yml
name: Validate Runner Config

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Validate configuration
        run: |
          runner -config config.toml -validate
```

### 3.3 Log Configuration

#### `-log-level <level>`

**Overview**

Specifies the log output level. Logs at or above the specified level are output.

**Syntax**

```bash
runner -config <path> -log-level <level>
```

**Options**

- `debug`: All logs including debug information
- `info`: Normal information logs and above (default)
- `warn`: Warning and above logs only
- `error`: Error logs only

**Usage Examples**

```bash
# Execute in debug mode
runner -config config.toml -log-level debug

# Show warnings and errors only
runner -config config.toml -log-level warn

# Show errors only
runner -config config.toml -log-level error
```

**Information Output at Each Level**

**debug Level**
```
2025-10-02T10:30:00Z DEBUG Loading configuration file path=/etc/runner/config.toml
2025-10-02T10:30:00Z DEBUG Verifying file hash file=/usr/bin/backup.sh hash=abc123...
2025-10-02T10:30:00Z DEBUG Environment variable filtered out var=SHELL reason=not_in_allowlist
2025-10-02T10:30:00Z INFO  Starting command group=backup command=db_backup
2025-10-02T10:30:05Z INFO  Command completed successfully group=backup command=db_backup duration=5.2s
```

**info Level (Default)**
```
2025-10-02T10:30:00Z INFO  Starting command group=backup command=db_backup
2025-10-02T10:30:05Z INFO  Command completed successfully group=backup command=db_backup duration=5.2s
```

**warn Level**
```
2025-10-02T10:30:10Z WARN  Command execution slow group=backup command=full_backup duration=125s timeout=120s
```

**error Level**
```
2025-10-02T10:30:15Z ERROR Command failed group=backup command=db_backup error="exit status 1"
```

**Using Log Levels**

- `debug`: During development and troubleshooting
- `info`: Normal operation (default)
- `warn`: Record only warning signs in production environment
- `error`: Record only errors in integration with monitoring systems

**Notes**

- Command-line flags take precedence over `global.log_level` in TOML configuration files
- Sensitive information is automatically masked (passwords, tokens, etc.)

#### `-log-dir <directory>`

**Overview**

Specifies the directory to save execution logs. A JSON log file with ULID is created for each execution.

**Syntax**

```bash
runner -config <path> -log-dir <directory>
```

**Parameters**

- `<directory>`: Directory path to save log files (absolute or relative path)

**Usage Examples**

```bash
# Execute with log directory specified
runner -config config.toml -log-dir /var/log/go-safe-cmd-runner

# Specify with relative path
runner -config config.toml -log-dir ./logs
```

**Log File Naming Convention**

```
<log-dir>/runner-<run-id>.json
```

Example:
```
/var/log/go-safe-cmd-runner/runner-01K2YK812JA735M4TWZ6BK0JH9.json
```

**Log File Content (JSON Format)**

```json
{
  "timestamp": "2025-10-02T10:30:00Z",
  "level": "INFO",
  "message": "Command completed successfully",
  "run_id": "01K2YK812JA735M4TWZ6BK0JH9",
  "group": "backup",
  "command": "db_backup",
  "duration_ms": 5200,
  "exit_code": 0
}
```

**Use Cases**

- **Audit Log Storage**: Record all execution history
- **Troubleshooting**: Analyze past execution logs
- **Statistical Analysis**: Analyze execution time, error rates, etc.
- **Compliance**: Save execution trail

**Log Rotation**

Log files are not automatically rotated. Regular cleanup is required.

```bash
# Delete logs older than 30 days
find /var/log/go-safe-cmd-runner -name "runner-*.json" -mtime +30 -delete
```

**Notes**

- Command-line flags take precedence over TOML configuration and environment variables
- The directory is created automatically if it does not exist
- Log files are created with 0600 permissions (readable/writable by owner only)

#### `-run-id <id>`

**Overview**

Explicitly specifies a unique ID to identify the execution. If not specified, a ULID is automatically generated.

**Syntax**

```bash
runner -config <path> -run-id <id>
```

**Parameters**

- `<id>`: Unique string to identify execution (recommended: ULID format)

**Usage Examples**

```bash
# Specify custom Run ID
runner -config config.toml -run-id my-custom-run-001

# Specify in ULID format
runner -config config.toml -run-id 01K2YK812JA735M4TWZ6BK0JH9

# Auto-generated (default)
runner -config config.toml
```

**About ULID Format**

ULID (Universally Unique Lexicographically Sortable Identifier) has the following characteristics:

- **Chronological Order**: Sortable by generation time
- **Uniqueness**: Extremely low possibility of collision
- **URL Safe**: Does not contain special characters
- **Fixed Length**: 26 characters
- **Example**: `01K2YK812JA735M4TWZ6BK0JH9`

**Use Cases**

- **External System Integration**: Link with CI/CD build IDs
- **Distributed Execution Tracking**: Manage executions across multiple servers with unified ID
- **Debugging**: Reproduce specific executions

**External System Integration Examples**

```bash
# Use GitHub Actions Run ID
runner -config config.toml -run-id "gh-${GITHUB_RUN_ID}"

# Use Jenkins build number
runner -config config.toml -run-id "jenkins-${BUILD_NUMBER}"

# Timestamp-based ID
runner -config config.toml -run-id "backup-$(date +%Y%m%d-%H%M%S)"
```

**Notes**

- Run ID is included in log file names and log entries
- Using the same Run ID multiple times may overwrite log files
- Formats other than ULID can be used, but chronological sorting may not be possible

### 3.4 Output Control

#### `-interactive`

**Overview**

Forcibly enables interactive mode. Color output and progress display are enabled.

**Syntax**

```bash
runner -config <path> -interactive
```

**Usage Examples**

```bash
# Execute in interactive mode
runner -config config.toml -interactive

# Enable color output even via pipe
runner -config config.toml -interactive | tee output.log
```

**Interactive Mode Features**

- **Color Output**: Errors in red, warnings in yellow, success in green
- **Progress Display**: Visually display command execution status
- **Interactive Experience**: Display information in human-readable format

**Output Example**

```
✓ Configuration loaded successfully
✓ File verification completed (5 files)

→ Starting group: backup [Priority: 1]
  ✓ db_backup completed (5.2s)
  ✓ file_backup completed (12.8s)

→ Starting group: cleanup [Priority: 2]
  ✓ old_logs_cleanup completed (2.1s)

✓ All commands completed successfully
  Total duration: 20.1s
```

**Use Cases**

- **Interactive Execution**: Manual execution from command line
- **Debugging**: Visual confirmation of issues
- **Demo**: Presenting execution status
- **Verification via Pipe**: Preserve color output with `less -R`

**Relationship with Environment Variables**

The `-interactive` flag takes precedence over environment variables:

```bash
# Color output occurs even if NO_COLOR is set
NO_COLOR=1 runner -config config.toml -interactive
```

**Notes**

- Not typically used in CI/CD environments (`-quiet` recommended)
- Log files do not contain ANSI escape sequences
- If specified with `-quiet` flag, `-quiet` takes precedence

#### `-quiet`

**Overview**

Forces non-interactive mode. Color output and progress display are disabled.

**Syntax**

```bash
runner -config <path> -quiet
```

**Usage Examples**

```bash
# Execute in non-interactive mode
runner -config config.toml -quiet

# Redirect to log file
runner -config config.toml -quiet > output.log 2>&1
```

**Non-Interactive Mode Features**

- **Plain Text**: No color codes
- **Concise Output**: Minimum necessary information only
- **Machine Processing Oriented**: Easy to process in scripts and pipelines

**Output Example**

```
2025-10-02T10:30:00Z INFO Configuration loaded
2025-10-02T10:30:00Z INFO File verification completed files=5
2025-10-02T10:30:00Z INFO Starting group name=backup priority=1
2025-10-02T10:30:05Z INFO Command completed group=backup command=db_backup duration=5.2s exit_code=0
2025-10-02T10:30:18Z INFO Command completed group=backup command=file_backup duration=12.8s exit_code=0
2025-10-02T10:30:20Z INFO All commands completed duration=20.1s
```

**Use Cases**

- **CI/CD Environment**: Automated build and deployment pipelines
- **Cron Jobs**: Periodic execution scripts
- **Log Analysis**: Analyzing logs later
- **Script Integration**: Called from other scripts

**CI/CD Usage Example**

```yaml
# .github/workflows/deploy.yml
name: Deploy

on: [push]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Run deployment
        run: |
          runner -config deploy.toml -quiet -log-dir ./logs
```

**Cron Usage Example**

```bash
# crontab
0 2 * * * /usr/local/bin/runner -config /etc/runner/backup.toml -quiet -log-dir /var/log/runner
```

**Notes**

- If specified with `-interactive` and `-quiet` flags simultaneously, `-quiet` takes precedence
- Error messages are output to stderr
- Log level settings remain effective

#### `--keep-temp-dirs`

**Overview**

Keeps temporary directories after execution completes instead of deleting them. Used for debugging purposes.

**Syntax**

```bash
runner -config <path> --keep-temp-dirs
```

**Usage Examples**

```bash
# Execute with keeping temporary directories
runner -config config.toml --keep-temp-dirs

# Combine with dry-run (to verify temporary directory paths)
runner -config config.toml --keep-temp-dirs -dry-run
```

**Behavior Details**

Normally, when a group does not specify `workdir`, a temporary directory is automatically generated and deleted after group execution completes. With this flag:

- Temporary directories are not deleted and remain
- Temporary directory paths are logged
- Can be used for debugging and result verification

**Temporary Directory Location**

```
/tmp/scr-<group-name>-<random-string>
```

Examples:
```
/tmp/scr-backup-20250102123456789
/tmp/scr-analysis-20250102123500123
```

**Use Cases**

- **Debugging**: Verify command execution result files
- **Troubleshooting**: Investigate intermediate files and temporary files
- **Development/Testing**: Verify impact of configuration changes
- **Auditing**: Save evidence of execution results

**Usage Example (Actual Workflow)**

```bash
# 1. Execute with keeping temporary directories
runner -config backup.toml --keep-temp-dirs

# 2. Check temporary directory path from logs
# Output example: "Created temporary directory for group 'backup': /tmp/scr-backup-20250102123456"

# 3. Verify temporary directory contents
ls -la /tmp/scr-backup-20250102123456

# 4. Manually cleanup if needed
rm -rf /tmp/scr-backup-20250102123456
```

**Combining with Dry-Run Mode**

```bash
# Verify temporary directory paths (not actually created)
runner -config config.toml --keep-temp-dirs -dry-run
```

In dry-run mode, temporary directories are not actually created, but you can verify which paths would be used.

**Notes**

- Temporary directories must be manually cleaned up
- Does not affect groups with fixed `workdir` specified
- Multiple executions create multiple temporary directories
- Watch out for disk space usage

## 4. Environment Variables

### 4.1 Color Output Control

The runner command supports standard color control environment variables.

#### `CLICOLOR`

Controls enabling/disabling of color output.

**Values**

- `0`: Disable color output
- `1` or set: Enable color output (if terminal supports it)

**Usage Examples**

```bash
# Enable color output
CLICOLOR=1 runner -config config.toml

# Disable color output
CLICOLOR=0 runner -config config.toml
```

#### `NO_COLOR`

Disables color output (compliant with [NO_COLOR standard specification](https://no-color.org/)).

**Values**

- Set (any value): Disable color output
- Unset: Default behavior

**Usage Examples**

```bash
# Disable color output
NO_COLOR=1 runner -config config.toml

# Set as environment variable
export NO_COLOR=1
runner -config config.toml
```

#### `CLICOLOR_FORCE`

Forces color output, ignoring terminal auto-detection.

**Values**

- `0` or `false`: Do not force
- Other values: Force color output

**Usage Examples**

```bash
# Color output even via pipe
CLICOLOR_FORCE=1 runner -config config.toml | less -R

# Color output even with redirect (ANSI escape sequences saved to file)
CLICOLOR_FORCE=1 runner -config config.toml > output-with-colors.log
```

#### Priority Order

Color output determination is made in the following priority order:

```
1. Command-line flags (-interactive, -quiet)
   ↓
2. CLICOLOR_FORCE environment variable
   ↓
3. NO_COLOR environment variable
   ↓
4. CLICOLOR environment variable
   ↓
5. Terminal auto-detection
```

**Priority Examples**

```bash
# -quiet has highest priority (no color output)
CLICOLOR_FORCE=1 runner -config config.toml -quiet

# CLICOLOR_FORCE takes precedence over terminal detection (color output)
CLICOLOR_FORCE=1 runner -config config.toml > output.log

# NO_COLOR takes precedence over CLICOLOR (no color output)
CLICOLOR=1 NO_COLOR=1 runner -config config.toml
```

### 4.2 Notification Configuration

#### `GSCR_SLACK_WEBHOOK_URL`

Specifies the Webhook URL for Slack notifications. When set, errors and important events are notified to Slack.

**Usage Examples**

```bash
# Enable Slack notifications
export GSCR_SLACK_WEBHOOK_URL="https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXX"
runner -config config.toml
```

**Events to be Notified**

- Start of command execution
- Command success/failure
- Security-related events (privilege escalation, file verification failure, etc.)
- Errors and warnings

**Notification Example**

```
🤖 go-safe-cmd-runner

✅ Command completed successfully
Group: backup
Command: db_backup
Duration: 5.2s
Run ID: 01K2YK812JA735M4TWZ6BK0JH9
```

**Security Notes**

- Treat Webhook URL as sensitive information
- Recommended to manage with environment variables or secret management tools
- Not included in logs or error messages

### 4.3 CI Environment Auto-Detection

When the following environment variables are set, they are automatically recognized as CI environment and operate in non-interactive mode.

**Detected Environment Variables**

| Environment Variable | CI/CD System |
|---------|-------------|
| `CI` | Generic CI environment |
| `CONTINUOUS_INTEGRATION` | Generic CI environment |
| `GITHUB_ACTIONS` | GitHub Actions |
| `TRAVIS` | Travis CI |
| `CIRCLECI` | CircleCI |
| `JENKINS_URL` | Jenkins |
| `GITLAB_CI` | GitLab CI |
| `APPVEYOR` | AppVeyor |
| `BUILDKITE` | Buildkite |
| `DRONE` | Drone CI |
| `TF_BUILD` | Azure Pipelines |

**CI Environment Behavior**

- Color output is automatically disabled
- Progress display becomes concise
- Log format with timestamps

**Enabling Color Output in CI Environment**

```bash
# Color output in GitHub Actions
runner -config config.toml -interactive

# Or force with environment variable
CLICOLOR_FORCE=1 runner -config config.toml
```

## 5. Practical Examples

### 5.1 Basic Execution

**Simple Execution**

```bash
runner -config config.toml
```

**Execute with Log Level Specified**

```bash
runner -config config.toml -log-level debug
```

**Execute with Log File Saved**

```bash
runner -config config.toml -log-dir /var/log/runner -log-level info
```

### 5.2 Using Dry Run

**Verification Before Configuration Changes**

```bash
# Edit configuration file
vim config.toml

# Verify with dry run
runner -config config.toml -dry-run

# Execute if no issues
runner -config config.toml
```

**Using Detail Levels**

```bash
# Display summary only (overall picture)
runner -config config.toml -dry-run -dry-run-detail summary

# Detailed display (regular verification)
runner -config config.toml -dry-run -dry-run-detail detailed

# Full information display (debugging)
runner -config config.toml -dry-run -dry-run-detail full
```

**Analysis with JSON Output**

```bash
# Output in JSON format and analyze with jq
runner -config config.toml -dry-run -dry-run-format json | jq '.'

# Check risk level of specific commands
runner -config config.toml -dry-run -dry-run-format json | \
  jq '.groups[].commands[] | select(.risk_level == "high")'

# Check long-running commands
runner -config config.toml -dry-run -dry-run-format json | \
  jq '.groups[].commands[] | select(.timeout > 3600)'
```

### 5.3 Log Management

**Save Logs to File**

```bash
# Specify log directory
runner -config config.toml -log-dir /var/log/runner

# Save debug logs
runner -config config.toml -log-dir /var/log/runner -log-level debug
```

**Log Rotation**

```bash
# Delete old logs (older than 30 days)
find /var/log/runner -name "runner-*.json" -mtime +30 -delete

# Archive logs (older than 7 days)
find /var/log/runner -name "runner-*.json" -mtime +7 -exec gzip {} \;
```

**Log Analysis**

```bash
# Display latest log
ls -t /var/log/runner/runner-*.json | head -1 | xargs cat | jq '.'

# Extract error logs only
cat /var/log/runner/runner-*.json | jq 'select(.level == "ERROR")'

# Display log of specific Run ID
cat /var/log/runner/runner-01K2YK812JA735M4TWZ6BK0JH9.json | jq '.'
```

### 5.4 Configuration File Validation

**Basic Validation**

```bash
# Validate configuration file
runner -config config.toml -validate
```

**Validation in CI/CD Pipeline**

**GitHub Actions**

```yaml
name: Validate Configuration

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install runner
        run: |
          # Download or build pre-built binary
          make build

      - name: Validate configuration
        run: |
          ./build/runner -config config.toml -validate
```

**GitLab CI**

```yaml
validate-config:
  stage: test
  script:
    - runner -config config.toml -validate
  rules:
    - changes:
      - config.toml
```

**pre-commit hook**

```bash
#!/bin/bash
# .git/hooks/pre-commit

if git diff --cached --name-only | grep -q "config.toml"; then
  echo "Validating configuration..."
  runner -config config.toml -validate || exit 1
fi
```

### 5.5 Usage in CI/CD Environment

**Execution in Non-Interactive Mode**

```bash
# Explicitly specify -quiet in CI environment
runner -config config.toml -quiet -log-dir ./logs
```

**GitHub Actions Execution Example**

```yaml
name: Deployment

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup runner
        run: |
          make build
          sudo install -o root -g root -m 4755 build/runner /usr/local/bin/runner

      - name: Record hashes
        run: |
          sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
          # Record hash of the TOML configuration file itself (most important)
          sudo ./build/record -file config.toml -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
          # Record hash of executable binaries
          sudo ./build/record -file /usr/local/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

      - name: Validate configuration
        run: |
          runner -config config.toml -validate

      - name: Dry run
        run: |
          runner -config config.toml -dry-run -dry-run-format json > dryrun.json
          cat dryrun.json | jq '.'

      - name: Deploy
        run: |
          runner -config config.toml -quiet -log-dir ./logs
        env:
          GSCR_SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}

      - name: Upload logs
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: runner-logs
          path: logs/
```

**Jenkins Pipeline Execution Example**

```groovy
pipeline {
    agent any

    stages {
        stage('Validate') {
            steps {
                sh 'runner -config config.toml -validate'
            }
        }

        stage('Dry Run') {
            steps {
                sh 'runner -config config.toml -dry-run'
            }
        }

        stage('Deploy') {
            steps {
                withCredentials([string(credentialsId: 'slack-webhook', variable: 'SLACK_WEBHOOK')]) {
                    sh '''
                        export GSCR_SLACK_WEBHOOK_URL="${SLACK_WEBHOOK}"
                        runner -config config.toml -quiet -log-dir ./logs -run-id "jenkins-${BUILD_NUMBER}"
                    '''
                }
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'logs/*.json', allowEmptyArchive: true
        }
    }
}
```

### 5.6 Color Output Control

**Output Adjustment According to Environment**

```bash
# Interactive execution (with color output)
runner -config config.toml

# Redirect to log file (without color output)
runner -config config.toml -quiet > output.log

# Preserve color output via pipe
runner -config config.toml -interactive | less -R
```

**Force Color Output (when verifying via pipe)**

```bash
# Color display even via pipe
CLICOLOR_FORCE=1 runner -config config.toml | less -R

# Color display in tmux session
CLICOLOR_FORCE=1 runner -config config.toml
```

**Completely Disable Color Output**

```bash
# Disable with environment variable
NO_COLOR=1 runner -config config.toml

# Disable with flag
runner -config config.toml -quiet
```

## 6. Troubleshooting

### 6.1 Configuration File Related

#### Configuration File Not Found

**Error Message**
```
Error: Configuration file not found: config.toml
```

**Solutions**

```bash
# Check file existence
ls -l config.toml

# Specify with absolute path
runner -config /path/to/config.toml

# Check current directory
pwd
```

#### Configuration Validation Error

**Error Message**
```
Configuration validation failed:
  - Group 'backup': command 'db_backup' has invalid timeout: -1
```

**Solutions**

```bash
# Validate configuration file
runner -config config.toml -validate

# Check detailed error messages
runner -config config.toml -validate -log-level debug
```

For detailed configuration methods, see [TOML Configuration File Guide](toml_config/README.md).

### 6.2 Runtime Errors

#### Permission Error

**Error Message**
```
Error: Permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**Solutions**

```bash
# Check directory permissions
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# Fix permissions (administrator privileges required)
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# Check runner executable permissions (setuid bit required)
ls -l /usr/local/bin/runner
# Confirm -rwsr-xr-x (4755)
```

#### File Verification Error

**Error Message**
```
Error: File verification failed: /usr/bin/backup.sh
Hash mismatch: expected abc123..., got def456...
```

**Solutions**

```bash
# Check if file has changed
ls -l /usr/bin/backup.sh

# Re-record hash
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes -force

# Verify individually
verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

For details, see [verify Command Guide](verify_command.md).

#### Timeout Error

**Error Message**
```
Error: Command timed out after 3600s
Group: backup
Command: full_backup
```

**Solutions**

```bash
# Check timeout value
runner -config config.toml -dry-run | grep -A 5 "full_backup"

# Extend timeout in configuration file
# config.toml
[[groups.commands]]
name = "full_backup"
timeout = 7200  # Extend to 2 hours
```

### 6.3 Log and Output Related

#### No Logs Output

**Symptom**

Log file is not created or log is empty

**Solutions**

```bash
# Check log directory
ls -ld /var/log/runner

# Create directory if it doesn't exist
sudo mkdir -p /var/log/runner
sudo chmod 755 /var/log/runner

# Increase log level for detailed verification
runner -config config.toml -log-dir /var/log/runner -log-level debug

# Check permission errors
runner -config config.toml -log-dir ./logs  # Try in current directory
```

#### Color Output Not Displayed

**Symptom**

Color output is not displayed as expected

**Solutions**

```bash
# Check terminal color support
echo $TERM
# Confirm xterm-256color, screen-256color, etc.

# If TERM environment variable is not set correctly
export TERM=xterm-256color

# Force color output
runner -config config.toml -interactive

# Or force with environment variable
CLICOLOR_FORCE=1 runner -config config.toml

# Check if NO_COLOR is set
env | grep NO_COLOR
unset NO_COLOR  # Unset if set
```

## 7. Related Documentation

### Command-Line Tools

- [record Command Guide](record_command.md) - Creating hash files (for administrators)
- [verify Command Guide](verify_command.md) - File integrity verification (for debugging)

### Configuration Files

- [TOML Configuration File User Guide](toml_config/README.md) - Detailed configuration file writing
  - [Introduction](toml_config/01_introduction.md)
  - [Configuration File Hierarchy](toml_config/02_hierarchy.md)
  - [Root Level Configuration](toml_config/03_root_level.md)
  - [Global Level Configuration](toml_config/04_global_level.md)
  - [Group Level Configuration](toml_config/05_group_level.md)
  - [Command Level Configuration](toml_config/06_command_level.md)
  - [Variable Expansion](toml_config/07_variable_expansion.md)
  - [Practical Examples](toml_config/08_practical_examples.md)
  - [Best Practices](toml_config/09_best_practices.md)
  - [Troubleshooting](toml_config/10_troubleshooting.md)

### Security

- [Security Risk Assessment](security-risk-assessment.md) - Risk level details

### Project Information

- [README.md](../../README.md) - Project overview
- [Developer Documentation](../dev/) - Architecture and security design

---

**Last Updated**: 2025-10-02
