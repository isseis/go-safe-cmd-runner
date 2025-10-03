# Go Safe Command Runner - Security Architecture Technical Document

## Overview

This document provides a comprehensive technical analysis of the security measures implemented in the Go Safe Command Runner project. It is intended for software engineers and security professionals who need to understand the design principles, implementation details, and security guarantees of the system.

## Executive Summary

Go Safe Command Runner implements multiple layers of security controls to enable secure delegation of privileged operations and automated batch processing. The security model is built on defense-in-depth principles, combining file integrity verification, environment variable isolation, privilege management, and secure file operations.

## Key Security Features

### 1. File Integrity Verification

#### Purpose
Verify that executables and critical files have not been tampered with before execution, preventing the execution of compromised binaries. The system now provides centralized verification management via the `internal/verification/` package.

#### Implementation Details

**Hash Algorithm**: SHA-256 cryptographic hash
- Location: `internal/filevalidator/hash_algo.go`
- Uses Go's standard `crypto/sha256` library
- Provides 256-bit hash values for strong collision resistance

**Hash Storage System**:
- Hash files are stored as JSON manifests in a dedicated directory
- File paths are encoded using Base64 URL-safe encoding to handle special characters
- Manifest format includes file path, hash value, algorithm, and timestamp
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
- Uses secure privilege management (see Privilege Management section)
- Location: `internal/filevalidator/privileged_file.go`

#### Security Guarantees
- Detects unauthorized modifications to executables and configuration files
- Prevents execution of tampered binaries
- Cryptographically strong hash algorithm (SHA-256)
- Atomic file operations to prevent race conditions

### 2. Environment Variable Isolation

#### Purpose
Implements strict allowlist-based filtering of environment variables to prevent information leakage and command injection attacks via environment manipulation.

#### Implementation Details

**Allowlist Architecture**:
```go
// Location: internal/runner/environment/filter.go:31-50
type Filter struct {
    config          *runnertypes.Config
    globalAllowlist map[string]bool // O(1) lookup performance
}
```

**3-Level Inheritance Model**:

1. **Global Allowlist**: Base environment variables available to all groups
2. **Group Override**: Groups can define their own allowlist, completely overriding global settings
3. **Inheritance Control**: Groups without explicit allowlist inherit global settings

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
- Validation of variable names and values against dangerous patterns

### 3. Secure File Operations

#### Purpose
Provides symlink-safe file I/O operations to prevent symlink attacks, TOCTOU (Time-of-Check-Time-of-Use) race conditions, and path traversal attacks.

#### Implementation Details

**Modern Linux Security (openat2)**:
```go
// Location: internal/safefileio/safe_file.go:99-122
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    // Atomically prevent symlink following with RESOLVE_NO_SYMLINKS flag
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
- Disallows device files, pipes, and special files

#### Security Guarantees
- Atomic symlink-safe operations on modern Linux (openat2)
- Comprehensive path traversal protection
- Elimination of TOCTOU race conditions
- Protection against memory exhaustion attacks
- Safe file type validation

### 4. Privilege Management

#### Purpose
Enables controlled privilege escalation for specific operations while maintaining the principle of least privilege, with comprehensive audit trail.

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
Validate command paths against configurable allowlists to ensure only authorized commands can be executed, preventing the execution of dangerous binaries. Stops environment variable inheritance and uses a secure fixed PATH.

#### Implementation Details

**Secure PATH Environment Enforcement**:
```go
// Location: internal/verification/manager.go
const securePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin"

// Do not inherit environment variable PATH, use secure fixed PATH
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
3. Check for dangerous privileged commands
4. Verify file integrity if hash is available

**Default Allow Patterns**:
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
Provides secure resource management that maintains security boundaries in both normal execution and dry-run modes.

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
- Normal Mode: Full privilege management and command execution
- Dry-run Mode: Security analysis without actual execution
- Consistent security validation across both modes

#### Security Guarantees
- Mode-independent security validation
- Privilege boundary enforcement
- Safe notification handling
- Resource lifecycle management

### 8. Secure Logging and Sensitive Data Protection

#### Purpose
Prevents sensitive information such as passwords, API keys, and tokens from being exposed in log files, providing a safe audit trail without compromising sensitive data. Enhanced by dedicated redaction service.

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

**Log Security Configuration**:
```go
// Location: internal/runner/security/types.go:92-107
type LoggingOptions struct {
    // IncludeErrorDetails controls whether to include full error messages in logs
    IncludeErrorDetails bool `json:"include_error_details"`

    // MaxErrorMessageLength limits the length of error messages in logs
    MaxErrorMessageLength int `json:"max_error_message_length"`

    // RedactSensitiveInfo enables automatic redaction of sensitive patterns
    RedactSensitiveInfo bool `json:"redact_sensitive_info"`

    // TruncateStdout controls whether to truncate stdout in error logs
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

    // If we shouldn't include error details, return a generic message
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
- `CreateSafeLogFields()`: Creates sanitized log field maps
- `LogFieldsWithError()`: Combines base fields with sanitized error information
- Automatic detection and redaction of sensitive patterns in structured logs

#### Security Guarante
- Automatic redaction of common sensitive patterns (passwords, tokens, API keys)
- Configurable log detail levels for different security environments
- Protection from credential exposure via error messages and command output
- Length-based truncation to prevent log file bloat and potential DoS
- Environment variable pattern detection and sanitization

### 9. Terminal Capability Detection (`internal/terminal/`)

#### Purpose
Provides terminal capability detection functionality to identify terminal color support and interactive execution environments, enabling appropriate output format selection.

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
- **TTY Detection**: Verifies TTY connection status of stdout/stderr
- **Terminal Environment Heuristics**: Identifies terminal-like environments based on TERM environment variable
- **Color Support Detection**: Identifies color-capable terminals based on TERM value
- **User Preference Priority**: Controls priority of command-line arguments and environment variables

#### Security Characteristics
- **Conservative Defaults**: Disables color output for unknown terminals
- **Environment Variable Validation**: Proper parsing of CI environment variables
- **Configuration Priority Control**: Security-conscious configuration inheritance

### 10. Color Management (`internal/color/`)

#### Purpose
Provides safe colored output based on terminal color support capabilities and manages color control sequences appropriately.

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
- **Known Terminal Pattern Matching**: Identifies color-capable terminals like xterm, screen, tmux, etc.
- **Conservative Fallback**: Disables color output for unknown terminals
- **TERM Environment Variable Parsing**: Determines color support based on terminal type
- **User Configuration Integration**: Controls priority of terminal capabilities and user settings

#### Security Characteristics
- **Conservative Approach**: Disables color output for unknown terminals to prevent escape sequence output
- **Validated Patterns**: Enables color only for known color-capable terminals
- **Safe Defaults**: Ensures safe behavior when color support is unknown

### 11. Common Utilities (`internal/common/`, `internal/cmdcommon/`)

#### Purpose
Provides cross-package foundational functionality, ensuring testable and reproducible safe implementations.

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
- Provides mock filesystem for testing, enabling testing with production-equivalent security characteristics
- Supports testing of error conditions and boundary cases

#### Security Guarantees
- Consistent security behavior across implementations
- Comprehensive test coverage of security paths
- Type-safe interface contracts
- Mock implementations preserve security properties

### 12. User and Group Execution Security

#### Purpose
Provides safe user and group switching capabilities while maintaining strict security boundaries and comprehensive audit trail.

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
- Automatic redaction of sensitive data before sending
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
Ensures configuration files and overall system configuration are not tampered with and follows security best practices.

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

**Hash Directory Security Enhancement (Command-Line Argument Removal)**:
```go
// Location: cmd/runner/main.go (after modification)
func getHashDir() string {
    // Always use only default directory in production environment
    // --hash-directory flag completely removed (security vulnerability countermeasure)
    return cmdcommon.DefaultHashDirectory
}
```

**Configuration File Pre-Verification**:
```go
// Location: cmd/runner/main.go (after modification)
// Execute hash verification before loading configuration file
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

**Configuration Validation Timing Improvements**:
- Execute hash verification before loading configuration file
- Completely eliminate system operation with unverified data
- Forced stderr output on verification failure (independent of log level settings)

**Hash Directory Configuration Security Enhancement**:
- Complete removal of `--hash-directory` command-line argument
- Always use only default directory in production environment
- Complete elimination of custom hash directory attack vector
- Maintained testability through test-only API

**Configuration Integrity**:
- TOML format validation
- Required field validation
- Type safety enforcement
- Duplicate group name detection and environment variable inheritance analysis

#### Security Guarantees
- Prevention of configuration tampering
- Safe file and directory permissions
- Prevention of path traversal attacks
- Configuration format validation
- Tampering detection through configuration file pre-verification
- Complete elimination of hash directory attack vector
- Enhanced early validation through absolute path requirements

## Security Architecture Patterns

### Defense-in-Depth

The system implements multiple security layers:

1. **Input Validation**: All inputs validated at entry points (including absolute path requirements)
2. **Pre-Verification**: Hash verification of configuration files before use
3. **Path Security**: Comprehensive path validation and symlink protection, use of secure fixed PATH
4. **File Integrity**: Hash-based verification of all critical files (configuration, executables)
5. **Privilege Control**: Least privilege principle with controlled escalation
6. **Environment Isolation**: Strict allowlist-based environment filtering, elimination of PATH inheritance
7. **Command Validation**: Risk-based command execution control with allowlist validation
8. **Data Protection**: Automatic redaction of sensitive information in all outputs
9. **User/Group Security**: Safe user/group switching with membership validation
10. **Hash Directory Security**: Complete prevention of custom hash directory attacks

### Zero-Trust Model

- No implicit trust in system environment
- All files verified before use
- Environment variables filtered through allowlist
- Commands validated against known-good patterns
- Privileges granted only when needed and immediately revoked

### Fail-Safe Design

- Default deny on all operations
- Emergency shutdown on security failures
- Comprehensive error handling and logging
- Graceful degradation when security features unavailable

### Auditing and Monitoring

- Structured logging with security context
- Privilege operation metrics and tracking
- Security event recording
- Multi-channel reporting of critical errors

## Threat Model and Mitigations

### Filesystem Attacks

**Threats**:
- Symlink attacks
- Path traversal
- TOCTOU race conditions
- File tampering
- System operation manipulation via malicious configuration files
- Verification bypass through custom hash directory

**Mitigations**:
- openat2 with RESOLVE_NO_SYMLINKS
- Step-by-step path validation
- SHA-256 hash validation
- Atomic file operations
- Pre-hash verification of configuration files
- Fixed hash directory default (custom specification completely prohibited)

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
- Privilege escalation via command manipulation
- Malicious binary execution through environment variable PATH

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
- Efficient streaming hash computation
- File size limits to prevent resource exhaustion

### Environment Processing
- O(1) allowlist lookup using map structures
- Compiled regular expressions for pattern matching
- Minimal string manipulation

### Privilege Operations
- Global mutex prevents race conditions but serializes privilege operations
- Fast privilege escalation/restoration using system calls
- Metrics collection for performance monitoring

### Risk Assessment
- Pre-compiled regex patterns for efficient command analysis
- O(1) risk level lookup using pre-compiled patterns
- Minimal overhead for risk evaluation
- Result caching for repeated command analysis

### Data Redaction
- Streaming redaction for large outputs
- Pre-compiled patterns for sensitive data
- Minimal performance impact on normal operations
- Configurable redaction policies

## Deployment Security

### Binary Distribution
- Binaries must have setuid bit set for privilege escalation
- Root ownership required for setuid functionality
- Verify binary integrity before deployment

### Configuration Management
- Hash directory must have secure permissions (755 or less)
- Configuration files should be write-protected
- Regular integrity verification of critical files

### Monitoring and Alerting
- Structured logging of security events
- Syslog integration for centralized logging
- Emergency shutdown events require immediate attention
- Slack integration for real-time security alerts
- Automatic sensitive data redaction across all monitoring channels

## Conclusion

Go Safe Command Runner provides a comprehensive security framework for safe command execution with privilege delegation. The multi-layered approach combines modern security primitives (openat2) with proven security principles (defense-in-depth, zero-trust, fail-safe design) to create a robust system suitable for production use in security-conscious environments.

The implementation demonstrates security engineering best practices including comprehensive input validation, risk-based command control, secure privilege management, automatic sensitive data protection, and extensive auditing capabilities. The system is designed to fail safely and provide complete visibility into security-related operations.

Key security innovations include intelligent risk assessment for command execution, unified resource management with consistent security boundaries, automatic sensitive data redaction across all channels, secure user/group execution capabilities, and comprehensive multi-channel notifications with security-aware messaging. The system provides enterprise-grade security controls while maintaining operational flexibility and transparency.
