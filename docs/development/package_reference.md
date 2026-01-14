# Package Structure Reference

This document provides a detailed reference of the package structure in this codebase.

## Directory Structure

- `cmd/`: Command-line entry points
  - `runner/`: Main command runner application
  - `record/`: Hash recording utility
  - `verify/`: File verification utility
- `internal/`: Core implementation
  - `cmdcommon/`: Shared command utilities
  - `color/`: Terminal color support and control
  - `common/`: Common utilities and filesystem abstraction
  - `filevalidator/`: File integrity validation
    - `encoding/`: Filename encoding for hash storage
  - `groupmembership/`: User/group membership validation
  - `logging/`: Advanced logging system with interactive UI and Slack integration
  - `redaction/`: Automatic sensitive data filtering
  - `runner/`: Command execution engine
    - `audit/`: Security audit logging
    - `bootstrap/`: System initialization and bootstrap
    - `cli/`: Command-line interface management
    - `config/`: Configuration management
    - `debug/`: Debug functionality and utilities
    - `environment/`: Environment variable processing and filtering
    - `errors/`: Centralized error handling
    - `executor/`: Command execution logic
    - `output/`: Output path validation and security
    - `privilege/`: Privilege management
    - `resource/`: Unified resource management (normal/dry-run)
    - `risk/`: Risk-based command assessment
    - `runnertypes/`: Type definitions and interfaces
    - `security/`: Security validation framework
    - `variable/`: Automatic variable generation and definitions
  - `safefileio/`: Secure file operations
  - `terminal/`: Terminal capabilities detection and interactive UI support
  - `verification/`: Centralized file verification management (pre-execution verification, path resolution)
- `docs/`: Project documentation with requirements and architecture

## Package Responsibilities

### Command-Line Tools (`cmd/`)

- **`runner/`**: Main application that executes commands based on TOML configuration files
- **`record/`**: Utility to generate hash files for integrity verification
- **`verify/`**: Utility to verify file integrity against recorded hashes

### Core Packages (`internal/`)

#### File Operations
- **`safefileio/`**: Secure file I/O operations with symlink attack prevention
- **`filevalidator/`**: File integrity verification using hash validation
- **`verification/`**: Centralized pre-execution file verification management

#### Command Execution
- **`runner/`**: Core command execution engine
  - **`executor/`**: Command execution with output handling
  - **`config/`**: TOML configuration loading and validation
  - **`runnertypes/`**: Shared type definitions and interfaces
  - **`environment/`**: Environment variable processing and filtering
  - **`variable/`**: Automatic variable generation

#### Security
- **`runner/security/`**: Security validation framework
- **`runner/audit/`**: Security audit logging
- **`runner/privilege/`**: Privilege management
- **`runner/risk/`**: Risk-based command assessment
- **`runner/output/`**: Output path validation and security
- **`groupmembership/`**: User/group membership validation

#### User Interface
- **`terminal/`**: Terminal capabilities detection
- **`color/`**: Terminal color support
- **`runner/cli/`**: Command-line interface management
- **`logging/`**: Advanced logging with interactive UI and Slack integration

#### Utilities
- **`common/`**: Common utilities and filesystem abstraction
- **`cmdcommon/`**: Shared command utilities
- **`redaction/`**: Automatic sensitive data filtering
- **`runner/debug/`**: Debug functionality
- **`runner/errors/`**: Centralized error handling
- **`runner/resource/`**: Unified resource management (normal/dry-run modes)
- **`runner/bootstrap/`**: System initialization and bootstrap

## Key Design Patterns

- **Separation of Concerns**: Each package has a single responsibility
- **Interface-based Design**: Heavy use of interfaces for testability (e.g., `CommandExecutor`, `FileSystem`, `OutputWriter`)
- **Security First**: Path validation, command injection prevention, privilege separation
- **Error Handling**: Comprehensive error types and validation
