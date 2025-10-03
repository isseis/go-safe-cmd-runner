# Chapter 4: Global Level Configuration

## 4.1 Overview

Global settings apply to all groups and commands unless overridden at lower levels.

## 4.2 Available Settings

### timeout
```toml
[global]
timeout = 300  # seconds
```
Default timeout for all commands.

### workdir
```toml
[global]
workdir = "/tmp/workspace"
```
Default working directory for command execution.

### log_level
```toml
[global]
log_level = "info"  # debug, info, warn, error
```
Logging verbosity level.

### skip_standard_paths
```toml
[global]
skip_standard_paths = false
```
Whether to skip verification of standard system paths.

### env_allowlist
```toml
[global]
env_allowlist = ["PATH", "HOME", "USER"]
```
List of environment variables allowed for command execution.

### verify_files
```toml
[global]
verify_files = ["/usr/bin/important-command"]
```
List of files to verify integrity before execution.

### max_output_size
```toml
[global]
max_output_size = 104857600  # bytes (100MB)
```
Maximum size for captured command output.

## 4.3 Example Configuration

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/opt/workspace"
log_level = "info"
env_allowlist = ["PATH", "HOME"]
verify_files = ["/usr/bin/rsync", "/bin/tar"]
max_output_size = 52428800  # 50MB
```
