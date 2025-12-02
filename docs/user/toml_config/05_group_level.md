# Chapter 5: Group Level Configuration [[groups]]

## Overview

The `[[groups]]` section is a logical unit that groups related commands. Each group can have a name, description, and common settings. The configuration file requires one or more groups.

## 5.1 Basic Group Settings

### 5.1.1 name - Group Name

#### Overview

Specifies a unique name to identify the group.

#### Syntax

```toml
[[groups]]
name = "group_name"
```

#### Parameter Details

| Item | Description #### Inheritance Behavior** | Three modes (described below) |

### 5.3.3 env_vars - Group Environment Variables

#### Overview

Defines environment variables that are commonly used by all commands within that group. Can override global-level environment variables.

#### Syntax

```toml
[[groups]]
name = "example"
env_vars = ["KEY1=value1", "KEY2=value2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group, Command |
| **Default Value** | [] (no environment variables) |
| **Format** | `"KEY=VALUE"` format |
| **Override** | Same-name variables can be overridden at command level |

#### Role

- **Group-Specific Settings**: Define environment variables specific to that group
- **Override Global Settings**: Change global-level environment variables
- **Share Between Commands**: Share settings across all commands in the group

#### Configuration Examples

##### Example 1: Group-Specific Environment Variables

```toml
version = "1.0"

[global]
env_vars = [
    "BASE_DIR=/opt/app",
    "LOG_LEVEL=info",
]
env_allowed = ["HOME"]

[[groups]]
name = "database_group"
env_vars = [
    "DB_HOST=localhost",
    "DB_PORT=5432",
    "DB_DATA=%{BASE_DIR}/db-data",  # References BASE_DIR from Global.env_vars
]

[[groups.commands]]
name = "connect"
cmd = "/usr/bin/psql"
args = ["-h", "%{DB_HOST}", "-p", "%{DB_PORT}"]
# DB_HOST and DB_PORT are obtained from Group.env_vars
```

##### Example 2: Overriding Global Settings

```toml
version = "1.0"

[global]
env_vars = [
    "LOG_LEVEL=info",
    "ENV_TYPE=production",
]

[[groups]]
name = "development_group"
env_vars = [
    "LOG_LEVEL=debug",      # Overrides Global.env_vars LOG_LEVEL
    "ENV_TYPE=development", # Overrides Global.env_vars ENV_TYPE
]

[[groups.commands]]
name = "run_dev"
cmd = "/opt/app/bin/app"
args = ["--log-level", "%{LOG_LEVEL}"]
# LOG_LEVEL=debug is used
```

##### Example 3: Variable References Within Group

```toml
version = "1.0"

[global]
env_vars = ["APP_ROOT=/opt/myapp"]

[[groups]]
name = "web_group"
env_vars = [
    "WEB_DIR=%{APP_ROOT}/web",         # References APP_ROOT from Global.env_vars
    "STATIC_DIR=%{WEB_DIR}/static",    # References WEB_DIR from Group.env_vars
    "UPLOAD_DIR=%{WEB_DIR}/uploads",   # References WEB_DIR from Group.env_vars
]

[[groups.commands]]
name = "start_server"
cmd = "%{WEB_DIR}/server"
args = ["--static", "%{STATIC_DIR}", "--upload", "%{UPLOAD_DIR}"]
```

#### Priority Order

Environment variables are resolved in the following priority order:

1. System environment variables (lowest priority)
2. Global.env_vars (global environment variables)
3. **Group.env_vars** (group environment variables) ← This section
4. Command.env_vars (command environment variables) (highest priority)

```toml
[global]
env_vars = ["SHARED=global", "OVERRIDE=global"]

[[groups]]
name = "example"
env_vars = ["OVERRIDE=group", "GROUP_ONLY=group"]  # Overrides OVERRIDE

[[groups.commands]]
name = "cmd1"
env_vars = ["OVERRIDE=command"]  # Further override

# Runtime environment variables:
# SHARED=global
# OVERRIDE=command
# GROUP_ONLY=group
```

#### Variable Expansion

Within Group.env_vars, you can reference variables defined in Global.env_vars or other variables within the same Group.env_vars.

##### Referencing Global.env_vars Variables

```toml
[global]
env_vars = ["BASE=/opt/app"]

[[groups]]
name = "services"
env_vars = [
    "SERVICE_DIR=%{BASE}/services",     # References BASE from Global.env_vars
    "CONFIG=%{SERVICE_DIR}/config",     # References SERVICE_DIR from Group.env_vars
]
```

##### Referencing System Environment Variables

```toml
[global]
env_allowed = ["HOME", "USER"]

[[groups]]
name = "user_specific"
env_vars = [
    "USER_DATA=${HOME}/%{USER}/data",  # References system environment variables HOME and USER
]
```

#### Precautions

##### 1. KEY Name Constraints

The same constraints as Global.env_vars apply (see Chapter 4).

##### 2. Duplicate Definitions

Defining the same KEY multiple times within the same group results in an error.

##### 3. Relationship with allowlist

When variables defined in Group.env_vars reference system environment variables, the referenced variables must be added to that group's `env_allowed`.

```toml
[global]
env_allowed = ["PATH"]

[[groups]]
name = "example"
env_vars = ["MY_HOME=%{HOME}/app"]  # References HOME
env_allowed = ["HOME"]       # Required: Allow HOME (overrides global)
```

##### 4. Independence Between Groups

Variables defined in Group.env_vars are only valid within that group. They do not affect other groups.

```toml
[[groups]]
name = "group1"
env_vars = ["VAR=value1"]

[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["%{VAR}"]  # value1

[[groups]]
name = "group2"
# VAR not defined in env_vars

[[groups.commands]]
name = "cmd2"
cmd = "/bin/echo"
args = ["%{VAR}"]  # Error: VAR is undefined
```

#### Best Practices

1. **Define Group-Specific Settings**: Put group-specific environment variables in Group.env_vars
2. **Coordination with Global.env_vars**: Base paths in Global.env_vars, derived paths in Group.env_vars
3. **Proper allowlist Settings**: Configure allowlist when referencing system environment variables
4. **Clear Naming**: Use variable names that indicate they are group-specific

```toml
# Recommended configuration
[global]
env_vars = [
    "APP_ROOT=/opt/myapp",
    "ENV_TYPE=production",
]
env_allowed = ["HOME", "PATH"]

[[groups]]
name = "database"
env_vars = [
    "DB_HOST=localhost",              # Group-specific
    "DB_PORT=5432",                   # Group-specific
    "DB_DATA=%{APP_ROOT}/db-data",    # References Global.env_vars
]

[[groups]]
name = "web"
env_vars = [
    "WEB_DIR=%{APP_ROOT}/web",        # References Global.env_vars
    "PORT=8080",                      # Group-specific
]
```

#### Next Steps

- **Command.env_vars**: See Chapter 6 for command-level environment variables
- **Variable Expansion Details**: See Chapter 7 for variable expansion mechanisms
- **Environment Variable Inheritance Modes**: See section 5.4 for allowlist inheritance

## 5.4 Environment Variable Inheritance Modes------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Required |
| **Configurable Level** | Group only |
| **Valid Values** | Alphanumeric characters, underscores, hyphens |
| **Uniqueness** | Must be unique within the configuration file |

#### Role

- **Identification**: Uniquely identifies the group
- **Log Output**: Displays which group is being executed in execution logs
- **Error Reporting**: Identifies which group had issues when errors occur

#### Configuration Example

```toml
version = "1.0"

[[groups]]
name = "database_backup"
# ...

[[groups]]
name = "log_rotation"
# ...

[[groups]]
name = "system_maintenance"
# ...
```

#### Naming Best Practices

```toml
# Recommended: clear, descriptive names
[[groups]]
name = "daily_database_backup"

[[groups]]
name = "weekly_log_cleanup"

# Not recommended: unclear names
[[groups]]
name = "group1"

[[groups]]
name = "temp"
```

### 5.1.2 description - Description

#### Overview

Human-readable text describing the purpose or role of the group.

#### Syntax

```toml
[[groups]]
name = "example"
description = "Group description"
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional (recommended) |
| **Configurable Level** | Group only |
| **Valid Values** | Any string |

#### Role

- **Documentation**: Clarifies the purpose of the group
- **Maintainability**: Makes configuration easier to understand for other developers
- **Log Output**: Displayed during execution to help understand what's being executed

#### Configuration Example

```toml
version = "1.0"

[[groups]]
name = "database_maintenance"
description = "Execute database backup and optimization"

[[groups.commands]]
name = "backup"
description = "Full PostgreSQL database backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]

[[groups.commands]]
name = "vacuum"
description = "Database optimization (VACUUM ANALYZE)"
cmd = "/usr/bin/psql"
args = ["-c", "VACUUM ANALYZE"]
```

## 5.2 Resource Management Settings

### 5.2.1 ❌ temp_dir - Temporary Directory (Deprecated)

#### ⚠️ Deprecation Notice

**This feature has been deprecated.** The `temp_dir` field at the group level is no longer supported.

#### Alternative Methods in the New Specification

The `temp_dir` field has been removed and replaced with a simpler specification:

1. **Automatic Temporary Directory (Default)**: If `workdir` is not specified, a temporary directory is automatically generated
2. **Fixed Directory**: If `workdir` is specified, that fixed directory is used
3. **`__runner_workdir` Variable**: A reserved variable is available to reference the working directory at execution time

#### Migration Example

```toml
# Old specification (will cause an error)
[[groups]]
name = "data_processing"
temp_dir = true  # ❌ This must be removed

# New specification (automatic temporary directory)
[[groups]]
name = "data_processing"
# workdir not specified - a temporary directory is automatically generated

[[groups.commands]]
name = "download_data"
cmd = "wget"
args = ["https://example.com/data.csv", "-O", "%{__runner_workdir}/data.csv"]
# ✅ Reference temporary directory with __runner_workdir variable
```

### 5.2.2 workdir - Working Directory

#### Overview

Specifies the working directory where all commands in the group are executed. If not specified, a temporary directory is automatically generated.

#### Syntax

```toml
[[groups]]
name = "example"
workdir = "directory_path"  # Optional
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | String (string) |
| **Required/Optional** | Optional |
| **Configurable Level** | Group, Command |
| **Default Value** | Automatically generated temporary directory |
| **Valid Values** | Absolute path |
| **Override** | Can be overridden at command level |

#### New Feature: Automatic Temporary Directories

**Default Behavior (Recommended)**: When `workdir` is not specified

```toml
[[groups]]
name = "backup"
# workdir not specified → Temporary directory is automatically generated

[[groups.commands]]
name = "create_backup"
cmd = "tar"
args = ["-czf", "%{__runner_workdir}/backup.tar.gz", "/etc"]
# backup.tar.gz is created in the temporary directory
```

**Temporary Directory Characteristics**:
- Path: `/tmp/scr-<group-name>-<random-string>`
- Permissions: 0700 (accessible only by owner)
- Auto-deletion: Deleted after group execution completes (can be kept with `--keep-temp-dirs` flag)

#### Configuration Examples

**Using a Fixed Directory**:

```toml
[[groups]]
name = "log_analysis"
workdir = "/var/log"  # Specify fixed working directory

[[groups.commands]]
name = "grep_errors"
cmd = "grep"
args = ["ERROR", "app.log"]
# Search from /var/log/app.log
```

**Using Automatic Temporary Directory (Recommended)**:

```toml
[[groups]]
name = "backup"
# workdir not specified → Temporary directory is automatically generated

[[groups.commands]]
name = "create_backup"
cmd = "tar"
args = ["-czf", "%{__runner_workdir}/backup.tar.gz", "/etc"]
# backup.tar.gz is created in the automatically generated temporary directory
```

#### Getting Directory Path at Execution Time

Use the `__runner_workdir` reserved variable to get the working directory path at execution time:

```toml
[[groups]]
name = "data_processing"

[[groups.commands]]
name = "show_workdir"
cmd = "echo"
args = ["Current working directory: %{__runner_workdir}"]

[[groups.commands]]
name = "create_output"
cmd = "touch"
args = ["%{__runner_workdir}/output.txt"]
```

## 5.3 Security Settings

### 5.3.1 verify_files - File Verification (Group Level)

#### Overview

Specifies a group-specific file verification list. Added to the global-level `verify_files` (merged).

#### Syntax

```toml
[[groups]]
name = "example"
verify_files = ["file_path1", "file_path2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group |
| **Default Value** | [] |
| **Valid Values** | List of absolute paths |
| **Merge Behavior** | Merged with global settings |

#### Configuration Example

```toml
version = "1.0"

[global]
verify_files = ["/bin/sh"]  # Verified across all groups

[[groups]]
name = "database_tasks"
verify_files = [
    "/usr/bin/psql",
    "/usr/bin/pg_dump",
]  # In this group, /bin/sh, /usr/bin/psql, /usr/bin/pg_dump are verified

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]

[[groups]]
name = "web_tasks"
verify_files = [
    "/usr/bin/curl",
    "/usr/bin/wget",
]  # In this group, /bin/sh, /usr/bin/curl, /usr/bin/wget are verified

[[groups.commands]]
name = "fetch_data"
cmd = "/usr/bin/curl"
args = ["https://example.com/data"]
```

### 5.3.2 vars - Group Internal Variables

#### Overview

Defines internal variables at group level. Merged with global `vars`, and can be referenced by all commands in the group.

#### Syntax

```toml
[[groups]]
name = "example"
vars = ["var1=value1", "var2=value2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group, Command |
| **Default Value** | [] (no variables) |
| **Format** | `"variable_name=value"` format |
| **Reference Syntax** | `%{variable_name}` |
| **Inheritance Behavior** | Merged with Global.vars (Group takes priority) |

#### Role

- **Group-Specific Settings**: Define internal variables specific to the group
- **Extension of Global vars**: Override or add to global variables
- **Scope Management**: Can be referenced only by commands in the group

#### Configuration Examples

##### Example 1: Overriding Global Variables

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/myapp",
    "log_level=info"
]

[[groups]]
name = "debug_group"
vars = [
    "log_level=debug"  # Override global log_level
]

[[groups.commands]]
name = "run_debug"
cmd = "%{app_dir}/bin/app"
args = ["--log-level", "%{log_level}"]
# Actual execution: /opt/myapp/bin/app --log-level debug
```

##### Example 2: Adding Group-Specific Variables

```toml
version = "1.0"

[global]
vars = ["base_dir=/opt"]

[[groups]]
name = "web_group"
vars = [
    "web_root=%{base_dir}/www",
    "port=8080"
]

[[groups.commands]]
name = "start_web"
cmd = "/usr/bin/nginx"
args = ["-c", "%{web_root}/nginx.conf", "-g", "daemon off;"]
env_vars = ["PORT=%{port}"]
```

##### Example 3: Environment-Specific Configuration

```toml
version = "1.0"

[global]
vars = ["app_dir=/opt/myapp"]

[[groups]]
name = "production"
vars = [
    "env_type=prod",
    "config_file=%{app_dir}/config/%{env_type}.yml",
    "db_host=prod-db.example.com"
]

[[groups.commands]]
name = "run_prod"
cmd = "%{app_dir}/bin/app"
args = ["--config", "%{config_file}", "--db-host", "%{db_host}"]

[[groups]]
name = "development"
vars = [
    "env_type=dev",
    "config_file=%{app_dir}/config/%{env_type}.yml",
    "db_host=localhost"
]

[[groups.commands]]
name = "run_dev"
cmd = "%{app_dir}/bin/app"
args = ["--config", "%{config_file}", "--db-host", "%{db_host}"]
```

### 5.3.3 env_import - System Environment Variable Import (Group Level)

#### Overview

Imports system environment variables as internal variables at group level. Uses **Merge** method — when a group defines `env_import` its entries are merged with Global.env_import (group entries take precedence when names collide).

#### Syntax

```toml
[[groups]]
name = "example"
env_import = ["internal_var_name=SYSTEM_ENV_VAR_NAME", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group |
| **Default Value** | nil (inherits Global.env_import) |
| **Format** | `"internal_var_name=SYSTEM_ENV_VAR_NAME"` format |
| **Inheritance Behavior** | **Merge (union) method** |

#### Inheritance Rules (Merge Method)

| Group.env_import Status | Behavior |
|---------------------|---------|
| **Undefined (nil)** | Inherits Global.env_import |
| **Empty array `[]`** | Inherits Global.env_import |
| **Defined with values** | Global.env_import + Group.env_import are merged (Group wins on name conflicts) |

#### Configuration Examples

##### Example 1: Inheriting Global.env_import

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER"]
env_import = [
    "home=HOME",
    "username=USER"
]

[[groups]]
name = "inherit_group"
# env_import undefined → Inherits Global.env_import

[[groups.commands]]
name = "show_home"
cmd = "/bin/echo"
args = ["Home: %{home}, User: %{username}"]
# home and username are available
```

##### Example 2: Merging with Global.env_import

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER", "PATH"]
env_import = [
    "home=HOME",
    "user=USER"
]

[[groups]]
name = "merge_group"
env_import = [
    "path=PATH"  # Merged with Global.env_import
]

[[groups.commands]]
name = "show_all"
cmd = "/bin/echo"
args = ["Home: %{home}, User: %{user}, Path: %{path}"]
# home, user, and path are all available
```

##### Example 3: Overriding via Merge (same-name)

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER", "HOSTNAME"]
env_import = [
    "home=HOME",
    "user=USER"
]

[[groups]]
name = "override_merge_group"
env_import = [
    "home=CUSTOM_HOME_DIR",  # home is overridden by the group
    "host=HOSTNAME"          # new variable added by the group
]

[[groups.commands]]
name = "show_info"
cmd = "/bin/echo"
args = ["Home: %{home}, User: %{user}, Host: %{host}"]
# home is taken from CUSTOM_HOME_DIR, user from global, host from group
```

##### Example 4: Empty array still inherits Global

```toml
version = "1.0"

[global]
env_allowed = ["HOME"]
env_import = ["home=HOME"]

[[groups]]
name = "empty_merge_group"
env_import = []  # Empty array: Global.env_import is still inherited (merge behavior)

[[groups.commands]]
name = "show_home"
cmd = "/bin/echo"
args = ["Home: %{home}"]
# home is available
```

#### Important Notes

**Merge method benefits**: Groups can add new variables while still inheriting common variables defined at the Global level. This reduces duplication and provides a predictable, consistent inheritance model.

### 5.3.4 env_allowed - Environment Variable Allowlist (Group Level)

#### Overview

Controls the import of system environment variables through `env_import` at group level. Works with **Override(override)** method for allowlist itself.

#### Syntax

```toml
[[groups]]
name = "example"
env_allowed = ["variable1", "variable2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group |
| **Default Value** | nil (inherits Global.env_allowed) |
| **Valid Values** | List of environment variable names, or empty array |
| **Inheritance Behavior** | **Override (replacement) method** |

### 5.3.5 env_vars - Group Process Environment Variables

#### Overview

Defines process environment variables commonly used by all commands within that group. Merged with global-level environment variables and passed to child processes. Internal variables in the form `%{VAR}` can be used in values.

#### Syntax

```toml
[[groups]]
name = "example"
env_vars = ["KEY1=value1", "KEY2=value2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (array of strings) |
| **Required/Optional** | Optional |
| **Configurable Level** | Global, Group, Command |
| **Default Value** | [] (no environment variables) |
| **Format** | `"KEY=VALUE"` format |
| **Variable Expansion in Values** | Can use internal variables `%{VAR}` |
| **Merge Behavior** | Merged with Global.env_vars (Group takes priority) |

#### Role

- **Group-Specific Settings**: Define process environment variables specific to the group
- **Internal Variable Utilization**: Can reference internal variables in `%{VAR}` format
- **Override Global Settings**: Change global-level environment variables
- **Sharing Between Commands**: Share settings across all commands in the group

#### Configuration Examples

##### Example 1: Group-Specific Environment Variables and Internal Variable Utilization

```toml
version = "1.0"

[global]
vars = ["base_dir=/opt/app"]
env_vars = ["LOG_LEVEL=info"]

[[groups]]
name = "database_group"
vars = [
    "db_data=%{base_dir}/db-data"
]
env_vars = [
    "DB_HOST=localhost",
    "DB_PORT=5432",
    "DB_DATA=%{db_data}"  # Reference internal variable
]

[[groups.commands]]
name = "connect"
cmd = "/usr/bin/psql"
args = ["-h", "%{DB_HOST}", "-p", "%{DB_PORT}"]
# DB_HOST and DB_PORT are obtained from Group.env_vars
```

##### Example 2: Overriding Global Settings

```toml
version = "1.0"

[global]
env_vars = [
    "LOG_LEVEL=info",
    "ENV_TYPE=production",
]

[[groups]]
name = "development_group"
env_vars = [
    "LOG_LEVEL=debug",      # Override global LOG_LEVEL
    "ENV_TYPE=development", # Override global ENV_TYPE
]

[[groups.commands]]
name = "run_dev"
cmd = "/opt/app/bin/app"
args = ["--log-level", "%{LOG_LEVEL}"]
# LOG_LEVEL=debug is used
```

##### Example 3: Variable References Within Group

```toml
version = "1.0"

[global]
env_vars = ["APP_ROOT=/opt/myapp"]

[[groups]]
name = "web_group"
env_vars = [
    "WEB_DIR=%{APP_ROOT}/web",         # Reference APP_ROOT from Global.env_vars
    "STATIC_DIR=%{WEB_DIR}/static",    # Reference WEB_DIR from Group.env_vars
    "UPLOAD_DIR=%{WEB_DIR}/uploads",   # Reference WEB_DIR from Group.env_vars
]
```

### 5.3.6 cmd_allowed - Group-Level Command Allowlist

#### Overview

Specifies additional commands that are allowed to execute within this specific group. This allows group-specific tools that are not covered by the hardcoded global patterns.

#### Syntax

```toml
[[groups]]
name = "example"
cmd_allowed = ["/path/to/command1", "%{variable}/command2", ...]
```

#### Parameter Details

| Item | Description |
|------|-------------|
| **Type** | Array of strings (absolute paths) |
| **Required/Optional** | Optional |
| **Configurable Level** | Group only |
| **Default Value** | [] (no additional commands) |
| **Valid Values** | List of absolute paths (variable expansion supported) |
| **Validation** | Paths must exist and be resolvable |

#### Role

- **Group-Specific Permissions**: Allow commands only within specific groups
- **Security Isolation**: Custom tools available only where needed
- **Flexibility**: Combine with global patterns for fine-grained control

#### Relationship with Hardcoded Global Patterns

The following global patterns are hardcoded (not configurable from TOML):
```
^/bin/.*
^/usr/bin/.*
^/usr/sbin/.*
^/usr/local/bin/.*
```

Commands are allowed if they match **EITHER**:
1. Any of the hardcoded global patterns above (regex matching)
2. Any exact path in group `cmd_allowed` (after variable expansion and symlink resolution)

This is an **OR relationship**, not AND.

#### Configuration Examples

##### Example 1: Basic Group-Specific Command

```toml
version = "1.0"

# Global patterns (^/bin/.*, ^/usr/bin/.*, etc.) are hardcoded
# and do not need to be specified in TOML

[[groups]]
name = "custom_build"
# Allow custom tool only in this group
cmd_allowed = ["/opt/myproject/bin/build_tool"]

[[groups.commands]]
name = "run_build"
cmd = "/opt/myproject/bin/build_tool"  # Allowed via cmd_allowed
args = ["--release"]

[[groups.commands]]
name = "run_sh"
cmd = "/bin/sh"  # Allowed via hardcoded global patterns
args = ["-c", "echo 'Build complete'"]
```

##### Example 2: With Variable Expansion

```toml
version = "1.0"

[global]
env_import = ["home=HOME"]
vars = ["tools_dir=/opt/tools"]

[[groups]]
name = "user_scripts"
cmd_allowed = [
    "%{home}/bin/my_script.sh",    # Expands to /home/user/bin/my_script.sh
    "%{tools_dir}/processor",      # Expands to /opt/tools/processor
]

[[groups.commands]]
name = "run_user_script"
cmd = "%{home}/bin/my_script.sh"
args = ["--verbose"]
```

##### Example 3: Multiple Groups with Different Permissions

```toml
version = "1.0"

# Global patterns are hardcoded

[[groups]]
name = "database_admin"
cmd_allowed = [
    "/opt/db-tools/backup",
    "/opt/db-tools/restore",
]

[[groups.commands]]
name = "backup_db"
cmd = "/opt/db-tools/backup"
args = ["--all"]

[[groups]]
name = "web_deploy"
cmd_allowed = [
    "/opt/deploy/push",
    "/opt/deploy/rollback",
]

[[groups.commands]]
name = "deploy_app"
cmd = "/opt/deploy/push"
args = ["--env_vars=production"]

[[groups]]
name = "monitoring"
# No cmd_allowed - only hardcoded global patterns apply

[[groups.commands]]
name = "check_status"
cmd = "/usr/bin/curl"  # Allowed via hardcoded global patterns
args = ["http://localhost/health"]
```

#### Security Features

##### 1. Absolute Paths Required

Relative paths are rejected to prevent path traversal attacks.

```toml
[[groups]]
cmd_allowed = ["./script.sh"]  # Error: relative path not allowed
cmd_allowed = ["../bin/tool"]  # Error: relative path not allowed
cmd_allowed = ["/opt/bin/tool"]  # Correct
```

##### 2. Path Existence Validation

Paths must exist at configuration load time. Non-existent paths cause an error.

```toml
[[groups]]
cmd_allowed = ["/nonexistent/path"]  # Error: path does not exist
```

##### 3. Symlink Resolution

Symbolic links in `cmd_allowed` are resolved to their real paths. Commands are matched against the resolved path.

```toml
# If /usr/local/bin/python -> /usr/bin/python3
[[groups]]
cmd_allowed = ["/usr/local/bin/python"]  # Stored as /usr/bin/python3
```

#### Notes

##### 1. Other Security Checks Still Apply

Even if a command is allowed via `cmd_allowed`, other security validations continue:
- File integrity verification (hash checking)
- Risk assessment
- Privilege validation
- Environment variable validation

##### 2. Variable Expansion Timing
Variables in `cmd_allowed` are expanded during the initial configuration loading and preparation phase, before any commands are executed. This allows the use of variables defined at the global or group level.

##### 3. Best Practices

- **Principle of Least Privilege**: Only add commands that are necessary for the group's purpose
- **Use Variables for Portability**: Use `%{home}` instead of hardcoded paths where appropriate
- **Document Purpose**: Add comments explaining why each command is allowed

```toml
[[groups]]
name = "deployment"
cmd_allowed = [
    "/opt/deploy/push",      # Required for production deployments
    "/opt/deploy/rollback",  # Emergency rollback capability
]
```

## 5.4 Environment Variable Inheritance Modes

The environment variable allowlist (`env_allowed`) has three inheritance modes. This is one of the important features of go-safe-cmd-runner.

### 5.4.1 Inherit Mode (inherit)

#### Behavior

When `env_allowed` is **not specified** at group level, inherits the global setting.

#### Use Case

- When global configuration is sufficient
- When multiple groups use the same environment variables

#### Configuration Example

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "USER"]

[[groups]]
name = "inherit_group"
# env_allowed not specified → inherits global

[[groups.commands]]
name = "show_env"
cmd = "printenv"
args = []
# PATH, HOME, USER are available
```

### 5.4.2 Explicit Mode (explicit)

#### Behavior

When `env_allowed` has **specific values** at group level, ignores global settings and uses only the specified values.

#### Use Case

- When a group-specific set of environment variables is needed
- When different restrictions from global settings are desired

#### Configuration Example

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "USER"]

[[groups]]
name = "explicit_group"
env_allowed = ["PATH", "DATABASE_URL", "API_KEY"]  # Ignores global

[[groups.commands]]
name = "run_app"
cmd = "/opt/app/bin/app"
args = []
env_vars = [
    "DATABASE_URL=postgresql://localhost/mydb",
    "API_KEY=secret123",
]
# Only PATH, DATABASE_URL, API_KEY are available
# HOME, USER are not available
```

### 5.4.3 Reject Mode (reject)

#### Behavior

When `env_allowed = []` (an **empty array**) is explicitly specified at group level, all environment variables are rejected.

#### Use Case

- When executing commands in a completely isolated environment
- When security requirements are very high

#### Configuration Example

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "USER"]

[[groups]]
name = "reject_group"
env_allowed = []  # Reject all environment variables

[[groups.commands]]
name = "isolated_command"
cmd = "/bin/echo"
args = ["Completely isolated execution"]
# Executed without environment variables
```

### 5.4.4 Inheritance Mode Determination Rules

Mode determination follows this logic:

```mermaid
flowchart TD
    A["Check env_allowed"] --> B{"Is env_allowed<br/>defined at<br/>group level?"}
    B -->|No| C["Inherit Mode<br/>inherit"]
    B -->|Yes| D{"Is value<br/>empty array<br/>[]?"}
    D -->|Yes| E["Reject Mode<br/>reject"]
    D -->|No| F["Explicit Mode<br/>explicit"]

    C --> G["Use global<br/>env_allowed"]
    E --> H["Reject all<br/>environment variables"]
    F --> I["Use group's<br/>env_allowed"]

    style C fill:#e8f5e9
    style E fill:#ffebee
    style F fill:#e3f2fd
```

#### Example: Comparison of Three Modes

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "USER"]

# Mode 1: Inherit Mode
[[groups]]
name = "group_inherit"
# env_allowed not specified
# Result: PATH, HOME, USER are available

[[groups.commands]]
name = "test1"
cmd = "printenv"
args = ["HOME"]  # HOME is output

# Mode 2: Explicit Mode
[[groups]]
name = "group_explicit"
env_allowed = ["PATH", "CUSTOM_VAR"]
# Result: Only PATH, CUSTOM_VAR are available (HOME, USER are not)

[[groups.commands]]
name = "test2"
cmd = "printenv"
args = ["HOME"]  # Error: HOME is not allowed

[[groups.commands]]
name = "test3"
cmd = "printenv"
args = ["CUSTOM_VAR"]
env_vars = ["CUSTOM_VAR=value"]  # CUSTOM_VAR is output

# Mode 3: Reject Mode
[[groups]]
name = "group_reject"
env_allowed = []
# Result: All environment variables are rejected

[[groups.commands]]
name = "test4"
cmd = "printenv"
args = ["PATH"]  # Error: PATH is also not allowed
```

### Practical Usage Examples

#### Example 1: Configuration Based on Security Level

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "USER"]

# Normal tasks: Inherit global
[[groups]]
name = "normal_tasks"
# env_allowed not specified → Inherit mode

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup.sh"
args = []

# Sensitive data processing: Minimal environment variables
[[groups]]
name = "sensitive_data"
env_allowed = ["PATH"]  # Allow only PATH → Explicit mode

[[groups.commands]]
name = "process_sensitive"
cmd = "/opt/secure/process"
args = []

# Completely isolated tasks: No environment variables
[[groups]]
name = "isolated_tasks"
env_allowed = []  # Reject all → Reject mode

[[groups.commands]]
name = "isolated_check"
cmd = "/bin/echo"
args = ["Completely isolated"]
```

#### Example 2: Configuration by Environment

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME"]

# Development environment group
[[groups]]
name = "development"
env_allowed = [
    "PATH",
    "HOME",
    "DEBUG_MODE",
    "DEV_DATABASE_URL",
]  # Explicit mode: Add development variables

[[groups.commands]]
name = "dev_server"
cmd = "/opt/app/server"
args = []
env_vars = ["DEBUG_MODE=true", "DEV_DATABASE_URL=postgresql://localhost/dev"]

# Production environment group
[[groups]]
name = "production"
env_allowed = [
    "PATH",
    "PROD_DATABASE_URL",
]  # Explicit mode: Only production variables

[[groups.commands]]
name = "prod_server"
cmd = "/opt/app/server"
args = []
env_vars = ["PROD_DATABASE_URL=postgresql://prod-server/prod"]
```

## Overall Group Configuration Example

Below is a practical example combining group-level settings:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/tmp"
env_allowed = ["PATH", "HOME", "USER"]
verify_files = ["/bin/sh"]

# Group 1: Database backup
[[groups]]
name = "database_backup"
description = "Daily PostgreSQL database backup"
workdir = "/var/backups/db"
verify_files = ["/usr/bin/pg_dump", "/usr/bin/psql"]
env_allowed = ["PATH", "PGDATA", "PGHOST"]

[[groups.commands]]
name = "backup_main_db"
description = "Backup of main database"
cmd = "/usr/bin/pg_dump"
args = ["-U", "postgres", "maindb"]
output_file = "maindb_backup.sql"
timeout = 600

# Group 2: Log rotation
[[groups]]
name = "log_rotation"
description = "Compression and deletion of old log files"
workdir = "/var/log/app"
env_allowed = ["PATH"]  # Explicit mode: PATH only

[[groups.commands]]
name = "compress_old_logs"
cmd = "gzip"
args = ["app.log.1"]

[[groups.commands]]
name = "delete_ancient_logs"
cmd = "find"
args = [".", "-name", "*.log.gz", "-mtime", "+30", "-delete"]

# Group 3: Temporary file processing
[[groups]]
name = "temp_processing"
description = "Data processing in temporary directory"
# workdir not specified - a temporary directory is automatically generated
env_allowed = []  # Reject mode: No environment variables

[[groups.commands]]
name = "create_temp_data"
cmd = "echo"
args = ["Temporary data"]
output_file = "temp_data.txt"

[[groups.commands]]
name = "process_temp_data"
cmd = "cat"
args = ["temp_data.txt"]
```

## Next Steps

The next chapter will provide detailed explanations of command-level configuration (`[[groups.commands]]`). You will learn how to configure the commands that are actually executed in detail.
