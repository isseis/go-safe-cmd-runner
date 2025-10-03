# Chapter 3: Root Level Configuration

## 3.1 version

### Overview

`version` is a required parameter that specifies the format version of the configuration file. It is written at the top level (root level) of the configuration file.

### Syntax

```toml
version = "version string"
```

### Parameter Details

| Item | Content |
|-----|------|
| **Type** | String |
| **Required/Optional** | Required |
| **Configurable Level** | Root level only |
| **Default Value** | None (must be specified) |
| **Valid Values** | "1.0" (currently supported version) |

### Role

- **Compatibility Guarantee**: Records version information to accommodate future changes to the configuration file format
- **Validation**: Checks the configuration file version at runtime and detects incompatible configurations
- **Documentation**: Clearly indicates which version of the specification the configuration file follows

### Configuration Examples

#### Basic Configuration

```toml
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "example"

[[groups.commands]]
name = "test"
cmd = "echo"
args = ["Hello"]
```

In this example, the configuration file version is specified as "1.0".

### Important Notes

#### 1. Always Write First

It is recommended to write `version` at the beginning of the configuration file. Placing it before other sections improves readability.

```toml
# Recommended: Write version first
version = "1.0"

[global]
timeout = 60
```

```toml
# Not recommended: Version written later
[global]
timeout = 60

version = "1.0"  # Works but has poor readability
```

#### 2. Version String Format

The currently supported version is "1.0" only. If new versions are released in the future, they may include incompatible changes.

```toml
# Correct
version = "1.0"

# Incorrect: Unsupported version
version = "2.0"  # May result in an error
```

#### 3. Version Cannot Be Omitted

The `version` parameter is required. Omitting it will result in an error.

```toml
# Incorrect: Version is omitted
[global]
timeout = 60

[[groups]]
name = "example"
# ... (Error: version is not specified)
```

### Frequently Asked Questions

#### Q1: Why is version specification required?

A: It is to accommodate future changes to the configuration file format. Version information allows distinguishing between old and new configuration files and processing them appropriately.

#### Q2: What happens if I specify the wrong version number?

A: If you specify an unsupported version, go-safe-cmd-runner will refuse to load the configuration file and display an error message.

#### Q3: What is supported in version "1.0"?

A: All features explained in this document are supported in version "1.0":
- Global configuration
- Group and command definitions
- Environment variable management
- File verification
- Privilege management
- Variable expansion
- Output capture

### Best Practices

1. **Always Use the Current Latest Version**: Specify the latest version number to utilize new features
2. **Place at the Top of the Configuration File**: Improves readability and maintainability
3. **Record with Comments**: Document the configuration file creation date and reason for version in comments

```toml
# Configuration file for go-safe-cmd-runner
# Created: 2025-01-15
# Version 1.0 format
version = "1.0"

[global]
# ... configuration continues below
```

## Next Steps

The next chapter will explain global level configuration (`[global]`) in detail. You will learn important settings that affect the entire system, such as timeout, working directory, and environment variable management.
