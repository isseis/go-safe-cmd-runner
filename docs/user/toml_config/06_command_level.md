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
| **Variable Expansion** | %{VAR} format variable expansion is possible (see Chapter 7) |

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
cmd = "%{TOOL_DIR}/my-script"
args = []
env_vars = ["TOOL_DIR=/opt/tools"]
# Actually executes /opt/tools/my-script
```

#### Security Notes

1. **Absolute Paths Recommended**: For security, using absolute paths is recommended
2. **Dangers of PATH Dependency**: When using commands on PATH, unintended commands may be executed
3. **Automatic Verification**: Command executables are automatically hash-verified

```toml
# Recommended: absolute path (automatically verified)
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/pg_dump"  # Absolute path, automatically verified
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
| **Variable Expansion** | %{VAR} format variable expansion is possible (see Chapter 7) |

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
args = ["-czf", "%{BACKUP_FILE}", "%{SOURCE_DIR}"]
env_vars = [
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
output_file = "output.txt"  # This is the correct way
```

##### 3. Arguments with Spaces

Arguments containing spaces can be naturally handled as array elements:

```toml
[[groups.commands]]
name = "echo_message"
cmd = "echo"
args = ["This is a message with spaces"]  # Contains spaces but is a single argument
```

## 6.2 Variables and Environment Configuration

### 6.2.1 vars - Internal Variables

#### Overview

Specifies internal variables for variable expansion within the TOML file in `KEY=VALUE` format. Variables defined at command level are merged with variables at global and group levels (Union merge).

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
vars = ["key1=value1", "key2=value2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group, Command |
| **Default Value** | [] |
| **Format** | "KEY=VALUE" |
| **Variable Name Constraints** | POSIX compliant (alphanumeric and underscore only, cannot start with digit), `__runner_` prefix is reserved |
| **Inheritance Behavior** | Merge (Union) - lower levels override upper levels |

#### Role

- **Variable Expansion in TOML**: Can be referenced in `cmd`, `args`, `env` values using `%{VAR}` format
- **Non-propagation to Process Environment**: Not included in child process environment variables
- **Hierarchical Merge**: Merged in order: Global → Group → Command

#### Configuration Examples

##### Example 1: Command-Specific Variables

```toml
[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/pg_dump"
vars = [
    "db_name=production_db",
    "backup_dir=/var/backups/postgres",
]
args = ["-d", "%{db_name}", "-f", "%{backup_dir}/%{db_name}.sql"]
```

##### Example 2: Hierarchical Merge

```toml
[global]
vars = ["base_dir=/opt/app", "log_level=info"]

[[groups]]
name = "admin_tasks"
vars = ["log_level=debug"]  # Override global log_level

[[groups.commands]]
name = "task1"
cmd = "/bin/task"
vars = ["task_id=42"]  # Inherit base_dir, log_level; add task_id
args = ["--dir", "%{base_dir}", "--log", "%{log_level}", "--id", "%{task_id}"]
# Final variables: base_dir=/opt/app, log_level=debug, task_id=42
```

#### Important Notes

##### 1. Non-propagation to Process Environment

Variables defined in `vars` are not set as environment variables in child processes:

```toml
[[groups.commands]]
name = "print_vars"
cmd = "/bin/sh"
vars = ["my_var=hello"]
args = ["-c", "echo $my_var"]  # my_var is empty string (not in environment)
```

To pass environment variables to child processes, use `env` parameter:

```toml
[[groups.commands]]
name = "print_vars"
cmd = "/bin/sh"
vars = ["my_var=hello"]
env_vars = ["MY_VAR=%{my_var}"]  # Convert vars value to environment variable
args = ["-c", "echo $MY_VAR"]  # MY_VAR=hello is output
```

##### 2. Variable Name Constraints

Variable names must follow these rules:

- **POSIX Compliant**: Only alphanumeric and underscore; cannot start with digit
- **Reserved Prefix**: Names starting with `__runner_` are reserved for automatic variables

```toml
# Correct examples
vars = ["my_var=value", "VAR_123=value", "_private=value"]

# Wrong examples
vars = [
    "123var=value",      # Starts with digit
    "my-var=value",      # Hyphen not allowed
    "__runner_custom=x", # Reserved prefix
]
```

##### 3. Automatic Variables

Runner provides the following automatic variables (cannot be overridden):

- `__RUNNER_DATETIME`: Command execution time (ISO 8601 format)
- `__RUNNER_PID`: PID of Runner process

```toml
[[groups.commands]]
name = "log_execution"
cmd = "/usr/bin/logger"
args = ["Executed at %{__RUNNER_DATETIME} by PID %{__RUNNER_PID}"]
```

### 6.2.2 env_import - System Environment Variable Import

#### Overview

Specifies the names of system environment variables for go-safe-cmd-runner to import for use in TOML variable expansion. Command-level `env_import` is merged with global and group-level `env_import` (Merge behavior).

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
env_import = ["VAR1", "VAR2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group, Command |
| **Default Value** | [] |
| **Format** | Variable names only (VALUE not needed) |
| **Security Constraint** | Only variables in `env_allowed` can be imported |
| **Inheritance Behavior** | Merge - lower levels are merged with upper levels |

#### Role

- **System Environment Variable Import**: Make Runner's environment variables available for use in TOML
- **Variable Expansion in TOML**: Imported variables can be referenced using `%{VAR}` format
- **Security Management**: Controlled through `env_allowed`

#### Configuration Examples

##### Example 1: Basic Import

```toml
[global]
env_allowed = ["HOME", "USER", "PATH"]

[[groups.commands]]
name = "show_user_info"
cmd = "/bin/echo"
env_import = ["USER", "HOME"]
args = ["User: %{USER}, Home: %{HOME}"]
```

##### Example 2: Merge Behavior

```toml
[global]
env_allowed = ["HOME", "USER", "PATH", "LANG"]
env_import = ["HOME", "USER"]  # Global level

[[groups]]
name = "intl_tasks"
env_import = ["LANG"]  # Group level: merges with global env_import

[[groups.commands]]
name = "task1"
cmd = "/bin/echo"
# env_import not specified → group's env_import is applied
# Inherited variables: HOME, USER (global) + LANG (group)
args = ["User: %{USER}, Language: %{LANG}"]

[[groups.commands]]
name = "task2"
cmd = "/bin/echo"
env_import = ["PATH"]  # Command level: merges with group
# Inherited variables: HOME, USER (global) + LANG (group) + PATH (command)
args = ["Path: %{PATH}, Home: %{HOME}"]
```

#### Important Notes

##### 1. Relationship with env_allowed

Variables to be imported with `env_import` must be included in `env_allowed`:

```toml
[global]
env_allowed = ["HOME", "USER"]

[[groups.commands]]
name = "example"
cmd = "/bin/echo"
env_import = ["HOME", "PATH"]  # Error: PATH not in env_allowed
args = ["%{HOME}"]
```

##### 2. Merge Behavior

When `env_import` is specified at command level, it is merged with global and group `env_import`:

```toml
[global]
env_import = ["HOME", "USER"]

[[groups]]
name = "tasks"
env_import = ["LANG", "LC_ALL"]

[[groups.commands]]
name = "task1"
cmd = "/bin/echo"
env_import = ["PWD"]  # HOME, USER, LANG, LC_ALL, PWD all available
args = ["User: %{USER}, PWD: %{PWD}"]
```

##### 3. Non-Existent Variables

If a variable specified in `env_import` does not exist in the system environment, it is treated as empty string:

```toml
[[groups.commands]]
name = "example"
cmd = "/bin/echo"
env_import = ["NONEXISTENT_VAR"]
args = ["Value: %{NONEXISTENT_VAR}"]  # Output: "Value: "
```

### 6.2.3 env - Process Environment Variables

#### Overview

Specifies environment variables to set for child process during command execution in `KEY=VALUE` format.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
env_vars = ["KEY1=value1", "KEY2=value2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Default Value** | [] |
| **Format** | "KEY=VALUE" |
| **Variable Expansion** | VALUE part supports %{VAR} format variable expansion |

#### Role

- **Process Environment Variables**: Set as environment variables for child processes
- **Command Configuration**: Control command behavior with environment variables
- **Credentials**: Settings like database connection information
- **Operation Mode**: Switch modes like debug mode

#### Configuration Examples

##### Example 1: Basic Environment Variables

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/app/server"
args = []
env_vars = [
    "LOG_LEVEL=debug",
    "PORT=8080",
    "CONFIG_FILE=/etc/app/config.yaml",
]
```

##### Example 2: Database Connection Information

```toml
[[groups.commands]]
name = "db_migration"
cmd = "/opt/app/migrate"
args = []
env_vars = [
    "DATABASE_URL=postgresql://localhost:5432/mydb",
    "DB_USER=appuser",
    "DB_PASSWORD=secret123",
]
```

##### Example 3: Using Variable Expansion

```toml
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup.sh"
vars = [
    "backup_dir=/var/backups",
    "date=2025-01-15",
]
env_vars = [
    "BACKUP_DIR=%{backup_dir}",
    "BACKUP_FILE=%{backup_dir}/backup-%{date}.tar.gz",
]
# BACKUP_FILE expands to /var/backups/backup-2025-01-15.tar.gz
```

#### Important Notes

##### 1. Relationship with env_allowed

Environment variables set must be included in the group or global `env_allowed`:

```toml
[global]
env_allowed = ["PATH", "LOG_LEVEL", "DATABASE_URL"]

[[groups]]
name = "app_group"

[[groups.commands]]
name = "run_app"
cmd = "/opt/app/server"
args = []
env_vars = [
    "LOG_LEVEL=debug",      # OK: in env_allowed
    "DATABASE_URL=...",     # OK: in env_allowed
    "UNAUTHORIZED_VAR=x",   # Error: not in env_allowed
]
```

##### 2. Format Rules

- `KEY=VALUE` format is required
- Error if `=` is not included
- Even if VALUE is empty, `KEY=` notation is required

```toml
# Correct
env_vars = [
    "KEY=value",
    "EMPTY_VAR=",  # Empty value
]

# Wrong
env_vars = [
    "KEY",         # Error: no =
    "KEY value",   # Error: no =
]
```

##### 3. No Duplicates

The same key cannot be defined multiple times:

```toml
# Wrong: LOG_LEVEL is duplicated
env_vars = [
    "LOG_LEVEL=debug",
    "LOG_LEVEL=info",  # Error: duplicate
]
```

### 6.2.4 workdir - Working Directory

#### Overview

Specifies a working directory specific to this command. When specified, it overrides the group or global working directory configuration.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
workdir = "directory_path"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Valid Values** | Absolute path |
| **Override** | Overrides group or global working directory configuration |
| **Auto-creation** | Directory is automatically created if it doesn't exist |

#### Available Variables

The following variables can be used in command-level `workdir`:

- `%{__runner_workdir}`: Temporary working directory automatically generated by the runner

#### Configuration Examples

#### Example 1: Specifying a Fixed Directory

```toml
[[groups]]
name = "report_tasks"

[[groups.commands]]
name = "generate_report"
cmd = "/opt/app/report"
args = ["--output", "daily_report.pdf"]
workdir = "/var/reports/daily"
# This command executes in /var/reports/daily
```

#### Example 2: Using `%{__runner_workdir}` Variable in Arguments

```toml
[[groups.commands]]
name = "temp_processing"
cmd = "/usr/bin/convert"
args = ["/data/input.jpg", "-resize", "800x600", "%{__runner_workdir}/output.jpg"]
# Output file is saved to the group's working directory (automatic temporary directory if not specified)
```

#### Example 3: Different Working Directories per Command

```toml
[[groups]]
name = "multi_dir_tasks"
workdir = "/var/app"  # Group default

[[groups.commands]]
name = "process_logs"
cmd = "/opt/tools/logparser"
args = ["access.log"]
workdir = "/var/log/apache"  # Executes in log directory

[[groups.commands]]
name = "process_data"
cmd = "/opt/tools/dataparser"
args = ["data.csv"]
# workdir not specified → uses group's /var/app
```

> **Note**: In past versions, a `dir` parameter was proposed but never implemented. In the current version, use the `workdir` parameter instead.

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
| **Type** | Integer (int) or null |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Command |
| **Default Value** | Global timeout (if set), otherwise 60 seconds |
| **Valid Values** | 0 (unlimited), positive integer (in seconds) |
| **Override** | Overrides global setting |

#### ⚠️ Breaking Change Notice (v2.0.0)

**BREAKING**: Starting from v2.0.0, `timeout = 0` means **unlimited execution** (no timeout).
- **Before v2.0.0**: `timeout = 0` was treated as invalid and defaulted to global timeout
- **From v2.0.0**: `timeout = 0` explicitly means unlimited execution time

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
output_file = "full_backup.sql"
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

#### Example 3: Unlimited Execution (v2.0.0+)

```toml
[global]
timeout = 120  # Default: 2 minutes

[[groups]]
name = "data_processing"

[[groups.commands]]
name = "interactive_data_entry"
cmd = "/usr/bin/data-entry-tool"
args = ["--interactive"]
timeout = 0  # ✅ Unlimited execution - waits for user interaction

[[groups.commands]]
name = "large_dataset_analysis"
cmd = "/usr/bin/analyze"
args = ["--dataset", "huge_dataset.csv"]
timeout = 0  # ✅ Unlimited execution - unpredictable processing time

[[groups.commands]]
name = "quick_validation"
cmd = "/usr/bin/validate"
args = ["small_file.txt"]
# Uses global 120 seconds
```

#### Inheritance Logic (v2.0.0+)

The effective timeout value follows this resolution hierarchy:

1. **Command-level `timeout`** (highest priority)
   ```toml
   [[groups.commands]]
   name = "cmd"
   timeout = 300  # ← This value is used
   ```

2. **Global-level `timeout`**
   ```toml
   [global]
   timeout = 180  # ← Used if command timeout is not specified
   ```

3. **System default** (lowest priority)
   ```
   60 seconds  # ← Used if neither global nor command timeout is specified
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

### 6.5.1 risk_level - Maximum Risk Level

#### Overview

Specifies the maximum risk level allowed for a command. If the command's risk exceeds the specified level, execution is rejected.

#### Syntax

```toml
[[groups.commands]]
name = "example"
cmd = "command"
args = []
risk_level = "risk_level"
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
risk_level = "low"  # Read-only, so low risk
```

#### Example 2: Medium-Risk Command

```toml
[[groups.commands]]
name = "create_backup"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
risk_level = "medium"  # File creation, so medium risk
```

#### Example 3: High-Risk Command

```toml
[[groups.commands]]
name = "install_package"
cmd = "/usr/bin/apt-get"
args = ["install", "-y", "package-name"]
run_as_user = "root"
risk_level = "high"  # System changes and privilege escalation, so high risk
```

#### Example 4: Behavior When Risk Level is Violated

```toml
[[groups.commands]]
name = "dangerous_operation"
cmd = "/bin/rm"
args = ["-rf", "/tmp/data"]
risk_level = "low"  # rm -rf is medium risk or higher
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
risk_level = "low"  # Read only

[[groups.commands]]
name = "backup_data"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
risk_level = "medium"  # File creation

[[groups.commands]]
name = "system_update"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
risk_level = "high"  # System changes and privilege escalation
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
output_file = "file_path"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Command only |
| **Valid Values** | Relative path or absolute path |
| **Size Limit** | Limited by global output_size_limit |
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
output_file = "users.csv"
# Saved to /var/app/output/users.csv
```

#### Example 2: Output with Absolute Path

```toml
[[groups.commands]]
name = "system_report"
cmd = "/usr/bin/systemctl"
args = ["status"]
output_file = "/var/log/reports/system_status.txt"
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
output_file = "logs/export/output.txt"
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
output_file = "disk_usage.txt"

[[groups.commands]]
name = "memory_info"
cmd = "/usr/bin/free"
args = ["-h"]
output_file = "memory_info.txt"

[[groups.commands]]
name = "process_list"
cmd = "/bin/ps"
args = ["aux"]
output_file = "processes.txt"
```

#### Important Notes

##### 1. Size Limit

Output size is limited by `output_size_limit` (global setting):

```toml
[global]
output_size_limit = 1048576  # 1MB

[[groups.commands]]
name = "large_export"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
output_file = "dump.sql"
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
output_file = "daily.txt"
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
env_allowed = ["PATH", "HOME", "DATABASE_URL", "BACKUP_DIR"]
output_size_limit = 10485760  # 10MB
verify_files = []  # Commands are automatically verified, so can be empty if no additional files

[[groups]]
name = "database_operations"
description = "Database-related operations"
priority = 10
workdir = "/var/backups/db"
env_allowed = ["PATH", "DATABASE_URL", "BACKUP_DIR"]
verify_files = ["/etc/postgresql/pg_hba.conf"]  # Only specify additional files like config files

# Command 1: Database backup
[[groups.commands]]
name = "full_backup"
description = "Backup of all PostgreSQL databases"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases", "--verbose"]
env_vars = ["DATABASE_URL=postgresql://localhost/postgres"]
output_file = "full_backup.sql"
timeout = 1800  # 30 minutes
risk_level = "medium"

# Command 2: Verify backup
[[groups.commands]]
name = "verify_backup"
description = "Verify backup file integrity"
cmd = "/usr/bin/psql"
args = ["--dry-run", "-f", "full_backup.sql"]
env_vars = ["DATABASE_URL=postgresql://localhost/testdb"]
output_file = "verification.log"
timeout = 600  # 10 minutes
risk_level = "low"

# Command 3: Delete old backups
[[groups.commands]]
name = "cleanup_old_backups"
description = "Delete backup files older than 30 days"
cmd = "/usr/bin/find"
args = ["/var/backups/db", "-name", "*.sql", "-mtime", "+30", "-delete"]
timeout = 300  # 5 minutes
risk_level = "medium"

[[groups]]
name = "system_maintenance"
description = "System maintenance tasks"
priority = 20
workdir = "/tmp"
env_allowed = []  # No environment variables

# Command 4: Disk usage report
[[groups.commands]]
name = "disk_report"
description = "Generate disk usage report"
cmd = "/bin/df"
args = ["-h", "/var"]
output_file = "/var/log/disk_usage.txt"
timeout = 60
risk_level = "low"

# Command 5: System update (root privileges)
[[groups.commands]]
name = "system_update"
description = "Update system packages"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
timeout = 600
risk_level = "high"

[[groups]]
name = "temporary_processing"
description = "Temporary directory processing tasks"
priority = 30

# Command 6: Image resizing in temporary directory
[[groups.commands]]
name = "image_resize"
description = "Resize image processing"
cmd = "/usr/bin/convert"
args = ["/data/input.jpg", "-resize", "800x600", "%{__runner_workdir}/resized.jpg"]
output_file = "conversion.log"
timeout = 300
risk_level = "low"

# Command 7: Work in fixed directory
[[groups.commands]]
name = "copy_result"
description = "Copy processing result to persistent directory"
cmd = "/usr/bin/cp"
args = ["%{__runner_workdir}/resized.jpg", "/var/output/final.jpg"]
workdir = "/var/output"
timeout = 60
risk_level = "low"
```

## Next Steps

The next chapter will provide detailed explanations of variable expansion functionality. You will learn how to perform dynamic command construction using `%{VAR}` format variables.
