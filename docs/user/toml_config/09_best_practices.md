# Chapter 9: Best Practices

This chapter introduces best practices for creating go-safe-cmd-runner configuration files. It provides guidelines for creating better configuration files from the perspectives of security, maintainability, and performance.

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
max_risk_level = "low"

# Avoid: Excessive privileges
[[groups.commands]]
name = "read_log"
cmd = "/bin/cat"
args = ["/var/log/app/app.log"]
run_as_user = "root"  # Unnecessarily using root privileges
```

### 9.1.2 Strict Environment Variable Management

Set environment variable allowlists to the minimum necessary.

#### Recommended Implementation

```toml
# Good example: Allow only necessary variables
[global]
env_allowlist = [
    "PATH",           # Essential for command search
    "HOME",           # Used for configuration file search
    "APP_CONFIG_DIR", # Application-specific configuration
]

# Avoid: Overly permissive settings
[global]
env_allowlist = [
    "PATH", "HOME", "USER", "SHELL", "EDITOR", "PAGER",
    "MAIL", "LOGNAME", "HOSTNAME", "DISPLAY", "XAUTHORITY",
    # ... too many
]
```

### 9.1.3 Utilize File Verification

Always verify important commands and configuration files.

#### Recommended Implementation

```toml
# Good example: Verify important files
[global]
skip_standard_paths = false
verify_files = [
    "/bin/sh",
    "/usr/bin/python3",
]

[[groups]]
name = "critical_operations"
verify_files = [
    "/opt/app/bin/critical-tool",
    "/opt/app/scripts/deploy.sh",
]
```

### 9.1.4 Use Absolute Paths

Specify commands with absolute paths.

#### Recommended Implementation

```toml
# Good example: Use absolute paths
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/rsync"
args = ["-av", "/data/", "/backup/"]

# Avoid: Relative commands (security risk)
[[groups.commands]]
name = "backup"
cmd = "rsync"  # Could execute malicious binary in PATH
args = ["-av", "/data/", "/backup/"]
```

### 9.1.5 Risk Level Management

Appropriately set risk levels for commands.

#### Risk Level Guidelines

```toml
# Low risk: Read-only operations
[[groups.commands]]
name = "view_status"
cmd = "/bin/systemctl"
args = ["status", "nginx"]
max_risk_level = "low"

# Medium risk: Configuration changes, data processing
[[groups.commands]]
name = "reload_config"
cmd = "/bin/systemctl"
args = ["reload", "nginx"]
max_risk_level = "medium"

# High risk: System changes, privilege escalation
[[groups.commands]]
name = "restart_service"
cmd = "/bin/systemctl"
args = ["restart", "nginx"]
run_as_user = "root"
max_risk_level = "high"
```

## 9.2 Environment Variable Management Best Practices

### 9.2.1 Inheritance Mode Selection

Choose appropriate inheritance modes based on security requirements.

#### Recommended Patterns

```toml
# High security: Explicit mode
[[groups]]
name = "secure_operations"
env_inheritance_mode = "explicit"
env_allowlist = ["PATH"]  # Only explicitly allowed variables

# Standard operations: Inherit mode with filtering
[[groups]]
name = "standard_operations"
env_inheritance_mode = "inherit"
env_allowlist = ["PATH", "HOME", "USER"]

# Development only: Reject mode for isolation
[[groups]]
name = "development_tests"
env_inheritance_mode = "reject"
# No environment variables inherited
```

### 9.2.2 Environment-Specific Configuration

Use variable expansion for environment-specific settings.

```toml
# Good example: Environment-aware configuration
[global]
env_allowlist = ["PATH", "ENVIRONMENT", "CONFIG_DIR"]

[[groups.commands]]
name = "deploy"
cmd = "/opt/scripts/deploy.sh"
args = [
    "--environment", "${ENVIRONMENT}",
    "--config", "${CONFIG_DIR}/app-${ENVIRONMENT}.conf"
]
```

## 9.3 Group Configuration Best Practices

### 9.3.1 Logical Grouping

Group related commands together for better organization.

#### Recommended Structure

```toml
# Good example: Logical grouping by function
[[groups]]
name = "database_operations"
description = "Database backup and maintenance"
# ... database-related commands

[[groups]]
name = "web_server_operations"
description = "Web server configuration and management"
# ... web server-related commands

[[groups]]
name = "monitoring"
description = "System monitoring and alerting"
# ... monitoring-related commands
```

### 9.3.2 Priority Management

Set appropriate priorities for execution order.

```toml
# Good example: Priority-based execution
[[groups]]
name = "preparation"
description = "Preparation tasks"
priority = 1

[[groups]]
name = "main_operations"
description = "Main processing tasks"
priority = 2

[[groups]]
name = "cleanup"
description = "Cleanup and finalization"
priority = 3
```

### 9.3.3 Resource Isolation

Use temporary directories for isolation.

```toml
# Good example: Isolated temporary workspace
[[groups]]
name = "data_processing"
temp_dir = "/var/tmp/processing"
workdir = "/opt/workspace"

[[groups.commands]]
name = "process_data"
cmd = "/opt/tools/processor"
args = ["--temp-dir", "/var/tmp/processing"]
```

## 9.4 Error Handling Best Practices

### 9.4.1 Appropriate Timeout Settings

Set realistic timeouts based on expected execution time.

```toml
# Good example: Appropriate timeouts
[global]
timeout = 300  # Default 5 minutes

[[groups.commands]]
name = "quick_check"
cmd = "/bin/systemctl"
args = ["is-active", "nginx"]
timeout = 30  # 30 seconds for quick operations

[[groups.commands]]
name = "database_backup"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases"]
timeout = 3600  # 1 hour for long operations
```

### 9.4.2 Output Size Management

Set appropriate output size limits.

```toml
# Good example: Reasonable output limits
[global]
max_output_size = 104857600  # 100MB default

[[groups.commands]]
name = "log_analysis"
cmd = "/opt/tools/analyzer"
args = ["--input", "/var/log/app.log"]
output = "analysis-report.txt"
# Uses global limit - appropriate for reports

[[groups.commands]]
name = "system_status"
cmd = "/bin/ps"
args = ["aux"]
output = "process-list.txt"
# Uses global limit - appropriate for system info
```

## 9.5 Maintainability Best Practices

### 9.5.1 Clear Documentation

Provide clear descriptions for all configuration elements.

```toml
# Good example: Well-documented configuration
version = "1.0"

[global]
# Set reasonable timeout for most operations
timeout = 600
# Use info level for production monitoring
log_level = "info"
# Allow only essential environment variables
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "daily_backup"
description = "Automated daily backup of critical data"
# Use dedicated backup workspace
workdir = "/var/backups/daily"

[[groups.commands]]
name = "backup_database"
description = "Create compressed backup of production database"
cmd = "/usr/bin/mysqldump"
args = [
    "--single-transaction",  # Consistent backup
    "--routines",           # Include stored procedures
    "--triggers",           # Include triggers
    "production_db"
]
# Database backups may take time
timeout = 1800
```

### 9.5.2 Consistent Naming Conventions

Use consistent and descriptive names.

#### Recommended Naming Patterns

```toml
# Good example: Consistent naming
[[groups]]
name = "web_server_maintenance"  # underscore_separated

[[groups.commands]]
name = "restart_nginx"           # action_target format
name = "backup_config_files"     # action_target format
name = "check_service_status"    # action_target format

# Avoid: Inconsistent naming
[[groups]]
name = "WebServerMaint"          # Mixed case
[[groups.commands]]
name = "restartNginx"            # camelCase
name = "backup-config"           # Mixed separators
```

### 9.5.3 Configuration Validation

Always validate configurations before deployment.

#### Validation Workflow

```bash
# Recommended validation process
# 1. Syntax validation
runner -config production.toml -validate

# 2. Dry run testing
runner -config production.toml -dry-run

# 3. Staging environment testing
runner -config production.toml -dry-run-format json > test-results.json

# 4. Production deployment only after validation
if [ $? -eq 0 ]; then
    runner -config production.toml
fi
```

## 9.6 Performance Optimization Best Practices

### 9.6.1 Efficient Command Sequencing

Order commands for optimal execution flow.

```toml
# Good example: Efficient sequencing
[[groups]]
name = "deployment"
priority = 1

# Fast preparation tasks first
[[groups.commands]]
name = "check_prerequisites"
cmd = "/opt/scripts/check-prereqs.sh"
timeout = 60

# Main deployment task
[[groups.commands]]
name = "deploy_application"
cmd = "/opt/scripts/deploy.sh"
timeout = 1800

# Quick verification at the end
[[groups.commands]]
name = "verify_deployment"
cmd = "/opt/scripts/verify.sh"
timeout = 120
```

### 9.6.2 Resource Optimization

Configure appropriate resource limits.

```toml
# Good example: Balanced resource configuration
[global]
max_output_size = 52428800  # 50MB - reasonable for most operations

[[groups]]
name = "log_processing"
workdir = "/var/tmp/logs"     # Use fast temporary storage
temp_dir = "/var/tmp/work"    # Dedicated temp space

[[groups.commands]]
name = "compress_logs"
cmd = "/bin/gzip"
args = ["/var/log/app/*.log"]
timeout = 300  # Appropriate for compression task
```

## 9.7 Testing and Validation Best Practices

### 9.7.1 Staged Testing Approach

Test configurations in multiple stages.

#### Testing Pipeline

```bash
# Stage 1: Configuration validation
echo "Validating configuration..."
runner -config test.toml -validate || exit 1

# Stage 2: Dry run testing
echo "Testing execution plan..."
runner -config test.toml -dry-run || exit 1

# Stage 3: Staging environment
echo "Testing in staging..."
runner -config test.toml -dry-run-detail full

# Stage 4: Production deployment
echo "Deploying to production..."
runner -config production.toml
```

### 9.7.2 Monitoring and Logging

Configure comprehensive monitoring.

```toml
# Good example: Production monitoring setup
[global]
log_level = "info"  # Detailed logging for production

[[groups]]
name = "production_operations"
description = "Production operations with monitoring"

[[groups.commands]]
name = "system_check"
description = "Pre-operation system check"
cmd = "/opt/monitoring/system-check.sh"
output = "system-check.log"  # Capture output for analysis

[[groups.commands]]
name = "main_operation"
description = "Main business operation"
cmd = "/opt/app/main-process.sh"
output = "operation.log"
timeout = 1800
```

## 9.8 Security Review Checklist

Before deploying configurations, review:

### ✅ Security Checklist

- [ ] All commands use absolute paths
- [ ] Environment variable allowlists are minimal
- [ ] File verification is enabled for critical executables
- [ ] Appropriate risk levels are set
- [ ] Privilege escalation is justified and minimal
- [ ] Timeouts are reasonable for each operation
- [ ] Output capture is configured appropriately
- [ ] Temporary directories are properly isolated

### ✅ Maintainability Checklist

- [ ] All groups and commands have clear descriptions
- [ ] Naming conventions are consistent
- [ ] Configuration is well-documented
- [ ] Priorities are set appropriately
- [ ] Resource limits are configured
- [ ] Validation passes without errors

### ✅ Operational Checklist

- [ ] Dry run completes successfully
- [ ] All referenced files exist and have recorded hashes
- [ ] Environment variables are properly set
- [ ] Working directories have appropriate permissions
- [ ] Monitoring and logging are configured

---

Following these best practices will help you create secure, maintainable, and efficient configurations for go-safe-cmd-runner. Regular review and updates of your configurations ensure continued security and performance.
