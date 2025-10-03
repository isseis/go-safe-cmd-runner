# Chapter 8: Practical Configuration Examples

This chapter introduces practical configuration examples based on real use cases. Use these examples as reference to create configuration files suited to your environment.

## 8.1 Basic Configuration Examples

### Simple Backup Task

Basic configuration for daily file backup:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/tmp"
log_level = "info"
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "daily_backup"
description = "Daily file backup"
workdir = "/var/backups"

[[groups.commands]]
name = "backup_configs"
description = "Backup configuration files"
cmd = "/bin/tar"
args = [
    "-czf",
    "config-backup.tar.gz",
    "/etc/myapp",
]
timeout = 600

[[groups.commands]]
name = "backup_logs"
description = "Backup log files"
cmd = "/bin/tar"
args = [
    "-czf",
    "logs-backup.tar.gz",
    "/var/log/myapp",
]
timeout = 600

[[groups.commands]]
name = "list_backups"
description = "List backup files"
cmd = "/bin/ls"
args = ["-lh", "*.tar.gz"]
output = "backup-list.txt"
```

## 8.2 Security-Focused Configuration Examples

### File Verification and Access Control

Configuration for high-security environments:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/opt/secure"
log_level = "info"
skip_standard_paths = false  # Verify all files
env_allowlist = ["PATH"]      # Minimal environment variables
verify_files = [
    "/bin/sh",
    "/bin/tar",
    "/usr/bin/gpg",
]

[[groups]]
name = "secure_backup"
description = "Secure backup processing"
workdir = "/var/secure/backups"
env_allowlist = ["PATH", "GPG_KEY_ID"]
verify_files = [
    "/opt/secure/bin/backup-tool",
]

[[groups.commands]]
name = "create_backup"
description = "Create backup archive"
cmd = "/bin/tar"
args = [
    "-czf",
    "data-backup.tar.gz",
    "/opt/secure/data",
]
max_risk_level = "medium"
timeout = 1800

[[groups.commands]]
name = "encrypt_backup"
description = "Encrypt backup"
cmd = "/usr/bin/gpg"
args = [
    "--encrypt",
    "--armor",
    "--recipient", "${GPG_KEY_ID}",
    "--output", "data-backup.tar.gz.asc",
    "data-backup.tar.gz"
]
max_risk_level = "high"
timeout = 600

[[groups.commands]]
name = "verify_backup"
description = "Verify backup integrity"
cmd = "/usr/bin/gpg"
args = ["--verify", "data-backup.tar.gz.asc"]
max_risk_level = "low"
```

## 8.3 Resource Management Configuration

### Multi-Stage Processing with Resource Control

Configuration with temporary directories and resource management:

```toml
version = "1.0"

[global]
timeout = 1800
log_level = "info"
env_allowlist = ["PATH", "HOME", "TMPDIR"]
max_output_size = 104857600  # 100MB

[[groups]]
name = "data_processing"
description = "Multi-stage data processing"
temp_dir = "/var/tmp/processing"
workdir = "/opt/data/workspace"

[[groups.commands]]
name = "prepare_workspace"
description = "Prepare processing workspace"
cmd = "/bin/mkdir"
args = ["-p", "input", "output", "temp"]
timeout = 30

[[groups.commands]]
name = "process_data"
description = "Process input data"
cmd = "/opt/tools/data-processor"
args = [
    "--input", "input/",
    "--output", "temp/",
    "--format", "json"
]
output = "processing.log"
timeout = 3600
max_risk_level = "medium"

[[groups.commands]]
name = "validate_output"
description = "Validate processed output"
cmd = "/opt/tools/validator"
args = ["--check", "temp/"]
timeout = 300

[[groups.commands]]
name = "move_results"
description = "Move results to final location"
cmd = "/bin/mv"
args = ["temp/*", "output/"]
timeout = 60
```

## 8.4 Privilege Escalation Examples

### Database Operations with User Switching

Configuration requiring privilege escalation:

```toml
version = "1.0"

[global]
timeout = 600
log_level = "info"
env_allowlist = ["PATH", "PGPASSWORD", "MYSQL_PWD"]

[[groups]]
name = "database_maintenance"
description = "Database maintenance operations"
workdir = "/var/lib/db-maintenance"

[[groups.commands]]
name = "backup_postgres"
description = "PostgreSQL database backup"
cmd = "/usr/bin/pg_dump"
args = [
    "--host=localhost",
    "--username=backup_user",
    "--format=custom",
    "--file=postgres_backup.sql",
    "production_db"
]
run_as_user = "postgres"
run_as_group = "postgres"
max_risk_level = "high"
timeout = 1800

[[groups.commands]]
name = "optimize_mysql"
description = "MySQL table optimization"
cmd = "/usr/bin/mysql"
args = [
    "--user=admin",
    "--execute=OPTIMIZE TABLE user_data, transaction_log;"
]
run_as_user = "mysql"
run_as_group = "mysql"
max_risk_level = "medium"
timeout = 900

[[groups.commands]]
name = "cleanup_logs"
description = "Clean old database logs"
cmd = "/bin/find"
args = [
    "/var/log/mysql",
    "-name", "*.log",
    "-mtime", "+7",
    "-delete"
]
run_as_user = "mysql"
max_risk_level = "low"
```

## 8.5 Output Capture Examples

### System Monitoring with Output Management

Configuration for capturing and processing command outputs:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/var/monitoring"
log_level = "info"
env_allowlist = ["PATH"]
max_output_size = 52428800  # 50MB

[[groups]]
name = "system_monitoring"
description = "System status monitoring"

[[groups.commands]]
name = "disk_usage"
description = "Check disk usage"
cmd = "/bin/df"
args = ["-h"]
output = "disk-usage.txt"

[[groups.commands]]
name = "memory_status"
description = "Check memory status"
cmd = "/usr/bin/free"
args = ["-h"]
output = "memory-status.txt"

[[groups.commands]]
name = "process_list"
description = "List running processes"
cmd = "/bin/ps"
args = ["aux"]
output = "process-list.txt"

[[groups.commands]]
name = "network_connections"
description = "Show network connections"
cmd = "/bin/netstat"
args = ["-tuln"]
output = "network-connections.txt"

[[groups.commands]]
name = "generate_report"
description = "Generate monitoring report"
cmd = "/opt/scripts/generate-report.sh"
args = [
    "--disk", "disk-usage.txt",
    "--memory", "memory-status.txt",
    "--processes", "process-list.txt",
    "--network", "network-connections.txt"
]
output = "monitoring-report.html"
timeout = 600
```

## 8.6 Variable Expansion Examples

### Environment-Specific Deployment

Configuration using variable expansion for different environments:

```toml
version = "1.0"

[global]
timeout = 900
log_level = "info"
env_allowlist = ["PATH", "ENVIRONMENT", "APP_VERSION", "DEPLOY_KEY"]

[[groups]]
name = "deployment"
description = "Application deployment"
workdir = "/opt/deployment"

[[groups.commands]]
name = "download_package"
description = "Download application package"
cmd = "/usr/bin/curl"
args = [
    "-L",
    "--output", "app-${APP_VERSION}.tar.gz",
    "https://releases.example.com/${ENVIRONMENT}/app-${APP_VERSION}.tar.gz"
]
timeout = 1200

[[groups.commands]]
name = "extract_package"
description = "Extract application package"
cmd = "/bin/tar"
args = [
    "-xzf", "app-${APP_VERSION}.tar.gz",
    "--strip-components=1",
    "-C", "/opt/app-${ENVIRONMENT}"
]

[[groups.commands]]
name = "update_config"
description = "Update configuration for environment"
cmd = "/opt/scripts/update-config.sh"
args = [
    "--environment", "${ENVIRONMENT}",
    "--config-dir", "/opt/app-${ENVIRONMENT}/config"
]

[[groups.commands]]
name = "restart_service"
description = "Restart application service"
cmd = "/bin/systemctl"
args = ["restart", "app-${ENVIRONMENT}"]
run_as_user = "root"
max_risk_level = "high"
```

## 8.7 Complex Multi-Group Configuration

### CI/CD Pipeline with Multiple Stages

Complex configuration with multiple groups and dependencies:

```toml
version = "1.0"

[global]
timeout = 1800
log_level = "info"
env_allowlist = ["PATH", "CI_COMMIT_SHA", "CI_PIPELINE_ID"]

# Build stage
[[groups]]
name = "build"
description = "Build and test application"
priority = 1
workdir = "/opt/ci/build"

[[groups.commands]]
name = "compile"
description = "Compile source code"
cmd = "/usr/bin/make"
args = ["build"]
output = "compile.log"
timeout = 900

[[groups.commands]]
name = "run_tests"
description = "Run test suite"
cmd = "/usr/bin/make"
args = ["test"]
output = "test-results.xml"
timeout = 600

# Package stage
[[groups]]
name = "package"
description = "Package application"
priority = 2
workdir = "/opt/ci/package"

[[groups.commands]]
name = "create_package"
description = "Create deployment package"
cmd = "/opt/tools/packager"
args = [
    "--input", "/opt/ci/build/dist",
    "--output", "app-${CI_COMMIT_SHA}.tar.gz",
    "--version", "${CI_COMMIT_SHA}"
]
timeout = 300

# Deploy stage
[[groups]]
name = "deploy"
description = "Deploy to staging"
priority = 3
workdir = "/opt/ci/deploy"
env_allowlist = ["PATH", "CI_COMMIT_SHA", "DEPLOY_TOKEN"]

[[groups.commands]]
name = "upload_package"
description = "Upload package to repository"
cmd = "/usr/bin/curl"
args = [
    "-X", "POST",
    "-H", "Authorization: Bearer ${DEPLOY_TOKEN}",
    "--data-binary", "@/opt/ci/package/app-${CI_COMMIT_SHA}.tar.gz",
    "https://deploy.example.com/packages"
]
max_risk_level = "medium"
timeout = 600

[[groups.commands]]
name = "deploy_to_staging"
description = "Deploy to staging environment"
cmd = "/opt/tools/deployer"
args = [
    "--environment", "staging",
    "--package", "app-${CI_COMMIT_SHA}.tar.gz"
]
run_as_user = "deploy"
max_risk_level = "high"
timeout = 900
```

## 8.8 Risk-Based Control Examples

### High-Security Operations with Risk Management

Configuration demonstrating risk-based command control:

```toml
version = "1.0"

[global]
timeout = 600
log_level = "info"
env_allowlist = ["PATH"]

[[groups]]
name = "security_operations"
description = "High-security operations with risk control"

# Low risk operations
[[groups.commands]]
name = "check_status"
description = "Check system status (low risk)"
cmd = "/bin/systemctl"
args = ["status", "nginx"]
max_risk_level = "low"

[[groups.commands]]
name = "view_logs"
description = "View application logs (low risk)"
cmd = "/bin/tail"
args = ["-n", "100", "/var/log/app.log"]
max_risk_level = "low"
output = "recent-logs.txt"

# Medium risk operations
[[groups.commands]]
name = "reload_config"
description = "Reload service configuration (medium risk)"
cmd = "/bin/systemctl"
args = ["reload", "nginx"]
run_as_user = "root"
max_risk_level = "medium"

[[groups.commands]]
name = "backup_database"
description = "Create database backup (medium risk)"
cmd = "/usr/bin/mysqldump"
args = [
    "--single-transaction",
    "--routines",
    "--triggers",
    "production_db"
]
run_as_user = "mysql"
max_risk_level = "medium"
output = "db-backup.sql"
timeout = 1800

# High risk operations
[[groups.commands]]
name = "restart_service"
description = "Restart critical service (high risk)"
cmd = "/bin/systemctl"
args = ["restart", "production-app"]
run_as_user = "root"
max_risk_level = "high"

[[groups.commands]]
name = "update_firewall"
description = "Update firewall rules (high risk)"
cmd = "/usr/sbin/iptables-restore"
args = ["/etc/iptables/production.rules"]
run_as_user = "root"
max_risk_level = "high"
```

## 8.9 Integration Examples

### Slack Notification Integration

Configuration with external service integration:

```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "SLACK_WEBHOOK_URL"]

[[groups]]
name = "maintenance_with_notifications"
description = "System maintenance with Slack notifications"

[[groups.commands]]
name = "pre_maintenance_check"
description = "Pre-maintenance system check"
cmd = "/opt/scripts/pre-check.sh"
output = "pre-check.log"

[[groups.commands]]
name = "notify_start"
description = "Notify maintenance start"
cmd = "/usr/bin/curl"
args = [
    "-X", "POST",
    "-H", "Content-type: application/json",
    "--data", '{"text":"🔧 Maintenance started"}',
    "${SLACK_WEBHOOK_URL}"
]

[[groups.commands]]
name = "perform_maintenance"
description = "Execute maintenance tasks"
cmd = "/opt/scripts/maintenance.sh"
output = "maintenance.log"
timeout = 1800

[[groups.commands]]
name = "notify_completion"
description = "Notify maintenance completion"
cmd = "/usr/bin/curl"
args = [
    "-X", "POST",
    "-H", "Content-type: application/json",
    "--data", '{"text":"✅ Maintenance completed successfully"}',
    "${SLACK_WEBHOOK_URL}"
]
```

---

These examples demonstrate various configuration patterns and can be adapted to your specific needs. Remember to:

1. **Validate configurations** using `-validate` flag
2. **Test with dry-run** before production use
3. **Record hash values** for all referenced executables
4. **Follow security best practices** from Chapter 9

For more specific configuration details, refer to the individual chapters covering each configuration level.
