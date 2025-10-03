# Chapter 1: Introduction

## 1.1 Purpose of This Document

This document is a user guide that explains how to write TOML configuration files for go-safe-cmd-runner. Through the structure of configuration files, details of each parameter, and practical usage examples, you can learn how to build a safe and efficient command execution environment.

### Target Audience

- System administrators who use go-safe-cmd-runner for batch processing and command execution
- Developers who want to build secure command execution environments
- Users who want to learn how to write TOML configuration files

## 1.2 TOML Configuration File Overview

go-safe-cmd-runner uses TOML (Tom's Obvious, Minimal Language) format configuration files to manage command execution. TOML is a configuration file format that is easy for humans to read and write, with clear semantics.

### Key Features

1. **Hierarchical Structure**: Three-layer structure of global settings, group settings, and command settings
2. **Security-Focused**: Security features including file verification, environment variable control, and privilege management
3. **Flexible Configuration**: Fine-grained control of timeouts, working directories, environment variables, etc.
4. **Variable Expansion**: Variable expansion functionality for dynamic command construction
5. **Output Management**: Command output capture and file saving

## 1.3 Basic Configuration File Structure

TOML configuration files have the following basic structure:

```toml
# Version specification (required)
version = "1.0"

# Global settings (optional)
[global]
timeout = 60
workdir = "/tmp/workspace"
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER"]

# Command groups (one or more required)
[[groups]]
name = "example_group"
description = "Sample group description"

# Commands within the group (one or more required)
[[groups.commands]]
name = "hello_world"
description = "Output Hello World"
cmd = "echo"
args = ["Hello, World!"]
```

### Structure Components

#### 1. Version Section (Required)
```toml
version = "1.0"
```
Specifies the configuration file format version. Currently, version "1.0" is supported.

#### 2. Global Section (Optional)
```toml
[global]
timeout = 60
log_level = "info"
# ... other global settings
```
Defines settings that apply to all command groups. These settings can be overridden at the group or command level.

#### 3. Groups Section (Required)
```toml
[[groups]]
name = "group_name"
description = "Group description"
# ... group-specific settings

[[groups.commands]]
name = "command_name"
cmd = "/path/to/command"
args = ["arg1", "arg2"]
# ... command-specific settings
```
Defines command groups and the commands within them. At least one group and one command within that group must be defined.

## 1.4 Configuration Principles

### Security First
- All executable files referenced in the configuration must have their hash values recorded beforehand
- Environment variables are controlled through allowlists
- Privilege escalation is managed through controlled mechanisms

### Inheritance and Override
- Settings are inherited from global → group → command levels
- Lower-level settings override higher-level settings
- This allows for efficient configuration management

### Validation and Safety
- Configuration files are thoroughly validated before execution
- Invalid configurations are rejected with clear error messages
- Dry-run functionality allows for safe testing of configurations

## 1.5 Getting Started

To begin using TOML configuration files:

1. **Study the Basic Structure**: Understand the three-layer hierarchy
2. **Start Simple**: Begin with minimal configurations and gradually add complexity
3. **Use Validation**: Always validate configurations using the `-validate` flag
4. **Test Safely**: Use dry-run mode to test configurations before production use
5. **Follow Security Practices**: Record hash values and use environment variable allowlists

### Next Steps

- **Chapter 2**: Learn about the detailed hierarchy structure
- **Chapter 3-6**: Understand each configuration level
- **Chapter 8**: Review practical examples
- **Chapter 9**: Learn best practices for security and maintainability

---

*This introduction provides the foundation for understanding TOML configuration files. The following chapters will dive deeper into each aspect of the configuration system.*
