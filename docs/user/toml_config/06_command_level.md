# Chapter 6: Command Level Configuration

## 6.1 Basic Command Settings

### name (Required)
```toml
[[groups.commands]]
name = "backup_database"
```

### description
```toml
[[groups.commands]]
name = "backup_database"
description = "Create PostgreSQL database backup"
```

### cmd (Required)
```toml
[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/pg_dump"
```

### args
```toml
[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/pg_dump"
args = ["-h", "localhost", "-U", "backup", "mydb"]
```

## 6.2 Environment Settings

### env
```toml
[[groups.commands]]
name = "deploy"
cmd = "/opt/scripts/deploy.sh"
env = { "DEPLOY_ENV" = "production", "VERSION" = "1.2.3" }
```

## 6.3 Execution Control

### timeout
```toml
[[groups.commands]]
name = "long_process"
cmd = "/opt/tools/processor"
timeout = 3600  # 1 hour
```

### run_as_user / run_as_group
```toml
[[groups.commands]]
name = "system_update"
cmd = "/usr/bin/apt"
args = ["update"]
run_as_user = "root"
run_as_group = "root"
```

## 6.4 Security Settings

### max_risk_level
```toml
[[groups.commands]]
name = "restart_service"
cmd = "/bin/systemctl"
args = ["restart", "nginx"]
max_risk_level = "high"  # low, medium, high
```

## 6.5 Output Management

### output
```toml
[[groups.commands]]
name = "system_info"
cmd = "/bin/uname"
args = ["-a"]
output = "system-info.txt"
```

## 6.6 Complete Example

```toml
[[groups.commands]]
name = "secure_backup"
description = "Create encrypted database backup"
cmd = "/usr/bin/pg_dump"
args = [
    "--host=localhost",
    "--username=backup_user",
    "--format=custom",
    "production_db"
]
env = { "PGPASSWORD" = "${DB_PASSWORD}" }
timeout = 1800
run_as_user = "postgres"
max_risk_level = "medium"
output = "db-backup.sql"
```
