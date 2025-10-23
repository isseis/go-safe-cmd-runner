# Appendix

## Appendix A: Parameter Reference

### A.1 Root Level Parameters

| Parameter | Type | Required | Default Value | Description |
|-----------|------|----------|---------------|-------------|
| version | string | ✓ | none | Configuration file version (current: "1.0") |

### A.2 Global Level Parameters ([global])

| Parameter | Type | Required | Default Value | Description |
|-----------|------|----------|---------------|-------------|
| timeout | int | - | System default | Command execution timeout (seconds) |
| workdir | string | - | Execution directory | Absolute path of working directory |
| log_level | string | - | "info" | Log level (debug/info/warn/error) |
| verify_standard_paths | bool | - | true | Verify standard path validation |
| env_allowed | []string | - | [] | Environment variable allowlist |
| verify_files | []string | - | [] | List of files to verify |
| output_size_limit | int64 | - | 10485760 | Maximum output size (bytes) |

### A.3 Group Level Parameters ([[groups]])

| Parameter | Type | Required | Default Value | Description |
|-----------|------|----------|---------------|-------------|
| name | string | ✓ | none | Group name (unique) |
| description | string | - | "" | Group description |
| priority | int | - | 0 | Execution priority (lower runs first) |
| workdir | string | - | Auto-generated | Working directory (auto-generates temporary directory if not specified) |
| verify_files | []string | - | [] | Files to verify (added to global) |
| env_allowed | []string | - | nil (inherit) | Environment variable allowlist (see inheritance mode) |

### A.4 Command Level Parameters ([[groups.commands]])

| Parameter | Type | Required | Default Value | Description |
|-----------|------|----------|---------------|-------------|
| name | string | ✓ | none | Command name (unique within group) |
| description | string | - | "" | Command description |
| cmd | string | ✓ | none | Command to execute (absolute path or in PATH) |
| args | []string | - | [] | Command arguments |
| env_vars | []string | - | [] | Environment variables ("KEY=VALUE" format) |
| workdir | string | - | Group setting | Working directory (overrides group setting) |
| timeout | int | - | Global setting | Timeout (overrides global) |
| run_as_user | string | - | "" | User to run as |
| run_as_group | string | - | "" | Group to run as |
| risk_level | string | - | "low" | Maximum risk level (low/medium/high) |
| output_file | string | - | "" | File path to save standard output |

### A.5 Environment Variable Inheritance Modes

| Mode | Condition | Behavior |
|------|-----------|----------|
| Inherit | env_allowed is undefined (nil) | Inherit global settings |
| Explicit | env_allowed has value set | Use only configured values (ignore global) |
| Reject | env_allowed = [] (empty array) | Reject all environment variables |

### A.6 Risk Levels

| Level | Value | Description | Examples |
|-------|-------|-------------|----------|
| Low Risk | "low" | Read-only operations | cat, ls, grep, echo |
| Medium Risk | "medium" | File creation/modification | tar, cp, mkdir, wget |
| High Risk | "high" | System changes/privilege escalation | apt-get, systemctl, rm -rf |

## Appendix B: Sample Configuration Files

### B.1 Minimal Configuration

```toml
version = "1.0"

[[groups]]
name = "minimal"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
args = ["Hello, World!"]
```

### B.2 Basic Backup

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/var/backups"
log_level = "info"
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "daily_backup"

[[groups.commands]]
name = "backup_data"
cmd = "/bin/tar"
args = ["-czf", "data-backup.tar.gz", "/opt/data"]

[[groups.commands]]
name = "backup_config"
cmd = "/bin/tar"
args = ["-czf", "config-backup.tar.gz", "/etc/myapp"]
```

### B.3 Secure Configuration

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/opt/secure"
log_level = "info"
verify_standard_paths = true
env_allowed = ["PATH"]
verify_files = []  # Commands are automatically verified

[[groups]]
name = "secure_backup"
verify_files = ["/opt/secure/config/backup.conf"]  # Only specify additional files

[[groups.commands]]
name = "backup"
cmd = "/opt/secure/bin/backup-tool"  # Automatically verified
args = ["--encrypt", "--output", "backup.enc"]
risk_level = "medium"
```

### B.4 Variable Expansion

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "APP_DIR", "ENV_TYPE"]

[[groups]]
name = "deployment"

[[groups.commands]]
name = "deploy"
cmd = "${APP_DIR}/bin/deploy"
args = ["--env", "${ENV_TYPE}", "--config", "${APP_DIR}/config/${ENV_TYPE}.yml"]
env_vars = [
    "APP_DIR=/opt/myapp",
    "ENV_TYPE=production",
]
```

### B.5 Privilege Management

```toml
version = "1.0"

[global]
timeout = 600
log_level = "info"
env_allowed = ["PATH"]

[[groups]]
name = "system_maintenance"

[[groups.commands]]
name = "check_status"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp"]
risk_level = "low"

[[groups.commands]]
name = "restart_service"
cmd = "/usr/bin/systemctl"
args = ["restart", "myapp"]
run_as_user = "root"
risk_level = "high"
```

### B.6 Output Capture

```toml
version = "1.0"

[global]
workdir = "/var/reports"
output_size_limit = 10485760

[[groups]]
name = "system_report"

[[groups.commands]]
name = "disk_usage"
cmd = "/bin/df"
args = ["-h"]
output_file = "disk-usage.txt"

[[groups.commands]]
name = "memory_usage"
cmd = "/usr/bin/free"
args = ["-h"]
output_file = "memory-usage.txt"
```

### B.7 Multi-Environment Support

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "APP_BIN", "CONFIG_DIR", "ENV_TYPE", "DB_URL"]

# Development environment
[[groups]]
name = "dev_deploy"
priority = 1

[[groups.commands]]
name = "run_dev"
cmd = "${APP_BIN}"
args = ["--config", "${CONFIG_DIR}/${ENV_TYPE}.yml", "--db", "${DB_URL}"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "CONFIG_DIR=/etc/app",
    "ENV_TYPE=development",
    "DB_URL=postgresql://localhost/dev_db",
]

# Production environment
[[groups]]
name = "prod_deploy"
priority = 2

[[groups.commands]]
name = "run_prod"
cmd = "${APP_BIN}"
args = ["--config", "${CONFIG_DIR}/${ENV_TYPE}.yml", "--db", "${DB_URL}"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "CONFIG_DIR=/etc/app",
    "ENV_TYPE=production",
    "DB_URL=postgresql://prod-db/prod_db",
]
run_as_user = "appuser"
risk_level = "high"
```

## Appendix C: Glossary

### C.1 General Terms

**TOML (Tom's Obvious, Minimal Language)**
: A human-readable configuration file format with clear syntax and data types.

**Absolute Path**
: A complete file path starting from the root directory (`/`). Example: `/usr/bin/tool`

**Relative Path**
: A file path relative to the current directory. Example: `./tool`, `../bin/tool`

**Environment Variable**
: A dynamic key-value pair (KEY=VALUE) provided by the operating system.

**Timeout**
: The maximum execution time for a command. Commands are forcibly terminated when this time is exceeded.

### C.2 Configuration Terms

**Global Configuration**
: Common settings applied to all groups and commands. Defined in the `[global]` section.

**Group**
: A logical unit that organizes related commands. Defined with `[[groups]]`.

**Command**
: The definition of a command to actually execute. Defined with `[[groups.commands]]`.

**Priority**
: A numeric value that controls the execution order of groups. Lower values execute first.

### C.3 Security Terms

**File Verification**
: A feature that detects tampering by comparing file hash values.

**Environment Variable Allowlist**
: A list of environment variables permitted for use. Variables not in the list are excluded.

**Principle of Least Privilege**
: A security principle of granting only the minimum necessary permissions.

**Privilege Escalation**
: Executing a command with higher privileges (such as root).

**Risk Level**
: An indicator of a command's security risk (low/medium/high).

### C.4 Variable Expansion Terms

**Variable Expansion**
: The process of replacing variables in `${VAR}` format with actual values.

**Command.Env**
: Environment variables defined at the command level. Set with the `env` parameter.

**Inheritance Mode**
: A mode that determines how environment variable allowlists are handled at the group level.

**Nested Variable**
: A nested structure where a variable's value contains another variable. Example: `VAR1=${VAR2}/path`

**Escape Sequence**
: A notation for treating special characters literally. Example: `\$`, `\\`

### C.5 Execution Terms

**Dry Run**
: A mode that displays the execution plan without actually executing.

**Working Directory**
: The current directory where commands are executed. Set with `workdir`.

**Temporary Directory**
: A working directory automatically created and managed by the runner. Accessible via the `%{__runner_workdir}` variable.

**Output Capture**
: A feature that saves command standard output to a file. Set with the `output` parameter.

**Standard Output (stdout)**
: The stream to which commands send normal output.

**Standard Error (stderr)**
: The stream to which commands send error messages.

### C.6 Override and Inheritance

**Override**
: When lower-level settings replace higher-level settings.

**Merge**
: Combining multiple settings. Example: Combining global and group `verify_files`.

**Inheritance**
: When lower levels inherit higher-level settings.

## Appendix D: Configuration File Templates

### D.1 Basic Template

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/path/to/workdir"
log_level = "info"
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "group_name"
description = "Group description"

[[groups.commands]]
name = "command_name"
description = "Command description"
cmd = "/path/to/command"
args = ["arg1", "arg2"]
```

### D.2 Secure Configuration Template

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/opt/secure"
log_level = "info"
verify_standard_paths = true
env_allowed = ["PATH"]
verify_files = [
    # Additional verification files (commands are automatically verified)
]

[[groups]]
name = "secure_group"
description = "Secure operations group"
verify_files = [
    # Group-specific verification files (e.g., config files, libraries)
]

[[groups.commands]]
name = "secure_command"
description = "Secure command"
cmd = "/path/to/verified/command"
args = []
risk_level = "medium"
```

### D.3 Variable Expansion Template

```toml
version = "1.0"

[global]
env_allowed = [
    "PATH",
    "HOME",
    # Additional allowed variables
]

[[groups]]
name = "variable_group"

[[groups.commands]]
name = "command_with_vars"
cmd = "${TOOL_DIR}/tool"
args = [
    "--config", "${CONFIG_FILE}",
    "--output", "${OUTPUT_DIR}/result.txt",
]
env_vars = [
    "TOOL_DIR=/opt/tools",
    "CONFIG_FILE=/etc/app/config.yml",
    "OUTPUT_DIR=/var/output",
]
```

### D.4 Multi-Environment Template

```toml
version = "1.0"

[global]
env_allowed = [
    "PATH",
    "APP_BIN",
    "ENV_TYPE",
    "CONFIG_DIR",
]

# Development environment
[[groups]]
name = "dev_environment"
priority = 1

[[groups.commands]]
name = "run_dev"
cmd = "${APP_BIN}"
args = ["--env", "${ENV_TYPE}", "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "ENV_TYPE=development",
    "CONFIG_DIR=/etc/app/configs",
]

# Production environment
[[groups]]
name = "prod_environment"
priority = 2

[[groups.commands]]
name = "run_prod"
cmd = "${APP_BIN}"
args = ["--env", "${ENV_TYPE}", "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "ENV_TYPE=production",
    "CONFIG_DIR=/etc/app/configs",
]
run_as_user = "appuser"
risk_level = "high"
```

## Appendix E: Reference Links

### E.1 Official Resources

- **Project Repository**: `github.com/isseis/go-safe-cmd-runner`
- **Sample Configurations**: `sample/` directory
- **Developer Documentation**: `docs/dev/` directory

### E.2 Related Technologies

- **TOML Specification**: https://toml.io/
- **Go Language**: https://golang.org/
- **Security Best Practices**: OWASP Secure Coding Practices

### E.3 Community

- **Issue Tracker**: Bug reports and feature requests via GitHub Issues
- **Pull Requests**: Improvement proposals and contributions are welcome

## Conclusion

This document is a complete guide to TOML configuration files for go-safe-cmd-runner. It is structured to provide progressive learning from basic concepts to advanced usage.

### Recommended Learning Path

1. **Chapters 1-3**: Understand basic concepts and structure
2. **Chapters 4-6**: Learn parameters at each level in detail
3. **Chapter 7**: Master variable expansion features
4. **Chapters 8-9**: Acquire practical examples and best practices
5. **Chapter 10**: Learn troubleshooting techniques
6. **Appendix**: Use as a reference

### Further Learning

- Refer to examples in the `sample/` directory
- Start with small configurations in actual environments
- Progressively add complexity while checking behavior with dry runs
- Post questions and improvement suggestions to the community

We hope this document helps you build a safe and efficient command execution environment.
