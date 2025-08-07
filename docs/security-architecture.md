# Go Safe Command Runner - Security Architecture Documentation

## Overview

This document provides a comprehensive technical analysis of the security measures implemented in the Go Safe Command Runner project. It is intended for software engineers and security professionals who need to understand the design principles, implementation details, and security guarantees of the system.

## Executive Summary

The Go Safe Command Runner implements multiple layers of security controls to enable safe delegation of privileged operations and automated batch processing. The security model is built around the principle of defense-in-depth, combining file integrity verification, environment variable isolation, privilege management, and secure file operations.

## Core Security Features

### 1. File Integrity Verification

#### Purpose
Ensure that executables and critical files have not been tampered with before execution, preventing the execution of compromised binaries.

#### Implementation Details

**Hash Algorithm**: SHA-256 cryptographic hashing
- Location: `internal/filevalidator/hash_algo.go`
- Uses Go's standard `crypto/sha256` library
- Provides 256-bit hash values for strong collision resistance

**Hash Storage System**:
- Hash files stored as JSON manifests in dedicated directory
- File path encoded using Base64 URL-safe encoding to handle special characters
- Manifest format includes file path, hash value, algorithm, and timestamp
- Collision detection prevents different files from sharing the same hash file

**Verification Process**:
```go
// Location: internal/filevalidator/validator.go:169-197
func (v *Validator) Verify(filePath string) error {
    // 1. Validate and resolve file path
    targetPath, err := validatePath(filePath)

    // 2. Calculate current file hash
    actualHash, err := v.calculateHash(targetPath.String())

    // 3. Read stored hash from manifest
    _, expectedHash, err := v.readAndParseHashFile(targetPath)

    // 4. Compare hashes
    if expectedHash != actualHash {
        return ErrMismatch
    }
    return nil
}
```

**Privileged File Access**:
- Falls back to privilege escalation when normal verification fails due to permissions
- Uses secure privilege management (see Privilege Management section)
- Location: `internal/filevalidator/privileged_file.go`

#### Security Guarantees
- Detects unauthorized modifications to executables and configuration files
- Prevents execution of tampered binaries
- Cryptographically strong hash algorithm (SHA-256)
- Atomic file operations prevent race conditions

### 2. Environment Variable Isolation

#### Purpose
Implement strict allowlist-based filtering of environment variables to prevent information leakage and command injection attacks through environment manipulation.

#### Implementation Details

**Allowlist Architecture**:
```go
// Location: internal/runner/environment/filter.go:31-50
type Filter struct {
    config          *runnertypes.Config
    globalAllowlist map[string]bool // O(1) lookup performance
}
```

**Three-Level Inheritance Model**:

1. **Global Allowlist**: Base environment variables available to all groups
2. **Group Override**: Groups can define their own allowlist, completely overriding global settings
3. **Inheritance Control**: Groups without explicit allowlist inherit from global settings

**Inheritance Modes**:
- `InheritanceModeInherit`: Use global allowlist
- `InheritanceModeExplicit`: Use group-specific allowlist only
- `InheritanceModeReject`: No environment variables allowed (empty allowlist)

**Variable Validation**:
```go
// Location: internal/runner/security/security.go:639-649
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
    // Check for dangerous patterns using compiled regexes
    for _, re := range v.dangerousEnvRegexps {
        if re.MatchString(value) {
            return fmt.Errorf("%w: environment variable %s contains potentially dangerous pattern",
                ErrUnsafeEnvironmentVar, key)
        }
    }
    return nil
}
```

**Dangerous Pattern Detection**:
- Command separators: `;`, `|`, `&&`, `||`
- Command substitution: `$(`, backticks
- File operations: `>`, `<`, `rm `, `dd if=`, `dd of=`
- Code execution: `exec `, `system `, `eval `

#### Security Guarantees
- Zero-trust environment variable model (allowlist only)
- Prevents environment-based command injection
- Group-level isolation of sensitive variables
- Validation of variable names and values against dangerous patterns

### 3. Secure File Operations

#### Purpose
Provide symlink-safe file I/O operations to prevent symlink attacks, TOCTOU (Time-of-Check-Time-of-Use) race conditions, and path traversal attacks.

#### Implementation Details

**Modern Linux Security (openat2)**:
```go
// Location: internal/safefileio/safe_file.go:99-122
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    // Uses RESOLVE_NO_SYMLINKS flag to atomically prevent symlink following
    pathBytes, err := syscall.BytePtrFromString(pathname)
    fd, _, errno := syscall.Syscall6(SysOpenat2, ...)
    return int(fd), nil
}
```

**Fallback Security (Traditional Systems)**:
```go
// Location: internal/safefileio/safe_file.go:409-433
func ensureParentDirsNoSymlinks(absPath string) error {
    // Step-by-step path validation from root to target
    for _, component := range components {
        fi, err := os.Lstat(currentPath) // Does not follow symlinks
        if fi.Mode()&os.ModeSymlink != 0 {
            return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
        }
    }
    return nil
}
```

**File Size Protection**:
- Maximum file size limit: 128 MB
- Prevents memory exhaustion attacks
- Uses `io.LimitReader` for consistent behavior

**Path Validation**:
- Absolute path requirement
- Path length limits (configurable, default 4096 characters)
- Regular file type validation
- No device files, pipes, or special files allowed

#### Security Guarantees
- Atomic symlink-safe operations on modern Linux (openat2)
- Comprehensive path traversal protection
- TOCTOU race condition elimination
- Protection against memory exhaustion attacks
- Secure file type validation

### 4. Privilege Management

#### Purpose
Enable controlled privilege escalation for specific operations while maintaining the principle of least privilege and providing comprehensive audit trails.

#### Implementation Details

**Unix Privilege Architecture**:
```go
// Location: internal/runner/privilege/unix.go:18-25
type UnixPrivilegeManager struct {
    logger             *slog.Logger
    originalUID        int
    privilegeSupported bool
    metrics            Metrics
    mu                 sync.Mutex  // Prevents race conditions
}
```

**Privilege Escalation Process**:
```go
// Location: internal/runner/privilege/unix.go:36-87
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) error {
    m.mu.Lock()  // Global lock for thread safety
    defer m.mu.Unlock()

    // 1. Escalate privileges
    if err := m.escalatePrivileges(elevationCtx); err != nil {
        return err
    }

    // 2. Execute operation with defer-based cleanup
    defer func() {
        if err := m.restorePrivileges(); err != nil {
            m.emergencyShutdown(err, shutdownContext) // Terminate on failure
        }
    }()

    return fn()
}
```

**Execution Modes**:

1. **Native Root Execution**: Running as root user (UID 0)
   - No privilege escalation needed
   - Direct execution with full privileges

2. **Setuid Binary Execution**: Binary with setuid bit set and root ownership
   - Uses `syscall.Seteuid(0)` for privilege escalation
   - Automatic privilege restoration after operation

**Security Validation**:
```go
// Location: internal/runner/privilege/unix.go:232-294
func isRootOwnedSetuidBinary(logger *slog.Logger) bool {
    // Validate setuid bit is set
    hasSetuidBit := fileInfo.Mode()&os.ModeSetuid != 0

    // Validate root ownership (essential for setuid to work)
    isOwnedByRoot := stat.Uid == 0

    // Validate non-root real UID (true setuid scenario)
    isValidSetuid := hasSetuidBit && isOwnedByRoot && originalUID != 0

    return isValidSetuid
}
```

**Emergency Shutdown Protocol**:
- Immediate process termination on privilege restoration failure
- Multi-channel logging (structured log, syslog, stderr)
- Security event recording with full context
- Prevention of continued execution in compromised state

#### Security Guarantees
- Thread-safe privilege operations with global mutex
- Automatic privilege restoration with panic protection
- Comprehensive audit logging of all privilege operations
- Emergency shutdown on security failures
- Support for both native root and setuid binary execution models

### 5. Command Path Validation

#### Purpose
Ensure that only authorized commands can be executed by validating command paths against configurable allowlists and preventing execution of dangerous binaries.

#### Implementation Details

**Path Resolution**:
```go
// Location: internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv            string
    securityValidator  *security.Validator
    skipStandardPaths  bool
}
```

**Command Validation Process**:
1. Resolve command to full path using PATH environment variable
2. Validate against allowlist patterns (regex-based)
3. Check for dangerous privileged commands
4. Verify file integrity if hash is available

**Default Allowed Patterns**:
```go
// Location: internal/runner/security/security.go:128-135
AllowedCommands: []string{
    "^/bin/.*",
    "^/usr/bin/.*",
    "^/usr/sbin/.*",
    "^/usr/local/bin/.*",
},
```

**Dangerous Command Detection**:
- Shell executables: `/bin/bash`, `/bin/sh`
- Privilege escalation tools: `sudo`, `su`, `doas`
- System administration: `rm`, `dd`, `mount`, `umount`
- Package management: `apt`, `yum`, `dnf`
- Service management: `systemctl`, `service`

#### Security Guarantees
- Allowlist-based command execution
- Prevention of arbitrary command execution
- Detection of dangerous privileged operations
- Path resolution security validation

### 6. Configuration Security

#### Purpose
Ensure that configuration files and the overall system configuration cannot be tampered with and follow security best practices.

#### Implementation Details

**File Permission Validation**:
```go
// Location: internal/runner/security/security.go:345-383
func (v *Validator) ValidateFilePermissions(filePath string) error {
    // Check for world-writable files
    disallowedBits := perm &^ requiredPerms
    if disallowedBits != 0 {
        return ErrInvalidFilePermissions
    }
    return nil
}
```

**Directory Security Validation**:
- Complete path traversal from root to target
- Symlink detection in path components
- World-writable directory detection
- Group-writable restrictions (root ownership required)

**Configuration Integrity**:
- TOML format validation
- Required field validation
- Type safety enforcement
- Cross-reference validation between sections

#### Security Guarantees
- Prevention of configuration tampering
- Secure file and directory permissions
- Path traversal attack prevention
- Configuration format validation

## Security Architecture Patterns

### Defense in Depth

The system implements multiple security layers:

1. **Input Validation**: All inputs validated at entry points
2. **Path Security**: Comprehensive path validation and symlink protection
3. **File Integrity**: Hash-based verification of all critical files
4. **Privilege Control**: Minimal privilege principle with controlled escalation
5. **Environment Isolation**: Strict allowlist-based environment filtering
6. **Command Validation**: Allowlist-based command execution control

### Zero Trust Model

- No implicit trust in system environment
- All files verified before use
- Environment variables filtered by allowlist
- Commands validated against known-good patterns
- Privileges granted only when necessary and immediately revoked

### Fail-Safe Design

- Default deny for all operations
- Emergency shutdown on security failures
- Comprehensive error handling and logging
- Graceful degradation when security features unavailable

### Audit and Monitoring

- Structured logging with security context
- Privilege operation metrics and tracking
- Security event recording
- Multi-channel critical error reporting

## Threat Model and Mitigations

### File System Attacks

**Threats**:
- Symlink attacks
- Path traversal
- TOCTOU race conditions
- File tampering

**Mitigations**:
- openat2 with RESOLVE_NO_SYMLINKS
- Step-by-step path validation
- SHA-256 hash verification
- Atomic file operations

### Privilege Escalation

**Threats**:
- Unauthorized privilege gain
- Privilege persistence
- Race conditions in privilege handling

**Mitigations**:
- Controlled privilege escalation
- Automatic privilege restoration
- Thread-safe operations
- Emergency shutdown on failures

### Environment Manipulation

**Threats**:
- Command injection via environment variables
- Information leakage through environment
- Privilege escalation via LD_PRELOAD, etc.

**Mitigations**:
- Strict allowlist-based filtering
- Dangerous pattern detection
- Group-level environment isolation
- Variable name and value validation

### Command Injection

**Threats**:
- Arbitrary command execution
- Shell metacharacter exploitation
- PATH manipulation

**Mitigations**:
- Allowlist-based command validation
- Full path resolution
- Shell metacharacter detection
- Command path verification

## Performance Considerations

### Hash Computation
- Efficient streaming hash calculation
- File size limits prevent resource exhaustion
- Caching mechanisms for repeated verifications

### Environment Processing
- O(1) allowlist lookups using map structures
- Compiled regex patterns for pattern matching
- Minimal string operations

### Privilege Operations
- Global mutex prevents race conditions but serializes privileged operations
- Fast privilege escalation/restoration using system calls
- Metrics collection for performance monitoring

## Deployment Security

### Binary Distribution
- Setuid bit must be set on binary for privilege escalation
- Root ownership required for setuid functionality
- Binary integrity should be verified before deployment

### Configuration Management
- Hash directory must have secure permissions (755 or stricter)
- Configuration files should be write-protected
- Regular integrity verification of critical files

### Monitoring and Alerting
- Structured logs for security events
- Syslog integration for centralized logging
- Emergency shutdown events require immediate attention

## Conclusion

The Go Safe Command Runner provides a comprehensive security framework for safe command execution with privilege delegation. The multi-layered approach combines modern security primitives (openat2) with proven security principles (defense in depth, zero trust, fail-safe design) to create a robust system suitable for production use in security-conscious environments.

The implementation demonstrates security engineering best practices including comprehensive input validation, secure privilege management, and extensive audit capabilities. The system is designed to fail securely and provide complete visibility into security-relevant operations.
