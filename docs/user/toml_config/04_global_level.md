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
| **Type** | Integer (int) or null |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Command |
| **Default Value** | 60 seconds (if not specified) |
| **Valid Values** | 0 (unlimited), positive integer (in seconds) |
| **Override** | Can be overridden at command level |

### ⚠️ Important Note

`timeout = 0` means **unlimited execution** (no timeout).

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

#### Example 3: Unlimited Execution

```toml
version = "1.0"

[global]
timeout = 60  # Default: 60 seconds for most commands

[[groups]]
name = "data_processing"

[[groups.commands]]
name = "interactive_setup"
cmd = "/usr/bin/interactive-setup"
args = []
timeout = 0  # ✅ Unlimited execution - no timeout

[[groups.commands]]
name = "large_data_import"
cmd = "/usr/bin/import-data"
args = ["--large-dataset"]
timeout = 0  # ✅ Unlimited execution for long-running task
```

### Timeout Resolution Hierarchy

Timeout values are resolved in the following order of priority:
1. **Command level**: `[[groups.commands]]` timeout setting
2. **Global level**: `[global]` timeout setting
3. **System default**: 60 seconds (used if neither global nor command timeout is specified)

#### Example of Hierarchy Resolution

```toml
version = "1.0"

[global]
timeout = 120  # Global default: 120 seconds

[[groups]]
name = "tasks"

[[groups.commands]]
name = "command1"
cmd = "/bin/sleep"
args = ["30"]
# Uses global timeout: 120 seconds

[[groups.commands]]
name = "command2"
cmd = "/bin/sleep"
args = ["30"]
timeout = 0  # Command-specific: unlimited (overrides global)

[[groups.commands]]
name = "command3"
cmd = "/bin/sleep"
args = ["30"]
timeout = 60  # Command-specific: 60 seconds (overrides global)
```

### Behavior Details

#### For Limited Timeout (positive integer)
When a timeout occurs:
1. Sends termination signal (SIGTERM) to the running command
2. After waiting for a certain period, sends forced termination signal (SIGKILL)
3. Records as error and proceeds to the next command

#### For Unlimited Timeout (`timeout = 0`)
When unlimited timeout is set:
1. **No automatic termination**: Command runs until completion
2. **Resource monitoring**: System monitors long-running processes
3. **Security logging**: Logs unlimited execution for security audit
4. **Manual interruption**: Can be stopped by user intervention (Ctrl+C)

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

#### 2. Unlimited Execution with `timeout = 0`

```toml
[global]
timeout = 0  # ✅ Unlimited execution (no timeout)
```

**Use cases for unlimited timeout:**
- Long-running data processing tasks
- Interactive commands requiring user input
- System maintenance scripts with unpredictable duration

⚠️ **Security considerations**: Use unlimited timeouts carefully to avoid resource exhaustion.

#### 3. Invalid Values

```toml
[global]
timeout = -1  # ❌ Invalid: negative values are not allowed
```

## 4.2 skip_standard_paths - Skip Standard Path Verification

### Overview

Skips file verification for standard system paths (`/bin`, `/usr/bin`, etc.).

### Syntax

```toml
[global]
verify_standard_paths = false/false
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
verify_standard_paths = false  # Skip verification of /bin, /usr/bin, etc.

[[groups]]
name = "system_commands"

[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"  # Can execute without verification
args = ["-la"]
```

#### Example 2: Verify Standard Paths (Default)

```toml
version = "1.0"

[global]
verify_standard_paths = true  # Or omit
verify_files = ["/etc/app/config.ini"]  # Add additional configuration file to verify

[[groups]]
name = "verified_commands"

[[groups.commands]]
name = "search"
cmd = "/usr/bin/grep"  # Standard path but still verified
args = ["pattern", "file.txt"]
# Both /usr/bin/grep and /etc/app/config.ini are verified
```

### Security Notice

Setting `verify_standard_paths = false` will not detect tampering of commands in standard paths. For environments with high security requirements, it is recommended to keep it as `false` (default).

## 4.3 vars - Global Internal Variables

### Overview

Defines internal variables for expansion within the TOML configuration file. Internal variables defined here can be referenced by all groups and commands. By default, internal variables are not passed as environment variables to child processes.

### Syntax

```toml
[global]
vars = ["var1=value1", "var2=value2", ...]
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group, Command |
| **Default Value** | [] (no variables) |
| **Format** | `"variable_name=value"` format |
| **Reference Syntax** | `%{variable_name}` |
| **Scope** | Global vars can be referenced from all groups and commands |

### Role

- **TOML Expansion Only**: Expands values in `cmd`, `args`, `env`, and `verify_files`
- **Enhanced Security**: Separates from environment variables passed to child processes
- **Configuration Reuse**: Centrally manage common values
- **Dynamic Path Building**: Build directory paths dynamically

### Configuration Examples

#### Example 1: Basic Internal Variable Definition

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/myapp",
    "config_file=%{app_dir}/config.yml"
]

[[groups]]
name = "app_group"

[[groups.commands]]
name = "show_config"
cmd = "/bin/cat"
args = ["%{config_file}"]
# Actual execution: /bin/cat /opt/myapp/config.yml
```

#### Example 2: Nested Variable References

```toml
version = "1.0"

[global]
vars = [
    "base=/opt",
    "app_root=%{base}/myapp",
    "bin_dir=%{app_root}/bin",
    "data_dir=%{app_root}/data",
    "log_dir=%{app_root}/logs"
]

[[groups]]
name = "deployment"

[[groups.commands]]
name = "start_app"
cmd = "%{bin_dir}/server"
args = ["--data", "%{data_dir}", "--log", "%{log_dir}"]
# Actual execution: /opt/myapp/bin/server --data /opt/myapp/data --log /opt/myapp/logs
```

#### Example 3: Combining Internal Variables and Process Environment Variables

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/myapp",
    "config_path=%{app_dir}/config.yml"
]
env_vars = [
    "APP_HOME=%{app_dir}",           # Define process environment variable using internal variable
    "CONFIG_FILE=%{config_path}"     # Pass config file path as environment variable
]

[[groups.commands]]
name = "run_app"
cmd = "%{app_dir}/bin/app"
args = ["--config", "%{config_path}"]
# Child process receives CONFIG_FILE environment variable and command-line argument --config /opt/myapp/config.yml
# app_dir, config_path internal variables are not passed as environment variables
```

### Variable Naming Rules

Internal variable names must follow these rules:

- **POSIX Compliance**: Format `[a-zA-Z_][a-zA-Z0-9_]*`
- **Recommended**: Use lowercase and underscores (e.g., `app_dir`, `config_file`)
- **Uppercase Allowed**: Uppercase letters can be used, but lowercase is recommended
- **Reserved Prefix Prohibited**: Names starting with `__runner_` cannot be used

```toml
[global]
vars = [
    "app_dir=/opt/app",        # Correct: lowercase and underscores
    "logLevel=info",           # Correct: camelCase
    "APP_ROOT=/opt",           # Correct: uppercase allowed
    "_private=/tmp",           # Correct: starts with underscore
    "var123=value",            # Correct: contains numbers
    "__runner_var=value",      # Error: reserved prefix
    "123invalid=value",        # Error: starts with number
    "my-var=value"             # Error: hyphens not allowed
]
```

### Precautions

#### 1. Internal Variables Are Not Passed to Child Processes

Variables defined in `vars` are not passed as environment variables to child processes by default:

```toml
[global]
vars = ["secret_key=abc123"]

[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["secret_key"]
# Output: (empty string) (secret_key is not passed as environment variable)
```

To pass to child process, explicitly define in `env` field:

```toml
[global]
vars = ["secret_key=abc123"]
env_vars = ["SECRET_KEY=%{secret_key}"]  # Define process environment variable using internal variable

[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["SECRET_KEY"]
# Output: abc123
```

#### 2. Circular References Prohibited

Creating circular references between variables results in an error:

```toml
[global]
vars = [
    "var1=%{var2}",
    "var2=%{var1}"  # Error: circular reference
]
```

#### 3. Undefined Variable References

Referencing undefined variables results in an error:

```toml
[global]
vars = ["app_dir=/opt/app"]

[[groups.commands]]
name = "test"
cmd = "%{undefined_var}/tool"  # Error: undefined_var is not defined
```

### Best Practices

1. **Centralize Path Management**: Define application root paths and similar values in vars
2. **Lowercase Recommended**: Use lowercase and underscores for internal variable names
3. **Hierarchical Structure**: Build hierarchical paths using nested variable references
4. **Security**: Manage sensitive information in vars and expose via env only when necessary

## 4.4 env_import - System Environment Variable Import

### Overview

Explicitly imports system environment variables as internal variables. Imported variables can be referenced as internal variables using `%{variable_name}`.

### Syntax

```toml
[global]
env_import = ["internal_var_name=SYSTEM_ENV_VAR_NAME", ...]
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group |
| **Default Value** | [] (no imports) |
| **Format** | `"internal_var_name=SYSTEM_ENV_VAR_NAME"` format |
| **Security Constraint** | Only variables included in `env_allowed` can be imported |

### Role

- **Explicit Import**: Intentionally import system environment variables
- **Name Mapping**: Reference system environment variables with different names
- **Enhanced Security**: Control with allowlist in combination

### Configuration Examples

#### Example 1: Basic System Environment Variable Import

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER"]
env_import = [
    "home=HOME",
    "username=USER"
]
vars = [
    "config_file=%{home}/.myapp/config.yml"
]

[[groups.commands]]
name = "show_config"
cmd = "/bin/cat"
args = ["%{config_file}"]
# When HOME=/home/alice: /bin/cat /home/alice/.myapp/config.yml
```

#### Example 2: Path Extension

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME"]
env_import = [
    "user_path=PATH",
    "home=HOME"
]
vars = [
    "custom_bin=%{home}/bin",
    "extended_path=%{custom_bin}:%{user_path}"
]

[[groups.commands]]
name = "run_tool"
cmd = "/bin/sh"
args = ["-c", "echo Path: %{extended_path}"]
env_vars = ["PATH=%{extended_path}"]
```

#### Example 3: Environment-Specific Configuration

```toml
version = "1.0"

[global]
env_allowed = ["APP_ENV"]
env_import = ["environment=APP_ENV"]
vars = [
    "config_dir=/etc/myapp/%{environment}",
    "log_level=%{environment}"  # Log level depends on environment
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_dir}/app.yml", "--log-level", "%{log_level}"]
# When APP_ENV=production: --config /etc/myapp/production/app.yml --log-level production
```

### Security Constraint

System environment variables referenced in `env_import` must be included in `env_allowed`:

```toml
[global]
env_allowed = ["HOME"]
env_import = [
    "home=HOME",    # OK: HOME is in allowlist
    "path=PATH"     # Error: PATH is not in allowlist
]
```

Error message example:
```
system environment variable 'PATH' (mapped to 'path' in global.env_import) is not in env_allowed: [HOME]
```

### Variable Name Mapping

Different names can be used for left side (internal variable name) and right side (system environment variable name):

```toml
[global]
env_allowed = ["HOME", "USER", "HOSTNAME"]
env_import = [
    "user_home=HOME",       # Reference HOME as user_home
    "current_user=USER",    # Reference USER as current_user
    "host=HOSTNAME"         # Reference HOSTNAME as host
]

[[groups.commands]]
name = "info"
cmd = "/bin/echo"
args = ["User: %{current_user}, Home: %{user_home}, Host: %{host}"]
```

### Precautions

#### 1. When Environment Variable Does Not Exist

If a system environment variable does not exist, a warning is displayed and empty string is set:

```toml
[global]
env_allowed = ["NONEXISTENT_VAR"]
env_import = ["var=NONEXISTENT_VAR"]
# Warning: System environment variable 'NONEXISTENT_VAR' is not set
# var is set to empty string
```

#### 2. Variable Naming Convention

Internal variable names (left side) must follow POSIX naming convention:

```toml
[global]
env_allowed = ["HOME"]
env_import = [
    "home=HOME",            # Correct
    "user_home=HOME",       # Correct
    "HOME=HOME",            # Correct (uppercase allowed)
    "__runner_home=HOME",   # Error: reserved prefix
    "123home=HOME",         # Error: starts with number
    "my-home=HOME"          # Error: hyphens not allowed
]
```

### Best Practices

1. **Lowercase Recommended**: Use lowercase and underscores for internal variable names (e.g., `home`, `user_path`)
2. **Explicit Import**: Import only necessary environment variables explicitly
3. **Use with Allowlist**: Import only variables allowed in env_allowed
4. **Clear Naming**: Use names that clearly distinguish between system environment variable names and internal variable names

## 4.5 env_vars - Global Process Environment Variables

### Overview

Defines process environment variables that are commonly used across all groups and commands. Environment variables defined here are passed to child processes of all commands. Internal variables in the form `%{VAR}` can be used in values.

### Syntax

```toml
[global]
env_vars = ["KEY1=value1", "KEY2=value2", ...]
```

### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group, Command |
| **Default Value** | [] (no environment variables) |
| **Format** | `"KEY=VALUE"` format |
| **Variable Expansion in Values** | Can use internal variables `%{VAR}` |
| **Override** | Same-name variables can be overridden at group/command level |

### Role

- **Child Process Environment Variable Setting**: Passed to child processes when executing commands
- **Internal Variable Utilization**: Can reference internal variables in `%{VAR}` format
- **Centralized Configuration**: Manage common environment variables in one place
- **Enhanced Maintainability**: Reduce modification points when changes are needed

### Configuration Examples

#### Example 1: Basic Process Environment Variables

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/app",
    "log_level=info"
]
env_vars = [
    "APP_HOME=%{app_dir}",
    "LOG_LEVEL=%{log_level}",
    "CONFIG_FILE=%{app_dir}/config.yaml"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/app/bin/app"
args = []
# Child process receives APP_HOME, LOG_LEVEL, CONFIG_FILE environment variables
```

#### Example 2: Path Construction Using Internal Variables

```toml
version = "1.0"

[global]
vars = [
    "base=/opt",
    "app_root=%{base}/myapp",
    "data_dir=%{app_root}/data"
]
env_vars = [
    "APP_ROOT=%{app_root}",
    "DATA_PATH=%{data_dir}",
    "BIN_PATH=%{app_root}/bin"
]

[[groups.commands]]
name = "start_app"
cmd = "%{app_root}/bin/server"
args = []
# Child process receives APP_ROOT, DATA_PATH, BIN_PATH
```

#### Example 3: Combination with System Environment Variables

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER"]
env_import = [
    "home=HOME",
    "username=USER"
]
vars = [
    "log_dir=%{home}/logs"
]
env_vars = [
    "USER_NAME=%{username}",
    "LOG_DIRECTORY=%{log_dir}"
]

[[groups.commands]]
name = "log_info"
cmd = "/bin/sh"
args = ["-c", "echo USER_NAME=$USER_NAME, LOG_DIRECTORY=$LOG_DIRECTORY"]
# Child process receives USER_NAME and LOG_DIRECTORY environment variables
```

### Priority and Merging

Environment variables are merged in the following order:

1. Global.env (global environment variables)
2. Merged with Group.env (group environment variables, see Chapter 5)
3. Merged with Command.env (command environment variables, see Chapter 6)

When the same environment variable is defined at multiple levels, the more specific level (Command > Group > Global) takes priority:

```toml
[global]
vars = ["base=global_value"]
env_vars = [
    "COMMON_VAR=%{base}",
    "GLOBAL_ONLY=from_global"
]

[[groups]]
name = "example"
vars = ["base=group_value"]
env_vars = ["COMMON_VAR=%{base}"]    # Overrides Global.env

[[groups.commands]]
name = "cmd1"
vars = ["base=command_value"]
env_vars = ["COMMON_VAR=%{base}"]    # Overrides Group.env

# Runtime environment variables:
# COMMON_VAR=command_value (command level takes priority)
# GLOBAL_ONLY=from_global (global only)
```

### Relationship with Internal Variables

- **env values**: Can use internal variables `%{VAR}`
- **Propagation to Child Processes**: Environment variables defined in env are passed to child processes
- **Internal Variables Not Propagated**: Internal variables defined in vars or env_import are not passed to child processes by default

```toml
[global]
vars = ["internal_value=secret"]     # Internal variable only
env_vars = ["PUBLIC_VAR=%{internal_value}"]  # Define process environment variable using internal variable

[[groups.commands]]
name = "test"
cmd = "/bin/sh"
args = ["-c", "echo $PUBLIC_VAR"]
# Child process receives PUBLIC_VAR environment variable with "secret" value
```

### KEY Name Constraints

Environment variable names (KEY part) must follow these rules:

```toml
[global]
vars = ["internal=value"]
env_vars = [
    "VALID_NAME=value",      # Correct: uppercase letters, numbers, underscores
    "MY_VAR_123=value",      # Correct
    "123INVALID=value",      # Error: starts with number
    "MY-VAR=value",          # Error: hyphens not allowed
    "__RUNNER_VAR=value",    # Error: reserved prefix
]
```

### Duplicate Definitions

Defining the same KEY multiple times results in an error:

```toml
[global]
env_vars = [
    "VAR=value1",
    "VAR=value2",  # Error: duplicate definition
]
```

### Best Practices

1. **Hierarchical Definition**: Define base paths first, then reference them for derived paths
2. **Proper Allowlist Settings**: Always add to allowlist when referencing system environment variables
3. **Configuration Reuse**: Leverage vars and env_import to avoid hardcoding values
4. **Clear Variable Names**: Use descriptive names for environment variables

```toml
# Recommended configuration
[global]
env_allowed = ["HOME", "PATH"]
env_import = ["home=HOME"]
vars = [
    "app_root=/opt/myapp",
    "data_dir=%{app_root}/data",
    "log_dir=%{app_root}/logs"
]
env_vars = [
    "APP_ROOT=%{app_root}",
    "DATA_DIR=%{data_dir}",
    "LOG_DIR=%{log_dir}",
    "HOME=%{home}"
]
```

## 4.6 env_allowed - Environment Variable Allowlist

### Overview

Specifies environment variables allowed to be used during command execution. All environment variables not in the list are excluded.

### Syntax

```toml
[global]
env_allowed = ["variable1", "variable2", ...]
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
env_allowed = [
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
env_allowed = [
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
args = ["--config", "%{APP_CONFIG_DIR}/config.yaml"]
env_vars = ["APP_CONFIG_DIR=/etc/myapp"]
```

#### Example 3: Empty List (Deny All)

```toml
version = "1.0"

[global]
env_allowed = []  # Deny all environment variables

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
env_allowed = [
    "PATH", "HOME", "USER", "SHELL", "EDITOR", "PAGER",
    "MAIL", "LOGNAME", "HOSTNAME", "DISPLAY", "XAUTHORITY",
    # ... too many
]

# Recommended: minimal necessary
[global]
env_allowed = ["PATH", "HOME", "USER"]
```

## 4.7 verify_files - File Verification List

### Overview

Specifies a list of additional files to verify for integrity before execution. The specified files are checked against hash values, and execution is aborted if tampering is detected.

**Important**: Command executables specified in `cmd` fields are automatically included in hash verification. Use `verify_files` to add additional files (configuration files, script files, etc.) beyond the commands themselves.

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

#### Example 1: Additional File Verification

```toml
version = "1.0"

[global]
verify_files = [
    "/opt/app/config/app.conf",
    "/opt/app/scripts/deploy.sh",
]

[[groups]]
name = "deployment"

[[groups.commands]]
name = "deploy"
cmd = "/opt/app/scripts/deploy.sh"  # This file is automatically verified
args = []
# Both /opt/app/scripts/deploy.sh and /opt/app/config/app.conf are verified before execution
```

#### Example 2: Additional Verification at Group Level

```toml
version = "1.0"

[global]
verify_files = ["/etc/app/global.conf"]  # Configuration file verified across all groups

[[groups]]
name = "database_group"
verify_files = ["/etc/app/db.conf"]  # Group-specific configuration file

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"  # This command is automatically verified
args = ["mydb"]
# /usr/bin/pg_dump (automatic), /etc/app/global.conf, /etc/app/db.conf are all verified (merged)
```

### Verification Mechanism

1. **Collect Automatic Verification Targets**: Automatically add command executables specified in `cmd` fields to verification targets
2. **Add Additional Files**: Add files listed in `verify_files` to verification targets
3. **Pre-create Hash Files**: Record file hashes using the `record` command
4. **Execution-time Verification**: Verify hashes of all collected files
5. **Behavior on Mismatch**: Abort execution and report error if hashes don't match

### How to Create Hash Files

```bash
# Specify files individually
$ record /opt/app/config/app.conf /opt/app/scripts/deploy.sh
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

#### 3. Security Best Practices

- **Commands are Automatically Verified**: All commands (`cmd`) are automatically verified, so they don't need to be added to `verify_files`
- **Verify Additional Files**: Add configuration files, script files, and libraries referenced by commands to `verify_files`
- **Performance**: File hash verification operates efficiently with minimal performance impact
- **Tampering Detection**: Increasing verification targets enhances protection against system compromise

## 4.8 Global Command Allowlist (Hardcoded)

### Overview

**IMPORTANT**: The global command allowlist is **hardcoded** in the program for security reasons and **cannot be configured** via the TOML file.

The following regex patterns are used to determine which commands are allowed to execute globally:

```
^/bin/.*
^/usr/bin/.*
^/usr/sbin/.*
^/usr/local/bin/.*
```

### Security Rationale

The global command allowlist is hardcoded to prevent security misconfigurations. Allowing users to weaken these restrictions through configuration could:

- Enable execution of commands from untrusted locations
- Bypass security controls
- Increase attack surface

### Extending Allowed Commands

For commands outside the hardcoded patterns, use **group-level `cmd_allowed`** (see Chapter 5.3.6):

```toml
[global]
env_import = ["home=HOME"]

[[groups]]
name = "custom_tools"
# Allow additional commands for this group only
cmd_allowed = [
    "%{home}/bin/custom_tool",
    "/opt/myapp/bin/processor",
]

[[groups.commands]]
name = "run_custom"
cmd = "%{home}/bin/custom_tool"  # Allowed via group-level cmd_allowed
args = ["--verbose"]

[[groups.commands]]
name = "run_sh"
cmd = "/bin/sh"  # Allowed via hardcoded global patterns
args = ["-c", "echo hello"]
```

### Command Validation Logic

Commands are allowed if they match **EITHER**:

1. **Hardcoded global patterns** (applies to all groups), **OR**
2. **Group-level `cmd_allowed`** (applies to specific group only)

See Chapter 5.3.6 for details on group-level `cmd_allowed`.

### Notes

#### 1. Pattern Matching

The hardcoded patterns are matched against the full absolute path of the command after resolving symlinks.

#### 2. Security Best Practices

- **Use Anchored Patterns**: Always use `^` at the start to anchor patterns
- **Be Specific**: Prefer specific patterns over broad ones
- **Combine with cmd_allowed**: Use group-level `cmd_allowed` for exceptions rather than broadening global patterns

## 4.9 output_size_limit - Maximum Output Size

### Overview

Specifies the maximum size in bytes when capturing command standard output.

### Syntax

```toml
[global]
output_size_limit = bytes
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
output_size_limit = 1048576  # 1MB = 1024 * 1024 bytes

[[groups]]
name = "log_analysis"

[[groups.commands]]
name = "grep_logs"
cmd = "grep"
args = ["ERROR", "/var/log/app.log"]
output_file = "errors.txt"
# Error if output exceeds 1MB
```

#### Example 2: Processing Large Files

```toml
version = "1.0"

[global]
output_size_limit = 104857600  # 100MB = 100 * 1024 * 1024 bytes

[[groups]]
name = "data_export"

[[groups.commands]]
name = "export_database"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
output_file = "database_dump.sql"
# Allow large database dumps
```

#### Example 3: Size Limit Guidelines

```toml
[global]
# Recommended values based on common use cases
output_size_limit = 1048576      # 1MB  - log analysis, small-scale data
output_size_limit = 10485760     # 10MB - default, medium-scale data
output_size_limit = 104857600    # 100MB - large-scale data, database dumps
output_size_limit = 1073741824   # 1GB  - very large data (caution required)
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
output_size_limit = 1024  # 1KB - insufficient for most commands

# Recommended: appropriate limit
[global]
output_size_limit = 10485760  # 10MB - appropriate for general use
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
verify_standard_paths = false

# Environment variable allowlist
env_allowed = [
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
output_size_limit = 10485760  # 10MB

[[groups]]
name = "application_tasks"
# ... group configuration continues
```

## Next Steps

The next chapter will provide detailed explanations of group-level configuration (`[[groups]]`). You will learn how to group commands and configure group-specific settings.
