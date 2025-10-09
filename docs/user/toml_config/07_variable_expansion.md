# Chapter 7: Variable Expansion

## 7.1 Overview of Variable Expansion

Variable expansion is a feature that allows you to embed variables in commands and their arguments, which are then replaced with actual values at runtime. This enables dynamic command construction and easy switching between environment-specific configurations.

### Key Benefits

1. **Dynamic Command Construction**: Values can be determined at runtime
2. **Configuration Reuse**: The same variable can be used in multiple places
3. **Environment Switching**: Easy switching between development/production environments
4. **Improved Maintainability**: Changes can be centralized in one location

### Locations Where Variables Can Be Used

Variable expansion can be used in the following locations:

- **cmd**: Path to the command to execute
- **args**: Command arguments
- **env**: Environment variable values (VALUE portion)

## 7.2 Variable Expansion Syntax

### Basic Syntax

Variables are written in the format `${VARIABLE_NAME}`:

```toml
cmd = "${VARIABLE_NAME}"
args = ["${ARG1}", "${ARG2}"]
env = ["VAR=${VALUE}"]
```

### Variable Naming Rules

- Uppercase letters, numbers, and underscores (`_`) are allowed
- Uppercase is used by convention (e.g., `MY_VARIABLE`)
- Must start with a letter or underscore

```toml
# Valid variable names
"${PATH}"
"${MY_TOOL}"
"${_PRIVATE_VAR}"
"${VAR123}"

# Invalid variable names
"${123VAR}"      # Starts with a number
"${my-var}"      # Hyphens not allowed
"${my.var}"      # Dots not allowed
```

## 7.3 Locations Where Variables Can Be Used

### 7.3.1 Variable Expansion in cmd

Command paths can be specified using variables.

#### Example 1: Basic Command Path Expansion

```toml
[[groups.commands]]
name = "docker_version"
cmd = "${DOCKER_CMD}"
args = ["version"]
env = ["DOCKER_CMD=/usr/bin/docker"]
```

At runtime:
- `${DOCKER_CMD}` → expands to `/usr/bin/docker`
- Actual execution: `/usr/bin/docker version`

#### Example 2: Version-Managed Tools

```toml
[[groups.commands]]
name = "gcc_compile"
cmd = "${TOOLCHAIN_DIR}/gcc-${VERSION}/bin/gcc"
args = ["-o", "output", "main.c"]
env = [
    "TOOLCHAIN_DIR=/opt/toolchains",
    "VERSION=11.2.0",
]
```

At runtime:
- `${TOOLCHAIN_DIR}` → expands to `/opt/toolchains`
- `${VERSION}` → expands to `11.2.0`
- Actual execution: `/opt/toolchains/gcc-11.2.0/bin/gcc -o output main.c`

### 7.3.2 Variable Expansion in args

Variables can be used in command arguments.

#### Example 1: File Path Construction

```toml
[[groups.commands]]
name = "backup_copy"
cmd = "/bin/cp"
args = ["${SOURCE_FILE}", "${DEST_FILE}"]
env = [
    "SOURCE_FILE=/data/original.txt",
    "DEST_FILE=/backups/backup.txt",
]
```

#### Example 2: Multiple Variables in One Argument

```toml
[[groups.commands]]
name = "ssh_connect"
cmd = "/usr/bin/ssh"
args = ["${USER}@${HOST}:${PORT}"]
env = [
    "USER=admin",
    "HOST=server01.example.com",
    "PORT=22",
]
```

At runtime:
- `${USER}@${HOST}:${PORT}` → expands to `admin@server01.example.com:22`

#### Example 3: Configuration File Switching

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "${CONFIG_DIR}/${ENV_TYPE}.yml"]
env = [
    "CONFIG_DIR=/etc/myapp/configs",
    "ENV_TYPE=production",
]
```

At runtime:
- `${CONFIG_DIR}/${ENV_TYPE}.yml` → expands to `/etc/myapp/configs/production.yml`

### 7.3.3 Combining Multiple Variables

Multiple variables can be combined to construct complex paths and strings.

#### Example 1: Backup Path with Timestamp

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/bin/mkdir"
args = ["-p", "${BACKUP_ROOT}/${DATE}/${USER}/data"]
env = [
    "BACKUP_ROOT=/var/backups",
    "DATE=2025-10-02",
    "USER=admin",
]
```

At runtime:
- `${BACKUP_ROOT}/${DATE}/${USER}/data` → expands to `/var/backups/2025-10-02/admin/data`

#### Example 2: Database Connection String

```toml
[[groups.commands]]
name = "db_connect"
cmd = "/usr/bin/psql"
args = ["postgresql://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}"]
env = [
    "DB_USER=appuser",
    "DB_PASS=secret123",
    "DB_HOST=localhost",
    "DB_PORT=5432",
    "DB_NAME=myapp_db",
]
```

At runtime:
- Connection string is fully expanded
- `postgresql://appuser:secret123@localhost:5432/myapp_db`

## 7.4 Practical Examples

### 7.4.1 Dynamic Command Path Construction

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
cmd = "${PYTHON_ROOT}/python${PY_VERSION}/bin/python"
args = ["-V"]
env = [
    "PYTHON_ROOT=/usr/local",
    "PY_VERSION=3.10",
]

# Using Python 3.11
[[groups.commands]]
name = "run_with_py311"
cmd = "${PYTHON_ROOT}/python${PY_VERSION}/bin/python"
args = ["-V"]
env = [
    "PYTHON_ROOT=/usr/local",
    "PY_VERSION=3.11",
]
```

### 7.4.2 Dynamic Argument Generation

Dynamically constructing Docker container startup parameters:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "DOCKER_BIN"]

[[groups]]
name = "docker_deployment"

[[groups.commands]]
name = "start_container"
cmd = "${DOCKER_BIN}"
args = [
    "run",
    "-d",
    "--name", "${CONTAINER_NAME}",
    "-v", "${HOST_PATH}:${CONTAINER_PATH}",
    "-e", "APP_ENV=${APP_ENV}",
    "-p", "${HOST_PORT}:${CONTAINER_PORT}",
    "${IMAGE_NAME}:${IMAGE_TAG}",
]
env = [
    "DOCKER_BIN=/usr/bin/docker",
    "CONTAINER_NAME=myapp-prod",
    "HOST_PATH=/opt/myapp/data",
    "CONTAINER_PATH=/app/data",
    "APP_ENV=production",
    "HOST_PORT=8080",
    "CONTAINER_PORT=80",
    "IMAGE_NAME=myapp",
    "IMAGE_TAG=v1.2.3",
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

### 7.4.3 Environment-Specific Configuration Switching

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
cmd = "${APP_BIN}"
args = [
    "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml",
    "--log-level", "${LOG_LEVEL}",
    "--db", "${DB_URL}",
]
env = [
    "APP_BIN=/opt/myapp/bin/myapp",
    "CONFIG_DIR=/etc/myapp/configs",
    "ENV_TYPE=development",
    "LOG_LEVEL=debug",
    "DB_URL=postgresql://localhost/dev_db",
]

# Production environment group
[[groups]]
name = "production"

[[groups.commands]]
name = "run_prod"
cmd = "${APP_BIN}"
args = [
    "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml",
    "--log-level", "${LOG_LEVEL}",
    "--db", "${DB_URL}",
]
env = [
    "APP_BIN=/opt/myapp/bin/myapp",
    "CONFIG_DIR=/etc/myapp/configs",
    "ENV_TYPE=production",
    "LOG_LEVEL=info",
    "DB_URL=postgresql://prod-server/prod_db",
]
```

## 7.5 Nested Variables

Variable values can contain other variables.

### Basic Example

```toml
[[groups.commands]]
name = "nested_vars"
cmd = "/bin/echo"
args = ["Message: ${FULL_MSG}"]
env = [
    "FULL_MSG=Hello, ${USER}!",
    "USER=Alice",
]
```

Expansion order:
1. `${USER}` → expands to `Alice`
2. `${FULL_MSG}` → expands to `Hello, Alice!`
3. Final argument: `Message: Hello, Alice!`

### Complex Path Construction

```toml
[[groups.commands]]
name = "complex_path"
cmd = "/bin/echo"
args = ["Config path: ${CONFIG_PATH}"]
env = [
    "CONFIG_PATH=${BASE_DIR}/${ENV_TYPE}/config.yml",
    "BASE_DIR=/opt/myapp",
    "ENV_TYPE=production",
]
```

Expansion order:
1. `${BASE_DIR}` → expands to `/opt/myapp`
2. `${ENV_TYPE}` → expands to `production`
3. `${CONFIG_PATH}` → expands to `/opt/myapp/production/config.yml`

## 7.6 Variable Self-Reference

Variable self-reference is an important feature commonly used when extending environment variables. It is particularly useful for environment variables like `PATH`, where you want to add new values to existing ones.

### How Self-Reference Works

In expressions like `PATH=/custom/bin:${PATH}`, the `${PATH}` refers to the **original value of the system environment variable**. This is not a circular reference but an intentionally supported feature.

### Basic Example: PATH Extension

```toml
[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["PATH is: ${PATH}"]
env = ["PATH=/opt/mytools/bin:${PATH}"]
```

Expansion process:
1. Retrieve the value of the system environment variable `PATH` (e.g., `/usr/bin:/bin`)
2. `${PATH}` → expands to `/usr/bin:/bin`
3. Final value: `/opt/mytools/bin:/usr/bin:/bin`

### Practical Example: Adding Custom Tool Directory

```toml
[[groups.commands]]
name = "use_custom_tools"
cmd = "${CUSTOM_TOOL}"
args = ["--version"]
env = [
    "PATH=${TOOL_DIR}/bin:${PATH}",
    "TOOL_DIR=/opt/custom-tools",
    "CUSTOM_TOOL=mytool",
]
```

With this configuration:
- `CUSTOM_TOOL` can be found from the extended `PATH` even when specified as just a command name (not a full path)
- The existing system `PATH` is preserved

### Self-Reference with Other Environment Variables

`PATH` is not the only environment variable that supports self-reference:

```toml
[[groups.commands]]
name = "extend_lib_path"
cmd = "/opt/myapp/bin/app"
args = []
env = [
    "LD_LIBRARY_PATH=/opt/myapp/lib:${LD_LIBRARY_PATH}",
    "PYTHONPATH=/opt/myapp/python:${PYTHONPATH}",
]
```

### Difference Between Self-Reference and Circular Reference

**Self-Reference (Normal)**: A variable defined in Command.Env references the **system environment variable** with the same name
```toml
env = ["PATH=/custom/bin:${PATH}"]  # ${PATH} refers to system environment variable
```

**Circular Reference (Error)**: Variables within Command.Env reference each other
```toml
env = [
    "VAR1=${VAR2}",
    "VAR2=${VAR1}",  # Error: Circular reference within Command.Env
]
```

### Important Notes

1. **When system environment variable doesn't exist**: If `PATH` doesn't exist in the system when referencing `${PATH}`, an error will occur
2. **Relationship with allowlist**: When referencing system environment variables, those variables must be included in `env_allowlist`

```toml
[global]
env_allowlist = ["PATH", "HOME"]  # Allow PATH self-reference

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["${PATH}"]
env = ["PATH=/custom:${PATH}"]  # OK: PATH is included in allowlist
```

## 7.7 Escape Sequences

When you want to use literal `$` or `\` characters, escaping is required.

### Escaping Dollar Signs

Use `\$` to represent a literal dollar sign:

```toml
[[groups.commands]]
name = "price_display"
cmd = "/bin/echo"
args = ["Price: \\$100 USD"]
```

Output: `Price: $100 USD`

### Escaping Backslashes

Use `\\` to represent a literal backslash:

```toml
[[groups.commands]]
name = "windows_path"
cmd = "/bin/echo"
args = ["Path: C:\\\\Users\\\\${USER}"]
env = ["USER=JohnDoe"]
```

Output: `Path: C:\Users\JohnDoe`

### Mixed Example

```toml
[[groups.commands]]
name = "mixed_escape"
cmd = "/bin/echo"
args = ["Literal \\$HOME is different from ${HOME}"]
env = ["HOME=/home/user"]
```

Output: `Literal $HOME is different from /home/user`

## 7.8 Automatic Environment Variables

### 7.8.1 Overview

The system automatically sets the following environment variables for each command execution:

- **`__RUNNER_DATETIME`**: Execution time (UTC) in YYYYMMDDHHmmSS.msec format
- **`__RUNNER_PID`**: Process ID of the runner process

These variables can be used in command paths, arguments, and environment variable values just like regular variables.

### 7.8.2 Usage Examples

#### Timestamped Backups

```toml
[[groups.commands]]
name = "backup_with_timestamp"
description = "Create backup with timestamp"
cmd = "/usr/bin/tar"
args = [
    "czf",
    "/tmp/backup/data-${__RUNNER_DATETIME}.tar.gz",
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
    "echo ${__RUNNER_PID} > /var/run/myapp-${__RUNNER_PID}.lock"
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
    "echo 'Executed at ${__RUNNER_DATETIME} by PID ${__RUNNER_PID}' >> /var/log/executions.log"
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
    "--output", "/reports/${__RUNNER_DATETIME}-${__RUNNER_PID}.html",
    "--title", "Report ${__RUNNER_DATETIME}",
]
```

Example execution:
- Output file: `/reports/20251005143022.123-12345.html`
- Report title: `Report 20251005143022.123`

### 7.8.3 DateTime Format

Format specification for `__RUNNER_DATETIME`:

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

### 7.8.4 Reserved Prefix

The prefix `__RUNNER_` is reserved for automatic environment variables and cannot be used for user-defined environment variables.

#### Error Example

```toml
[[groups.commands]]
name = "invalid_env"
cmd = "/bin/echo"
args = ["${__RUNNER_CUSTOM}"]
env = ["__RUNNER_CUSTOM=value"]  # Error: Using reserved prefix
```

Error message:
```
environment variable "__RUNNER_CUSTOM" uses reserved prefix "__RUNNER_";
this prefix is reserved for automatically generated variables
```

#### Correct Example

```toml
[[groups.commands]]
name = "valid_env"
cmd = "/bin/echo"
args = ["${MY_CUSTOM_VAR}"]
env = ["MY_CUSTOM_VAR=value"]  # OK: Not using reserved prefix
```

### 7.8.5 Timing of Variable Generation

Automatic environment variables (`__RUNNER_DATETIME` and `__RUNNER_PID`) are generated once when the configuration file is loaded, not at each command execution time. All commands in all groups share the exact same values throughout the entire runner execution.

```toml
[[groups]]
name = "backup_group"

[[groups.commands]]
name = "backup_db"
cmd = "/usr/bin/pg_dump"
args = ["-f", "/tmp/backup/db-${__RUNNER_DATETIME}.sql", "mydb"]

[[groups.commands]]
name = "backup_files"
cmd = "/usr/bin/tar"
args = ["czf", "/tmp/backup/files-${__RUNNER_DATETIME}.tar.gz", "/data"]
```

**Key Point**: Both commands use the exact same timestamp because `__RUNNER_DATETIME` is sampled at config load time, not at execution time:
- `/tmp/backup/db-20251005143022.123.sql`
- `/tmp/backup/files-20251005143022.123.tar.gz`

This ensures consistency across all commands in a single runner execution, even if commands are executed at different times or in different groups.

## 7.9 Security Considerations

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

### 7.9.5 Isolation Between Commands

Each command's `env` is independent and does not affect other commands:

```toml
[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["DB: ${DB_HOST}"]
env = ["DB_HOST=db1.example.com"]

[[groups.commands]]
name = "cmd2"
cmd = "/bin/echo"
args = ["DB: ${DB_HOST}"]
env = ["DB_HOST=db2.example.com"]
# Independent from cmd1's DB_HOST
```

## 7.10 Troubleshooting

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

## 7.11 Variable Expansion in verify_files

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
args = ["starting"]
```

When system environment variable `APP_ROOT=/opt/myapp`, expansion result:
- `${APP_ROOT}/config/app.yml` → `/opt/myapp/config/app.yml`
- `${APP_ROOT}/bin/server` → `/opt/myapp/bin/server`

### 7.11.4 Using Multiple Variables

By combining multiple environment variables, you can construct more flexible paths.

```toml
version = "1.0"

[global]
env_allowlist = ["BASE_DIR", "APP_NAME"]
verify_files = [
    "${BASE_DIR}/${APP_NAME}/config.toml",
    "${BASE_DIR}/${APP_NAME}/data/db.sqlite",
]

[[groups]]
name = "app_tasks"

[[groups.commands]]
name = "run"
cmd = "/bin/echo"
args = ["running"]
```

Expansion result (when `BASE_DIR=/opt`, `APP_NAME=myapp`):
- `${BASE_DIR}/${APP_NAME}/config.toml` → `/opt/myapp/config.toml`
- `${BASE_DIR}/${APP_NAME}/data/db.sqlite` → `/opt/myapp/data/db.sqlite`

### 7.11.5 Environment-Specific Configuration Example

Example of verifying different files for development and production environments:

```toml
version = "1.0"

[global]
env_allowlist = ["ENV_TYPE", "CONFIG_ROOT"]
verify_files = ["${CONFIG_ROOT}/${ENV_TYPE}/global.toml"]

[[groups]]
name = "development"
env_allowlist = ["ENV_TYPE", "CONFIG_ROOT"]
verify_files = [
    "${CONFIG_ROOT}/${ENV_TYPE}/dev.toml",
    "${CONFIG_ROOT}/${ENV_TYPE}/dev_db.sqlite",
]

[[groups.commands]]
name = "dev_task"
cmd = "/bin/echo"
args = ["dev mode"]

[[groups]]
name = "production"
env_allowlist = ["ENV_TYPE", "CONFIG_ROOT"]
verify_files = [
    "${CONFIG_ROOT}/${ENV_TYPE}/prod.toml",
    "${CONFIG_ROOT}/${ENV_TYPE}/prod_db.sqlite",
]

[[groups.commands]]
name = "prod_task"
cmd = "/bin/echo"
args = ["prod mode"]
```

For development environment (`ENV_TYPE=dev`, `CONFIG_ROOT=/etc/myapp`):
- Global: `/etc/myapp/dev/global.toml`
- development group: `/etc/myapp/dev/dev.toml`, `/etc/myapp/dev/dev_db.sqlite`

For production environment (`ENV_TYPE=prod`, `CONFIG_ROOT=/etc/myapp`):
- Global: `/etc/myapp/prod/global.toml`
- production group: `/etc/myapp/prod/prod.toml`, `/etc/myapp/prod/prod_db.sqlite`

### 7.11.6 Relationship with allowlist

Security controls through `env_allowlist` are also applied to variable expansion in `verify_files`.

#### Global Level allowlist

Global `verify_files` uses the global `env_allowlist`:

```toml
[global]
env_allowlist = ["HOME", "USER"]
verify_files = [
    "${HOME}/config.toml",    # OK: HOME is in allowlist
    "${USER}/data.txt",       # OK: USER is in allowlist
]
```

#### Group Level allowlist Inheritance

Group `verify_files` uses the group's `env_allowlist`. If the group doesn't have an `env_allowlist` defined, it inherits the global configuration:

```toml
[global]
env_allowlist = ["GLOBAL_VAR"]

[[groups]]
name = "group_with_inheritance"
# env_allowlist not defined → inherits global configuration
verify_files = ["${GLOBAL_VAR}/file.txt"]  # OK: inherits global allowlist

[[groups]]
name = "group_with_explicit"
env_allowlist = ["GROUP_VAR"]  # explicitly defined
verify_files = ["${GROUP_VAR}/file.txt"]   # OK: uses group allowlist
```

#### allowlist Violation Errors

An error occurs when using variables not in the allowlist:

```toml
[global]
env_allowlist = ["SAFE_VAR"]
verify_files = ["${FORBIDDEN_VAR}/file.txt"]  # Error: not in allowlist
```

Example error message:
```
failed to expand global verify_files[0]: variable not allowed by group allowlist: FORBIDDEN_VAR
```

### 7.11.7 Escape Sequences

Escape sequences can also be used in verify_files:

```toml
[global]
env_allowlist = ["HOME"]
verify_files = [
    "${HOME}/config.toml",     # Variable will be expanded
    "\\${HOME}/literal.txt",   # Literal string "${HOME}/literal.txt"
]
```

Expansion result (when `HOME=/home/user`):
- `/home/user/config.toml`
- `${HOME}/literal.txt` (not expanded)

### 7.11.8 Runtime Behavior

Variable expansion in verify_files is automatically executed when the configuration file is loaded:

1. **Configuration file loading**: Parse TOML file
2. **Variable expansion execution**: Expand variables in verify_files
3. **Save expansion results**: Save expanded paths to internal fields
4. **Execute verification**: Use expanded paths for file verification

### 7.11.9 Troubleshooting

#### Undefined Variable Errors

An error occurs if a variable doesn't exist in the environment:

```toml
[global]
env_allowlist = ["UNDEFINED_VAR"]
verify_files = ["${UNDEFINED_VAR}/file.txt"]
```

Example error message:
```
failed to expand global verify_files[0]: variable not found in environment: UNDEFINED_VAR
```

**Solution**: Set the required environment variables in the system

#### allowlist Errors

An error occurs if a variable is not in the allowlist:

```toml
[global]
env_allowlist = ["ALLOWED_VAR"]
verify_files = ["${FORBIDDEN_VAR}/file.txt"]
```

Example error message:
```
failed to expand global verify_files[0]: variable not allowed by group allowlist: FORBIDDEN_VAR
```

**Solution**: Add the required variables to `env_allowlist`

#### Circular Reference Errors

An error occurs if variables reference each other (although circular references in system environment variables are extremely rare):

```bash
# Circular reference in system environment variables (unlikely in practice)
export VAR1="${VAR2}"
export VAR2="${VAR1}"
```

**Solution**: Fix the environment variable definitions

### 7.11.10 Practical Example: Multi-Environment Deployment

A practical example of using verify_files in multi-environment deployment:

```toml
version = "1.0"

[global]
env_allowlist = ["DEPLOY_ENV", "APP_ROOT", "CONFIG_ROOT"]
verify_files = [
    "${CONFIG_ROOT}/${DEPLOY_ENV}/global.yml",
    "${CONFIG_ROOT}/${DEPLOY_ENV}/secrets.enc",
]

[[groups]]
name = "web_servers"
env_allowlist = ["DEPLOY_ENV", "APP_ROOT"]
verify_files = [
    "${APP_ROOT}/web/nginx.conf",
    "${APP_ROOT}/web/ssl/cert.pem",
    "${APP_ROOT}/web/ssl/key.pem",
]

[[groups.commands]]
name = "deploy_web"
cmd = "${APP_ROOT}/scripts/deploy.sh"
args = ["web", "${DEPLOY_ENV}"]
env = [
    "APP_ROOT=/opt/myapp",
    "DEPLOY_ENV=production",
]

[[groups]]
name = "database"
env_allowlist = ["DEPLOY_ENV", "APP_ROOT"]
verify_files = [
    "${APP_ROOT}/db/schema.sql",
    "${APP_ROOT}/db/migrations/${DEPLOY_ENV}",
]

[[groups.commands]]
name = "migrate_db"
cmd = "${APP_ROOT}/scripts/migrate.sh"
args = ["${DEPLOY_ENV}"]
env = [
    "APP_ROOT=/opt/myapp",
    "DEPLOY_ENV=production",
]
```

Environment variable setup example (production environment):
```bash
export DEPLOY_ENV=production
export APP_ROOT=/opt/myapp
export CONFIG_ROOT=/etc/myapp/config
```

This configuration verifies the following files:
- `/etc/myapp/config/production/global.yml`
- `/etc/myapp/config/production/secrets.enc`
- `/opt/myapp/web/nginx.conf`
- `/opt/myapp/web/ssl/cert.pem`
- `/opt/myapp/web/ssl/key.pem`
- `/opt/myapp/db/schema.sql`
- `/opt/myapp/db/migrations/production/`

### 7.11.11 Limitations

1. **Absolute path requirement**: Expanded paths must be absolute paths
2. **System environment variables only**: Command.Env variables cannot be used in verify_files
3. **Expansion timing**: Variables are expanded once at configuration load time (not at execution time)

## Next Steps

In the next chapter, we will introduce practical examples that combine the configurations we have learned so far. You will learn how to create configuration files based on actual use cases.
