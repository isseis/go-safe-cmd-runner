# Chapter 6: Command Level Configuration [[groups.commands]]

## Overview

The `[[groups.commands]]` section defines the commands to be actually executed. Each group requires one or more commands. Commands are executed in the order they are defined within the group.

## 6.1 Basic Command Settings

### 6.1.1 name - Command Name

#### Overview

Specifies a unique name to identify the command.

#### Syntax

```toml
[[groups.commands]]
name = "command_name"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Required |
| **Configurable Level** | Command only |
| **Valid Values** | Alphanumeric characters, underscores, hyphens |
| **Uniqueness** | Must be unique within the group |

#### Configuration Example

```toml
version = "1.0"

[[groups]]
name = "backup_tasks"

[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]

[[groups.commands]]
name = "backup_config"
cmd = "/usr/bin/tar"
args = ["-czf", "config.tar.gz", "/etc/myapp"]
```

### 6.1.2 description - Description

#### Overview

Human-readable text describing the purpose or role of the command.

#### Syntax

```toml
[[groups.commands]]
name = "example"
description = "Command description"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional (recommended) |
| **Configurable Level** | Command only |
| **Valid Values** | Any string |

#### Configuration Example

```toml
[[groups.commands]]
name = "daily_backup"
description = "Daily full PostgreSQL database backup (all tables)"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases"]
```

### 6.1.3 cmd - Command to Execute

#### Overview

Specifies the path or name of the command to execute. This is the most important parameter of a command.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command_path"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Required |
| **Configurable Level** | Command only |
| **Valid Values** | Absolute path, or command name on PATH |
| **Variable Expansion** | ${VAR} format variable expansion is possible (see Chapter 7) |

#### Configuration Examples

#### Example 1: Specifying Absolute Path

```toml
[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"
args = ["-la"]
```

#### Example 2: Command Name on PATH

```toml
[[groups.commands]]
name = "list_files"
cmd = "ls"  # Searched from PATH
args = ["-la"]
```

#### Example 3: Using Variable Expansion

```toml
[[groups.commands]]
name = "custom_tool"
cmd = "${TOOL_DIR}/my-script"
args = []
env = ["TOOL_DIR=/opt/tools"]
# Actually executes /opt/tools/my-script
```

#### Security Notes

1. **Absolute Paths Recommended**: For security, using absolute paths is recommended
2. **Dangers of PATH Dependency**: When using commands on PATH, unintended commands may be executed
3. **Importance of Verification**: Verify command integrity with `verify_files`

```toml
# Recommended: absolute path and verification
[global]
verify_files = ["/usr/bin/pg_dump"]

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/pg_dump"  # Absolute path
args = ["mydb"]

# Not recommended: PATH dependency
[[groups.commands]]
name = "backup"
cmd = "pg_dump"  # Unclear which pg_dump will be executed
args = ["mydb"]
```

### 6.1.4 args - Arguments

#### Overview

Specifies arguments to pass to the command as an array.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = ["arg1", "arg2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Default Value** | [] (no arguments) |
| **Valid Values** | List of any strings |
| **Variable Expansion** | ${VAR} format variable expansion is possible (see Chapter 7) |

#### Configuration Examples

#### Example 1: Basic Arguments

```toml
[[groups.commands]]
name = "echo_message"
cmd = "echo"
args = ["Hello, World!"]
```

#### Example 2: Multiple Arguments

```toml
[[groups.commands]]
name = "copy_file"
cmd = "/bin/cp"
args = ["-v", "/source/file.txt", "/dest/file.txt"]
```

#### Example 3: No Arguments

```toml
[[groups.commands]]
name = "show_date"
cmd = "date"
args = []  # Or omit
```

#### Example 4: Arguments with Variable Expansion

```toml
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/tar"
args = ["-czf", "${BACKUP_FILE}", "${SOURCE_DIR}"]
env = [
    "BACKUP_FILE=/backups/backup.tar.gz",
    "SOURCE_DIR=/data",
]
```

#### Important Notes

##### 1. Argument Security

Specify each argument as a separate array element. Shell quoting and escaping are not needed.

```toml
# Correct: specify arguments individually
[[groups.commands]]
name = "find_files"
cmd = "/usr/bin/find"
args = ["/var/log", "-name", "*.log", "-type", "f"]

# Wrong: do not combine into a single space-separated string
[[groups.commands]]
name = "find_files"
cmd = "/usr/bin/find"
args = ["/var/log -name *.log -type f"]  # This is treated as a single argument
```

##### 2. Shell Features Are Not Available

go-safe-cmd-runner executes commands directly without using a shell. The following shell features are not available:

```toml
# Wrong: pipes are not available
[[groups.commands]]
name = "grep_and_count"
cmd = "grep"
args = ["ERROR", "app.log", "|", "wc", "-l"]  # Pipes don't work

# Wrong: redirects are not available
[[groups.commands]]
name = "save_output"
cmd = "echo"
args = ["test", ">", "output.txt"]  # Redirects don't work

# Correct: use output parameter
[[groups.commands]]
name = "save_output"
cmd = "echo"
args = ["test"]
output = "output.txt"  # This is the correct way
```

##### 3. Arguments with Spaces

Arguments containing spaces can be naturally handled as array elements:

```toml
[[groups.commands]]
name = "echo_message"
cmd = "echo"
args = ["This is a message with spaces"]  # Contains spaces but is a single argument
```

## 6.2 Environment Settings

### 6.2.1 env - Environment Variables

#### Overview

Specifies environment variables to set during command execution in `KEY=VALUE` format.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
env = ["KEY1=value1", "KEY2=value2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Default Value** | [] |
| **Format** | "KEY=VALUE" |
| **Variable Expansion** | ${VAR} format variable expansion is possible in VALUE part |

#### Role

- **Command Configuration**: Control command behavior with environment variables
- **Credentials**: Settings like database connection information
- **Operation Mode**: Switch modes like debug mode

#### Configuration Examples

#### Example 1: Basic Environment Variables

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/app/server"
args = []
env = [
    "LOG_LEVEL=debug",
    "PORT=8080",
    "CONFIG_FILE=/etc/app/config.yaml",
]
```

#### Example 2: Database Connection Information

```toml
[[groups.commands]]
name = "db_migration"
cmd = "/opt/app/migrate"
args = []
env = [
    "DATABASE_URL=postgresql://localhost:5432/mydb",
    "DB_USER=appuser",
    "DB_PASSWORD=secret123",
]
```

#### Example 3: Using Variable Expansion

```toml
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup.sh"
args = []
env = [
    "BACKUP_DIR=/var/backups",
    "BACKUP_FILE=${BACKUP_DIR}/backup-${DATE}.tar.gz",
    "DATE=2025-01-15",
]
# BACKUP_FILE expands to /var/backups/backup-2025-01-15.tar.gz
```

#### Important Notes

##### 1. Relationship with env_allowlist

Set environment variables must be included in the group or global `env_allowlist`:

```toml
[global]
env_allowlist = ["PATH", "LOG_LEVEL", "DATABASE_URL"]

[[groups]]
name = "app_group"

[[groups.commands]]
name = "run_app"
cmd = "/opt/app/server"
args = []
env = [
    "LOG_LEVEL=debug",      # OK: included in env_allowlist
    "DATABASE_URL=...",     # OK: included in env_allowlist
    "UNAUTHORIZED_VAR=x",   # Error: not included in env_allowlist
]
```

##### 2. Format Rules

- `KEY=VALUE` format is required
- Error if `=` is not included
- Even if VALUE is empty, `KEY=` notation is required

```toml
# Correct
env = [
    "KEY=value",
    "EMPTY_VAR=",  # Empty value
]

# Wrong
env = [
    "KEY",         # Error: no =
    "KEY value",   # Error: no =
]
```

##### 3. No Duplicates

The same key cannot be defined multiple times:

```toml
# Wrong: LOG_LEVEL is duplicated
env = [
    "LOG_LEVEL=debug",
    "LOG_LEVEL=info",  # Error: duplicate
]
```

### 6.2.2 dir - Execution Directory

#### Overview

Specifies an execution directory specific to this command.

> **Note**: In the current version, the `dir` parameter is not implemented. The working directory should be controlled with group-level `workdir` or global `workdir`.

## 6.3 Timeout Settings

### 6.3.1 timeout - Command-Specific Timeout

#### Overview

Specifies a timeout specific to this command in seconds. Overrides global `timeout`.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
timeout = seconds
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Integer (int) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Command |
| **Default Value** | Global timeout |
| **Valid Values** | Positive integer (in seconds) |
| **Override** | Overrides global setting |

#### Configuration Examples

#### Example 1: Long-Running Command

```toml
[global]
timeout = 60  # Default: 60 seconds

[[groups]]
name = "mixed_tasks"

[[groups.commands]]
name = "quick_check"
cmd = "ping"
args = ["-c", "3", "localhost"]
# timeout not specified → global 60 seconds

[[groups.commands]]
name = "long_backup"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases"]
output = "full_backup.sql"
timeout = 1800  # 30 minutes = 1800 seconds
```

#### Example 2: Gradual Timeout Settings

```toml
[global]
timeout = 300  # Default: 5 minutes

[[groups]]
name = "backup_tasks"

[[groups.commands]]
name = "small_db_backup"
cmd = "/usr/bin/pg_dump"
args = ["small_db"]
timeout = 60  # 1 minute is sufficient

[[groups.commands]]
name = "medium_db_backup"
cmd = "/usr/bin/pg_dump"
args = ["medium_db"]
# Uses global 300 seconds (5 minutes)

[[groups.commands]]
name = "large_db_backup"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
timeout = 3600  # 1 hour
```

## 6.4 Privilege Management

### 6.4.1 run_as_user - Execution User

#### Overview

Executes the command with specific user privileges.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
run_as_user = "username"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Valid Values** | Username that exists on the system |
| **Prerequisites** | go-safe-cmd-runner must be running with root privileges |

#### Configuration Examples

#### Example 1: Command Execution with root Privileges

```toml
[[groups.commands]]
name = "system_update"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
# Package update requiring root privileges
```

#### Example 2: Command Execution as Specific User

```toml
[[groups.commands]]
name = "user_backup"
cmd = "/home/appuser/backup.sh"
args = []
run_as_user = "appuser"
# Execute script with appuser privileges
```

#### Security Notes

1. **Principle of Least Privilege**: Execute with minimal necessary privileges
2. **Minimize root Use**: Use root privileges only when truly necessary
3. **Audit Logs**: Privilege escalation is automatically recorded in audit logs

### 6.4.2 run_as_group - Execution Group

#### Overview

Executes the command with specific group privileges.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
run_as_group = "group_name"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Valid Values** | Group name that exists on the system |
| **Prerequisites** | go-safe-cmd-runner must be running with appropriate privileges |

#### Configuration Example

```toml
[[groups.commands]]
name = "read_log"
cmd = "/usr/bin/cat"
args = ["/var/log/app/app.log"]
run_as_group = "loggroup"
# Read logs with loggroup group privileges
```

#### Combined Example

```toml
[[groups.commands]]
name = "privileged_operation"
cmd = "/opt/admin/tool"
args = []
run_as_user = "admin"
run_as_group = "admingroup"
# Execute with admin user and admingroup group privileges
```

## 6.5 Risk Management

### 6.5.1 max_risk_level - Maximum Risk Level

#### Overview

Specifies the maximum risk level allowed for a command. If the command's risk exceeds the specified level, execution is rejected.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
max_risk_level = "risk_level"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Default Value** | "low" |
| **Valid Values** | "low", "medium", "high" |

### 6.5.2 Risk Level Types

#### Risk Level Definitions

| Level | Description | Examples |
|-------|-------------|----------|
| **low** | Low risk | Read-only commands, information retrieval |
| **medium** | Medium risk | File creation/modification, network access |
| **high** | High risk | System configuration changes, package installation |

#### Risk Assessment Mechanism

go-safe-cmd-runner automatically assesses command risk from the following elements:

1. **Command Type**: Dangerous commands like rm, chmod, chown
2. **Argument Patterns**: Recursive deletion (-rf), forced execution (-f), etc.
3. **Privilege Escalation**: Use of run_as_user, run_as_group
4. **Network Access**: Network commands like curl, wget

#### Configuration Examples

#### Example 1: Low-Risk Command

```toml
[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"
args = ["-la"]
max_risk_level = "low"  # Read-only, so low risk
```

#### Example 2: Medium-Risk Command

```toml
[[groups.commands]]
name = "create_backup"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
max_risk_level = "medium"  # File creation, so medium risk
```

#### Example 3: High-Risk Command

```toml
[[groups.commands]]
name = "install_package"
cmd = "/usr/bin/apt-get"
args = ["install", "-y", "package-name"]
run_as_user = "root"
max_risk_level = "high"  # System changes and privilege escalation, so high risk
```

#### Example 4: Behavior When Risk Level is Violated

```toml
[[groups.commands]]
name = "dangerous_operation"
cmd = "/bin/rm"
args = ["-rf", "/tmp/data"]
max_risk_level = "low"  # rm -rf is medium risk or higher
# This command is rejected (risk level exceeded)
```

#### Security Best Practices

```toml
# Recommended: appropriate risk level settings
[[groups]]
name = "safe_operations"

[[groups.commands]]
name = "read_config"
cmd = "/bin/cat"
args = ["/etc/app/config.yaml"]
max_risk_level = "low"  # Read only

[[groups.commands]]
name = "backup_data"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
max_risk_level = "medium"  # File creation

[[groups.commands]]
name = "system_update"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
max_risk_level = "high"  # System changes and privilege escalation
```

## 6.6 Output Management

### 6.6.1 output - Standard Output Capture

#### Overview

Saves the command's standard output to a file.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
output = "file_path"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Valid Values** | Relative path or absolute path |
| **Size Limit** | Limited by global max_output_size |
| **Directory Creation** | Automatically created as needed |

#### Role

- **Log Preservation**: Persist command output
- **Record Results**: Save processing results as files
- **Audit Trail**: Store as evidence of execution history

#### Configuration Examples

#### Example 1: Output with Relative Path

```toml
[[groups]]
name = "data_export"
workdir = "/var/app/output"

[[groups.commands]]
name = "export_users"
cmd = "/opt/app/export"
args = ["--table", "users"]
output = "users.csv"
# Saved to /var/app/output/users.csv
```

#### Example 2: Output with Absolute Path

```toml
[[groups.commands]]
name = "system_report"
cmd = "/usr/bin/systemctl"
args = ["status"]
output = "/var/log/reports/system_status.txt"
# Saved with absolute path
```

#### Example 3: Output to Subdirectory

```toml
[[groups]]
name = "log_export"
workdir = "/var/app"

[[groups.commands]]
name = "export_logs"
cmd = "/opt/app/export-logs"
args = []
output = "logs/export/output.txt"
# /var/app/logs/export/ directory is automatically created,
# saved to /var/app/logs/export/output.txt
```

#### Example 4: Output from Multiple Commands

```toml
[[groups]]
name = "system_info"
workdir = "/tmp/reports"

[[groups.commands]]
name = "disk_usage"
cmd = "/bin/df"
args = ["-h"]
output = "disk_usage.txt"

[[groups.commands]]
name = "memory_info"
cmd = "/usr/bin/free"
args = ["-h"]
output = "memory_info.txt"

[[groups.commands]]
name = "process_list"
cmd = "/bin/ps"
args = ["aux"]
output = "processes.txt"
```

#### Important Notes

##### 1. Size Limit

Output size is limited by `max_output_size` (global setting):

```toml
[global]
max_output_size = 1048576  # 1MB

[[groups.commands]]
name = "large_export"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
output = "dump.sql"
# If output exceeds 1MB, a warning is recorded
```

##### 2. Permissions

Output file permissions are set as follows:
- File: 0600 (readable/writable by owner only)
- Directory: 0700 (accessible by owner only)

##### 3. Handling Existing Files

If a file with the same name exists, it will be overwritten:

```toml
[[groups.commands]]
name = "daily_report"
cmd = "/opt/app/report"
args = []
output = "daily.txt"
# Existing daily.txt is overwritten
```

##### 4. Standard Error Output

The `output` parameter captures only standard output (stdout). Standard error output (stderr) is recorded in normal logs.

## Overall Command Configuration Example

Below is a practical example combining command-level settings:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/var/app"
log_level = "info"
env_allowlist = ["PATH", "HOME", "DATABASE_URL", "BACKUP_DIR"]
max_output_size = 10485760  # 10MB
verify_files = ["/bin/sh"]

[[groups]]
name = "database_operations"
description = "Database-related operations"
priority = 10
workdir = "/var/backups/db"
env_allowlist = ["PATH", "DATABASE_URL", "BACKUP_DIR"]
verify_files = ["/usr/bin/pg_dump", "/usr/bin/psql"]

# Command 1: Database backup
[[groups.commands]]
name = "full_backup"
description = "Backup of all PostgreSQL databases"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases", "--verbose"]
env = ["DATABASE_URL=postgresql://localhost/postgres"]
output = "full_backup.sql"
timeout = 1800  # 30 minutes
max_risk_level = "medium"

# Command 2: Verify backup
[[groups.commands]]
name = "verify_backup"
description = "Verify backup file integrity"
cmd = "/usr/bin/psql"
args = ["--dry-run", "-f", "full_backup.sql"]
env = ["DATABASE_URL=postgresql://localhost/testdb"]
output = "verification.log"
timeout = 600  # 10 minutes
max_risk_level = "low"

# Command 3: Delete old backups
[[groups.commands]]
name = "cleanup_old_backups"
description = "Delete backup files older than 30 days"
cmd = "/usr/bin/find"
args = [".", "-name", "*.sql", "-mtime", "+30", "-delete"]
timeout = 300  # 5 minutes
max_risk_level = "medium"

[[groups]]
name = "system_maintenance"
description = "System maintenance tasks"
priority = 20
workdir = "/tmp"
env_allowlist = []  # No environment variables

# Command 4: Disk usage report
[[groups.commands]]
name = "disk_report"
description = "Generate disk usage report"
cmd = "/bin/df"
args = ["-h", "/var"]
output = "/var/log/disk_usage.txt"
timeout = 60
max_risk_level = "low"

# Command 5: System update (root privileges)
[[groups.commands]]
name = "system_update"
description = "Update system packages"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
timeout = 600
max_risk_level = "high"
```

## Next Steps

The next chapter will provide detailed explanations of variable expansion functionality. You will learn how to perform dynamic command construction using `${VAR}` format variables.
