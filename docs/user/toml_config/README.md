# go-safe-cmd-runner TOML Configuration File User Guide

A comprehensive user guide explaining how to write TOML configuration files for go-safe-cmd-runner.

## Table of Contents

### Chapter 1: [Introduction](01_introduction.md)
- Purpose of this document
- Overview of TOML configuration files
- Basic structure of configuration files

### Chapter 2: [Configuration File Hierarchy](02_hierarchy.md)
- Hierarchy overview diagram
- Three-tier structure explanation
  - Root level
  - Global level
  - Group level
  - Command level
- Configuration inheritance and override mechanism

### Chapter 3: [Root Level Configuration](03_root_level.md)
- version parameter
  - Overview and purpose
  - Configuration examples
  - Notes and considerations

### Chapter 4: [Global Level Configuration](04_global_level.md)
- timeout - Timeout configuration
- workdir - Working directory
- skip_standard_paths - Skip standard path validation
- env_allowed - Environment variable allowlist
- verify_files - File verification list
- output_size_limit - Maximum output size

### Chapter 5: [Group Level Configuration](05_group_level.md)
- Group basic configuration
  - name - Group name
  - description - Description
  - priority - Priority
- Resource management configuration
  - workdir - Working directory
- Security configuration
  - verify_files - File verification
  - env_allowed - Environment variable allowlist
- Environment variable inheritance modes
  - Inherit mode (inherit)
  - Explicit mode (explicit)
  - Reject mode (reject)

### Chapter 6: [Command Level Configuration](06_command_level.md)
- Command basic configuration
  - name - Command name
  - description - Description
  - cmd - Execution command
  - args - Arguments
- Environment configuration
  - env - Environment variables
- Timeout configuration
  - timeout - Command-specific timeout
- Privilege Management
  - run_as_user - Execution user
  - run_as_group - Execution group
- Risk management
  - risk_level - Maximum risk level
- Output management
  - output - Standard output capture

### Chapter 7: [Variable Expansion](07_variable_expansion.md)
- Overview of variable expansion
- Variable expansion syntax
- Available locations
  - Variable expansion in cmd
  - Variable expansion in args
  - Combining multiple variables
- Practical examples
  - Dynamic command path construction
  - Dynamic argument generation
  - Environment-specific configuration switching
- Nested variables
- Escape sequences
- Security considerations

### Chapter 8: [Practical Examples](08_practical_examples.md)
- Basic configuration examples
- Security-focused configuration examples
- Configuration examples with resource management
- Configuration examples with privilege escalation
- Configuration examples using output capture
- Configuration examples utilizing variable expansion
- Complex configuration examples
- Risk-based control examples

### Chapter 9: [Best Practices](09_best_practices.md)
- Security best practices
- Environment variable management best practices
- Group configuration best practices
- Error handling best practices
- Maintainability best practices
- Performance best practices
- Testing and validation
- Documentation

### Chapter 10: [Troubleshooting](10_troubleshooting.md)
- Common errors and solutions
- Configuration validation methods
- Debugging techniques
- Performance issues
- Frequently Asked Questions (FAQ)

### [Appendix](appendix.md)
- Appendix A: Parameter Reference Table
- Appendix B: Sample Configuration File Collection
- Appendix C: Glossary
- Appendix D: Configuration File Templates
- Appendix E: Reference Links

## Quick Start

### Minimal Configuration Example

```toml
version = "1.0"

[[groups]]
name = "hello_world"

[[groups.commands]]
name = "greet"
cmd = "/bin/echo"
args = ["Hello, World!"]
```

### Basic Usage

1. **Create configuration file**: Create a `config.toml` file
2. **Create hash file** (if needed):
   ```bash
  record -file config.toml
  record -file /bin/echo
   ```
3. **Execute**:
   ```bash
   go-safe-cmd-runner -file config.toml
   ```

## Recommended Learning Order

1. **Beginners**: Understand the basics with Chapters 1-3
2. **Intermediate**: Learn parameters with Chapters 4-6
3. **Advanced**: Master advanced features and best practices with Chapters 7-9
4. **Troubleshooting**: Resolve issues with Chapter 10
5. **Reference**: Consult the appendix

## Sample Files

The project's `sample/` directory contains sample configuration files for various use cases:

- `sample/comprehensive.toml` - Comprehensive example covering all features
- `sample/variable_expansion_basic.toml` - Basic variable expansion example
- `sample/output_capture_basic.toml` - Basic output capture example
- Many other sample files

## Contributing

For documentation improvement suggestions or error reports, please use GitHub Issues or Pull Requests.

## License

This document is provided under the same license as the go-safe-cmd-runner project.

---

**Last Updated**: 2025-10-02
**Version**: 1.0
