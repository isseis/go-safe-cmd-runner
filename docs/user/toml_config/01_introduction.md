# Chapter 1: Introduction

## 1.1 Purpose of This Document

This document is a user guide that explains how to write TOML configuration files for go-safe-cmd-runner. Through the configuration file structure, detailed parameters, and practical usage examples, you will learn how to build a secure and efficient command execution environment.

### Target Audience

- System administrators who use go-safe-cmd-runner for batch processing and command execution
- Developers who want to build secure command execution environments
- Users who want to master how to write TOML configuration files

## 1.2 TOML Configuration File Overview

go-safe-cmd-runner uses TOML (Tom's Obvious, Minimal Language) format configuration files to manage command execution. TOML is a configuration file format that is easy for humans to read and write, with clear semantics.

### Key Features

1. **Hierarchical Structure**: Three-layer structure of global configuration, group configuration, and command configuration
2. **Security Focus**: Security features including file verification, environment variable control, and privilege management
3. **Flexible Configuration**: Fine-grained control of timeout, working directory, environment variables, etc.
4. **Variable Expansion**: Variable expansion functionality for dynamic command construction
5. **Output Management**: Command output capture and file storage

## 1.3 Basic Configuration File Structure

TOML configuration files have the following basic structure:

```toml
# Version specification (required)
version = "1.0"

# Global configuration (optional)
[global]
timeout = 60
workdir = "/tmp/workspace"
env_allowed = ["PATH", "HOME", "USER"]

# Command group (one or more required)
[[groups]]
name = "example_group"
description = "Description of sample group"

# Commands within group (one or more required)
[[groups.commands]]
name = "hello_world"
description = "Output Hello World"
cmd = "echo"
args = ["Hello, World!"]
```

### Configuration Components

1. **Root Level**: Version information
2. **Global Level** (`[global]`): Common configuration applied to all groups
3. **Group Level** (`[[groups]]`): Unit for grouping related commands
4. **Command Level** (`[[groups.commands]]`): Definition of commands to actually execute

Details of each level will be explained in subsequent chapters.

### Minimal Configuration Example

The simplest configuration file looks like this:

```toml
version = "1.0"

[[groups]]
name = "minimal"

[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
```

This example defines only version information, one group, and one command.

## Next Steps

The next chapter will explain the hierarchical structure of the configuration file in detail, and you will learn how each level interacts with each other.
