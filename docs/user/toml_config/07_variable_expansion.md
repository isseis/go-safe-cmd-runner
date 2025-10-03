# Chapter 7: Variable Expansion Feature

## 7.1 Overview

Variable expansion allows dynamic construction of command parameters using environment variables.

## 7.2 Syntax

Variables are referenced using `${VARIABLE_NAME}` syntax:

```toml
[[groups.commands]]
name = "deploy"
cmd = "/opt/scripts/deploy.sh"
args = ["--env", "${ENVIRONMENT}", "--version", "${APP_VERSION}"]
```

## 7.3 Available Locations

### Command Path (cmd)
```toml
[[groups.commands]]
name = "dynamic_tool"
cmd = "/opt/tools/${TOOL_NAME}"
args = ["--config", "config.yml"]
```

### Arguments (args)
```toml
[[groups.commands]]
name = "backup"
cmd = "/bin/tar"
args = ["-czf", "backup-${DATE}.tar.gz", "${SOURCE_DIR}"]
```

### Multiple Variables
```toml
[[groups.commands]]
name = "process"
cmd = "/opt/processors/${PROCESSOR_TYPE}"
args = [
    "--input", "${INPUT_DIR}/data-${DATE}.json",
    "--output", "${OUTPUT_DIR}/processed-${DATE}.json"
]
```

## 7.4 Security Considerations

### Environment Variable Allowlist
Variables must be in the allowlist to be expanded:

```toml
[global]
env_allowlist = ["PATH", "ENVIRONMENT", "APP_VERSION", "DATE"]

[[groups.commands]]
name = "deploy"
cmd = "/opt/scripts/deploy.sh"
args = ["--env", "${ENVIRONMENT}"]  # ✓ Allowed
# args = ["--user", "${USER}"]      # ✗ Not in allowlist
```

### Validation
- Variables are validated before expansion
- Invalid variables cause configuration rejection
- Empty variables are treated as errors

## 7.5 Practical Examples

### Environment-Specific Deployment
```toml
[global]
env_allowlist = ["ENVIRONMENT", "CONFIG_DIR"]

[[groups.commands]]
name = "deploy_app"
cmd = "/opt/scripts/deploy.sh"
args = [
    "--environment", "${ENVIRONMENT}",
    "--config", "${CONFIG_DIR}/app-${ENVIRONMENT}.conf"
]
```

### Date-Based Backup
```toml
[global]
env_allowlist = ["BACKUP_DATE", "BACKUP_DIR"]

[[groups.commands]]
name = "daily_backup"
cmd = "/bin/tar"
args = [
    "-czf",
    "${BACKUP_DIR}/backup-${BACKUP_DATE}.tar.gz",
    "/opt/data"
]
```

### Tool Selection
```toml
[global]
env_allowlist = ["PROCESSOR_TYPE", "INPUT_FILE"]

[[groups.commands]]
name = "process_data"
cmd = "/opt/tools/${PROCESSOR_TYPE}"
args = ["--input", "${INPUT_FILE}"]
```

## 7.6 Best Practices

1. **Validate Variables**: Ensure all used variables are in allowlist
2. **Use Descriptive Names**: Clear variable names improve maintainability
3. **Document Requirements**: Document required environment variables
4. **Test Expansion**: Use dry-run to verify variable expansion
5. **Escape Special Characters**: Use appropriate escaping when needed

Variable expansion provides powerful flexibility while maintaining security through allowlist validation.
