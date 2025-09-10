# Go Safe Command Runner - Security Architecture Documentation

## Overview

This document provides a comprehensive technical analysis of the security measures implemented in the Go Safe Command Runner project. It is intended for software engineers and security professionals who need to understand the design principles, implementation details, and security guarantees of the system.

## Executive Summary

The Go Safe Command Runner implements multiple layers of security controls to enable safe delegation of privileged operations and automated batch processing. The security model is built around the principle of defense-in-depth, combining file integrity verification, environment variable isolation, privilege management, and secure file operations.

## Core Security Features

### 1. File Integrity Verification

#### Purpose
Ensure that executables and critical files have not been tampered with before execution, preventing the execution of compromised binaries. The system now provides centralized verification management through the `internal/verification/` package.

#### Implementation Details

**Hash Algorithm**: SHA-256 cryptographic hashing
- Location: `internal/filevalidator/hash_algo.go`
- Uses Go's standard `crypto/sha256` library
- Provides 256-bit hash values for strong collision resistance

**Hash Storage System**:
- Hash files stored as JSON manifests in dedicated directory
- File path encoded using Base64 URL-safe encoding to handle special characters
- Manifest format includes file path, hash value, algorithm, and timestamp
- Collision detection prevents two different file paths from mapping to the same hash manifest file, which could happen if their path hashes collide.
- Environment file verification support - integrity validation of `.env` files before execution

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

**Environment File Verification**:
```go
// Location: internal/verification/manager.go:153-185
func (m *Manager) VerifyEnvironmentFile(envFilePath string) error {
    // Validate hash directory
    if err := m.ValidateHashDirectory(); err != nil {
        return &Error{Op: "ValidateHashDirectory", Path: m.hashDir, Err: err}
    }

    // Verify file with privilege fallback
    if err := m.verifyFileWithFallback(envFilePath); err != nil {
        return &Error{Op: "VerifyHash", Path: envFilePath, Err: err}
    }
    return nil
}
```

**Centralized Verification Management**:
- Location: `internal/verification/manager.go`
- Unified interface for all file verification operations
- Automatic privilege escalation fallback for permission-restricted files
- Standard system path skip functionality

**Privileged File Access**:
- Falls back to privilege escalation when normal verification fails due to permissions
- Uses secure privilege management (see Privilege Management section)
- Location: `internal/filevalidator/privileged_file.go`

#### Security Guarantees
- Detects unauthorized modifications to executables, configuration files, and environment files
- Prevents execution of tampered binaries
- Cryptographically strong hash algorithm (SHA-256)
- Atomic file operations prevent race conditions
- Environment file integrity verification prevents configuration tampering

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
// Location: internal/runner/config/validator.go
func (v *Validator) validateVariableValue(value string) error {
    // Use centralized security validation
    if err := security.IsVariableValueSafe(value); err != nil {
        // Wrap the security error with our validation error type for consistency
        return fmt.Errorf("%w: %s", ErrDangerousPattern, err.Error())
    }
    return nil
}
```

**Dangerous Pattern Detection**:
- Command separators: `;`, `|`, `&&`, `||`
- Command substitution: `$(...)`, backticks
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
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
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
// Location: internal/runner/security/types.go:147-154
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

### 6. Risk-Based Command Control

#### Purpose
Implement intelligent security controls based on command risk assessment, automatically blocking high-risk operations while allowing safe commands to execute normally.

#### Implementation Details

**Risk Assessment Engine**:
```go
// Location: internal/runner/risk/evaluator.go
type StandardEvaluator struct{}

func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error) {
    // Check for privilege escalation commands (critical risk - should be blocked)
    isPrivEsc, err := security.IsPrivilegeEscalationCommand(cmd.Cmd)
    if err != nil {
        return runnertypes.RiskLevelUnknown, err
    }
    if isPrivEsc {
        return runnertypes.RiskLevelCritical, nil
    }
    // ... additional risk assessment logic
}
```

**Command Risk Analysis**:
- Low Risk: Standard system utilities (ls, cat, grep)
- Medium Risk: File modification commands (cp, mv, chmod), package management (apt, yum)
- High Risk: System administration commands (mount, systemctl), destructive operations (rm -rf)
- Critical Risk: Privilege escalation commands (sudo, su) - automatically blocked

**Risk Level Configuration**:
```go
// Location: internal/runner/runnertypes/config.go
type Command struct {
    MaxRiskLevel string `toml:"max_risk_level"` // Maximum allowed risk level
}
```

#### Security Guarantees
- Automatic blocking of privilege escalation attempts
- Configurable risk thresholds per command
- Comprehensive command pattern matching
- Risk-based audit logging

### 7. Resource Management Security

#### Purpose
Provide secure resource management that maintains security boundaries across both normal execution and dry-run modes.

#### Implementation Details

**Unified Resource Interface**:
```go
// Location: internal/runner/resource/manager.go
type ResourceManager interface {
    ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error)
    WithPrivileges(ctx context.Context, fn func() error) error
    SendNotification(message string, details map[string]any) error
}
```

**Execution Mode Security**:
- Normal mode: Full privilege management and command execution
- Dry-run mode: Security analysis without actual execution
- Consistent security validation across both modes

#### Security Guarantees
- Mode-independent security validation
- Privilege boundary enforcement
- Secure notification handling
- Resource lifecycle management

### 8. Secure Logging and Sensitive Data Protection

#### Purpose
Prevent sensitive information such as passwords, API keys, and tokens from being exposed in log files, providing secure audit trails without compromising confidential data. Enhanced with dedicated redaction services.

#### Implementation Details

**Centralized Data Redaction**:
```go
// Location: internal/redaction/redactor.go
type Redactor struct {
    patterns []SensitivePattern
}

func (r *Redactor) RedactText(text string) string {
    // Apply all configured redaction patterns
}
```

**Logging Security Configuration**:
```go
// Location: internal/runner/security/types.go:92-107
type LoggingOptions struct {
    // IncludeErrorDetails controls whether full error messages are logged
    IncludeErrorDetails bool `json:"include_error_details"`

    // MaxErrorMessageLength limits the length of error messages in logs
    MaxErrorMessageLength int `json:"max_error_message_length"`

    // RedactSensitiveInfo enables automatic redaction of potentially sensitive patterns
    RedactSensitiveInfo bool `json:"redact_sensitive_info"`

    // TruncateStdout controls whether stdout is truncated in error logs
    TruncateStdout bool `json:"truncate_stdout"`

    // MaxStdoutLength limits the length of stdout in error logs
    MaxStdoutLength int `json:"max_stdout_length"`
}
```

**Sensitive Pattern Detection and Redaction**:
```go
// Location: internal/runner/security/logging_security.go:49-52
func (v *Validator) redactSensitivePatterns(text string) string {
    sensitivePatterns := []struct {
        pattern     string
        replacement string
    }{
        // API keys, tokens, passwords (common patterns)
        {"password=", "password=[REDACTED]"},
        {"token=", "token=[REDACTED]"},
        {"key=", "key=[REDACTED]"},
        {"secret=", "secret=[REDACTED]"},
        {"api_key=", "api_key=[REDACTED]"},

        // Environment variable assignments that might contain secrets
        {"_PASSWORD=", "_PASSWORD=[REDACTED]"},
        {"_TOKEN=", "_TOKEN=[REDACTED]"},
        {"_KEY=", "_KEY=[REDACTED]"},
        {"_SECRET=", "_SECRET=[REDACTED]"},

        // Common credential patterns
        {"Bearer ", "Bearer [REDACTED]"},
        {"Basic ", "Basic [REDACTED]"},
    }
    // Pattern matching and replacement logic
}
```

**Error Message Sanitization**:
```go
// Location: internal/runner/security/logging_security.go:4-26
func (v *Validator) SanitizeErrorForLogging(err error) string {
    if err == nil {
        return ""
    }

    errMsg := err.Error()

    // If error details should not be included, return a generic message
    if !v.config.LoggingOptions.IncludeErrorDetails {
        return "[error details redacted for security]"
    }

    // Redact sensitive information if enabled
    if v.config.LoggingOptions.RedactSensitiveInfo {
        errMsg = v.redactSensitivePatterns(errMsg)
    }

    // Truncate if too long
    if len(errMsg) > v.config.LoggingOptions.MaxErrorMessageLength {
        errMsg = errMsg[:v.config.LoggingOptions.MaxErrorMessageLength] + "...[truncated]"
    }

    return errMsg
}
```

**Output Sanitization**:
- Command output sanitization to prevent credential leakage
- Configurable output length truncation
- Automatic pattern-based redaction of sensitive information
- Support for both key=value and Authorization header patterns

**Safe Logging Functions**:
- `CreateSafeLogFields()`: Creates sanitized log field maps
- `LogFieldsWithError()`: Combines base fields with sanitized error information
- Automatic detection and redaction of sensitive patterns in structured logs

#### Security Guarantees
- Automatic redaction of common sensitive patterns (passwords, tokens, API keys)
- Configurable log detail levels for different security environments
- Protection against credential exposure through error messages and command output
- Length-based truncation to prevent log file bloat and potential DoS
- Environment variable pattern detection and sanitization

### 9. User and Group Execution Security

#### Purpose
Provide secure user and group switching capabilities while maintaining strict security boundaries and comprehensive audit trails.

#### Implementation Details

**User/Group Configuration**:
```go
// Location: internal/runner/runnertypes/config.go
type Command struct {
    RunAsUser    string `toml:"run_as_user"`    // User to execute command as
    RunAsGroup   string `toml:"run_as_group"`   // Group to execute command as
    MaxRiskLevel string `toml:"max_risk_level"` // Maximum allowed risk level
}
```

**Group Membership Validation**:
```go
// Location: internal/groupmembership/membership.go
type GroupMembershipChecker interface {
    IsUserInGroup(username, groupname string) (bool, error)
    GetGroupMembers(groupname string) ([]string, error)
}
```

**Security Validation Flow**:
1. Validate user existence and permissions
2. Verify group membership if group is specified
3. Check privilege escalation requirements
4. Apply risk-based restrictions
5. Execute with appropriate privileges

#### Security Guarantees
- Comprehensive user and group validation
- Privilege escalation boundary enforcement
- Group membership verification
- Full audit trail for user/group switches

### 10. Multi-Channel Notification Security

#### Purpose
Provide secure notification capabilities for critical security events while protecting sensitive information in external communications.

#### Implementation Details

**Slack Integration**:
```go
// Location: internal/logging/slack_handler.go
type SlackHandler struct {
    webhookURL string
    redactor   *redaction.Redactor
}
```

**Secure Notification Processing**:
- Automatic redaction of sensitive data before transmission
- Configurable notification channels
- Rate limiting and error handling
- Secure webhook URL management

#### Security Guarantees
- Sensitive data protection in external notifications
- Secure communication channel management
- Rate limiting prevents abuse
- Comprehensive error handling

### 11. Terminal Capability Detection (`internal/terminal/`)

#### Purpose
Provide terminal capability detection for color support and interactive execution environments.

#### Implementation Details

**Terminal Capability Detection**:
```go
// Location: internal/terminal/capabilities.go
type Capabilities interface {
    IsInteractive() bool
    SupportsColor() bool
    HasExplicitUserPreference() bool
}
```

**Interactive Environment Detection**:
- CI/CD environment automatic detection for proper output control
- TTY detection for stdout/stderr connections
- Terminal environment heuristics via TERM environment variable
- Conservative defaults for unknown terminals

#### Security Guarantees
- Conservative approach prevents escape sequence output on unknown terminals
- Safe terminal capability detection
- Consistent cross-platform behavior

### 12. Color Management (`internal/color/`)

#### Purpose
Provide secure color output capabilities based on terminal color support capabilities.

#### Implementation Details

**Color Support Detection**:
```go
// Location: internal/terminal/color.go
type ColorDetector interface {
    SupportsColor() bool
}
```

**Color Output Control**:
- Known terminal pattern matching for color-capable terminals
- Conservative fallback disabling color output for unknown terminals
- TERM environment variable parsing for terminal type determination
- User preference priority control integrated with terminal capabilities

#### Security Guarantees
- Conservative approach prevents escape sequence output on unknown terminals
- Validated control only on known color-supporting terminals
- Safe output control based on terminal capabilities

### 13. Common Utilities (`internal/common/`, `internal/cmdcommon/`)

#### Purpose
Provide secure foundational interfaces and utilities with comprehensive testing support.

#### Implementation Details

**File System Abstraction**:
```go
// Location: internal/common/filesystem.go
type FileSystem interface {
    CreateTempDir(dir string, prefix string) (string, error)
    FileExists(path string) (bool, error)
    Lstat(path string) (fs.FileInfo, error)
    IsDir(path string) (bool, error)
}
```

**Mock Implementation Security**:
- Comprehensive mock file system for testing
- Consistent security behavior in test and production
- Type-safe interface implementations
- Error condition testing support

#### Security Guarantees
- Consistent security behavior across implementations
- Comprehensive test coverage for security paths
- Type-safe interface contracts
- Mock implementations maintain security properties

### 14. Configuration Security

#### Purpose
Ensure that configuration files and the overall system configuration cannot be tampered with and follow security best practices.

#### Implementation Details

**File Permission Validation**:
```go
// Location: internal/runner/security/file_validation.go:44-75
func (v *Validator) ValidateFilePermissions(filePath string) error {
    // Check for world-writable files
    disallowedBits := perm &^ requiredPerms
    if disallowedBits != 0 {
        return ErrInvalidFilePermissions
    }
    return nil
}
```

**Configuration Simplification (Environment Variable Fallback Removal)**:
```go
// Location: cmd/runner/main.go:61-68 (after change)
func getHashDir() string {
    // Command line arguments take precedence
    if *hashDirectory != "" {
        return *hashDirectory
    }
    // Set default hash directory (environment variable fallback removed)
    return cmdcommon.DefaultHashDirectory
}
```

**Early Path Validation**:
```go
// Location: cmd/runner/main.go:188-199
hashDir := getHashDir()
if !filepath.IsAbs(hashDir) {
    return &logging.PreExecutionError{
        Type:      logging.ErrorTypeFileAccess,
        Message:   fmt.Sprintf("Hash directory must be absolute path, got relative path: %s", hashDir),
        Component: "file",
        RunID:     runID,
    }
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
- Reduced attack surface through environment variable fallback removal
- Enhanced early validation with absolute path requirements

## Security Architecture Patterns

### Defense in Depth

The system implements multiple security layers:

1. **Input Validation**: All inputs validated at entry points (including absolute path requirements)
2. **Path Security**: Comprehensive path validation and symlink protection
3. **File Integrity**: Hash-based verification of all critical files (configuration, environment files, executables)
4. **Privilege Control**: Minimal privilege principle with controlled escalation
5. **Environment Isolation**: Strict allowlist-based environment filtering
6. **Command Validation**: Risk-based command execution control with allowlist validation
7. **Data Protection**: Automatic redaction of sensitive information across all outputs
8. **User/Group Security**: Secure user and group switching with membership validation

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
- Privilege escalation through command manipulation

**Mitigations**:
- Risk-based command validation with allowlist enforcement
- Full path resolution with security validation
- Shell metacharacter detection
- Command path verification
- Risk level enforcement and blocking
- User/group execution validation

## Performance Considerations

### Hash Computation
- Efficient streaming hash calculation
- File size limits prevent resource exhaustion

### Environment Processing
- O(1) allowlist lookups using map structures
- Compiled regex patterns for pattern matching
- Minimal string operations

### Privilege Operations
- Global mutex prevents race conditions but serializes privileged operations
- Fast privilege escalation/restoration using system calls
- Metrics collection for performance monitoring

### Risk Assessment
- Pre-compiled regex patterns for efficient command analysis
- O(1) risk level lookups using pre-compiled pattern matching
- Minimal overhead for risk evaluation
- Cached results for repeated command analysis

### Data Redaction
- Streaming redaction for large outputs
- Pre-compiled sensitive data patterns
- Minimal performance impact on normal operations
- Configurable redaction policies

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
- Slack integration for real-time security alerts
- Automatic sensitive data redaction in all monitoring channels

## Conclusion

The Go Safe Command Runner provides a comprehensive security framework for safe command execution with privilege delegation. The multi-layered approach combines modern security primitives (openat2) with proven security principles (defense in depth, zero trust, fail-safe design) to create a robust system suitable for production use in security-conscious environments.

The implementation demonstrates security engineering best practices including comprehensive input validation, risk-based command control, secure privilege management, automatic sensitive data protection, and extensive audit capabilities. The system is designed to fail securely and provide complete visibility into security-relevant operations.

Key security innovations include intelligent risk assessment for command execution, unified resource management with consistent security boundaries, automatic sensitive data redaction across all channels, secure user/group execution capabilities, and comprehensive multi-channel notification with security-aware messaging. The system provides enterprise-grade security controls while maintaining operational flexibility and transparency.
