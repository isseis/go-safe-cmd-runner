# Go Safe Command Runner - Design and Implementation Overview

## Executive Summary

The Go Safe Command Runner is a security-focused command execution framework designed for privileged task delegation and automated batch processing. The system provides multiple layers of security controls to enable safe execution of privileged operations while maintaining strict security boundaries.

**Key Use Cases:**
- Scheduled backup operations with root privileges
- Delegating specific administrative tasks to non-root users
- Automated system maintenance with security controls
- Batch processing with file integrity verification

## System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Command Line Interface                   │
├─────────────────────┬───────────────────┬───────────────────┤
│ runner              │ record            │ verify            │
│ (Main executor)     │ (Hash recording)  │ (File validation) │
└─────────────────────┴───────────────────┴───────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      Core Engine                            │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │ Configuration   │  │ Security        │                  │
│  │ Management      │  │ Framework       │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │ Command         │  │ File Integrity  │                  │
│  │ Execution       │  │ Verification    │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │ Environment     │  │ Privilege       │                  │
│  │ Management      │  │ Management      │                  │
│  └─────────────────┘  └─────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                  System Interface Layer                     │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │ Safe File I/O   │  │ Process         │                  │
│  │ (Symlink        │  │ Execution       │                  │
│  │ Protection)     │  │                 │                  │
│  └─────────────────┘  └─────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
```

### Core Components

#### 1. Configuration Management (`internal/runner/config/`)
- **Purpose**: TOML-based configuration loading and validation
- **Key Features**:
  - Schema validation with required field checking
  - Path security validation (absolute paths, no relative components)
  - Default value assignment
  - Cross-reference validation between sections

**Implementation Highlights:**
```go
// Safe configuration loading with validation
func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
    data, err := safefileio.SafeReadFile(path)  // Secure file reading
    if err := toml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }
    // Path validation and default assignment
}
```

#### 2. Command Execution Engine (`internal/runner/executor/`)
- **Purpose**: Safe command execution with output capture and timeout control
- **Key Features**:
  - Process isolation and resource management
  - Configurable timeouts at global and command level
  - Structured output capture with size limits
  - Background process support with signal handling

#### 3. File Integrity System (`internal/filevalidator/`)
- **Purpose**: SHA-256 based file verification to prevent tampered binary execution
- **Key Features**:
  - Hash recording and verification workflow
  - Privileged file access with controlled escalation
  - Atomic operations to prevent race conditions
  - Integration with privilege management

**Security Flow:**
```
File Access Request → Permission Check → Privilege Escalation (if needed)
→ File Open → Privilege Restoration → Hash Calculation → Verification
```

#### 4. Privilege Management (`internal/runner/privilege/`)
- **Purpose**: Controlled privilege escalation with comprehensive audit trails
- **Key Features**:
  - Thread-safe privilege operations with global mutex
  - Automatic privilege restoration with panic protection
  - Support for both native root and setuid binary execution
  - Emergency shutdown protocol on security failures

**Privilege Escalation Pattern:**
```go
func (m *Manager) WithPrivileges(ctx ElevationContext, fn func() error) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if err := m.escalatePrivileges(ctx); err != nil {
        return err
    }

    defer m.emergencyShutdownOnRestoreFailure(fn) // Fail-safe mechanism
    return fn()
}
```

#### 5. Environment Security (`internal/runner/environment/`)
- **Purpose**: Zero-trust environment variable filtering
- **Key Features**:
  - Allowlist-based filtering at global and group levels
  - Dangerous pattern detection (passwords, tokens, etc.)
  - Inheritance control (inherit/explicit/reject modes)
  - Variable name and value validation

#### 6. Safe File Operations (`internal/safefileio/`)
- **Purpose**: Symlink-safe file operations using modern Linux security primitives
- **Key Features**:
  - openat2 with RESOLVE_NO_SYMLINKS for symlink attack prevention
  - Step-by-step path validation
  - Atomic file operations
  - Cross-platform compatibility with fallback mechanisms

#### 7. Security Framework (`internal/runner/security/`)
- **Purpose**: Centralized security validation and policy enforcement
- **Key Features**:
  - Command path allowlist validation
  - Dangerous command detection
  - File permission validation
  - Path traversal attack prevention

## Data Flow Architecture

### Command Execution Flow

```
Configuration Loading → Security Validation → Group Processing → Command Execution

1. Configuration Loading:
   ├── TOML parsing and validation
   ├── Path security checks
   ├── Default value assignment
   └── Cross-reference validation

2. Security Validation:
   ├── File integrity verification
   ├── Environment variable filtering
   ├── Command path validation
   └── Permission checks

3. Group Processing:
   ├── Dependency resolution
   ├── Priority ordering
   ├── Resource allocation (temp directories)
   └── Environment preparation

4. Command Execution:
   ├── Privilege escalation (if needed)
   ├── Process spawning with isolation
   ├── Output capture and monitoring
   ├── Privilege restoration
   └── Cleanup and logging
```

### File Verification Flow

```
File Path Input → Security Check → Hash Calculation → Verification → Result

1. Security Check:
   ├── Path validation (no symlinks, absolute path)
   ├── Permission analysis
   └── Privilege requirement determination

2. File Access & Hash Calculation:
   ├── Privilege escalation (if file requires root access)
   ├── Safe file opening
   ├── Privilege restoration (immediately after file open)
   ├── Streaming SHA-256 calculation (with normal privileges)
   └── Hash comparison preparation

3. Verification:
   ├── Hash comparison with stored values
   ├── Error reporting with detailed context
   └── Audit logging
```

## Security Design Principles

### 1. Defense in Depth
Multiple security layers ensure that a single point of failure doesn't compromise the entire system:
- **Input Validation**: All inputs validated at entry points
- **Path Security**: Comprehensive path validation and symlink protection
- **File Integrity**: Hash-based verification of all critical files
- **Privilege Control**: Minimal privilege principle with controlled escalation
- **Environment Isolation**: Strict allowlist-based environment filtering
- **Command Validation**: Allowlist-based command execution control

### 2. Zero Trust Model
No implicit trust in system environment:
- All files verified before use
- Environment variables filtered by allowlist
- Commands validated against known-good patterns
- Privileges granted only when necessary and immediately revoked

### 3. Fail-Safe Design
System designed to fail securely:
- Default deny for all operations
- Emergency shutdown on security failures
- Comprehensive error handling and logging
- Graceful degradation when security features unavailable

### 4. Audit and Monitoring
Complete visibility into security-relevant operations:
- Structured logging with security context
- Privilege operation metrics and tracking
- Security event recording
- Multi-channel critical error reporting

## Implementation Patterns

### 1. Interface-Driven Design
Heavy use of interfaces for testability and modularity:
```go
type PrivilegeManager interface {
    WithPrivileges(context ElevationContext, fn func() error) error
    IsSupported() bool
}

type FileValidator interface {
    Verify(filepath string) error
    Record(filepath string) (string, error)
}
```

### 2. Composition Over Inheritance
Component composition for feature extension:
```go
type ValidatorWithPrivileges struct {
    *Validator                    // Base functionality
    privMgr      PrivilegeManager // Extended capability
    logger       *slog.Logger     // Observability
}
```

### 3. Context-Aware Operations
Operations carry context for security and observability:
```go
type ElevationContext struct {
    Operation  string
    FilePath   string
    Reason     string
}
```

### 4. Builder Pattern for Configuration
Flexible configuration with sensible defaults:
```go
func NewRunnerWithOptions(config *Config, opts ...Option) (*Runner, error) {
    options := &runnerOptions{}
    for _, opt := range opts {
        opt(options)
    }
    // Apply options and create runner
}
```

## Testing Strategy

### 1. Unit Testing
- Comprehensive test coverage for all core components
- Mock implementations for external dependencies
- Error condition testing with custom error types
- Race condition testing for concurrent operations

### 2. Integration Testing
- End-to-end workflow testing
- File system interaction testing
- Privilege operation testing
- Configuration loading and validation testing

### 3. Security Testing
- Symlink attack prevention testing
- Path traversal attack testing
- Privilege escalation boundary testing
- Environment variable injection testing

### 4. Performance Testing
- Hash calculation performance benchmarks
- Memory usage optimization
- Concurrent operation performance
- Large file handling efficiency

## Deployment Considerations

### 1. Binary Distribution
- Setuid bit configuration for privilege escalation
- Root ownership requirements
- Binary integrity verification before deployment
- Secure installation procedures

### 2. Configuration Management
- Secure hash directory permissions (755 or stricter)
- Write-protected configuration files
- Regular integrity verification of critical files
- Configuration template management

### 3. Monitoring and Alerting
- Structured logs for security events
- Syslog integration for centralized logging
- Emergency shutdown event monitoring
- Performance metrics collection

### 4. Security Operations
- Regular security audits of configuration
- Privilege operation monitoring
- File integrity verification schedules
- Incident response procedures

## Performance Characteristics

### 1. Hash Computation
- Efficient streaming hash calculation
- File size limits prevent resource exhaustion
- Parallel processing for multiple files
- Memory-efficient implementation

### 2. Environment Processing
- O(1) allowlist lookups using map structures
- Compiled regex patterns for pattern matching
- Minimal string operations
- Batch processing optimization

### 3. Privilege Operations
- Global mutex serializes privileged operations
- Fast privilege escalation/restoration using system calls
- Metrics collection for performance monitoring
- Resource usage tracking

## Future Extensibility

### 1. Plugin Architecture
The interface-driven design enables easy extension:
- Custom hash algorithms
- Additional privilege backends
- Extended security validators
- Custom output formatters

### 2. Platform Support
Current focus on Linux/Unix with extensibility for:
- Windows privilege management
- macOS security features
- Container runtime integration
- Cloud platform adapters

### 3. Integration Points
Well-defined interfaces for integration with:
- Configuration management systems
- Monitoring and alerting platforms
- Audit and compliance systems
- Identity and access management

## Conclusion

The Go Safe Command Runner demonstrates security engineering best practices through its multi-layered security approach, comprehensive input validation, secure privilege management, and extensive audit capabilities. The system is designed to fail securely and provide complete visibility into security-relevant operations, making it suitable for production use in security-conscious environments.

The implementation showcases modern Go development patterns including interface-driven design, composition-based architecture, and comprehensive testing strategies. The system's modular design enables easy extension and customization while maintaining strict security boundaries.
