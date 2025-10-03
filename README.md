# Go Safe Command Runner

A secure command execution framework in Go with comprehensive security controls, designed for delegating privileged tasks and automated batch processing.

Project Page: https://github.com/isseis/go-safe-cmd-runner/

## Table of Contents

- [Background](#background)
- [Key Security Features](#key-security-features)
- [Recent Security Enhancements](#recent-security-enhancements)
- [Core Features](#core-features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Security Model](#security-model)
- [Command-Line Tools](#command-line-tools)
- [Build and Installation](#build-and-installation)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Background

Go Safe Command Runner addresses the critical need for secure command execution in environments where:
- General users need to safely perform privileged operations.
- Automated systems require secure batch processing capabilities.
- File integrity verification is essential before command execution.
- Strict control over environment variable exposure is necessary.
- Audit trails and security boundaries are required for command execution.

Common use cases include periodic backups, system maintenance tasks, and delegating specific administrative operations to non-root users while maintaining security controls.

## Key Security Features

### Multi-Layered Defense Architecture
- **Pre-execution Verification**: Prevents malicious configuration attacks with pre-use hash verification of configuration and environment files.
- **Fixed Hash Directory**: Production builds use only the default hash directory, eliminating custom hash directory attack vectors.
- **Secure Fixed PATH**: Uses a hardcoded secure PATH (`/sbin:/usr/sbin:/bin:/usr/bin`), completely eliminating PATH manipulation attacks.
- **Risk-Based Command Control**: Intelligent security assessment that automatically blocks high-risk operations.
- **Environment Variable Segregation**: Strict allowlist-based filtering with a zero-trust approach.
- **Hybrid Hash Encoding**: Space-efficient file integrity verification with automatic fallback.
- **Sensitive Data Protection**: Automatic detection and redaction of passwords, tokens, and API keys.

### Command Execution Security
- **User/Group Execution Control**: Secure user and group switching with comprehensive validation.
- **Privilege Management**: Controlled privilege escalation with automatic privilege dropping.
- **Path Validation**: Command path resolution with symbolic link attack prevention.
- **Output Capture Security**: Secure file permissions (0600) for output files.
- **Timeout Control**: Prevention of resource exhaustion attacks.

### Auditing and Monitoring
- **ULID Execution Tracking**: Chronologically sorted execution tracking with unique identifiers.
- **Multi-Handler Logging**: Console, file, and Slack integration with sensitive data redaction.
- **Interactive Terminal Support**: Color-coded output with smart terminal detection.
- **Comprehensive Audit Trail**: Complete logging of privileged operations and security events.

## Core Features

### File Integrity and Verification
- **SHA-256 Hash Verification**: Verification of all executables and critical files before execution.
- **Pre-execution Verification**: Verification of configuration and environment files before use.
- **Hybrid Hash Encoding**: Space-efficient encoding with a human-readable fallback.
- **Centralized Verification**: Unified verification management with automatic privilege handling.
- **Group and Global Verification**: Flexible file verification at multiple levels.

### Command Execution
- **Batch Processing**: Execution of commands in organized groups with dependency management.
- **Variable Expansion**: `${VAR}` style expansion in command names and arguments.
- **Output Capture**: Saving command output to files with secure permissions.
- **Background Execution**: Support for long-running processes with signal handling.
- **Enhanced Dry Run**: Realistic simulation with comprehensive security analysis.
- **Timeout Control**: Configurable timeouts for command execution.
- **User/Group Context**: Execution of commands as a specific user with validation.

### Logging and Monitoring
- **Multi-Handler Logging**: Log routing to multiple destinations (console, file, Slack).
- **Interactive Terminal Support**: Color-coded output for enhanced visibility.
- **Smart Terminal Detection**: Automatic detection of terminal capabilities.
- **Color Control**: Support for CLICOLOR, NO_COLOR, and CLICOLOR_FORCE environment variables.
- **Slack Integration**: Real-time notifications for security events.
- **Sensitive Data Redaction**: Automatic filtering of sensitive information.
- **ULID Execution Tracking**: Chronologically sorted execution tracking.

### File Operations
- **Safe File I/O**: Symlink-aware file operations with security checks.
- **Hash Recording**: Recording of SHA-256 hashes for integrity verification.
- **Verification Tool**: Standalone utility for file verification.

## Architecture

The system follows a modular architecture with a clear separation of concerns:

```
cmd/                    # Command-line entry points
├── runner/            # Main command runner application
├── record/            # Hash recording utility
└── verify/            # File verification utility

internal/              # Core implementation
├── cmdcommon/         # Shared command utilities
├── color/             # Terminal color support
├── common/            # Common utilities and filesystem abstraction
├── filevalidator/     # File integrity verification
│   └── encoding/      # Hybrid hash filename encoding
├── groupmembership/   # User/group membership validation
├── logging/           # Advanced logging with Slack integration
├── redaction/         # Automatic filtering of sensitive data
├── runner/            # Command execution engine
│   ├── audit/         # Security audit logging
│   ├── bootstrap/     # System initialization
│   ├── cli/           # Command-line interface
│   ├── config/        # Configuration management
│   ├── environment/   # Environment variable handling
│   ├── errors/        # Centralized error handling
│   ├── executor/      # Command execution logic
│   ├── hashdir/       # Hash directory security
│   ├── output/        # Output capture management
│   ├── privilege/     # Privilege management
│   ├── resource/      # Resource management (normal/dry-run)
│   ├── risk/          # Risk-based command evaluation
│   ├── runnertypes/   # Type definitions and interfaces
│   └── security/      # Security validation framework
├── safefileio/        # Secure file operations
├── terminal/          # Terminal capability detection
└── verification/      # Centralized verification management
```

## Quick Start

### Basic Usage

```bash
# Execute commands from a configuration file
./runner -config config.toml

# Dry-run mode (preview without execution)
./runner -config config.toml -dry-run

# Validate the configuration file
./runner -config config.toml -validate
```

For detailed usage, please refer to the [runner command guide](docs/user/runner_command.md).

### Simple Configuration Example

```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "backup"
description = "System backup operations"

[[groups.commands]]
name = "database_backup"
description = "Backup the database"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases"]
output = "backup.sql"  # Save output to a file
run_as_user = "mysql"
max_risk_level = "medium"
```

## Configuration

A TOML configuration file defines how commands are executed. The configuration file has the following hierarchical structure:

- **Root Level**: Version information
- **Global Level**: Default settings applied to all groups
- **Group Level**: Grouping of related commands
- **Command Level**: Individual command settings

### Basic Configuration Example

```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER", "LANG"]

[[groups]]
name = "maintenance"
description = "System maintenance tasks"
priority = 1

[[groups.commands]]
name = "system_check"
cmd = "/usr/bin/systemctl"
args = ["status"]
max_risk_level = "medium"
```

### Detailed Configuration

For detailed information on how to write the configuration file, please refer to the following documents:

- [TOML Configuration File User Guide](docs/user/toml_config/README.md) - Comprehensive configuration guide
  - [Configuration File Hierarchy](docs/user/toml_config/02_hierarchy.md)
  - [Global Level Settings](docs/user/toml_config/04_global_level.md)
  - [Group Level Settings](docs/user/toml_config/05_group_level.md)
  - [Command Level Settings](docs/user/toml_config/06_command_level.md)
  - [Variable Expansion Feature](docs/user/toml_config/07_variable_expansion.md)
  - [Practical Configuration Examples](docs/user/toml_config/08_practical_examples.md)

## Security Model

### File Integrity Verification

1. **Pre-execution Verification**
   - Configuration file verification before loading.
   - Environment file verification before use.
   - Prevention of malicious configuration attacks.

2. **Hash Directory Security**
   - Fixed default: `/usr/local/etc/go-safe-cmd-runner/hashes`
   - No custom directories in production environments.
   - Test API separated by build tags.

3. **Hybrid Hash Encoding**
   - Space-efficient encoding (1.00x inflation).
   - Human-readable for debugging.
   - Automatic SHA256 fallback.

4. **Verification Management**
   - Centralized verification.
   - Automatic privilege handling.
   - Group and global verification lists.

### Risk-Based Security Control

- **Automatic Risk Assessment**: Command classification by risk level.
- **Configurable Thresholds**: Risk level limits per command.
- **Automatic Blocking**: Automatic blocking of high-risk commands.
- **Risk Categories**:
  - **Low**: Basic operations (ls, cat, grep).
  - **Medium**: File modifications (cp, mv), package management.
  - **High**: System administration (systemctl), destructive operations.
  - **Critical**: Privilege escalation (sudo, su) - always blocked.

### Environment Segregation

- **Secure Fixed PATH**: Hardcoded `/sbin:/usr/sbin:/bin:/usr/bin`.
- **No PATH Inheritance**: Elimination of PATH manipulation attacks.
- **Allowlist Filtering**: Strict zero-trust environment control.
- **Variable Expansion**: Secure `${VAR}` expansion with an allowlist.
- **Command.Env Priority**: Configuration overrides the OS environment.

### Privilege Management

- **Automatic Dropping**: Privilege dropping after initialization.
- **Controlled Escalation**: Risk-aware privilege management.
- **User/Group Switching**: Secure context switching with validation.
- **Audit Trail**: Complete logging of privilege changes.

### Output Capture Security

- **Secure Permissions**: Output files created with 0600 permissions.
- **Privilege Separation**: Output files use the real UID (not `run_as_user`).
- **Directory Security**: Automatic directory creation with secure permissions.
- **Path Validation**: Prevention of path traversal attacks.

### Logging Security

- **Sensitive Data Redaction**: Automatic detection of secrets, tokens, and API keys.
- **Multi-Channel Notifications**: Encrypted Slack communication.
- **Audit Trail Protection**: Tamper-resistant structured logs.
- **Real-Time Alerts**: Immediate notification of security violations.

## Command-Line Tools

go-safe-cmd-runner provides three command-line tools:

### runner - Main Execution Command

```bash
# Basic execution
./runner -config config.toml

# Dry run (check what will be executed)
./runner -config config.toml -dry-run

# Configuration validation
./runner -config config.toml -validate
```

For more details, see the [runner command guide](docs/user/runner_command.md).

### record - Hash Recording Command

```bash
# Record a file hash
./record -file /path/to/executable

# Force overwrite of an existing hash
./record -file /path/to/file -force
```

For more details, see the [record command guide](docs/user/record_command.md).

### verify - File Verification Command

```bash
# Verify file integrity
./verify -file /path/to/file
```

For more details, see the [verify command guide](docs/user/verify_command.md).

### Comprehensive User Guide

For detailed usage, configuration examples, and troubleshooting, please refer to the [User Guide](docs/user/README.md).

## Build and Installation

### Prerequisites

- Go 1.23 or later (required for the `slices` package and `range over count`)
- golangci-lint (for development)
- gofumpt (for code formatting)

### Build Commands

```bash
# Build all binaries
make build

# Build a specific binary
make build/runner
make build/record
make build/verify

# Run tests
make test

# Run the linter
make lint

# Format the code
make fmt

# Clean build artifacts
make clean

# Run benchmarks
make benchmark

# Generate a coverage report
make coverage
```

### Installation

```bash
# Install from source
git clone https://github.com/isseis/go-safe-cmd-runner.git
cd go-safe-cmd-runner
make build

# Install binaries to a system location
sudo install -o root -g root -m 4755 build/runner /usr/local/bin/go-safe-cmd-runner
sudo install -o root -g root -m 0755 build/record /usr/local/bin/go-safe-cmd-record
sudo install -o root -g root -m 0755 build/verify /usr/local/bin/go-safe-cmd-verify

# Create the default hash directory
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes
```

## Development

### Dependencies

- `github.com/pelletier/go-toml/v2` - TOML configuration parsing
- `github.com/oklog/ulid/v2` - ULID generation for execution tracking
- `github.com/stretchr/testify` - Testing framework
- `golang.org/x/term` - Terminal capability detection

### Testing

```bash
# Run all tests
go test -v ./...

# Run tests for a specific package
go test -v ./internal/runner

# Run integration tests
make integration-test

# Run Slack notification tests (requires GSCR_SLACK_WEBHOOK_URL)
make slack-notify-test
make slack-group-notification-test
```

### Project Structure

The codebase follows Go best practices:
- **Interface-driven design** for testability
- **Comprehensive error handling** with custom error types
- **Security-first approach** with extensive validation
- **Modular architecture** with clear boundaries
- **Build tag separation** for production/test code

### Execution Identification with ULID

The system uses ULID (Universally Unique Lexicographically Sortable Identifier):
- **Chronologically sortable**: Naturally ordered by creation time.
- **URL-safe**: No special characters, suitable for filenames.
- **Compact**: 26-character fixed length.
- **Collision-resistant**: Monotonic entropy ensures uniqueness.
- **Example**: `01K2YK812JA735M4TWZ6BK0JH9`

## Out of Scope

This project explicitly does **not** provide:
- Container orchestration or Docker integration
- Network security features (firewalls, VPNs, etc.)
- User authentication or authorization systems
- Web interfaces or REST APIs
- Database management functions
- Real-time monitoring or alerting systems
- Cross-platform GUI applications
- Package management or software installation

It focuses on secure command execution with comprehensive security controls in Unix-like environments.

## Contributing

This project emphasizes security and reliability. When contributing:
- Follow security-first design principles.
- Add comprehensive tests for new features.
- Update documentation for configuration changes.
- Ensure all security validations are tested.
- Use static analysis tools (golangci-lint).
- Follow Go coding standards and best practices.

For questions or contributions, please refer to the project's issue tracker.

## License

This project is licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.
