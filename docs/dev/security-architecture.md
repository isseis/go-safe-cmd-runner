# Go Safe Command Runner - Security Architecture Technical Documentation

## Overview

This document provides a comprehensive technical analysis of security measures implemented in the Go Safe Command Runner project. It is intended for software engineers and security professionals who need to understand the design principles, implementation details, and security guarantees of the system.

## Executive Summary

Go Safe Command Runner implements multiple layers of security controls to enable safe delegation of privileged operations and automated batch processing. The security model is built on defense-in-depth principles, combining file integrity verification, environment variable isolation, privilege management, and safe file operations.

## Key Security Features

### 1. File Integrity Verification

#### Purpose
Ensures that executables and critical files are not tampered with before execution, preventing execution of compromised binaries. The system now provides centralized verification management through the `internal/verification/` package.

#### Implementation Details

**Hash Algorithm**: SHA-256 cryptographic hash
- Location: `internal/filevalidator/hash_algo.go`
- Uses Go standard `crypto/sha256` library
- Provides 256-bit hash values for strong collision resistance

**Hash Storage System**:
- Hash files stored as JSON manifests in dedicated directory
- File paths encoded using Base64 URL-safe encoding to handle special characters
- Manifest format includes file path, hash value, algorithm, timestamp
- Collision detection prevents different file paths from mapping to the same hash manifest file when path hashes collide

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

**Centralized Verification Management**:
- Location: `internal/verification/manager.go`
- Unified interface for all file verification operations
- Automatic privilege escalation fallback for permission-restricted files
- Standard system path skipping capability

**Privileged File Access**:
- Falls back to privilege escalation if normal verification fails due to permissions
- Uses safe privilege management (see Privilege Management section)
- Location: `internal/filevalidator/privileged_file.go`

#### Security Guarantees
- Detects unauthorized modifications to executables and config files
- Prevents execution of tampered binaries
- Cryptographically strong hash algorithm (SHA-256)
- Atomic file operations prevent race conditions

### 2. Environment Variable Isolation

#### Purpose
Implements strict allowlist-based filtering of environment variables to prevent information disclosure and command injection attacks through environment manipulation.

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
2. **Group Override**: Groups define their own allowlists, completely overriding global settings
3. **Inheritance Control**: Groups without explicit allowlists inherit global settings

**Inheritance Modes**:
- `InheritanceModeInherit`: Use global allowlist
- `InheritanceModeExplicit`: Use only group-specific allowlist
- `InheritanceModeReject`: Allow no environment variables (empty allowlist)

**Variable Validation**:
```go
// Location: internal/runner/config/validator.go
func (v *Validator) validateVariableValue(value string) error {
    // Use centralized security validation
    if err := security.IsVariableValueSafe(value); err != nil {
        // Wrap security error with validation error type for consistency
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
- Variable name and value validation against dangerous patterns

### 3. Safe File Operations

#### Purpose
Provides symlink-safe file I/O operations to prevent symlink attacks, TOCTOU (Time-of-Check-Time-of-Use) race conditions, and path traversal attacks.

#### Implementation Details

**Modern Linux Security (openat2)**:
```go
// Location: internal/safefileio/safe_file.go:99-122
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    // Use RESOLVE_NO_SYMLINKS flag to atomically prevent symlink following
    pathBytes, err := syscall.BytePtrFromString(pathname)
    fd, _, errno := syscall.Syscall6(SysOpenat2, ...)
    return int(fd), nil
}
```

**Fallback Security (Legacy Systems)**:
```go
// Location: internal/safefileio/safe_file.go:409-433
func ensureParentDirsNoSymlinks(absPath string) error {
    // Step-by-step path validation from root to target
    for _, component := range components {
        fi, err := os.Lstat(currentPath) // Don't follow symlinks
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
- Disallows device files, pipes, special files

#### Security Guarantees
- Atomic symlink-safe operations on modern Linux (openat2)
- Comprehensive path traversal protection
- Eliminates TOCTOU race conditions
- Protection against memory exhaustion attacks
- Safe file type validation

### 4. Privilege Management

#### Purpose
Enables controlled privilege escalation for specific operations while maintaining the principle of least privilege and providing comprehensive audit trails.

#### Implementation Details

**Unix Privilege Architecture**:
```go
// Location: internal/runner/privilege/unix.go:18-25
type UnixPrivilegeManager struct {
    logger             *slog.Logger
    originalUID        int
    privilegeSupported bool
    metrics            Metrics
    mu                 sync.Mutex  // Prevent race conditions
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
            m.emergencyShutdown(err, shutdownContext) // Exit on failure
        }
    }()

    return fn()
}
```

**Execution Modes**:

1. **Native Root Execution**: Running as root user (UID 0)
   - No privilege escalation needed
   - Direct execution with full privileges

2. **Setuid Binary Execution**: Binary with setuid bit and root ownership
   - Uses `syscall.Seteuid(0)` for privilege escalation
   - Automatic privilege restoration after operations

**Security Validation**:
```go
// Location: internal/runner/privilege/unix.go:232-294
func isRootOwnedSetuidBinary(logger *slog.Logger) bool {
    // Verify setuid bit is set
    hasSetuidBit := fileInfo.Mode()&os.ModeSetuid != 0

    // Verify root ownership (essential for setuid to work)
    isOwnedByRoot := stat.Uid == 0

    // Verify non-root real UID (true setuid scenario)
    isValidSetuid := hasSetuidBit && isOwnedByRoot && originalUID != 0

    return isValidSetuid
}
```

**Emergency Shutdown Protocol**:
- Immediate process termination on privilege restoration failure
- Multi-channel logging (structured logs, syslog, stderr)
- Security event recording with full context
- Prevents continued execution in compromised state

#### Security Guarantees
- Thread-safe privilege operations with global mutex
- Automatic privilege restoration with panic protection
- Comprehensive audit logging of all privilege operations
- Emergency shutdown on security failures
- Support for both native root and setuid binary execution models

### 5. Command Path Validation

#### Purpose
Validates command paths against configurable allowlists and prevents execution of dangerous binaries, ensuring only authorized commands can be executed. Stops environment variable inheritance and uses secure fixed PATH.

#### Implementation Details

**Secure PATH Environment Enforcement**:
```go
// Location: internal/verification/manager.go
const securePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin"

// Use secure fixed PATH without inheriting environment variable PATH
pathResolver := NewPathResolver(securePathEnv, securityValidator, false)
```

**Path Resolution**:
```go
// Location: internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv            string    // Use secure fixed PATH
    securityValidator  *security.Validator
    skipStandardPaths  bool
}
```

**Command Validation Process**:
1. Resolve command to full path using PATH environment variable
2. Validate against allowlist patterns (regex-based)
3. Check for dangerous privilege commands
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
- Prevents arbitrary command execution
- Detection of dangerous privilege operations
- Path resolution security validation
- Complete elimination of environment variable PATH inheritance
- Enforced use of secure fixed PATH (/sbin:/usr/sbin:/bin:/usr/bin)

### 6. Risk-Based Command Control

#### Purpose
Implements intelligent security controls based on command risk assessment, automatically blocking high-risk operations while allowing safe commands to execute normally.

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
Provides safe resource management that maintains security boundaries in both normal execution and dry-run modes.

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
- Safe notification handling
- Resource lifecycle management

### 8. Secure Logging and Sensitive Data Protection

#### Purpose
Prevents sensitive information like passwords, API keys, and tokens from being exposed in log files, providing safe audit trails without compromising confidential data. Enhanced with dedicated redaction service.

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
    // IncludeErrorDetails controls whether full error messages are included in logs
    IncludeErrorDetails bool `json:"include_error_details"`

    // MaxErrorMessageLength limits the length of error messages in logs
    MaxErrorMessageLength int `json:"max_error_message_length"`

    // RedactSensitiveInfo enables automatic redaction of sensitive patterns
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

        // Common authentication credential patterns
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

    // If we shouldn't include error details, return generic message
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
- Sanitizes command output to prevent credential leakage
- Configurable output length truncation
- Automatic pattern-based redaction of sensitive information
- Supports both key=value format and authentication header patterns

**Safe Logging Functions**:
- `CreateSafeLogFields()`: Creates sanitized log field map
- `LogFieldsWithError()`: Combines base fields with sanitized error information
- Automatic detection and redaction of sensitive patterns in structured logs

#### Security Guarantees
- Automatic redaction of common sensitive patterns (passwords, tokens, API keys)
- Configurable log detail levels for different security environments
- Protection against credential exposure through error messages and command output
- Length-based truncation prevents log file bloat and potential DoS
- Environment variable pattern detection and sanitization

### 9. Terminal Capability Detection (`internal/terminal/`)

#### Purpose
Detects terminal color support and interactive execution environment to provide appropriate output formatting and terminal capability determination.

#### Implementation Details

**Terminal Capability Detection Interface**:
```go
// Location: internal/terminal/capabilities.go
type Capabilities interface {
    IsInteractive() bool
    SupportsColor() bool
    HasExplicitUserPreference() bool
}
```

**Interactive Environment Detection**:
```go
// Location: internal/terminal/detector.go
type InteractiveDetector interface {
    IsInteractive() bool
    IsTerminal() bool // Check for TTY environment or terminal-like environment
    IsCIEnvironment() bool
}
```

**Implementation Features**:
- **CI/CD Environment Detection**: Automatic detection of GitHub Actions, Travis CI, Jenkins, etc.
- **TTY Detection**: Check stdout/stderr TTY connection status
- **Terminal Environment Heuristics**: Terminal-like environment determination based on TERM environment variable
- **Color Support Detection**: Color-capable terminal identification based on TERM values
- **User Setting Priorities**: Priority control for command line arguments and environment variables

#### Security Characteristics
- **Conservative Defaults**: Disable color output for unknown terminals
- **Environment Variable Validation**: Proper parsing of CI environment variables
- **Configuration Priority Control**: Security-aware configuration inheritance

### 10. Color Management (`internal/color/`)

#### Purpose
Provides safe colored output based on terminal color support capabilities and proper management of color control sequences.

#### Implementation Details

**Color Management Interface**:
```go
// Location: internal/color/color.go
type ColorManager interface {
    Enable() bool
    Colorize(text string, color ColorCode) string
}
```

**Color Support Detection**:
```go
// Location: internal/terminal/color.go
type ColorDetector interface {
    SupportsColor() bool
}
```

**Implementation Features**:
- **Known Terminal Pattern Matching**: Identification of color-capable terminals like xterm, screen, tmux, etc.
- **Conservative Fallback**: Disable color output for unknown terminals
- **TERM Environment Variable Analysis**: Color support determination based on terminal type
- **User Setting Integration**: Priority control for terminal capabilities and user settings

#### Security Characteristics
- **Conservative Approach**: Disable color output for unknown terminals to prevent escape sequence output
- **Validated Patterns**: Enable colors only for known color-capable terminals
- **Safe Defaults**: Ensure safe behavior when color support is unknown

### 11. Common Utilities (`internal/common/`, `internal/cmdcommon/`)

#### Purpose
Provides cross-package foundational functionality ensuring testable and reproducible safe implementations.

#### Implementation Details

**Filesystem Abstraction**:
```go
// Location: internal/common/filesystem.go
type FileSystem interface {
    CreateTempDir(dir string, prefix string) (string, error)
    FileExists(path string) (bool, error)
    Lstat(path string) (fs.FileInfo, error)
    IsDir(path string) (bool, error)
}
```

**Mock Implementations**:
- Provides mock filesystems for testing with equivalent security characteristics to production
- Supports testing of error conditions and boundary cases

#### Security Guarantees
- Consistent security behavior across implementations
- Comprehensive test coverage of security paths
- Type-safe interface contracts
- Mock implementations preserve security properties

### 12. User and Group Execution Security

#### Purpose
Provides safe user and group switching capabilities while maintaining strict security boundaries and comprehensive audit trails.

#### Implementation Details

**User/Group Configuration**:
```go
// Location: internal/runner/runnertypes/config.go
type Command struct {
    RunAsUser    string `toml:"run_as_user"`    // User to run command as
    RunAsGroup   string `toml:"run_as_group"`   // Group to run command as
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
2. Check group membership if group is specified
3. Verify privilege escalation requirements
4. Apply risk-based restrictions
5. Execute command with appropriate privileges

#### Security Guarantees
- Comprehensive user and group validation
- Privilege escalation boundary enforcement
- Group membership verification
- Complete audit trail of user/group switching

### 13. Multi-Channel Notification Security

#### Purpose
Provides safe notification capabilities for critical security events while protecting sensitive information in external communications.

#### Implementation Details

**Slack Integration**:
```go
// Location: internal/logging/slack_handler.go
type SlackHandler struct {
    webhookURL string
    redactor   *redaction.Redactor
}
```

**Safe Notification Handling**:
- Automatic sensitive data redaction before sending
- Configurable notification channels
- Rate limiting and error handling
- Secure webhook URL management

#### Security Guarantees
- Sensitive data protection in external notifications
- Secure communication channel management
- Rate limiting to prevent abuse
- Comprehensive error handling

### 14. Configuration Security

#### Purpose
Ensures configuration files and overall system configuration are not tampered with and follow security best practices.

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

**Hash Directory Security Enhancement (Command Line Argument Removal)**:
```go
// Location: cmd/runner/main.go (after changes)
func getHashDir() string {
    // Always use only default directory in production environment
    // --hash-directory flag completely removed (security vulnerability countermeasure)
    return cmdcommon.DefaultHashDirectory
}
```

**Configuration File Pre-Validation**:
```go
// Location: cmd/runner/main.go (after changes)
// Execute hash verification before config file loading
if err := verificationManager.VerifyConfigFile(configPath); err != nil {
    // Completely eliminate system operation with unverified data
    return &logging.PreExecutionError{
        Type:      logging.ErrorTypeConfigValidation,
        Message:   fmt.Sprintf("Configuration file verification failed: %s", err),
        Component: "config",
        RunID:     runID,
    }
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
- Group write restrictions (root ownership required)

**Configuration Validation Timing Improvement**:
- Execute hash verification before config file loading
- Completely eliminate system operation with unverified data
- Forced stderr output on validation failure (independent of log level settings)

**Hash Directory Configuration Security Enhancement**:
- Complete removal of `--hash-directory` command line argument
- Always use only default directory in production environment
- Complete elimination of attack vectors through custom hash directories
- Maintain testability with test environment-specific APIs

**Configuration Integrity**:
- TOML format validation
- Required field validation
- Type safety enforcement
- Duplicate group name detection and environment variable inheritance analysis

#### Security Guarantees
- Prevention of configuration tampering
- Safe file and directory permissions
- Protection against path traversal attacks
- Configuration format validation
- Configuration file pre-validation for tampering detection
- Complete elimination of hash directory attack vectors
- Early validation enhancement through absolute path requirements

## Security Architecture Patterns

### Defense in Depth

The system implements multiple security layers:

1. **Input Validation**: All inputs validated at entry points (including absolute path requirements)
2. **Pre-Validation**: Configuration file hash verification before use
3. **Path Security**: Comprehensive path validation and symlink protection, secure fixed PATH usage
4. **File Integrity**: Hash-based verification of all critical files (config, executables)
5. **Privilege Control**: Principle of least privilege with controlled escalation
6. **Environment Isolation**: Strict allowlist-based environment filtering, elimination of PATH inheritance
7. **Command Validation**: Risk-based command execution control with allowlist validation
8. **Data Protection**: Automatic redaction of sensitive information in all output
9. **User/Group Security**: Safe user/group switching with membership validation
10. **Hash Directory Security**: Complete prevention of custom hash directory attacks

### Zero Trust Model

- No implicit trust in system environment
- All files verified before use
- Environment variables filtered by allowlist
- Commands validated against known good patterns
- Privileges granted only when needed and immediately revoked

### Fail-Safe Design

- Default deny for all operations
- Emergency shutdown on security failures
- Comprehensive error handling and logging
- Graceful degradation when security features unavailable

### Audit and Monitoring

- Structured logging with security context
- Privileged operation metrics and tracking
- Security event recording
- Multi-channel reporting for critical errors

## Threat Model and Mitigations

### Filesystem Attacks

**Threats**:
- Symlink attacks
- Path traversal
- TOCTOU race conditions
- File tampering
- Malicious configuration file system operation manipulation
- Custom hash directory verification bypass

**Mitigations**:
- openat2 with RESOLVE_NO_SYMLINKS
- Step-by-step path validation
- SHA-256 hash verification
- Atomic file operations
- Configuration file pre-hash verification
- Fixed default hash directory value (complete prohibition of custom specification)

### Privilege Escalation

**Threats**:
- Unauthorized privilege acquisition
- Privilege persistence
- Race conditions in privilege handling

**Mitigations**:
- Controlled privilege escalation
- Automatic privilege restoration
- Thread-safe operations
- Emergency shutdown on failure

### Environment Manipulation

**Threats**:
- Command injection via environment variables
- Information disclosure through environment
- Privilege escalation via LD_PRELOAD, etc.

**Mitigations**:
- Strict allowlist-based filtering
- Dangerous pattern detection
- Group-level environment isolation
- Variable name and value validation

### Command Injection

**Threats**:
- Arbitrary command execution
- Shell metacharacter abuse
- PATH manipulation
- Privilege escalation through command manipulation
- Malicious binary execution via environment variable PATH

**Mitigations**:
- Risk-based command validation with allowlist enforcement
- Full path resolution with security validation
- Shell metacharacter detection
- Command path validation
- Risk level enforcement and blocking
- User/group execution validation
- Complete elimination of environment variable PATH inheritance
- Enforced use of secure fixed PATH (/sbin:/usr/sbin:/bin:/usr/bin)

## Performance Considerations

### Hash Calculation
- Efficient streaming hash calculation
- File size limits to prevent resource exhaustion

### Environment Processing
- O(1) allowlist lookups using map structures
- Compiled regular expressions for pattern matching
- Minimal string manipulations

### Privileged Operations
- Global mutex prevents race conditions but serializes privileged operations
- Fast privilege escalation/restoration using system calls
- Metrics collection for performance monitoring

### Risk Assessment
- Pre-compiled regular expression patterns for efficient command analysis
- O(1) risk level lookups using pre-compiled patterns
- Minimal overhead for risk assessment
- Result caching for repeated command analysis

### Data Redaction
- Streaming redaction for large output
- Pre-compiled patterns for sensitive data
- Minimal performance impact on normal operations
- Configurable redaction policies

## Deployment Security

### Binary Distribution
- Binary must have setuid bit set for privilege escalation
- Root ownership required for setuid functionality
- Binary integrity should be verified before deployment

### Configuration Management
- Hash directory must have secure permissions (755 or more restrictive)
- Configuration files should be write-protected
- Regular integrity verification of critical files

### Monitoring and Alerting
- Structured logging for security events
- Syslog integration for centralized logging
- Emergency shutdown events require immediate attention
- Slack integration for real-time security alerts
- Automatic sensitive data redaction across all monitoring channels

## Conclusion

Go Safe Command Runner provides a comprehensive security framework for safe command execution with privilege delegation. The multi-layered approach combines modern security primitives (openat2) with proven security principles (defense-in-depth, zero-trust, fail-safe design) to create a robust system suitable for production use in security-conscious environments.

The implementation demonstrates security engineering best practices including comprehensive input validation, risk-based command controls, safe privilege management, automatic sensitive data protection, and extensive audit capabilities. The system is designed to fail safely and provide complete visibility into security-related operations.

Key security innovation features include intelligent risk assessment for command execution, unified resource management with consistent security boundaries, automatic sensitive data redaction across all channels, safe user/group execution capabilities, and comprehensive multi-channel notifications with security-aware messaging. The system provides enterprise-grade security controls while maintaining operational flexibility and transparency.
