# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Documents
- Documents should be placed under docs
- Default language is Japanese (exceptions: README.md, CLAUDE.md)
- Default format is markdown
 - Use Mermaid syntax for diagrams.
  - Follow the style and legend used in `docs/tasks/0030_verify_files_variable_expansion/02_architecture.md`.
  - Use a cylinder shape for "data" nodes instead of the default rectangle (in Mermaid flowcharts a cylinder node can be written as `[(data)]`).

## Commands

### Build Commands
- `make build` - Build all binaries (record and verify executables)
- `make clean` - Clean build artifacts
- `make all` - Default build target

### Test Commands
- `make test` - Run all tests with verbose output
- `go test -tags test -v ./...` - Run all tests directly
- `go test -tags test -v ./internal/specific/package` - Run tests for specific package

### Code Quality
- `make lint` - Run linter with golangci-lint
- `golangci-lint run` - Run linter directly
- `make fmt` - Run formetter with gofumpt

### Individual Binary Builds
- Build record binary: `go build -o build/record -v cmd/record/main.go`
- Build verify binary: `go build -o build/verify -v cmd/verify/main.go`

## Architecture Overview

This is a Go-based secure command runner with the following core components:

### Core Architecture
- **Command Runner**: Safe execution wrapper for batch processing with security controls
- **File Validator**: Integrity verification using hash validation (`internal/filevalidator`)
- **Safe File I/O**: Secure file operations with symlink protection (`internal/safefileio`)
- **Command Executor**: Core execution engine with output handling (`internal/runner/executor`)
- **Config Management**: TOML-based configuration loading (`internal/runner/config`)

### Key Design Patterns
- **Separation of Concerns**: Each package has a single responsibility
- **Interface-based Design**: Heavy use of interfaces for testability (e.g., `CommandExecutor`, `FileSystem`, `OutputWriter`)
- **Security First**: Path validation, command injection prevention, privilege separation
- **Error Handling**: Comprehensive error types and validation
- **YAGNI**: Use simple and clear approach to satisfy the requirement. Don't take complex approach for not-yet-planned features.
 - **DRY**: Don't repeat yourself. Before adding new code, check the codebase and prefer reusing existing implementations.

### Security Features
- Command path validation and sanitization
- Environment variable isolation
- Working directory validation
- File integrity verification with hash validation
- Safe file operations with symlink attack prevention

### Configuration
- Uses TOML format for configuration files
- Supports environment variable management
- Template-based command definitions
- Group-based command execution with dependency management

### Testing Strategy
- Unit tests for all core components
- Mock implementations for external dependencies
- File system abstraction for testing
- Output capture and verification
- **Error Testing**: Use `errors.Is()` to validate error types, not string matching on error messages

### Mock File Organization
All mock implementations should follow this standardized directory and file naming convention:

```
<package>/
├── <implementation>.go
├── <implementation>_test.go
└── testing/
    ├── mocks.go              # Lightweight mocks (no external dependencies)
    ├── testify_mocks.go      # testify-based mocks (for complex scenarios)
    ├── mocks_test.go         # Tests for mock implementations
    └── helpers.go            # Test utility functions
```

**File Naming Rules:**
- **`testing/mocks.go`**: Simple mock implementations without external library dependencies
- **`testing/testify_mocks.go`**: Advanced mocks using stretchr/testify framework
- **`testing/mocks_test.go`**: Unit tests for mock implementations
- **`testing/helpers.go`**: Common test utility functions and setup helpers

**Package Naming:**
- All testing utilities use `package testing` within the `testing/` subdirectory
- Import as: `<module>/internal/<package>/testing`

## Package Structure

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
    - `environment/`: Environment variable processing and filtering
    - `errors/`: Centralized error handling
    - `executor/`: Command execution logic
    - `hashdir/`: Hash directory security management
    - `output/`: Output path validation and security
    - `privilege/`: Privilege management
    - `resource/`: Unified resource management (normal/dry-run)
    - `risk/`: Risk-based command assessment
    - `runnertypes/`: Type definitions and interfaces
    - `security/`: Security validation framework
  - `safefileio/`: Secure file operations
  - `terminal/`: Terminal capabilities detection and interactive UI support
  - `verification/`: Centralized file verification management (pre-execution verification, path resolution)
- `pkg/cmdutil/`: Public utilities for command-line tools
- `docs/`: Project documentation with requirements and architecture

## Development Notes

- Uses Go modules with Go 1.23.10
- Dependencies: go-toml/v2, stretchr/testify
- Security-focused codebase with extensive validation
- Comprehensive error handling with custom error types
- Interface-driven design for testability and modularity
- After editing *go files, make sure to run `make fmt` to format the files.
- After editing files, make sure to run `make test` and `make lint` and fix errors.
