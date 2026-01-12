# Chapter 7: Command Template Feature

## 7.1 Overview of Command Templates

The command template feature allows you to manage common command definitions in a single location and reuse them across multiple groups.

### Background and Purpose

Repeating the same command definitions across multiple groups creates several problems:

1. **Reduced Maintainability**: The same command definition must be modified in multiple places
2. **Lack of Consistency**: Copy errors can lead to inconsistencies in command definitions across groups
3. **Reduced Readability**: Configuration files become redundant, making it difficult to see essential differences

```toml
# Without template feature (redundant)
[[groups]]
name = "group1"
[[groups.commands]]
name = "restic_prune"
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]

[[groups]]
name = "group2"
[[groups.commands]]
name = "restic_prune"
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]
```

Using the template feature, you can consolidate common command definitions in one place and reference them from each group with different parameters:

```toml
# Template definition
[command_templates.restic_prune]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]

# Use template in groups
[[groups]]
name = "group1"
[[groups.commands]]
name = "restic_prune"
template = "restic_prune"

[[groups]]
name = "group2"
[[groups.commands]]
name = "restic_prune"
template = "restic_prune"
```

### Main Advantages

1. **DRY Principle**: Don't repeat the same definition
2. **Improved Maintainability**: Changes are consolidated in one place
3. **Ensured Consistency**: The same definition is used across all groups
4. **Improved Readability**: Only group-specific settings (parameters) are made explicit
5. **Parameterization**: Different values can be specified for each group while maintaining common parts

## 7.2 Template Definition

### 7.2.1 Basic Syntax

Templates are defined in the `[command_templates.<template_name>]` section.

```toml
[command_templates.<template_name>]
cmd = "<command>"
args = ["<arg1>", "<arg2>", ...]
# Other command execution related fields
```

### 7.2.2 Available Fields

Most execution-related fields available in `[[groups.commands]]` can be specified in template definitions.

| Field | Description | Required |
|-------|-------------|----------|
| `cmd` | Path of the command to execute | Required |
| `args` | Array of arguments to pass to the command | Optional |
| `env_vars` | Array of environment variables | Optional |
| `workdir` | Working directory | Optional |
| `timeout` | Timeout (seconds) | Optional |
| `run_as_user` | User to run as | Optional |
| `run_as_group` | Group to run as | Optional |
| `risk_level` | Risk level | Optional |
| `output_file` | Output file | Optional |

### 7.2.3 Prohibited Fields

The following fields cannot be used in template definitions:

| Field | Reason |
|-------|--------|
| `name` | Command name is specified by the caller |
| `template` | Nesting templates (referencing another template from a template) is prohibited |

### 7.2.4 Template Naming Rules

Template names must follow these rules:

- Must start with a letter or underscore
- Can only use letters, digits, and underscores
- Names starting with `__` (two underscores) are reserved

```toml
# Valid template names
[command_templates.restic_backup]
[command_templates.daily_cleanup]
[command_templates._internal_task]

# Invalid template names
[command_templates.123_task]        # Starts with digit
[command_templates.my-template]     # Hyphen not allowed
[command_templates.__reserved]      # Reserved prefix
```

### 7.2.5 Configuration Examples

#### Example 1: Simple Template

```toml
[command_templates.disk_check]
cmd = "/bin/df"
args = ["-h"]
timeout = 30
risk_level = "low"
```

#### Example 2: Template with Multiple Arguments

```toml
[command_templates.restic_forget]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]
timeout = 3600
risk_level = "medium"
```

## 7.3 Parameter Expansion

By defining parameters in templates and passing values when calling them, flexible command definitions are possible.

### 7.3.1 Types of Parameter Expansion

go-safe-cmd-runner provides three types of parameter expansion syntax:

| Notation | Name | Use Case | Behavior When Empty |
|----------|------|----------|---------------------|
| `${param}` | String Parameter | Required string value | Preserved as empty string |
| `${?param}` | Optional Parameter | Optional string value | Element removed |
| `${@list}` | Array Parameter | Expand multiple values | Nothing added |

### 7.3.2 String Parameter Expansion `${param}`

The `${param}` in a template is replaced with the specified string value.

```toml
[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]

[[groups.commands]]
name = "backup_data"
template = "backup"
params.path = "/data/important"
# Result: args = ["backup", "/data/important"]
```

**Characteristics**:
- Even if the parameter is an empty string `""`, it is preserved as an array element
- Suitable for required parameters

```toml
[[groups.commands]]
name = "backup_empty"
template = "backup"
params.path = ""
# Result: args = ["backup", ""]  ← Empty string passed as argument
```

### 7.3.3 Optional Parameter Expansion `${?param}`

The `${?param}` in a template removes the element from the array if it's an empty string.

```toml
[command_templates.backup_with_option]
cmd = "restic"
args = ["backup", "${?verbose_flag}", "${path}"]

# When verbose_flag is specified
[[groups.commands]]
name = "backup_verbose"
template = "backup_with_option"
params.verbose_flag = "--verbose"
params.path = "/data"
# Result: args = ["backup", "--verbose", "/data"]

# When verbose_flag is empty
[[groups.commands]]
name = "backup_quiet"
template = "backup_with_option"
params.verbose_flag = ""
params.path = "/data"
# Result: args = ["backup", "/data"]  ← "--verbose" removed
```

**Characteristics**:
- Suitable for optional flags and options
- Can remove elements with empty string

### 7.3.4 Array Parameter Expansion `${@list}`

The `${@list}` in a template is expanded with all elements of the array.

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${path}"]

# Specify multiple flags
[[groups.commands]]
name = "backup_debug"
template = "restic_backup"
params.verbose_flags = ["-v", "-v", "--no-cache"]
params.path = "/data"
# Result: args = ["-v", "-v", "--no-cache", "backup", "/data"]

# No flags (empty array)
[[groups.commands]]
name = "backup_silent"
template = "restic_backup"
params.verbose_flags = []
params.path = "/data"
# Result: args = ["backup", "/data"]  ← Nothing added at verbose_flags position
```

**Characteristics**:
- Multiple flags and options can be specified at once
- Nothing added with empty array `[]`

### 7.3.5 Parameter Naming Rules

Parameter names follow the same rules as variable names:

- Must start with a letter or underscore
- Can only use letters, digits, and underscores
- `__runner_` prefix is reserved

```toml
# Valid parameter names
params.backup_path = "/data"
params.verbose_level = "2"
params._internal = "value"

# Invalid parameter names
params.123path = "/data"           # Starts with digit
params.backup-path = "/data"       # Hyphen not allowed
params.__runner_test = "value"     # Reserved prefix
```

## 7.4 Using Templates

### 7.4.1 Basic Usage

Reference a template by specifying the `template` field in `[[groups.commands]]`.

```toml
[[groups.commands]]
name = "<command_name>"      # Required
template = "<template_name>"
params.<param1> = "<value1>"
params.<param2> = ["<value2a>", "<value2b>"]
```

### 7.4.2 Required Fields

| Field | Description |
|-------|-------------|
| `name` | Command name (unique within group) |
| `template` | Template name to reference |

### 7.4.3 Exclusive Fields

When `template` is specified, the following fields cannot be specified (will result in an error):

- `cmd`
- `args`
- `env_vars`
- `workdir`
- `timeout`
- `run_as_user`
- `run_as_group`
- `risk_level`
- `output_file`

These fields should be defined in the template.

```toml
# Error example: Using both template and cmd
[[groups.commands]]
name = "test"
template = "restic_backup"
cmd = "foo"  # Error: template and cmd are exclusive
```

### 7.4.4 Compatible Fields

Fields that can be used with `template`:

| Field | Description |
|-------|-------------|
| `name` | Command name (required) |
| `params` | Parameter specification |
| `description` | Command description |

### 7.4.5 Creating Multiple Commands from the Same Template

Multiple commands can be defined from the same template with different `name` values:

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]

[[groups]]
name = "backup_tasks"

[[groups.commands]]
name = "backup_data"
template = "restic_backup"
params.path = "/data"

[[groups.commands]]
name = "backup_config"
template = "restic_backup"
params.path = "/etc"

[[groups.commands]]
name = "backup_home"
template = "restic_backup"
params.path = "/home"
```

## 7.5 Field Inheritance

### 7.5.1 Inheritance Overview

Fields defined in templates are automatically inherited unless explicitly overridden in command definitions. This feature allows you to define common settings in templates and customize them at the command level as needed.

### 7.5.2 Inheritable Fields

The following fields support inheritance:

| Field | Inheritance Model | Description |
|-------|-------------------|-------------|
| `workdir` | Override | Overwrite only when specified in command |
| `output_file` | Override | Overwrite only when specified in command |
| `env_import` | Union Merge | Combine values from both template and command |
| `vars` | Map Merge | Command variables override template variables |

### 7.5.3 Override Model (workdir, output_file)

**Behavior:**
- When field is not specified in command: Inherit value from template
- When field is specified in command: Completely overwrite template value
- Specifying empty string: Explicitly represents "unspecified" (`workdir=""` uses current directory)

**Example: workdir Inheritance**

```toml
# Template definition
[command_templates.build_template]
cmd = "make"
workdir = "/workspace/project"

# Case 1: Inherit workdir
[[groups.commands]]
name = "build-default"
template = "build_template"
# Result: workdir="/workspace/project" (inherited from template)

# Case 2: Override workdir
[[groups.commands]]
name = "build-custom"
template = "build_template"
workdir = "/opt/custom"
# Result: workdir="/opt/custom" (overrides template)

# Case 3: Explicitly set workdir to empty
[[groups.commands]]
name = "build-current"
template = "build_template"
workdir = ""
# Result: workdir="" (use current directory)
```

### 7.5.4 Union Merge Model (env_import)

**Behavior:**
- Combine environment variables specified in both template and command
- Duplicates are automatically removed
- Template values are inherited even if empty array is specified

**Example: env_import Merge**

```toml
[global]
env_allowed = ["CC", "CXX", "LDFLAGS", "CFLAGS", "PATH"]

[command_templates.compiler_template]
cmd = "gcc"
env_import = ["cc=CC", "cxx=CXX"]

# Case 1: Import additional environment variables
[[groups.commands]]
name = "compile-with-flags"
template = "compiler_template"
env_import = ["ldflags=LDFLAGS", "cflags=CFLAGS"]
# Result: env_import=["cc=CC", "cxx=CXX", "ldflags=LDFLAGS", "cflags=CFLAGS"] (union)

# Case 2: Use template only
[[groups.commands]]
name = "compile-basic"
template = "compiler_template"
# Result: env_import=["cc=CC", "cxx=CXX"] (inherited from template)

# Case 3: When duplicates exist (command overrides)
[[groups.commands]]
name = "compile-dup"
template = "compiler_template"
env_import = ["cc=GCC", "ldflags=LDFLAGS"]
# Result: env_import=["cc=GCC", "cxx=CXX", "ldflags=LDFLAGS"] (command's cc=GCC overrides template's cc=CC)
```

### 7.5.5 Map Merge Model (vars)

**Behavior:**
- Combine variables defined in both template and command
- When same key exists, command level value takes priority
- New variables can be added at command level

**Example: vars Merge**

```toml
[command_templates.backup_template]
cmd = "restic"
args = ["backup"]

[command_templates.backup_template.vars]
retention_days = "7"
compression = "auto"
backup_type = "incremental"

# Case 1: Add variables
[[groups.commands]]
name = "backup-with-tag"
template = "backup_template"

[groups.commands.vars]
backup_tag = "daily"
# Result: {retention_days: "7", compression: "auto", backup_type: "incremental", backup_tag: "daily"}

# Case 2: Override variables
[[groups.commands]]
name = "backup-full"
template = "backup_template"

[groups.commands.vars]
backup_type = "full"  # Override template value
retention_days = "30"  # Override template value
# Result: {retention_days: "30", compression: "auto", backup_type: "full"}

# Case 3: Use template only
[[groups.commands]]
name = "backup-default"
template = "backup_template"
# Result: {retention_days: "7", compression: "auto", backup_type: "incremental"}
```

### 7.5.6 Combining Inheritance and Parameter Expansion

Parameter expansion can be used in template `workdir`, `output_file`, and `vars` fields.

**Note**: Parameter expansion is not supported in `env_import`. Environment variable imports must be statically defined.

```toml
[command_templates.project_template]
cmd = "make"
workdir = "/workspace/${project}"
output_file = "/var/log/${project}.log"

[command_templates.project_template.vars]
build_type = "${?type}"

[[groups.commands]]
name = "build-projectA"
template = "project_template"

[groups.commands.params]
project = "projectA"
type = "release"
# Result:
#   workdir="/workspace/projectA"
#   output_file="/var/log/projectA.log"
#   vars={build_type: "release"}

[[groups.commands]]
name = "build-projectB"
template = "project_template"
workdir = "/opt/builds"  # Override template workdir

[groups.commands.params]
project = "projectB"
# Result:
#   workdir="/opt/builds" (override)
#   output_file="/var/log/projectB.log"
```

### 7.5.7 Inheritance Priority

Field values are determined in the following priority order:

1. **Explicit specification at command level** (highest priority)
2. **Template value**
3. **System default** (lowest priority)

This priority order allows you to define common settings in templates while customizing them at the command level as needed.

## 7.6 Combining with Variable Expansion

### 7.6.1 Expansion Order

Template expansion and variable expansion (`%{...}`) are processed in the following order:

1. **Template Expansion**: Replace `${...}`, `${?...}`, `${@...}` with params values
2. **Variable Expansion**: Expand `%{...}` in the result

### 7.6.2 Variable References in params

Variable references (`%{...}`) can be included in `params` values. This allows group-specific variables to be passed to templates.

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${backup_path}"]

[[groups]]
name = "group1"

[groups.vars]
group_root = "/data/group1"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"
params.backup_path = "%{group_root}/volumes"

# Expansion process:
# Step 1: Template expansion (replace ${...} with params value)
#   args = ["backup", "%{group_root}/volumes"]
# Step 2: Variable expansion (expand %{...})
#   args = ["backup", "/data/group1/volumes"]
```

### 7.6.3 Variable References in Template Definitions

In template definitions, **only global variables** can be referenced. Global variables are those that start with an uppercase letter (e.g., `%{BackupDir}`).

#### Allowed Variable References

```toml
# Define global variables
[global.vars]
BackupDir = "/data/backups"
ToolsPath = "/opt/tools"

# OK: Reference global variables in template
[command_templates.backup_tool]
cmd = "%{ToolsPath}/backup"
args = ["--output", "%{BackupDir}", "${path}"]
```

#### Prohibited Variable References

**Local variables** (starting with lowercase or underscore) cannot be referenced in template definitions:

```toml
[groups.vars]
backup_date = "20250101"  # Local variable

# Error: Referencing local variable in template
[command_templates.bad_template]
cmd = "echo"
args = ["%{backup_date}"]  # Error: Local variables cannot be referenced
```

**Reason**:
- Templates are reused across multiple groups
- Local variables may have different values per group
- Restricting to global variables ensures predictable and safe behavior

#### Recommended Pattern

To use local variables, pass them via `params`:

```toml
# Template definition: Use global variables and parameters
[command_templates.backup_with_date]
cmd = "%{ToolsPath}/backup"
args = ["--output", "%{BackupDir}/${date}", "${path}"]

# Group level: Define local variables
[groups.vars]
backup_date = "20250101"

# Command: Pass local variable via params
[[groups.commands]]
name = "daily_backup"
template = "backup_with_date"
[groups.commands.params]
date = "%{backup_date}"  # Reference local variable in params
path = "/data/volumes"
```

## 7.7 Escape Sequences

### 7.7.1 Writing Literal `$`

To use a literal `$` character in a template definition, escape it with `\$`.

> **Note**: In TOML files, you need to write `\\$` (TOML escape rules convert `\$` to `$`).

```toml
[command_templates.cost_report]
cmd = "echo"
args = ["Cost: \\$100", "${item}"]

[[groups.commands]]
name = "report"
template = "cost_report"
params.item = "Widget"
# Result: args = ["Cost: $100", "Widget"]
```

### 7.7.2 Consistency with Existing Escapes

This escape notation is the same as the `\%` escape for variable expansion:

- `\%{var}` → `%{var}` (not expanded)
- `\$` → `$` (literal)

## 7.8 Errors and Validation

### 7.8.1 Common Errors

#### Referencing Non-Existent Template

```toml
[[groups.commands]]
name = "test"
template = "nonexistent_template"  # Error: template "nonexistent_template" not found
```

#### Required Parameter Not Specified

```toml
[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]  # path is required

[[groups.commands]]
name = "backup_test"
template = "backup"
# Error: parameter "path" is required but not provided in template "backup"
```

#### Using Both template and cmd

```toml
[[groups.commands]]
name = "test"
template = "backup"
cmd = "/bin/echo"  # Error: cannot specify both "template" and "cmd" fields
```

#### name Field in Template Definition

```toml
[command_templates.bad_template]
name = "should_not_be_here"  # Error: template definition cannot contain "name" field
cmd = "echo"
```

#### Variable Reference in Template Definition

```toml
[command_templates.dangerous]
cmd = "echo"
args = ["%{secret}"]  # Error: template contains forbidden pattern "%{" in args
```

### 7.8.2 Warnings

#### Unused Parameters

If you pass a parameter that is not used in the template, a warning will be output (not an error):

```toml
[command_templates.simple]
cmd = "echo"
args = ["hello"]  # Does not use parameters

[[groups.commands]]
name = "test"
template = "simple"
params.unused = "value"  # Warning: unused parameter "unused" in template "simple"
```

## 7.9 Practical Configuration Examples

### 7.9.1 Common Backup Tasks (Inheriting workdir and output_file)

This example demonstrates efficient backup task management leveraging `workdir` and `output_file` inheritance features.

```toml
version = "1.0"

[global.vars]
BackupRoot = "/var/backups"
LogDir = "/var/log/backups"

# Template definition: Define common working directory and log output
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${backup_path}"]
workdir = "/opt/restic"  # Default working directory
output_file = "%{LogDir}/${log_name}.log"  # Log file (parameterized)
timeout = 3600
risk_level = "medium"

[command_templates.restic_forget]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "${keep_daily}", "--keep-weekly", "${keep_weekly}", "--keep-monthly", "${keep_monthly}"]
workdir = "/opt/restic"  # Common working directory
output_file = "%{LogDir}/${log_name}.log"
timeout = 1800
risk_level = "medium"

# Group 1: Important data (detailed logs, long-term retention, custom working directory)
[[groups]]
name = "important_data"

[groups.vars]
data_root = "/data/important"

[[groups.commands]]
name = "backup"
template = "restic_backup"
workdir = "/data/important"  # Override template workdir
params.verbose_flags = ["-v", "-v"]
params.backup_path = "%{data_root}"
params.log_name = "important_backup"
# Inheritance result:
#   workdir="/data/important" (override)
#   output_file="/var/log/backups/important_backup.log" (inherited, parameter expansion)

[[groups.commands]]
name = "cleanup"
template = "restic_forget"
# workdir inherited from template: "/opt/restic"
params.keep_daily = "14"
params.keep_weekly = "8"
params.keep_monthly = "12"
params.log_name = "important_cleanup"
# Inheritance result:
#   workdir="/opt/restic" (inherited)
#   output_file="/var/log/backups/important_cleanup.log" (inherited, parameter expansion)

# Group 2: Temporary data (silent mode, short-term retention, inherit default settings)
[[groups]]
name = "temp_data"

[groups.vars]
data_root = "/data/temp"

[[groups.commands]]
name = "backup"
template = "restic_backup"
# Inherit workdir and output_file from template
params.verbose_flags = []  # Silent mode
params.backup_path = "%{data_root}"
params.log_name = "temp_backup"
# Inheritance result:
#   workdir="/opt/restic" (inherited)
#   output_file="/var/log/backups/temp_backup.log" (inherited, parameter expansion)

[[groups.commands]]
name = "cleanup"
template = "restic_forget"
params.keep_daily = "3"
params.keep_weekly = "1"
params.keep_monthly = "0"
params.log_name = "temp_cleanup"
# Inheritance result:
#   workdir="/opt/restic" (inherited)
#   output_file="/var/log/backups/temp_cleanup.log" (inherited, parameter expansion)
```

### 7.9.2 Common Database Operations (Inheriting env_import and vars)

This example demonstrates efficient database operation management leveraging `env_import` and `vars` inheritance features.

```toml
version = "1.0"

[global]
env_allowed = ["PGHOST", "PGPORT", "PGPASSWORD", "PGUSER", "PATH"]

[global.vars]
BackupDir = "/var/backups/postgres"

# Template definition: Define common environment variables and configuration variables
[command_templates.pg_dump]
cmd = "/usr/bin/pg_dump"
args = ["${?verbose}", "-U", "${db_user}", "-d", "${database}", "-f", "${output_file}"]
env_import = ["pghost=PGHOST", "pgport=PGPORT"]  # Basic environment variables
timeout = 1800
risk_level = "medium"

# Template level vars: Provide default values
[command_templates.pg_dump.vars]
dump_format = "custom"
compression_level = "6"

[command_templates.pg_restore]
cmd = "/usr/bin/pg_restore"
args = ["${?verbose}", "-U", "${db_user}", "-d", "${database}", "${input_file}"]
env_import = ["pghost=PGHOST", "pgport=PGPORT"]  # Basic environment variables
timeout = 3600
risk_level = "high"

[command_templates.pg_restore.vars]
restore_mode = "clean"

[[groups]]
name = "database_backup"

[groups.vars]
backup_dir = "%{BackupDir}"

# Command 1: Main DB (add environment variables, override vars)
[[groups.commands]]
name = "backup_main_db"
template = "pg_dump"
env_import = ["pgpassword=PGPASSWORD"]  # Add to template env_import
params.verbose = "--verbose"
params.db_user = "postgres"
params.database = "main_production"
params.output_file = "%{backup_dir}/main_db.dump"

[groups.commands.vars]
compression_level = "9"  # Override template default (high compression)
backup_priority = "high"  # Add new variable
# Inheritance result:
#   env_import=["pghost=PGHOST", "pgport=PGPORT", "pgpassword=PGPASSWORD"] (merge)
#   vars={dump_format: "custom", compression_level: "9", backup_priority: "high"}

# Command 2: Logs DB (inherit template settings as-is)
[[groups.commands]]
name = "backup_logs_db"
template = "pg_dump"
# env_import inherited from template
params.verbose = ""  # Silent mode
params.db_user = "postgres"
params.database = "logs"
params.output_file = "%{backup_dir}/logs_db.dump"

[groups.commands.vars]
backup_priority = "low"  # Add new variable
# Inheritance result:
#   env_import=["PGHOST", "PGPORT"] (inherited)
#   vars={dump_format: "custom", compression_level: "6", backup_priority: "low"}

# Command 3: Restore (requires additional user environment variables)
[[groups.commands]]
name = "restore_main_db"
template = "pg_restore"
env_import = ["PGPASSWORD", "PGUSER"]  # Add to template
params.verbose = "--verbose"
params.db_user = "postgres"
params.database = "main_production_restored"
params.input_file = "%{backup_dir}/main_db.dump"

[groups.commands.vars]
restore_mode = "drop"  # Override template default
# Inheritance result:
#   env_import=["PGHOST", "PGPORT", "PGPASSWORD", "PGUSER"] (merge, remove duplicates)
#   vars={restore_mode: "drop"}
```

### 7.9.3 Common System Monitoring Tasks

```toml
version = "1.0"

[command_templates.check_disk]
cmd = "/bin/df"
args = ["-h", "${mount_point}"]
timeout = 30
risk_level = "low"

[command_templates.check_service]
cmd = "/usr/bin/systemctl"
args = ["status", "${service_name}"]
timeout = 30
risk_level = "low"

[[groups]]
name = "system_monitoring"

[[groups.commands]]
name = "check_root_disk"
template = "check_disk"
params.mount_point = "/"

[[groups.commands]]
name = "check_data_disk"
template = "check_disk"
params.mount_point = "/data"

[[groups.commands]]
name = "check_nginx"
template = "check_service"
params.service_name = "nginx"

[[groups.commands]]
name = "check_postgres"
template = "check_service"
params.service_name = "postgresql"
```

### 7.9.4 Leveraging Combined Inheritance

This example demonstrates using all inheritance features (workdir, output_file, env_import, vars) together.

```toml
[global]
env_allowed = ["CC", "CXX", "CFLAGS", "LDFLAGS", "PATH", "HOME"]

# Generic build template
[command_templates.build_base]
cmd = "make"
workdir = "/workspace"
output_file = "/var/log/build.log"
env_import = ["cc=CC", "cxx=CXX"]
timeout = 3600

[command_templates.build_base.vars]
optimization = "O2"
debug = "false"

[[groups]]
name = "development"

# Debug build: Override some settings
[[groups.commands]]
name = "build-debug"
template = "build_base"
args = ["debug"]
env_import = ["cflags=CFLAGS"]  # Import cflags in addition to cc, cxx

[groups.commands.vars]
optimization = "O0"  # Disable optimization
debug = "true"       # Enable debug mode
# Inheritance result:
#   workdir="/workspace" (inherited from template)
#   output_file="/var/log/build.log" (inherited from template)
#   env_import=["cc=CC", "cxx=CXX", "cflags=CFLAGS"] (merge)
#   vars={optimization: "O0", debug: "true"} (override)
#   timeout=3600 (inherited from template)

# Release build: Change working directory and output destination
[[groups.commands]]
name = "build-release"
template = "build_base"
args = ["release"]
workdir = "/opt/releases"
output_file = "/var/log/release.log"
env_import = ["ldflags=LDFLAGS"]

[groups.commands.vars]
optimization = "O3"
# Inheritance result:
#   workdir="/opt/releases" (override)
#   output_file="/var/log/release.log" (override)
#   env_import=["cc=CC", "cxx=CXX", "ldflags=LDFLAGS"] (merge)
#   vars={optimization: "O3", debug: "false"} (partial override)
#   timeout=3600 (inherited from template)
```

## 7.10 Best Practices

### 7.10.1 Template Design Guidelines

1. **Single Responsibility**: Each template should focus on one purpose
2. **Appropriate Parameterization**: Parameterize parts that are likely to change
3. **Meaningful Names**: Template names should indicate their purpose
4. **Consider Default Values**: Leverage optional parameters (`${?...}`)

### 7.10.2 Parameter Design Guidelines

1. **Required vs Optional**: Use `${param}` for always required values, `${?param}` for optional values
2. **Leverage Arrays**: Pass multiple flags and options as arrays using `${@list}`
3. **Combine with Variables**: Reference group-specific values using `%{var}` in `params`

### 7.10.3 Leveraging Inheritance in Design

1. **Define Common Settings in Templates**: Consolidate common settings like `workdir`, `env_import`, `vars` in templates
2. **Specify Only Differences in Commands**: Explicitly specify only command-specific settings
3. **Understand Appropriate Inheritance Models**: Understand and leverage the inheritance behavior for each field

### 7.10.4 vars Inheritance Guidelines

1. **Provide Default Values**: Set common default values in templates
2. **Design for Overrideability**: Allow flexible customization at command level
3. **Variable Name Consistency**: Use same variable names between template and commands

### 7.10.5 env_import Inheritance Guidelines

1. **Minimum Imports**: Import only required environment variables in templates
2. **Additional Imports in Commands**: Add optional environment variables at command level
3. **Maintain env_allowed**: Define allowed environment variables appropriately

### 7.10.6 Security Guidelines

1. **No Variable References in Template Definitions**: Always pass explicitly via `params`
2. **Validate Parameter Values**: Expanded commands are automatically validated for security
3. **Principle of Least Privilege**: Set `run_as_user` and `risk_level` appropriately in templates

## Next Steps

After understanding the command template feature, also refer to the following chapters:

- [Chapter 6: Command Level Configuration](06_command_level.md) - Command definitions without templates
- [Chapter 8: Variable Expansion Feature](08_variable_expansion.md) - `%{var}` format variable expansion
- [Chapter 9: Practical Configuration Examples](09_practical_examples.md) - More configuration examples
