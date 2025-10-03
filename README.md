# Go Safe Command Runner# Go Safe Command Runner



A secure command execution framework for Go designed for privileged task delegation and automated batch processing with comprehensive security controls.A secure command execution framework for Go designed for privileged task delegation and automated batch processing with comprehensive security controls.



Project page: https://github.com/isseis/go-safe-cmd-runner/Project page: https://github.com/isseis/go-safe-cmd-runner/



## Table of Contents## Table of Contents



- [Background](#background)- [Background](#background)

- [Key Security Features](#key-security-features)- [Key Security Features](#key-security-features)

- [Recent Security Enhancements](#recent-security-enhancements)- [Recent Security Enhancements](#recent-security-enhancements)

- [Core Features](#core-features)- [Core Features](#core-features)

- [Architecture](#architecture)- [Architecture](#architecture)

- [Quick Start](#quick-start)- [Quick Start](#quick-start)

- [Configuration](#configuration)- [Configuration](#configuration)

- [Security Model](#security-model)- [Security Model](#security-model)

- [Command Line Tools](#command-line-tools)- [Command Line Tools](#command-line-tools)

- [Building and Installation](#building-and-installation)- [Building and Installation](#building-and-installation)

- [Development](#development)- [Development](#development)

- [Contributing](#contributing)- [Contributing](#contributing)

- [License](#license)- [License](#license)



## Background## Background



Go Safe Command Runner addresses the critical need for secure command execution in environments where:Go Safe Command Runner addresses the critical need for secure command execution in environments where:

- Regular users need to execute privileged operations safely- Regular users need to execute privileged operations safely

- Automated systems require secure batch processing capabilities- Automated systems require secure batch processing capabilities

- File integrity verification is essential before command execution- File integrity verification is essential before command execution

- Environment variable exposure needs strict control- Environment variable exposure needs strict control

- Command execution requires audit trails and security boundaries- Command execution requires audit trails and security boundaries



Common use cases include scheduled backups, system maintenance tasks, and delegating specific administrative operations to non-root users while maintaining security controls.Common use cases include scheduled backups, system maintenance tasks, and delegating specific administrative operations to non-root users while maintaining security controls.



## Key Security Features## Key Security Features



### Multi-layered Defense Architecture### Multi-Layer Defense Architecture

- **Pre-execution Validation**: Hash verification of config and environment files before use prevents malicious configuration attacks- **Pre-Execution Verification**: Configuration and environment files are hash-verified before use, preventing malicious configuration attacks

- **Fixed Hash Directory**: Production builds use default hash directory only, eliminating custom hash directory attack vectors- **Fixed Hash Directory**: Production builds use only the default hash directory, eliminating custom hash directory attack vectors

- **Secure Fixed PATH**: Uses hardcoded secure PATH (`/sbin:/usr/sbin:/bin:/usr/bin`), completely eliminating PATH manipulation attacks- **Secure Fixed PATH**: Uses hardcoded secure PATH (`/sbin:/usr/sbin:/bin:/usr/bin`), completely eliminating PATH manipulation attacks

- **Risk-based Command Control**: Intelligent security assessment that automatically blocks high-risk operations- **Risk-Based Command Control**: Intelligent security assessment that automatically blocks high-risk operations

- **Environment Variable Isolation**: Strict allowlist-based filtering with zero-trust approach- **Environment Variable Isolation**: Strict allowlist-based filtering with zero-trust approach

- **Hybrid Hash Encoding**: Space-efficient file integrity verification with automatic fallback capabilities- **Hybrid Hash Encoding**: Space-efficient file integrity verification with automatic fallback

- **Sensitive Data Protection**: Automatic detection and redaction of passwords, tokens, and API keys- **Sensitive Data Protection**: Automatic detection and redaction of passwords, tokens, and API keys



### Command Execution Security### Command Execution Security

- **User/Group Execution Control**: Secure user and group switching with comprehensive validation- **User/Group Execution Control**: Secure user and group switching with comprehensive validation

- **Privilege Management**: Controlled privilege escalation with automatic privilege dropping- **Privilege Management**: Controlled privilege escalation with automatic privilege dropping

- **Path Validation**: Command path resolution with symlink attack prevention- **Path Validation**: Command path resolution with symlink attack prevention

- **Output Capture Security**: Secure file permissions (0600) for output files- **Output Capture Security**: Secure file permissions (0600) for output files

- **Timeout Controls**: Configurable timeouts to prevent resource exhaustion attacks- **Timeout Control**: Prevents resource exhaustion attacks



### Auditing and Monitoring### Audit and Monitoring

- **ULID Execution Tracking**: Time-ordered execution tracking with unique identifiers- **ULID Run Tracking**: Time-ordered execution tracking with unique identifiers

- **Multi-handler Logging**: Log routing to multiple destinations (console, file, Slack) with sensitive data redaction- **Multi-Handler Logging**: Console, file, and Slack integration with sensitive data redaction

- **Interactive Terminal Support**: Color-coded output with smart terminal detection- **Interactive Terminal Support**: Color-coded output with smart terminal detection

- **Comprehensive Audit Trail**: Full logging of privileged operations and security events- **Comprehensive Audit Trail**: Complete logging of privileged operations and security events



## Recent Security Enhancements## Recent Security Enhancements



### ⚠️ Breaking Changes (Important Security Improvements)### ⚠️ Breaking Changes (Critical Security Improvements)



Recent versions have introduced significant security improvements with breaking changes:Recent versions introduce critical security improvements with breaking changes:



#### Removed Features (Security)#### Removed Features (Security)

- **`--hash-directory` flag**: Completely removed from runner to prevent custom hash directory attacks- **`--hash-directory` flag**: Completely removed from the runner to prevent custom hash directory attacks

- **Custom Hash Directory API**: Internal APIs do not accept custom hash directories in production builds- **Custom hash directory API**: Internal APIs no longer accept custom hash directories in production builds

- **Hash Directory Configuration**: Hash directory specification in config files is not supported- **Hash directory configuration**: Configuration file hash directory specification is no longer supported

- **PATH Environment Variable Inheritance**: Environment variable PATH is not inherited from parent process- **PATH environment inheritance**: Environment variable PATH is no longer inherited from the parent process



#### Enhanced Security Features#### Enhanced Security Features

1. **Pre-execution Validation** (Task 0021)1. **Pre-Execution Verification** (Task 0021)

   - Config file validation before loading   - Configuration files verified before reading

   - Environment file validation before use   - Environment files verified before use

   - Prevention of malicious configuration attacks   - Prevents malicious configuration attacks

   - Forced stderr output on validation failures   - Force stderr output for verification failures



2. **Hash Directory Security** (Task 0022)2. **Hash Directory Security** (Task 0022)

   - Fixed default hash directory: `/usr/local/etc/go-safe-cmd-runner/hashes`   - Fixed default hash directory: `/usr/local/etc/go-safe-cmd-runner/hashes`

   - Production/test API separation via build tags   - Production/test API separation with build tags

   - Static analysis detection of security violations   - Static analysis detection of security violations

   - Complete prevention of custom hash directory attacks   - Complete prevention of custom hash directory attacks



3. **Hybrid Hash Encoding** (Task 0023)3. **Hybrid Hash Encoding** (Task 0023)

   - Space-efficient replacement+double-escape encoding   - Space-efficient substitution + double escape encoding

   - 1.00x expansion ratio for common paths   - 1.00x expansion ratio for common paths

   - Automatic SHA256 fallback for long paths   - Automatic SHA256 fallback for long paths

   - Human-readable hash filenames for debugging   - Human-readable hash file names for debugging



4. **Output Capture Security** (Task 0025)4. **Output Capture Security** (Task 0025)

   - Secure file permissions (0600) for output files   - Secure file permissions (0600) for output files

   - Tee functionality (screen + file output)   - Tee functionality (screen + file output)

   - Privilege separation (output files use real UID)   - Privilege separation (output files use real UID)

   - Automatic directory creation with secure permissions   - Automatic directory creation with secure permissions



5. **Variable Expansion** (Task 0026)5. **Variable Expansion** (Task 0026)

   - `${VAR}` format variable expansion in cmd and args   - `${VAR}` format variable expansion in cmd and args

   - Circular reference detection with visited map   - Circular reference detection with visited map

   - Allowlist integration for security   - Allowlist integration for security

   - Command.Env takes precedence over OS environment   - Command.Env priority over OS environment



#### Migration Guide#### Migration Guide

- **Configuration**: Remove `hash_directory` setting from TOML files- **Configuration**: Remove any `hash_directory` settings from TOML files

- **Scripts**: Remove `--hash-directory` flags from scripts and automation- **Scripts**: Remove `--hash-directory` flag from scripts or automation

- **Development**: Use test APIs with `//go:build test` tags for testing- **Development**: Use test APIs with `//go:build test` tag for testing

- **PATH Dependencies**: Ensure all required binaries are in standard system paths (/sbin, /usr/sbin, /bin, /usr/bin)- **PATH Dependencies**: Ensure all required binaries are in standard system paths

- **Environment Variables**: Review and update environment variable allowlists- **Environment Variables**: Review and update environment variable allowlists



For detailed migration information, see the [Verification API Documentation](docs/verification_api.md).For detailed migration information, see [Verification API Documentation](docs/verification_api.md).



## Core Features## Core Features



### File Integrity and Verification### File Integrity and Verification

- **SHA-256 Hash Verification**: Pre-execution verification of all executables and critical files- **SHA-256 Hash Validation**: All executables and critical files verified before execution

- **Pre-execution Validation**: Config file and environment file verification before use- **Pre-Execution Verification**: Configuration and environment files verified before use

- **Hybrid Hash Encoding**: Space-efficient encoding with human-readable fallback- **Hybrid Hash Encoding**: Space-efficient encoding with human-readable fallback

- **Centralized Verification**: Unified verification management with automatic privilege handling- **Centralized Verification**: Unified verification management with automatic privilege handling

- **Group and Global Verification**: Flexible file verification at multiple levels- **Group and Global Verification**: Flexible file verification at multiple levels



### Command Execution### Command Execution

- **Batch Processing**: Command execution in organized groups with dependency management- **Batch Processing**: Execute commands in organized groups with dependency management

- **Variable Expansion**: `${VAR}` format expansion in command names and arguments- **Variable Expansion**: `${VAR}` format expansion in command names and arguments

- **Output Capture**: Save command output to files with secure permissions- **Output Capture**: Save command output to files with secure permissions

- **Background Execution**: Support for long-running processes with signal handling- **Background Execution**: Support for long-running processes with signal handling

- **Enhanced Dry Run**: Realistic simulation with comprehensive security analysis- **Enhanced Dry Run**: Realistic simulation with comprehensive security analysis

- **Timeout Controls**: Configurable timeouts for command execution- **Timeout Control**: Configurable timeouts for command execution

- **User/Group Context**: Execute commands as specific users with validation- **User/Group Context**: Execute commands as specific users with validation



### Logging and Monitoring### Logging and Monitoring

- **Multi-handler Logging**: Log routing to multiple destinations (console, file, Slack)- **Multi-Handler Logging**: Route logs to multiple destinations (console, file, Slack)

- **Interactive Terminal Support**: Color-coded output for enhanced visibility- **Interactive Terminal Support**: Color-coded output with enhanced visibility

- **Smart Terminal Detection**: Automatic detection of terminal capabilities- **Smart Terminal Detection**: Automatic detection of terminal capabilities

- **Color Control**: Support for CLICOLOR, NO_COLOR, CLICOLOR_FORCE environment variables- **Color Control**: Support for CLICOLOR, NO_COLOR, and CLICOLOR_FORCE environment variables

- **Slack Integration**: Real-time notifications for security events- **Slack Integration**: Real-time notifications for security events

- **Sensitive Data Redaction**: Automatic filtering of sensitive information- **Sensitive Data Redaction**: Automatic filtering of sensitive information

- **ULID Execution Tracking**: Time-ordered execution tracking- **ULID Run Tracking**: Time-ordered execution tracking



### File Operations### File Operations

- **Safe File I/O**: Symlink-aware file operations with security checks- **Safe File I/O**: Symlink-aware file operations with security checks

- **Hash Recording**: SHA-256 hash recording for integrity verification- **Hash Recording**: Record SHA-256 hashes for integrity verification

- **Verification Tools**: Standalone utilities for file verification- **Verification Tools**: Standalone utilities for file verification



## Architecture## Architecture



The system follows a modular architecture with clear separation of concerns:The system follows a modular architecture with clear separation of concerns:



``````

cmd/                    # Command-line entry pointscmd/                    # Command-line entry points

├── runner/            # Main command runner application├── runner/            # Main command runner application

├── record/            # Hash recording utility├── record/            # Hash recording utility

└── verify/            # File verification utility└── verify/            # File verification utility



internal/              # Core implementationinternal/              # Core implementation

├── cmdcommon/         # Shared command utilities├── cmdcommon/         # Shared command utilities

├── color/             # Terminal color support├── color/             # Terminal color support

├── common/            # Common utilities and filesystem abstraction├── common/            # Common utilities and filesystem abstraction

├── filevalidator/     # File integrity verification├── filevalidator/     # File integrity validation

│   └── encoding/      # Hybrid hash filename encoding│   └── encoding/      # Hybrid hash filename encoding

├── groupmembership/   # User/group membership validation├── groupmembership/   # User/group membership validation

├── logging/           # Advanced logging with Slack integration├── logging/           # Advanced logging with Slack integration

├── redaction/         # Automatic sensitive data filtering├── redaction/         # Automatic sensitive data filtering

├── runner/            # Command execution engine├── runner/            # Command execution engine

│   ├── audit/         # Security audit logging│   ├── audit/         # Security audit logging

│   ├── bootstrap/     # System initialization│   ├── bootstrap/     # System initialization

│   ├── cli/           # Command-line interface│   ├── cli/           # Command-line interface

│   ├── config/        # Configuration management│   ├── config/        # Configuration management

│   ├── environment/   # Environment variable processing│   ├── environment/   # Environment variable processing

│   ├── errors/        # Centralized error handling│   ├── errors/        # Centralized error handling

│   ├── executor/      # Command execution logic│   ├── executor/      # Command execution logic

│   ├── hashdir/       # Hash directory security│   ├── hashdir/       # Hash directory security

│   ├── output/        # Output capture management│   ├── output/        # Output capture management

│   ├── privilege/     # Privilege management│   ├── privilege/     # Privilege management

│   ├── resource/      # Resource management (normal/dry-run)│   ├── resource/      # Resource management (normal/dry-run)

│   ├── risk/          # Risk-based command assessment│   ├── risk/          # Risk-based command assessment

│   ├── runnertypes/   # Type definitions and interfaces│   ├── runnertypes/   # Type definitions and interfaces

│   └── security/      # Security validation framework│   └── security/      # Security validation framework

├── safefileio/        # Secure file operations├── safefileio/        # Secure file operations

├── terminal/          # Terminal capability detection├── terminal/          # Terminal capabilities detection

└── verification/      # Centralized verification management└── verification/      # Centralized verification management

``````



## Quick Start## Quick Start



### Basic Usage### Basic Usage



```bash```bash

# Execute commands from config file# Execute commands from configuration file

./runner -config config.toml./runner -config config.toml



# Dry run mode (preview without execution)# Dry run mode (preview without execution)

./runner -config config.toml -dry-run./runner -config config.toml -dry-run



# Validate config file# Validate configuration file

./runner -config config.toml -validate./runner -config config.toml -validate

``````



For detailed usage, see the [runner command guide](docs/user/runner_command.md).### Simple Configuration Example



### Simple Configuration Example```toml

version = "1.0"

```toml

version = "1.0"[global]

timeout = 3600

[global]log_level = "info"

timeout = 3600env_allowlist = ["PATH", "HOME", "USER"]

log_level = "info"

env_allowlist = ["PATH", "HOME", "USER"][[groups]]

name = "backup"

[[groups]]description = "System backup operations"

name = "backup"

description = "System backup operations"[[groups.commands]]

name = "database_backup"

[[groups.commands]]description = "Backup database"

name = "database_backup"cmd = "/usr/bin/mysqldump"

description = "Backup database"args = ["--all-databases"]

cmd = "/usr/bin/mysqldump"output = "backup.sql"  # Save output to file

args = ["--all-databases"]run_as_user = "mysql"

output = "backup.sql"  # Save output to filemax_risk_level = "medium"

run_as_user = "mysql"```

max_risk_level = "medium"

```## Configuration



## Configuration### Basic Configuration Structure



Configuration files define how commands should be executed using TOML format. The configuration file has the following hierarchical structure:```toml

version = "1.0"

- **Root Level**: Version information

- **Global Level**: Default settings applied to all groups[global]

- **Group Level**: Grouping of related commandstimeout = 3600

- **Command Level**: Individual command settingsworkdir = "/tmp"

log_level = "info"

### Basic Configuration Exampleskip_standard_paths = true  # Skip verification for system paths

env_allowlist = ["PATH", "HOME", "USER", "LANG"]

```tomlverify_files = ["/etc/passwd", "/bin/bash"]

version = "1.0"

[[groups]]

[global]name = "maintenance"

timeout = 3600description = "System maintenance tasks"

log_level = "info"priority = 1

env_allowlist = ["PATH", "HOME", "USER", "LANG"]env_allowlist = ["PATH", "HOME"]  # Override global allowlist



[[groups]][[groups.commands]]

name = "maintenance"name = "system_check"

description = "System maintenance tasks"cmd = "/usr/bin/systemctl"

priority = 1args = ["status"]

max_risk_level = "medium"

[[groups.commands]]```

name = "system_check"

cmd = "/usr/bin/systemctl"### Variable Expansion

args = ["status"]

max_risk_level = "medium"Use `${VAR}` format for dynamic configuration:

```

```toml

### Detailed Configuration Instructions[[groups.commands]]

name = "deploy"

For detailed configuration file description, refer to the following documentation:cmd = "${TOOL_DIR}/deploy.sh"

args = ["--config", "${CONFIG_FILE}"]

- [TOML Configuration File User Guide](docs/user/toml_config/README.md) - Comprehensive configuration guideenv = ["TOOL_DIR=/opt/tools", "CONFIG_FILE=/etc/app.conf"]

  - [Configuration File Hierarchy](docs/user/toml_config/02_hierarchy.md)```

  - [Global Level Settings](docs/user/toml_config/04_global_level.md)

  - [Group Level Settings](docs/user/toml_config/05_group_level.md)### Output Capture

  - [Command Level Settings](docs/user/toml_config/06_command_level.md)

  - [Variable Expansion Features](docs/user/toml_config/07_variable_expansion.md)Save command output to files:

  - [Practical Configuration Examples](docs/user/toml_config/08_practical_examples.md)

```toml

## Security Model[[groups.commands]]

name = "generate_report"

### File Integrity Verificationcmd = "/usr/bin/df"

args = ["-h"]

1. **Pre-execution Validation**output = "reports/disk_usage.txt"  # Tee output to file (0600 permissions)

   - Config file validation before loading```

   - Environment file validation before use

   - Prevention of malicious configuration attacks### Risk-Based Control



2. **Hash Directory Security**Configure security risk thresholds:

   - Fixed default: `/usr/local/etc/go-safe-cmd-runner/hashes`

   - No custom directories in production```toml

   - Build tag-separated test API[[groups.commands]]

name = "file_operation"

3. **Hybrid Hash Encoding**cmd = "/bin/cp"

   - Space-efficient encoding (1.00x expansion)args = ["source.txt", "dest.txt"]

   - Human-readable for debuggingmax_risk_level = "low"  # Only allow low-risk commands

   - Automatic SHA256 fallback

[[groups.commands]]

4. **Verification Management**name = "system_admin"

   - Centralized verificationcmd = "/usr/bin/systemctl"

   - Automatic privilege handlingargs = ["restart", "nginx"]

   - Group and global verification listsmax_risk_level = "high"  # Allow high-risk operations

```

### Risk-based Security Controls

### User and Group Execution

- **Automatic Risk Assessment**: Command classification by risk level

- **Configurable Thresholds**: Per-command risk level limitsExecute commands with specific user/group context:

- **Automatic Blocking**: Automatic blocking of high-risk commands

- **Risk Categories**:```toml

  - **Low**: Basic operations (ls, cat, grep)[[groups.commands]]

  - **Medium**: File modification (cp, mv), package managementname = "db_backup"

  - **High**: System administration (systemctl), destructive operationscmd = "/usr/bin/pg_dump"

  - **Critical**: Privilege escalation (sudo, su) - Always blockedargs = ["mydb"]

run_as_user = "postgres"

### Environment Isolationrun_as_group = "postgres"

output = "/backups/db.sql"

- **Secure Fixed PATH**: Hardcoded `/sbin:/usr/sbin:/bin:/usr/bin````

- **No PATH Inheritance**: Eliminates PATH manipulation attacks

- **Allowlist Filtering**: Strict zero-trust environment control### Environment Variable Security

- **Variable Expansion**: Secure `${VAR}` expansion with allowlists

- **Command.Env Priority**: Configuration overrides OS environmentStrict allowlist-based control:



### Privilege Management```toml

[global]

- **Automatic Dropping**: Privilege dropping after initialization# Global allowlist (default for all groups)

- **Controlled Escalation**: Risk-responsive privilege managementenv_allowlist = ["PATH", "HOME"]

- **User/Group Switching**: Secure context switching with validation

- **Audit Trail**: Complete logging of privilege changes[[groups]]

name = "secure_group"

### Output Capture Security# Override with empty list = no environment variables

env_allowlist = []

- **Secure Permissions**: Output files created with 0600 permissions

- **Privilege Separation**: Output files use real UID (not run_as_user)[[groups]]

- **Directory Security**: Automatic directory creation with secure permissionsname = "web_group"

- **Path Validation**: Prevention of path traversal attacks# Override with custom list

env_allowlist = ["PATH", "HOME", "WEB_ROOT"]

### Logging Security```



- **Sensitive Data Redaction**: Automatic detection of secrets, tokens, API keys## Security Model

- **Multi-channel Notifications**: Encrypted Slack communications

- **Audit Trail Protection**: Tamper-resistant structured logging### File Integrity Verification

- **Real-time Alerts**: Immediate notification of security violations

1. **Pre-Execution Verification**

## Command Line Tools   - Configuration files verified before reading

   - Environment files verified before use

go-safe-cmd-runner provides three command-line tools:   - Prevents malicious configuration attacks



### runner - Main Execution Command2. **Hash Directory Security**

   - Fixed default: `/usr/local/etc/go-safe-cmd-runner/hashes`

```bash   - No custom directories in production

# Basic execution   - Test API separated with build tags

./runner -config config.toml

3. **Hybrid Hash Encoding**

# Dry run (check execution content)   - Space-efficient encoding (1.00x expansion)

./runner -config config.toml -dry-run   - Human-readable for debugging

   - Automatic SHA256 fallback

# Configuration validation

./runner -config config.toml -validate4. **Verification Management**

```   - Centralized verification

   - Automatic privilege handling

See [runner command guide](docs/user/runner_command.md) for details.   - Group and global verification lists



### record - Hash Recording Command### Risk-Based Security Control



```bash- **Automatic Risk Assessment**: Commands classified by risk level

# Record file hash- **Configurable Thresholds**: Per-command risk level limits

./record -file /path/to/executable- **Automatic Blocking**: High-risk commands blocked automatically

- **Risk Categories**:

# Force overwrite existing hash  - **Low**: Basic operations (ls, cat, grep)

./record -file /path/to/file -force  - **Medium**: File modifications (cp, mv), package management

```  - **High**: System administration (systemctl), destructive operations

  - **Critical**: Privilege escalation (sudo, su) - always blocked

See [record command guide](docs/user/record_command.md) for details.

### Environment Isolation

### verify - File Verification Command

- **Secure Fixed PATH**: Hardcoded `/sbin:/usr/sbin:/bin:/usr/bin`

```bash- **No PATH Inheritance**: Eliminates PATH manipulation attacks

# Verify file integrity- **Allowlist Filtering**: Strict zero-trust environment control

./verify -file /path/to/file- **Variable Expansion**: Secure `${VAR}` expansion with allowlist

```- **Command.Env Priority**: Configuration overrides OS environment



See [verify command guide](docs/user/verify_command.md) for details.### Privilege Management



### Comprehensive User Guide- **Automatic Dropping**: Privileges dropped after initialization

- **Controlled Escalation**: Risk-aware privilege management

For detailed usage, configuration examples, and troubleshooting, see the [User Guide](docs/user/README.md).- **User/Group Switching**: Secure context switching with validation

- **Audit Trail**: Complete logging of privilege changes

## Building and Installation

### Output Capture Security

### Prerequisites

- **Secure Permissions**: Output files created with 0600 permissions

- Go 1.23 or later (required for slices package, range over count)- **Privilege Separation**: Output files use real UID (not run_as_user)

- golangci-lint (for development)- **Directory Security**: Automatic directory creation with secure permissions

- gofumpt (for code formatting)- **Path Validation**: Prevention of path traversal attacks



### Build Commands### Logging Security



```bash- **Sensitive Data Redaction**: Automatic detection of secrets, tokens, API keys

# Build all binaries- **Multi-Channel Notifications**: Encrypted Slack communications

make build- **Audit Trail Protection**: Tamper-resistant structured logging

- **Real-Time Alerts**: Immediate notification of security violations

# Build specific binary

make build/runner## Command Line Tools

make build/record

make build/verify### Main Runner



# Run tests```bash

make test# Basic execution

./runner -config config.toml

# Run linter

make lint# Dry run with security analysis

./runner -config config.toml -dry-run

# Format code

make fmt# Validate configuration

./runner -config config.toml -validate

# Clean build artifacts

make clean# Custom log settings

./runner -config config.toml -log-dir /var/log/runner -log-level debug

# Run benchmarks

make benchmark# Color control

CLICOLOR=1 ./runner -config config.toml       # Enable color

# Generate coverage reportNO_COLOR=1 ./runner -config config.toml       # Disable color

make coverageCLICOLOR_FORCE=1 ./runner -config config.toml # Force color

```

# Slack notifications (requires GSCR_SLACK_WEBHOOK_URL)

### InstallationGSCR_SLACK_WEBHOOK_URL=https://hooks.slack.com/... ./runner -config config.toml

```

```bash

# Install from source### Hash Management

git clone https://github.com/isseis/go-safe-cmd-runner.git

cd go-safe-cmd-runner```bash

make build# Record file hash (uses default hash directory)

./record -file /path/to/executable

# Install binaries to system location

sudo install -o root -g root -m 4755 build/runner /usr/local/bin/go-safe-cmd-runner# Force overwrite existing hash

sudo install -o root -g root -m 0755 build/record /usr/local/bin/go-safe-cmd-record./record -file /path/to/file -force

sudo install -o root -g root -m 0755 build/verify /usr/local/bin/go-safe-cmd-verify

# Verify file integrity (uses default hash directory)

# Create default hash directory./verify -file /path/to/file

sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes

sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes# Note: -hash-dir option available for testing only

sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes./record -file /path/to/file -hash-dir /custom/test/hashes

``````



## Development## Building and Installation



### Dependencies### Prerequisites



- `github.com/pelletier/go-toml/v2` - TOML configuration parsing- Go 1.23 or later (required for slices package, range over count)

- `github.com/oklog/ulid/v2` - ULID generation for execution tracking- golangci-lint (for development)

- `github.com/stretchr/testify` - Testing framework- gofumpt (for code formatting)

- `golang.org/x/term` - Terminal capability detection

### Build Commands

### Testing

```bash

```bash# Build all binaries

# Run all testsmake build

go test -v ./...

# Build specific binary

# Run tests for specific packagemake build/runner

go test -v ./internal/runnermake build/record

make build/verify

# Run integration tests

make integration-test# Run tests

make test

# Run Slack notification tests (requires GSCR_SLACK_WEBHOOK_URL)

make slack-notify-test# Run linter

make slack-group-notification-testmake lint

```

# Format code

### Project Structuremake fmt



The codebase follows Go best practices:# Clean build artifacts

- **Interface-driven design** for testabilitymake clean

- **Comprehensive error handling** with custom error types

- **Security-first approach** with extensive validation# Run benchmarks

- **Modular architecture** with clear boundariesmake benchmark

- **Build tag separation** for production/test code

# Generate coverage report

### Execution Identification with ULIDmake coverage

```

The system uses ULID (Universally Unique Lexicographically Sortable Identifier):

- **Chronologically sortable**: Naturally ordered by creation time### Installation

- **URL-safe**: No special characters, suitable for filenames

- **Compact**: Fixed 26-character length```bash

- **Collision-resistant**: Guaranteed uniqueness with monotonic entropy# Install from source

- **Example**: `01K2YK812JA735M4TWZ6BK0JH9`git clone https://github.com/isseis/go-safe-cmd-runner.git

cd go-safe-cmd-runner

## Out of Scopemake build



This project explicitly does **not** provide:# Install binaries to system location

- Container orchestration or Docker integrationsudo install -o root -g root -m 4755 build/runner /usr/local/bin/go-safe-cmd-runner

- Network security features (firewall, VPN, etc.)sudo install -o root -g root -m 0755 build/record /usr/local/bin/go-safe-cmd-record

- User authentication or authorization systemssudo install -o root -g root -m 0755 build/verify /usr/local/bin/go-safe-cmd-verify

- Web interfaces or REST APIs

- Database management capabilities# Create default hash directory

- Real-time monitoring or alerting systemssudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes

- Cross-platform GUI applicationssudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes

- Package management or software installationsudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

```

It focuses on secure command execution with comprehensive security controls in Unix-like environments.

## Development

## Contributing

### Dependencies

This project prioritizes security and reliability. When contributing:

- Follow security-first design principles- `github.com/pelletier/go-toml/v2` - TOML configuration parsing

- Add comprehensive tests for new features- `github.com/oklog/ulid/v2` - ULID generation for run tracking

- Update documentation for configuration changes- `github.com/stretchr/testify` - Testing framework

- Ensure all security validations are tested- `golang.org/x/term` - Terminal capability detection

- Use static analysis tools (golangci-lint)

- Follow Go coding standards and best practices### Testing



For questions and contributions, refer to the project's issue tracker.```bash

# Run all tests

## Licensego test -v ./...



This project is released under the MIT License. See the [LICENSE](./LICENSE) file for details.# Run tests for specific package
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
