# Go Safe Command Runner

A secure command execution framework for Go designed for privileged task delegation and automated batch processing with comprehensive security controls.

Project page: https://github.com/isseis/go-safe-cmd-runner/

## Table of Contents

- [Background](#background)
- [Key Security Features](#key-security-features)
- [Recent Security Enhancements](#recent-security-enhancements)
- [Core Features](#core-features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Security Model](#security-model)
- [Command Line Tools](#command-line-tools)
- [Building and Installation](#building-and-installation)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Background

Go Safe Command Runner addresses the critical need for secure command execution in environments where:
- Regular users need to execute privileged operations safely
- Automated systems require secure batch processing capabilities
- File integrity verification is essential before command execution
- Environment variable exposure needs strict control
- Command execution requires audit trails and security boundaries

Common use cases include scheduled backups, system maintenance tasks, and delegating specific administrative operations to non-root users while maintaining security controls.

## Key Security Features

### Multi-Layer Defense Architecture
- **Pre-Execution Verification**: Configuration and environment files are hash-verified before use, preventing malicious configuration attacks
- **Fixed Hash Directory**: Production builds use only the default hash directory, eliminating custom hash directory attack vectors
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
- **Timeout Control**: Prevents resource exhaustion attacks

### Audit and Monitoring
- **ULID Run Tracking**: Time-ordered execution tracking with unique identifiers
- **Multi-Handler Logging**: Console, file, and Slack integration with sensitive data redaction
- **Interactive Terminal Support**: Color-coded output with smart terminal detection
- **Comprehensive Audit Trail**: Complete logging of privileged operations and security events

## Recent Security Enhancements

### ⚠️ Breaking Changes (Critical Security Improvements)

Recent versions introduce critical security improvements with breaking changes:

#### Removed Features (Security)
- **`--hash-directory` flag**: Completely removed from the runner to prevent custom hash directory attacks
- **Custom hash directory API**: Internal APIs no longer accept custom hash directories in production builds
- **Hash directory configuration**: Configuration file hash directory specification is no longer supported
- **PATH environment inheritance**: Environment variable PATH is no longer inherited from the parent process

#### Enhanced Security Features
1. **Pre-Execution Verification** (Task 0021)
   - Configuration files verified before reading
   - Environment files verified before use
   - Prevents malicious configuration attacks
   - Force stderr output for verification failures

2. **Hash Directory Security** (Task 0022)
   - Fixed default hash directory: `/usr/local/etc/go-safe-cmd-runner/hashes`
   - Production/test API separation with build tags
   - Static analysis detection of security violations
   - Complete prevention of custom hash directory attacks

3. **Hybrid Hash Encoding** (Task 0023)
   - Space-efficient substitution + double escape encoding
   - 1.00x expansion ratio for common paths
   - Automatic SHA256 fallback for long paths
   - Human-readable hash file names for debugging

4. **Output Capture Security** (Task 0025)
   - Secure file permissions (0600) for output files
   - Tee functionality (screen + file output)
   - Privilege separation (output files use real UID)
   - Automatic directory creation with secure permissions

5. **Variable Expansion** (Task 0026)
   - `${VAR}` format variable expansion in cmd and args
   - Circular reference detection with visited map
   - Allowlist integration for security
   - Command.Env priority over OS environment

#### Migration Guide
- **Configuration**: Remove any `hash_directory` settings from TOML files
- **Scripts**: Remove `--hash-directory` flag from scripts or automation
- **Development**: Use test APIs with `//go:build test` tag for testing
- **PATH Dependencies**: Ensure all required binaries are in standard system paths
- **Environment Variables**: Review and update environment variable allowlists

For detailed migration information, see [Verification API Documentation](docs/verification_api.md).

## Core Features

### File Integrity and Verification
- **SHA-256 Hash Validation**: All executables and critical files verified before execution
- **Pre-Execution Verification**: Configuration and environment files verified before use
- **Hybrid Hash Encoding**: Space-efficient encoding with human-readable fallback
- **Centralized Verification**: Unified verification management with automatic privilege handling
- **Group and Global Verification**: Flexible file verification at multiple levels

### Command Execution
- **Batch Processing**: Execute commands in organized groups with dependency management
- **Variable Expansion**: `${VAR}` format expansion in command names and arguments
- **Output Capture**: Save command output to files with secure permissions
- **Background Execution**: Support for long-running processes with signal handling
- **Enhanced Dry Run**: Realistic simulation with comprehensive security analysis
- **Timeout Control**: Configurable timeouts for command execution
- **User/Group Context**: Execute commands as specific users with validation

### Logging and Monitoring
- **Multi-Handler Logging**: Route logs to multiple destinations (console, file, Slack)
- **Interactive Terminal Support**: Color-coded output with enhanced visibility
- **Smart Terminal Detection**: Automatic detection of terminal capabilities
- **Color Control**: Support for CLICOLOR, NO_COLOR, and CLICOLOR_FORCE environment variables
- **Slack Integration**: Real-time notifications for security events
- **Sensitive Data Redaction**: Automatic filtering of sensitive information
- **ULID Run Tracking**: Time-ordered execution tracking

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
│   ├── environment/   # Environment variable processing
│   ├── errors/        # Centralized error handling
│   ├── executor/      # Command execution logic
│   ├── hashdir/       # Hash directory security
│   ├── output/        # Output capture management
│   ├── privilege/     # Privilege management
│   ├── resource/      # Resource management (normal/dry-run)
│   ├── risk/          # Risk-based command assessment
│   ├── runnertypes/   # Type definitions and interfaces
│   └── security/      # Security validation framework
├── safefileio/        # Secure file operations
├── terminal/          # Terminal capabilities detection
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
description = "Backup database"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases"]
output = "backup.sql"  # Save output to file
run_as_user = "mysql"
max_risk_level = "medium"
```

## Configuration

### Basic Configuration Structure

```toml
version = "1.0"

[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"
skip_standard_paths = true  # Skip verification for system paths
env_allowlist = ["PATH", "HOME", "USER", "LANG"]
verify_files = ["/etc/passwd", "/bin/bash"]

[[groups]]
name = "maintenance"
description = "System maintenance tasks"
priority = 1
env_allowlist = ["PATH", "HOME"]  # Override global allowlist

[[groups.commands]]
name = "system_check"
cmd = "/usr/bin/systemctl"
args = ["status"]
max_risk_level = "medium"
```

### Variable Expansion

Use `${VAR}` format for dynamic configuration:

```toml
[[groups.commands]]
name = "deploy"
cmd = "${TOOL_DIR}/deploy.sh"
args = ["--config", "${CONFIG_FILE}"]
env = ["TOOL_DIR=/opt/tools", "CONFIG_FILE=/etc/app.conf"]
```

### Output Capture

Save command output to files:

```toml
[[groups.commands]]
name = "generate_report"
cmd = "/usr/bin/df"
args = ["-h"]
output = "reports/disk_usage.txt"  # Tee output to file (0600 permissions)
```

### Risk-Based Control

Configure security risk thresholds:

```toml
[[groups.commands]]
name = "file_operation"
cmd = "/bin/cp"
args = ["source.txt", "dest.txt"]
max_risk_level = "low"  # Only allow low-risk commands

[[groups.commands]]
name = "system_admin"
cmd = "/usr/bin/systemctl"
args = ["restart", "nginx"]
max_risk_level = "high"  # Allow high-risk operations
```

### User and Group Execution

Execute commands with specific user/group context:

```toml
[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
run_as_user = "postgres"
run_as_group = "postgres"
output = "/backups/db.sql"
```

### Environment Variable Security

Strict allowlist-based control:

```toml
[global]
# Global allowlist (default for all groups)
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "secure_group"
# Override with empty list = no environment variables
env_allowlist = []

[[groups]]
name = "web_group"
# Override with custom list
env_allowlist = ["PATH", "HOME", "WEB_ROOT"]
```

## Security Model

### File Integrity Verification

1. **Pre-Execution Verification**
   - Configuration files verified before reading
   - Environment files verified before use
   - Prevents malicious configuration attacks

2. **Hash Directory Security**
   - Fixed default: `/usr/local/etc/go-safe-cmd-runner/hashes`
   - No custom directories in production
   - Test API separated with build tags

3. **Hybrid Hash Encoding**
   - Space-efficient encoding (1.00x expansion)
   - Human-readable for debugging
   - Automatic SHA256 fallback

4. **Verification Management**
   - Centralized verification
   - Automatic privilege handling
   - Group and global verification lists

### Risk-Based Security Control

- **Automatic Risk Assessment**: Commands classified by risk level
- **Configurable Thresholds**: Per-command risk level limits
- **Automatic Blocking**: High-risk commands blocked automatically
- **Risk Categories**:
  - **Low**: Basic operations (ls, cat, grep)
  - **Medium**: File modifications (cp, mv), package management
  - **High**: System administration (systemctl), destructive operations
  - **Critical**: Privilege escalation (sudo, su) - always blocked

### Environment Isolation

- **Secure Fixed PATH**: Hardcoded `/sbin:/usr/sbin:/bin:/usr/bin`
- **No PATH Inheritance**: Eliminates PATH manipulation attacks
- **Allowlist Filtering**: Strict zero-trust environment control
- **Variable Expansion**: Secure `${VAR}` expansion with allowlist
- **Command.Env Priority**: Configuration overrides OS environment

### Privilege Management

- **Automatic Dropping**: Privileges dropped after initialization
- **Controlled Escalation**: Risk-aware privilege management
- **User/Group Switching**: Secure context switching with validation
- **Audit Trail**: Complete logging of privilege changes

### Output Capture Security

- **Secure Permissions**: Output files created with 0600 permissions
- **Privilege Separation**: Output files use real UID (not run_as_user)
- **Directory Security**: Automatic directory creation with secure permissions
- **Path Validation**: Prevention of path traversal attacks

### Logging Security

- **Sensitive Data Redaction**: Automatic detection of secrets, tokens, API keys
- **Multi-Channel Notifications**: Encrypted Slack communications
- **Audit Trail Protection**: Tamper-resistant structured logging
- **Real-Time Alerts**: Immediate notification of security violations

## Command Line Tools

### Main Runner

```bash
# Basic execution
./runner -config config.toml

# Dry run with security analysis
./runner -config config.toml -dry-run

# Validate configuration
./runner -config config.toml -validate

# Custom log settings
./runner -config config.toml -log-dir /var/log/runner -log-level debug

# Color control
CLICOLOR=1 ./runner -config config.toml       # Enable color
NO_COLOR=1 ./runner -config config.toml       # Disable color
CLICOLOR_FORCE=1 ./runner -config config.toml # Force color

# Slack notifications (requires GSCR_SLACK_WEBHOOK_URL)
GSCR_SLACK_WEBHOOK_URL=https://hooks.slack.com/... ./runner -config config.toml
```

### Hash Management

```bash
# Record file hash (uses default hash directory)
./record -file /path/to/executable

# Force overwrite existing hash
./record -file /path/to/file -force

# Verify file integrity (uses default hash directory)
./verify -file /path/to/file

# Note: -hash-dir option available for testing only
./record -file /path/to/file -hash-dir /custom/test/hashes
```

## Building and Installation

### Prerequisites

- Go 1.23 or later (required for slices package, range over count)
- golangci-lint (for development)
- gofumpt (for code formatting)

### Build Commands

```bash
# Build all binaries
make build

# Build specific binary
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
- `github.com/oklog/ulid/v2` - ULID generation for run tracking
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

### Run Identification with ULID

The system uses ULID (Universally Unique Lexicographically Sortable Identifier):
- **Chronologically sortable**: Naturally ordered by creation time
- **URL-safe**: No special characters, suitable for filenames
- **Compact**: 26-character fixed length
- **Collision-resistant**: Monotonic entropy ensures uniqueness
- **Example**: `01K2YK812JA735M4TWZ6BK0JH9`

## Out of Scope

This project explicitly does **not** provide:
- Container orchestration or Docker integration
- Network security features (firewall, VPN, etc.)
- User authentication or authorization systems
- Web interface or REST API
- Database management capabilities
- Real-time monitoring or alerting systems
- Cross-platform GUI applications
- Package management or software installation

The focus remains on secure command execution with comprehensive security controls in Unix-like environments.

## Contributing

This project emphasizes security and reliability. When contributing:
- Follow security-first design principles
- Add comprehensive tests for new features
- Update documentation for configuration changes
- Ensure all security validations are tested
- Use static analysis tools (golangci-lint)
- Follow Go coding standards and best practices

For questions or contributions, please refer to the project's issue tracker.

## License

This project is licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.
