# Chapter 3: Root Level Configuration

## 3.1 Version Parameter

The `version` parameter is the only required root-level setting.

### Syntax
```toml
version = "1.0"
```

### Purpose
- Specifies the configuration file format version
- Ensures compatibility with the go-safe-cmd-runner version
- Currently, only "1.0" is supported

### Examples

```toml
# Minimal valid configuration
version = "1.0"

[[groups]]
name = "example"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
args = ["Hello, World!"]
```

### Important Notes
- Must be the first line in the configuration file
- Required for all configuration files
- Invalid versions will cause configuration rejection
