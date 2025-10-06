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

## 7.6 Escape Sequences

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

## 7.7 Automatic Environment Variables

### 7.7.1 Overview

The system automatically sets the following environment variables for each command execution:

- **`__RUNNER_DATETIME`**: Execution time (UTC) in YYYYMMDDHHmm.msec format
- **`__RUNNER_PID`**: Process ID of the runner process

These variables can be used in command paths, arguments, and environment variable values just like regular variables.

### 7.7.2 Usage Examples

#### Timestamped Backups

```toml
[[groups.commands]]
name = "backup_with_timestamp"
description = "Create backup with timestamp"
cmd = "/usr/bin/tar"
args = [
    "czf",
    "/backup/data-${__RUNNER_DATETIME}.tar.gz",
    "/data"
]
```

Example execution:
- If execution time is 2025-10-05 14:30:22.123 UTC
- Backup filename: `/backup/data-202510051430.123.tar.gz`

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
Executed at 202510051430.123 by PID 12345
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
- Output file: `/reports/202510051430.123-12345.html`
- Report title: `Report 202510051430.123`

### 7.7.3 DateTime Format

Format specification for `__RUNNER_DATETIME`:

| Part | Description | Example |
|------|-------------|---------|
| YYYY | 4-digit year | 2025 |
| MM | 2-digit month (01-12) | 10 |
| DD | 2-digit day (01-31) | 05 |
| HH | 2-digit hour (00-23, UTC) | 14 |
| mm | 2-digit minute (00-59) | 30 |
| SS | 2-digit minute (00-59) | 45 |
| .msec | 3-digit milliseconds (000-999) | .123 |

Complete example: `20251005143045.123` = October 5, 2025 14:30:45.123 (UTC)

**Note**: The timezone is always UTC, not local timezone.

### 7.7.4 Reserved Prefix

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

### 7.7.5 Timing of Variable Generation

Automatic environment variables (`__RUNNER_DATETIME` and `__RUNNER_PID`) are generated once when the configuration file is loaded, not at each command execution time. All commands in all groups share the exact same values throughout the entire runner execution.

```toml
[[groups]]
name = "backup_group"

[[groups.commands]]
name = "backup_db"
cmd = "/usr/bin/pg_dump"
args = ["-f", "/backup/db-${__RUNNER_DATETIME}.sql", "mydb"]

[[groups.commands]]
name = "backup_files"
cmd = "/usr/bin/tar"
args = ["czf", "/backup/files-${__RUNNER_DATETIME}.tar.gz", "/data"]
```

**Key Point**: Both commands use the exact same timestamp because `__RUNNER_DATETIME` is sampled at config load time, not at execution time:
- `/backup/db-202510051430.123.sql`
- `/backup/files-202510051430.123.tar.gz`

This ensures consistency across all commands in a single runner execution, even if commands are executed at different times or in different groups.

## 7.8 Security Considerations

### 7.8.1 Command.Env Priority

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

### 7.8.2 Relationship with env_allowlist

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

### 7.8.3 Absolute Path Requirements

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

### 7.8.4 Handling Sensitive Information

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

### 7.8.5 Isolation Between Commands

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

## 7.9 Troubleshooting

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

## Next Steps

In the next chapter, we will introduce practical examples that combine the configurations we have learned so far. You will learn how to create configuration files based on actual use cases.
