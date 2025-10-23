# Chapter 9: Best Practices

This chapter introduces best practices for creating go-safe-cmd-runner configuration files. It provides guidance for creating better configuration files from the perspectives of security, maintainability, and performance.

## 9.1 Security Best Practices

### 9.1.1 Principle of Least Privilege

Execute commands with the minimum necessary privileges.

#### Recommended Implementation

```toml
# Good example: Use only necessary privileges
[[groups.commands]]
name = "read_log"
cmd = "/bin/cat"
args = ["/var/log/app/app.log"]
run_as_group = "loggroup"  # Only privileges needed for log reading
risk_level = "low"

# Example to avoid: Excessive privileges
[[groups.commands]]
name = "read_log"
cmd = "/bin/cat"
args = ["/var/log/app/app.log"]
run_as_user = "root"  # Unnecessarily using root privileges
```

### 9.1.2 Strict Management of Environment Variables

Set the environment variable allowlist to the minimum necessary.

#### Recommended Implementation

```toml
# Good example: Allow only necessary variables
[global]
env_allowed = [
    "PATH",           # Required for command search
    "HOME",           # Used for config file search
    "APP_CONFIG_DIR", # App-specific configuration
]

# Example to avoid: Overly permissive configuration
[global]
env_allowed = [
    "PATH", "HOME", "USER", "SHELL", "EDITOR", "PAGER",
    "MAIL", "LOGNAME", "HOSTNAME", "DISPLAY", "XAUTHORITY",
    # ... too many
]
```

### 9.1.3 Utilizing File Verification

Always verify important configuration files and libraries. Command executables are automatically verified.

#### Recommended Implementation

```toml
# Good example: Verify configuration files and scripts
[global]
verify_standard_paths = true  # Also verify commands in standard paths
verify_files = [
    "/etc/app/global.conf",  # Global configuration file
]

[[groups]]
name = "critical_operations"
verify_files = [
    "/opt/app/config/critical.conf",  # Important configuration file
    "/opt/app/lib/helper.sh",         # Helper script
]
# Note: Commands themselves are automatically verified, no need to add them to verify_files
```

### 9.1.4 Using Absolute Paths

Specify commands with absolute paths.

#### Recommended Implementation

```toml
# Good example: Absolute path
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]

# Example to avoid: PATH dependency
[[groups.commands]]
name = "backup"
cmd = "tar"  # Unclear which tar will be executed
args = ["-czf", "backup.tar.gz", "/data"]
```

### 9.1.5 Handling Sensitive Information

Manage sensitive information with Command.Env and isolate it from the system environment.

#### Recommended Implementation

```toml
# Good example: Use vars and env appropriately
[global]
env_allowed = ["PATH", "HOME"]  # Don't include sensitive information

[[groups.commands]]
name = "api_call"
cmd = "/usr/bin/curl"
vars = [
    "api_token=sk-secret123",
    "api_endpoint=https://api.example.com",
]
args = [
    "-H", "Authorization: Bearer %{api_token}",
    "%{api_endpoint}",
]
env_vars = ["API_TOKEN=%{api_token}"]  # Set as environment variable if needed

# Example to avoid: Allowing sensitive information globally
[global]
env_allowed = ["PATH", "HOME", "API_TOKEN"]  # Dangerous!
```

### 9.1.6 Appropriate Risk Level Settings

Set appropriate risk levels according to the nature of the command.

#### Recommended Implementation

```toml
# Read-only: low
[[groups.commands]]
name = "read_config"
cmd = "/bin/cat"
args = ["/etc/app/config.yml"]
risk_level = "low"

# File creation/modification: medium
[[groups.commands]]
name = "create_backup"
cmd = "/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
risk_level = "medium"

# System changes/privilege escalation: high
[[groups.commands]]
name = "install_package"
cmd = "/usr/bin/apt-get"
args = ["install", "-y", "package"]
run_as_user = "root"
risk_level = "high"
```

## 9.2 Environment Variable Management Best Practices

### 9.2.1 Appropriate Use of Inheritance Modes

Use environment variable inheritance modes according to purpose.

#### Usage Guidelines

```toml
[global]
env_allowed = ["PATH", "HOME", "USER"]

# Pattern 1: Inheritance mode - When global settings are sufficient
[[groups]]
name = "standard_group"
# env_allowed unspecified → Inherits from global

# Pattern 2: Explicit mode - When group-specific variables are needed
[[groups]]
name = "database_group"
env_allowed = ["PATH", "DB_HOST", "DB_USER"]  # Different from global

# Pattern 3: Deny mode - When complete isolation is needed
[[groups]]
name = "isolated_group"
env_allowed = []  # Deny all environment variables
```

### 9.2.2 Variable Naming Conventions

Use consistent naming conventions.

#### Recommended Naming Conventions

```toml
# Good example: Consistent naming conventions
env_vars = [
    "APP_DIR=/opt/myapp",           # APP_ prefix for app-related
    "APP_CONFIG=/etc/myapp/config.yml",
    "APP_LOG_DIR=/var/log/myapp",
    "DB_HOST=localhost",            # DB_ prefix for database-related
    "DB_PORT=5432",
    "DB_NAME=myapp_db",
    "BACKUP_DIR=/var/backups",      # BACKUP_ prefix for backup-related
    "BACKUP_RETENTION_DAYS=30",
]

# Example to avoid: Inconsistent naming
env_vars = [
    "app_directory=/opt/myapp",     # Lowercase with underscore
    "APPCONFIG=/etc/myapp/config.yml",  # No prefix
    "log-dir=/var/log/myapp",       # Using hyphens
    "DatabaseHost=localhost",       # CamelCase
]
```

### 9.2.3 Variable Reuse

Define common values as variables and reuse them.

#### Recommended Implementation

```toml
# Good example: Variable reuse using vars
[global]
vars = ["config_dest=/etc/myapp"]

[[groups.commands]]
name = "deploy_config"
cmd = "/bin/cp"
vars = ["config_source=/opt/configs/prod"]
args = ["%{config_source}/app.yml", "%{config_dest}/app.yml"]

[[groups.commands]]
name = "backup_config"
cmd = "/bin/cp"
vars = ["backup_dir=/var/backups"]
args = ["%{config_dest}/app.yml", "%{backup_dir}/app.yml"]
# config_dest is inherited from global, no need to redefine
```

## 9.3 Group Organization Best Practices

### 9.3.1 Logical Grouping

Logically group related commands.

#### Recommended Structure

```toml
# Good example: Logical grouping
[[groups]]
name = "database_operations"
description = "All database-related operations"
# ... database-related commands

[[groups]]
name = "file_operations"
description = "All file operation tasks"
# ... file operation-related commands

[[groups]]
name = "network_operations"
description = "All network communication tasks"
# ... network-related commands

# Example to avoid: Disorganized grouping
[[groups]]
name = "group1"
# Mixed database, file, and network operations
```

### 9.3.2 Effective Use of Priority

Set priorities based on dependencies and importance.

#### Recommended Implementation

```toml
# Good example: Clear priority settings
[[groups]]
name = "prerequisites"
description = "Verify prerequisites"
priority = 1  # Execute first

[[groups]]
name = "main_operations"
description = "Main processing"
priority = 10  # Execute after prerequisites

[[groups]]
name = "cleanup"
description = "Post-processing and cleanup"
priority = 100  # Execute last
```

### 9.3.3 Comprehensive Descriptions

Write clear descriptions for each group and command.

#### Recommended Implementation

```toml
# Good example: Detailed descriptions
[[groups]]
name = "database_backup"
description = "PostgreSQL database daily backup process. Dumps all databases, then compresses and encrypts for storage."

[[groups.commands]]
name = "full_backup"
description = "Full backup of all databases (pg_dump --all-databases)"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases"]

# Example to avoid: Insufficient descriptions
[[groups]]
name = "db_backup"
description = "Backup"  # Unclear what is being backed up

[[groups.commands]]
name = "backup"
description = "Run backup"  # Lacks specificity
```

## 9.4 Error Handling Best Practices

### 9.4.1 Appropriate Timeout Settings

Set appropriate timeouts according to the nature of the command.

#### Recommended Implementation

```toml
[global]
timeout = 300  # Default: 5 minutes

[[groups.commands]]
name = "quick_check"
cmd = "/bin/ping"
args = ["-c", "3", "localhost"]
timeout = 10  # Short timeout for quick commands

[[groups.commands]]
name = "database_dump"
cmd = "/usr/bin/pg_dump"
args = ["large_database"]
timeout = 3600  # Longer timeout for large databases

[[groups.commands]]
name = "file_sync"
cmd = "/usr/bin/rsync"
args = ["-av", "/source", "/dest"]
timeout = 7200  # Sufficient time for network transfers
```

### 9.4.2 Appropriate Output Size Limits

Set output size limits according to the amount of data being processed.

#### Recommended Implementation

```toml
# For log file analysis with large output
[global]
output_size_limit = 104857600  # 100MB

# For small output only
[global]
output_size_limit = 1048576  # 1MB
```

## 9.5 Maintainability Best Practices

### 9.5.1 Using Comments

Document the intent and precautions in comments.

#### Recommended Implementation

```toml
# Production environment deployment configuration
# Last updated: 2025-10-02
# Owner: DevOps Team
version = "1.0"

[global]
# Timeout set to 1.5x the longest execution time
timeout = 900

# Information-level logs only in production
log_level = "info"

# Due to security requirements, verify system paths as well
verify_standard_paths = true

[[groups]]
name = "production_deployment"
# Warning: Execute this group only in production environment
# Use deploy_staging group for staging environment
```

### 9.5.2 Configuration Structuring

Structure the configuration logically to improve readability.

#### Recommended Implementation

```toml
version = "1.0"

# ========================================
# Global Settings
# ========================================
[global]
timeout = 600
workdir = "/opt/deploy"
log_level = "info"
env_allowed = ["PATH", "HOME"]

# ========================================
# Phase 1: Preparation
# ========================================
[[groups]]
name = "preparation"
priority = 1
# ... command definitions

# ========================================
# Phase 2: Deployment
# ========================================
[[groups]]
name = "deployment"
priority = 2
# ... command definitions

# ========================================
# Phase 3: Verification
# ========================================
[[groups]]
name = "verification"
priority = 3
# ... command definitions
```

### 9.5.3 Splitting Configuration Files

Consider splitting large configurations into multiple files.

#### Recommended Structure

```
configs/
├── base.toml              # Common settings
├── development.toml       # Development environment-specific settings
├── staging.toml           # Staging environment-specific settings
└── production.toml        # Production environment-specific settings
```

Use appropriate configuration files for each environment:
```bash
# Development environment
go-safe-cmd-runner -file configs/development.toml

# Production environment
go-safe-cmd-runner -file configs/production.toml
```

## 9.6 Performance Best Practices

### 9.6.1 Considering Parallel Execution

Design independent groups to enable parallel execution.

#### Recommended Implementation

```toml
# Good example: Independent groups (parallel execution possible)
[[groups]]
name = "backup_database"
priority = 10
# Database backup

[[groups]]
name = "backup_files"
priority = 10  # Same priority → Parallel execution possible
# File backup

# Example to avoid: Unnecessary dependencies
[[groups]]
name = "backup_database"
priority = 10

[[groups]]
name = "backup_files"
priority = 11  # Unnecessarily creating dependencies
```

### 9.6.2 Optimizing File Verification

Specify only files that need verification.

#### Recommended Implementation

```toml
# Good example: Verify only necessary files
[global]
verify_standard_paths = false  # Skip standard paths
verify_files = [
    "/opt/app/bin/critical-tool",  # Verify only app-specific tools
]

# Example to avoid: Excessive verification
[global]
verify_standard_paths = true
verify_files = [
    "/bin/ls", "/bin/cat", "/bin/grep", "/bin/sed",
    # ... Many standard commands (performance degradation)
]
```

### 9.6.3 Appropriate Use of Output Capture

Capture output only when necessary.

#### Recommended Implementation

```toml
# Good example: Capture only necessary output
[[groups.commands]]
name = "system_info"
cmd = "/bin/df"
args = ["-h"]
output_file = "disk-usage.txt"  # Needed for report generation

[[groups.commands]]
name = "simple_echo"
cmd = "/bin/echo"
args = ["Processing..."]
# output unspecified → No capture (display on stdout)

# Example to avoid: Unnecessary output capture
[[groups.commands]]
name = "simple_echo"
cmd = "/bin/echo"
args = ["Processing..."]
output_file = "echo-output.txt"  # Unnecessary capture (waste of resources)
```

## 9.7 Testing and Validation

### 9.7.1 Incremental Testing

Test configuration files incrementally.

#### Recommended Procedure

1. **Start with basic commands**
```toml
# Step 1: Test with minimal configuration
[[groups.commands]]
name = "test_basic"
cmd = "/bin/echo"
args = ["test"]
```

2. **Gradually increase complexity**
```toml
# Step 2: Add variable expansion
[[groups.commands]]
name = "test_variables"
cmd = "/bin/echo"
vars = ["test_var=hello"]
args = ["Value: %{test_var}"]
```

3. **Production-equivalent configuration**
```toml
# Step 3: Test with full configuration
[[groups.commands]]
name = "production_command"
cmd = "/opt/app/bin/tool"
vars = ["config=/etc/app/config.yml"]
args = ["--config", "%{config}"]
run_as_user = "appuser"
risk_level = "high"
```

### 9.7.2 Utilizing Dry Run Feature

Verify behavior with dry run before production execution.

```bash
# Validate configuration with dry run
go-safe-cmd-runner --dry-run --file config.toml

# Execute in production if no issues
go-safe-cmd-runner -file config.toml
```

## 9.8 Documentation

### 9.8.1 Documenting Configuration Files

Create a README along with the configuration file.

#### README.md Example

```markdown
# Application Deployment Configuration

## Overview
Configuration file to automate application deployment to production environment.

## Prerequisites
- PostgreSQL must be installed
- /opt/app directory must exist
- appuser user must exist

## Execution
```bash
go-safe-cmd-runner -file production-deploy.toml
```

## Environment Variables
Set the following environment variables:
- `DB_PASSWORD`: Database password
- `API_KEY`: External API key

## Troubleshooting
### Database Connection Error
- Check if PostgreSQL service is running
- Verify authentication credentials are correct
```

### 9.8.2 Recording Change History

Record configuration file change history in comments.

```toml
# Change history:
# 2025-10-02: Extended timeout from 300s → 600s (to support large DBs)
# 2025-09-15: Added encryption processing
# 2025-09-01: Initial version created

version = "1.0"
# ...
```

## Summary

Best practices introduced in this chapter:

1. **Security**: Least privilege, strict environment variable management, file verification, absolute path usage
2. **Environment Variable Management**: Appropriate inheritance modes, consistent naming conventions, variable reuse
3. **Group Organization**: Logical grouping, effective use of priority, comprehensive descriptions
4. **Error Handling**: Appropriate timeouts, output size limits
5. **Maintainability**: Using comments, structuring, splitting configurations
6. **Performance**: Parallel execution, verification optimization, appropriate output capture
7. **Testing**: Incremental testing, dry run utilization
8. **Documentation**: README creation, change history recording

By following these practices, you can create safe and maintainable configuration files.

## Next Steps

The next chapter will cover common problems when creating configuration files and their solutions. Let's master troubleshooting techniques.
