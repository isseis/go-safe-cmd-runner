# Chapter 5: Group Level Configuration

## 5.1 Basic Group Settings

### name (Required)
```toml
[[groups]]
name = "backup_operations"
```

### description
```toml
[[groups]]
name = "backup_operations"
description = "Daily backup tasks"
```

### priority
```toml
[[groups]]
name = "backup_operations"
priority = 1  # Lower numbers execute first
```

## 5.2 Resource Management

### workdir
```toml
[[groups]]
name = "data_processing"
workdir = "/opt/processing"
```

### temp_dir
```toml
[[groups]]
name = "data_processing"
temp_dir = "/var/tmp/processing"
```

## 5.3 Security Settings

### env_allowlist
```toml
[[groups]]
name = "secure_operations"
env_allowlist = ["PATH"]  # Restrictive list
```

### verify_files
```toml
[[groups]]
name = "critical_tasks"
verify_files = ["/opt/app/critical-tool"]
```

## 5.4 Environment Variable Inheritance

### inherit (Default)
```toml
[[groups]]
name = "standard_ops"
env_inheritance_mode = "inherit"
env_allowlist = ["PATH", "HOME"]  # Filter inherited variables
```

### explicit
```toml
[[groups]]
name = "secure_ops"
env_inheritance_mode = "explicit"
env_allowlist = ["PATH"]  # Only these variables allowed
```

### reject
```toml
[[groups]]
name = "isolated_ops"
env_inheritance_mode = "reject"  # No environment inheritance
```

## 5.5 Example Configuration

```toml
[[groups]]
name = "database_operations"
description = "Database backup and maintenance"
priority = 1
workdir = "/var/db-backup"
temp_dir = "/var/tmp/db-work"
env_inheritance_mode = "explicit"
env_allowlist = ["PATH", "PGPASSWORD"]
verify_files = ["/usr/bin/pg_dump"]
```
