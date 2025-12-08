# Go Safe Command Runner

A secure command execution framework in Go with comprehensive security controls designed for privilege delegation and automated batch processing.

Project page: https://github.com/isseis/go-safe-cmd-runner/

## Table of Contents

- [Background](#background)
- [Key Security Features](#key-security-features)
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
- Regular users need to execute privileged operations safely
- Automated systems require secure batch processing capabilities
- File integrity verification is essential before command execution
- Strict control over environment variable exposure is necessary
- Audit trail and security boundaries are required for command execution

Common use cases include scheduled backups, system maintenance tasks, and delegating specific administrative operations to non-root users while maintaining security controls.

## Key Security Features

### Defense-in-Depth Architecture
- **Pre-execution Verification**: Hash validation of configuration and environment files before use prevents malicious configuration attacks
- **Fixed Hash Directory**: Production builds use only default hash directory, eliminating custom hash directory attack vectors
- **Secure Fixed PATH**: Uses hardcoded secure PATH (`/sbin:/usr/sbin:/bin:/usr/bin`), completely eliminating PATH manipulation attacks
- **Risk-Based Command Control**: Intelligent security assessment that automatically blocks high-risk operations
- **Environment Variable Isolation**: Strict allowlist-based filtering with zero-trust approach
- **Hybrid Hash Encoding**: Space-efficient file integrity verification with automatic fallback
- **Sensitive Data Protection**: Automatic detection and redaction of passwords, tokens, and API keys

### Command Execution Security
- **User/Group Execution Control**: Secure user and group switching with comprehensive validation
- **Privilege Management**: Controlled privilege escalation with automatic privilege dropping
- **Path Validation**: Command path resolution with symlink attack prevention
- **Output Capture Security**: Secure file permissions (0600) for output files
- **Timeout Control**: Prevention of resource exhaustion attacks

### Auditing and Monitoring
- **ULID Execution Tracking**: Time-ordered execution tracking with unique identifiers
- **Multi-handler Logging**: Console, file, and Slack integration with sensitive data redaction
- **Interactive Terminal Support**: Color-coded output with smart terminal detection
- **Comprehensive Audit Trail**: Complete logging of privileged operations and security events

## Core Features

### File Integrity and Verification
- **SHA-256 Hash Validation**: Verification of all executables and critical files before execution
- **Pre-execution Verification**: Validation of configuration and environment files before use
- **Hybrid Hash Encoding**: Space-efficient encoding with human-readable fallback
- **Centralized Verification**: Unified verification management with automatic privilege handling
- **Group and Global Verification**: Flexible file verification at multiple levels

### Command Execution
- **Batch Processing**: Command execution in organized groups with dependency management
- **Automatic Temporary Directories**: Auto-generation and cleanup of temporary directories per group
- **Working Directory Control**: Execute in fixed directories or auto-generated temporary directories
- **`__runner_workdir` Variable**: Reserved variable that references the runtime working directory
- **Variable Expansion**: `%{var}` format expansion in command names and arguments
- **Automatic Environment Variables**: Automatically generated variables for timestamps and process tracking
- **Output Capture**: Save command output to files with secure permissions
- **Background Execution**: Support for long-running processes with signal handling
- **Extended Dry Run**: Realistic simulation with comprehensive security analysis
  - Separate output streams: stdout for dry-run results, stderr for execution logs
  - `--dry-run-format=json`: JSON output for machine processing with debug information
  - `--dry-run-detail=full`: Display final environment variables with their origins and inheritance analysis
  - `--show-sensitive`: Show sensitive information in plain text (for debugging, use with caution)
- **Timeout Control**: Configurable timeouts for command execution
- **User/Group Context**: Execute commands as specific users with validation

### Logging and Monitoring
- **Multi-handler Logging**: Route logs to multiple destinations (console, file, Slack)
- **Interactive Terminal Support**: Color-coded output with enhanced visibility
- **Smart Terminal Detection**: Automatic detection of terminal capabilities
- **Color Control**: Support for CLICOLOR, NO_COLOR, CLICOLOR_FORCE environment variables
- **Slack Integration**: Real-time notifications for security events
- **Sensitive Data Redaction**: Automatic filtering of sensitive information
- **ULID Execution Tracking**: Time-ordered execution tracking

### File Operations
- **Safe File I/O**: Symlink-aware file operations with security checks
- **Hash Recording**: Record SHA-256 hashes for integrity verification
- **Verification Tools**: Standalone utilities for file verification

## Architecture

The system follows a modular architecture with clear separation of concerns:

```
cmd/                    # Command-line entry points
├── runner/            # Main command runner application
├── record/            # Hash recording utility
└── verify/            # File verification utility

internal/              # Core implementation
├── cmdcommon/         # Shared command utilities
├── color/             # Terminal color support
├── common/            # Common utilities and filesystem abstraction
├── filevalidator/     # File integrity validation
│   └── encoding/      # Hybrid hash filename encoding
├── groupmembership/   # User/group membership validation
├── logging/           # Advanced logging with Slack integration
├── redaction/         # Automatic sensitive data filtering
├── runner/            # Command execution engine
│   ├── audit/         # Security audit logging
│   ├── bootstrap/     # System initialization
│   ├── cli/           # Command-line interface
│   ├── config/        # Configuration management
│   ├── debug/         # Debug functionality and utilities
│   ├── environment/   # Environment variable processing
│   ├── errors/        # Centralized error handling
│   ├── executor/      # Command execution logic
│   ├── output/        # Output capture management
│   ├── privilege/     # Privilege management
│   ├── resource/      # Resource management (normal/dry-run)
│   ├── risk/          # Risk-based command assessment
│   ├── runnertypes/   # Type definitions and interfaces
│   ├── security/      # Security validation framework
│   └── variable/      # Automatic variable generation and definitions
├── safefileio/        # Secure file operations
├── terminal/          # Terminal capability detection
└── verification/      # Centralized verification management
```

## Quick Start

### Basic Usage

```bash
# Execute commands from configuration file
./runner -config config.toml

# Dry run mode (preview without execution)
./runner -config config.toml -dry-run

# Validate configuration file
./runner -config config.toml -validate
```

For detailed usage instructions, see the [runner command guide](docs/user/runner_command.md).

### Simple Configuration Example

```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowed = ["PATH", "HOME", "USER"]

[[groups]]
name = "backup"
description = "System backup operations"

[[groups.commands]]
name = "database_backup"
description = "Backup database"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases"]
output_file = "backup.sql"  # Save output to file
run_as_user = "mysql"
risk_level = "medium"
```

## Configuration

TOML-format configuration files define how commands are executed. Configuration files have the following hierarchical structure:

- **Root Level**: Version information
- **Global Level**: Default settings applied to all groups
- **Group Level**: Grouping of related commands
- **Command Level**: Individual command configuration

### Basic Configuration Example

```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowed = ["PATH", "HOME", "USER", "LANG"]

[[groups]]
name = "backup"
description = "Backup operations"
# No workdir specified - automatic temporary directory will be created

[[groups.commands]]
name = "database_backup"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases", "--result-file=%{__runner_workdir}/db.sql"]
risk_level = "medium"

[[groups]]
name = "maintenance"
description = "System maintenance tasks"
workdir = "/tmp/maintenance"  # Fixed working directory

[[groups.commands]]
name = "system_check"
cmd = "/usr/bin/systemctl"
args = ["status"]
risk_level = "medium"
```

### Detailed Configuration Guide

### Automatic Variables

The system automatically provides the following internal variables:

- `__runner_datetime`: Runner start timestamp in `YYYYMMDDHHmmSS.msec` format (UTC)
- `__runner_pid`: Process ID of the runner
- `__runner_workdir`: Working directory for the group (available at command level)

These variables can be referenced using `%{var}` syntax in command paths, arguments, and environment variable values:

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/usr/bin/tar"
args = ["czf", "/tmp/backup/data-%{__runner_datetime}.tar.gz", "/data"]

[[groups.commands]]
name = "log_execution"
cmd = "/bin/sh"
args = ["-c", "echo 'PID: %{__runner_pid}, Time: %{__runner_datetime}' >> /var/log/executions.log"]
```

**Note**: The prefix `__runner_` is reserved and cannot be used for user-defined variables.

### Group-Level Command Allowlist

Groups can define additional allowed commands beyond the hardcoded global patterns:

**Hardcoded global patterns** (not configurable from TOML):
```
^/bin/.*
^/usr/bin/.*
^/usr/sbin/.*
^/usr/local/bin/.*
```

```toml
[global]
env_import = ["home=HOME"]

[[groups]]
name = "custom_build"
# Additional commands allowed only in this group
cmd_allowed = [
    "%{home}/bin/custom_tool",
    "/opt/myapp/bin/processor"
]

[[groups.commands]]
name = "run_custom"
cmd = "%{home}/bin/custom_tool"
args = ["--verbose"]
```

**Key features**:
- Commands pass if they match EITHER hardcoded global patterns OR group-level `cmd_allowed` list
- Variable expansion (`%{variable}`) is supported in `cmd_allowed` paths
- Only absolute paths are allowed (relative paths are rejected for security)
- All other security checks (permissions, risk assessment) remain active
- Global patterns are hardcoded for security (cannot be configured from TOML)

See `sample/group_cmd_allowed.toml` for complete examples.

For detailed configuration file documentation, refer to the following documents:

- [TOML Configuration File User Guide](docs/user/toml_config/README.md) - Comprehensive configuration guide
  - [Configuration File Hierarchy](docs/user/toml_config/02_hierarchy.md)
  - [Global Level Settings](docs/user/toml_config/04_global_level.md)
  - [Group Level Settings](docs/user/toml_config/05_group_level.md)
  - [Command Level Settings](docs/user/toml_config/06_command_level.md)
  - [Variable Expansion](docs/user/toml_config/07_variable_expansion.md)
  - [Practical Configuration Examples](docs/user/toml_config/08_practical_examples.md)

## Security Model

### File Integrity Verification

1. **Pre-execution Verification**
   - Validate configuration files before loading
   - Verify environment files before use
   - Prevent malicious configuration attacks

2. **Hash Directory Security**
   - Fixed default: `/usr/local/etc/go-safe-cmd-runner/hashes`
   - No custom directories in production builds
   - Test APIs separated by build tags

3. **Hybrid Hash Encoding**
   - Space-efficient encoding (1.00x expansion)
   - Human-readable for debugging
   - Automatic SHA256 fallback

4. **Verification Management**
   - Centralized verification
   - Automatic privilege handling
   - Group and global verification lists

### Risk-Based Security Controls

- **Automatic Risk Assessment**: Command classification by risk level
- **Configurable Thresholds**: Risk level limits per command
- **Automatic Blocking**: Automatic blocking of high-risk commands
- **Risk Categories**:
  - **Low**: Basic operations (ls, cat, grep)
  - **Medium**: File modifications (cp, mv), package management
  - **High**: System administration (systemctl), destructive operations
  - **Critical**: Privilege escalation (sudo, su) - always blocked

### Environment Isolation

- **Secure Fixed PATH**: Hardcoded `/sbin:/usr/sbin:/bin:/usr/bin`
- **No PATH Inheritance**: Eliminates PATH manipulation attacks
- **Allowlist Filtering**: Strict zero-trust environment control
- **Variable Expansion**: Secure `%{var}` expansion with allowlist
- **Command.Env Priority**: Configuration overrides OS environment

### Privilege Management

- **Automatic Dropping**: Privilege dropping after initialization
- **Controlled Escalation**: Risk-aware privilege management
- **User/Group Switching**: Secure context switching with validation
- **Audit Trail**: Complete logging of privilege changes

### Output Capture Security

- **Secure Permissions**: Output files created with 0600 permissions
- **Privilege Separation**: Output files use real UID (not run_as_user)
- **Directory Security**: Automatic directory creation with secure permissions
- **Path Validation**: Prevention of path traversal attacks

### Log Security

- **Sensitive Data Redaction**: Automatic detection of secrets, tokens, API keys
- **Multi-channel Notifications**: Encrypted Slack communication
- **Audit Trail Protection**: Tamper-resistant structured logs
- **Real-time Alerts**: Immediate notification of security violations

## Command-Line Tools

go-safe-cmd-runner provides three command-line tools:

### runner - Main Execution Command

```bash
# Basic execution (long form)
./runner --config config.toml

# Basic execution (short form)
./runner -c config.toml

# Dry run (preview execution)
./runner -c config.toml --dry-run
./runner -c config.toml -n          # Short form

# Validate configuration
./runner -c config.toml --validate
./runner -c config.toml -V           # Short form

# Group filtering
./runner -c config.toml --groups=build,test
./runner -c config.toml -g build,test  # Short form

# Quiet mode
./runner -c config.toml --quiet
./runner -c config.toml -q           # Short form

# Set log level
./runner -c config.toml --log-level=debug
./runner -c config.toml -l debug     # Short form
```

#### Short Flags

The runner command supports the following short flags for commonly used options:

| Long Form | Short Form | Description |
|-----------|------------|-------------|
| `--config` | `-c` | Configuration file path |
| `--dry-run` | `-n` | Dry run mode (preview execution) |
| `--groups` | `-g` | Filter groups to execute |
| `--log-level` | `-l` | Set logging level |
| `--quiet` | `-q` | Quiet mode (disable colored output) |
| `--validate` | `-V` | Validate configuration and exit |

For details, see the [runner command guide](docs/user/runner_command.md).

### Group Filtering

Run only the groups you need by passing the `--groups` flag with a comma-separated list.

```bash
# Single group
./runner -config config.toml --groups=build

# Multiple groups
./runner -config config.toml --groups=build,test

# Default (all groups)
./runner -config config.toml
```

When a selected group declares dependencies via `depends_on`, those prerequisite groups are automatically appended and executed first.

```toml
[[groups]]
name = "build"
depends_on = ["preparation"]

[[groups]]
name = "test"
depends_on = ["build"]
```

```bash
./runner -config config.toml --groups=test
# Execution order: preparation -> build -> test
```

Group names follow the same constraints as environment variable identifiers: they must match the pattern `[A-Za-z_][A-Za-z0-9_]*` (letters or underscore first, followed by alphanumerics or underscores).

### record - Hash Recording Command

```bash
# Record hash for a single file
./record --hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /path/to/executable

# Record hash using short form
./record -d /usr/local/etc/go-safe-cmd-runner/hashes /path/to/executable

# Record hashes for multiple files at once
./record -d /usr/local/etc/go-safe-cmd-runner/hashes file1.dat file2.txt file3.sh

# Force overwrite existing hash
./record -d /usr/local/etc/go-safe-cmd-runner/hashes -force /path/to/file
```

#### Multiple File Processing

The record command can process multiple files in a single invocation:

```bash
# Process multiple files
./record -d /tmp/hash file1.dat file2.txt file3.sh

# Output example:
# Processing 3 files...
# [1/3] file1.dat: OK
# [2/3] file2.txt: OK
# [3/3] file3.sh: OK
#
# Summary: 3 succeeded, 0 failed
```

- Files are specified as positional arguments after flags
- Progress is shown as `[current/total]` for each file
- Final summary reports success and failure counts
- Non-zero exit code if any file fails

#### Short Flags

| Long Form | Short Form | Description |
|-----------|------------|-------------|
| `--hash-dir` | `-d` | Hash directory path |

For details, see the [record command guide](docs/user/record_command.md).

### verify - File Verification Command

```bash
# Verify single file integrity
./verify --hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /path/to/file

# Verify using short form
./verify -d /usr/local/etc/go-safe-cmd-runner/hashes /path/to/file

# Verify multiple files at once
./verify -d /usr/local/etc/go-safe-cmd-runner/hashes file1.dat file2.txt file3.sh
```

#### Multiple File Processing

The verify command can verify multiple files in a single invocation:

```bash
# Verify multiple files
./verify -d /tmp/hash file1.dat file2.txt file3.sh

# Output example:
# Verifying 3 files...
# [1/3] file1.dat: OK
# [2/3] file2.txt: OK
# [3/3] file3.sh: OK
#
# Summary: 3 succeeded, 0 failed
```

- Files are specified as positional arguments after flags
- Progress is shown as `[current/total]` for each file
- Final summary reports success and failure counts
- Non-zero exit code if any file fails

#### Short Flags

| Long Form | Short Form | Description |
|-----------|------------|-------------|
| `--hash-dir` | `-d` | Hash directory path |

For details, see the [verify command guide](docs/user/verify_command.md).

### Comprehensive User Guide

For detailed usage instructions, configuration examples, and troubleshooting, refer to the [User Guide](docs/user/README.md).

## Build and Installation

### Prerequisites

- Go 1.23 or later (required for slices package and range over count)
- golangci-lint (for development)
- gofumpt (for code formatting)

### Build Commands

```bash
# Build all binaries
make build

# Build specific binaries
make build/runner
make build/record
make build/verify

# Run tests
make test

# Run linter
make lint

# Format code
make fmt

# Clean build artifacts
make clean

# Run benchmarks
make benchmark

# Generate coverage report
make coverage
```

### Installation

```bash
# Install from source
git clone https://github.com/isseis/go-safe-cmd-runner.git
cd go-safe-cmd-runner
make build

# Install binaries to system location
sudo install -o root -g root -m 4755 build/runner /usr/local/bin/go-safe-cmd-runner
sudo install -o root -g root -m 0755 build/record /usr/local/bin/go-safe-cmd-record
sudo install -o root -g root -m 0755 build/verify /usr/local/bin/go-safe-cmd-verify

# Create default hash directory
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

# Run tests for specific package
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
- **Time-ordered sortable**: Naturally sorted by creation time
- **URL-safe**: No special characters, suitable for filenames
- **Compact**: Fixed 26-character length
- **Collision-resistant**: Monotonic entropy ensures uniqueness
- **Example**: `01K2YK812JA735M4TWZ6BK0JH9`

## Out of Scope

This project explicitly does **not** provide:
- Container orchestration or Docker integration
- Network security features (firewalls, VPNs, etc.)
- User authentication or authorization systems
- Web interfaces or REST APIs
- Database management capabilities
- Real-time monitoring or alerting systems
- Cross-platform GUI applications
- Package management or software installation

It focuses on secure command execution with comprehensive security controls in Unix-like environments.

## Contributing

This project emphasizes security and reliability. When contributing:
- Follow security-first design principles
- Add comprehensive tests for new features
- Update documentation for configuration changes
- Ensure all security validations are tested
- Use static analysis tools (golangci-lint)
- Follow Go coding standards and best practices

For questions or contributions, refer to the project's issue tracker.

## License

This project is released under the MIT License. For details, see the [LICENSE](./LICENSE) file.
