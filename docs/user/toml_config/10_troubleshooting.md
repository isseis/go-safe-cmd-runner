# Chapter 10: Troubleshooting

This chapter introduces common problems when creating configuration files and their solutions. Let's learn how to read error messages and debugging techniques.

## 10.1 Common Errors and Solutions

### 10.1.1 Configuration File Loading Errors

#### Error Example

```
Error: failed to load configuration: toml: line 15: expected key, but got '=' instead
```

#### Cause

TOML file syntax error. Key not specified or invalid format.

#### Solution

```toml
# Wrong: No key
= "value"

# Correct
key = "value"

# Wrong: Unclosed quote
name = "unclosed string

# Correct
name = "closed string"
```

### 10.1.2 Version Specification Error

#### Error Example

```
Error: unsupported configuration version: 2.0
```

#### Cause

Unsupported version specified.

#### Solution

```toml
# Wrong: Unsupported version
version = "2.0"

# Correct: Use supported version
version = "1.0"
```

### 10.1.3 Missing Required Fields

#### Error Example

```
Error: group 'backup_tasks' is missing required field 'name'
Error: command is missing required field 'cmd'
```

#### Cause

Required fields (`name`, `cmd`, etc.) are not set.

#### Solution

```toml
# Wrong: No name
[[groups]]
description = "Backup tasks"

# Correct: Add name
[[groups]]
name = "backup_tasks"
description = "Backup tasks"

# Wrong: No cmd
[[groups.commands]]
name = "backup"
args = ["/data"]

# Correct: Add cmd
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
```

### 10.1.4 Environment Variable Permission Error

#### Error Example

```
Error: environment variable 'CUSTOM_VAR' is not allowed by env_allowlist
```

#### Cause

The environment variable being used is not included in `env_allowlist`.

#### Solution

**Method 1**: Add to global or group's `env_allowlist`

```toml
[global]
env_allowlist = ["PATH", "HOME", "CUSTOM_VAR"]  # Add CUSTOM_VAR
```

**Method 2**: Define in Command.Env (recommended)

```toml
# No need to add to env_allowlist
[[groups.commands]]
name = "custom_command"
cmd = "${CUSTOM_TOOL}"
args = []
env = ["CUSTOM_TOOL=/opt/tools/mytool"]  # Define in Command.Env
```

### 10.1.5 Variable Expansion Error

#### Error Example

```
Error: undefined variable: UNDEFINED_VAR
Error: circular variable reference detected: VAR1 -> VAR2 -> VAR1
```

#### Cause

- Variable is not defined
- Circular variable reference

#### Solution

**For undefined variables**:

```toml
# Wrong: TOOL_DIR is not defined
[[groups.commands]]
name = "run_tool"
cmd = "${TOOL_DIR}/mytool"
args = []

# Correct: Define in env
[[groups.commands]]
name = "run_tool"
cmd = "${TOOL_DIR}/mytool"
args = []
env = ["TOOL_DIR=/opt/tools"]
```

**For circular references**:

```toml
# Wrong: Circular reference
env = [
    "VAR1=${VAR2}",
    "VAR2=${VAR1}",
]

# Correct: Resolve the circular reference
env = [
    "VAR1=/path/to/dir",
    "VAR2=${VAR1}/subdir",
]
```

### 10.1.6 File Verification Error

#### Error Example

```
Error: file verification failed: /usr/bin/tool: hash mismatch
Error: file verification failed: /opt/app/script.sh: hash file not found
```

#### Cause

- File has been tampered with
- Hash file has not been created

#### Solution

**Creating hash files**:

```bash
# Record hashes with record command
record -file /usr/bin/tool
record -file /opt/app/script.sh
```

**If file was legitimately changed**:

```bash
# Re-record hash
record -file /usr/bin/tool
```

### 10.1.7 Command Path Errors

#### Error Example

```
Error: command path must be absolute: ./tool
Error: command not found: mytool
```

#### Cause

- Relative path is being used
- Command does not exist

#### Solution

```toml
# Wrong: Relative path
[[groups.commands]]
name = "run"
cmd = "./mytool"

# Correct: Absolute path
[[groups.commands]]
name = "run"
cmd = "/opt/tools/mytool"

# Wrong: PATH-dependent but doesn't exist
[[groups.commands]]
name = "run"
cmd = "nonexistent-command"

# Correct: Absolute path to existing command
[[groups.commands]]
name = "run"
cmd = "/usr/bin/existing-command"
```

### 10.1.8 Timeout Errors

#### Error Example

```
Error: command timeout: exceeded 60 seconds
```

#### Cause

Command execution time exceeded timeout value.

#### Solution

```toml
# Method 1: Extend global timeout
[global]
timeout = 600  # 60s → 600s

# Method 2: Extend only specific command
[[groups.commands]]
name = "long_running"
cmd = "/usr/bin/long-process"
args = []
timeout = 3600  # 1 hour for this command only
```

### 10.1.9 Permission Errors

#### Error Example

```
Error: permission denied: /var/secure/data
Error: failed to change user: operation not permitted
```

#### Cause

- No access permission to file or directory
- User change not permitted

#### Solution

**For file permissions**:

```bash
# Check file permissions
ls -la /var/secure/data

# Set appropriate permissions
sudo chmod 644 /var/secure/data
sudo chown user:group /var/secure/data
```

**For user change**:

```toml
# Using run_as_user requires root privileges
# Execute go-safe-cmd-runner as root or with appropriate privileges
```

```bash
# Execute with root privileges
sudo go-safe-cmd-runner -file config.toml
```

### 10.1.10 Risk Level Exceeded Error

#### Error Example

```
Error: command risk level exceeds maximum: command risk=medium, max_risk_level=low
```

#### Cause

Command risk level exceeds `max_risk_level`.

#### Solution

```toml
# Method 1: Increase max_risk_level
[[groups.commands]]
name = "risky_command"
cmd = "/bin/rm"
args = ["-rf", "/tmp/data"]
max_risk_level = "medium"  # Change low → medium

# Method 2: Change to safer command
[[groups.commands]]
name = "safer_command"
cmd = "/bin/rm"
args = ["/tmp/data/specific-file.txt"]  # Remove -rf
max_risk_level = "low"
```

## 10.2 Configuration Validation Methods

### 10.2.1 Syntax Checking

Validate configuration file syntax:

```bash
# Test configuration file loading
go-safe-cmd-runner --validate config.toml

# Pre-execution validation with dry run
go-safe-cmd-runner --dry-run --file config.toml
```

### 10.2.2 Incremental Validation

Validate complex configurations incrementally:

```toml
# Step 1: Minimal configuration
version = "1.0"

[[groups]]
name = "test"

[[groups.commands]]
name = "simple"
cmd = "/bin/echo"
args = ["test"]
```

```bash
# Execute to confirm basic operation
go-safe-cmd-runner -file minimal.toml
```

```toml
# Step 2: Add variable expansion
[[groups.commands]]
name = "with_variables"
cmd = "/bin/echo"
args = ["Value: ${VAR}"]
env = ["VAR=hello"]
```

```bash
# Confirm variable expansion behavior
go-safe-cmd-runner -file with-vars.toml
```

### 10.2.3 Utilizing Log Levels

Enable detailed logs when debugging:

```toml
[global]
log_level = "debug"  # Output detailed debug information
```

Output example:
```
[DEBUG] Loading configuration from: config.toml
[DEBUG] Parsed version: 1.0
[DEBUG] Global timeout: 300
[DEBUG] Processing group: backup_tasks
[DEBUG] Expanding variables in command: backup_database
[DEBUG] Variable BACKUP_DIR expanded to: /var/backups
[DEBUG] Executing: /usr/bin/pg_dump --all-databases
[INFO] Command completed successfully
```

## 10.3 Debugging Techniques

### 10.3.1 Verifying Variables with Echo Command

Confirm variables are expanded correctly:

```toml
# Debug command
[[groups.commands]]
name = "debug_variables"
cmd = "/bin/echo"
args = [
    "TOOL_DIR=${TOOL_DIR}",
    "CONFIG=${CONFIG}",
    "ENV=${ENV_TYPE}",
]
env = [
    "TOOL_DIR=/opt/tools",
    "CONFIG=/etc/app/config.yml",
    "ENV_TYPE=production",
]
output = "debug-vars.txt"
```

After execution, check `debug-vars.txt`:
```
TOOL_DIR=/opt/tools CONFIG=/etc/app/config.yml ENV=production
```

### 10.3.2 Diagnosis with Output Capture

Save command output to examine details:

```toml
[[groups.commands]]
name = "diagnose"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp.service"]
output = "service-status.txt"
```

After execution, check output file:
```bash
cat service-status.txt
```

### 10.3.3 Testing Individual Commands

Test problematic commands individually:

```toml
# Test configuration with only problematic command
version = "1.0"

[[groups]]
name = "test_single_command"

[[groups.commands]]
name = "problematic_command"
cmd = "/usr/bin/tool"
args = ["--option", "value"]
env = ["CUSTOM_VAR=test"]
```

### 10.3.4 Utilizing Dry Run

Confirm behavior without actual execution:

```bash
# Display execution plan with dry run
go-safe-cmd-runner --dry-run --file config.toml
```

Output example:
```
[DRY RUN] Would execute: /usr/bin/pg_dump --all-databases
[DRY RUN] Working directory: /var/backups
[DRY RUN] Timeout: 600 seconds
[DRY RUN] Environment variables: PATH=/usr/bin, DB_USER=postgres
```

### 10.3.5 Checking Permissions

Diagnose permission-related issues:

```toml
# Permission check commands
[[groups.commands]]
name = "check_permissions"
cmd = "/usr/bin/id"
args = []
output = "current-user.txt"

[[groups.commands]]
name = "check_file_access"
cmd = "/bin/ls"
args = ["-la", "/path/to/file"]
output = "file-permissions.txt"
```

### 10.3.6 Checking Environment Variables

Diagnose environment variable state:

```toml
[[groups.commands]]
name = "dump_env"
cmd = "/usr/bin/env"
args = []
output = "environment.txt"
```

## 10.4 Performance Issues

### 10.4.1 Slow Startup

#### Cause

- Large number of file verifications
- Heavy initialization processing

#### Solution

```toml
# Skip standard path verification
[global]
skip_standard_paths = true

# Verify only minimum necessary files
verify_files = [
    "/opt/app/bin/critical-tool",
]
```

### 10.4.2 Slow Execution

#### Cause

- Timeout too long
- Unnecessary output capture

#### Solution

```toml
# Set appropriate timeout
[[groups.commands]]
name = "quick_command"
cmd = "/bin/echo"
args = ["test"]
timeout = 10  # Short timeout for quick commands

# Remove unnecessary output capture
[[groups.commands]]
name = "simple_command"
cmd = "/bin/echo"
args = ["Processing..."]
# Don't specify output
```

## 10.5 Frequently Asked Questions (FAQ)

### Q1: Environment variables are not expanded

**Q**: `${HOME}` is not expanded and is treated as a literal string.

**A**: Environment variables must be included in `env_allowlist` or defined in `Command.Env`.

```toml
# Method 1: Add to env_allowlist
[global]
env_allowlist = ["PATH", "HOME"]

# Method 2: Define in Command.Env (recommended)
[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["${MY_HOME}"]
env = ["MY_HOME=/home/user"]
```

### Q2: Command not found

**Q**: `command not found` error occurs.

**A**: Use absolute paths or verify PATH is set correctly.

```toml
# Recommended: Absolute path
cmd = "/usr/bin/tool"

# Or: Check PATH
[global]
env_allowlist = ["PATH"]
```

### Q3: File verification fails

**Q**: Hash validation error occurs.

**A**: Create or update hash files.

```bash
# Record individual files
record -file config.toml
record -file /usr/bin/tool
```

### Q4: Frequent timeout errors

**Q**: Many commands timeout.

**A**: Extend timeout value or review commands.

```toml
# Extend global timeout
[global]
timeout = 1800  # 30 minutes

# Or extend only specific commands
[[groups.commands]]
name = "long_process"
cmd = "/usr/bin/process"
timeout = 3600  # 1 hour
```

### Q5: Permission errors occur

**Q**: `permission denied` error occurs.

**A**: Execute go-safe-cmd-runner with appropriate privileges.

```bash
# When root privileges are needed
sudo go-safe-cmd-runner -file config.toml

# Or use run_as_user in configuration
```

```toml
[[groups.commands]]
name = "privileged_op"
cmd = "/usr/bin/privileged-tool"
args = []
run_as_user = "root"
```

## 10.6 Support and Help

### Community Resources

- **Documentation**: Refer to official documentation
- **Issue Tracker**: Bug reports and questions on GitHub Issues
- **Sample Configurations**: Refer to configuration examples in `sample/` directory

### Collecting Debug Information

Include the following information when reporting issues:

```bash
# Version information
go-safe-cmd-runner --version

# Configuration file (excluding sensitive information)
cat config.toml

# Error logs (debug level)
go-safe-cmd-runner --log-level=debug --file config.toml 2>&1 | tee debug.log
```

## Summary

This chapter covered the following troubleshooting techniques:

1. **Common Errors**: Configuration files, environment variables, variable expansion, file verification errors and solutions
2. **Configuration Validation**: Syntax checking, incremental validation, log level utilization
3. **Debugging Techniques**: Echo commands, output capture, dry run, permission checking
4. **Performance**: Improving startup and execution speed
5. **FAQ**: Common questions and answers

Using this knowledge, you can quickly diagnose and resolve problems.

## Next Steps

The appendix provides parameter reference tables, sample configuration collections, and a glossary. Use them as references.
