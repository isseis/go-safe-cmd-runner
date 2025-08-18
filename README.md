# Go Safe Command Runner

A secure command execution framework for Go designed for privileged task delegation and automated batch processing with comprehensive security controls.

Project page: https://github.com/isseis/go-safe-cmd-runner/

## Background

Go Safe Command Runner addresses the critical need for secure command execution in environments where:
- Regular users need to execute privileged operations safely
- Automated systems require secure batch processing capabilities
- File integrity verification is essential before command execution
- Environment variable exposure needs strict control
- Command execution requires audit trails and security boundaries

Common use cases include scheduled backups, system maintenance tasks, and delegating specific administrative operations to non-root users while maintaining security controls.

## Features

### Core Security Features
- **File Integrity Verification**: SHA-256 hash validation of executables and configuration files before execution
- **Environment Variable Isolation**: Allowlist-based environment variable filtering at global and group levels
- **Privilege Management**: Controlled privilege escalation and automatic privilege dropping
- **Path Validation**: Command path resolution with symlink attack prevention
- **Configuration Validation**: Comprehensive TOML configuration file validation

### Command Execution
- **Batch Processing**: Execute commands in organized groups with dependency management
- **Background Execution**: Support for long-running processes with proper signal handling
- **Output Capture**: Structured logging and output management
- **Dry Run Mode**: Preview command execution without actual execution
- **Timeout Control**: Configurable timeouts for command execution

### Logging and Monitoring
- **Multi-Handler Logging**: Route logs to multiple destinations simultaneously (console, file, Slack)
- **Slack Integration**: Real-time notifications for security events and failures
- **Audit Logging**: Comprehensive audit trail for privileged operations and security events
- **Sensitive Data Redaction**: Automatic filtering of sensitive information from logs
- **Structured Logging**: JSON-formatted logs with rich contextual information
- **ULID Run Tracking**: Universally Unique Lexicographically Sortable Identifiers for time-ordered execution tracking

### File Operations
- **Safe File I/O**: Symlink-aware file operations with security checks
- **Hash Recording**: Record SHA-256 hashes of critical files for later verification
- **Verification Tools**: Standalone utilities for file integrity verification

## Architecture

The system follows a modular architecture with clear separation of concerns:

```
cmd/                    # Command-line entry points
├── runner/            # Main command runner application
├── record/            # Hash recording utility
└── verify/            # File verification utility

internal/              # Core implementation
├── cmdcommon/         # Shared command utilities
├── filevalidator/     # File integrity validation
├── logging/           # Advanced logging system
│   ├── multihandler/  # Multi-destination log handling
│   ├── slack/         # Slack notification integration
│   └── redaction/     # Sensitive data redaction
├── runner/            # Command execution engine
│   ├── audit/         # Security audit logging
│   ├── config/        # Configuration management
│   ├── executor/      # Command execution logic
│   └── privilege/     # Privilege management
├── safefileio/        # Secure file operations
└── verification/      # Hash verification system
```

## Command Line Tools

### Main Runner
```bash
# Execute commands from configuration file
./runner -config config.toml

# Dry run mode (preview without execution)
./runner -config config.toml -dry-run

# Validate configuration file
./runner -config config.toml -validate

# Use custom environment file
./runner -config config.toml -env-file .env.production

# Custom hash directory
./runner -config config.toml -hash-directory /custom/hash/dir

# Custom log directory and level
./runner -config config.toml -log-dir /var/log/go-safe-cmd-runner -log-level debug

# Execute with Slack notifications (requires SLACK_WEBHOOK_URL in environment file)
./runner -config config.toml -env-file .env
```

### Hash Management
```bash
# Record file hash
./record -file /path/to/executable -hash-dir /etc/hashes

# Force overwrite existing hash
./record -file /path/to/file -force

# Verify file integrity
./verify -file /path/to/file -hash-dir /etc/hashes
```

## Configuration

### Basic Configuration Example
```toml
version = "1.0"

[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"
# Environment variable allowlist for security
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG"
]
# Files to verify before execution
verify_files = ["/etc/passwd", "/bin/bash"]

[[groups]]
name = "backup"
description = "System backup operations"
priority = 1
# Group-specific environment variables (overrides global)
env_allowlist = ["PATH", "HOME", "BACKUP_DIR"]

[[groups.commands]]
name = "database_backup"
description = "Backup database"
cmd = "mysqldump"
args = ["--all-databases", "--single-transaction"]
env = ["BACKUP_DIR=/backups"]
privileged = false

[[groups.commands]]
name = "system_backup"
description = "Backup system files"
cmd = "rsync"
args = ["-av", "/etc/", "/backups/etc/"]
privileged = true
```

### Advanced Configuration Features
```toml
[global]
# Skip verification of standard system paths
skip_standard_paths = true
# Global file verification list
verify_files = ["/usr/bin/rsync", "/etc/rsync.conf"]

[[groups]]
name = "web_deployment"
description = "Web application deployment"
priority = 2
# Strict environment control (empty list = no environment variables)
env_allowlist = []
# Group-specific file verification
verify_files = ["/usr/local/bin/deploy.sh"]

[[groups.commands]]
name = "deploy_app"
cmd = "/usr/local/bin/deploy.sh"
args = ["production"]
# No environment variables available due to empty env_allowlist
```

### Environment Variable Security
The system implements a strict allowlist-based approach for environment variables:

1. **Global Allowlist**: Defines base environment variables available to all groups
2. **Group Override**: Groups can define their own allowlist, completely overriding global settings
3. **Inheritance**: Groups without an explicit allowlist inherit from global settings
4. **Zero Trust**: Undefined allowlists result in no environment variables being passed

### Environment File Configuration
Create a `.env` file for sensitive configuration that shouldn't be stored in the main TOML configuration:

```bash
# .env file for production environment
# Slack webhook URL for notifications
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK

# Optional: Override default log settings
LOG_LEVEL=info
LOG_DIR=/var/log/go-safe-cmd-runner

# Application-specific variables
DATABASE_URL=postgresql://localhost:5432/myapp
API_KEY=your-secret-api-key
```

**Security Note**: The `.env` file undergoes strict security validation:
- File permissions are checked (should be readable only by the owner)
- Path traversal attacks are prevented
- Content is parsed securely using safe file I/O operations

## Security Model

### File Integrity Verification
- All executables and critical files are verified against pre-recorded SHA-256 hashes
- Configuration files are automatically verified before execution
- Group-specific and global file verification lists
- Execution is aborted if any verification fails

### Privilege Management
- Automatic privilege dropping after initialization
- Controlled privilege escalation for specific commands
- Minimal privilege principle enforcement
- Comprehensive audit logging

### Environment Isolation
- Strict allowlist-based environment variable filtering
- Protection against environment variable injection attacks
- Group-level and global environment control
- Secure variable reference resolution

### Logging Security
- **Sensitive Data Redaction**: Automatic detection and redaction of secrets, tokens, and sensitive patterns
- **Secure Notifications**: Encrypted Slack webhook communications for security alerts
- **Audit Trail Protection**: Tamper-resistant logging with structured format
- **Access Control**: Log file permissions and secure storage practices

## Out of Scope

This project explicitly does **not** provide:
- **Container orchestration** or Docker integration
- **Network security** features (firewall, VPN, etc.)
- **User authentication** or authorization systems
- **Web interface** or REST API
- **Database management** capabilities
- **Real-time monitoring** or alerting systems
- **Cross-platform GUI** applications
- **Package management** or software installation

The focus remains on secure command execution with file integrity verification in Unix-like environments.

## License
This project is licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.

## Building and Installation

### Prerequisites
- Go 1.21 or later (required for slices package support)
- golangci-lint (for development)

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

# Clean build artifacts
make clean
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
```

## Development

### Dependencies
- `github.com/pelletier/go-toml/v2` - TOML configuration parsing
- `github.com/joho/godotenv` - Environment file loading
- `github.com/oklog/ulid/v2` - ULID generation for run tracking and identification
- `github.com/stretchr/testify` - Testing framework

### Testing
```bash
# Run all tests
go test -v ./...

# Run tests for specific package
go test -v ./internal/runner

# Run integration tests
make integration-test
```

### Project Structure
The codebase follows Go best practices with:
- Interface-driven design for testability
- Comprehensive error handling with custom error types
- Security-first approach with extensive validation
- Modular architecture with clear boundaries

### Run Identification with ULID
The system uses ULID (Universally Unique Lexicographically Sortable Identifier) for run tracking:
- **Chronologically sortable**: ULIDs are naturally ordered by creation time
- **URL-safe**: No special characters, making them suitable for filenames and URLs
- **Compact**: 26-character fixed length (shorter than UUID's 36 characters)
- **Collision-resistant**: Monotonic entropy ensures uniqueness even within the same millisecond
- **Example**: `01K2YK812JA735M4TWZ6BK0JH9`

## Contributing

This project emphasizes security and reliability. When contributing:
- Follow the security-first design principles
- Add comprehensive tests for new features
- Update documentation for any configuration changes
- Ensure all security validations are properly tested

For questions or contributions, please refer to the project's issue tracker or contact the maintainers.
