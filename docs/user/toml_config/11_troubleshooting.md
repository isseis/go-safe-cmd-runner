# Chapter 11: Troubleshooting

This chapter introduces common problems when creating configuration files and their solutions. Let's learn how to read error messages and debugging techniques.

## 11.1 Common Errors and Solutions

### 11.1.1 Configuration File Loading Errors

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

### 11.1.2 Version Specification Error

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

### 11.1.3 Missing Required Fields

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

### 11.1.4 Environment Variable Permission Error

#### Error Example

```
Error: environment variable 'CUSTOM_VAR' is not allowed by env_allowed
```

#### Cause

The environment variable being used is not included in `env_allowed`.

#### Solution

**Method 1**: Add to global or group's `env_allowed`

```toml
[global]
env_allowed = ["PATH", "HOME", "CUSTOM_VAR"]  # Add CUSTOM_VAR
```

**Method 2**: Define as internal variable using vars (recommended)

```toml
# No need to add to env_allowed
[[groups.commands]]
name = "custom_command"
cmd = "%{custom_tool}"
args = []

[groups.commands.vars]
custom_tool = "/opt/tools/mytool"  # Define as internal variable using vars
```

### 11.1.5 Variable Expansion Error

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
# Wrong: tool_dir is not defined
[[groups.commands]]
name = "run_tool"
cmd = "%{tool_dir}/mytool"
args = []

# Correct: Define using vars
[[groups.commands]]
name = "run_tool"
cmd = "%{tool_dir}/mytool"
args = []

[groups.commands.vars]
tool_dir = "/opt/tools"
```

**For circular references**:

```toml
# Wrong: Circular reference
env_vars = [
    "VAR1=${VAR2}",
    "VAR2=${VAR1}",
]

# Correct: Resolve the circular reference
env_vars = [
    "VAR1=/path/to/dir",
    "VAR2=${VAR1}/subdir",
]
```

### 11.1.6 File Verification Error

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
record /usr/bin/tool /opt/app/script.sh
```

**If file was legitimately changed**:

```bash
# Re-record hash
record /usr/bin/tool
```

### 11.1.7 Command Path Errors

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

### 11.1.8 Timeout Errors

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

### 11.1.9 Permission Errors

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

### 11.1.10 Risk Level Exceeded Error

#### Error Example

```
Error: command risk level exceeds maximum: command risk=medium, risk_level=low
```

#### Cause

Command risk level exceeds `risk_level`.

#### Solution

```toml
# Method 1: Increase risk_level
[[groups.commands]]
name = "risky_command"
cmd = "/bin/rm"
args = ["-rf", "/tmp/data"]
risk_level = "medium"  # Change low → medium

# Method 2: Change to safer command
[[groups.commands]]
name = "safer_command"
cmd = "/bin/rm"
args = ["/tmp/data/specific-file.txt"]  # Remove -rf
risk_level = "low"
```

## 11.2 Configuration Validation Methods

### 11.2.1 Syntax Checking

Validate configuration file syntax:

```bash
# Pre-execution validation with dry run
go-safe-cmd-runner --dry-run --file config.toml
```

### 11.2.2 Incremental Validation

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
args = ["Value: %{var}"]

[groups.commands.vars]
var = "hello"
```

```bash
# Confirm variable expansion behavior
go-safe-cmd-runner -file with-vars.toml
```

### 11.2.3 Utilizing Log Levels

Enable detailed logs when debugging: `-log-level debug`

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

## 11.3 Debugging Techniques

### 11.3.1 Verifying Variables with Echo Command

Confirm variables are expanded correctly:

```toml
# Debug command
[[groups.commands]]
name = "debug_variables"
cmd = "/usr/bin/env"
args = []
env_vars = [
    "TOOL_DIR=/opt/tools",
    "CONFIG=/etc/app/config.yml",
    "ENV_TYPE=production",
]
output_file = "debug-vars.txt"
```

After execution, check `debug-vars.txt`:
```
TOOL_DIR=/opt/tools
CONFIG=/etc/app/config.yml
ENV_TYPE=production
PATH=/usr/bin:/bin
... (other environment variables)
```

### 11.3.2 Diagnosis with Output Capture

Save command output to examine details:

```toml
[[groups.commands]]
name = "diagnose"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp.service"]
output_file = "service-status.txt"
```

After execution, check output file:
```bash
cat service-status.txt
```

### 11.3.3 Testing Individual Commands

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
env_vars = ["CUSTOM_VAR=test"]
```

### 11.3.4 Utilizing Dry Run

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

### 11.3.5 Checking Permissions

Diagnose permission-related issues:

```toml
# Permission check commands
[[groups.commands]]
name = "check_permissions"
cmd = "/usr/bin/id"
args = []
output_file = "current-user.txt"

[[groups.commands]]
name = "check_file_access"
cmd = "/bin/ls"
args = ["-la", "/path/to/file"]
output_file = "file-permissions.txt"
```

### 11.3.6 Checking Environment Variables

Diagnose environment variable state:

```toml
[[groups.commands]]
name = "dump_env"
cmd = "/usr/bin/env"
args = []
output_file = "environment.txt"
```

## 11.4 Performance Issues

### 11.4.1 Slow Startup

#### Cause

- Large number of file verifications
- Heavy initialization processing

#### Solution

```toml
# Skip standard path verification
[global]
verify_standard_paths = false

# Verify only minimum necessary files
verify_files = [
    "/opt/app/bin/critical-tool",
]
```

### 11.4.2 Slow Execution

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

## 11.5 Frequently Asked Questions (FAQ)

### Q1: Variables are not expanded

**Q**: `%{HOME}` is not expanded and is treated as a literal string.

**A**: Internal variables must be defined in the `vars` field. Variables defined in `env_vars` cannot be used for TOML expansion.

```toml
# Incorrect: Variables defined in env_vars cannot be expanded
[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["%{my_home}"]
env_vars = ["MY_HOME=/home/user"]  # This only sets child process environment

# Correct: Define internal variable using vars
[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["%{my_home}"]

[groups.commands.vars]
my_home = "/home/user"
```

### Q2: Command not found

**Q**: `command not found` error occurs.

**A**: Use absolute paths or verify PATH is set correctly.

```toml
# Recommended: Absolute path
cmd = "/usr/bin/tool"

# Or: Check PATH
[global]
env_allowed = ["PATH"]
```

### Q3: File verification fails

**Q**: Hash validation error occurs.

**A**: Create or update hash files.

```bash
# Record individual files
record config.toml /usr/bin/tool
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

## 11.6 Support and Help

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
