# go-safe-cmd-runner TOML Configuration File User Guide

A comprehensive user guide explaining how to write TOML configuration files for go-safe-cmd-runner.

## Table of Contents

### Chapter 1: [Introduction](01_introduction.md)
- Purpose of this document
- Overview of TOML configuration files
- Basic structure of configuration files

### Chapter 2: [Configuration File Hierarchy](02_hierarchy.md)
- Hierarchy overview diagram
- Three-layer structure explanation
  - Root level
  - Global level
  - Group level
  - Command level
- Configuration inheritance and override mechanisms

### Chapter 3: [Root Level Configuration](03_root_level.md)
- version parameter
  - Overview and role
  - Configuration examples
  - Important notes

### Chapter 4: [Global Level Configuration](04_global_level.md)
- timeout - Timeout settings
- workdir - Working directory
- log_level - Log level
- skip_standard_paths - Skip standard path validation
- env_allowlist - Environment variable allowlist
- verify_files - File verification list
- max_output_size - Output size limit

### Chapter 5: [Group Level Configuration](05_group_level.md)
- Basic group settings
  - name - Group name
  - description - Description
  - priority - Priority
- Resource management settings
  - temp_dir - Temporary directory
  - workdir - Working directory
- Security settings
  - verify_files - File verification
  - env_allowlist - Environment variable allowlist
- Environment variable inheritance modes
  - Inheritance mode (inherit)
  - Explicit mode (explicit)
  - Rejection mode (reject)

### Chapter 6: [Command Level Configuration](06_command_level.md)
- Basic command settings
  - name - Command name
  - description - Description
  - cmd - Executable command
  - args - Arguments
- Environment settings
  - env - Environment variables
- Timeout settings
  - timeout - Command-specific timeout
- Privilege management
  - run_as_user - Execution user
  - run_as_group - Execution group
- Risk management
  - max_risk_level - Maximum risk level
- Output management
  - output - Standard output capture

### Chapter 7: [Variable Expansion Feature](07_variable_expansion.md)
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

### Chapter 8: [Practical Configuration Examples](08_practical_examples.md)
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
- Performance optimization best practices
- Testing and validation best practices

### Chapter 10: [Output Capture Feature](10_output_capture.md)
- Overview of output capture
- Configuration methods
- File output settings
- Security considerations
- Performance considerations
- Practical usage examples
- Integration with other features

### Chapter 11: [Risk-Based Command Control](11_risk_based_control.md)
- Risk level system overview
- Risk assessment criteria
- Risk level configuration
- Command execution control based on risk
- Security policy integration
- Monitoring and auditing
- Best practices for risk management

## Quick Reference

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

### Security-Enhanced Configuration
```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "HOME"]
verify_files = ["/usr/bin/important-command"]

[[groups]]
name = "secure_operations"
description = "Security-enhanced command group"
priority = 1

[[groups.commands]]
name = "secure_backup"
description = "Secure backup operation"
cmd = "/usr/local/bin/backup"
args = ["--secure", "--verify"]
timeout = 1800
run_as_user = "backup"
max_risk_level = "medium"
```

## Getting Started

1. **Read Chapter 1-3**: Understand basic concepts and structure
2. **Review Chapter 8**: Study practical examples relevant to your use case
3. **Follow Chapter 9**: Implement security best practices
4. **Test Configuration**: Use `runner -config your-config.toml -validate` to validate
5. **Dry Run**: Use `runner -config your-config.toml -dry-run` to test safely

## Advanced Topics

For complex use cases, refer to:
- **Variable Expansion (Chapter 7)**: Dynamic configuration based on environment
- **Output Capture (Chapter 10)**: Capturing and processing command output
- **Risk-Based Control (Chapter 11)**: Implementing security policies based on risk levels

## Related Documentation

- [Runner Command Guide](../runner_command.md) - How to execute configurations
- [Record Command Guide](../record_command.md) - Managing file hashes for verification
- [Verify Command Guide](../verify_command.md) - Verifying file integrity
- [Security Risk Assessment](../security-risk-assessment.md) - Understanding security implications
- [Project README](../../../README.md) - Overall project overview

---

*This guide provides comprehensive coverage of TOML configuration capabilities. For specific implementation details and advanced features, refer to the individual chapter files listed above.*
