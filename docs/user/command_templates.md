# Command Templates Feature

## Overview

The command templates feature allows you to create reusable command definitions that can be executed multiple times with different parameters. This reduces duplication in configuration files and improves maintainability.

## Basic Usage

### Defining Templates

Templates are defined in `[command_templates.template_name]` sections:

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]
```

### Using Templates

Specify the `template` field in a command definition and pass parameter values via `params`:

```toml
[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "/data/volumes"
repo = "/backup/repo"
```

## Template Includes

### Overview

The includes feature allows you to organize command templates across multiple files and import them into your main configuration. This is particularly useful for:

- Sharing common templates across multiple projects
- Organizing templates by category or purpose
- Maintaining team-wide template libraries

### Basic Include Usage

In your main configuration file, use the `includes` array to specify template files:

```toml
version = "1.0"

# Include template files (paths relative to this config file)
includes = ["templates/backup.toml", "templates/docker.toml"]

[global]
timeout = 300

[[groups]]
name = "backup"

[[groups.commands]]
name = "backup_data"
template = "restic_backup"  # Defined in templates/backup.toml

[groups.commands.params]
path = "/data"
repo = "/backup/repo"
```

### Template File Format

Template files must contain **only** `version` and `command_templates` sections:

**templates/backup.toml:**
```toml
version = "1.0"

[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[command_templates.restic_check]
cmd = "restic"
args = ["check"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]
```

**Invalid template file (will cause error):**
```toml
version = "1.0"

# ❌ Error: Template files cannot contain 'global' or 'groups'
[global]
timeout = 300

[command_templates.example]
cmd = "echo"
```

### Path Resolution

Include paths can be specified as:

1. **Relative paths** (resolved relative to the config file directory):
   ```toml
   includes = ["templates/common.toml"]           # Same directory
   includes = ["../shared/backup.toml"]          # Parent directory
   includes = ["../../lib/templates/db.toml"]    # Multiple levels up
   ```

2. **Absolute paths**:
   ```toml
   includes = ["/etc/safe-cmd-runner/templates/system.toml"]
   ```

### Template Merging Rules

When templates are loaded from multiple sources:

1. **Loading order**:
   - Templates from included files (in the order specified in `includes`)
   - Templates from the main config file's `command_templates` section

2. **Duplicate detection**:
   - If the same template name appears in multiple files, an error is raised
   - The error message shows all locations where the template is defined

3. **Example**:
   ```toml
   # main.toml
   includes = ["a.toml", "b.toml"]

   [command_templates.backup]
   cmd = "rsync"
   # ...
   ```

   If `a.toml` also defines a `backup` template, you'll get an error:
   ```
   Error: duplicate command template name "backup"
     Defined in: /path/to/a.toml
     Also found in: /path/to/main.toml
   ```

### Limitations

1. **No multi-level includes**: Template files cannot include other template files
2. **No circular references**: Not possible due to the above limitation
3. **Template-only content**: Included files can only contain templates, not global config or groups

### Security Considerations

The includes feature follows the same security model as the rest of the system:

- **Path traversal protection**: Paths are validated to prevent accessing unauthorized files
- **Checksum verification**: All included files are verified using the same hash-based system
- **Symbolic link checks**: Symbolic links are validated for security

To record hashes for included files:
```bash
# Record hashes for all files (main config + includes)
safe-cmd-runner record -c config.toml -o hashes/

# The hash directory will contain:
# - config.toml.sha256 (main config)
# - templates_backup.toml.sha256 (included file)
# - templates_docker.toml.sha256 (included file)
```

## Placeholder Syntax

The following placeholder syntax can be used within templates:

### Required Parameters: `${param}`

Parameters where a value must be provided. Omitting the value results in an error.

```toml
[command_templates.example]
cmd = "echo"
args = ["${message}"]  # message is required
```

### Optional Parameters: `${?param}`

Parameters where a value can be omitted. If the value is an empty string or unspecified, the entire argument is removed.

```toml
[command_templates.example]
cmd = "restic"
args = ["${?verbose}", "backup", "${path}"]
# If verbose is empty, args becomes ["backup", "/data"]
```

### Array Parameters: `${@param}`

Expands array values as multiple elements. Can be used at the array element level in `args` and `env_vars`.

**Usage in args:**
```toml
[command_templates.example]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]

# If params.flags = ["-v", "-q"]
# Expansion result: args = ["-v", "-q", "backup", "/data"]
```

**Usage in env_vars:**
```toml
[command_templates.docker_run]
cmd = "docker"
args = ["run", "${image}"]
env_vars = ["REQUIRED=value", "${@optional_env}"]

# If params.optional_env = ["DEBUG=1", "VERBOSE=1"]
# Expansion result: env_vars = ["REQUIRED=value", "DEBUG=1", "VERBOSE=1"]

# If params.optional_env is empty array or unspecified
# Expansion result: env_vars = ["REQUIRED=value"]
```

**Note:** `${@param}` cannot be used in the VALUE portion (right side of `=`) in `env_vars`:
```toml
# ❌ Error: Array expansion in VALUE portion of env_vars is not allowed
env_vars = ["PATH=${@paths}"]  # Invalid

# ✓ OK: Array expansion at element level in env_vars
env_vars = ["${@path_defs}"]
# path_defs = ["PATH=/usr/bin", "LD_LIBRARY_PATH=/lib"]
```

### Escape Sequences

To use a literal `$`, escape it with `\$` (in TOML, `\\$`):

```toml
[command_templates.example]
cmd = "echo"
args = ["Price: \\$100"]  # Expands as "Price: $100"
```

## Parameter Types

Template parameters support the following types:

- **string**: `params.name = "value"`
- **array**: `params.flags = ["-v", "-q"]` (can only be used with `${@param}`)

## Combination with Variable Expansion

The `%{var}` syntax can be used within parameter values to reference group variables:

```toml
[groups.vars]
backup_root = "/data/backups"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "/data/volumes"
repo = "%{backup_root}/repo"  # Expands to "/data/backups/repo"
```

**Expansion Order**:
1. Template expansion (`${param}` → parameter value)
2. Variable expansion (`%{var}` → variable value)
3. Security validation

## Available Fields in Templates

The following fields can be used in template definitions:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cmd` | string | ✓ | Command path |
| `args` | []string | | Command arguments |
| `env_vars` | []string | | Environment variables (KEY=VALUE format) |
| `workdir` | string | | Working directory |
| `timeout` | int32 | | Timeout (seconds) ※1 |
| `output_size_limit` | int64 | | Output size limit (bytes) ※1 |
| `risk_level` | string | | Risk level (low/medium/high) ※1 |

**Notes**:
- The `name` field cannot be included in template definitions. The command name is specified when using the template.
- ※1 Execution settings (`timeout`, `output_size_limit`, `risk_level`) can be overridden by explicitly specifying them in the command definition. If not specified in the command definition, the template values are used.

## Security Constraints

### Constraints Within Template Definitions

In template definitions (`cmd`, `args`, `env_vars`, `workdir`), **the `%{var}` syntax is prohibited**. This is to avoid ambiguity in the expansion order.

```toml
# ❌ Error: %{var} cannot be used in template definitions
[command_templates.bad_example]
cmd = "%{root}/bin/restic"  # Error
args = ["backup", "${path}"]
```

### Usage Within Parameter Values

The `%{var}` syntax can be used in parameter values (`params.*`):

```toml
# ✅ OK: %{var} can be used in params
[[groups.commands]]
template = "restic_backup"

[groups.commands.params]
path = "%{backup_root}/data"  # OK
```

### Field Exclusivity

When using `template` in a command definition, the following fields cannot be specified simultaneously:
- `cmd`
- `args`
- `env_vars`

**Note**: `workdir` can be used together with templates (it can override the template's default value).

```toml
# ❌ Error: Specifying both template and cmd
[[groups.commands]]
name = "backup"
template = "restic_backup"
cmd = "restic"  # Error

# ✅ OK: template and workdir can be used together
[[groups.commands]]
name = "backup"
template = "restic_backup"
workdir = "/custom/dir"  # Overrides the template's workdir
```

## Template Naming Conventions

Template names must follow these rules:

- Must start with a letter or underscore (`_`)
- Only letters, numbers, and underscores are allowed
- Names starting with `__` (two underscores) are reserved

```toml
# ✅ Valid names
[command_templates.restic_backup]
[command_templates.backup_v2]
[command_templates._internal]

# ❌ Invalid names
[command_templates.123backup]      # Starts with a number
[command_templates.backup-name]    # Uses hyphen
[command_templates.__reserved]     # Reserved prefix
```

## Practical Examples

### Example 1: Basic Backup Template

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[[groups]]
name = "daily_backup"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "/data/volumes"
repo = "/backup/repo"

[[groups.commands]]
name = "backup_database"
template = "restic_backup"

[groups.commands.params]
path = "/data/database"
repo = "/backup/repo"
```

### Example 2: Leveraging Optional Parameters

```toml
[command_templates.restic_backup_flexible]
cmd = "restic"
args = ["${?verbose}", "backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[[groups.commands]]
name = "backup_verbose"
template = "restic_backup_flexible"

[groups.commands.params]
verbose = "-v"  # Verbose mode
path = "/data"
repo = "/backup/repo"

[[groups.commands]]
name = "backup_quiet"
template = "restic_backup_flexible"

[groups.commands.params]
# verbose is omitted (removed from arguments)
path = "/data"
repo = "/backup/repo"
```

### Example 3: Flexible Argument Specification with Array Parameters

```toml
[command_templates.restic_backup_advanced]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[[groups.commands]]
name = "backup_full"
template = "restic_backup_advanced"

[groups.commands.params]
flags = ["-v", "--exclude-caches", "--one-file-system"]
path = "/home"
repo = "/backup/repo"

[[groups.commands]]
name = "backup_simple"
template = "restic_backup_advanced"

[groups.commands.params]
flags = []  # No flags
path = "/home"
repo = "/backup/repo"
```

### Example 4: Dynamic Addition of Environment Variables (Array Parameters)

```toml
[command_templates.docker_run]
cmd = "docker"
args = ["run", "${@docker_flags}", "${image}"]
env_vars = ["${@common_env}", "${@app_env}"]

[[groups.commands]]
name = "run_dev"
template = "docker_run"

[groups.commands.params]
docker_flags = ["-it", "--rm"]
image = "myapp:dev"
common_env = ["PATH=/usr/local/bin:/usr/bin", "LANG=C.UTF-8"]
app_env = ["DEBUG=1", "LOG_LEVEL=debug"]

# Expansion result:
# cmd = docker run -it --rm myapp:dev
# env_vars = [
#   "PATH=/usr/local/bin:/usr/bin",
#   "LANG=C.UTF-8",
#   "DEBUG=1",
#   "LOG_LEVEL=debug"
# ]

[[groups.commands]]
name = "run_prod"
template = "docker_run"

[groups.commands.params]
docker_flags = ["-d"]
image = "myapp:latest"
common_env = ["PATH=/usr/local/bin:/usr/bin", "LANG=C.UTF-8"]
app_env = []  # No additional environment variables in production

# Expansion result:
# cmd = docker run -d myapp:latest
# env_vars = ["PATH=/usr/local/bin:/usr/bin", "LANG=C.UTF-8"]
```

### Example 5: Combination with Group Variables

```toml
[global.vars]
backup_root = "/data/backups"

[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[[groups]]
name = "daily_backup"

[groups.vars]
data_dir = "/data"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "%{data_dir}/volumes"           # Group variable reference
repo = "%{backup_root}/repo"           # Global variable reference
```

## Error Messages

Common errors and their solutions:

### `template "xxx" not found`
- The specified template name does not exist
- Check the spelling of the template name

### `required parameter "xxx" missing`
- A required parameter has not been provided
- Add the parameter to the `params` section

### `forbidden pattern %{ in template definition`
- The `%{var}` syntax is being used in a template definition
- Move variable expansion to the `params` side

### `cannot use both template and cmd fields`
- Both `template` and `cmd` are specified simultaneously
- Use only one of them

### `array parameter ${@xxx} cannot be used in mixed context`
- An array parameter is mixed with a string
- Array parameters must be used as standalone elements
- In env_vars, array expansion cannot be used in the VALUE portion (right side of `=`)
  ```toml
  # ❌ Error
  env_vars = ["PATH=${@paths}"]

  # ✓ OK
  env_vars = ["${@env_vars}"]
  ```

## Best Practices

### 1. Use Descriptive Template Names

```toml
# ✅ Good
[command_templates.restic_backup_with_excludes]

# ❌ Bad
[command_templates.rb]
```

### 2. Minimize Required Parameters

Leverage optional parameters to ensure flexibility:

```toml
[command_templates.flexible_backup]
cmd = "restic"
args = ["${?verbose}", "${@extra_flags}", "backup", "${path}"]
```

### 3. Perform Variable Expansion on the Parameter Side

Keep template definitions generic and inject environment-specific values via parameters:

```toml
# Template: Generic
[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

# Usage: Environment-specific
[groups.commands.params]
repo = "%{backup_root}/repo"  # Reference environment variables
```

### 4. Clarify Template Responsibilities

Avoid having a single template carry too many functions:

```toml
# ✅ Good: Clear responsibilities
[command_templates.restic_backup]
[command_templates.restic_restore]
[command_templates.restic_check]

# ❌ Bad: All-in-one template
[command_templates.restic_all_in_one]
```

### 5. Overriding Execution Settings

Define default values for execution settings (`timeout`, `output_size_limit`, `risk_level`) in templates and override them in command definitions as needed:

```toml
# Template: Normal timeout
[command_templates.database_backup]
cmd = "pg_dump"
args = ["${database}"]
timeout = 300  # Default 5 minutes
risk_level = "medium"

[[groups.commands]]
name = "backup_small_db"
template = "database_backup"
# Inherits template's timeout=300 and risk_level="medium"
[groups.commands.params]
database = "small_db"

[[groups.commands]]
name = "backup_large_db"
template = "database_backup"
timeout = 1800  # Override to 30 minutes (for large database)
risk_level = "high"  # Also override risk level
[groups.commands.params]
database = "large_db"
```

**Override Priority**:
- Values explicitly specified in command definitions take highest priority
- If not specified, template values are used
- If not specified in template either, system defaults are used (`risk_level` only, default is "low")

## Reference Information

- Sample configuration: `sample/command_template_example.toml`
- Detailed specification: `docs/tasks/0062_command_templates/03_detailed_spec.md`
- Architecture: `docs/tasks/0062_command_templates/02_architecture.md`
