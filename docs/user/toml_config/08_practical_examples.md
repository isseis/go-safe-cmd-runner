# Chapter 8: Practical Examples

This chapter introduces practical configuration examples based on real-world use cases. Use these examples as a reference to create configuration files suited to your own environment.

## 8.1 Basic Configuration Examples

### Simple Backup Task

Basic configuration for daily file backups:

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

Configuration for environments with high security requirements:

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
description = "Secure backup process"
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
    "--recipient", "${GPG_KEY_ID}",
    "data-backup.tar.gz",
]
env = ["GPG_KEY_ID=admin@example.com"]
max_risk_level = "medium"

[[groups.commands]]
name = "verify_encrypted"
description = "Verify encrypted file"
cmd = "/usr/bin/gpg"
args = [
    "--verify",
    "data-backup.tar.gz.gpg",
]
max_risk_level = "low"
output = "verification-result.txt"
```

## 8.3 Configuration Examples with Resource Management

### Temporary Directory and Automatic Cleanup

Using a temporary workspace that is automatically deleted after processing:

```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "temp_processing"
description = "Data processing in temporary directory"
temp_dir = true   # Automatically create temporary directory
cleanup = true    # Automatically delete after processing

[[groups.commands]]
name = "download_data"
description = "Download data"
cmd = "/usr/bin/curl"
args = [
    "-o", "data.csv",
    "https://example.com/data/export.csv",
]
timeout = 600

[[groups.commands]]
name = "process_data"
description = "Process data"
cmd = "/opt/tools/process"
args = [
    "--input", "data.csv",
    "--output", "processed.csv",
]
timeout = 900

[[groups.commands]]
name = "upload_result"
description = "Upload processing result"
cmd = "/usr/bin/curl"
args = [
    "-X", "POST",
    "-F", "file=@processed.csv",
    "https://example.com/api/upload",
]
timeout = 600
output = "upload-response.txt"

# Temporary directory is automatically deleted
```

## 8.4 Configuration Examples with Privilege Escalation

### System Administration Tasks

System maintenance requiring root privileges:

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/tmp"
log_level = "info"
env_allowlist = ["PATH", "HOME"]
verify_files = [
    "/usr/bin/apt-get",
    "/usr/bin/systemctl",
]

[[groups]]
name = "system_maintenance"
description = "System maintenance tasks"
priority = 1

# Non-privileged task: Check system status
[[groups.commands]]
name = "check_disk_space"
description = "Check disk usage"
cmd = "/bin/df"
args = ["-h"]
max_risk_level = "low"
output = "disk-usage.txt"

# Privileged task: Update packages
[[groups.commands]]
name = "update_packages"
description = "Update package list"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
max_risk_level = "high"
timeout = 900

# Privileged task: Restart service
[[groups.commands]]
name = "restart_service"
description = "Restart application service"
cmd = "/usr/bin/systemctl"
args = ["restart", "myapp.service"]
run_as_user = "root"
max_risk_level = "high"

# Non-privileged task: Check service status
[[groups.commands]]
name = "check_service_status"
description = "Check service status"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp.service"]
max_risk_level = "low"
output = "service-status.txt"
```

## 8.5 Configuration Examples Using Output Capture

### Log Collection and Report Generation

Collecting output from multiple commands to create a report:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/var/reports"
log_level = "info"
env_allowlist = ["PATH", "HOME"]
max_output_size = 10485760  # 10MB

[[groups]]
name = "system_report"
description = "Generate system status report"

[[groups.commands]]
name = "disk_usage_report"
description = "Disk usage report"
cmd = "/bin/df"
args = ["-h"]
output = "reports/disk-usage.txt"

[[groups.commands]]
name = "memory_report"
description = "Memory usage report"
cmd = "/usr/bin/free"
args = ["-h"]
output = "reports/memory-usage.txt"

[[groups.commands]]
name = "process_report"
description = "Process list report"
cmd = "/bin/ps"
args = ["aux"]
output = "reports/processes.txt"

[[groups.commands]]
name = "network_report"
description = "Network connection status report"
cmd = "/bin/netstat"
args = ["-tuln"]
output = "reports/network-connections.txt"

[[groups.commands]]
name = "service_report"
description = "Service status report"
cmd = "/usr/bin/systemctl"
args = ["list-units", "--type=service", "--state=running"]
output = "reports/services.txt"

# Archive report files
[[groups.commands]]
name = "archive_reports"
description = "Compress reports"
cmd = "/bin/tar"
args = [
    "-czf",
    "system-report-${DATE}.tar.gz",
    "reports/",
]
env = ["DATE=2025-10-02"]
```

## 8.6 Configuration Examples Using Variable Expansion

### Environment-Specific Deployment

Using different configurations for development, staging, and production environments:

```toml
version = "1.0"

[global]
timeout = 600
log_level = "info"
env_allowlist = [
    "PATH",
    "HOME",
    "APP_BIN",
    "CONFIG_DIR",
    "ENV_TYPE",
    "LOG_LEVEL",
    "DB_URL",
    "API_PORT",
]

# Development environment
[[groups]]
name = "deploy_development"
description = "Deploy to development environment"
priority = 1

[[groups.commands]]
name = "deploy_dev_config"
cmd = "/bin/cp"
args = [
    "${CONFIG_DIR}/${ENV_TYPE}/app.yml",
    "/etc/myapp/app.yml",
]
env = [
    "CONFIG_DIR=/opt/configs",
    "ENV_TYPE=development",
]

[[groups.commands]]
name = "start_dev_server"
cmd = "${APP_BIN}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "${LOG_LEVEL}",
    "--port", "${API_PORT}",
    "--database", "${DB_URL}",
]
env = [
    "APP_BIN=/opt/myapp/bin/server",
    "LOG_LEVEL=debug",
    "API_PORT=8080",
    "DB_URL=postgresql://localhost/dev_db",
]

# Staging environment
[[groups]]
name = "deploy_staging"
description = "Deploy to staging environment"
priority = 2

[[groups.commands]]
name = "deploy_staging_config"
cmd = "/bin/cp"
args = [
    "${CONFIG_DIR}/${ENV_TYPE}/app.yml",
    "/etc/myapp/app.yml",
]
env = [
    "CONFIG_DIR=/opt/configs",
    "ENV_TYPE=staging",
]

[[groups.commands]]
name = "start_staging_server"
cmd = "${APP_BIN}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "${LOG_LEVEL}",
    "--port", "${API_PORT}",
    "--database", "${DB_URL}",
]
env = [
    "APP_BIN=/opt/myapp/bin/server",
    "LOG_LEVEL=info",
    "API_PORT=8081",
    "DB_URL=postgresql://staging-db/staging_db",
]

# Production environment
[[groups]]
name = "deploy_production"
description = "Deploy to production environment"
priority = 3

[[groups.commands]]
name = "deploy_prod_config"
cmd = "/bin/cp"
args = [
    "${CONFIG_DIR}/${ENV_TYPE}/app.yml",
    "/etc/myapp/app.yml",
]
env = [
    "CONFIG_DIR=/opt/configs",
    "ENV_TYPE=production",
]

[[groups.commands]]
name = "start_prod_server"
cmd = "${APP_BIN}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "${LOG_LEVEL}",
    "--port", "${API_PORT}",
    "--database", "${DB_URL}",
]
env = [
    "APP_BIN=/opt/myapp/bin/server",
    "LOG_LEVEL=warn",
    "API_PORT=8082",
    "DB_URL=postgresql://prod-db/prod_db",
]
run_as_user = "appuser"
max_risk_level = "high"
```

## 8.7 Comprehensive Configuration Examples

### Full-Stack Application Deployment

Integrated deployment of database, application, and web server:

```toml
version = "1.0"

[global]
timeout = 900
workdir = "/opt/deploy"
log_level = "info"
skip_standard_paths = true
env_allowlist = [
    "PATH",
    "HOME",
    "DB_USER",
    "DB_NAME",
    "APP_DIR",
    "WEB_ROOT",
    "BACKUP_DIR",
]
max_output_size = 52428800  # 50MB

# Phase 1: Preparation
[[groups]]
name = "preparation"
description = "Pre-deployment preparation"
priority = 1
workdir = "/opt/deploy/prep"
temp_dir = true
cleanup = true

[[groups.commands]]
name = "backup_current_version"
description = "Backup current version"
cmd = "/bin/tar"
args = [
    "-czf",
    "${BACKUP_DIR}/app-backup-${TIMESTAMP}.tar.gz",
    "${APP_DIR}",
]
env = [
    "BACKUP_DIR=/var/backups/app",
    "APP_DIR=/opt/myapp",
    "TIMESTAMP=2025-10-02-120000",
]
timeout = 1800

[[groups.commands]]
name = "check_dependencies"
description = "Check dependencies"
cmd = "/usr/bin/dpkg"
args = ["-l"]
output = "installed-packages.txt"

# Phase 2: Database update
[[groups]]
name = "database_migration"
description = "Update database schema"
priority = 2
env_allowlist = ["PATH", "DB_USER", "DB_NAME", "PGPASSWORD"]
verify_files = ["/usr/bin/psql", "/usr/bin/pg_dump"]

[[groups.commands]]
name = "backup_database"
description = "Backup database"
cmd = "/usr/bin/pg_dump"
args = [
    "-U", "${DB_USER}",
    "-d", "${DB_NAME}",
    "-F", "c",
    "-f", "/var/backups/db/backup-${TIMESTAMP}.dump",
]
env = [
    "DB_USER=appuser",
    "DB_NAME=myapp_db",
    "TIMESTAMP=2025-10-02-120000",
    "PGPASSWORD=secret123",
]
timeout = 1800
output = "db-backup-log.txt"

[[groups.commands]]
name = "run_migrations"
description = "Run database migrations"
cmd = "/opt/myapp/bin/migrate"
args = [
    "--database", "postgresql://${DB_USER}@localhost/${DB_NAME}",
    "--migrations", "/opt/myapp/migrations",
]
env = [
    "DB_USER=appuser",
    "DB_NAME=myapp_db",
]
timeout = 600

# Phase 3: Application deployment
[[groups]]
name = "application_deployment"
description = "Deploy application"
priority = 3
workdir = "/opt/myapp"

[[groups.commands]]
name = "stop_application"
description = "Stop application"
cmd = "/usr/bin/systemctl"
args = ["stop", "myapp.service"]
run_as_user = "root"
max_risk_level = "high"

[[groups.commands]]
name = "deploy_new_version"
description = "Deploy new version"
cmd = "/bin/tar"
args = [
    "-xzf",
    "/opt/deploy/releases/myapp-v2.0.0.tar.gz",
    "-C", "/opt/myapp",
]

[[groups.commands]]
name = "install_dependencies"
description = "Install dependencies"
cmd = "/usr/bin/pip3"
args = [
    "install",
    "-r", "/opt/myapp/requirements.txt",
]
timeout = 600

[[groups.commands]]
name = "start_application"
description = "Start application"
cmd = "/usr/bin/systemctl"
args = ["start", "myapp.service"]
run_as_user = "root"
max_risk_level = "high"

# Phase 4: Web server configuration update
[[groups]]
name = "web_server_update"
description = "Update web server configuration"
priority = 4

[[groups.commands]]
name = "update_nginx_config"
description = "Update Nginx configuration"
cmd = "/bin/cp"
args = [
    "/opt/deploy/configs/nginx/myapp.conf",
    "/etc/nginx/sites-available/myapp.conf",
]
run_as_user = "root"

[[groups.commands]]
name = "test_nginx_config"
description = "Validate Nginx configuration"
cmd = "/usr/bin/nginx"
args = ["-t"]
run_as_user = "root"
output = "nginx-config-test.txt"

[[groups.commands]]
name = "reload_nginx"
description = "Reload Nginx"
cmd = "/usr/bin/systemctl"
args = ["reload", "nginx"]
run_as_user = "root"
max_risk_level = "high"

# Phase 5: Deployment verification
[[groups]]
name = "deployment_verification"
description = "Verify deployment"
priority = 5

[[groups.commands]]
name = "health_check"
description = "Application health check"
cmd = "/usr/bin/curl"
args = [
    "-f",
    "-s",
    "http://localhost:8080/health",
]
timeout = 30
output = "health-check-result.txt"

[[groups.commands]]
name = "smoke_test"
description = "Basic functionality test"
cmd = "/usr/bin/curl"
args = [
    "-f",
    "-s",
    "http://localhost:8080/api/status",
]
output = "smoke-test-result.txt"

[[groups.commands]]
name = "verify_database_connection"
description = "Verify database connection"
cmd = "/usr/bin/psql"
args = [
    "-U", "${DB_USER}",
    "-d", "${DB_NAME}",
    "-c", "SELECT version();",
]
env = [
    "DB_USER=appuser",
    "DB_NAME=myapp_db",
]
output = "db-connection-test.txt"

# Phase 6: Post-processing and reporting
[[groups]]
name = "post_deployment"
description = "Post-deployment processing"
priority = 6
workdir = "/var/reports/deployment"

[[groups.commands]]
name = "generate_deployment_report"
description = "Generate deployment report"
cmd = "/opt/tools/generate-report"
args = [
    "--deployment-log", "/var/log/deploy.log",
    "--output", "deployment-report-${TIMESTAMP}.html",
]
env = ["TIMESTAMP=2025-10-02-120000"]

[[groups.commands]]
name = "cleanup_temp_files"
description = "Delete temporary files"
cmd = "/bin/rm"
args = ["-rf", "/opt/deploy/temp"]
max_risk_level = "medium"

[[groups.commands]]
name = "send_notification"
description = "Send deployment completion notification"
cmd = "/usr/bin/curl"
args = [
    "-X", "POST",
    "-H", "Content-Type: application/json",
    "-d", '{"message":"Deployment completed successfully"}',
    "https://slack.example.com/webhook",
]
```

## 8.8 Risk-Based Control Examples

### Command Execution Based on Risk Level

```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "risk_controlled_operations"
description = "Operations controlled based on risk level"

# Low risk: Read-only operations
[[groups.commands]]
name = "read_config"
description = "Read configuration file"
cmd = "/bin/cat"
args = ["/etc/myapp/config.yml"]
max_risk_level = "low"
output = "config-content.txt"

# Medium risk: File creation/modification
[[groups.commands]]
name = "update_cache"
description = "Update cache file"
cmd = "/opt/myapp/update-cache"
args = ["--refresh"]
max_risk_level = "medium"

# High risk: System changes
[[groups.commands]]
name = "system_update"
description = "Update system packages"
cmd = "/usr/bin/apt-get"
args = ["upgrade", "-y"]
run_as_user = "root"
max_risk_level = "high"
timeout = 1800

# Example that will be rejected for exceeding risk level
[[groups.commands]]
name = "dangerous_deletion"
description = "Mass deletion (cannot run at low risk level)"
cmd = "/bin/rm"
args = ["-rf", "/tmp/old-data"]
max_risk_level = "low"  # rm -rf is medium risk or higher → execution rejected
```

## Summary

This chapter introduced the following practical configuration examples:

1. **Basic Configuration**: Simple backup task
2. **Security-Focused**: File verification and access control
3. **Resource Management**: Temporary directory and automatic cleanup
4. **Privilege Escalation**: System administration tasks
5. **Output Capture**: Log collection and report generation
6. **Variable Expansion**: Environment-specific deployment
7. **Comprehensive Configuration**: Full-stack application deployment
8. **Risk-Based Control**: Execution control based on risk level

Use these examples as references to create configuration files suited to your own environment and use cases.

## Next Steps

In the next chapter, we will learn best practices for creating configuration files. We will provide guidelines for creating better configuration files from the perspectives of security, maintainability, and performance.
