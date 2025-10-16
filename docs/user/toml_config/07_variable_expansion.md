# Chapter 7: Variable Expansion

## 7.1 Overview of Variable Expansion

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
| **Internal Variables** | For expansion only within TOML configuration files | `%{VAR}` | `vars`, `from_env` | None (default) |
| **Process Environment Variables** | Set as environment variables for child processes | - | `env` | Yes |

### Locations Where Variables Can Be Used

Variable expansion can be used in the following locations:

- **cmd**: Path to the command to execute (use `%{VAR}`)
- **args**: Command arguments (use `%{VAR}`)
- **env**: Process environment variable values (use `%{VAR}` if needed)
- **verify_files**: Paths of files to verify (use `%{VAR}`)
- **vars**: Internal variable definitions (can reference other internal variables with `%{VAR}`)

## 7.2 Variable Expansion Syntax

### Internal Variable Reference Syntax

Internal variables are written in the format `%{variable_name}`:

```toml
cmd = "%{VARIABLE_NAME}"
args = ["%{ARG1}", "%{ARG2}"]
env = ["VAR=%{VALUE}"]
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

## 7.3 Internal Variable Definition

### 7.3.1 Defining Internal Variables Using the `vars` Field

#### Overview

Using the `vars` field, you can define internal variables for TOML expansion only. These variables do not affect the environment variables of child processes.

#### Configuration Format

```toml
[global]
vars = [
    "app_dir=/opt/myapp",
    "log_level=info"
]

[[groups]]
name = "backup"
vars = [
    "backup_dir=%{app_dir}/backups",
    "retention_days=30"
]

[[groups.commands]]
name = "backup_db"
vars = [
    "timestamp=20250114",
    "output_file=%{backup_dir}/dump_%{timestamp}.sql"
]
cmd = "/usr/bin/pg_dump"
args = ["-f", "%{output_file}", "mydb"]
```

#### Scope and Inheritance

| Level | Scope | Inheritance Rule |
|-------|-------|------------------|
| **Global.vars** | Accessible from all groups and commands | - |
| **Group.vars** | Accessible from commands in that group | Merge with Global.vars (Group takes priority) |
| **Command.vars** | Accessible only within that command | Merge Global + Group + Command |

#### Reference Syntax

- Reference in the format `%{variable_name}`
- Can be used in the values of `cmd`, `args`, `verify_files`, `env`, and in other `vars` definitions

#### Basic Example

```toml
version = "1.0"

[global]
vars = ["base_dir=/opt"]

[[groups]]
name = "prod_backup"
vars = ["db_tools=%{base_dir}/db-tools"]

[[groups.commands]]
name = "db_dump"
vars = [
    "timestamp=20250114",
    "output_file=%{base_dir}/dump_%{timestamp}.sql"
]
cmd = "%{db_tools}/dump.sh"
args = ["-o", "%{output_file}"]
```

### 7.3.2 Importing System Environment Variables Using `from_env`

#### Overview

Using the `from_env` field, you can import system environment variables as internal variables.

#### Configuration Format

```toml
[global]
env_allowlist = ["HOME", "PATH", "USER"]
from_env = [
    "home=HOME",
    "user_path=PATH",
    "username=USER"
]

[[groups]]
name = "example"
from_env = [
    "custom=CUSTOM_VAR"  # Import specific to this group
]
```

#### Syntax

Written in the format `internal_variable_name=system_environment_variable_name`:

- **Left side**: Internal variable name (recommended lowercase, e.g., `home`, `user_path`)
- **Right side**: System environment variable name (typically uppercase, e.g., `HOME`, `PATH`)

#### Security Constraints

- System environment variables referenced in `from_env` must be included in `env_allowlist`
- An error will occur if you reference a variable not in `env_allowlist`

#### Inheritance Rules

| Level | Inheritance Behavior |
|-------|----------------------|
| **Global.from_env** | Inherited by all groups (default) |
| **Group.from_env** | If defined, **overrides** (Override) Global.from_env |
| **Group.from_env is nil** | Inherits Global.from_env |
| **Group.from_env is []** | Empty mapping (no environment variables imported) |

#### Example: Importing System Environment Variables

```toml
version = "1.0"

[global]
env_allowlist = ["HOME", "PATH"]
from_env = [
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

### 7.3.3 Nesting Internal Variables

Internal variable values can contain references to other internal variables.

#### Basic Example

```toml
[global]
vars = [
    "base=/opt",
    "app_dir=%{base}/myapp",
    "log_dir=%{app_dir}/logs"
]

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

### 7.3.4 Circular Reference Detection

Circular references are detected as errors:

```toml
[[groups.commands]]
name = "circular"
vars = [
    "var1=%{var2}",
    "var2=%{var1}"  # Error: circular reference
]
cmd = "/bin/echo"
args = ["%{var1}"]
```

## 7.4 Defining Process Environment Variables

### 7.4.1 Setting Environment Variables Using the `env` Field

#### Overview

Environment variables defined in the `env` field are passed to child processes when commands are executed. Internal variables (`%{VAR}`) can be used in these values.

#### Configuration Format

```toml
[global]
env = [
    "LOG_LEVEL=info",
    "APP_ENV=production"
]

[[groups]]
name = "app_tasks"
env = [
    "DB_HOST=localhost",
    "DB_PORT=5432"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
env = [
    "CONFIG_FILE=%{config_path}"  # Internal variables can be used
]
vars = ["config_path=/etc/myapp/config.yml"]
```

#### Inheritance and Merging

The `env` field is merged as follows:

1. Global.env
2. Group.env (combined with Global)
3. Command.env (combined with Global + Group)

When the same environment variable name is defined at multiple levels, the more specific level (Command > Group > Global) takes priority.

#### Relationship with Internal Variables

- Internal variables can be referenced in `env` values using the `%{VAR}` format
- By default, environment variables defined in `env` are passed only to child processes and cannot be used as internal variables
- If you want to use them as internal variables, define them in the `vars` field

#### Example: Setting Process Environment Variables Using Internal Variables

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/myapp",
    "log_dir=%{app_dir}/logs"
]
env = [
    "APP_HOME=%{app_dir}",
    "LOG_PATH=%{log_dir}/app.log"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--verbose"]
# Child process receives APP_HOME=/opt/myapp, LOG_PATH=/opt/myapp/logs/app.log
```

## 7.5 Detailed Locations Where Variables Can Be Used

### 7.5.1 Variable Expansion in cmd

Internal variables can be used in command paths.

#### Example 1: Basic Command Path Expansion

```toml
[[groups.commands]]
name = "docker_version"
cmd = "%{docker_cmd}"
args = ["version"]
vars = ["docker_cmd=/usr/bin/docker"]
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
vars = [
    "toolchain_dir=/opt/toolchains",
    "version=11.2.0"
]
```

At runtime:
- `%{toolchain_dir}` → expands to `/opt/toolchains`
- `%{version}` → expands to `11.2.0`
- Actual execution: `/opt/toolchains/gcc-11.2.0/bin/gcc -o output main.c`

### 7.5.2 Variable Expansion in args

Internal variables can be used in command arguments.

#### Example 1: File Path Construction

```toml
[[groups.commands]]
name = "backup_copy"
cmd = "/bin/cp"
args = ["%{source_file}", "%{dest_file}"]
vars = [
    "source_file=/data/original.txt",
    "dest_file=/backups/backup.txt"
]
```

#### Example 2: Multiple Variables in One Argument

```toml
[[groups.commands]]
name = "ssh_connect"
cmd = "/usr/bin/ssh"
args = ["%{user}@%{host}:%{port}"]
vars = [
    "user=admin",
    "host=server01.example.com",
    "port=22"
]
```

At runtime:
- `%{user}@%{host}:%{port}` → expands to `admin@server01.example.com:22`

#### Example 3: Configuration File Switching

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_dir}/%{env_type}.yml"]
vars = [
    "config_dir=/etc/myapp/configs",
    "env_type=production"
]
```

At runtime:
- `%{config_dir}/%{env_type}.yml` → expands to `/etc/myapp/configs/production.yml`

### 7.5.3 Combining Multiple Variables

Multiple variables can be combined to construct complex paths and strings.

#### Example 1: Backup Path with Timestamp

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/bin/mkdir"
args = ["-p", "%{backup_root}/%{date}/%{user}/data"]
vars = [
    "backup_root=/var/backups",
    "date=2025-10-02",
    "user=admin"
]
```

At runtime:
- `%{backup_root}/%{date}/%{user}/data` → expands to `/var/backups/2025-10-02/admin/data`

#### Example 2: Database Connection String

```toml
[[groups.commands]]
name = "db_connect"
cmd = "/usr/bin/psql"
args = ["postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"]
vars = [
    "db_user=appuser",
    "db_pass=secret123",
    "db_host=localhost",
    "db_port=5432",
    "db_name=myapp_db"
]
```

At runtime:
- Connection string is fully expanded
- `postgresql://appuser:secret123@localhost:5432/myapp_db`

## 7.6 Practical Examples

### 7.6.1 Dynamic Command Path Construction

Example of switching command paths based on environment:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "PYTHON_ROOT", "PY_VERSION"]

[[groups]]
name = "python_tasks"

# Using Python 3.10
[[groups.commands]]
name = "run_with_py310"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]
vars = [
    "python_root=/usr/local",
    "py_version=3.10"
]

# Using Python 3.11
[[groups.commands]]
name = "run_with_py311"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]
vars = [
    "python_root=/usr/local",
    "py_version=3.11"
]
```

### 7.6.2 Dynamic Argument Generation

Dynamically constructing Docker container startup parameters:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "DOCKER_BIN"]

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
vars = [
    "docker_bin=/usr/bin/docker",
    "container_name=myapp-prod",
    "host_path=/opt/myapp/data",
    "container_path=/app/data",
    "app_env=production",
    "host_port=8080",
    "container_port=80",
    "image_name=myapp",
    "image_tag=v1.2.3"
]
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

### 7.6.3 Environment-Specific Configuration Switching

Using different configurations for development and production environments:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "APP_BIN", "CONFIG_DIR", "ENV_TYPE", "LOG_LEVEL", "DB_URL"]

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
vars = [
    "app_bin=/opt/myapp/bin/myapp",
    "config_dir=/etc/myapp/configs",
    "env_type=development",
    "log_level=debug",
    "db_url=postgresql://localhost/dev_db"
]

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
vars = [
    "app_bin=/opt/myapp/bin/myapp",
    "config_dir=/etc/myapp/configs",
    "env_type=production",
    "log_level=info",
    "db_url=postgresql://prod-server/prod_db"
]
```

## 7.8 Nested Variables

Variable values can contain other variables.

### Basic Example

```toml
[[groups.commands]]
name = "nested_vars"
cmd = "/bin/echo"
args = ["Message: %{full_msg}"]
vars = [
    "full_msg=Hello, %{user}!",
    "user=Alice"
]
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
vars = [
    "config_path=%{base_dir}/%{env_type}/config.yml",
    "base_dir=/opt/myapp",
    "env_type=production"
]
```

Expansion order:
1. `%{base_dir}` → expands to `/opt/myapp`
2. `%{env_type}` → expands to `production`
3. `%{config_path}` → expands to `/opt/myapp/production/config.yml`

## 7.9 Variable Self-Reference

Variable self-reference is an important feature commonly used when extending environment variables. It is particularly useful for environment variables like `PATH`, where you want to add new values to existing ones.

### How Self-Reference Works

In expressions like `PATH=/custom/bin:%{path}`, the `%{path}` refers to a **system environment variable imported via `from_env`** or it can reference an internal variable. This is not a circular reference but an intentionally supported feature.

### Basic Example: PATH Extension

```toml
[global]
env_allowlist = ["PATH"]
from_env = ["path=PATH"]

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["PATH is: %{path}"]
env = ["PATH=/opt/mytools/bin:%{path}"]
```

Expansion process:
1. Import system environment variable `PATH` as `%{path}` (e.g., `/usr/bin:/bin`)
2. `%{path}` → expands to `/usr/bin:/bin`
3. Final value: `/opt/mytools/bin:/usr/bin:/bin`

### Practical Example: Adding Custom Tool Directory

```toml
[global]
env_allowlist = ["PATH"]
from_env = ["path=PATH"]

[[groups.commands]]
name = "use_custom_tools"
cmd = "%{custom_tool}"
args = ["--version"]
vars = [
    "tool_dir=/opt/custom-tools",
    "custom_tool=%{tool_dir}/bin/mytool"
]
env = [
    "PATH=%{tool_dir}/bin:%{path}"
]
```

With this configuration:
- `%{custom_tool}` can be found from the extended `PATH` even when specified as just a command name (not a full path)
- The existing system `PATH` is preserved

### Self-Reference with Other Environment Variables

`PATH` is not the only environment variable that supports self-reference:

```toml
[global]
env_allowlist = ["LD_LIBRARY_PATH", "PYTHONPATH"]
from_env = [
    "ld_library_path=LD_LIBRARY_PATH",
    "pythonpath=PYTHONPATH"
]

[[groups.commands]]
name = "extend_lib_path"
cmd = "/opt/myapp/bin/app"
args = []
env = [
    "LD_LIBRARY_PATH=/opt/myapp/lib:%{ld_library_path}",
    "PYTHONPATH=/opt/myapp/python:%{pythonpath}"
]
```

### Difference Between Self-Reference and Circular Reference

**Self-Reference (Normal)**: Referencing a system environment variable imported via `from_env` or an internal variable
```toml
env = ["PATH=/custom/bin:%{path}"]  # %{path} refers to system environment variable
```

**Circular Reference (Error)**: Variables within vars reference each other circularly
```toml
vars = [
    "var1=%{var2}",
    "var2=%{var1}",  # Error: Circular reference
]
```

### Important Notes

1. **When system environment variable doesn't exist**: If the system environment variable referenced in `from_env` doesn't exist, an error will occur
2. **Relationship with allowlist**: When referencing system environment variables via `from_env`, those variables must be included in `env_allowlist`

```toml
[global]
env_allowlist = ["PATH", "HOME"]  # Allow PATH and HOME to be imported

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["%{path}"]
vars = ["path=PATH_PREFIX:/custom:%{system_path}"]
from_env = ["system_path=PATH"]  # OK: PATH is included in allowlist
env = ["PATH=%{path}"]
```

## 7.10 Escape Sequences

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
vars = ["user=JohnDoe"]
```

Output: `Path: C:\Users\JohnDoe`

### Mixed Example

```toml
[[groups.commands]]
name = "mixed_escape"
cmd = "/bin/echo"
args = ["Literal \\%{HOME} is different from %{home}"]
vars = ["home=/home/user"]
```

Output: `Literal $HOME is different from /home/user`

## 7.11 Automatic Environment Variables

### 7.11.1 Overview

The system automatically sets the following internal variables for each command execution:

- **`__runner_datetime`**: Execution time (UTC) in YYYYMMDDHHmmSS.msec format
- **`__runner_pid`**: Process ID of the runner process

These variables can be used in command paths, arguments, and environment variable values just like regular internal variables.

### 7.11.2 Usage Examples

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

### 7.11.3 DateTime Format

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

### 7.11.4 Reserved Prefix

The prefix `__runner_` is reserved for automatic variables and cannot be used for user-defined variables.

#### Error Example

```toml
[[groups.commands]]
name = "invalid_var"
cmd = "/bin/echo"
args = ["%{__runner_custom}"]
vars = ["__runner_custom=value"]  # Error: Using reserved prefix
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
vars = ["my_custom_var=value"]  # OK: Not using reserved prefix
```

### 7.11.5 Timing of Variable Generation

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

## 7.12 Security Considerations

### 7.9.1 Command.Env Priority

Variables defined in `Command.Env` take priority over system environment variables:

```toml
[global]
env_allowlist = ["PATH", "HOME"]

[[groups.commands]]
name = "override_home"
cmd = "/bin/echo"
args = ["Home: ${HOME}"]
env = ["HOME=/opt/custom-home"]
# The HOME from Command.Env is used, not the system $HOME
```

### 7.9.2 Relationship with env_allowlist

**Important**: Variables defined in `Command.Env` are not subject to `env_allowlist` checks.

```toml
[global]
env_allowlist = ["PATH", "HOME"]
# CUSTOM_VAR is not in allowlist

[[groups.commands]]
name = "custom_var"
cmd = "${CUSTOM_TOOL}"
args = []
env = ["CUSTOM_TOOL=/opt/tools/mytool"]
# CUSTOM_TOOL is not in allowlist, but can be used because it's defined in Command.Env
```

### 7.9.3 Absolute Path Requirements

Command paths after expansion must be absolute paths:

```toml
# Correct: expands to absolute path
[[groups.commands]]
name = "valid"
cmd = "${TOOL_DIR}/mytool"
env = ["TOOL_DIR=/opt/tools"]  # Absolute path

# Incorrect: expands to relative path
[[groups.commands]]
name = "invalid"
cmd = "${TOOL_DIR}/mytool"
env = ["TOOL_DIR=./tools"]  # Relative path - error
```

### 7.9.4 Handling Sensitive Information

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
env = [
    "API_TOKEN=sk-1234567890abcdef",
    "API_ENDPOINT=https://api.example.com",
]
```

### 7.12.5 Isolation Between Commands

Each command's `env` is independent and does not affect other commands:

```toml
[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["DB: %{db_host}"]
vars = ["db_host=db1.example.com"]
env = ["DB_HOST=%{db_host}"]

[[groups.commands]]
name = "cmd2"
cmd = "/bin/echo"
args = ["DB: %{db_host}"]
vars = ["db_host=db2.example.com"]
env = ["DB_HOST=%{db_host}"]
# Independent from cmd1's DB_HOST
```

## 7.13 Troubleshooting

### Undefined Variables

If a variable is not defined, an error occurs:

```toml
[[groups.commands]]
name = "undefined_var"
cmd = "/bin/echo"
args = ["Value: ${UNDEFINED}"]
# UNDEFINED is not defined in env → error
```

**Solution**: Define all required variables in `env`

### Circular References

If variables reference each other, an error occurs:

```toml
[[groups.commands]]
name = "circular"
cmd = "/bin/echo"
args = ["${VAR1}"]
env = [
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
env = ["TOOL=../tool"]  # Relative path → error
```

**Solution**: Use absolute paths

## Comprehensive Practical Example

The following is a practical configuration example utilizing variable expansion:

```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "application_deployment"
description = "Application deployment process"

# Step 1: Deploy configuration file
[[groups.commands]]
name = "deploy_config"
description = "Deploy environment-specific configuration file"
cmd = "/bin/cp"
args = [
    "${CONFIG_SOURCE}/${ENV_TYPE}/app.yml",
    "${CONFIG_DEST}/app.yml",
]
env = [
    "CONFIG_SOURCE=/opt/configs/templates",
    "CONFIG_DEST=/etc/myapp",
    "ENV_TYPE=production",
]

# Step 2: Database migration
[[groups.commands]]
name = "db_migration"
description = "Database schema migration"
cmd = "${APP_BIN}/migrate"
args = [
    "--database", "${DB_URL}",
    "--migrations", "${MIGRATION_DIR}",
]
env = [
    "APP_BIN=/opt/myapp/bin",
    "DB_URL=postgresql://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}",
    "DB_USER=appuser",
    "DB_PASS=secret123",
    "DB_HOST=localhost",
    "DB_PORT=5432",
    "DB_NAME=myapp_prod",
    "MIGRATION_DIR=/opt/myapp/migrations",
]
timeout = 600

# Step 3: Start application
[[groups.commands]]
name = "start_application"
description = "Start application server"
cmd = "${APP_BIN}/server"
args = [
    "--config", "${CONFIG_DEST}/app.yml",
    "--port", "${APP_PORT}",
    "--workers", "${WORKER_COUNT}",
]
env = [
    "APP_BIN=/opt/myapp/bin",
    "CONFIG_DEST=/etc/myapp",
    "APP_PORT=8080",
    "WORKER_COUNT=4",
]

# Step 4: Health check
[[groups.commands]]
name = "health_check"
description = "Application health check"
cmd = "/usr/bin/curl"
args = [
    "-f",
    "${HEALTH_URL}",
]
env = [
    "HEALTH_URL=http://localhost:8080/health",
]
timeout = 30
```

## 7.14 Variable Expansion in verify_files

### 7.11.1 Overview

The `verify_files` field also supports environment variable expansion. This allows you to dynamically construct file verification paths and provides flexible verification configuration depending on the environment.

### 7.11.2 Target Fields

Variable expansion can be used in the following `verify_files` fields:

- **Global level**: `verify_files` in the `[global]` section
- **Group level**: `verify_files` in the `[[groups]]` section

### 7.11.3 Basic Examples

#### Global Level Expansion

```toml
version = "1.0"

[global]
env_allowlist = ["HOME"]
verify_files = [
    "${HOME}/config.toml",
    "${HOME}/data.txt",
]

[[groups]]
name = "example"

[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["hello"]
```

Expansion result (when `HOME=/home/user`):
- `${HOME}/config.toml` → `/home/user/config.toml`
- `${HOME}/data.txt` → `/home/user/data.txt`

#### Group Level Expansion

```toml
version = "1.0"

[global]
env_allowlist = ["APP_ROOT"]

[[groups]]
name = "app_group"
env_allowlist = ["APP_ROOT"]
verify_files = [
    "${APP_ROOT}/config/app.yml",
    "${APP_ROOT}/bin/server",
]

[[groups.commands]]
name = "start"
cmd = "/bin/echo"
args = ["Starting app"]
```

Expansion result (when `APP_ROOT=/opt/myapp`):
- `${APP_ROOT}/config/app.yml` → `/opt/myapp/config/app.yml`
- `${APP_ROOT}/bin/server` → `/opt/myapp/bin/server`

### 7.11.4 Complex Example

Example with dynamic path construction:

```toml
version = "1.0"

[global]
env_allowlist = ["ENV", "APP_ROOT"]
from_env = [
    "env_type=ENV",
    "app_root=APP_ROOT"
]
vars = [
    "config_base=%{app_root}/configs",
    "config_path=%{config_base}/%{env_type}"
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

[[groups]]
name = "deployment"

[[groups.commands]]
name = "deploy"
cmd = "/opt/deploy.sh"
```

Execution (when `ENV=production APP_ROOT=/opt/myapp`):
```bash
export DEPLOY_ENV=production
export APP_ROOT=/opt/myapp
export CONFIG_ROOT=/etc/myapp/config
```

The following files will be verified:
- `/opt/myapp/configs/production/global.yml`
- `/opt/myapp/configs/production/secrets.enc`
- `/opt/myapp/web/nginx.conf`
- `/opt/myapp/web/ssl/cert.pem`
- `/opt/myapp/web/ssl/key.pem`
- `/opt/myapp/db/schema.sql`
- `/opt/myapp/db/migrations/production/`

### 7.11.5 Limitations

1. **Absolute Path Requirement**: Expanded paths must be absolute paths
2. **System Environment Variables Only**: verify_files can only use system environment variables, not Command.Env variables
3. **Expansion Timing**: Expansion happens once at configuration load time (not at execution time)

## 7.12 Practical Comprehensive Example

Below is a practical configuration example using variable expansion features:

```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER"]
from_env = [
    "home=HOME",
    "username=USER"
]
vars = [
    "app_root=/opt/myapp",
    "config_dir=%{app_root}/config",
    "bin_dir=%{app_root}/bin"
]

[[groups]]
name = "application_deployment"
description = "Application deployment process"
vars = [
    "env_type=production",
    "log_dir=%{app_root}/logs"
]

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
vars = [
    "db_user=appuser",
    "db_pass=secret123",
    "db_host=localhost",
    "db_port=5432",
    "db_name=myapp_prod",
    "db_url=postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}",
    "migration_dir=%{app_root}/migrations"
]
timeout = 600

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
vars = [
    "app_port=8080",
    "worker_count=4"
]
env = [
    "LOG_LEVEL=info",
    "LOG_PATH=%{log_dir}/app.log"
]

# Step 4: Health check
[[groups.commands]]
name = "health_check"
description = "Application health check"
cmd = "/usr/bin/curl"
args = ["-f", "%{health_url}"]
vars = ["health_url=http://localhost:%{app_port}/health"]
timeout = 30
```

## 7.13 Summary

### Overall View of Variable System

The go-safe-cmd-runner variable system consists of three main components:

1. **Internal Variables** (`vars`, `from_env`)
   - Used exclusively for TOML expansion
   - Referenced using `%{VAR}` syntax
   - Not passed to child processes (by default)

2. **Process Environment Variables** (`env`)
   - Environment variables passed to child processes at execution time
   - Can use internal variables `%{VAR}` in values

3. **Automatic Variables** (`__runner_datetime`, `__runner_pid`)
   - Automatically generated by the system
   - Available as internal variables

### Best Practices

1. **Utilize internal variables**: Define values that are only needed for TOML expansion (like paths and URLs) using `vars`
2. **Explicitly import with from_env**: Import system environment variables explicitly using `from_env` to make intentions clear
3. **Minimize env usage**: Keep environment variables passed to child processes to the minimum necessary
4. **Consider security**: Handle sensitive information carefully and avoid passing unnecessary environment variables
5. **Standardize naming conventions**: Use lowercase with underscores for internal variables (e.g., `app_dir`), and uppercase for environment variables

### Next Steps

In the next chapter, we'll cover practical examples combining all the variable expansion features you've learned. You'll learn how to create configuration files based on real-world use cases.
