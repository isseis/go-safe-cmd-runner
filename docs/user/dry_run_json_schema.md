# Dry-Run JSON Output Schema Definition

## Overview

This document defines the data structures output in JSON format during dry-run execution of the `runner` command.

## Related Documents

- [runner Command User Guide](runner_command.md)

## Top-Level Structure

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": { ... }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "workdir": "/var/backups",
      "timeout": 3600,
      "risk_level": "medium",
      "debug_info": { ... }
    }
  ]
}
```

## ResourceAnalysis

Represents the analysis result of each resource (group or command).

### Common Fields

| Field | Type | Description |
|---------|------|------|
| `resource_type` | string | Type of resource (`"group"` or `"command"`) |
| `operation` | string | Type of operation (`"analyze"` or `"execute"`) |
| `group_name` | string | Group name |
| `debug_info` | object? | Debug information (included depending on detail level) |

### Group Resource Specific Fields

For group resources (`resource_type: "group"`), only common fields are present.

### Command Resource Specific Fields

For command resources (`resource_type: "command"`), the following fields are added:

| Field | Type | Description |
|---------|------|------|
| `command_name` | string | Command name |
| `cmd` | string | Path to the command to execute |
| `args` | string[]? | Command-line arguments |
| `workdir` | string? | Working directory |
| `timeout` | number? | Timeout (seconds) |
| `risk_level` | string? | Risk level (`"low"`, `"medium"`, `"high"`) |

## DebugInfo

An object containing debug information. The content varies depending on the detail level (`-dry-run-detail`).

### Content by Detail Level

| Detail Level | `from_env_inheritance` | `final_environment` |
|----------|----------------------|-------------------|
| `summary` | None | None |
| `detailed` | Basic information only | Basic information only |
| `full` | Complete information including diff | Complete information |

### Fields

| Field | Type | Description | Occurrence Condition |
|---------|------|------|---------|
| `from_env_inheritance` | InheritanceAnalysis? | Analysis result of environment variable inheritance | Group resources with `detailed` or higher |
| `final_environment` | FinalEnvironment? | Final environment variables | Command resources with `detailed` or higher |

## InheritanceAnalysis

Analysis information related to environment variable inheritance.

### Fields

| Field | Type | Description | Occurrence Condition |
|---------|------|------|---------|
| `global_env_import` | string[]? | Global-level `env_import` configuration | Always |
| `global_allowlist` | string[]? | Global-level `allowlist` configuration | Always |
| `group_env_import` | string[]? | Group-level `env_import` configuration | Always |
| `group_allowlist` | string[]? | Group-level `allowlist` configuration | Always |
| `inheritance_mode` | string | Inheritance mode (`"inherit"`, `"explicit"`, `"reject"`) | Always |
| `inherited_variables` | string[]? | List of variable names actually inherited | `full` only |
| `removed_allowlist_variables` | string[]? | List of variable names removed from allowlist | `full` only |
| `unavailable_env_import_variables` | string[]? | List of variable names specified in env_import but unavailable | `full` only |

### Inheritance Mode (inheritance_mode)

| Value | Description |
|----|------|
| `"inherit"` | Inherit global-level environment variable settings |
| `"explicit"` | Use only explicitly set variables at group level |
| `"reject"` | Reject global-level environment variables |

### Diff Information Description

**inherited_variables**

A list of environment variable names actually inherited from the global level to the group level. Includes variables that meet the following conditions:

- Specified in `global_env_import`
- Exists in the system environment
- Remains after filtering by `global_allowlist`

**removed_allowlist_variables**

A list of variable names removed from `global_allowlist`. Includes variables that meet the following conditions:

- Specified in `global_allowlist`
- Not specified in `group_allowlist` (removed at group level)

**unavailable_env_import_variables**

A list of variable names specified in `env_import` but not present in the system environment.

## FinalEnvironment

The final state of environment variables at command execution time.

### Fields

| Field | Type | Description |
|---------|------|------|
| `variables` | EnvironmentVariable[] | List of environment variables |

## EnvironmentVariable

Represents an individual environment variable.

### Fields

| Field | Type | Description |
|---------|------|------|
| `name` | string | Environment variable name |
| `value` | string | Environment variable value (sensitive information is masked) |
| `source` | string | Source of the value |

### Source Field Values

| Value | Description |
|----|------|
| `"Global"` | Variable defined at global level |
| `"Group[<name>]"` | Variable defined in a specific group (`<name>` is the group name) |
| `"Command[<name>]"` | Variable defined in a specific command (`<name>` is the command name) |
| `"System (filtered by allowlist)"` | Variable filtered from system environment by allowlist |
| `"Internal"` | Internal variable automatically set by the system |

## Sensitive Information Masking

By default, the values of environment variable names matching the following patterns are masked with `[REDACTED]`:

- `*PASSWORD*`
- `*SECRET*`
- `*TOKEN*`
- `*KEY*`
- `*CREDENTIAL*`
- `*AUTH*`

Using the `--show-sensitive` flag displays values in plain text without masking.

## Usage Examples

### DetailLevelSummary

```json
{
  "resource_analyses": [
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "risk_level": "medium"
    }
  ]
}
```

The `debug_info` field is not included.

### DetailLevelDetailed

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": {
        "from_env_inheritance": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR"],
          "inheritance_mode": "inherit"
        }
      }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "risk_level": "medium",
      "debug_info": {
        "final_environment": {
          "variables": [
            {
              "name": "BACKUP_DIR",
              "value": "/var/backups",
              "source": "Group[backup]"
            },
            {
              "name": "HOME",
              "value": "/root",
              "source": "System (filtered by allowlist)"
            },
            {
              "name": "PATH",
              "value": "/usr/local/bin:/usr/bin:/bin",
              "source": "System (filtered by allowlist)"
            }
          ]
        }
      }
    }
  ]
}
```

Basic debug information is included.

### DetailLevelFull

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": {
        "from_env_inheritance": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH", "USER"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR", "TEMP_DIR"],
          "inheritance_mode": "inherit",
          "inherited_variables": ["HOME", "PATH"],
          "removed_allowlist_variables": ["USER"],
          "unavailable_env_import_variables": []
        }
      }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "workdir": "/var/backups",
      "timeout": 3600,
      "risk_level": "medium",
      "debug_info": {
        "final_environment": {
          "variables": [
            {
              "name": "BACKUP_DIR",
              "value": "/var/backups",
              "source": "Group[backup]"
            },
            {
              "name": "DB_PASSWORD",
              "value": "[REDACTED]",
              "source": "Command[db_backup]"
            },
            {
              "name": "HOME",
              "value": "/root",
              "source": "System (filtered by allowlist)"
            },
            {
              "name": "PATH",
              "value": "/usr/local/bin:/usr/bin:/bin",
              "source": "System (filtered by allowlist)"
            }
          ]
        }
      }
    }
  ]
}
```

Complete debug information including diff information is included.

## Analysis Examples Using jq

### Extract Only Debug Information

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info != null) | .debug_info'
```

### Check Environment Variable Inheritance Mode

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail detailed | \
  jq '.resource_analyses[] | select(.debug_info.from_env_inheritance != null) | .debug_info.from_env_inheritance.inheritance_mode'
```

### Check Final Environment Variables of a Specific Command

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.command_name == "db_backup") | .debug_info.final_environment.variables'
```

### Get List of Inherited Variables

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.from_env_inheritance.inherited_variables != null) | .debug_info.from_env_inheritance.inherited_variables[]'
```

### Identify Commands with Sensitive Variables

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.final_environment != null) | select(.debug_info.final_environment.variables[] | select(.value == "[REDACTED]")) | .command_name'
```

---

**Last Updated**: 2025-11-03
