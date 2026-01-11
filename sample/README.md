# Sample Configuration Files

This directory contains example configuration files demonstrating various features of go-safe-cmd-runner.

## Template Includes Feature

### Template Files

- **`templates_backup_commands.toml`**: Reusable backup command templates (restic, tar)
- **`templates_docker_commands.toml`**: Reusable Docker command templates (run, exec, compose)

### Main Configuration Using Includes

- **`includes_example.toml`**: Demonstrates how to use template includes to organize commands

Example workflow:
```bash
# 1. Validate the configuration with includes
safe-cmd-runner runner -c sample/includes_example.toml --dry-run

# 2. Record hashes for all files (main config + included templates)
safe-cmd-runner record -c sample/includes_example.toml -o /tmp/hashes/

# 3. Run commands using templates from included files
safe-cmd-runner runner -c sample/includes_example.toml -g backup_tasks -d /tmp/hashes/ -r backup-run-001
```

## Command Templates

- **`command_template_example.toml`**: Basic command template usage examples
- **`starter.toml`**: Simple starter configuration

## Variable Expansion

- **`variable_expansion_basic.toml`**: Basic variable expansion examples
- **`variable_expansion_advanced.toml`**: Advanced variable expansion patterns
- **`variable_expansion_security.toml`**: Security-focused variable examples
- **`vars_env_separation_e2e.toml`**: Demonstrates separation of vars and env_vars

## Environment Variables

- **`auto_env_example.toml`**: Automatic environment variable import examples
- **`auto_env_group.toml`**: Group-level environment variable examples
- **`dot.env.sample`**: Sample .env file format

## Output Capture

- **`output_capture_basic.toml`**: Basic output capture configuration
- **`output_capture_advanced.toml`**: Advanced output capture with size limits
- **`output_capture_security.toml`**: Security considerations for output capture

## Risk-Based Control

- **`risk-based-control.toml`**: Demonstrates risk level configuration and control

## Other Features

- **`timeout_examples.toml`**: Timeout configuration examples
- **`workdir_examples.toml`**: Working directory configuration examples
- **`comprehensive.toml`**: Comprehensive example showing multiple features

## Testing Configurations

Files used for testing purposes:
- `auto_env_test.toml`
- `group_cmd_allowed.toml`
- `output_capture_error_test.toml`
- `output_capture_single_error.toml`
- `output_capture_too_large_error.toml`
- `slack-group-notification-test.toml`
- `slack-notify.toml`
- `variable_expansion_test.toml`
