# Chapter 4: Global Level Configuration [global]

## Overview

The `[global]` section defines common settings that apply to all groups and commands. While this section is optional, it is recommended to use it for centralized management of default values.

## 4.1 timeout - Timeout Setting

### Overview

Specifies the maximum wait time for command execution in seconds.

### Syntax

```toml
[global]
timeout = seconds
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Integer (int) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Command |
| **Default Value** | System default (typically no limit) |
| **Valid Values** | Positive integer (in seconds) |
| **Override** | Can be overridden at command level |

### Role

- **Prevent Infinite Loops**: Automatically terminates commands that hang
- **Resource Management**: Prevents excessive system resource occupation
- **Predictable Execution Time**: Makes batch processing completion time predictable

### Configuration Examples

#### Example 1: Setting Global Timeout

```toml
version = "1.0"

[global]
timeout = 60  # Set default timeout for all commands to 60 seconds

[[groups]]
name = "quick_tasks"

[[groups.commands]]
name = "fast_command"
cmd = "echo"
args = ["Complete"]
# timeout not specified → uses global 60 seconds
```

#### Example 2: Override at Command Level

```toml
version = "1.0"

[global]
timeout = 60  # Default: 60 seconds

[[groups]]
name = "mixed_tasks"

[[groups.commands]]
name = "quick_check"
cmd = "ping"
args = ["-c", "3", "localhost"]
# timeout not specified → uses global 60 seconds

[[groups.commands]]
name = "long_backup"
cmd = "/usr/bin/backup.sh"
args = []
timeout = 300  # Set to 300 seconds only for this command
```

### Behavior Details

When a timeout occurs:
1. Sends termination signal (SIGTERM) to the running command
2. After waiting for a certain period, sends forced termination signal (SIGKILL)
3. Records as error and proceeds to the next command

### Notes

#### 1. Selecting Timeout Values

Set appropriate values considering the command execution time:

```toml
[global]
timeout = 10  # Too short: normal commands may fail

[[groups.commands]]
name = "database_dump"
cmd = "/usr/bin/pg_dump"
args = ["large_database"]
# Likely won't complete in 10 seconds → timeout error
```

#### 2. 0 or Negative Values Are Invalid

```toml
[global]
timeout = 0   # Invalid setting
timeout = -1  # Invalid setting
```

## 4.2 workdir - Working Directory

### Overview

Specifies the working directory (current directory) where commands are executed.

### Syntax

```toml
[global]
workdir = "directory_path"
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group |
| **Default Value** | Execution directory of go-safe-cmd-runner |
| **Valid Values** | Absolute path |
| **Override** | Can be overridden at group level |

### Role

- **Unified Execution Environment**: Ensures all commands run in the same directory
- **Base for Relative Paths**: Sets the base directory for commands using relative paths
- **Security**: Prevents command execution in unexpected directories

### Configuration Examples

#### Example 1: Setting Global Working Directory

```toml
version = "1.0"

[global]
workdir = "/var/app/workspace"

[[groups]]
name = "file_operations"

[[groups.commands]]
name = "create_file"
cmd = "touch"
args = ["test.txt"]
# /var/app/workspace/test.txt will be created
```

#### Example 2: Override at Group Level

```toml
version = "1.0"

[global]
workdir = "/tmp"

[[groups]]
name = "log_processing"
workdir = "/var/log/app"  # Group-specific working directory

[[groups.commands]]
name = "grep_errors"
cmd = "grep"
args = ["ERROR", "app.log"]
# Executed in /var/log/app directory
```

### Notes

#### 1. Use Absolute Paths

Relative paths cannot be used:

```toml
[global]
workdir = "./workspace"  # Error: relative paths not allowed
workdir = "/tmp/workspace"  # Correct: absolute path
```

#### 2. Directory Existence Check

If the specified directory does not exist, an error occurs:

```toml
[global]
workdir = "/nonexistent/directory"  # Error if directory doesn't exist
```

#### 3. Permission Check

Read and write permissions are required for the specified directory.

## 4.3 log_level - Log Level

### Overview

Controls the verbosity of log output.

### Syntax

```toml
[global]
log_level = "log_level"
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global only |
| **Default Value** | "info" |
| **Valid Values** | "debug", "info", "warn", "error" |
| **Override** | Not possible (global level only) |

### Log Level Details

| Level | Use Case | Output Information |
|-------|----------|-------------------|
| **debug** | Development/debugging | All detailed information (variable values, internal states, etc.) |
| **info** | Normal operation | Execution status, completion notifications, etc. |
| **warn** | Warning monitoring | Warnings and important information only |
| **error** | Errors only | Error messages only |

### Configuration Examples

#### Example 1: Debug Mode

```toml
version = "1.0"

[global]
log_level = "debug"  # Output detailed debug information

[[groups]]
name = "troubleshooting"

[[groups.commands]]
name = "test_command"
cmd = "echo"
args = ["test"]
```

Output example:
```
[DEBUG] Configuration loaded: version=1.0
[DEBUG] Global settings: timeout=default, workdir=default
[DEBUG] Processing group: troubleshooting
[DEBUG] Executing command: test_command
[DEBUG] Command path: /usr/bin/echo
[DEBUG] Arguments: [test]
[INFO] Command completed successfully
```

#### Example 2: Production Environment (info level)

```toml
version = "1.0"

[global]
log_level = "info"  # Output standard information only

[[groups]]
name = "production"

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup.sh"
args = []
```

Output example:
```
[INFO] Starting command group: production
[INFO] Executing command: backup
[INFO] Command completed successfully
```

#### Example 3: Errors Only (error level)

```toml
version = "1.0"

[global]
log_level = "error"  # Output errors only

[[groups]]
name = "silent_operation"

[[groups.commands]]
name = "routine_check"
cmd = "test"
args = ["-f", "/tmp/check.txt"]
```

No output during normal operation; messages are displayed only when errors occur.

### Best Practices

- **During Development**: Use `debug` level to check details
- **During Testing**: Use `info` level to verify execution status
- **Production Environment**: Use `info` or `warn` level
- **Silent Operation**: Use `error` level to record errors only

## 4.4 skip_standard_paths - Skip Standard Path Verification

### Overview

Skips file verification for standard system paths (`/bin`, `/usr/bin`, etc.).

### Syntax

```toml
[global]
skip_standard_paths = true/false
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Boolean (boolean) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global only |
| **Default Value** | false |
| **Valid Values** | true, false |

### Role

- **Performance Improvement**: Reduces startup time by skipping verification of standard commands
- **Convenience**: Eliminates the need to create hash files for standard system commands

### Configuration Examples

#### Example 1: Skip Verification of Standard Paths

```toml
version = "1.0"

[global]
skip_standard_paths = true  # Skip verification of /bin, /usr/bin, etc.

[[groups]]
name = "system_commands"

[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"  # Can execute without verification
args = ["-la"]
```

#### Example 2: Verify All Commands (Default)

```toml
version = "1.0"

[global]
skip_standard_paths = false  # Or omit
verify_files = ["/bin/ls", "/usr/bin/grep"]  # Explicit hash specification required

[[groups]]
name = "verified_commands"

[[groups.commands]]
name = "search"
cmd = "/usr/bin/grep"
args = ["pattern", "file.txt"]
```

### Security Notice

Setting `skip_standard_paths = true` will not detect tampering of commands in standard paths. For environments with high security requirements, it is recommended to keep it as `false` (default).

## 4.5 env_allowlist - Environment Variable Allowlist

### Overview

Specifies environment variables allowed to be used during command execution. All environment variables not in the list are excluded.

### Syntax

```toml
[global]
env_allowlist = ["variable1", "variable2", ...]
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group |
| **Default Value** | [] (deny all environment variables) |
| **Valid Values** | List of environment variable names |
| **Override** | Can be inherited/overridden at group level (see Chapter 5) |

### Role

- **Security**: Prevents leakage of unnecessary environment variables
- **Environment Uniformity**: Makes command execution environment predictable
- **Principle of Least Privilege**: Allows only necessary environment variables

### Configuration Examples

#### Example 1: Allowing Basic Environment Variables

```toml
version = "1.0"

[global]
env_allowlist = [
    "PATH",    # Command search path
    "HOME",    # Home directory
    "USER",    # Username
    "LANG",    # Language settings
]

[[groups]]
name = "basic_commands"

[[groups.commands]]
name = "show_env"
cmd = "printenv"
args = []
# Only PATH, HOME, USER, LANG are available
```

#### Example 2: Application-Specific Environment Variables

```toml
version = "1.0"

[global]
env_allowlist = [
    "PATH",
    "HOME",
    "APP_CONFIG_DIR",   # App configuration directory
    "APP_LOG_LEVEL",    # Log level
    "DATABASE_URL",     # Database connection string
]

[[groups]]
name = "app_tasks"

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "${APP_CONFIG_DIR}/config.yaml"]
env = ["APP_CONFIG_DIR=/etc/myapp"]
```

#### Example 3: Empty List (Deny All)

```toml
version = "1.0"

[global]
env_allowlist = []  # Deny all environment variables

[[groups]]
name = "isolated_tasks"

[[groups.commands]]
name = "pure_command"
cmd = "/bin/echo"
args = ["Hello"]
# Executed without environment variables
```

### Commonly Used Environment Variables

| Variable Name | Purpose | Recommendation |
|---------------|---------|----------------|
| PATH | Command search path | High (almost essential) |
| HOME | Home directory | High |
| USER | Username | Medium |
| LANG, LC_ALL | Language/locale settings | Medium |
| TZ | Timezone | Low |
| TERM | Terminal type | Low |

### Security Best Practices

1. **Minimal Allowance**: Allow only necessary variables
2. **Exclude Sensitive Information**: Do not allow variables containing passwords or tokens
3. **Regular Review**: Remove variables that are no longer needed

```toml
# Not recommended: overly permissive
[global]
env_allowlist = [
    "PATH", "HOME", "USER", "SHELL", "EDITOR", "PAGER",
    "MAIL", "LOGNAME", "HOSTNAME", "DISPLAY", "XAUTHORITY",
    # ... too many
]

# Recommended: minimal necessary
[global]
env_allowlist = ["PATH", "HOME", "USER"]
```

## 4.6 verify_files - File Verification List

### Overview

Specifies a list of files to verify for integrity before execution. The specified files are checked against hash values, and execution is aborted if tampering is detected.

### Syntax

```toml
[global]
verify_files = ["file_path1", "file_path2", ...]
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group |
| **Default Value** | [] (no verification) |
| **Valid Values** | List of absolute paths |
| **Merge Behavior** | Merged with group-level settings |

### Role

- **Tampering Detection**: Verifies that files have not been modified
- **Security**: Prevents execution of malicious code
- **Integrity Assurance**: Ensures the intended version of files is used

### Configuration Examples

#### Example 1: Basic File Verification

```toml
version = "1.0"

[global]
verify_files = [
    "/bin/sh",
    "/bin/bash",
    "/usr/bin/python3",
]

[[groups]]
name = "scripts"

[[groups.commands]]
name = "run_script"
cmd = "/usr/bin/python3"
args = ["script.py"]
# Verifies hash of /usr/bin/python3 before execution
```

#### Example 2: Additional Verification at Group Level

```toml
version = "1.0"

[global]
verify_files = ["/bin/sh"]  # Verified across all groups

[[groups]]
name = "database_group"
verify_files = ["/usr/bin/psql", "/usr/bin/pg_dump"]  # Group-specific verification

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
# /bin/sh, /usr/bin/psql, /usr/bin/pg_dump are verified (merged)
```

### Verification Mechanism

1. **Pre-create Hash Files**: Record file hashes using the `record` command
2. **Execution-time Verification**: Verify hashes of files specified in configuration
3. **Behavior on Mismatch**: Abort execution and report error if hashes don't match

### How to Create Hash Files

```bash
# Record hashes of verification target files using record command
$ go-safe-cmd-runner record config.toml

# Or specify files individually
$ go-safe-cmd-runner record /bin/sh /usr/bin/python3
```

### Notes

#### 1. Absolute Paths Required

```toml
[global]
verify_files = ["./script.sh"]  # Error: relative paths not allowed
verify_files = ["/opt/app/script.sh"]  # Correct
```

#### 2. Hash File Management

If the hash of a specified file has not been recorded in advance, a verification error occurs.

#### 3. Performance Impact

Verifying many files increases startup time. Specify only necessary files.

## 4.7 max_output_size - Maximum Output Size

### Overview

Specifies the maximum size in bytes when capturing command standard output.

### Syntax

```toml
[global]
max_output_size = bytes
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Integer (int64) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global only |
| **Default Value** | 10485760 (10MB) |
| **Valid Values** | Positive integer (in bytes) |
| **Override** | Not possible (global level only) |

### Role

- **Resource Protection**: Prevents increased disk usage from excessive output
- **Memory Management**: Prevents memory exhaustion
- **Predictable Behavior**: Clarifies the upper limit on output size

### Configuration Examples

#### Example 1: 1MB Limit

```toml
version = "1.0"

[global]
max_output_size = 1048576  # 1MB = 1024 * 1024 bytes

[[groups]]
name = "log_analysis"

[[groups.commands]]
name = "grep_logs"
cmd = "grep"
args = ["ERROR", "/var/log/app.log"]
output = "errors.txt"
# Error if output exceeds 1MB
```

#### Example 2: Processing Large Files

```toml
version = "1.0"

[global]
max_output_size = 104857600  # 100MB = 100 * 1024 * 1024 bytes

[[groups]]
name = "data_export"

[[groups.commands]]
name = "export_database"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
output = "database_dump.sql"
# Allow large database dumps
```

#### Example 3: Size Limit Guidelines

```toml
[global]
# Recommended values based on common use cases
max_output_size = 1048576      # 1MB  - log analysis, small-scale data
max_output_size = 10485760     # 10MB - default, medium-scale data
max_output_size = 104857600    # 100MB - large-scale data, database dumps
max_output_size = 1073741824   # 1GB  - very large data (caution required)
```

### Behavior When Limit is Exceeded

When output size exceeds the limit:
1. Command execution continues (only output is limited)
2. Error message warning of excess is recorded
3. Output up to that point is saved

### Best Practices

1. **Configure Based on Use Case**: Consider the size of data to be processed
2. **Set with Margin**: Configure to 1.5-2 times the expected size
3. **Monitoring**: Regularly check cases where the limit was reached

```toml
# Not recommended: limit too small
[global]
max_output_size = 1024  # 1KB - insufficient for most commands

# Recommended: appropriate limit
[global]
max_output_size = 10485760  # 10MB - appropriate for general use
```

## Overall Configuration Example

Below is a practical example combining global-level settings:

```toml
version = "1.0"

[global]
# Timeout setting
timeout = 300  # Default 5 minutes

# Working directory
workdir = "/var/app/workspace"

# Log level
log_level = "info"

# Skip standard path verification
skip_standard_paths = true

# Environment variable allowlist
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "APP_CONFIG_DIR",
    "DATABASE_URL",
]

# File verification list
verify_files = [
    "/opt/app/bin/main",
    "/opt/app/scripts/backup.sh",
]

# Output size limit
max_output_size = 10485760  # 10MB

[[groups]]
name = "application_tasks"
# ... group configuration continues
```

## Next Steps

The next chapter will provide detailed explanations of group-level configuration (`[[groups]]`). You will learn how to group commands and configure group-specific settings.
