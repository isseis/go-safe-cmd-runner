# Chapter 2: Configuration File Hierarchy

## 2.1 Hierarchy Overview

go-safe-cmd-runner uses a three-layer configuration hierarchy that allows for flexible and secure command execution management.

### Configuration Layers

```
Root Level (version)
├── Global Level ([global])
├── Group Level ([[groups]])
└── Command Level ([[groups.commands]])
```

## 2.2 Inheritance and Override

Settings are inherited and can be overridden at lower levels:

- **Global** → **Group** → **Command**
- Lower levels override higher level settings
- This provides both default values and specific customization

## 2.3 Example Structure

```toml
version = "1.0"                    # Root level

[global]                          # Global level
timeout = 300
env_allowlist = ["PATH", "HOME"]

[[groups]]                        # Group level
name = "backup"
timeout = 600                     # Overrides global timeout

[[groups.commands]]               # Command level
name = "db_backup"
timeout = 1800                    # Overrides group timeout
```

## 2.4 Configuration Priority

1. **Command-level** settings (highest priority)
2. **Group-level** settings
3. **Global-level** settings
4. **System defaults** (lowest priority)

This hierarchy ensures both flexibility and consistency across your command configurations.
