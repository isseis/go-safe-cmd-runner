# Chapter 8: Variable Expansion

## 8.1 Overview of Variable Expansion

Variable expansion is a feature that allows you to embed variables in commands and their arguments, which are then replaced with actual values at runtime. In go-safe-cmd-runner, **internal variables** (for TOML expansion only) and **process environment variables** (environment variables passed to child processes) are clearly separated to improve security and clarity.

### Key Benefits

1. **Improved Security**: Internal variables and process environment variables are separated to prevent unintended information leakage
2. **Dynamic Command Construction**: Values can be determined at runtime
3. **Configuration Reuse**: The same variable can be used in multiple places
4. **Environment Switching**: Easy switching between development/production environments
5. **Improved Maintainability**: Changes can be centralized in one location

### Types of Variables

go-safe-cmd-runner handles 2 types of variables:

| Variable Type | Purpose | Reference Syntax | Definition Method | Impact on Child Process |
|---------------|---------|------------------|-------------------|------------------------|
| **Internal Variables** | For expansion only within TOML configuration files | `%{var}` | `vars`, `env_import` | None (default) |
| **Process Environment Variables** | Set as environment variables for child processes | - | `env_vars` | Yes |

### Locations Where Variables Can Be Used

Variable expansion can be used in the following locations:

- **cmd**: Path to the command to execute (use `%{var}`)
- **args**: Command arguments (use `%{var}`)
- **env_vars**: Process environment variable values (use `%{var}` if needed)
- **verify_files**: Paths of files to verify (use `%{var}`)
- **vars**: Internal variable definitions (can reference other internal variables with `%{var}`)

## 8.2 Variable Expansion Syntax

### Internal Variable Reference Syntax

Internal variables are written in the format `%{variable_name}`:

```toml
cmd = "%{variable_name}"
args = ["%{arg1}", "%{arg2}"]
env_vars = ["VAR=%{value}"]
```

### Variable Naming Rules

- Letters (uppercase/lowercase), numbers, and underscores (`_`) are allowed
- Recommended to use lowercase and underscores (e.g., `my_variable`, `app_dir`)
- Must start with a letter or underscore
- Case-sensitive (`home` and `HOME` are different variables)
- Reserved prefix `__runner_` cannot be used to start variable names

```
# Valid variable names
"%{path}"
"%{my_tool}"
"%{_private_var}"
"%{var123}"
"%{HOME}"

# Invalid variable names
"%{123var}"         # Starts with a number
"%{my-var}"         # Hyphens not allowed
"%{my.var}"         # Dots not allowed
"%{__runner_test}"  # Reserved prefix
```

## 8.3 Internal Variable Definition

### 8.3.1 Defining Internal Variables Using the `vars` Field

#### Overview

Using the `vars` field, you can define internal variables for TOML expansion only. These variables do not affect the environment variables of child processes.

#### Configuration Format

```toml
[global.vars]
app_dir = "/opt/myapp"

[[groups]]
name = "backup"

[groups.vars]
backup_dir = "%{app_dir}/backups"
retention_days = "30"

[[groups.commands]]
name = "backup_db"
cmd = "/usr/bin/pg_dump"
args = ["-f", "%{output_file}", "mydb"]

[groups.commands.vars]
timestamp = "20250114"
output_file = "%{backup_dir}/dump_%{timestamp}.sql"
```

#### Scope and Inheritance

| Level | Scope | Inheritance Rule |
|-------|-------|------------------|
| **Global.vars** | Accessible from all groups and commands | - |
| **Group.vars** | Accessible from commands in that group | Merge with Global.vars (Group takes priority) |
| **Command.vars** | Accessible only within that command | Merge Global + Group + Command |

#### Reference Syntax

- Reference in the format `%{variable_name}`
- Can be used in the values of `cmd`, `args`, `verify_files`, `env_vars`, and in other `vars` definitions

#### Basic Example

```toml
version = "1.0"

[global.vars]
base_dir = "/opt"

[[groups]]
name = "prod_backup"

[groups.vars]
db_tools = "%{base_dir}/db-tools"

[[groups.commands]]
name = "db_dump"
cmd = "%{db_tools}/dump.sh"
args = ["-o", "%{output_file}"]

[groups.commands.vars]
timestamp = "20250114"
output_file = "%{base_dir}/dump_%{timestamp}.sql"
```

### 8.3.2 Importing System Environment Variables Using `env_import`

#### Overview

Using the `env_import` field, you can import system environment variables as internal variables.

#### Configuration Format

```toml
[global]
env_allowed = ["HOME", "PATH", "USER"]
env_import = [
    "home=HOME",
    "user_path=PATH",
    "username=USER"
]

[[groups]]
name = "example"
env_import = [
    "custom=CUSTOM_VAR"  # Import specific to this group
]
```

#### Syntax

Written in the format `internal_variable_name=system_environment_variable_name`:

- **Left side**: Internal variable name (recommended lowercase, e.g., `home`, `user_path`)
- **Right side**: System environment variable name (typically uppercase, e.g., `HOME`, `PATH`)

#### Security Constraints

- System environment variables referenced in `env_import` must be included in `env_allowed`
- An error will occur if you reference a variable not in `env_allowed`

#### Inheritance Rules

| Level | Inheritance Behavior |
|-------|----------------------|
| **Global.env_import** | Inherited by all groups and commands (default) |
| **Group.env_import** | If defined, **merges** (Merge) with Global.env_import |
| **Command.env_import** | If defined, **merges** (Merge) with Global + Group env_import |
| **Undefined** | Inherits env_import from upper levels |

#### Example: Importing System Environment Variables

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "PATH"]
env_import = [
    "home=HOME",
    "user_path=PATH"
]

[[groups]]
name = "file_operations"

[[groups.commands]]
name = "list_home"
cmd = "/bin/ls"
args = ["-la", "%{home}"]
# %{home} expands to /home/username, etc.
```

### 8.3.3 Nesting Internal Variables

Internal variable values can contain references to other internal variables.

#### Basic Example

```toml
[global.vars]
base = "/opt"
app_dir = "%{base}/myapp"
log_dir = "%{app_dir}/logs"

[[groups.commands]]
name = "show_log_dir"
cmd = "/bin/echo"
args = ["Log directory: %{log_dir}"]
# Actual: Log directory: /opt/myapp/logs
```

#### Expansion Order

Variables are expanded in the order of definition:

1. `base` → `/opt`
2. `app_dir` → `%{base}/myapp` → `/opt/myapp`
3. `log_dir` → `%{app_dir}/logs` → `/opt/myapp/logs`

### 8.3.4 Circular Reference Detection

Circular references are detected as errors:

```toml
[[groups.commands]]
name = "circular"
cmd = "/bin/echo"
args = ["%{var1}"]

[groups.commands.vars]
var1 = "%{var2}"
var2 = "%{var1}"  # Error: circular reference
```

## 8.4 Defining Process Environment Variables

### 8.4.1 Setting Environment Variables Using the `env_vars` Field

#### Overview

Environment variables defined in the `env_vars` field are passed to child processes when commands are executed. Internal variables (`%{var}`) can be used in these values.

#### Configuration Format

```toml
[global]
env_vars = [
    "LOG_LEVEL=info",
    "APP_ENV=production"
]

[[groups]]
name = "app_tasks"
env_vars = [
    "DB_HOST=localhost",
    "DB_PORT=5432"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
env_vars = [
    "CONFIG_FILE=%{config_path}"  # Internal variables can be used
]

[groups.commands.vars]
config_path = "/etc/myapp/config.yml"
```

#### Inheritance and Merging

The `env_vars` field is merged as follows:

1. Global.env_vars
2. Group.env_vars (combined with Global)
3. Command.env_vars (combined with Global + Group)

When the same environment variable name is defined at multiple levels, the more specific level (Command > Group > Global) takes priority.

#### Relationship with Internal Variables

- Internal variables can be referenced in `env_vars` values using the `%{var}` format
- By default, environment variables defined in `env_vars` are passed only to child processes and cannot be used as internal variables
- If you want to use them as internal variables, define them in the `vars` field

#### Example: Setting Process Environment Variables Using Internal Variables

```toml
version = "1.0"

[global.vars]
app_dir = "/opt/myapp"
log_dir = "%{app_dir}/logs"

[global]
env_vars = [
    "APP_HOME=%{app_dir}",
    "LOG_PATH=%{log_dir}/app.log"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--verbose"]
# Child process receives APP_HOME=/opt/myapp, LOG_PATH=/opt/myapp/logs/app.log
```

## 8.5 Detailed Locations Where Variables Can Be Used

### 8.5.1 Variable Expansion in cmd

Internal variables can be used in command paths.

#### Example 1: Basic Command Path Expansion

```toml
[[groups.commands]]
name = "docker_version"
cmd = "%{docker_cmd}"
args = ["version"]

[groups.commands.vars]
docker_cmd = "/usr/bin/docker"
```

At runtime:
- `%{docker_cmd}` → expands to `/usr/bin/docker`
- Actual execution: `/usr/bin/docker version`

#### Example 2: Version-Managed Tools

```toml
[[groups.commands]]
name = "gcc_compile"
cmd = "%{toolchain_dir}/gcc-%{version}/bin/gcc"
args = ["-o", "output", "main.c"]

[groups.commands.vars]
toolchain_dir = "/opt/toolchains"
version = "11.2.0"
```

At runtime:
- `%{toolchain_dir}` → expands to `/opt/toolchains`
- `%{version}` → expands to `11.2.0`
- Actual execution: `/opt/toolchains/gcc-11.2.0/bin/gcc -o output main.c`

### 8.5.2 Variable Expansion in args

Internal variables can be used in command arguments.

#### Example 1: File Path Construction

```toml
[[groups.commands]]
name = "backup_copy"
cmd = "/bin/cp"
args = ["%{source_file}", "%{dest_file}"]

[groups.commands.vars]
source_file = "/data/original.txt"
dest_file = "/backups/backup.txt"
```

#### Example 2: Multiple Variables in One Argument

```toml
[[groups.commands]]
name = "ssh_connect"
cmd = "/usr/bin/ssh"
args = ["%{user}@%{host}:%{port}"]

[groups.commands.vars]
user = "admin"
host = "server01.example.com"
port = "22"
```

At runtime:
- `%{user}@%{host}:%{port}` → expands to `admin@server01.example.com:22`

#### Example 3: Configuration File Switching

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_dir}/%{env_type}.yml"]

[groups.commands.vars]
config_dir = "/etc/myapp/configs"
env_type = "production"
```

At runtime:
- `%{config_dir}/%{env_type}.yml` → expands to `/etc/myapp/configs/production.yml`

### 8.5.3 Combining Multiple Variables

Multiple variables can be combined to construct complex paths and strings.

#### Example 1: Backup Path with Timestamp

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/bin/mkdir"
args = ["-p", "%{backup_root}/%{date}/%{user}/data"]

[groups.commands.vars]
backup_root = "/var/backups"
date = "2025-10-02"
user = "admin"
```

At runtime:
- `%{backup_root}/%{date}/%{user}/data` → expands to `/var/backups/2025-10-02/admin/data`

#### Example 2: Database Connection String

```toml
[[groups.commands]]
name = "db_connect"
cmd = "/usr/bin/psql"
args = ["postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"]

[groups.commands.vars]
db_user = "appuser"
db_pass = "secret123"
db_host = "localhost"
db_port = "5432"
db_name = "myapp_db"
```

At runtime:
- Connection string is fully expanded
- `postgresql://appuser:secret123@localhost:5432/myapp_db`

## 8.6 Practical Examples

### 8.6.1 Dynamic Command Path Construction

Example of switching command paths based on environment:

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "PYTHON_ROOT", "PY_VERSION"]

[[groups]]
name = "python_tasks"

# Using Python 3.10
[[groups.commands]]
name = "run_with_py310"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]

[groups.commands.vars]
python_root = "/usr/local"
py_version = "3.10"

# Using Python 3.11
[[groups.commands]]
name = "run_with_py311"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]

[groups.commands.vars]
python_root = "/usr/local"
py_version = "3.11"
```

### 8.6.2 Dynamic Argument Generation

Dynamically constructing Docker container startup parameters:

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "DOCKER_BIN"]

[[groups]]
name = "docker_deployment"

[[groups.commands]]
name = "start_container"
cmd = "%{docker_bin}"
args = [
    "run",
    "-d",
    "--name", "%{container_name}",
    "-v", "%{host_path}:%{container_path}",
    "-e", "APP_ENV=%{app_env}",
    "-p", "%{host_port}:%{container_port}",
    "%{image_name}:%{image_tag}",
]

[groups.commands.vars]
docker_bin = "/usr/bin/docker"
container_name = "myapp-prod"
host_path = "/opt/myapp/data"
container_path = "/app/data"
app_env = "production"
host_port = "8080"
container_port = "80"
image_name = "myapp"
image_tag = "v1.2.3"
```

Executed command:
```bash
/usr/bin/docker run -d \
  --name myapp-prod \
  -v /opt/myapp/data:/app/data \
  -e APP_ENV=production \
  -p 8080:80 \
  myapp:v1.2.3
```

### 8.6.3 Environment-Specific Configuration Switching

Using different configurations for development and production environments:

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "APP_BIN", "CONFIG_DIR", "ENV_TYPE", "LOG_LEVEL", "DB_URL"]

# Development environment group
[[groups]]
name = "development"

[[groups.commands]]
name = "run_dev"
cmd = "%{app_bin}"
args = [
    "--config", "%{config_dir}/%{env_type}.yml",
    "--log-level", "%{log_level}",
    "--db", "%{db_url}",
]

[groups.commands.vars]
app_bin = "/opt/myapp/bin/myapp"
config_dir = "/etc/myapp/configs"
env_type = "development"
db_url = "postgresql://localhost/dev_db"

# Production environment group
[[groups]]
name = "production"

[[groups.commands]]
name = "run_prod"
cmd = "%{app_bin}"
args = [
    "--config", "%{config_dir}/%{env_type}.yml",
    "--log-level", "%{log_level}",
    "--db", "%{db_url}",
]

[groups.commands.vars]
app_bin = "/opt/myapp/bin/myapp"
config_dir = "/etc/myapp/configs"
env_type = "production"
db_url = "postgresql://prod-server/prod_db"
```

## 8.8 Nested Variables

Variable values can contain other variables.

### Basic Example

```toml
[[groups.commands]]
name = "nested_vars"
cmd = "/bin/echo"
args = ["Message: %{full_msg}"]

[groups.commands.vars]
full_msg = "Hello, %{user}!"
user = "Alice"
```

Expansion order:
1. `%{user}` → expands to `Alice`
2. `%{full_msg}` → expands to `Hello, Alice!`
3. Final argument: `Message: Hello, Alice!`

### Complex Path Construction

```toml
[[groups.commands]]
name = "complex_path"
cmd = "/bin/echo"
args = ["Config path: %{config_path}"]

[groups.commands.vars]
config_path = "%{base_dir}/%{env_type}/config.yml"
base_dir = "/opt/myapp"
env_type = "production"
```

Expansion order:
1. `%{base_dir}` → expands to `/opt/myapp`
2. `%{env_type}` → expands to `production`
3. `%{config_path}` → expands to `/opt/myapp/production/config.yml`

## 8.9 Variable Self-Reference

Variable self-reference is an important feature commonly used when extending environment variables. It is particularly useful for environment variables like `PATH`, where you want to add new values to existing ones.

### How Self-Reference Works

In expressions like `PATH=/custom/bin:%{path}`, the `%{path}` refers to a **system environment variable imported via `env_import`** or it can reference an internal variable. This is not a circular reference but an intentionally supported feature.

### Basic Example: PATH Extension

```toml
[global]
env_allowed = ["PATH"]
env_import = ["path=PATH"]

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["PATH is: %{path}"]
env_vars = ["PATH=/opt/mytools/bin:%{path}"]
```

Expansion process:
1. Import system environment variable `PATH` as `%{path}` (e.g., `/usr/bin:/bin`)
2. `%{path}` → expands to `/usr/bin:/bin`
3. Final value: `/opt/mytools/bin:/usr/bin:/bin`

### Practical Example: Adding Custom Tool Directory

```toml
[global]
env_allowed = ["PATH"]
env_import = ["path=PATH"]

[[groups.commands]]
name = "use_custom_tools"
cmd = "%{custom_tool}"
args = ["--version"]
env_vars = [
    "PATH=%{tool_dir}/bin:%{path}"
]

[groups.commands.vars]
tool_dir = "/opt/custom-tools"
custom_tool = "%{tool_dir}/bin/mytool"
```

With this configuration:
- `%{custom_tool}` can be found from the extended `PATH` even when specified as just a command name (not a full path)
- The existing system `PATH` is preserved

### Self-Reference with Other Environment Variables

`PATH` is not the only environment variable that supports self-reference:

```toml
[global]
env_allowed = ["LD_LIBRARY_PATH", "PYTHONPATH"]
env_import = [
    "ld_library_path=LD_LIBRARY_PATH",
    "pythonpath=PYTHONPATH"
]

[[groups.commands]]
name = "extend_lib_path"
cmd = "/opt/myapp/bin/app"
args = []
env_vars = [
    "LD_LIBRARY_PATH=/opt/myapp/lib:%{ld_library_path}",
    "PYTHONPATH=/opt/myapp/python:%{pythonpath}"
]
```

### Difference Between Self-Reference and Circular Reference

**Self-Reference (Normal)**: Referencing a system environment variable imported via `env_import` or an internal variable
```toml
env_vars = ["PATH=/custom/bin:%{path}"]  # %{path} refers to system environment variable
```

**Circular Reference (Error)**: Variables within vars reference each other circularly
```toml
vars = [
    "var1=%{var2}",
    "var2=%{var1}",  # Error: Circular reference
]
```

### Important Notes

1. **When system environment variable doesn't exist**: If the system environment variable referenced in `env_import` doesn't exist, an error will occur
2. **Relationship with allowlist**: When referencing system environment variables via `env_import`, those variables must be included in `env_allowed`

```toml
[global]
env_allowed = ["PATH", "HOME"]  # Allow PATH and HOME to be imported

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["%{path}"]
vars = ["path=PATH_PREFIX:/custom:%{system_path}"]
env_import = ["system_path=PATH"]  # OK: PATH is included in allowlist
env_vars = ["PATH=%{path}"]
```

## 8.10 Escape Sequences

When you want to use literal `%` or `\` characters, escaping is required.

### Escaping Percent Signs

Use `\%` to represent a literal percent sign:

```toml
[[groups.commands]]
name = "percentage_display"
cmd = "/bin/echo"
args = ["Progress: 100\\% complete"]
```

Output: `Progress: 100% complete`

### Escaping Backslashes

Use `\\` to represent a literal backslash:

```toml
[[groups.commands]]
name = "windows_path"
cmd = "/bin/echo"
args = ["Path: C:\\\\Users\\\\%{user}"]

[groups.commands.vars]
user = "JohnDoe"
```

Output: `Path: C:\Users\JohnDoe`

### Mixed Example

```toml
[[groups.commands]]
name = "mixed_escape"
cmd = "/bin/echo"
args = ["Literal \\%{HOME} is different from %{home}"]

[groups.commands.vars]
home = "/home/user"
```

Output: `Literal $HOME is different from /home/user`

## 8.11 Automatic Variables

### 8.11.1 Overview

The system automatically sets the following internal variables:

- **`__runner_datetime`**: Runner start time (UTC) in YYYYMMDDHHmmSS.msec format (global variable)
- **`__runner_pid`**: Process ID of the runner process (global variable)
- **`__runner_workdir`**: Working directory for the group (set during group execution, available at command level only)

These variables can be used in command paths, arguments, and environment variable values just like regular internal variables.

### 8.11.2 Usage Examples

#### Timestamped Backups

```toml
[[groups.commands]]
name = "backup_with_timestamp"
description = "Create backup with timestamp"
cmd = "/usr/bin/tar"
args = [
    "czf",
    "/tmp/backup/data-%{__runner_datetime}.tar.gz",
    "/data"
]
```

Example execution:
- If execution time is 2025-10-05 14:30:22.123 UTC
- Backup filename: `/tmp/backup/data-20251005143022.123.tar.gz`

#### PID-based Lock Files

```toml
[[groups.commands]]
name = "create_lock_file"
description = "Create lock file with PID"
cmd = "/bin/sh"
args = [
    "-c",
    "echo %{__runner_pid} > /var/run/myapp-%{__runner_pid}.lock"
]
```

Example execution:
- If PID is 12345
- Lock file: `/var/run/myapp-12345.lock` (content: 12345)

#### Execution Logging

```toml
[[groups.commands]]
name = "log_execution"
description = "Log execution time and PID"
cmd = "/bin/sh"
args = [
    "-c",
    "echo 'Executed at %{__runner_datetime} by PID %{__runner_pid}' >> /var/log/executions.log"
]
```

Example output:
```
Executed at 20251005143022.123 by PID 12345
```

#### Working Directory Reference

```toml
[[groups]]
name = "backup_group"

[[groups.commands]]
name = "create_backup"
description = "Create backup file in working directory"
cmd = "/usr/bin/tar"
args = ["czf", "%{__runner_workdir}/backup.tar.gz", "/data"]
```

Example execution:
- If the group's working directory is `/tmp/scr-backup_group-XXXXXX`
- Backup file: `/tmp/scr-backup_group-XXXXXX/backup.tar.gz`

#### Combining Multiple Automatic Variables

```toml
[[groups.commands]]
name = "timestamped_report"
description = "Report with timestamp and PID"
cmd = "/opt/myapp/bin/report"
args = [
    "--output", "/reports/%{__runner_datetime}-%{__runner_pid}.html",
    "--title", "Report %{__runner_datetime}",
]
```

Example execution:
- Output file: `/reports/20251005143022.123-12345.html`
- Report title: `Report 20251005143022.123`

### 8.11.3 DateTime Format

Format specification for `__runner_datetime`:

| Part | Description | Example |
|------|-------------|---------|
| YYYY | 4-digit year | 2025 |
| MM | 2-digit month (01-12) | 10 |
| DD | 2-digit day (01-31) | 05 |
| HH | 2-digit hour (00-23, UTC) | 14 |
| mm | 2-digit minute (00-59) | 30 |
| SS | 2-digit second (00-59) | 45 |
| .msec | 3-digit milliseconds (000-999) | .123 |

Complete example: `20251005143045.123` = October 5, 2025 14:30:45.123 (UTC)

**Note**: The timezone is always UTC, not local timezone.

### 8.11.4 Reserved Prefix

The prefix `__runner_` is reserved for automatic variables and cannot be used for user-defined variables.

#### Error Example

```toml
[[groups.commands]]
name = "invalid_var"
cmd = "/bin/echo"
args = ["%{__runner_custom}"]

[groups.commands.vars]
__runner_custom = "value"  # Error: Using reserved prefix
```

Error message:
```
variable "__runner_custom" uses reserved prefix "__runner_";
this prefix is reserved for automatically generated variables
```

#### Correct Example

```toml
[[groups.commands]]
name = "valid_var"
cmd = "/bin/echo"
args = ["%{my_custom_var}"]

[groups.commands.vars]
my_custom_var = "value"  # OK: Not using reserved prefix
```

### 8.11.5 Timing of Variable Generation

Automatic variables (`__runner_datetime` and `__runner_pid`) are generated once when the configuration file is loaded, not at each command execution time. All commands in all groups share the exact same values throughout the entire runner execution.

```toml
[[groups]]
name = "backup_group"

[[groups.commands]]
name = "backup_db"
cmd = "/usr/bin/pg_dump"
args = ["-f", "/tmp/backup/db-%{__runner_datetime}.sql", "mydb"]

[[groups.commands]]
name = "backup_files"
cmd = "/usr/bin/tar"
args = ["czf", "/tmp/backup/files-%{__runner_datetime}.tar.gz", "/data"]
```

**Key Point**: Both commands use the exact same timestamp because `__runner_datetime` is sampled at config load time, not at execution time:
- `/tmp/backup/db-20251005143022.123.sql`
- `/tmp/backup/files-20251005143022.123.tar.gz`

This ensures consistency across all commands in a single runner execution, even if commands are executed at different times or in different groups.

## 8.12 Security Considerations

### 8.9.1 Command.Env Priority

Variables defined in `Command.Env` take priority over system environment variables:

```toml
[global]
env_allowed = ["PATH", "HOME"]

[[groups.commands]]
name = "override_home"
cmd = "/bin/echo"
args = ["Home: ${HOME}"]
env_vars = ["HOME=/opt/custom-home"]
# The HOME from Command.Env is used, not the system $HOME
```

### 8.9.2 Relationship with env_allowed

**Important**: Variables defined in `Command.Env` are not subject to `env_allowed` checks.

```toml
[global]
env_allowed = ["PATH", "HOME"]
# CUSTOM_VAR is not in allowlist

[[groups.commands]]
name = "custom_var"
cmd = "${CUSTOM_TOOL}"
args = []
env_vars = ["CUSTOM_TOOL=/opt/tools/mytool"]
# CUSTOM_TOOL is not in allowlist, but can be used because it's defined in Command.Env
```

### 8.9.3 Command Path Requirements

Command paths after expansion must meet the following requirements:

#### Regular Commands

For regular commands (without `run_as_user` or `run_as_group`), both local paths (relative paths) and absolute paths are allowed:

```toml
# Correct: expands to absolute path
[[groups.commands]]
name = "valid_absolute"
cmd = "${TOOL_DIR}/mytool"
env_vars = ["TOOL_DIR=/opt/tools"]  # Absolute path

# Correct: expands to relative path (allowed for regular commands)
[[groups.commands]]
name = "valid_relative"
cmd = "${TOOL_DIR}/mytool"
env_vars = ["TOOL_DIR=./tools"]  # Relative path - OK for regular commands
```

#### Privileged Commands

For privileged commands (with `run_as_user` or `run_as_group`), **only absolute paths** are allowed for security reasons:

```toml
# Correct: expands to absolute path
[[groups.commands]]
name = "valid_privileged"
cmd = "${TOOL_DIR}/mytool"
run_as_user = "appuser"
env_vars = ["TOOL_DIR=/opt/tools"]  # Absolute path

# Incorrect: expands to relative path (error for privileged commands)
[[groups.commands]]
name = "invalid_privileged"
cmd = "${TOOL_DIR}/mytool"
run_as_user = "appuser"
env_vars = ["TOOL_DIR=./tools"]  # Relative path - error for privileged commands
```

Why absolute paths are required for privileged commands:
- Prevents PATH-based attacks
- Explicitly specifies the exact location of the command
- Reduces the risk of executing unintended commands

### 8.9.4 Handling Sensitive Information

Define sensitive information (API keys, passwords, etc.) in `Command.Env` to isolate from system environment variables:

```toml
[[groups.commands]]
name = "api_call"
cmd = "/usr/bin/curl"
args = [
    "-H", "Authorization: Bearer ${API_TOKEN}",
    "${API_ENDPOINT}/data",
]
# Sensitive information is defined in Command.Env and isolated from system environment
env_vars = [
    "API_TOKEN=sk-1234567890abcdef",
    "API_ENDPOINT=https://api.example.com",
]
```

### 8.12.5 Isolation Between Commands

Each command's `env_vars` is independent and does not affect other commands:

```toml
[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["DB: %{db_host}"]
env_vars = ["DB_HOST=%{db_host}"]

[groups.commands.vars]
db_host = "db1.example.com"

[[groups.commands]]
name = "cmd2"
cmd = "/bin/echo"
args = ["DB: %{db_host}"]
env_vars = ["DB_HOST=%{db_host}"]
# Independent from cmd1's DB_HOST

[groups.commands.vars]
db_host = "db2.example.com"
```

## 8.13 Troubleshooting

### Undefined Variables

If a variable is not defined, an error occurs:

```toml
[[groups.commands]]
name = "undefined_var"
cmd = "/bin/echo"
args = ["Value: ${UNDEFINED}"]
# UNDEFINED is not defined in env_vars → error
```

**Solution**: Define all required variables in `env_vars`

### Circular References

If variables reference each other, an error occurs:

```toml
[[groups.commands]]
name = "circular"
cmd = "/bin/echo"
args = ["${VAR1}"]
env_vars = [
    "VAR1=${VAR2}",
    "VAR2=${VAR1}",  # Circular reference → error
]
```

**Solution**: Organize variable dependencies

**Note**: Self-references like `PATH=/custom:${PATH}` are not circular references. See "7.6 Variable Self-Reference" for details.

### Path Validation Errors After Expansion

If the path after expansion is invalid, an error occurs:

```toml
[[groups.commands]]
name = "invalid_path"
cmd = "${TOOL}"
args = []
env_vars = ["TOOL=../tool"]  # Relative path → error
```

**Solution**: Use absolute paths

## Comprehensive Practical Example

The following is a practical configuration example utilizing variable expansion:

```toml
version = "1.0"

[global]
timeout = 300
env_allowed = ["PATH", "HOME", "USER"]
env_import = [
    "home=HOME",
    "username=USER"
]

[global.vars]
app_root = "/opt/myapp"
config_dir = "%{app_root}/config"
bin_dir = "%{app_root}/bin"

[[groups]]
name = "application_deployment"
description = "Application deployment process"

[groups.vars]
env_type = "production"
config_source = "%{config_dir}/templates"
migration_dir = "%{app_root}/migrations"

# Step 1: Deploy configuration file
[[groups.commands]]
name = "deploy_config"
description = "Deploy environment-specific configuration file"
cmd = "/bin/cp"
args = [
    "%{config_source}/%{env_type}/app.yml",
    "%{config_dir}/app.yml"
]

# Step 2: Database migration
[[groups.commands]]
name = "db_migration"
description = "Database schema migration"
cmd = "%{bin_dir}/migrate"
args = [
    "--database", "%{db_url}",
    "--migrations", "%{migration_dir}"
]
timeout = 600

[groups.commands.vars]
db_user = "appuser"
db_pass = "secret123"
db_host = "localhost"
db_port = "5432"
db_name = "myapp_prod"
db_url = "postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"

# Step 3: Start application
[[groups.commands]]
name = "start_application"
description = "Start application server"
cmd = "%{bin_dir}/server"
args = [
    "--config", "%{config_dir}/app.yml",
    "--port", "%{app_port}",
    "--workers", "%{worker_count}"
]
env_vars = [
    "LOG_LEVEL=info",
    "LOG_PATH=%{app_root}/logs/app.log"
]

[groups.commands.vars]
app_port = "8080"
worker_count = "4"

# Step 4: Health check
[[groups.commands]]
name = "health_check"
description = "Application health check"
cmd = "/usr/bin/curl"
args = ["-f", "%{health_url}"]
timeout = 30

[groups.commands.vars]
health_url = "http://localhost:%{app_port}/health"
```

## 8.14 Variable Expansion in verify_files

### 8.11.1 Overview

The `verify_files` field also supports environment variable expansion. This allows you to dynamically construct file verification paths and provides flexible verification configuration depending on the environment.

### 8.11.2 Target Fields

Variable expansion can be used in the following `verify_files` fields:

- **Global level**: `verify_files` in the `[global]` section
- **Group level**: `verify_files` in the `[[groups]]` section

### 8.11.3 Basic Examples

#### Global Level Expansion

```toml
version = "1.0"

[global]
env_allowed = ["HOME"]
env_import = ["home=HOME"]
verify_files = [
    "%{home}/config.toml",
    "%{home}/data.txt"
]

[[groups]]
name = "example"

[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["hello"]
```

Expansion result (when `HOME=/home/user`):
- `%{home}/config.toml` → `/home/user/config.toml`
- `%{home}/data.txt` → `/home/user/data.txt`

#### Group Level Expansion

```toml
version = "1.0"

[global]
env_allowed = ["APP_ROOT"]
env_import = ["app_root=APP_ROOT"]

[[groups]]
name = "app_group"
verify_files = [
    "%{app_root}/config/app.yml",
    "%{app_root}/bin/server"
]

[[groups.commands]]
name = "start"
cmd = "/bin/echo"
args = ["Starting app"]
```

Expansion result (when `APP_ROOT=/opt/myapp`):
- `%{app_root}/config/app.yml` → `/opt/myapp/config/app.yml`
- `%{app_root}/bin/server` → `/opt/myapp/bin/server`

### 8.11.4 Complex Example

Example with dynamic path construction:

```toml
version = "1.0"

[global]
env_allowed = ["ENV", "APP_ROOT"]
env_import = [
    "env_type=ENV",
    "app_root=APP_ROOT"
]
verify_files = [
    "%{config_path}/global.yml",
    "%{config_path}/secrets.enc",
    "%{app_root}/web/nginx.conf",
    "%{app_root}/web/ssl/cert.pem",
    "%{app_root}/web/ssl/key.pem",
    "%{app_root}/db/schema.sql",
    "%{app_root}/db/migrations/%{env_type}/"
]

[global.vars]
config_base = "%{app_root}/configs"
config_path = "%{config_base}/%{env_type}"

[[groups]]
name = "deployment"

[[groups.commands]]
name = "deploy"
cmd = "/opt/deploy.sh"
```

When the following environment variables are set:
- `ENV=production`
- `APP_ROOT=/opt/myapp`

The following files will be verified:
- `/opt/myapp/configs/production/global.yml`
- `/opt/myapp/configs/production/secrets.enc`
- `/opt/myapp/web/nginx.conf`
- `/opt/myapp/web/ssl/cert.pem`
- `/opt/myapp/web/ssl/key.pem`
- `/opt/myapp/db/schema.sql`
- `/opt/myapp/db/migrations/production/`

### 8.11.5 Limitations

1. **Absolute Path Requirement**: Expanded paths must be absolute paths
2. **System Environment Variables Only**: verify_files can only use system environment variables, not Command.Env variables
3. **Expansion Timing**: Expansion happens once at configuration load time (not at execution time)

## 8.12 Practical Comprehensive Example

Below is a practical configuration example using variable expansion features:

```toml
version = "1.0"

[global]
timeout = 300
env_allowed = ["PATH", "HOME", "USER"]
env_import = [
    "home=HOME",
    "username=USER"
]

[global.vars]
app_root = "/opt/myapp"
config_dir = "%{app_root}/config"
bin_dir = "%{app_root}/bin"

[[groups]]
name = "application_deployment"
description = "Application deployment process"

[groups.vars]
env_type = "production"
log_dir = "%{app_root}/logs"

# Step 1: Deploy configuration file
[[groups.commands]]
name = "deploy_config"
description = "Deploy environment-specific configuration"
cmd = "/bin/cp"
args = [
    "%{config_dir}/templates/%{env_type}/app.yml",
    "%{config_dir}/app.yml"
]

# Step 2: Database migration
[[groups.commands]]
name = "db_migration"
description = "Database schema migration"
cmd = "%{bin_dir}/migrate"
args = [
    "--database", "%{db_url}",
    "--migrations", "%{migration_dir}"
]
timeout = 600

[groups.commands.vars]
db_user = "appuser"
db_pass = "secret123"
db_host = "localhost"
db_port = "5432"
db_name = "myapp_prod"
db_url = "postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"
migration_dir = "%{app_root}/migrations"

# Step 3: Start application
[[groups.commands]]
name = "start_application"
description = "Start application server"
cmd = "%{bin_dir}/server"
args = [
    "--config", "%{config_dir}/app.yml",
    "--port", "%{app_port}",
    "--workers", "%{worker_count}"
]
env_vars = [
    "LOG_LEVEL=info",
    "LOG_PATH=%{log_dir}/app.log"
]

[groups.commands.vars]
app_port = "8080"
worker_count = "4"

# Step 4: Health check
[[groups.commands]]
name = "health_check"
description = "Application health check"
cmd = "/usr/bin/curl"
args = ["-f", "%{health_url}"]
timeout = 30

[groups.commands.vars]
health_url = "http://localhost:%{app_port}/health"
```

## 8.13 Summary

### Overall View of Variable System

The go-safe-cmd-runner variable system consists of three main components:

1. **Internal Variables** (`vars`, `env_import`)
   - Used exclusively for TOML expansion
   - Referenced using `%{var}` syntax
   - Not passed to child processes (by default)

2. **Process Environment Variables** (`env_vars`)
   - Environment variables passed to child processes at execution time
   - Can use internal variables `%{var}` in values

3. **Automatic Variables** (`__runner_datetime`, `__runner_pid`)
   - Automatically generated by the system
   - Available as internal variables

### Best Practices

1. **Utilize internal variables**: Define values that are only needed for TOML expansion (like paths and URLs) using `vars`
2. **Explicitly import with env_import**: Import system environment variables explicitly using `env_import` to make intentions clear
3. **Minimize env_vars usage**: Keep environment variables passed to child processes to the minimum necessary
4. **Consider security**: Handle sensitive information carefully and avoid passing unnecessary environment variables
5. **Standardize naming conventions**: Use lowercase with underscores for internal variables (e.g., `app_dir`), and uppercase for environment variables

### Next Steps

In the next chapter, we'll cover practical examples combining all the variable expansion features you've learned. You'll learn how to create configuration files based on real-world use cases.
