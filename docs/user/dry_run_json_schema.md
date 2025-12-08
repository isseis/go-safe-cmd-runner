# Dry-Run JSON Output Schema Definition

## Overview

This document defines the data structures output in JSON format during dry-run execution of the `runner` command.

**Output Streams**: In dry-run mode, the runner command separates output streams:
- **stdout**: Dry-run JSON output (defined by this schema)
- **stderr**: Execution logs (slog messages)

This separation allows you to redirect dry-run output and logs independently. See the [runner Command User Guide](runner_command.md) for usage examples.

## Related Documents

- [runner Command User Guide](runner_command.md)

## Top-Level Structure

```json
{
  "metadata": {
    "generated_at": "2025-11-23T10:00:00Z",
    "run_id": "abc123",
    "config_path": "/path/to/config.toml",
    "environment_file": "",
    "version": "1.0.0",
    "duration": 1500000000
  },
  "status": "success",
  "phase": "completed",
  "summary": {
    "total_resources": 5,
    "successful": 5,
    "failed": 0,
    "skipped": 0,
    "groups": {
      "total": 2,
      "successful": 2,
      "failed": 0,
      "skipped": 0
    },
    "commands": {
      "total": 3,
      "successful": 3,
      "failed": 0,
      "skipped": 0
    }
  },
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "backup",
      "status": "success",
      "parameters": {},
      "impact": {
        "reversible": true,
        "persistent": false,
        "description": "Configuration analysis only"
      },
      "timestamp": "2025-11-23T10:00:00Z",
      "debug_info": { ... }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "workdir": "/var/backups",
        "timeout": 3600000000000,
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:01Z",
      "debug_info": { ... }
    }
  ],
  "security_analysis": { ... },
  "environment_info": { ... },
  "file_verification": { ... },
  "errors": [],
  "warnings": []
}
```

## DryRunResult (Top-Level Object)

Represents the complete result of a dry-run execution.

### Fields

| Field | Type | Description |
|---------|------|------|
| `metadata` | ResultMetadata | Execution metadata |
| `status` | string | Execution status (`"success"` or `"error"`) |
| `phase` | string | Execution phase (`"completed"`, `"pre_execution"`, `"initialization"`, `"group_execution"`) |
| `error` | ExecutionError? | Top-level error information (only on error) |
| `summary` | ExecutionSummary | Execution summary statistics |
| `resource_analyses` | ResourceAnalysis[] | List of resource analysis results |
| `security_analysis` | SecurityAnalysis | Security analysis results |
| `environment_info` | EnvironmentInfo | Environment variable information |
| `file_verification` | FileVerificationSummary? | File verification summary (only when verification is performed) |
| `errors` | DryRunError[] | List of errors that occurred |
| `warnings` | DryRunWarning[] | List of warnings that occurred |

## ResultMetadata

Contains metadata about the dry-run result.

### Fields

| Field | Type | Description |
|---------|------|------|
| `generated_at` | string | Result generation timestamp (RFC3339 format) |
| `run_id` | string | Execution ID |
| `config_path` | string | Configuration file path |
| `environment_file` | string | Environment file path |
| `version` | string | Version information |
| `duration` | number | Execution duration (nanoseconds) |

## ExecutionSummary

Provides summary statistics for the execution.

### Fields

| Field | Type | Description |
|---------|------|------|
| `total_resources` | number | Total number of resources |
| `successful` | number | Number of successful resources |
| `failed` | number | Number of failed resources |
| `skipped` | number | Number of skipped resources |
| `groups` | Counts | Group statistics |
| `commands` | Counts | Command statistics |

### Counts

Provides counts for a specific resource type.

| Field | Type | Description |
|---------|------|------|
| `total` | number | Total count |
| `successful` | number | Successful count |
| `failed` | number | Failed count |
| `skipped` | number | Skipped count |

## ResourceAnalysis

Represents the analysis result of each resource (group or command).

### Fields

| Field | Type | Description |
|---------|------|------|
| `type` | string | Resource type (`"group"`, `"command"`, `"filesystem"`, `"privilege"`, `"network"`) |
| `operation` | string | Operation type (`"analyze"`, `"execute"`, `"create"`, `"delete"`, `"escalate"`, `"send"`) |
| `target` | string | Target identifier (e.g., group name, `group.command` format) |
| `status` | string | Execution status (`"success"` or `"error"`) |
| `error` | ExecutionError? | Error information (only on error) |
| `skip_reason` | string? | Skip reason (only when skipped) |
| `parameters` | object | Parameters for the resource operation |
| `impact` | ResourceImpact | Impact of the resource operation |
| `timestamp` | string | Timestamp (RFC3339 format) |
| `debug_info` | DebugInfo? | Debug information (included depending on detail level) |

### Parameters Field

The `parameters` field contains different content depending on the resource type and operation:

#### Group Analysis (type="group", operation="analyze")

| Field | Type | Description |
|---------|------|------|
| `group_name` | string | Group name |

#### Command Execution (type="command", operation="execute")

| Field | Type | Description |
|---------|------|------|
| `cmd` | string | Path to the command to execute |
| `args` | string | Arguments to pass to the command executed |
| `workdir` | string | Working directory |
| `timeout` | number | Timeout (nanoseconds) |
| `timeout_level` | string | Timeout level |
| `environment` | map[string]string? | Map of environment variables (only if present) |
| `group` | string? | Group name (only if within a group) |
| `group_description` | string? | Group description (only if within a group) |
| `run_as_user` | string? | Run-as user (only if specified) |
| `run_as_group` | string? | Run-as group (only if specified) |

#### Filesystem Operation - Temporary Directory Creation (type="filesystem", operation="create")

| Field | Type | Description |
|---------|------|------|
| `group_name` | string | Group name |
| `purpose` | string | Purpose (e.g., `"temporary_directory"`) |

#### Filesystem Operation - Directory Deletion (type="filesystem", operation="delete")

| Field | Type | Description |
|---------|------|------|
| `path` | string | Directory path |

#### Filesystem Operation - Output Capture (type="filesystem", operation="create")

| Field | Type | Description |
|---------|------|------|
| `output_path` | string | Output file path |
| `command` | string | Command path |
| `working_directory` | string | Working directory |
| `resolved_path` | string | Resolved path |
| `directory_exists` | boolean | Whether directory exists |
| `write_permission` | boolean | Whether write permission exists |
| `security_risk` | string | Security risk level |
| `max_size_limit` | number | Maximum size limit |

#### Privilege Operation (type="privilege", operation="escalate")

| Field | Type | Description |
|---------|------|------|
| `context` | string | Privilege escalation context (e.g., `"privilege_escalation"`) |

#### Network Operation (type="network", operation="send")

| Field | Type | Description |
|---------|------|------|
| `message` | string | Message to send |
| `details` | object | Message details |

## ResourceImpact

Describes the impact of a resource operation.

### Fields

| Field | Type | Description |
|---------|------|------|
| `reversible` | boolean | Whether the operation is reversible |
| `persistent` | boolean | Whether the operation is persistent |
| `security_risk` | string? | Security risk (`"low"`, `"medium"`, `"high"`) |
| `description` | string | Impact description |

## ExecutionError

Represents an execution error.

### Fields

| Field | Type | Description |
|---------|------|------|
| `type` | string | Error type |
| `message` | string | Error message |
| `component` | string | Component where error occurred |
| `details` | object? | Error details |

## DebugInfo

An object containing debug information. The content varies depending on the detail level (`-dry-run-detail`).

### Content by Detail Level

| Detail Level | `inheritance_analysis` | `final_environment` |
|----------|----------------------|-------------------|
| `summary` | None | None |
| `detailed` | Basic information only | None |
| `full` | Complete information including diff | Complete information |

### Fields

| Field | Type | Description | Occurrence Condition |
|---------|------|------|---------|
| `inheritance_analysis` | InheritanceAnalysis? | Analysis result of environment variable inheritance | Group resources with `detailed` or higher |
| `final_environment` | FinalEnvironment? | Final environment variables | Command resources with `full` only |

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
| `variables` | map[string]EnvironmentVariable | Map of environment variables (key is variable name) |

## EnvironmentVariable

Represents an individual environment variable.

### Fields

| Field | Type | Description |
|---------|------|------|
| `value` | string | Environment variable value (empty string for sensitive variables when `show_sensitive=false`) |
| `source` | string | Source of the value |
| `masked` | boolean? | Whether the value was masked (only `true` when sensitive data with `show_sensitive=false`) |

### Source Field Values

| Value | Description |
|----|------|
| `"system"` | Variable allowed from system environment by `env_allowlist` |
| `"vars"` | Variable defined in global or group-level `vars`/`env_import`/`env_vars` sections |
| `"command"` | Variable defined in command-level `env_vars` section |

**Note**: Currently, variables imported from `env_import` are not distinguished from `vars`. This is because variables from `env_import` are merged with `vars` during configuration expansion. Both are reported as `"vars"`. This is a known limitation that maintains simplicity in the current architecture.

## Sensitive Information Masking

By default (when `show_sensitive=false`), for environment variable names matching the following patterns:
- The `value` field is set to an empty string
- The `masked` field is set to `true`

Sensitive patterns:
- `*PASSWORD*`
- `*SECRET*`
- `*TOKEN*`
- `*KEY*`
- `*CREDENTIAL*`
- `*AUTH*`

Using the `--show-sensitive` flag sets the actual value in the `value` field and the `masked` field is not included.

## SecurityAnalysis

Contains security analysis results.

### Fields

| Field | Type | Description |
|---------|------|------|
| `risks` | SecurityRisk[] | List of security risks |
| `privilege_changes` | PrivilegeChange[] | List of privilege changes |
| `environment_access` | EnvironmentAccess[] | List of environment variable accesses |
| `file_access` | FileAccess[] | List of file accesses |

### SecurityRisk

Represents an individual security risk.

| Field | Type | Description |
|---------|------|------|
| `level` | string | Risk level (`"low"`, `"medium"`, `"high"`) |
| `type` | string | Risk type (`"privilege_escalation"`, `"dangerous_command"`, `"data_exposure"`) |
| `description` | string | Risk description |
| `command` | string | Related command |
| `group` | string | Related group |
| `mitigation` | string | Risk mitigation |

### PrivilegeChange

Represents a privilege change.

| Field | Type | Description |
|---------|------|------|
| `group` | string | Group name |
| `command` | string | Command name |
| `from_user` | string | User before change |
| `to_user` | string | User after change |
| `mechanism` | string | Change mechanism |

### EnvironmentAccess

Represents environment variable access.

| Field | Type | Description |
|---------|------|------|
| `variable` | string | Variable name |
| `access_type` | string | Access type (`"read"`, `"write"`) |
| `commands` | string[] | List of commands accessing the variable |
| `groups` | string[] | List of groups accessing the variable |
| `sensitive` | boolean | Whether the variable is sensitive |

### FileAccess

Represents file access.

| Field | Type | Description |
|---------|------|------|
| `path` | string | File path |
| `access_type` | string | Access type (`"read"`, `"write"`, `"execute"`) |
| `commands` | string[] | List of commands accessing the file |
| `groups` | string[] | List of groups accessing the file |

## EnvironmentInfo

Contains information about environment variables.

### Fields

| Field | Type | Description |
|---------|------|------|
| `total_variables` | number | Total number of environment variables |
| `allowed_variables` | string[] | List of allowed variable names |
| `filtered_variables` | string[] | List of filtered variable names |
| `variable_usage` | map[string]string[] | Map of variable names to commands using them |

## FileVerificationSummary

Contains file verification result summary. Only included when verification is performed.

### Fields

| Field | Type | Description |
|---------|------|------|
| `total_files` | number | Total number of files to verify |
| `verified_files` | number | Number of files successfully verified |
| `skipped_files` | number | Number of files skipped |
| `failed_files` | number | Number of files that failed verification |
| `duration` | number | Verification processing time (nanoseconds) |
| `hash_dir_status` | HashDirectoryStatus | Hash directory status |
| `failures` | FileVerificationFailure[]? | List of verification failures (only when failures occur) |

### HashDirectoryStatus

Represents the status of the hash directory.

| Field | Type | Description |
|---------|------|------|
| `path` | string | Hash directory path |
| `exists` | boolean | Whether the directory exists |
| `validated` | boolean | Whether the directory has been validated |

### FileVerificationFailure

Represents an individual file verification failure.

| Field | Type | Description |
|---------|------|------|
| `path` | string | File path |
| `reason` | string | Failure reason (`"hash_directory_not_found"`, `"hash_file_not_found"`, `"hash_mismatch"`, `"file_read_error"`, `"permission_denied"`) |
| `level` | string | Severity level |
| `message` | string | Error message |
| `context` | string | Context information |

## DryRunError

Represents an error that occurred during dry-run execution.

### Fields

| Field | Type | Description |
|---------|------|------|
| `type` | string | Error type (`"configuration_error"`, `"verification_error"`, `"variable_error"`, `"security_error"`, `"system_error"`, `"execution_error"`) |
| `code` | string | Error code |
| `message` | string | Error message |
| `component` | string | Component where error occurred |
| `group` | string? | Related group name (only for group-level errors) |
| `command` | string? | Related command name (only for command-level errors) |
| `details` | object? | Error details |
| `recoverable` | boolean | Whether the error is recoverable |

## DryRunWarning

Represents a warning that occurred during dry-run execution.

### Fields

| Field | Type | Description |
|---------|------|------|
| `type` | string | Warning type (`"deprecated_feature"`, `"security_concern"`, `"performance_concern"`, `"compatibility"`) |
| `message` | string | Warning message |
| `component` | string | Component where warning occurred |
| `group` | string? | Related group name (only for group-level warnings) |
| `command` | string? | Related command name (only for command-level warnings) |

## Usage Examples

### DetailLevelSummary

```json
{
  "metadata": {
    "generated_at": "2025-11-23T10:00:00Z",
    "run_id": "abc123",
    "config_path": "/path/to/config.toml",
    "environment_file": "",
    "version": "1.0.0",
    "duration": 1500000000
  },
  "status": "success",
  "phase": "completed",
  "summary": {
    "total_resources": 1,
    "successful": 1,
    "failed": 0,
    "skipped": 0,
    "groups": {
      "total": 0,
      "successful": 0,
      "failed": 0,
      "skipped": 0
    },
    "commands": {
      "total": 1,
      "successful": 1,
      "failed": 0,
      "skipped": 0
    }
  },
  "resource_analyses": [
    {
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:00Z"
    }
  ],
  "security_analysis": {},
  "environment_info": {},
  "errors": [],
  "warnings": []
}
```

The `debug_info` field is not included.

### DetailLevelDetailed

```json
{
  "metadata": { ... },
  "status": "success",
  "phase": "completed",
  "summary": { ... },
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "backup",
      "status": "success",
      "parameters": {},
      "impact": {
        "reversible": true,
        "persistent": false,
        "description": "Configuration analysis only"
      },
      "timestamp": "2025-11-23T10:00:00Z",
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR"],
          "inheritance_mode": "inherit"
        }
      }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:01Z"
    }
  ],
  "security_analysis": {},
  "environment_info": {},
  "errors": [],
  "warnings": []
}
```

Basic debug information is included. At `detailed` level, `final_environment` is not included for command resources.

### DetailLevelFull

```json
{
  "metadata": { ... },
  "status": "success",
  "phase": "completed",
  "summary": { ... },
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "backup",
      "status": "success",
      "parameters": {},
      "impact": {
        "reversible": true,
        "persistent": false,
        "description": "Configuration analysis only"
      },
      "timestamp": "2025-11-23T10:00:00Z",
      "debug_info": {
        "inheritance_analysis": {
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
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "workdir": "/var/backups",
        "timeout": 3600000000000,
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:01Z",
      "debug_info": {
        "final_environment": {
          "variables": {
            "BACKUP_DIR": {
              "value": "/var/backups",
              "source": "vars"
            },
            "DB_PASSWORD": {
              "value": "",
              "source": "command",
              "masked": true
            },
            "HOME": {
              "value": "/root",
              "source": "system"
            },
            "PATH": {
              "value": "/usr/local/bin:/usr/bin:/bin",
              "source": "system"
            }
          }
        }
      }
    }
  ],
  "security_analysis": {},
  "environment_info": {},
  "errors": [],
  "warnings": []
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
  jq '.resource_analyses[] | select(.debug_info.inheritance_analysis != null) | .debug_info.inheritance_analysis.inheritance_mode'
```

### Check Final Environment Variables of a Specific Command

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.target == "backup.db_backup") | .debug_info.final_environment.variables'
```

### Get List of Inherited Variables

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.inheritance_analysis.inherited_variables != null) | .debug_info.inheritance_analysis.inherited_variables[]'
```

### Identify Commands with Sensitive Variables

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.final_environment != null) | select(.debug_info.final_environment.variables | to_entries[] | select(.value.masked == true)) | .target'
```

---

**Last Updated**: 2025-11-23
