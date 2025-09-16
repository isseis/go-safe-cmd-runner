# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build Commands
- `make build` - Build all binaries (record and verify executables)
- `make clean` - Clean build artifacts
- `make all` - Default build target

### Test Commands
- `make test` - Run all tests with verbose output
- `go test -v ./...` - Run all tests directly
- `go test -v ./internal/specific/package` - Run tests for specific package

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

## Package Structure

- `cmd/`: Entry points for different executables
- `internal/cmdcommon/`: Common utilities shared across commands
- `internal/filevalidator/`: File integrity validation with hash algorithms
- `internal/safefileio/`: Secure file I/O operations
- `internal/runner/`: Core command execution engine
  - `config/`: Configuration loading and validation
  - `executor/`: Command execution with security controls
  - `runnertypes/`: Type definitions and interfaces
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
