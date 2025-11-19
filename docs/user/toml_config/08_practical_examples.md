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
env_allowed = ["PATH", "HOME"]

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
risk_level = "medium"
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
risk_level = "medium"
timeout = 600

[[groups.commands]]
name = "list_backups"
description = "List backup files"
cmd = "/bin/ls"
args = ["-lh", "*.tar.gz"]
output_file = "backup-list.txt"
```

## 8.2 Security-Focused Configuration Examples

### File Verification and Access Control

Configuration for environments with high security requirements:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/opt/secure"
verify_standard_paths = true  # Verify all files
env_allowed = ["PATH"]      # Minimal environment variables
verify_files = [
    "/bin/sh",
    "/bin/tar",
    "/usr/bin/gpg",
]

[[groups]]
name = "secure_backup"
description = "Secure backup process"
workdir = "/var/secure/backups"
env_allowed = ["PATH", "GPG_KEY_ID"]
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
risk_level = "medium"
timeout = 1800

[[groups.commands]]
name = "encrypt_backup"
description = "Encrypt backup"
cmd = "/usr/bin/gpg"
vars = ["gpg_key_id=admin@example.com"]
args = [
    "--encrypt",
    "--recipient", "%{gpg_key_id}",
    "data-backup.tar.gz",
]
risk_level = "medium"

[[groups.commands]]
name = "verify_encrypted"
description = "Verify encrypted file"
cmd = "/usr/bin/gpg"
args = [
    "--verify",
    "data-backup.tar.gz.gpg",
]
output_file = "verification-result.txt"
```

## 8.3 Configuration Examples with Resource Management

### Temporary Directory and Automatic Cleanup

Using a temporary workspace that is automatically deleted after processing:

```toml
version = "1.0"

[global]
timeout = 300
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "temp_processing"
description = "Data processing in temporary directory"
# Working directory uses automatically created temporary directory

[[groups.commands]]
name = "download_data"
description = "Download data"
cmd = "/usr/bin/curl"
args = [
    "-o", "data.csv",
    "https://example.com/data/export.csv",
]
risk_level = "medium"
timeout = 600

[[groups.commands]]
name = "process_data"
description = "Process data"
cmd = "/opt/tools/process"
args = [
    "--input", "data.csv",
    "--output", "processed.csv",
]
risk_level = "medium"
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
risk_level = "medium"
timeout = 600
output_file = "upload-response.txt"
```

## 8.4 Configuration Examples with Privilege Escalation

### System Administration Tasks

System maintenance requiring root privileges:

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/tmp"
env_allowed = ["PATH", "HOME"]
verify_files = [
    "/usr/bin/apt-get",
    "/usr/bin/systemctl",
]

[[groups]]
name = "system_maintenance"
description = "System maintenance tasks"

# Non-privileged task: Check system status
[[groups.commands]]
name = "check_disk_space"
description = "Check disk usage"
cmd = "/bin/df"
args = ["-h"]
output_file = "disk-usage.txt"

# Privileged task: Update packages
[[groups.commands]]
name = "update_packages"
description = "Update package list"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
risk_level = "high"
timeout = 900

# Privileged task: Restart service
[[groups.commands]]
name = "restart_service"
description = "Restart application service"
cmd = "/usr/bin/systemctl"
args = ["restart", "myapp.service"]
run_as_user = "root"
risk_level = "high"

# Non-privileged task: Check service status
[[groups.commands]]
name = "check_service_status"
description = "Check service status"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp.service"]
output_file = "service-status.txt"
```

## 8.5 Configuration Examples Using Output Capture

### Log Collection and Report Generation

Collecting output from multiple commands to create a report:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/var/reports"
env_allowed = ["PATH", "HOME"]
output_size_limit = 10485760  # 10MB

[[groups]]
name = "system_report"
description = "Generate system status report"

[[groups.commands]]
name = "disk_usage_report"
description = "Disk usage report"
cmd = "/bin/df"
args = ["-h"]
output_file = "reports/disk-usage.txt"

[[groups.commands]]
name = "memory_report"
description = "Memory usage report"
cmd = "/usr/bin/free"
args = ["-h"]
output_file = "reports/memory-usage.txt"

[[groups.commands]]
name = "process_report"
description = "Process list report"
cmd = "/bin/ps"
args = ["aux"]
output_file = "reports/processes.txt"

[[groups.commands]]
name = "network_report"
description = "Network connection status report"
cmd = "/bin/netstat"
args = ["-tuln"]
output_file = "reports/network-connections.txt"

[[groups.commands]]
name = "service_report"
description = "Service status report"
cmd = "/usr/bin/systemctl"
args = ["list-units", "--type=service", "--state=running"]
output_file = "reports/services.txt"

# Archive report files
[[groups.commands]]
name = "archive_reports"
description = "Compress reports"
cmd = "/bin/tar"
vars = ["date=2025-10-02"]
args = [
    "-czf",
    "system-report-%{date}.tar.gz",
    "reports/",
]
risk_level = "medium"
```

## 8.6 Configuration Examples Using Variable Expansion

### Environment-Specific Deployment

Using different configurations for development, staging, and production environments:

```toml
version = "1.0"

[global]
timeout = 600
env_allowed = ["PATH", "HOME"]

# Development environment
[[groups]]
name = "deploy_development"
description = "Deploy to development environment"

[[groups.commands]]
name = "deploy_dev_config"
cmd = "/bin/cp"
vars = [
    "config_dir=/opt/configs",
    "env_type=development",
]
args = [
    "%{config_dir}/%{env_type}/app.yml",
    "/etc/myapp/app.yml",
]
risk_level = "medium"

[[groups.commands]]
name = "start_dev_server"
vars = [
    "app_bin=/opt/myapp/bin/server",
    "log_level=debug",
    "api_port=8080",
    "db_url=postgresql://localhost/dev_db",
]
cmd = "%{app_bin}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "%{log_level}",
    "--port", "%{api_port}",
    "--database", "%{db_url}",
]
risk_level = "high"

# Staging environment
[[groups]]
name = "deploy_staging"
description = "Deploy to staging environment"

[[groups.commands]]
name = "deploy_staging_config"
cmd = "/bin/cp"
vars = [
    "config_dir=/opt/configs",
    "env_type=staging",
]
args = [
    "%{config_dir}/%{env_type}/app.yml",
    "/etc/myapp/app.yml",
]
risk_level = "medium"

[[groups.commands]]
name = "start_staging_server"
vars = [
    "app_bin=/opt/myapp/bin/server",
    "log_level=info",
    "api_port=8081",
    "db_url=postgresql://staging-db/staging_db",
]
cmd = "%{app_bin}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "%{log_level}",
    "--port", "%{api_port}",
    "--database", "%{db_url}",
]
risk_level = "high"

# Production environment
[[groups]]
name = "deploy_production"
description = "Deploy to production environment"

[[groups.commands]]
name = "deploy_prod_config"
cmd = "/bin/cp"
vars = [
    "config_dir=/opt/configs",
    "env_type=production",
]
args = [
    "%{config_dir}/%{env_type}/app.yml",
    "/etc/myapp/app.yml",
]
risk_level = "medium"

[[groups.commands]]
name = "start_prod_server"
vars = [
    "app_bin=/opt/myapp/bin/server",
    "log_level=warn",
    "api_port=8082",
    "db_url=postgresql://prod-db/prod_db",
]
cmd = "%{app_bin}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "%{log_level}",
    "--port", "%{api_port}",
    "--database", "%{db_url}",
]
run_as_user = "appuser"
risk_level = "high"
```

## 8.7 Comprehensive Configuration Examples

### Full-Stack Application Deployment

Integrated deployment of database, application, and web server:

```toml
version = "1.0"

[global]
timeout = 900
workdir = "/opt/deploy"
verify_standard_paths = false
env_allowed = [
    "PATH",
    "HOME",
    "DB_USER",
    "DB_NAME",
    "APP_DIR",
    "WEB_ROOT",
    "BACKUP_DIR",
]
output_size_limit = 52428800  # 50MB

# Phase 1: Preparation
[[groups]]
name = "preparation"
description = "Pre-deployment preparation"
workdir = "/opt/deploy/prep"

[[groups.commands]]
name = "backup_current_version"
description = "Backup current version"
cmd = "/bin/tar"
vars = [
    "backup_dir=/var/backups/app",
    "app_dir=/opt/myapp",
    "timestamp=2025-10-02-120000",
]
args = [
    "-czf",
    "%{backup_dir}/app-backup-%{timestamp}.tar.gz",
    "%{app_dir}",
]
risk_level = "medium"
timeout = 1800

[[groups.commands]]
name = "check_dependencies"
description = "Check dependencies"
cmd = "/usr/bin/dpkg"
args = ["-l"]
output_file = "installed-packages.txt"

# Phase 2: Database update
[[groups]]
name = "database_migration"
description = "Update database schema"
env_allowed = ["PATH", "DB_USER", "DB_NAME", "PGPASSWORD"]
verify_files = ["/usr/bin/psql", "/usr/bin/pg_dump"]

[[groups.commands]]
name = "backup_database"
description = "Backup database"
cmd = "/usr/bin/pg_dump"
vars = [
    "db_user=appuser",
    "db_name=myapp_db",
    "timestamp=2025-10-02-120000",
]
env_vars = ["PGPASSWORD=secret123"]
args = [
    "-U", "%{db_user}",
    "-d", "%{db_name}",
    "-F", "c",
    "-f", "/var/backups/db/backup-%{timestamp}.dump",
]
risk_level = "medium"
timeout = 1800
output_file = "db-backup-log.txt"

[[groups.commands]]
name = "run_migrations"
description = "Run database migrations"
cmd = "/opt/myapp/bin/migrate"
vars = [
    "db_user=appuser",
    "db_name=myapp_db",
]
args = [
    "--database", "postgresql://%{db_user}@localhost/%{db_name}",
    "--migrations", "/opt/myapp/migrations",
]
risk_level = "high"
timeout = 600

# Phase 3: Application deployment
[[groups]]
name = "application_deployment"
description = "Deploy application"
workdir = "/opt/myapp"

[[groups.commands]]
name = "stop_application"
description = "Stop application"
cmd = "/usr/bin/systemctl"
args = ["stop", "myapp.service"]
run_as_user = "root"
risk_level = "high"

[[groups.commands]]
name = "deploy_new_version"
description = "Deploy new version"
cmd = "/bin/tar"
args = [
    "-xzf",
    "/opt/deploy/releases/myapp-v2.0.0.tar.gz",
    "-C", "/opt/myapp",
]
risk_level = "medium"

[[groups.commands]]
name = "install_dependencies"
description = "Install dependencies"
cmd = "/usr/bin/pip3"
args = [
    "install",
    "-r", "/opt/myapp/requirements.txt",
]
risk_level = "high"
timeout = 600

[[groups.commands]]
name = "start_application"
description = "Start application"
cmd = "/usr/bin/systemctl"
args = ["start", "myapp.service"]
run_as_user = "root"
risk_level = "high"

# Phase 4: Web server configuration update
[[groups]]
name = "web_server_update"
description = "Update web server configuration"

[[groups.commands]]
name = "update_nginx_config"
description = "Update Nginx configuration"
cmd = "/bin/cp"
args = [
    "/opt/deploy/configs/nginx/myapp.conf",
    "/etc/nginx/sites-available/myapp.conf",
]
run_as_user = "root"
risk_level = "high"

[[groups.commands]]
name = "test_nginx_config"
description = "Validate Nginx configuration"
cmd = "/usr/bin/nginx"
args = ["-t"]
run_as_user = "root"
risk_level = "medium"
output_file = "nginx-config-test.txt"

[[groups.commands]]
name = "reload_nginx"
description = "Reload Nginx"
cmd = "/usr/bin/systemctl"
args = ["reload", "nginx"]
run_as_user = "root"
risk_level = "high"

# Phase 5: Deployment verification
[[groups]]
name = "deployment_verification"
description = "Verify deployment"

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
output_file = "health-check-result.txt"

[[groups.commands]]
name = "smoke_test"
description = "Basic functionality test"
cmd = "/usr/bin/curl"
args = [
    "-f",
    "-s",
    "http://localhost:8080/api/status",
]
output_file = "smoke-test-result.txt"

[[groups.commands]]
name = "verify_database_connection"
description = "Verify database connection"
cmd = "/usr/bin/psql"
vars = [
    "db_user=appuser",
    "db_name=myapp_db",
]
args = [
    "-U", "%{db_user}",
    "-d", "%{db_name}",
    "-c", "SELECT version();",
]
output_file = "db-connection-test.txt"

# Phase 6: Post-processing and reporting
[[groups]]
name = "post_deployment"
description = "Post-deployment processing"
workdir = "/var/reports/deployment"

[[groups.commands]]
name = "generate_deployment_report"
description = "Generate deployment report"
cmd = "/opt/tools/generate-report"
vars = ["timestamp=2025-10-02-120000"]
args = [
    "--deployment-log", "/var/log/deploy.log",
    "--output", "deployment-report-%{timestamp}.html",
]

[[groups.commands]]
name = "cleanup_temp_files"
description = "Delete temporary files"
cmd = "/bin/rm"
args = ["-rf", "/opt/deploy/temp"]
risk_level = "medium"

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
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "risk_controlled_operations"
description = "Operations controlled based on risk level"

# Low risk: Read-only operations
[[groups.commands]]
name = "read_config"
description = "Read configuration file"
cmd = "/bin/cat"
args = ["/etc/myapp/config.yml"]
output_file = "config-content.txt"

# Medium risk: File creation/modification
[[groups.commands]]
name = "update_cache"
description = "Update cache file"
cmd = "/opt/myapp/update-cache"
args = ["--refresh"]
risk_level = "medium"

# High risk: System changes
[[groups.commands]]
name = "system_update"
description = "Update system packages"
cmd = "/usr/bin/apt-get"
args = ["upgrade", "-y"]
run_as_user = "root"
risk_level = "high"
timeout = 1800

# Example that will be rejected for exceeding risk level
[[groups.commands]]
name = "dangerous_deletion"
description = "Mass deletion (cannot run at default risk level)"
cmd = "/bin/rm"
args = ["-rf", "/tmp/old-data"]
# risk_level defaults to "low"
# rm -rf requires medium risk or higher → execution rejected
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
