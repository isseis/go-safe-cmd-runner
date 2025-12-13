# Chapter 11: Command Template Feature

## 11.1 Overview of Command Templates

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

## 11.2 Template Definition

### 11.2.1 Basic Syntax

Templates are defined in the `[command_templates.<template_name>]` section.

```toml
[command_templates.<template_name>]
cmd = "<command>"
args = ["<arg1>", "<arg2>", ...]
# Other command execution related fields
```

### 11.2.2 Available Fields

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

### 11.2.3 Prohibited Fields

The following fields cannot be used in template definitions:

| Field | Reason |
|-------|--------|
| `name` | Command name is specified by the caller |
| `template` | Nesting templates (referencing another template from a template) is prohibited |

### 11.2.4 Template Naming Rules

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

### 11.2.5 Configuration Examples

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

## 11.3 Parameter Expansion

By defining parameters in templates and passing values when calling them, flexible command definitions are possible.

### 11.3.1 Types of Parameter Expansion

go-safe-cmd-runner provides three types of parameter expansion syntax:

| Notation | Name | Use Case | Behavior When Empty |
|----------|------|----------|---------------------|
| `${param}` | String Parameter | Required string value | Preserved as empty string |
| `${?param}` | Optional Parameter | Optional string value | Element removed |
| `${@list}` | Array Parameter | Expand multiple values | Nothing added |

### 11.3.2 String Parameter Expansion `${param}`

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

### 11.3.3 Optional Parameter Expansion `${?param}`

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

### 11.3.4 Array Parameter Expansion `${@list}`

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

### 11.3.5 Parameter Naming Rules

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

## 11.4 Using Templates

### 11.4.1 Basic Usage

Reference a template by specifying the `template` field in `[[groups.commands]]`.

```toml
[[groups.commands]]
name = "<command_name>"      # Required
template = "<template_name>"
params.<param1> = "<value1>"
params.<param2> = ["<value2a>", "<value2b>"]
```

### 11.4.2 Required Fields

| Field | Description |
|-------|-------------|
| `name` | Command name (unique within group) |
| `template` | Template name to reference |

### 11.4.3 Exclusive Fields

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

### 11.4.4 Compatible Fields

Fields that can be used with `template`:

| Field | Description |
|-------|-------------|
| `name` | Command name (required) |
| `params` | Parameter specification |
| `description` | Command description |

### 11.4.5 Creating Multiple Commands from the Same Template

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

## 11.5 Combining with Variable Expansion

### 11.5.1 Expansion Order

Template expansion and variable expansion (`%{...}`) are processed in the following order:

1. **Template Expansion**: Replace `${...}`, `${?...}`, `${@...}` with params values
2. **Variable Expansion**: Expand `%{...}` in the result

### 11.5.2 Variable References in params

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

### 11.5.3 Prohibition of Variable References in Template Definitions

**Important**: For security reasons, variable references (`%{...}`) cannot be included in template definitions (`cmd`, `args`, `env_vars`, `workdir`).

```toml
# Error: Using %{ in template definition
[command_templates.dangerous]
cmd = "echo"
args = ["%{secret_var}"]  # Error: %{ prohibited in template definition
```

**Reason**:
- Templates are reused across multiple groups
- The same variable name may have different meanings in different group contexts
- Prevents the risk of unintentionally referencing sensitive variables

**Correct Approach**: Variable references should be made explicitly via `params`

```toml
# Correct: No variable references in template definition
[command_templates.echo_message]
cmd = "echo"
args = ["${message}"]

# Explicitly reference variables in params
[[groups.commands]]
name = "echo_test"
template = "echo_message"
params.message = "%{my_variable}"  # Allowed
```

## 11.6 Escape Sequences

### 11.6.1 Writing Literal `$`

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

### 11.6.2 Consistency with Existing Escapes

This escape notation is the same as the `\%` escape for variable expansion:

- `\%{var}` → `%{var}` (not expanded)
- `\$` → `$` (literal)

## 11.7 Errors and Validation

### 11.7.1 Common Errors

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

### 11.7.2 Warnings

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

## 11.8 Practical Configuration Examples

### 11.8.1 Common Backup Tasks

```toml
version = "1.0"

# Template definitions
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${backup_path}"]
timeout = 3600
risk_level = "medium"

[command_templates.restic_forget]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "${keep_daily}", "--keep-weekly", "${keep_weekly}", "--keep-monthly", "${keep_monthly}"]
timeout = 1800
risk_level = "medium"

# Group 1: Important data (detailed logs, long-term retention)
[[groups]]
name = "important_data"

[groups.vars]
data_root = "/data/important"

[[groups.commands]]
name = "backup"
template = "restic_backup"
params.verbose_flags = ["-v", "-v"]
params.backup_path = "%{data_root}"

[[groups.commands]]
name = "cleanup"
template = "restic_forget"
params.keep_daily = "14"
params.keep_weekly = "8"

# Group 2: Temporary data (silent mode, short-term retention)
[[groups]]
name = "temp_data"

[groups.vars]
data_root = "/data/temp"

[[groups.commands]]
name = "backup"
template = "restic_backup"
params.verbose_flags = []  # Silent mode
params.backup_path = "%{data_root}"

[[groups.commands]]
name = "cleanup"
template = "restic_forget"
params.keep_daily = "3"
params.keep_weekly = "1"
```

### 11.8.2 Common Database Operations

```toml
version = "1.0"

[command_templates.pg_dump]
cmd = "/usr/bin/pg_dump"
args = ["${?verbose}", "-U", "${db_user}", "-d", "${database}", "-f", "${output_file}"]
timeout = 1800
risk_level = "medium"

[command_templates.pg_restore]
cmd = "/usr/bin/pg_restore"
args = ["${?verbose}", "-d", "${database}", "${input_file}"]
timeout = 3600
risk_level = "high"

[[groups]]
name = "database_backup"

[groups.vars]
backup_dir = "/var/backups/postgres"

[[groups.commands]]
name = "backup_main_db"
template = "pg_dump"
params.verbose = "--verbose"
params.database = "main_production"
params.output_file = "%{backup_dir}/main_db.dump"

[[groups.commands]]
name = "backup_logs_db"
template = "pg_dump"
params.verbose = ""  # Silent mode
params.database = "logs"
params.output_file = "%{backup_dir}/logs_db.dump"
```

### 11.8.3 Common System Monitoring Tasks

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

## 11.9 Best Practices

### 11.9.1 Template Design Guidelines

1. **Single Responsibility**: Each template should focus on one purpose
2. **Appropriate Parameterization**: Parameterize parts that are likely to change
3. **Meaningful Names**: Template names should indicate their purpose
4. **Consider Default Values**: Leverage optional parameters (`${?...}`)

### 11.9.2 Parameter Design Guidelines

1. **Required vs Optional**: Use `${param}` for always required values, `${?param}` for optional values
2. **Leverage Arrays**: Pass multiple flags and options as arrays using `${@list}`
3. **Combine with Variables**: Reference group-specific values using `%{var}` in `params`

### 11.9.3 Security Guidelines

1. **No Variable References in Template Definitions**: Always pass explicitly via `params`
2. **Validate Parameter Values**: Expanded commands are automatically validated for security
3. **Principle of Least Privilege**: Set `run_as_user` and `risk_level` appropriately in templates

## Next Steps

After understanding the command template feature, also refer to the following chapters:

- [Chapter 6: Command Level Configuration](06_command_level.md) - Command definitions without templates
- [Chapter 7: Variable Expansion Feature](07_variable_expansion.md) - `%{var}` format variable expansion
- [Chapter 8: Practical Configuration Examples](08_practical_examples.md) - More configuration examples
