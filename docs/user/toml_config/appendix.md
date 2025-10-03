# Appendix: TOML Configuration Reference

## A.1 Quick Reference

### Minimal Configuration
```toml
version = "1.0"

[[groups]]
name = "example"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
args = ["Hello, World!"]
```

### Complete Configuration Template
```toml
version = "1.0"

[global]
timeout = 600
workdir = "/opt/workspace"
log_level = "info"
skip_standard_paths = false
env_allowlist = ["PATH", "HOME"]
verify_files = ["/usr/bin/important"]
max_output_size = 104857600

[[groups]]
name = "example_group"
description = "Example command group"
priority = 1
workdir = "/opt/group-workspace"
temp_dir = "/var/tmp/group-temp"
env_inheritance_mode = "inherit"
env_allowlist = ["PATH", "HOME", "USER"]
verify_files = ["/opt/group/tool"]

[[groups.commands]]
name = "example_command"
description = "Example command"
cmd = "/bin/echo"
args = ["Hello", "${USER}"]
env = { "CUSTOM_VAR" = "value" }
timeout = 300
run_as_user = "nobody"
run_as_group = "nogroup"
max_risk_level = "low"
output = "output.txt"
```

## A.2 Parameter Reference

### Root Level
- `version` (string, required): Configuration format version

### Global Level ([global])
- `timeout` (integer): Default timeout in seconds
- `workdir` (string): Default working directory
- `log_level` (string): Logging level (debug, info, warn, error)
- `skip_standard_paths` (boolean): Skip standard path verification
- `env_allowlist` (array): Allowed environment variables
- `verify_files` (array): Files to verify integrity
- `max_output_size` (integer): Maximum output size in bytes

### Group Level ([[groups]])
- `name` (string, required): Group name
- `description` (string): Group description
- `priority` (integer): Execution priority
- `workdir` (string): Group working directory
- `temp_dir` (string): Group temporary directory
- `env_inheritance_mode` (string): inherit, explicit, reject
- `env_allowlist` (array): Group environment variable allowlist
- `verify_files` (array): Group-specific files to verify

### Command Level ([[groups.commands]])
- `name` (string, required): Command name
- `description` (string): Command description
- `cmd` (string, required): Executable path
- `args` (array): Command arguments
- `env` (table): Command environment variables
- `timeout` (integer): Command-specific timeout
- `run_as_user` (string): Execution user
- `run_as_group` (string): Execution group
- `max_risk_level` (string): low, medium, high
- `output` (string): Output capture file

## A.3 Environment Variable Inheritance Modes

### inherit (default)
- Inherits parent environment
- Filters through allowlist
- Allows additional variables at command level

### explicit
- Only explicitly allowed variables
- No inheritance from parent environment
- Strict allowlist enforcement

### reject
- No environment variable inheritance
- Only command-level env variables
- Maximum isolation

## A.4 Risk Levels

### low
- Read-only operations
- No system modifications
- Minimal security impact

### medium
- Configuration changes
- Data processing operations
- Moderate security impact

### high
- System modifications
- Privilege escalation required
- High security impact

## A.5 Validation Rules

1. **Version**: Must be "1.0"
2. **Groups**: At least one group required
3. **Commands**: At least one command per group required
4. **Names**: Must be unique within scope
5. **Paths**: Commands must use absolute paths
6. **Environment**: Variables must be in allowlist for expansion
7. **Files**: Referenced files should have recorded hashes
8. **Timeouts**: Must be positive integers
9. **Risk Levels**: Must be valid level (low/medium/high)

## A.6 Common TOML Syntax

### Strings
```toml
name = "simple string"
path = "/absolute/path"
```

### Arrays
```toml
args = ["arg1", "arg2", "arg3"]
env_allowlist = ["PATH", "HOME"]
```

### Tables (Inline)
```toml
env = { "VAR1" = "value1", "VAR2" = "value2" }
```

### Boolean
```toml
skip_standard_paths = true
```

### Integer
```toml
timeout = 300
priority = 1
```

This reference provides a complete overview of all configuration options available in go-safe-cmd-runner TOML files.
