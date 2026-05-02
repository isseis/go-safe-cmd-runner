# Package Structure Reference

This document provides a detailed reference of the package structure in this codebase.

## Directory Structure

- `cmd/`: Command-line entry points
  - `runner/`: Main command runner application
  - `record/`: Hash recording utility
  - `verify/`: File verification utility
- `internal/`: Core implementation
  - `ansicolor/`: Terminal color support (ANSI escape codes)
  - `arm64util/`: Shared ARM64 instruction decoding utilities
  - `cmdcommon/`: Shared command utilities
  - `common/`: Common utilities and filesystem abstraction
  - `dynlib/`: Dynamic library dependency analysis
    - `elfdynlib/`: ELF binary dynamic library dependency analysis
    - `machodylib/`: Mach-O binary dynamic library dependency analysis
  - `fileanalysis/`: Unified file analysis records (hash, syscall, symbol, shebang)
  - `filevalidator/`: File integrity validation
    - `pathencoding/`: Hybrid hash filename encoding
  - `groupmembership/`: User/group membership validation
  - `libccache/`: libc syscall wrapper symbol caching and matching
  - `logging/`: Advanced logging system with Slack integration
  - `redaction/`: Automatic sensitive data filtering
  - `runner/`: Command execution engine
    - `base/`: Generic packages (no dependency on flat packages)
      - `audit/`: Security audit logging
      - `environment/`: Environment variable processing and filtering
      - `executor/`: Command execution logic
      - `output/`: Output path validation and security
      - `privilege/`: Privilege management
      - `risk/`: Risk-based command assessment
      - `runnertypes/`: Type definitions and interfaces
      - `security/`: Security validation framework
      - `variable/`: Automatic variable generation and definitions
    - `bootstrap/`: System initialization and bootstrap
    - `cli/`: Command-line interface management
    - `config/`: Configuration management
    - `debuginfo/`: Debug functionality and utilities
    - `resource/`: Unified resource management (normal/dry-run)
    - `runerrors/`: Centralized error handling
  - `safefileio/`: Secure file operations with symlink protection
  - `security/`: Binary security analysis framework
    - `binaryanalyzer/`: Common interfaces and types for binary analysis
    - `elfanalyzer/`: ELF binary network capability and syscall detection
    - `machoanalyzer/`: Mach-O binary network capability detection
  - `shebang/`: Shebang line parsing and interpreter path resolution
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
- **`filevalidator/pathencoding/`**: Hybrid hash filename encoding with automatic fallback
- **`verification/`**: Centralized pre-execution file verification management

#### Binary Analysis
- **`security/`**: Binary security analysis framework
  - **`binaryanalyzer/`**: Common interfaces and types shared across binary analyzers
  - **`elfanalyzer/`**: ELF binary analysis — network capability detection (socket/connect symbols), dangerous syscall patterns (mprotect/pkey_mprotect with PROT_EXEC), and static syscall number extraction
  - **`machoanalyzer/`**: Mach-O binary network capability detection
- **`dynlib/`**: Dynamic library dependency analysis
  - **`elfdynlib/`**: ELF binary dynamic library dependency analysis (DT_NEEDED, RPATH, RUNPATH)
  - **`machodylib/`**: Mach-O binary dynamic library dependency analysis
- **`fileanalysis/`**: Unified file analysis records combining hash, syscall, symbol, and shebang results
- **`libccache/`**: libc syscall wrapper symbol caching and matching
- **`arm64util/`**: Shared ARM64 instruction decoding utilities used by elfanalyzer and related packages
- **`shebang/`**: Shebang line parsing and interpreter path resolution

#### Command Execution
- **`runner/`**: Core command execution engine
  - **`runner/base/executor/`**: Command execution with output handling
  - **`runner/config/`**: TOML configuration loading and validation
  - **`runner/base/runnertypes/`**: Shared type definitions and interfaces
  - **`runner/base/environment/`**: Environment variable processing and filtering
  - **`runner/base/variable/`**: Automatic variable generation

#### Security
- **`runner/base/security/`**: Security validation framework
- **`runner/base/audit/`**: Security audit logging
- **`runner/base/privilege/`**: Privilege management
- **`runner/base/risk/`**: Risk-based command assessment
- **`runner/base/output/`**: Output path validation and security
- **`groupmembership/`**: User/group membership validation

#### User Interface
- **`terminal/`**: Terminal capabilities detection
- **`ansicolor/`**: Terminal color support (ANSI escape codes)
- **`runner/cli/`**: Command-line interface management
- **`logging/`**: Advanced logging with Slack integration

#### Utilities
- **`common/`**: Common utilities and filesystem abstraction
- **`cmdcommon/`**: Shared command utilities
- **`redaction/`**: Automatic sensitive data filtering
- **`runner/debuginfo/`**: Debug functionality
- **`runner/runerrors/`**: Centralized error handling
- **`runner/resource/`**: Unified resource management (normal/dry-run modes)
- **`runner/bootstrap/`**: System initialization and bootstrap

## Key Design Patterns

- **Separation of Concerns**: Each package has a single responsibility
- **Interface-based Design**: Heavy use of interfaces for testability (e.g., `CommandExecutor`, `FileSystem`, `OutputWriter`)
- **Security First**: Path validation, command injection prevention, privilege separation
- **Error Handling**: Comprehensive error types and validation
